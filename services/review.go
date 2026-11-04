package services

import (
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"codereviewAI/services/ai"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type ReviewService struct {
	gitlabService *GitLabService
	aiService     *AIService
	wsHub         *WSHub
	jira          *JiraService // nil when Jira is not configured
	config        *ReviewConfig
	repo          *RepoManager // used by the claude-code engine
}

func NewReviewService(gitlabService *GitLabService, aiService *AIService, wsHub *WSHub) *ReviewService {
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
	return &ReviewService{
		gitlabService: gitlabService,
		aiService:     aiService,
		wsHub:         wsHub,
		jira:          NewJiraService(),
		config:        cfg,
		repo:          NewRepoManager(gitlabService, cfg.WorkDir),
	}
}

func (r *ReviewService) GetMergeRequests(c *gin.Context) {
	state := c.DefaultQuery("state", "opened")

	mrs, err := r.gitlabService.GetMergeRequests(state)
	if err != nil {
		logrus.Errorf("Error fetching merge requests: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching merge requests",
		})
		return
	}

	logrus.Infof("Fetched %d merge requests", len(mrs))

	c.JSON(http.StatusOK, gin.H{
		"merge_requests": mrs,
		"count":          len(mrs),
	})
}

func (r *ReviewService) GetMergeRequest(c *gin.Context) {
	iidStr := c.Param("id")
	iid, err := strconv.Atoi(iidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid merge request ID",
		})
		return
	}

	mr, err := r.gitlabService.GetMergeRequest(iid)
	if err != nil {
		logrus.Errorf("Error fetching merge request %d: %v", iid, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching merge request",
		})
		return
	}

	diffs, err := r.gitlabService.GetMergeRequestDiffs(iid)
	if err != nil {
		logrus.Errorf("Error fetching diff for MR %d: %v", iid, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"merge_request": mr,
		"diffs":         diffs,
		"diff_count":    len(diffs),
	})
}

func (r *ReviewService) ReviewMergeRequest(c *gin.Context) {
	iidStr := c.Param("id")
	logrus.Infof("[Review] Starting review for MR %s (background)", iidStr)
	iid, err := strconv.Atoi(iidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid merge request ID",
		})
		return
	}

	mr, err := r.gitlabService.GetMergeRequest(iid)
	if err != nil {
		logrus.Errorf("Error fetching merge request %d: %v", iid, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching merge request",
		})
		return
	}

	diffs, err := r.gitlabService.GetMergeRequestDiffs(iid)
	if err != nil {
		logrus.Errorf("Error fetching diff for MR %d: %v", iid, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching diff",
		})
		return
	}

	reviewable := r.filterDiffs(iid, diffs)

	// Return 202 immediately, run review in background
	c.JSON(http.StatusAccepted, gin.H{
		"status":      "started",
		"message":     "Review started. Results will be delivered via WebSocket.",
		"mr_id":       iid,
		"mr_title":    mr.Title,
		"total_files": len(reviewable),
	})

	go r.runReview(mr, reviewable, diffs)
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
			r.wsHub.BroadcastReviewError(iid, fmt.Sprintf("review failed: %v", rec))
		}
	}()

	if len(diffs) == 0 {
		r.wsHub.BroadcastReview(iid, &ai.CodeReview{Summary: "Brak plików do review (wszystkie pominięte lub usunięte).", Score: 0})
		return
	}

	refs, err := r.gitlabService.GetDiffRefs(iid)
	if err != nil {
		logrus.Errorf("[Review] MR %d: %v", iid, err)
		r.wsHub.BroadcastReviewError(iid, err.Error())
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

	// 2. Review with the configured engine
	var results []fileResult
	var taskReview *ai.CodeReview
	total := len(diffs)
	if r.config.Engine == "claude-code" {
		results, taskReview, err = r.runAgentReview(mr, refs, diffs, taskContext)
		if err != nil {
			logrus.Errorf("[Review] MR %d: %v", iid, err)
			r.wsHub.BroadcastReviewError(iid, err.Error())
			return
		}
		for i, res := range results {
			r.wsHub.BroadcastReviewPart(iid, res.diff.NewPath, i, total, res.review)
		}
	} else {
		results, taskReview = r.runLLMReview(mr, refs, diffs, allDiffs, taskContext)
	}

	// 3. Scope guard + final verdict (never milder than the findings justify)
	if taskReview == nil {
		taskReview = &ai.CodeReview{Summary: "Brak oceny całościowej (błąd modelu)."}
	}
	applyScope(iid, results, taskReview, allDiffs)
	allIssues := append([]ai.Issue{}, taskReview.Issues...)
	for _, res := range results {
		if !res.failed {
			allIssues = append(allIssues, res.review.Issues...)
		}
	}
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
		published, err := r.publishToGitLab(mr, refs, results, taskReview, issues)
		if err != nil {
			logrus.Errorf("[Review] MR %d: publishing to GitLab failed: %v", iid, err)
			aggregated.Summary += "\n\n⚠️ Błąd publikacji w GitLab: " + err.Error()
		} else {
			aggregated.Summary += "\n\n" + published
		}
	}

	r.wsHub.BroadcastReview(iid, aggregated)
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
func (r *ReviewService) runLLMReview(mr *MergeRequest, refs *DiffRefs, diffs []Diff, allDiffs []Diff, taskContext string) ([]fileResult, *ai.CodeReview) {
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
			})
			res := fileResult{diff: d, lines: lines, review: review}
			if err != nil {
				logrus.Warnf("[Review] MR %d file %s: %v", iid, d.NewPath, err)
				res.failed = true
				res.review = &ai.CodeReview{Summary: "Nie udało się wykonać review tego pliku: " + err.Error()}
			}
			results[idx] = res
			r.wsHub.BroadcastReviewPart(iid, d.NewPath, idx, total, res.review)
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
	projects, err := r.gitlabService.GetProjects()
	if err != nil {
		logrus.Errorf("Error fetching projects: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Error fetching projects",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projects": projects,
		"count":    len(projects),
	})
}

func (r *ReviewService) GetStats(c *gin.Context) {
	mrs, err := r.gitlabService.GetMergeRequests("opened")
	if err != nil {
		logrus.Errorf("Error fetching stats: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"totalMRs":    "-",
			"reviewedMRs": "-",
			"avgScore":    "-",
			"totalIssues": "-",
		})
		return
	}

	// TODO: Implement statistics logic based on saved reviews
	// For now, return basic data
	c.JSON(http.StatusOK, gin.H{
		"totalMRs":    len(mrs),
		"reviewedMRs": 0,   // TODO: policz z localStorage lub bazy danych
		"avgScore":    "-", // TODO: policz średnią ocenę
		"totalIssues": 0,   // TODO: policz łączne problemy
	})
}
