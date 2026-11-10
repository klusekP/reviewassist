package services

import (
	"codereviewAI/services/i18n"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// SettingDef describes one configurable environment variable.
// Label, Description and a localized Placeholder come from the i18n catalog
// ("settings.<KEY>", "settings.<KEY>.desc", "settings.<KEY>.placeholder").
type SettingDef struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Group       string   `json:"group"`
	Type        string   `json:"type"` // string | int | bool | select | secret | list | text
	Default     string   `json:"default"`
	Options     []string `json:"options,omitempty"`
	Description string   `json:"description"`
	Restart     bool     `json:"restart"` // change takes effect only after restarting the app
	Placeholder string   `json:"placeholder,omitempty"`
}

// SettingGroup is a section of the settings page; Label and Description come from
// the i18n catalog ("settings.group.<id>", "settings.group.<id>.desc").
type SettingGroup struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Test        string `json:"test,omitempty"` // connection test target
}

var settingGroups = []SettingGroup{
	{ID: "gitlab", Test: "gitlab"},
	{ID: "jira", Test: "jira"},
	{ID: "review"},
	{ID: "agent", Test: "claude"},
	{ID: "llm"},
	{ID: "limits"},
	{ID: "server"},
}

var settingDefs = []SettingDef{
	// GitLab
	{Key: "GITLAB_URL", Group: "gitlab", Type: "string", Default: "https://gitlab.com"},
	{Key: "GITLAB_TOKEN", Group: "gitlab", Type: "secret"},
	{Key: "GITLAB_PROJECTS", Group: "gitlab", Type: "list"},
	{Key: "GITLAB_PROJECTS_LIMIT", Group: "gitlab", Type: "int", Default: "100"},
	{Key: "GITLAB_PROJECT_ID", Group: "gitlab", Type: "int"},

	// Jira
	{Key: "JIRA_URL", Group: "jira", Type: "string"},
	{Key: "JIRA_EMAIL", Group: "jira", Type: "string"},
	{Key: "JIRA_API_TOKEN", Group: "jira", Type: "secret"},
	{Key: "JIRA_PROJECT_KEYS", Group: "jira", Type: "list", Placeholder: "KM, SHOP"},
	{Key: "JIRA_ISSUE_KEY_PATTERN", Group: "jira", Type: "string", Placeholder: `\b(?:KM|SHOP)-\d+\b`},
	{Key: "JIRA_MAX_COMMENTS", Group: "jira", Type: "int", Default: "20"},
	{Key: "JIRA_MAX_CHARS", Group: "jira", Type: "int", Default: "20000"},

	// Review
	{Key: "REVIEW_ENGINE", Group: "review", Type: "select", Default: "llm", Options: []string{"claude-code", "llm"}},
	{Key: "REVIEW_LANGUAGE", Group: "review", Type: "select", Default: "pl", Options: []string{"pl", "en", "de"}},
	{Key: "REVIEW_POST_TO_GITLAB", Group: "review", Type: "bool", Default: "true"},
	{Key: "REVIEW_GITLAB_MODE", Group: "review", Type: "select", Default: "draft", Options: []string{"draft", "comment"}},
	{Key: "REVIEW_MIN_SEVERITY", Group: "review", Type: "select", Default: "low", Options: []string{"low", "medium", "high", "critical"}},
	{Key: "REVIEW_MAX_FILES", Group: "review", Type: "int", Default: "30"},
	{Key: "REVIEW_IGNORE", Group: "review", Type: "list", Placeholder: "*.lock, vendor/*, *.min.js"},

	// Agent
	{Key: "CLAUDE_CODE_MODEL", Group: "agent", Type: "string", Placeholder: "opus, sonnet…"},
	{Key: "CLAUDE_CODE_MAX_TURNS", Group: "agent", Type: "int", Default: "60"},
	{Key: "REVIEW_AGENT_TIMEOUT_MIN", Group: "agent", Type: "int", Default: "20"},
	{Key: "CLAUDE_CODE_USE_API_KEY", Group: "agent", Type: "bool", Default: "false"},
	{Key: "ANTHROPIC_API_KEY", Group: "agent", Type: "secret"},
	{Key: "CLAUDE_CODE_BIN", Group: "agent", Type: "string", Default: "claude"},
	{Key: "REVIEW_WORKDIR", Group: "agent", Type: "string", Placeholder: "~/Library/Caches/codereviewAI"},

	// LLM
	{Key: "AI_MODEL", Group: "llm", Type: "select", Default: "openai", Options: []string{"ollama", "mlx", "openai", "claude", "kimi", "opencode"}},
	{Key: "OLLAMA_MODEL", Group: "llm", Type: "string", Default: "llama2", Placeholder: "qwen2.5-coder:32b"},
	{Key: "OLLAMA_BASE_URL", Group: "llm", Type: "string", Default: "http://localhost:11434"},
	{Key: "OLLAMA_NUM_CTX", Group: "llm", Type: "int", Default: "32768"},
	{Key: "MLX_MODEL", Group: "llm", Type: "string"},
	{Key: "MLX_BASE_URL", Group: "llm", Type: "string", Default: "http://localhost:8081/v1"},
	{Key: "MLX_API_KEY", Group: "llm", Type: "secret"},
	{Key: "OPENAI_API_KEY", Group: "llm", Type: "secret"},
	{Key: "CLAUDE_API_KEY", Group: "llm", Type: "secret"},
	{Key: "KIMI_API_KEY", Group: "llm", Type: "secret"},
	{Key: "OPENCODE_API_KEY", Group: "llm", Type: "secret"},

	// Limits
	{Key: "REVIEW_CONCURRENCY", Group: "limits", Type: "int", Default: "2"},
	{Key: "REVIEW_DOCS_PATHS", Group: "limits", Type: "list", Placeholder: "README.md, CLAUDE.md, docs"},
	{Key: "REVIEW_DOCS_MAX_CHARS", Group: "limits", Type: "int", Default: "12000"},
	{Key: "REVIEW_FILE_MAX_CHARS", Group: "limits", Type: "int", Default: "15000"},
	{Key: "REVIEW_DIFF_MAX_CHARS", Group: "limits", Type: "int", Default: "12000"},
	{Key: "REVIEW_TREE_MAX_PATHS", Group: "limits", Type: "int", Default: "300"},

	// Server
	{Key: "UI_LANGUAGE", Group: "server", Type: "select", Default: i18n.Default, Options: i18n.UILanguages},
	{Key: "PORT", Group: "server", Type: "int", Default: "8080", Restart: true},
}

