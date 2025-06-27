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
}

func NewReviewService(gitlabService *GitLabService, aiService *AIService) *ReviewService {
	return &ReviewService{
		gitlabService: gitlabService,
		aiService:     aiService,
	}
}

// GetMergeRequests - pobiera listę merge requests
func (r *ReviewService) GetMergeRequests(c *gin.Context) {
	state := c.DefaultQuery("state", "opened")

	mrs, err := r.gitlabService.GetMergeRequests(state)
	if err != nil {
		logrus.Errorf("Błąd pobierania merge requests: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Błąd pobierania merge requests",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"merge_requests": mrs,
		"count":          len(mrs),
	})
}

// GetMergeRequest - pobiera szczegóły konkretnego merge request
func (r *ReviewService) GetMergeRequest(c *gin.Context) {
	iidStr := c.Param("id")
	iid, err := strconv.Atoi(iidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Nieprawidłowy ID merge request",
		})
		return
	}

	mr, err := r.gitlabService.GetMergeRequest(iid)
	if err != nil {
		logrus.Errorf("Błąd pobierania merge request %d: %v", iid, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Błąd pobierania merge request",
		})
		return
	}

	// Pobierz diff
	diffs, err := r.gitlabService.GetMergeRequestDiffs(iid)
	if err != nil {
		logrus.Errorf("Błąd pobierania diff dla MR %d: %v", iid, err)
		// Nie zwracamy błędu, tylko logujemy
	}

	c.JSON(http.StatusOK, gin.H{
		"merge_request": mr,
		"diffs":         diffs,
		"diff_count":    len(diffs),
	})
}

// ReviewMergeRequest - przeprowadza automatyczną recenzję merge request (tylko analiza, bez dodawania komentarzy)
func (r *ReviewService) ReviewMergeRequest(c *gin.Context) {
	iidStr := c.Param("id")
	iid, err := strconv.Atoi(iidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Nieprawidłowy ID merge request",
		})
		return
	}

	// Pobierz merge request
	mr, err := r.gitlabService.GetMergeRequest(iid)
	if err != nil {
		logrus.Errorf("Błąd pobierania merge request %d: %v", iid, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Błąd pobierania merge request",
		})
		return
	}

	// Pobierz diff
	diffs, err := r.gitlabService.GetMergeRequestDiffs(iid)
	if err != nil {
		logrus.Errorf("Błąd pobierania diff dla MR %d: %v", iid, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Błąd pobierania diff",
		})
		return
	}

	// Przeprowadź recenzję AI
	review, err := r.aiService.ReviewCode(diffs, mr.Title, mr.Description)
	if err != nil {
		logrus.Errorf("Błąd recenzji AI dla MR %d: %v", iid, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Błąd przeprowadzania recenzji AI",
		})
		return
	}

	// Zwróć tylko wynik recenzji bez dodawania komentarzy do GitLab
	c.JSON(http.StatusOK, gin.H{
		"message":        "Recenzja zakończona pomyślnie",
		"review":         review,
		"comments_added": 0, // Brak komentarzy dodanych do GitLab
		"merge_request":  mr,
	})
}

// GetProjects - pobiera listę projektów
func (r *ReviewService) GetProjects(c *gin.Context) {
	projects, err := r.gitlabService.GetProjects()
	if err != nil {
		logrus.Errorf("Błąd pobierania projektów: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Błąd pobierania projektów",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projects": projects,
		"count":    len(projects),
	})
}
