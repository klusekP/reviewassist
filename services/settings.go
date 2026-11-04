package services

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// SettingDef describes one configurable environment variable.
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

type SettingGroup struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Test        string `json:"test,omitempty"` // connection test target
}

var settingGroups = []SettingGroup{
	{ID: "gitlab", Label: "GitLab", Description: "Dostęp do repozytoriów i merge requestów.", Test: "gitlab"},
	{ID: "jira", Label: "Jira", Description: "Zadania, których opis i komentarze trafiają do review. Tylko odczyt.", Test: "jira"},
	{ID: "review", Label: "Review", Description: "Jak i gdzie publikowane są wyniki."},
	{ID: "agent", Label: "Agent Claude Code", Description: "Używany, gdy silnik review to claude-code.", Test: "claude"},
	{ID: "llm", Label: "Modele LLM", Description: "Używane, gdy silnik review to llm."},
	{ID: "limits", Label: "Kontekst i limity", Description: "Ile kodu i dokumentacji trafia do modelu."},
	{ID: "server", Label: "Serwer", Description: "Ustawienia aplikacji."},
}

var settingDefs = []SettingDef{
	// GitLab
	{Key: "GITLAB_URL", Label: "Adres GitLab", Group: "gitlab", Type: "string", Default: "https://gitlab.com"},
	{Key: "GITLAB_TOKEN", Label: "Token GitLab", Group: "gitlab", Type: "secret", Description: "Personal access token z uprawnieniem api. W trybie draft komentarze tworzą się jako właściciel tokena."},
	{Key: "GITLAB_PROJECTS", Label: "Projekty w panelu", Group: "gitlab", Type: "list", Placeholder: "grupa/api, grupa/front, 123456", Description: "ID lub ścieżki projektów oddzielone przecinkami. Puste = wszystkie projekty, których jesteś członkiem."},
	{Key: "GITLAB_PROJECTS_LIMIT", Label: "Maks. liczba projektów", Group: "gitlab", Type: "int", Default: "100"},
	{Key: "GITLAB_PROJECT_ID", Label: "Domyślny projekt (ID)", Group: "gitlab", Type: "int", Description: "Używany przez stare trasy /api/v1/merge-requests. Opcjonalny."},

	// Jira
	{Key: "JIRA_URL", Label: "Adres Jira", Group: "jira", Type: "string", Placeholder: "https://firma.atlassian.net", Description: "Puste wyłącza integrację z Jirą."},
	{Key: "JIRA_EMAIL", Label: "E-mail (Jira Cloud)", Group: "jira", Type: "string", Description: "Dla Jira Cloud. Dla Server/Data Center zostaw puste i użyj Personal Access Token."},
	{Key: "JIRA_API_TOKEN", Label: "Token API", Group: "jira", Type: "secret"},
	{Key: "JIRA_PROJECT_KEYS", Label: "Klucze projektów", Group: "jira", Type: "list", Placeholder: "KM, SHOP", Description: "Ogranicza wykrywanie zadań do tych kluczy (unika fałszywych trafień typu UTF-8)."},
	{Key: "JIRA_ISSUE_KEY_PATTERN", Label: "Własny wzorzec klucza", Group: "jira", Type: "string", Placeholder: `\b(?:KM|SHOP)-\d+\b`, Description: "Wyrażenie regularne. Ma pierwszeństwo przed kluczami projektów."},
	{Key: "JIRA_MAX_COMMENTS", Label: "Maks. komentarzy", Group: "jira", Type: "int", Default: "20"},
	{Key: "JIRA_MAX_CHARS", Label: "Maks. znaków z Jiry", Group: "jira", Type: "int", Default: "20000"},

	// Review
	{Key: "REVIEW_ENGINE", Label: "Silnik review", Group: "review", Type: "select", Default: "llm", Options: []string{"claude-code", "llm"}, Description: "claude-code: agent na sklonowanym repo. llm: prompt per plik do modelu z sekcji Modele LLM."},
	{Key: "REVIEW_LANGUAGE", Label: "Język komentarzy", Group: "review", Type: "select", Default: "pl", Options: []string{"pl", "en", "de"}},
	{Key: "REVIEW_POST_TO_GITLAB", Label: "Publikuj w GitLab", Group: "review", Type: "bool", Default: "true"},
	{Key: "REVIEW_GITLAB_MODE", Label: "Tryb publikacji", Group: "review", Type: "select", Default: "draft", Options: []string{"draft", "comment"}, Description: "draft: szkic review widoczny tylko dla Ciebie do „Submit review”. comment: komentarze widoczne od razu dla wszystkich."},
	{Key: "REVIEW_MIN_SEVERITY", Label: "Minimalna waga uwag", Group: "review", Type: "select", Default: "low", Options: []string{"low", "medium", "high", "critical"}, Description: "Uwagi o niższej wadze nie trafiają do GitLab."},
	{Key: "REVIEW_MAX_FILES", Label: "Maks. plików w review", Group: "review", Type: "int", Default: "30"},
	{Key: "REVIEW_IGNORE", Label: "Ignorowane pliki", Group: "review", Type: "list", Placeholder: "*.lock, vendor/*, *.min.js", Description: "Wzorce oddzielone przecinkami. Puste = domyślna lista (lock-i, vendor, node_modules, obrazy…)."},

	// Agent
	{Key: "CLAUDE_CODE_MODEL", Label: "Model", Group: "agent", Type: "string", Placeholder: "opus, sonnet…", Description: "Puste = domyślny model Claude Code."},
	{Key: "CLAUDE_CODE_MAX_TURNS", Label: "Maks. kroków agenta", Group: "agent", Type: "int", Default: "60"},
	{Key: "REVIEW_AGENT_TIMEOUT_MIN", Label: "Limit czasu (min)", Group: "agent", Type: "int", Default: "20"},
	{Key: "CLAUDE_CODE_USE_API_KEY", Label: "Płać przez API key", Group: "agent", Type: "bool", Default: "false", Description: "Wyłączone: agent używa Twojego logowania claude.ai (subskrypcja). Włączone: używa ANTHROPIC_API_KEY."},
	{Key: "ANTHROPIC_API_KEY", Label: "Anthropic API key", Group: "agent", Type: "secret", Description: "Potrzebny tylko, gdy „Płać przez API key” jest włączone."},
	{Key: "CLAUDE_CODE_BIN", Label: "Ścieżka do claude", Group: "agent", Type: "string", Default: "claude"},
	{Key: "REVIEW_WORKDIR", Label: "Katalog roboczy", Group: "agent", Type: "string", Placeholder: "~/Library/Caches/codereviewAI", Description: "Klon repozytoriów i tymczasowe worktree. Puste = katalog cache systemu."},

	// LLM
	{Key: "AI_MODEL", Label: "Dostawca", Group: "llm", Type: "select", Default: "openai", Options: []string{"ollama", "mlx", "openai", "claude", "kimi", "opencode"}},
	{Key: "OLLAMA_MODEL", Label: "Ollama: model", Group: "llm", Type: "string", Default: "llama2", Placeholder: "qwen2.5-coder:32b"},
	{Key: "OLLAMA_BASE_URL", Label: "Ollama: adres", Group: "llm", Type: "string", Default: "http://localhost:11434"},
	{Key: "OLLAMA_NUM_CTX", Label: "Ollama: okno kontekstu", Group: "llm", Type: "int", Default: "32768", Description: "W tokenach. Domyślne okno Ollamy ucina prompty review."},
	{Key: "MLX_MODEL", Label: "MLX: model", Group: "llm", Type: "string"},
	{Key: "MLX_BASE_URL", Label: "MLX: adres", Group: "llm", Type: "string", Default: "http://localhost:8081/v1"},
	{Key: "MLX_API_KEY", Label: "MLX: klucz", Group: "llm", Type: "secret"},
	{Key: "OPENAI_API_KEY", Label: "OpenAI API key", Group: "llm", Type: "secret"},
	{Key: "CLAUDE_API_KEY", Label: "Claude API key (tryb llm)", Group: "llm", Type: "secret"},
	{Key: "KIMI_API_KEY", Label: "Kimi API key", Group: "llm", Type: "secret"},
	{Key: "OPENCODE_API_KEY", Label: "OpenCode API key", Group: "llm", Type: "secret"},

	// Limits
	{Key: "REVIEW_CONCURRENCY", Label: "Równoległe zapytania", Group: "limits", Type: "int", Default: "2", Description: "Tryb llm. Przy lokalnych modelach trzymaj nisko."},
	{Key: "REVIEW_DOCS_PATHS", Label: "Dokumentacja projektu", Group: "limits", Type: "list", Placeholder: "README.md, CLAUDE.md, docs", Description: "Tryb llm. Pliki i katalogi czytane jako kontekst. Puste = domyślna lista."},
	{Key: "REVIEW_DOCS_MAX_CHARS", Label: "Maks. znaków dokumentacji", Group: "limits", Type: "int", Default: "12000"},
	{Key: "REVIEW_FILE_MAX_CHARS", Label: "Maks. znaków pliku", Group: "limits", Type: "int", Default: "15000"},
	{Key: "REVIEW_DIFF_MAX_CHARS", Label: "Maks. znaków diffa", Group: "limits", Type: "int", Default: "12000"},
	{Key: "REVIEW_TREE_MAX_PATHS", Label: "Maks. ścieżek struktury repo", Group: "limits", Type: "int", Default: "300"},

	// Server
	{Key: "PORT", Label: "Port HTTP", Group: "server", Type: "int", Default: "8080", Restart: true},
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

func (m *SettingsManager) View() ([]SettingGroup, []SettingView) {
	m.mu.Lock()
	defer m.mu.Unlock()
	views := make([]SettingView, 0, len(settingDefs))
	for _, d := range settingDefs {
		v, src := m.effective(d.Key)
		view := SettingView{SettingDef: d, Value: v, Source: src, IsSet: v != "", EnvSet: m.env[d.Key] != ""}
		if d.Type == "secret" {
			view.Value = maskSecret(v)
		} else {
			view.EnvValue = m.env[d.Key]
		}
		views = append(views, view)
	}
	return settingGroups, views
}

func validateSetting(d SettingDef, v string) error {
	if v == "" {
		return nil
	}
	switch d.Type {
	case "int":
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Errorf("%s: wymagana liczba całkowita ≥ 0", d.Label)
		}
	case "bool":
		if _, err := strconv.ParseBool(v); err != nil {
			return fmt.Errorf("%s: wymagane true/false", d.Label)
		}
	case "select":
		for _, o := range d.Options {
			if o == v {
				return nil
			}
		}
		return fmt.Errorf("%s: dozwolone wartości: %s", d.Label, strings.Join(d.Options, ", "))
	}
	if d.Key == "JIRA_ISSUE_KEY_PATTERN" {
		if _, err := regexp.Compile(v); err != nil {
			return fmt.Errorf("%s: niepoprawne wyrażenie regularne: %v", d.Label, err)
		}
	}
	if strings.HasSuffix(d.Key, "_URL") && !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		return fmt.Errorf("%s: adres musi zaczynać się od http:// lub https://", d.Label)
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
			return nil, fmt.Errorf("nieznane ustawienie %s", k)
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
			return nil, fmt.Errorf("nieznane ustawienie %s", k)
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
