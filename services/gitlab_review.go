package services

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/xanzy/go-gitlab"
)

// BotMarker is appended to every note created by this app so previous drafts can be replaced on re-run.
const BotMarker = "<!-- codereviewAI -->"

type DiffRefs struct {
	BaseSha  string
	HeadSha  string
	StartSha string
}

// GetDiffRefs returns the SHAs needed to anchor inline comments to the MR diff.
func (g *GitLabService) GetDiffRefs(iid int) (*DiffRefs, error) {
	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.projectID, iid, &gitlab.GetMergeRequestsOptions{})
	if err != nil {
		return nil, fmt.Errorf("error fetching merge request: %v", err)
	}
	refs := &DiffRefs{BaseSha: mr.DiffRefs.BaseSha, HeadSha: mr.DiffRefs.HeadSha, StartSha: mr.DiffRefs.StartSha}
	if refs.HeadSha == "" {
		refs.HeadSha = mr.SHA
	}
	return refs, nil
}

// GetRawFile returns file content at the given ref.
func (g *GitLabService) GetRawFile(filePath, ref string) (string, error) {
	data, _, err := g.client.RepositoryFiles.GetRawFile(g.projectID, filePath, &gitlab.GetRawFileOptions{Ref: &ref})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListTree lists repository paths (files and dirs) under dir. maxPaths <= 0 means no limit.
func (g *GitLabService) ListTree(dir, ref string, recursive bool, maxPaths int) ([]*gitlab.TreeNode, error) {
	opts := &gitlab.ListTreeOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
		Ref:         &ref,
		Recursive:   &recursive,
	}
	if dir != "" {
		opts.Path = &dir
	}
	var all []*gitlab.TreeNode
	for {
		nodes, resp, err := g.client.Repositories.ListTree(g.projectID, opts)
		if err != nil {
			return all, err
		}
		all = append(all, nodes...)
		if (maxPaths > 0 && len(all) >= maxPaths) || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

type notePosition struct {
	PositionType string `json:"position_type"`
	BaseSha      string `json:"base_sha"`
	StartSha     string `json:"start_sha"`
	HeadSha      string `json:"head_sha"`
	OldPath      string `json:"old_path"`
	NewPath      string `json:"new_path"`
	NewLine      int    `json:"new_line,omitempty"`
	OldLine      int    `json:"old_line,omitempty"`
}

// InlineNote is a comment to publish on the MR. Without NewPath it is a general (non-inline) note.
type InlineNote struct {
	Body    string
	OldPath string
	NewPath string
	NewLine int
	OldLine int
}

// PublishNote creates either a draft note (pending review, visible only to the token owner until
// "Submit review" in GitLab) or a regular discussion, depending on mode.
func (g *GitLabService) PublishNote(iid int, refs *DiffRefs, n InlineNote, mode string) error {
	body := n.Body + "\n\n" + BotMarker
	var pos *notePosition
	if n.NewPath != "" && refs != nil {
		pos = &notePosition{
			PositionType: "text",
			BaseSha:      refs.BaseSha,
			StartSha:     refs.StartSha,
			HeadSha:      refs.HeadSha,
			OldPath:      n.OldPath,
			NewPath:      n.NewPath,
			NewLine:      n.NewLine,
			OldLine:      n.OldLine,
		}
	}

	var urlPath string
	var payload interface{}
	if mode == "comment" {
		urlPath = fmt.Sprintf("projects/%d/merge_requests/%d/discussions", g.projectID, iid)
		payload = struct {
			Body     string        `json:"body"`
			Position *notePosition `json:"position,omitempty"`
		}{body, pos}
	} else {
		urlPath = fmt.Sprintf("projects/%d/merge_requests/%d/draft_notes", g.projectID, iid)
		payload = struct {
			Note     string        `json:"note"`
			Position *notePosition `json:"position,omitempty"`
		}{body, pos}
	}

	req, err := g.client.NewRequest(http.MethodPost, urlPath, payload, nil)
	if err != nil {
		return err
	}
	_, err = g.client.Do(req, nil)
	return err
}

type draftNote struct {
	ID   int    `json:"id"`
	Note string `json:"note"`
}

// DeleteBotDrafts removes draft notes created by this app earlier (identified by BotMarker),
// so re-running a review does not duplicate comments. User's own drafts are left untouched.
func (g *GitLabService) DeleteBotDrafts(iid int) (int, error) {
	req, err := g.client.NewRequest(http.MethodGet, fmt.Sprintf("projects/%d/merge_requests/%d/draft_notes", g.projectID, iid),
		&gitlab.ListOptions{PerPage: 100}, nil)
	if err != nil {
		return 0, err
	}
	var drafts []draftNote
	if _, err := g.client.Do(req, &drafts); err != nil {
		return 0, err
	}
	deleted := 0
	for _, d := range drafts {
		if !strings.Contains(d.Note, BotMarker) {
			continue
		}
		del, err := g.client.NewRequest(http.MethodDelete, fmt.Sprintf("projects/%d/merge_requests/%d/draft_notes/%d", g.projectID, iid, d.ID), nil, nil)
		if err != nil {
			return deleted, err
		}
		if _, err := g.client.Do(del, nil); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// GetCloneURL returns the HTTPS clone URL of the configured project.
func (g *GitLabService) GetCloneURL() (string, error) {
	p, _, err := g.client.Projects.GetProject(g.projectID, nil)
	if err != nil {
		return "", fmt.Errorf("error fetching project: %v", err)
	}
	return p.HTTPURLToRepo, nil
}

// GetMergeRequestCommitMessages returns titles and messages of the MR commits (used to find Jira keys).
func (g *GitLabService) GetMergeRequestCommitMessages(iid int) ([]string, error) {
	commits, _, err := g.client.MergeRequests.GetMergeRequestCommits(g.projectID, iid, &gitlab.GetMergeRequestCommitsOptions{PerPage: 100})
	if err != nil {
		return nil, err
	}
	var msgs []string
	for _, c := range commits {
		msgs = append(msgs, c.Message)
	}
	return msgs, nil
}
