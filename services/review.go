package services

import (
	"net/http"
	"strconv"
	"sync"

	"codereviewAI/services/ai"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type ReviewService struct {
	gitlabService *GitLabService
	aiService     *AIService
	wsHub         *WSHub
}

func NewReviewService(gitlabService *GitLabService, aiService *AIService, wsHub *WSHub) *ReviewService {
	return &ReviewService{
		gitlabService: gitlabService,
		aiService:     aiService,
		wsHub:         wsHub,
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

	totalFiles := len(diffs)
	if totalFiles > 10 {
		totalFiles = 10
	}

	// Return 202 immediately, run review in background
	c.JSON(http.StatusAccepted, gin.H{
		"status":       "started",
		"message":      "Review started. Results will be delivered via WebSocket.",
		"mr_id":        iid,
		"mr_title":     mr.Title,
		"total_files":  totalFiles,
	})

	go func() {
		r.runPerFileReview(iid, diffs, mr.Title, mr.Description)
	}()
}

func (r *ReviewService) runPerFileReview(iid int, diffs []Diff, mrTitle, mrDescription string) {
	if len(diffs) == 0 {
		r.wsHub.BroadcastReview(iid, r.aiService.createFallbackReview(diffs))
		return
	}

	totalFiles := len(diffs)
	if totalFiles > 10 {
		diffs = diffs[:10]
		totalFiles = 10
		logrus.Infof("[Review] MR %d: limiting to first 10 files", iid)
	}

	const maxConcurrent = 2
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	results := make([]*ai.CodeReview, totalFiles)
	var resultsMu sync.Mutex

	for i, diff := range diffs {
		wg.Add(1)
		go func(idx int, d Diff) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			review, err := r.aiService.ReviewSingleDiff(d, mrTitle, mrDescription)
			if err != nil {
				logrus.Warnf("[Review] MR %d file %s: %v", iid, d.NewPath, err)
				review = r.aiService.createFallbackReview([]Diff{d})
			}
			resultsMu.Lock()
			results[idx] = review
			resultsMu.Unlock()
			r.wsHub.BroadcastReviewPart(iid, d.NewPath, idx, totalFiles, review)
		}(i, diff)
	}

	wg.Wait()
	aggregated := aggregateReviews(results)
	r.wsHub.BroadcastReview(iid, aggregated)
	logrus.Infof("[Review] MR %d review completed (%d files), sent via WebSocket", iid, totalFiles)
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
	avgScore := scoreSum / len(reviews)
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
