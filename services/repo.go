package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// RepoManager keeps a bare clone of the GitLab project and creates a throw-away git worktree
// checked out at the MR head for every agent review.
type RepoManager struct {
	gitlab  *GitLabService
	baseDir string
	token   string

	mu       sync.Mutex // serialises clone/fetch/worktree operations on the bare repo
	cloneURL string
}

func NewRepoManager(gl *GitLabService, baseDir string) *RepoManager {
	return &RepoManager{gitlab: gl, baseDir: baseDir, token: os.Getenv("GITLAB_TOKEN")}
}

func (m *RepoManager) bareDir() string {
	return filepath.Join(m.baseDir, fmt.Sprintf("project-%d.git", m.gitlab.projectID))
}

// authArgs passes the token as an HTTP header for a single git command,
// so it is never written to .git/config or shown in clone URLs/logs.
func (m *RepoManager) authArgs() []string {
	cred := base64.StdEncoding.EncodeToString([]byte("oauth2:" + m.token))
	return []string{"-c", "http.extraHeader=Authorization: Basic " + cred}
}

func (m *RepoManager) git(ctx context.Context, withAuth bool, args ...string) (string, error) {
	var full []string
	if withAuth {
		full = append(full, m.authArgs()...)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// PrepareWorktree clones (first time) or fetches the project, fetches the MR source branch and
// target branch, and checks out the MR head commit in a fresh worktree.
// The returned cleanup function removes the worktree.
func (m *RepoManager) PrepareWorktree(ctx context.Context, mr *MergeRequest, refs *DiffRefs) (string, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.baseDir, 0o755); err != nil {
		return "", nil, err
	}
	bare := m.bareDir()

	if m.cloneURL == "" {
		url, err := m.gitlab.GetCloneURL()
		if err != nil {
			return "", nil, err
		}
		m.cloneURL = url
	}

	if _, err := os.Stat(filepath.Join(bare, "HEAD")); os.IsNotExist(err) {
		logrus.Infof("[Repo] Cloning %s into %s (first run, may take a while)", m.cloneURL, bare)
		if _, err := m.git(ctx, true, "clone", "--bare", "--no-tags", m.cloneURL, bare); err != nil {
			return "", nil, err
		}
	}

	// Source branch by name (as requested), target branch for the diff base,
	// and the MR ref as a fallback for MRs from forks.
	refspecs := []string{
		fmt.Sprintf("+refs/heads/%s:refs/heads/%s", mr.SourceBranch, mr.SourceBranch),
		fmt.Sprintf("+refs/heads/%s:refs/heads/%s", mr.TargetBranch, mr.TargetBranch),
	}
	logrus.Infof("[Repo] Fetching branches %s and %s", mr.SourceBranch, mr.TargetBranch)
	if _, err := m.git(ctx, true, append([]string{"--git-dir", bare, "fetch", "--no-tags", "--force", m.cloneURL}, refspecs...)...); err != nil {
		logrus.Warnf("[Repo] Branch fetch failed (%v), falling back to MR ref", err)
	}
	if !m.hasCommit(ctx, refs.HeadSha) || !m.hasCommit(ctx, refs.BaseSha) {
		mrRef := fmt.Sprintf("+refs/merge-requests/%d/head:refs/mr/%d", mr.IID, mr.IID)
		if _, err := m.git(ctx, true, "--git-dir", bare, "fetch", "--no-tags", "--force", m.cloneURL, mrRef, refspecs[1]); err != nil {
			return "", nil, err
		}
	}
	if !m.hasCommit(ctx, refs.HeadSha) {
		return "", nil, fmt.Errorf("MR head commit %s not found after fetch (branch %s pushed?)", shortSha(refs.HeadSha), mr.SourceBranch)
	}
	if !m.hasCommit(ctx, refs.BaseSha) {
		return "", nil, fmt.Errorf("MR base commit %s not found after fetch", shortSha(refs.BaseSha))
	}

	dir := filepath.Join(m.baseDir, "worktrees", fmt.Sprintf("mr-%d-%s-%d", mr.IID, shortSha(refs.HeadSha), time.Now().Unix()))
	if _, err := m.git(ctx, false, "--git-dir", bare, "worktree", "add", "--detach", dir, refs.HeadSha); err != nil {
		return "", nil, err
	}
	logrus.Infof("[Repo] MR %d (%s @ %s) checked out in %s", mr.IID, mr.SourceBranch, shortSha(refs.HeadSha), dir)

	cleanup := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		bg := context.Background()
		if _, err := m.git(bg, false, "--git-dir", bare, "worktree", "remove", "--force", dir); err != nil {
			logrus.Warnf("[Repo] Cannot remove worktree %s: %v", dir, err)
			_ = os.RemoveAll(dir)
			_, _ = m.git(bg, false, "--git-dir", bare, "worktree", "prune")
		}
	}
	return dir, cleanup, nil
}

func (m *RepoManager) hasCommit(ctx context.Context, sha string) bool {
	if sha == "" {
		return false
	}
	_, err := m.git(ctx, false, "--git-dir", m.bareDir(), "cat-file", "-e", sha+"^{commit}")
	return err == nil
}
