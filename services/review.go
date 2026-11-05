package services

import (
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"codereviewAI/services/ai"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type ReviewService struct {
	gitlabService *GitLabService // bound to one project (see forProject)
	aiService     *AIService
	wsHub         *WSHub
	jira          *JiraService // nil when Jira is not configured
	config        *ReviewConfig
	repo          *RepoManager // used by the claude-code engine, bound to the same project
	shared        *reviewShared
	running       *runningState // survives settings reloads
	store         *Store
}

// reviewShared is state common to all project-scoped copies of ReviewService.
type reviewShared struct {
	mu    sync.Mutex
	repos map[int]*RepoManager
}

// runningState tracks reviews in progress; shared across settings reloads.
type runningState struct {
	mu sync.Mutex
	m  map[string]*ReviewProgress // "projectID/iid" -> progress
}

func newRunningState() *runningState { return &runningState{m: map[string]*ReviewProgress{}} }

// ReviewProgress describes a review that is currently running on the server.
type ReviewProgress struct {
	ProjectID int    `json:"project_id"`
	IID       int    `json:"iid"`
	StartedAt string `json:"started_at"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Engine    string `json:"engine"`
}

func NewReviewService(gitlabService *GitLabService, aiService *AIService, wsHub *WSHub, store *Store, running *runningState) *ReviewService {
	cfg := LoadReviewConfig()
	switch cfg.Engine {
	case "claude-code":
		if _, err := exec.LookPath(cfg.AgentBin); err != nil {
			logrus.Errorf("[Review] REVIEW_ENGINE=claude-code but %q not found in PATH (set CLAUDE_CODE_BIN)", cfg.AgentBin)
		}
		if _, err := exec.LookPath("git"); err != nil {
			logrus.Error("[Review] REVIEW_ENGINE=claude-code requires git in PATH")
		}
		logrus.Infof("[Review] Engine: claude-code (workdir %s)", cfg.WorkDir)
	case "llm":
		logrus.Info("[Review] Engine: llm (AI_MODEL)")
	default:
		logrus.Warnf("[Review] Unknown REVIEW_ENGINE=%q, using llm", cfg.Engine)
		cfg.Engine = "llm"
	}
	r := &ReviewService{
		gitlabService: gitlabService,
		aiService:     aiService,
		wsHub:         wsHub,
		jira:          NewJiraService(),
		config:        cfg,
		shared:        &reviewShared{repos: map[int]*RepoManager{}},
		running:       running,
		store:         store,
	}
	return r.forProject(gitlabService.ProjectID())
}

// forProject returns a copy of the service bound to the given GitLab project.
func (r *ReviewService) forProject(projectID int) *ReviewService {
	c := *r
	c.gitlabService = r.gitlabService.ForProject(projectID)
	r.shared.mu.Lock()
	repo, ok := r.shared.repos[projectID]
	if !ok {
		repo = NewRepoManager(c.gitlabService, r.config.WorkDir)
		r.shared.repos[projectID] = repo
	}
	r.shared.mu.Unlock()
	c.repo = repo
	return &c
}

func (r *ReviewService) pid() int { return r.gitlabService.ProjectID() }

// scoped resolves the project from the :pid route param (or the default project) and the MR from :id.
func (r *ReviewService) scoped(c *gin.Context, needIID bool) (*ReviewService, int, bool) {
	pid := r.pid()
	if s := c.Param("pid"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
			return nil, 0, false
		}
		pid = n
	}
	if pid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No project selected (set GITLAB_PROJECT_ID or use /projects/:pid/...)"})
		return nil, 0, false
	}
	iid := 0
	if needIID {
		n, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid merge request ID"})
			return nil, 0, false
		}
		iid = n
	}
	return r.forProject(pid), iid, true
}

func runningKey(pid, iid int) string { return fmt.Sprintf("%d/%d", pid, iid) }

func (r *ReviewService) startRunning(iid, total int) bool {
	r.running.mu.Lock()
	defer r.running.mu.Unlock()
	key := runningKey(r.pid(), iid)
	if _, busy := r.running.m[key]; busy {
		return false
	}
	r.running.m[key] = &ReviewProgress{ProjectID: r.pid(), IID: iid, StartedAt: time.Now().UTC().Format(time.RFC3339), Total: total, Engine: r.config.Engine}
	return true
}

func (r *ReviewService) progressRunning(iid int) {
	r.running.mu.Lock()
	defer r.running.mu.Unlock()
	if p, ok := r.running.m[runningKey(r.pid(), iid)]; ok {
		p.Done++
	}
}

func (r *ReviewService) stopRunning(iid int) {
	r.running.mu.Lock()
	defer r.running.mu.Unlock()
	delete(r.running.m, runningKey(r.pid(), iid))
}

func (r *ReviewService) runningFor(pid int) map[int]ReviewProgress {
	r.running.mu.Lock()
	defer r.running.mu.Unlock()
	out := map[int]ReviewProgress{}
	for _, p := range r.running.m {
		if p.ProjectID == pid {
			out[p.IID] = *p
		}
	}
	return out
}

func (r *ReviewService) GetMergeRequests(c *gin.Context) {
	r, _, ok := r.scoped(c, false)
	if !ok {
		return
	}
	state := c.DefaultQuery("state", "opened")

	mrs, err := r.gitlabService.GetMergeRequests(state)
	if err != nil {
		logrus.Errorf("Error fetching merge requests of project %d: %v", r.pid(), err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Error fetching merge requests: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id":     r.pid(),
		"merge_requests": mrs,
		"count":          len(mrs),
		"running":        r.runningFor(r.pid()),
	})
}

func (r *ReviewService) GetMergeRequest(c *gin.Context) {
	r, iid, ok := r.scoped(c, true)
	if !ok {
		return
	}

	mr, err := r.gitlabService.GetMergeRequest(iid)
	if err != nil {
		logrus.Errorf("Error fetching merge request %d: %v", iid, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Error fetching merge request: " + err.Error()})
		return
	}

	diffs, err := r.gitlabService.GetMergeRequestDiffs(iid)
	if err != nil {
		logrus.Errorf("Error fetching diff for MR %d: %v", iid, err)
	}

	var running *ReviewProgress
	if p, ok := r.runningFor(r.pid())[iid]; ok {
		running = &p
	}

	c.JSON(http.StatusOK, gin.H{
		"merge_request": mr,
		"diffs":         diffs,
		"diff_count":    len(diffs),
		"running":       running,
	})
}

func (r *ReviewService) ReviewMergeRequest(c *gin.Context) {
	r, iid, ok := r.scoped(c, true)
	if !ok {
		return
	}
	logrus.Infof("[Review] Starting review for project %d MR %d (background)", r.pid(), iid)

	mr, err := r.gitlabService.GetMergeRequest(iid)
	if err != nil {
		logrus.Errorf("Error fetching merge request %d: %v", iid, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Error fetching merge request: " + err.Error()})
		return
	}

	diffs, err := r.gitlabService.GetMergeRequestDiffs(iid)
	if err != nil {
		logrus.Errorf("Error fetching diff for MR %d: %v", iid, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Error fetching diff: " + err.Error()})
		return
	}

	reviewable := r.filterDiffs(iid, diffs)
	if !r.startRunning(iid, len(reviewable)) {
		c.JSON(http.StatusConflict, gin.H{"error": "Review of this merge request is already running"})
		return
	}

	// Return 202 immediately, run review in background
	c.JSON(http.StatusAccepted, gin.H{
		"status":      "started",
		"message":     "Review started. Results will be delivered via WebSocket.",
		"project_id":  r.pid(),
		"mr_id":       iid,
		"mr_title":    mr.Title,
		"total_files": len(reviewable),
		"engine":      r.config.Engine,
	})

	go func() {
		defer r.stopRunning(iid)
		r.runReview(mr, reviewable, diffs)
	}()
}

// GetConfig exposes non-secret settings for the UI.
func (r *ReviewService) GetConfig(c *gin.Context) {
	model := r.config.AgentModel
	if r.config.Engine == "llm" {
		model = envString("AI_MODEL", "openai")
	}
	c.JSON(http.StatusOK, gin.H{
		"default_project_id": r.pid(),
		"gitlab_url":         r.gitlabService.baseURL,
		"engine":             r.config.Engine,
		"model":              model,
		"language":           r.config.Language,
		"post_to_gitlab":     r.config.PostToGitLab,
		"gitlab_mode":        r.config.GitLabMode,
		"jira_enabled":       r.jira != nil,
		"db_path":            r.store.Path(),
	})
}

// filterDiffs drops ignored/binary files and applies the REVIEW_MAX_FILES limit.
func (r *ReviewService) filterDiffs(iid int, diffs []Diff) []Diff {
	var out []Diff
	for _, d := range diffs {
		if d.DeletedFile || strings.TrimSpace(d.Diff) == "" || r.config.IsIgnored(d.NewPath) {
			continue
		}
		out = append(out, d)
	}
	if len(out) > r.config.MaxFiles {
		logrus.Infof("[Review] MR %d: limiting review to first %d of %d files (REVIEW_MAX_FILES)", iid, r.config.MaxFiles, len(out))
		out = out[:r.config.MaxFiles]
	}
	return out
}

type fileResult struct {
	diff   Diff
	lines  *DiffLines
	review *ai.CodeReview
	failed bool
}

func (r *ReviewService) runReview(mr *MergeRequest, diffs []Diff, allDiffs []Diff) {
	iid := mr.IID
	defer func() {
		if rec := recover(); rec != nil {
			logrus.Errorf("[Review] MR %d panic: %v", iid, rec)
			r.wsHub.BroadcastReviewError(r.pid(), iid, fmt.Sprintf("review failed: %v", rec))
		}
	}()

	if len(diffs) == 0 {
		r.wsHub.BroadcastReview(r.pid(), iid, &ai.CodeReview{Summary: "Brak plików do review (wszystkie pominięte lub usunięte).", Score: 0})
		return
	}

	refs, err := r.gitlabService.GetDiffRefs(iid)
	if err != nil {
		logrus.Errorf("[Review] MR %d: %v", iid, err)
		r.wsHub.BroadcastReviewError(r.pid(), iid, err.Error())
		return
	}

	// 1. Task context from Jira (used by both engines)
	// Commit messages are only a fallback: merge commits often mention unrelated issues.
	keySources := []string{mr.Title, mr.SourceBranch, mr.Description}
	if r.jira != nil && len(r.jira.FindIssueKeys(keySources...)) == 0 {
		if msgs, err := r.gitlabService.GetMergeRequestCommitMessages(iid); err == nil {
			keySources = append(keySources, msgs...)
		} else {
			logrus.Warnf("[Review] MR %d: cannot read commits for Jira keys: %v", iid, err)
		}
	}
	taskContext, issues := r.jira.BuildTaskContext(keySources...)

	// Re-review: findings of the previous review to check against the current code
	rr := r.loadPreviousFindings(mr, refs)
	if rr.empty() {
		rr = nil
	}

	// 2. Review with the configured engine
	var results []fileResult
	var taskReview *ai.CodeReview
	total := len(diffs)
	if r.config.Engine == "claude-code" {
		results, taskReview, err = r.runAgentReview(mr, refs, diffs, taskContext, rr)
		if err != nil {
			logrus.Errorf("[Review] MR %d: %v", iid, err)
			r.wsHub.BroadcastReviewError(r.pid(), iid, err.Error())
			return
		}
		for i, res := range results {
			r.wsHub.BroadcastReviewPart(r.pid(), iid, res.diff.NewPath, i, total, res.review)
		}
	} else {
		results, taskReview = r.runLLMReview(mr, refs, diffs, allDiffs, taskContext, rr)
	}

	// 3. Scope guard + final verdict (never milder than the findings justify)
	if taskReview == nil {
		taskReview = &ai.CodeReview{Summary: "Brak oceny całościowej (błąd modelu)."}
	}
	applyScope(iid, results, taskReview, allDiffs)

	// 3a. Re-review: status of previous findings, no repeated comments
	var stillOpen []ai.Issue
	if rr != nil {
		statuses := taskReview.PreviousFindings // claude-code answers in the main review
		if r.config.Engine != "claude-code" {
			var err error
			if statuses, err = r.checkPreviousFindings(mr, refs, rr); err != nil {
				logrus.Warnf("[ReReview] MR %d: check of previous findings failed: %v", iid, err)
			}
		}
		rr.applyStatuses(statuses)
		rr.dropRepeated(iid, results, taskReview)
		stillOpen = rr.openIssues()
	}
	taskReview.PreviousFindings = nil

	allIssues := append([]ai.Issue{}, taskReview.Issues...)
	for _, res := range results {
		if !res.failed {
			allIssues = append(allIssues, res.review.Issues...)
		}
	}
	if rr != nil {
		rr.newFindings = len(allIssues)
	}
	allIssues = append(allIssues, stillOpen...)
	taskReview.Verdict = finalVerdict(taskReview.Verdict, allIssues, taskReview.Requirements)

	// 4. Aggregate for the web UI
	var reviews []*ai.CodeReview
	for _, res := range results {
		if !res.failed {
			reviews = append(reviews, res.review)
		}
	}
	aggregated := aggregateReviews(reviews)
	if taskReview != nil {
		aggregated.Summary = taskReview.Summary
		aggregated.Issues = append(taskReview.Issues, aggregated.Issues...)
		aggregated.Suggestions = dedupeStrings(append(taskReview.Suggestions, aggregated.Suggestions...))
		if taskReview.Score > 0 {
			aggregated.Score = taskReview.Score
		}
		aggregated.Verdict = taskReview.Verdict
		aggregated.SecuritySummary = taskReview.SecuritySummary
		aggregated.QualitySummary = taskReview.QualitySummary
		aggregated.Requirements = taskReview.Requirements
		aggregated.Issues = append(aggregated.Issues, stillOpen...)
		if rr != nil {
			aggregated.Followup = rr.followup()
			aggregated.Summary += "\n\n🔁 " + followupLine(aggregated.Followup)
		}
		if line := requirementsLine(taskReview.Requirements); line != "" {
			aggregated.Summary += "\n\n📋 " + line
		}
		if taskReview.SecuritySummary != "" {
			aggregated.Summary += "\n\n🔒 " + taskReview.SecuritySummary
		}
		if taskReview.QualitySummary != "" {
			aggregated.Summary += "\n\n⚡ " + taskReview.QualitySummary
		}
	}

	// 5. Publish to GitLab
	if r.config.PostToGitLab {
		published, err := r.publishToGitLab(mr, refs, results, taskReview, issues, rr)
		if err != nil {
			logrus.Errorf("[Review] MR %d: publishing to GitLab failed: %v", iid, err)
			aggregated.Summary += "\n\n⚠️ Błąd publikacji w GitLab: " + err.Error()
		} else {
			aggregated.Summary += "\n\n" + published
		}
	}

	r.saveReview(mr, aggregated)
	r.wsHub.BroadcastReview(r.pid(), iid, aggregated)
	logrus.Infof("[Review] MR %d review completed (%d files)", iid, total)
}

// applyScope keeps only findings about code changed by this MR:
// file findings must point to an added/modified line (normalised to that line),
// MR-level findings must be general (no file) or about a file changed in the MR.
func applyScope(iid int, results []fileResult, taskReview *ai.CodeReview, allDiffs []Diff) {
	dropped := 0
	for _, res := range results {
		if res.failed || res.review == nil {
			continue
		}
		kept := res.review.Issues[:0]
		for _, is := range res.review.Issues {
			if newLine, _, ok := res.lines.ResolveLine(issueLine(is.Line)); ok {
				is.Line = newLine
				kept = append(kept, is)
			} else {
				dropped++
				logrus.Debugf("[Scope] MR %d: dropped %s:%v (not a changed line)", iid, is.File, is.Line)
			}
		}
		res.review.Issues = kept
	}

	changed := map[string]bool{}
	for _, d := range allDiffs {
		changed[d.NewPath] = true
		changed[d.OldPath] = true
	}
	kept := taskReview.Issues[:0]
	for _, is := range taskReview.Issues {
		file := strings.TrimPrefix(strings.TrimPrefix(is.File, "./"), "/")
		if file == "" || changed[file] {
			kept = append(kept, is)
		} else {
			dropped++
			logrus.Debugf("[Scope] MR %d: dropped MR-level finding about unchanged file %s", iid, is.File)
		}
	}
	taskReview.Issues = kept
	if dropped > 0 {
		logrus.Infof("[Scope] MR %d: dropped %d finding(s) outside the changes", iid, dropped)
	}
}

// runLLMReview is the prompt-based engine: one LLM call per file plus one MR-level call.
func (r *ReviewService) runLLMReview(mr *MergeRequest, refs *DiffRefs, diffs []Diff, allDiffs []Diff, taskContext string, rr *reReview) ([]fileResult, *ai.CodeReview) {
	iid := mr.IID
	project := r.BuildProjectContext(refs.HeadSha)

	var allPaths []string
	for _, d := range allDiffs {
		allPaths = append(allPaths, d.NewPath)
	}

	// per-file review
	total := len(diffs)
	results := make([]fileResult, total)
	sem := make(chan struct{}, r.config.Concurrency)
	var wg sync.WaitGroup

	for i, diff := range diffs {
		wg.Add(1)
		go func(idx int, d Diff) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			lines := ParseDiff(d.Diff)
			var others []string
			for _, p := range allPaths {
				if p != d.NewPath {
					others = append(others, p)
				}
			}
			review, err := r.aiService.ReviewFileWithContext(r.config, FileReviewInput{
				Diff:          d,
				NumberedDiff:  lines.Numbered,
				FileContent:   r.GetFileContent(d, refs.HeadSha),
				OtherFiles:    others,
				MRTitle:       mr.Title,
				MRDescription: mr.Description,
				SourceBranch:  mr.SourceBranch,
				TaskContext:   taskContext,
				Project:       project,

				PreviousFindings: rr.fileSection(d.NewPath),
			})
			res := fileResult{diff: d, lines: lines, review: review}
			if err != nil {
				logrus.Warnf("[Review] MR %d file %s: %v", iid, d.NewPath, err)
				res.failed = true
				res.review = &ai.CodeReview{Summary: "Nie udało się wykonać review tego pliku: " + err.Error()}
			}
			results[idx] = res
			r.progressRunning(iid)
			r.wsHub.BroadcastReviewPart(r.pid(), iid, d.NewPath, idx, total, res.review)
		}(i, diff)
	}
	wg.Wait()

	// MR-level review against the task
	taskReview, err := r.aiService.ReviewTaskCompliance(r.config, TaskReviewInput{
		MRTitle:       mr.Title,
		MRDescription: mr.Description,
		SourceBranch:  mr.SourceBranch,
		TaskContext:   taskContext,
		Project:       project,
		Diffs:         diffs,
	})
	if err != nil {
		logrus.Warnf("[Review] MR %d task compliance review failed: %v", iid, err)
	}
	return results, taskReview
}

func aggregateReviews(reviews []*ai.CodeReview) *ai.CodeReview {
	if len(reviews) == 0 {
		return &ai.CodeReview{Summary: "No files to review", Issues: nil, Suggestions: nil, Score: 5}
	}
	var allIssues []ai.Issue
	var allSuggestions []string
	scoreSum := 0
	for _, r := range reviews {
		if r != nil {
			allIssues = append(allIssues, r.Issues...)
			allSuggestions = append(allSuggestions, r.Suggestions...)
			scoreSum += r.Score
		}
	}
	avgScore := 0
	if len(reviews) > 0 {
		avgScore = scoreSum / len(reviews)
	}
	if avgScore > 10 {
		avgScore = 10
	}
	if avgScore == 0 {
		avgScore = 5
	}
	return &ai.CodeReview{
		Summary:     "Review completed for " + strconv.Itoa(len(reviews)) + " files",
		Issues:      allIssues,
		Suggestions: dedupeStrings(allSuggestions),
		Score:       avgScore,
	}
}

func dedupeStrings(s []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func (r *ReviewService) GetProjects(c *gin.Context) {
	projects, err := r.gitlabService.GetProjects(c.Query("refresh") != "")
	if err != nil {
		logrus.Errorf("Error fetching projects: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Error fetching projects: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projects": projects,
		"count":    len(projects),
	})
}
