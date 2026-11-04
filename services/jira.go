package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// JiraService reads issue descriptions and comments (read-only).
//
// Auth:
//   - Jira Cloud: JIRA_EMAIL + JIRA_API_TOKEN (Basic auth)
//   - Jira Server / Data Center: JIRA_API_TOKEN only (Personal Access Token, Bearer auth)
type JiraService struct {
	baseURL     string
	email       string
	token       string
	keyPattern  *regexp.Regexp
	maxComments int
	maxChars    int
	httpClient  *http.Client
}

type JiraIssue struct {
	Key         string
	URL         string
	Summary     string
	Type        string
	Status      string
	Priority    string
	Description string
	Parent      string
	Subtasks    []string
	Links       []string
	Attachments []string
	Comments    []JiraComment
}

type JiraComment struct {
	Author  string
	Created string
	Body    string
}

// NewJiraService returns nil when Jira is not configured.
func NewJiraService() *JiraService {
	baseURL := strings.TrimRight(envString("JIRA_URL", ""), "/")
	token := envString("JIRA_API_TOKEN", "")
	if baseURL == "" || token == "" {
		logrus.Info("[Jira] Not configured (JIRA_URL / JIRA_API_TOKEN empty) - task context disabled")
		return nil
	}

	pattern := envString("JIRA_ISSUE_KEY_PATTERN", "")
	if pattern == "" {
		if keys := envList("JIRA_PROJECT_KEYS", nil); len(keys) > 0 {
			pattern = `\b(?:` + strings.Join(keys, "|") + `)-\d+\b`
		} else {
			pattern = `\b[A-Z][A-Z0-9]+-\d+\b`
		}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		logrus.Errorf("[Jira] Invalid JIRA_ISSUE_KEY_PATTERN %q: %v - Jira disabled", pattern, err)
		return nil
	}

	logrus.Infof("[Jira] Enabled: %s (key pattern %s)", baseURL, pattern)
	return &JiraService{
		baseURL:     baseURL,
		email:       envString("JIRA_EMAIL", ""),
		token:       token,
		keyPattern:  re,
		maxComments: envInt("JIRA_MAX_COMMENTS", 20),
		maxChars:    envInt("JIRA_MAX_CHARS", 20000),
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// FindIssueKeys extracts unique issue keys from the given texts (MR title, branch, description).
func (j *JiraService) FindIssueKeys(texts ...string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, t := range texts {
		// branch names are often lowercase: feature/abc-123-something
		for _, candidate := range []string{t, strings.ToUpper(t)} {
			for _, k := range j.keyPattern.FindAllString(candidate, -1) {
				k = strings.ToUpper(k)
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
	}
	return keys
}

func (j *JiraService) GetIssue(key string) (*JiraIssue, error) {
	url := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=summary,description,status,issuetype,priority,parent,comment,subtasks,issuelinks,attachment", j.baseURL, key)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if j.email != "" {
		req.SetBasicAuth(j.email, j.token)
	} else {
		req.Header.Set("Authorization", "Bearer "+j.token)
	}

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("jira %s: HTTP %d %s", key, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Status      struct {
				Name string `json:"name"`
			} `json:"status"`
			IssueType struct {
				Name string `json:"name"`
			} `json:"issuetype"`
			Priority *struct {
				Name string `json:"name"`
			} `json:"priority"`
			Parent *struct {
				Key    string `json:"key"`
				Fields struct {
					Summary string `json:"summary"`
				} `json:"fields"`
			} `json:"parent"`
			Subtasks []struct {
				Key    string `json:"key"`
				Fields struct {
					Summary string `json:"summary"`
					Status  struct {
						Name string `json:"name"`
					} `json:"status"`
				} `json:"fields"`
			} `json:"subtasks"`
			IssueLinks []struct {
				Type struct {
					Inward  string `json:"inward"`
					Outward string `json:"outward"`
				} `json:"type"`
				InwardIssue  *jiraLinkedIssue `json:"inwardIssue"`
				OutwardIssue *jiraLinkedIssue `json:"outwardIssue"`
			} `json:"issuelinks"`
			Attachment []struct {
				Filename string `json:"filename"`
			} `json:"attachment"`
			Comment struct {
				Comments []struct {
					Author struct {
						DisplayName string `json:"displayName"`
					} `json:"author"`
					Created string `json:"created"`
					Body    string `json:"body"`
				} `json:"comments"`
			} `json:"comment"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("jira %s: decode error: %v", key, err)
	}

	issue := &JiraIssue{
		Key:         raw.Key,
		URL:         j.baseURL + "/browse/" + raw.Key,
		Summary:     raw.Fields.Summary,
		Type:        raw.Fields.IssueType.Name,
		Status:      raw.Fields.Status.Name,
		Description: raw.Fields.Description,
	}
	if raw.Fields.Priority != nil {
		issue.Priority = raw.Fields.Priority.Name
	}
	if raw.Fields.Parent != nil {
		issue.Parent = raw.Fields.Parent.Key + " " + raw.Fields.Parent.Fields.Summary
	}
	for _, st := range raw.Fields.Subtasks {
		issue.Subtasks = append(issue.Subtasks, fmt.Sprintf("%s %s (%s)", st.Key, st.Fields.Summary, st.Fields.Status.Name))
	}
	for _, l := range raw.Fields.IssueLinks {
		if l.OutwardIssue != nil {
			issue.Links = append(issue.Links, l.Type.Outward+" "+l.OutwardIssue.String())
		}
		if l.InwardIssue != nil {
			issue.Links = append(issue.Links, l.Type.Inward+" "+l.InwardIssue.String())
		}
	}
	for _, a := range raw.Fields.Attachment {
		issue.Attachments = append(issue.Attachments, a.Filename)
	}
	comments := raw.Fields.Comment.Comments
	if len(comments) > j.maxComments {
		comments = comments[len(comments)-j.maxComments:] // keep the newest
	}
	for _, c := range comments {
		created := c.Created
		if len(created) >= 10 {
			created = created[:10]
		}
		issue.Comments = append(issue.Comments, JiraComment{Author: c.Author.DisplayName, Created: created, Body: c.Body})
	}
	return issue, nil
}

// BuildTaskContext fetches all issues referenced by the MR and formats them for the prompt.
// Returns the formatted context and the list of found issues.
func (j *JiraService) BuildTaskContext(texts ...string) (string, []*JiraIssue) {
	if j == nil {
		return "", nil
	}
	keys := j.FindIssueKeys(texts...)
	if len(keys) == 0 {
		logrus.Info("[Jira] No issue key found in MR title/branch/description/commits")
		return "", nil
	}
	if len(keys) > 5 {
		logrus.Infof("[Jira] Found %d keys, using the first 5: %v", len(keys), keys[:5])
		keys = keys[:5]
	}

	var b strings.Builder
	var issues []*JiraIssue
	for _, key := range keys {
		issue, err := j.GetIssue(key)
		if err != nil {
			logrus.Warnf("[Jira] %v", err)
			continue
		}
		issues = append(issues, issue)
		fmt.Fprintf(&b, "### %s: %s\nType: %s | Status: %s", issue.Key, issue.Summary, issue.Type, issue.Status)
		if issue.Priority != "" {
			fmt.Fprintf(&b, " | Priority: %s", issue.Priority)
		}
		if issue.Parent != "" {
			fmt.Fprintf(&b, "\nParent/Epic: %s", issue.Parent)
		}
		fmt.Fprintf(&b, "\n\nDescription:\n%s\n", orDash(issue.Description))
		if len(issue.Subtasks) > 0 {
			b.WriteString("\nSubtasks:\n- " + strings.Join(issue.Subtasks, "\n- ") + "\n")
		}
		if len(issue.Links) > 0 {
			b.WriteString("\nLinked issues:\n- " + strings.Join(issue.Links, "\n- ") + "\n")
		}
		if len(issue.Attachments) > 0 {
			b.WriteString("\nAttachments (not readable here - mockups/specs may contain extra requirements): " + strings.Join(issue.Attachments, ", ") + "\n")
		}
		if len(issue.Comments) > 0 {
			b.WriteString("\nComments (oldest first):\n")
			for _, c := range issue.Comments {
				fmt.Fprintf(&b, "- [%s, %s]: %s\n", c.Author, c.Created, strings.TrimSpace(c.Body))
			}
		}
		b.WriteString("\n")
	}
	logrus.Infof("[Jira] Loaded %d issue(s) for keys %v", len(issues), keys)
	return truncate(b.String(), j.maxChars, "[Jira context truncated]"), issues
}

type jiraLinkedIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
	} `json:"fields"`
}

func (l *jiraLinkedIssue) String() string {
	return fmt.Sprintf("%s %s (%s)", l.Key, l.Fields.Summary, l.Fields.Status.Name)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func truncate(s string, max int, marker string) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// avoid cutting a multi-byte UTF-8 rune in half
	cut := max
	for cut > 0 && cut < len(s) && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "\n\n" + marker
}
