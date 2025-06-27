package services

import (
	"fmt"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/xanzy/go-gitlab"
)

type GitLabService struct {
	client    *gitlab.Client
	projectID int
}

type MergeRequest struct {
	ID          int    `json:"id"`
	IID         int    `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	WebURL       string `json:"web_url"`
}

type Diff struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	AMode       string `json:"a_mode"`
	BMode       string `json:"b_mode"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

type Project struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	WebURL      string `json:"web_url"`
}

func NewGitLabService() *GitLabService {
	token := os.Getenv("GITLAB_TOKEN")
	gitlabURL := os.Getenv("GITLAB_URL")

	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}

	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(gitlabURL))
	if err != nil {
		logrus.Fatalf("Błąd tworzenia klienta GitLab: %v", err)
	}

	projectIDStr := os.Getenv("GITLAB_PROJECT_ID")
	projectID, err := strconv.Atoi(projectIDStr)
	if err != nil {
		logrus.Fatalf("Nieprawidłowy GITLAB_PROJECT_ID: %v", err)
	}

	return &GitLabService{
		client:    client,
		projectID: projectID,
	}
}

func (g *GitLabService) GetMergeRequests(state string) ([]MergeRequest, error) {
	opts := &gitlab.ListProjectMergeRequestsOptions{
		State: &state,
	}

	mrs, _, err := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, opts)
	if err != nil {
		return nil, fmt.Errorf("błąd pobierania merge requests: %v", err)
	}

	var result []MergeRequest
	for _, mr := range mrs {
		result = append(result, MergeRequest{
			ID:          mr.ID,
			IID:         mr.IID,
			Title:       mr.Title,
			Description: mr.Description,
			State:       mr.State,
			Author: struct {
				Name string `json:"name"`
			}{
				Name: mr.Author.Name,
			},
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			CreatedAt:    mr.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    mr.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			WebURL:       mr.WebURL,
		})
	}

	return result, nil
}

func (g *GitLabService) GetMergeRequest(iid int) (*MergeRequest, error) {
	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.projectID, iid, &gitlab.GetMergeRequestsOptions{})
	if err != nil {
		return nil, fmt.Errorf("błąd pobierania merge request: %v", err)
	}

	return &MergeRequest{
		ID:          mr.ID,
		IID:         mr.IID,
		Title:       mr.Title,
		Description: mr.Description,
		State:       mr.State,
		Author: struct {
			Name string `json:"name"`
		}{
			Name: mr.Author.Name,
		},
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		CreatedAt:    mr.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    mr.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		WebURL:       mr.WebURL,
	}, nil
}

func (g *GitLabService) GetMergeRequestDiffs(iid int) ([]Diff, error) {
	diffs, _, err := g.client.MergeRequests.ListMergeRequestDiffs(g.projectID, iid, &gitlab.ListMergeRequestDiffsOptions{})
	if err != nil {
		return nil, fmt.Errorf("błąd pobierania diff: %v", err)
	}

	var result []Diff
	for _, diff := range diffs {
		result = append(result, Diff{
			OldPath:     diff.OldPath,
			NewPath:     diff.NewPath,
			AMode:       diff.AMode,
			BMode:       diff.BMode,
			Diff:        diff.Diff,
			NewFile:     diff.NewFile,
			RenamedFile: diff.RenamedFile,
			DeletedFile: diff.DeletedFile,
		})
	}

	return result, nil
}

func (g *GitLabService) AddComment(iid int, body string, line int, path string) error {
	opts := &gitlab.CreateMergeRequestNoteOptions{
		Body: &body,
	}

	// Uproszczone dodawanie komentarza - bez pozycjonowania
	_, _, err := g.client.Notes.CreateMergeRequestNote(g.projectID, iid, opts)
	if err != nil {
		return fmt.Errorf("błąd dodawania komentarza: %v", err)
	}

	return nil
}

func (g *GitLabService) GetProjects() ([]Project, error) {
	opts := &gitlab.ListProjectsOptions{
		Membership: gitlab.Bool(true),
	}

	projects, _, err := g.client.Projects.ListProjects(opts)
	if err != nil {
		return nil, fmt.Errorf("błąd pobierania projektów: %v", err)
	}

	var result []Project
	for _, project := range projects {
		result = append(result, Project{
			ID:          project.ID,
			Name:        project.Name,
			Description: project.Description,
			WebURL:      project.WebURL,
		})
	}

	return result, nil
}
