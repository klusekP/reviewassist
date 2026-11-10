package services

import (
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"codereviewAI/services/i18n"
)

// ReviewConfig holds review behaviour settings loaded from environment variables.
type ReviewConfig struct {
	Language string // language of comments written by the AI, e.g. "pl"

	MaxFiles     int // max number of changed files reviewed per MR
	DiffMaxChars int // max diff size per file sent to the model
	FileMaxChars int // max full-file content per file sent to the model
	DocsMaxChars int // total budget for project documentation
	TreeMaxPaths int // max number of repository paths listed as project structure

	DocsPaths   []string // files/directories with project documentation
	IgnoreGlobs []string // changed files matching these patterns are skipped

	PostToGitLab bool   // publish results to GitLab after review
	GitLabMode   string // "draft" (pending review, visible only to token owner) or "comment"
	MinSeverity  string // lowest severity posted as inline comment

	Concurrency int // parallel AI requests (keep low for local models)

	// Review engine: "llm" (prompt per file via AI_MODEL) or "claude-code" (agent on a cloned repo)
	Engine          string
	WorkDir         string // where the repository clone and worktrees live
	AgentBin        string // path to the claude CLI
	AgentModel      string // optional --model for Claude Code (e.g. opus, sonnet)
	AgentMaxTurns   int
	AgentTimeoutMin int
	AgentUseAPIKey  bool // false: strip ANTHROPIC_API_KEY so Claude Code uses the claude.ai login
}

var defaultDocsPaths = []string{
	"README.md", "README.rst", "README", "CLAUDE.md", "AGENTS.md", "CONTRIBUTING.md",
	"ARCHITECTURE.md", "CONVENTIONS.md", ".cursorrules", ".github/copilot-instructions.md",
	"docs", "doc",
}

var defaultIgnoreGlobs = []string{
	"*.lock", "package-lock.json", "composer.lock", "yarn.lock", "pnpm-lock.yaml", "go.sum",
	"vendor/*", "node_modules/*", "*.min.js", "*.min.css", "*.map", "*.svg", "*.png", "*.jpg",
	"*.jpeg", "*.gif", "*.ico", "*.pdf", "*.woff", "*.woff2", "*.ttf",
}

func LoadReviewConfig() *ReviewConfig {
	return &ReviewConfig{
		Language:     envString("REVIEW_LANGUAGE", "pl"),
		MaxFiles:     envInt("REVIEW_MAX_FILES", 30),
		DiffMaxChars: envInt("REVIEW_DIFF_MAX_CHARS", 12000),
		FileMaxChars: envInt("REVIEW_FILE_MAX_CHARS", 15000),
		DocsMaxChars: envInt("REVIEW_DOCS_MAX_CHARS", 12000),
		TreeMaxPaths: envInt("REVIEW_TREE_MAX_PATHS", 300),
		DocsPaths:    envList("REVIEW_DOCS_PATHS", defaultDocsPaths),
		IgnoreGlobs:  envList("REVIEW_IGNORE", defaultIgnoreGlobs),
		PostToGitLab: envBool("REVIEW_POST_TO_GITLAB", true),
		GitLabMode:   envString("REVIEW_GITLAB_MODE", "draft"),
		MinSeverity:  envString("REVIEW_MIN_SEVERITY", "low"),
		Concurrency:  envInt("REVIEW_CONCURRENCY", 2),

		Engine:          strings.ToLower(envString("REVIEW_ENGINE", "llm")),
		WorkDir:         envString("REVIEW_WORKDIR", defaultWorkDir()),
		AgentBin:        envString("CLAUDE_CODE_BIN", "claude"),
		AgentModel:      envString("CLAUDE_CODE_MODEL", ""),
		AgentMaxTurns:   envInt("CLAUDE_CODE_MAX_TURNS", 60),
		AgentTimeoutMin: envInt("REVIEW_AGENT_TIMEOUT_MIN", 20),
		AgentUseAPIKey:  envBool("CLAUDE_CODE_USE_API_KEY", false),
	}
}

func defaultWorkDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "codereviewAI")
	}
	return filepath.Join(os.TempDir(), "codereviewAI")
}

// Lang is the review output language code used for labels and messages (unknown languages fall back to English).
func (c *ReviewConfig) Lang() string {
	return i18n.Or(c.Language, i18n.EN)
}

// LanguageName returns a human readable language name for the prompt.
func (c *ReviewConfig) LanguageName() string {
	switch strings.ToLower(c.Language) {
	case "pl", "polish", "polski":
		return "Polish (język polski)"
	case "en", "english":
		return "English"
	case "de", "german":
		return "German"
	default:
		return c.Language
	}
}

func (c *ReviewConfig) IsIgnored(filePath string) bool {
	base := path.Base(filePath)
	for _, g := range c.IgnoreGlobs {
		if strings.HasSuffix(g, "/*") && strings.HasPrefix(filePath, strings.TrimSuffix(g, "*")) {
			return true
		}
		if strings.HasSuffix(g, "/*") && strings.Contains(filePath, "/"+strings.TrimSuffix(g, "*")) {
			return true
		}
		if ok, _ := path.Match(g, base); ok {
			return true
		}
		if ok, _ := path.Match(g, filePath); ok {
			return true
		}
	}
	return false
}

var severityRank = map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

func (c *ReviewConfig) SeverityAllowed(sev string) bool {
	min, ok := severityRank[strings.ToLower(c.MinSeverity)]
	if !ok {
		min = 1
	}
	return severityRank[strings.ToLower(sev)] >= min
}

func envString(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && v > 0 {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}

func envList(key string, def []string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
