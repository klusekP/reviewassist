package services

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/xanzy/go-gitlab"
)

// GitLabService talks to the GitLab API. It is bound to one project; use ForProject to work on another.
type GitLabService struct {
	client    *gitlab.Client
	projectID int
	baseURL   string

	projects *projectCache // shared between ForProject copies
}

type projectCache struct {
	mu      sync.Mutex
	items   []Project
	fetched time.Time
}

type MergeRequest struct {
	ID          int    `json:"id"`
	IID         int    `json:"iid"`
	ProjectID   int    `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	Author      struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"author"`
	SourceBranch        string   `json:"source_branch"`
	TargetBranch        string   `json:"target_branch"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
	WebURL              string   `json:"web_url"`
	Draft               bool     `json:"draft"`
	Labels              []string `json:"labels"`
	DetailedMergeStatus string   `json:"detailed_merge_status"`
	HasConflicts        bool     `json:"has_conflicts"`
	UserNotesCount      int      `json:"user_notes_count"`
	SHA                 string   `json:"sha"`
	PipelineStatus      string   `json:"pipeline_status,omitempty"` // only on single MR fetch
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
	ID                int    `json:"id"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace"`
	PathWithNamespace string `json:"path_with_namespace"`
	Description       string `json:"description"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch"`
	LastActivityAt    string `json:"last_activity_at"`
	OpenMRs           int    `json:"open_mrs"`

	rootGroup string // top-level group path, empty for personal namespaces
}

func NewGitLabService() *GitLabService {
	token := os.Getenv("GITLAB_TOKEN")
	gitlabURL := os.Getenv("GITLAB_URL")

	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}

	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(gitlabURL))
	if err != nil {
		logrus.Fatalf("Error creating GitLab client: %v", err)
	}

	// GITLAB_PROJECT_ID is optional now: it is the default project for the legacy /merge-requests routes.
	projectID := 0
	if s := strings.TrimSpace(os.Getenv("GITLAB_PROJECT_ID")); s != "" {
		if projectID, err = strconv.Atoi(s); err != nil {
			logrus.Fatalf("Invalid GITLAB_PROJECT_ID: %v", err)
		}
	}

	return &GitLabService{
		client:    client,
		projectID: projectID,
		baseURL:   strings.TrimRight(gitlabURL, "/"),
		projects:  &projectCache{},
	}
}

// ForProject returns a copy of the service bound to another project.
func (g *GitLabService) ForProject(projectID int) *GitLabService {
	c := *g
	c.projectID = projectID
	return &c
}

func (g *GitLabService) ProjectID() int { return g.projectID }

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func toMergeRequest(mr *gitlab.MergeRequest) MergeRequest {
	out := MergeRequest{
		ID:                  mr.ID,
		IID:                 mr.IID,
		ProjectID:           mr.ProjectID,
		Title:               mr.Title,
		Description:         mr.Description,
		State:               mr.State,
		SourceBranch:        mr.SourceBranch,
		TargetBranch:        mr.TargetBranch,
		CreatedAt:           formatTime(mr.CreatedAt),
		UpdatedAt:           formatTime(mr.UpdatedAt),
		WebURL:              mr.WebURL,
		Draft:               mr.Draft || mr.WorkInProgress,
		Labels:              []string(mr.Labels),
		DetailedMergeStatus: mr.DetailedMergeStatus,
		HasConflicts:        mr.HasConflicts,
		UserNotesCount:      mr.UserNotesCount,
		SHA:                 mr.SHA,
	}
	if mr.Author != nil {
		out.Author.Name = mr.Author.Name
		out.Author.Username = mr.Author.Username
	}
	if out.Labels == nil {
		out.Labels = []string{}
	}
	if mr.HeadPipeline != nil {
		out.PipelineStatus = mr.HeadPipeline.Status
	}
	return out
}

// GetMergeRequests lists MRs of the project. state: opened | merged | closed | all.
func (g *GitLabService) GetMergeRequests(state string) ([]MergeRequest, error) {
	orderBy := "updated_at"
	opts := &gitlab.ListProjectMergeRequestsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
		State:       &state,
		OrderBy:     &orderBy,
	}

	result := []MergeRequest{}
	for {
		mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, opts)
		if err != nil {
			return nil, fmt.Errorf("error fetching merge requests: %v", err)
		}
		for _, mr := range mrs {
			result = append(result, toMergeRequest(mr))
		}
		// opened MRs: all of them; merged/closed history: the most recent 100 is enough for the UI
		if resp.NextPage == 0 || state != "opened" || len(result) >= 500 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (g *GitLabService) GetMergeRequest(iid int) (*MergeRequest, error) {
	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.projectID, iid, &gitlab.GetMergeRequestsOptions{})
	if err != nil {
		return nil, fmt.Errorf("error fetching merge request: %v", err)
	}
	out := toMergeRequest(mr)
	return &out, nil
}