// SettingView is a setting as shown in the UI. Secret values are never returned.
type SettingView struct {
	SettingDef
	Value    string `json:"value"`  // effective value (masked for secrets)
	Source   string `json:"source"` // ui | env | default
	IsSet    bool   `json:"is_set"`
	EnvSet   bool   `json:"env_set"`             // a value exists in .env / environment
	EnvValue string `json:"env_value,omitempty"` // .env value shown when resetting (never for secrets)
}

// SettingsManager merges settings: UI (SQLite) > environment/.env > defaults.
// Effective values are exported to the process environment, so all code keeps using os.Getenv.
type SettingsManager struct {
	store *Store
	mu    sync.Mutex
	env   map[string]string // environment snapshot taken before UI overrides were applied
	ui    map[string]string
}

func findSetting(key string) (SettingDef, bool) {
	for _, d := range settingDefs {
		if d.Key == key {
			return d, true
		}
	}
	return SettingDef{}, false
}

func NewSettingsManager(store *Store) (*SettingsManager, error) {
	m := &SettingsManager{store: store, env: map[string]string{}}
	for _, d := range settingDefs {
		if v, ok := os.LookupEnv(d.Key); ok {
			m.env[d.Key] = v
		}
	}
	ui, err := store.Settings()
	if err != nil {
		return nil, err
	}
	m.ui = ui
	for k, v := range ui {
		if _, ok := findSetting(k); ok {
			os.Setenv(k, v)
		}
	}
	return m, nil
}

func (m *SettingsManager) effective(key string) (string, string) {
	if v, ok := m.ui[key]; ok {
		return v, "ui"
	}
	if v, ok := m.env[key]; ok && v != "" {
		return v, "env"
	}
	d, _ := findSetting(key)
	return d.Default, "default"
}

func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 8 {
		return "••••"
	}
	return "••••" + v[len(v)-4:]
}

