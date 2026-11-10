package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"codereviewAI/services/ai"
	"codereviewAI/services/i18n"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Runtime owns the services built from the current settings and rebuilds them when
// settings change in the UI. Reviews in progress keep their own service instances.
type Runtime struct {
	mu       sync.RWMutex
	review   *ReviewService
	hub      *WSHub
	store    *Store
	settings *SettingsManager
	running  *runningState
}

func NewRuntime(hub *WSHub, store *Store, settings *SettingsManager) (*Runtime, error) {
	rt := &Runtime{hub: hub, store: store, settings: settings, running: newRunningState()}
	return rt, rt.Reload()
}

// Reload rebuilds GitLab, AI, Jira and review configuration from the current environment.
func (rt *Runtime) Reload() error {
	gl, err := NewGitLabService()
	if err != nil {
		return err
	}
	rs := NewReviewService(gl, NewAIService(), rt.hub, rt.store, rt.running)
	rt.mu.Lock()
	rt.review = rs
	rt.mu.Unlock()
	return nil
}

func (rt *Runtime) Review() *ReviewService {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.review
}

// Handle adapts a ReviewService method to a gin handler that always uses the current instance.
func (rt *Runtime) Handle(f func(*ReviewService, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { f(rt.Review(), c) }
}

// ---------- settings API ----------

// uiLang is the language of messages returned to the web UI: the Accept-Language header
// sent by the UI, or the UI_LANGUAGE setting.
func uiLang(c *gin.Context) string {
	return i18n.FromAcceptLanguage(c.GetHeader("Accept-Language"), i18n.UILanguage())
}

func (rt *Runtime) GetSettings(c *gin.Context) {
	groups, views := rt.settings.View(uiLang(c))
	c.JSON(http.StatusOK, gin.H{"groups": groups, "settings": views, "db_path": rt.store.Path()})
}

func (rt *Runtime) UpdateSettings(c *gin.Context) {
	var body struct {
		Values map[string]string `json:"values"`
		Reset  []string          `json:"reset"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(uiLang(c), "api.invalidData", err.Error())})
		return
	}
	restart, err := rt.settings.Update(body.Values, body.Reset)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.Message(uiLang(c), err)})
		return
	}
	if err := rt.Reload(); err != nil {
		logrus.Errorf("[Settings] Reload failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(uiLang(c), "api.savedNotApplied", err.Error())})
		return
	}
	logrus.Infof("[Settings] Updated %d value(s), reset %d; services reloaded", len(body.Values), len(body.Reset))
	groups, views := rt.settings.View(uiLang(c))
	c.JSON(http.StatusOK, gin.H{"groups": groups, "settings": views, "restart_required": restart, "db_path": rt.store.Path()})
}

// TestSettings checks a connection with the currently saved settings.
func (rt *Runtime) TestSettings(c *gin.Context) {
	target := c.Param("target")
	lang := uiLang(c)
	ok, msg := false, ""
	switch target {
	case "gitlab":
		user, err := rt.Review().gitlabService.CurrentUser()
		if err != nil {
			msg = i18n.T(lang, "api.error", err.Error())
		} else {
			ok, msg = true, i18n.T(lang, "api.loggedInAs", user)
		}
	case "jira":
		j := NewJiraService()
		if j == nil {
			msg = i18n.T(lang, "api.jiraNotConfigured")
		} else if name, err := j.Myself(); err != nil {
			msg = i18n.T(lang, "api.error", err.Error())
		} else {
			ok, msg = true, i18n.T(lang, "api.loggedInAs", name)
		}
	case "claude":
		cfg := rt.Review().config
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, cfg.AgentBin, "--version").CombinedOutput()
		if err != nil {
			msg = i18n.T(lang, "api.cannotRun", cfg.AgentBin, err)
		} else {
			auth := i18n.T(lang, "api.billingClaudeAI")
			if cfg.AgentUseAPIKey {
				auth = "ANTHROPIC_API_KEY"
			}
			ok, msg = true, i18n.T(lang, "api.billing", strings.TrimSpace(string(out)), auth)
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(lang, "api.unknownTest", target)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": ok, "message": msg})
}

// ---------- reviews API ----------

// saveReview stores a finished review in SQLite.
func (r *ReviewService) saveReview(mr *MergeRequest, review *ai.CodeReview) {
	data, err := json.Marshal(review)
	if err != nil {
		logrus.Errorf("[Store] Cannot encode review: %v", err)
		return
	}
	rec := &ReviewRecord{
		ProjectID: r.pid(), IID: mr.IID, SHA: mr.SHA, Title: mr.Title,
		Verdict: review.Verdict, Score: review.Score, Engine: r.config.Engine, Review: data,
	}
	if err := r.store.SaveReview(rec); err != nil {
		logrus.Errorf("[Store] Cannot save review of MR %d: %v", mr.IID, err)
	}
}

// ListReviews returns the latest review of each MR (optionally ?project_id=).
func (r *ReviewService) ListReviews(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Query("project_id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	recs, err := r.store.LatestReviews(pid, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reviews": recs})
}

func (r *ReviewService) ReviewHistory(c *gin.Context) {
	r, iid, ok := r.scoped(c, true)
	if !ok {
		return
	}
	recs, err := r.store.ReviewHistory(r.pid(), iid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reviews": recs})
}

func (r *ReviewService) DeleteReviews(c *gin.Context) {
	r, iid, ok := r.scoped(c, true)
	if !ok {
		return
	}
	n, err := r.store.DeleteReviews(r.pid(), iid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// ImportReviews moves results kept in the browser (older versions) into the database.
// MRs that already have a stored review are skipped.
func (r *ReviewService) ImportReviews(c *gin.Context) {
	var items []struct {
		ProjectID int             `json:"project_id"`
		IID       int             `json:"iid"`
		SHA       string          `json:"sha"`
		Title     string          `json:"title"`
		Timestamp string          `json:"timestamp"`
		Review    json.RawMessage `json:"review"`
	}
	if err := c.ShouldBindJSON(&items); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	imported := 0
	for _, it := range items {
		if it.ProjectID == 0 || it.IID == 0 || len(it.Review) == 0 {
			continue
		}
		if has, err := r.store.HasReview(it.ProjectID, it.IID); err != nil || has {
			continue
		}
		var rv ai.CodeReview
		if err := json.Unmarshal(it.Review, &rv); err != nil {
			continue
		}
		rec := &ReviewRecord{ProjectID: it.ProjectID, IID: it.IID, SHA: it.SHA, Title: it.Title, Verdict: rv.Verdict,
			Score: rv.Score, Engine: "import", Review: it.Review, CreatedAt: it.Timestamp}
		if err := r.store.SaveReview(rec); err == nil {
			imported++
		}
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported})
}

// ---------- Jira connection test ----------

// Myself returns the display name of the authenticated Jira user.
func (j *JiraService) Myself() (string, error) {
	req, err := http.NewRequest(http.MethodGet, j.baseURL+"/rest/api/2/myself", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	if j.email != "" {
		req.SetBasicAuth(j.email, j.token)
	} else {
		req.Header.Set("Authorization", "Bearer "+j.token)
	}
	resp, err := j.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return "", fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var me struct {
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return "", err
	}
	if me.EmailAddress != "" {
		return me.DisplayName + " (" + me.EmailAddress + ")", nil
	}
	return me.DisplayName, nil
}
