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

// BotThread is an inline discussion started by this app (a finding of an earlier review).
type BotThread struct {
	DiscussionID string
	NoteID       int
	Body         string // first note without BotMarker
	File         string
	Line         int
	HeadSha      string // MR head the finding was made on
	Resolved     bool
	Replies      []string // "author: text" of later notes written by people
}

// ListBotThreads returns inline discussions whose first note was created by this app.
func (g *GitLabService) ListBotThreads(iid int) ([]BotThread, error) {
	opts := &gitlab.ListMergeRequestDiscussionsOptions{PerPage: 100, Page: 1}
	var out []BotThread
	for {
		discussions, resp, err := g.client.Discussions.ListMergeRequestDiscussions(g.projectID, iid, opts)
		if err != nil {
			return nil, err
		}
		for _, d := range discussions {
			if len(d.Notes) == 0 {
				continue
			}
			first := d.Notes[0]
			if first.System || first.Position == nil || !strings.Contains(first.Body, BotMarker) {
				continue
			}
			t := BotThread{
				DiscussionID: d.ID,
				NoteID:       first.ID,
				Body:         strings.TrimSpace(strings.ReplaceAll(first.Body, BotMarker, "")),
				File:         first.Position.NewPath,
				Line:         first.Position.NewLine,
				HeadSha:      first.Position.HeadSHA,
				Resolved:     first.Resolvable && first.Resolved,
			}
			for _, n := range d.Notes[1:] {
				if n.System || strings.Contains(n.Body, BotMarker) {
					continue
				}
				t.Replies = append(t.Replies, n.Author.Username+": "+strings.TrimSpace(n.Body))
			}
			out = append(out, t)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

// ReplyToThread answers a bot discussion and optionally resolves it. In draft mode the reply is a
// draft note (resolution happens on "Submit review"); otherwise it is published immediately.
func (g *GitLabService) ReplyToThread(iid int, discussionID, body string, resolve bool, mode string) error {
	body += "\n\n" + BotMarker
	if mode != "comment" {
		payload := struct {
			Note                  string `json:"note"`
			InReplyToDiscussionID string `json:"in_reply_to_discussion_id"`
			ResolveDiscussion     bool   `json:"resolve_discussion,omitempty"`
		}{body, discussionID, resolve}
		req, err := g.client.NewRequest(http.MethodPost, fmt.Sprintf("projects/%d/merge_requests/%d/draft_notes", g.projectID, iid), payload, nil)
		if err != nil {
			return err
		}
		_, err = g.client.Do(req, nil)
		return err
	}
	if _, _, err := g.client.Discussions.AddMergeRequestDiscussionNote(g.projectID, iid, discussionID,
		&gitlab.AddMergeRequestDiscussionNoteOptions{Body: &body}); err != nil {
		return err
	}
	if !resolve {
		return nil
	}
	resolved := true
	_, _, err := g.client.Discussions.ResolveMergeRequestDiscussion(g.projectID, iid, discussionID,
		&gitlab.ResolveMergeRequestDiscussionOptions{Resolved: &resolved})
	return err
}

// CompareDiffs returns file diffs between two commits (e.g. previous review head and current head).
func (g *GitLabService) CompareDiffs(from, to string) ([]Diff, error) {
	cmp, _, err := g.client.Repositories.Compare(g.projectID, &gitlab.CompareOptions{From: &from, To: &to})
	if err != nil {
		return nil, err
	}
	out := make([]Diff, 0, len(cmp.Diffs))
	for _, d := range cmp.Diffs {
		out = append(out, Diff{OldPath: d.OldPath, NewPath: d.NewPath, Diff: d.Diff,
			NewFile: d.NewFile, RenamedFile: d.RenamedFile, DeletedFile: d.DeletedFile})
	}
	return out, nil
}