// translated returns the definition with label, description and placeholder in lang.
func (d SettingDef) translated(lang string) SettingDef {
	d.Label = i18n.T(lang, "settings."+d.Key)
	if k := "settings." + d.Key + ".desc"; i18n.Has(k) {
		d.Description = i18n.T(lang, k)
	}
	if k := "settings." + d.Key + ".placeholder"; i18n.Has(k) {
		d.Placeholder = i18n.T(lang, k)
	}
	return d
}

// View returns the settings groups and values with texts in the UI language lang.
func (m *SettingsManager) View(lang string) ([]SettingGroup, []SettingView) {
	m.mu.Lock()
	defer m.mu.Unlock()
	groups := make([]SettingGroup, len(settingGroups))
	for i, g := range settingGroups {
		g.Label = i18n.T(lang, "settings.group."+g.ID)
		g.Description = i18n.T(lang, "settings.group."+g.ID+".desc")
		groups[i] = g
	}
	views := make([]SettingView, 0, len(settingDefs))
	for _, d := range settingDefs {
		d = d.translated(lang)
		v, src := m.effective(d.Key)
		view := SettingView{SettingDef: d, Value: v, Source: src, IsSet: v != "", EnvSet: m.env[d.Key] != ""}
		if d.Type == "secret" {
			view.Value = maskSecret(v)
		} else {
			view.EnvValue = m.env[d.Key]
		}
		views = append(views, view)
	}
	return groups, views
}

func validateSetting(d SettingDef, v string) error {
	if v == "" {
		return nil
	}
	switch d.Type {
	case "int":
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return i18n.Errorf("settings.err.int", i18n.Key("settings."+d.Key))
		}
	case "bool":
		if _, err := strconv.ParseBool(v); err != nil {
			return i18n.Errorf("settings.err.bool", i18n.Key("settings."+d.Key))
		}
	case "select":
		for _, o := range d.Options {
			if o == v {
				return nil
			}
		}
		return i18n.Errorf("settings.err.select", i18n.Key("settings."+d.Key), strings.Join(d.Options, ", "))
	}
	if d.Key == "JIRA_ISSUE_KEY_PATTERN" {
		if _, err := regexp.Compile(v); err != nil {
			return i18n.Errorf("settings.err.regex", i18n.Key("settings."+d.Key), err)
		}
	}
	if strings.HasSuffix(d.Key, "_URL") && !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		return i18n.Errorf("settings.err.url", i18n.Key("settings."+d.Key))
	}
	return nil
}

// Update saves values from the UI and removes UI overrides listed in reset (falling back to .env).
// Secrets sent as an empty string are left unchanged. Returns keys that need an app restart.
func (m *SettingsManager) Update(values map[string]string, reset []string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	set := map[string]string{}
	for k, v := range values {
		d, ok := findSetting(k)
		if !ok {
			return nil, i18n.Errorf("settings.err.unknown", k)
		}
		v = strings.TrimSpace(v)
		if d.Type == "secret" && v == "" {
			continue // keep current secret
		}
		if d.Type == "list" {
			v = strings.Join(envListValue(v), ",")
		}
		if err := validateSetting(d, v); err != nil {
			return nil, err
		}
		if cur, _ := m.effective(k); cur == v && m.ui[k] == v {
			continue
		}
		set[k] = v
	}
	var del []string
	for _, k := range reset {
		if _, ok := findSetting(k); !ok {
			return nil, i18n.Errorf("settings.err.unknown", k)
		}
		if _, ok := m.ui[k]; ok {
			del = append(del, k)
		}
	}

	if err := m.store.ApplySettings(set, del); err != nil {
		return nil, err
	}

	var restart []string
	changed := func(k string) {
		if d, _ := findSetting(k); d.Restart {
			restart = append(restart, k)
		}
	}
	for k, v := range set {
		m.ui[k] = v
		os.Setenv(k, v)
		changed(k)
	}
	for _, k := range del {
		delete(m.ui, k)
		if v, ok := m.env[k]; ok {
			os.Setenv(k, v)
		} else {
			os.Unsetenv(k)
		}
		changed(k)
	}
	return restart, nil
}

// envListValue normalises a comma separated list ("a, b,,c" -> [a b c]).
func envListValue(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
