package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestPrepareWorktreeChecksOutSourceBranch(t *testing.T) {
	remote := t.TempDir()
	gitT(t, remote, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(remote, "a.txt"), []byte("v1\n"), 0o644)
	gitT(t, remote, "add", ".")
	gitT(t, remote, "commit", "-qm", "base")
	base := gitT(t, remote, "rev-parse", "HEAD")
	gitT(t, remote, "checkout", "-qb", "feature/SHOP-1-x")
	os.WriteFile(filepath.Join(remote, "a.txt"), []byte("v2\n"), 0o644)
	gitT(t, remote, "commit", "-qam", "change")
	head := gitT(t, remote, "rev-parse", "HEAD")
	gitT(t, remote, "checkout", "-q", "main")

	m := &RepoManager{gitlab: &GitLabService{projectID: 42}, baseDir: t.TempDir(), cloneURL: "file://" + remote}
	mr := &MergeRequest{IID: 7, SourceBranch: "feature/SHOP-1-x", TargetBranch: "main"}
	refs := &DiffRefs{BaseSha: base, HeadSha: head, StartSha: base}

	for run := 0; run < 2; run++ { // second run exercises the fetch-only path
		dir, cleanup, err := m.PrepareWorktree(context.Background(), mr, refs)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(got) != "v2\n" {
			t.Errorf("run %d: worktree has %q, want MR head content", run, got)
		}
		if got := gitT(t, dir, "rev-parse", "HEAD"); got != head {
			t.Errorf("run %d: HEAD %s, want %s", run, got, head)
		}
		if diff := gitT(t, dir, "diff", "--name-only", base, "HEAD"); diff != "a.txt" {
			t.Errorf("run %d: diff vs base = %q", run, diff)
		}
		cleanup()
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("run %d: worktree not removed", run)
		}
	}
}

func TestSplitAgentReview(t *testing.T) {
	diffs := []Diff{{NewPath: "calc.go", Diff: sampleDiff}}
	review := &CodeReview{Summary: "ok", Score: 6, Issues: []Issue{
		{File: "calc.go", Line: float64(11), Description: "inline"},
		{File: "./calc.go", Line: float64(42), Description: "inline with ./"},
		{File: "", Line: float64(0), Description: "MR-level"},
		{File: "other.go", Line: float64(3), Description: "not reviewed file"},
	}}
	results, task := splitAgentReview(review, diffs)
	if n := len(results[0].review.Issues); n != 2 {
		t.Errorf("file issues = %d, want 2", n)
	}
	if n := len(task.Issues); n != 2 {
		t.Errorf("MR-level issues = %d, want 2", n)
	}
	if task.Summary != "ok" || task.Score != 6 {
		t.Errorf("task review = %+v", task)
	}
}
