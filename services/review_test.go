package services

import (
	"codereviewAI/services/i18n"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const sampleDiff = `@@ -10,4 +10,5 @@ func a() {
 	x := 1
-	y := 2
+	y := 3
+	z := 4
 	return x
@@ -40,2 +41,3 @@ func b() {
 	foo()
+	bar()
 }`

func TestParseDiffLineNumbers(t *testing.T) {
	d := ParseDiff(sampleDiff)

	for _, l := range []int{11, 12, 42} {
		if !d.Added[l] {
			t.Errorf("expected line %d to be added, got %v", l, d.Added)
		}
	}
	if len(d.Added) != 3 {
		t.Errorf("expected 3 added lines, got %v", d.Added)
	}
	if old := d.Context[10]; old != 10 {
		t.Errorf("context line 10 should map to old 10, got %d", old)
	}
	if old := d.Context[13]; old != 12 {
		t.Errorf("context line 13 should map to old 12, got %d", old)
	}
	if old := d.Context[43]; old != 41 {
		t.Errorf("context line 43 should map to old 41, got %d", old)
	}
	if !strings.Contains(d.Numbered, "   42 + \tbar()") {
		t.Errorf("numbered diff missing annotated line:\n%s", d.Numbered)
	}
}

func TestResolveLine(t *testing.T) {
	d := ParseDiff(sampleDiff)
	cases := []struct {
		in, newLine, oldLine int
		ok                   bool
	}{
		{11, 11, 0, true},  // exact added line
		{14, 12, 0, true},  // off by two -> nearest added
		{43, 42, 0, true},  // off by one -> added line wins over context
		{100, 0, 0, false}, // outside diff
		{13, 12, 0, true},  // context line next to an added line -> snapped to the change
		{30, 0, 0, false},  // line far from any change -> out of scope
		{0, 0, 0, false},
	}
	for _, c := range cases {
		n, o, ok := d.ResolveLine(c.in)
		if n != c.newLine || o != c.oldLine || ok != c.ok {
			t.Errorf("ResolveLine(%d) = (%d,%d,%v), want (%d,%d,%v)", c.in, n, o, ok, c.newLine, c.oldLine, c.ok)
		}
	}
}

func TestIssueLine(t *testing.T) {
	for in, want := range map[interface{}]int{float64(12): 12, "15": 15, "L7": 7, "20-25": 20, nil: 0, "abc": 0} {
		if got := issueLine(in); got != want {
			t.Errorf("issueLine(%v) = %d, want %d", in, got, want)
		}
	}
}

func TestFindIssueKeys(t *testing.T) {
	j := &JiraService{keyPattern: regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)}
	got := j.FindIssueKeys("SHOP-123 Ulubione produkty", "feature/shop-123-favorites", "Closes SHOP-124, refs SHOP-123")
	want := []string{"SHOP-123", "SHOP-124"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindIssueKeys = %v, want %v", got, want)
	}
}