func (g *GitLabService) GetMergeRequestDiffs(iid int) ([]Diff, error) {
	opts := &gitlab.ListMergeRequestDiffsOptions{PerPage: 100, Page: 1}
	var diffs []*gitlab.MergeRequestDiff
	for {
		page, resp, err := g.client.MergeRequests.ListMergeRequestDiffs(g.projectID, iid, opts)
		if err != nil {
			return nil, fmt.Errorf("error fetching diff: %v", err)
		}
		diffs = append(diffs, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
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

	_, _, err := g.client.Notes.CreateMergeRequestNote(g.projectID, iid, opts)
	if err != nil {
		return fmt.Errorf("error adding comment: %v", err)
	}

	return nil
}

func toProject(p *gitlab.Project) Project {
	root := ""
	if p.Namespace != nil && p.Namespace.Kind == "group" {
		root = strings.SplitN(p.Namespace.FullPath, "/", 2)[0]
	}
	return Project{
		rootGroup:         root,
		ID:                p.ID,
		Name:              p.Name,
		NameWithNamespace: p.NameWithNamespace,
		PathWithNamespace: p.PathWithNamespace,
		Description:       p.Description,
		WebURL:            p.WebURL,
		DefaultBranch:     p.DefaultBranch,
		LastActivityAt:    formatTime(p.LastActivityAt),
	}
}

// GetProjects returns the projects shown in the sidebar, with the number of open MRs.
// GITLAB_PROJECTS (comma separated IDs or paths) restricts the list; otherwise all projects
// the token user is a member of (up to GITLAB_PROJECTS_LIMIT) are returned.
// Results are cached for 5 minutes unless refresh is true.
func (g *GitLabService) GetProjects(refresh bool) ([]Project, error) {
	g.projects.mu.Lock()
	defer g.projects.mu.Unlock()
	if !refresh && g.projects.items != nil && time.Since(g.projects.fetched) < 5*time.Minute {
		return g.projects.items, nil
	}

	start := time.Now()
	var projects []Project
	seen := map[int]bool{}
	add := func(p *gitlab.Project) {
		if p != nil && !seen[p.ID] {
			seen[p.ID] = true
			projects = append(projects, toProject(p))
		}
	}

	if configured := envList("GITLAB_PROJECTS", nil); len(configured) > 0 {
		for _, ref := range configured {
			var pid interface{} = ref
			if n, err := strconv.Atoi(ref); err == nil {
				pid = n
			}
			p, _, err := g.client.Projects.GetProject(pid, nil)
			if err != nil {
				logrus.Warnf("[GitLab] Cannot load project %q from GITLAB_PROJECTS: %v", ref, err)
				continue
			}
			add(p)
		}
	} else {
		limit := envInt("GITLAB_PROJECTS_LIMIT", 100)
		orderBy := "last_activity_at"
		opts := &gitlab.ListProjectsOptions{
			ListOptions:              gitlab.ListOptions{PerPage: 100, Page: 1},
			Membership:               gitlab.Bool(true),
			Archived:                 gitlab.Bool(false),
			WithMergeRequestsEnabled: gitlab.Bool(true),
			Simple:                   gitlab.Bool(true),
			OrderBy:                  &orderBy,
		}
		for len(projects) < limit {
			page, resp, err := g.client.Projects.ListProjects(opts)
			if err != nil {
				return nil, fmt.Errorf("error fetching projects: %v", err)
			}
			for _, p := range page {
				if len(projects) < limit {
					add(p)
				}
			}
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
	}

	// always include the default project
	if g.projectID != 0 && !seen[g.projectID] {
		if p, _, err := g.client.Projects.GetProject(g.projectID, nil); err == nil {
			add(p)
		}
	}

	listed := time.Since(start)
	g.countOpenMRs(projects)
	logrus.Infof("[GitLab] Loaded %d projects in %s (list %s, MR counts %s)", len(projects),
		time.Since(start).Round(time.Millisecond), listed.Round(time.Millisecond), (time.Since(start) - listed).Round(time.Millisecond))

	// projects with open MRs first, then by recent activity
	sort.SliceStable(projects, func(i, j int) bool {
		if (projects[i].OpenMRs > 0) != (projects[j].OpenMRs > 0) {
			return projects[i].OpenMRs > 0
		}
		return projects[i].LastActivityAt > projects[j].LastActivityAt
	})

	g.projects.items = projects
	g.projects.fetched = time.Now()
	return projects, nil
}

// countOpenMRs fills OpenMRs. Projects in groups are counted from one paginated group-level
// MR listing per top-level group (a few requests instead of one per project); projects in
// personal namespaces, or when the group listing fails, are counted one by one.
func (g *GitLabService) countOpenMRs(projects []Project) {
	state := "opened"
	counts := map[int]int{}
	countedGroups := map[string]bool{}

	groups := map[string]bool{}
	for _, p := range projects {
		if p.rootGroup != "" {
			groups[p.rootGroup] = true
		}
	}
	for group := range groups {
		opts := &gitlab.ListGroupMergeRequestsOptions{ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1}, State: &state}
		ok := true
		for page := 0; page < 30; page++ {
			mrs, resp, err := g.client.MergeRequests.ListGroupMergeRequests(group, opts)
			if err != nil {
				logrus.Warnf("[GitLab] Group MR listing for %s failed, counting per project: %v", group, err)
				ok = false
				break
			}
			for _, mr := range mrs {
				counts[mr.ProjectID]++
			}
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
		countedGroups[group] = ok
	}

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range projects {
		p := &projects[i]
		if countedGroups[p.rootGroup] {
			p.OpenMRs = counts[p.ID]
			continue
		}
		wg.Add(1)
		go func(p *Project) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(p.ID, &gitlab.ListProjectMergeRequestsOptions{
				ListOptions: gitlab.ListOptions{PerPage: 1},
				State:       &state,
			})
			if err != nil {
				logrus.Warnf("[GitLab] Cannot count MRs of %s: %v", p.PathWithNamespace, err)
				return
			}
			p.OpenMRs = resp.TotalItems
			if p.OpenMRs == 0 && len(mrs) > 0 { // X-Total missing on very large projects
				p.OpenMRs = len(mrs)
			}
		}(p)
	}
	wg.Wait()
}
