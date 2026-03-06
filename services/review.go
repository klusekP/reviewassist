package services

import (
	"net/http"
	"strconv"

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

	// Return 202 immediately, run review in background
	c.JSON(http.StatusAccepted, gin.H{
		"status":   "started",
		"message":  "Review started. Results will be delivered via WebSocket.",
		"mr_id":    iid,
		"mr_title": mr.Title,
	})

	// Run AI review in background and broadcast result via WebSocket
	go func() {
		review, err := r.aiService.ReviewCode(diffs, mr.Title, mr.Description)
		if err != nil {
			logrus.Errorf("Error performing AI review for MR %d: %v", iid, err)
			r.wsHub.BroadcastReviewError(iid, err.Error())
			return
		}
		r.wsHub.BroadcastReview(iid, review)
		logrus.Infof("[Review] MR %d review completed, sent via WebSocket", iid)
	}()
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