func TestIsIgnored(t *testing.T) {
	cfg := &ReviewConfig{IgnoreGlobs: defaultIgnoreGlobs}
	for path, want := range map[string]bool{
		"composer.lock":                   true,
		"frontend/package-lock.json":      true,
		"vendor/symfony/x.php":            true,
		"app/node_modules/x/index.js":     true,
		"public/app.min.js":               true,
		"src/Controller/Product.php":      false,
		"config/api_platform/Product.xml": false,
	} {
		if got := cfg.IsIgnored(path); got != want {
			t.Errorf("IsIgnored(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestTruncateKeepsUTF8(t *testing.T) {
	s := truncate("zażółć gęślą jaźń", 4, "…")
	if !strings.HasPrefix(s, "za") || strings.ContainsRune(s[:4], '�') {
		t.Errorf("unexpected truncate result %q", s)
	}
}

func TestFinalVerdict(t *testing.T) {
	high := []Issue{{Severity: "high"}}
	low := []Issue{{Severity: "low"}}
	cases := []struct {
		model  string
		issues []Issue
		want   string
	}{
		{"approve", nil, VerdictApprove},
		{"approve", high, VerdictRequestChanges},        // model too lenient -> overridden
		{"approve", low, VerdictApproveWithComments},    // low issues -> at least with comments
		{"request_changes", low, VerdictRequestChanges}, // model may be stricter
		{"", nil, VerdictApprove},                       // missing verdict
		{"approve_with_comments", []Issue{{Severity: "critical"}}, VerdictRequestChanges},
	}
	for _, c := range cases {
		if got := finalVerdict(c.model, c.issues, nil); got != c.want {
			t.Errorf("finalVerdict(%q, %v) = %q, want %q", c.model, c.issues, got, c.want)
		}
	}
}

func TestApplyScope(t *testing.T) {
	lines := ParseDiff(sampleDiff)
	results := []fileResult{{diff: Diff{NewPath: "calc.go"}, lines: lines, review: &CodeReview{Issues: []Issue{
		{File: "calc.go", Line: float64(11), Description: "changed line"},
		{File: "calc.go", Line: float64(13), Description: "off by one -> snapped to 12"},
		{File: "calc.go", Line: float64(30), Description: "unchanged code far away"},
		{File: "calc.go", Line: float64(200), Description: "outside the diff"},
	}}}}
	task := &CodeReview{Issues: []Issue{
		{File: "", Description: "missing tests"},
		{File: "composer.lock", Description: "changed but not reviewed file"},
		{File: "./calc.go", Description: "file-level"},
		{File: "src/Legacy.php", Description: "unrelated file"},
	}}
	applyScope(1, results, task, []Diff{{NewPath: "calc.go", OldPath: "calc.go"}, {NewPath: "composer.lock", OldPath: "composer.lock"}})

	got := results[0].review.Issues
	if len(got) != 2 || issueLine(got[0].Line) != 11 || issueLine(got[1].Line) != 12 {
		t.Errorf("file issues after scope = %+v", got)
	}
	if len(task.Issues) != 3 {
		t.Errorf("MR-level issues after scope = %+v", task.Issues)
	}
	for _, is := range task.Issues {
		if is.File == "src/Legacy.php" {
			t.Error("unrelated file finding was kept")
		}
	}
}

func TestFinalVerdictRequirements(t *testing.T) {
	if got := finalVerdict("approve", nil, []Requirement{{Status: "done"}, {Status: "missing"}}); got != VerdictRequestChanges {
		t.Errorf("missing requirement -> %s, want request_changes", got)
	}
	if got := finalVerdict("approve", nil, []Requirement{{Status: "partial"}}); got != VerdictApproveWithComments {
		t.Errorf("partial requirement -> %s, want approve_with_comments", got)
	}
	if got := finalVerdict("approve", nil, []Requirement{{Status: "done"}, {Status: "unclear"}}); got != VerdictApprove {
		t.Errorf("done+unclear -> %s, want approve", got)
	}
}

func TestRequirementsLineAndTableCell(t *testing.T) {
	reqs := []Requirement{{Status: "done"}, {Status: "done"}, {Status: "partial"}, {Status: "missing"}}
	for lang, want := range map[string]string{
		i18n.PL: "Wymagania: 2/4 zrealizowane, 1 częściowo, 1 brak",
		i18n.EN: "Requirements: 2/4 done, 1 partial, 1 missing",
		i18n.DE: "Anforderungen: 2/4 umgesetzt, 1 teilweise, 1 fehlt",
	} {
		if line := requirementsLine(lang, reqs); line != want {
			t.Errorf("requirementsLine(%s) = %q, want %q", lang, line, want)
		}
	}
	if got := tableCell("a | b\nc"); got != `a \| b<br>c` {
		t.Errorf("tableCell = %q", got)
	}
}
