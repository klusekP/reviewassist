package services

import (
	"strings"
	"testing"

	"codereviewAI/services/ai"
	"codereviewAI/services/i18n"
)

func TestParseBotNote(t *testing.T) {
	l := LoadReviewConfig().labels()
	body := formatIssue(l, ai.Issue{Type: "security", Severity: "high", Description: "SQL injection", Fix: "use params"}, false)
	sev, typ, desc := parseBotNote(body)
	if sev != "high" || typ != "security" {
		t.Errorf("got %q %q", sev, typ)
	}
	if !strings.HasPrefix(desc, "SQL injection") || !strings.Contains(desc, "use params") {
		t.Errorf("desc = %q", desc)
	}
}

func newTestReReview() *reReview {
	rr := &reReview{source: "gitlab", sinceSHA: "abc"}
	rr.add(previousFinding{File: "calc.go", Line: 11, Severity: "high", Type: "bug", Description: "fixed one"})
	rr.add(previousFinding{File: "calc.go", Line: 40, Severity: "medium", Type: "performance", Description: "still here"})
	rr.add(previousFinding{File: "calc.go", Line: 50, Severity: "low", Type: "style", Description: "author explained"})
	rr.add(previousFinding{File: "other.go", Line: 5, Severity: "low", Type: "bug", Description: "not answered"})
	return rr
}

func TestReReviewStatusesAndDedup(t *testing.T) {
	rr := newTestReReview()
	rr.applyStatuses(i18n.EN, []ai.FindingStatus{
		{ID: "P1", Status: "fixed"},
		{ID: "p2", Status: "open", Line: 42, Comment: "moved"},
		{ID: "P3", Status: "accepted"},
		// P4 missing -> stays open
	})
	want := []string{ai.FindingFixed, ai.FindingOpen, ai.FindingAccepted, ai.FindingOpen}
	for i, f := range rr.findings {
		if f.Status != want[i] {
			t.Errorf("%s status = %s, want %s", f.ID, f.Status, want[i])
		}
	}

	results := []fileResult{{diff: Diff{NewPath: "calc.go"}, review: &ai.CodeReview{Issues: []ai.Issue{
		{File: "calc.go", Line: 11, Type: "bug"},         // previous one was fixed -> a new problem, keep
		{File: "calc.go", Line: 43, Type: "performance"}, // repeats P2 at its current line -> drop
		{File: "calc.go", Line: 44, Type: "bug"},         // near P2 but different type -> keep
		{File: "calc.go", Line: 50, Type: "style"},       // accepted P3 -> drop
	}}}}
	task := &ai.CodeReview{Issues: []ai.Issue{{File: "", Line: 0, Type: "tests"}}}
	rr.dropRepeated(1, results, task)
	got := results[0].review.Issues
	if len(got) != 2 || issueLine(got[0].Line) != 11 || issueLine(got[1].Line) != 44 {
		t.Errorf("kept issues = %+v", got)
	}
	if len(task.Issues) != 1 {
		t.Errorf("general finding must be kept, got %+v", task.Issues)
	}

	open := rr.openIssues()
	if len(open) != 2 || open[0].Line != 42 || open[1].Line != 5 {
		t.Errorf("open issues = %+v", open)
	}
	// still-open medium finding keeps the verdict from being "approve"
	if v := finalVerdict("approve", open, nil); v != VerdictApproveWithComments {
		t.Errorf("verdict = %s", v)
	}

	rr.newFindings = 3
	fu := rr.followup()
	if fu.Fixed != 1 || fu.Open != 2 || fu.Accepted != 1 || fu.New != 3 || len(fu.Items) != 4 {
		t.Errorf("followup = %+v", fu)
	}
}

func TestReReviewPromptSections(t *testing.T) {
	var nilRR *reReview
	if nilRR.fileSection("x.go") != "" || nilRR.agentSection("Polish") != "" {
		t.Error("nil re-review must add nothing to prompts")
	}
	rr := newTestReReview()
	rr.findings[1].Replies = []string{"dev: it's fine, list has max 10 items"}
	sec := rr.fileSection("other.go")
	if !strings.Contains(sec, "[P4]") || strings.Contains(sec, "[P1]") {
		t.Errorf("file section = %s", sec)
	}
	agent := rr.agentSection("Polish")
	if !strings.Contains(agent, "previous_findings") || !strings.Contains(agent, "max 10 items") {
		t.Errorf("agent section = %s", agent)
	}
}
