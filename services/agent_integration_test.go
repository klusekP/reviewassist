//go:build agent

package services

import (
	"codereviewAI/services/i18n"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Runs the real Claude Code CLI with the app's real review prompt on a small repo
// containing a deliberately insecure change. Usage:
//
//	go test -tags agent -run TestAgentSecurityReview -v ./services/
func TestAgentSecurityReview(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, content string) {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-q", "-b", "main")
	write("README.md", "# Shop API\nAll /api/orders endpoints must check that the order belongs to the logged-in user (middleware.CurrentUser).\n")
	write("orders.go", "package shop\n\nimport \"database/sql\"\n\ntype Store struct{ db *sql.DB }\n")
	// pre-existing, unchanged and unrelated problem - must NOT be reported
	write("legacy.go", "package shop\n\nimport \"database/sql\"\n\nfunc FindUser(db *sql.DB, name string) (*sql.Rows, error) {\n\treturn db.Query(\"SELECT * FROM users WHERE name = '\" + name + \"'\")\n}\n")
	write("util.go", "package shop\n\n// Unique returns ids without duplicates, preserving order.\nfunc Unique(ids []int) []int {\n\tseen := make(map[int]bool, len(ids))\n\tout := make([]int, 0, len(ids))\n\tfor _, id := range ids {\n\t\tif !seen[id] {\n\t\t\tseen[id] = true\n\t\t\tout = append(out, id)\n\t\t}\n\t}\n\treturn out\n}\n")
	run("add", ".")
	run("commit", "-qm", "base")
	base := run("rev-parse", "HEAD")

	write("orders.go", `package shop

import (
	"database/sql"
	"net/http"
)

type Store struct{ db *sql.DB }

// GET /api/orders/export?id=...&status=...
func (s *Store) ExportOrder(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")
	rows, err := s.db.Query("SELECT id, email, total FROM orders WHERE id = " + id + " AND status = '" + status + "'")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	w.Write([]byte("ok"))
}

// OrderTotals returns the total of each distinct order, called from the dashboard on every request.
func (s *Store) OrderTotals(ids []int) map[int]float64 {
	var distinct []int
	for _, id := range ids {
		found := false
		for _, d := range distinct {
			if d == id {
				found = true
			}
		}
		if !found {
			distinct = append(distinct, id)
		}
	}
	totals := map[int]float64{}
	for _, id := range distinct {
		var total float64
		s.db.QueryRow("SELECT total FROM orders WHERE id = ?", id).Scan(&total)
		totals[id] = total
	}
	return totals
}
`)
	run("commit", "-qam", "SHOP-1 export order")
	head := run("rev-parse", "HEAD")
	diff := run("diff", base, head, "--", "orders.go")
	// strip the file header, keep hunks (GitLab diff format)
	diff = diff[strings.Index(diff, "@@"):]

	r := &ReviewService{config: &ReviewConfig{Language: "pl", AgentBin: "claude", AgentMaxTurns: 30, AgentTimeoutMin: 8}}
	mr := &MergeRequest{IID: 1, Title: "SHOP-1 Eksport zamówienia", SourceBranch: "feature/SHOP-1", TargetBranch: "main"}
	diffs := []Diff{{OldPath: "orders.go", NewPath: "orders.go", Diff: diff}}
	prompt := r.buildAgentPrompt(mr, &DiffRefs{BaseSha: base, HeadSha: head}, diffs,
		"### SHOP-1: Eksport zamówienia\nType: Story | Status: In Progress\n\nDescription:\n"+
			"- Użytkownik może pobrać dane swojego zamówienia (id, email, total) w formacie JSON.\n"+
			"- Eksport dostępny tylko dla zamówień o statusie 'paid'.\n"+
			"- Próba pobrania cudzego zamówienia zwraca 404.\n\n"+
			"Comments (oldest first):\n- [Product Owner, 2026-10-01]: Zmiana ustaleń: zamiast JSON eksportujemy CSV z nagłówkiem.\n", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	res, err := runClaude(ctx, r.config, dir, prompt)
	if err != nil {
		t.Fatal(err)
	}
	review, err := parseAgentReview(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range review.Issues {
		if strings.Contains(is.File, "legacy.go") || strings.Contains(is.Description, "FindUser") {
			t.Logf("NOTE: agent reported out-of-scope legacy code (will be filtered): %s", is.Description)
		}
	}
	results, task := splitAgentReview(i18n.EN, review, diffs)
	applyScope(1, results, task, diffs)
	for _, is := range append(task.Issues, results[0].review.Issues...) {
		if strings.Contains(is.File, "legacy.go") {
			t.Errorf("out-of-scope finding survived the filter: %+v", is)
		}
	}
	verdict := finalVerdict(task.Verdict, append(task.Issues, results[0].review.Issues...), task.Requirements)

	t.Logf("turns=%d cost=$%.2f verdict(model)=%s verdict(final)=%s score=%d", res.NumTurns, res.TotalCostUSD, review.Verdict, verdict, review.Score)
	t.Logf("SECURITY: %s", review.SecuritySummary)
	t.Logf("QUALITY: %s", review.QualitySummary)
	t.Logf("%s", requirementsLine(i18n.EN, task.Requirements))
	for _, rq := range task.Requirements {
		t.Logf("REQ %s %s | %s", requirementIcon[rq.Status], rq.Requirement, rq.Evidence)
	}
	security, perf := 0, 0
	for _, is := range append(task.Issues, results[0].review.Issues...) {
		if is.Type == "performance" || is.Type == "refactoring" {
			perf++
		}
		t.Logf("[%s/%s] %s:%v %s", is.Severity, is.Type, is.File, is.Line, is.Description)
		if is.Type == "security" {
			security++
		}
	}
	if security == 0 {
		t.Error("expected security findings (SQL injection, missing ownership check)")
	}
	if verdict != VerdictRequestChanges {
		t.Errorf("verdict = %s, want request_changes", verdict)
	}
	if perf == 0 {
		t.Error("expected performance/refactoring findings (O(n^2) dedupe duplicating Unique, N+1 queries)")
	}
	if len(task.Requirements) < 3 {
		t.Errorf("expected at least 3 requirements, got %d", len(task.Requirements))
	}
	if review.QualitySummary == "" {
		t.Error("quality_summary is empty")
	}
	if review.SecuritySummary == "" {
		t.Error("security_summary is empty")
	}
}
