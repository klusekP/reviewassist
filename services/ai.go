package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

type AIService struct {
	openaiClient *openai.Client
	claudeAPIKey string
	model        string
}

type CodeReview struct {
	Summary     string   `json:"summary"`
	Issues      []Issue  `json:"issues"`
	Suggestions []string `json:"suggestions"`
	Score       int      `json:"score"` // 1-10
}

type Issue struct {
	Type        string `json:"type"`     // "bug", "security", "performance", "style", "best_practice"
	Severity    string `json:"severity"` // "low", "medium", "high", "critical"
	Description string `json:"description"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Code        string `json:"code"`
	Fix         string `json:"fix"`
}

// Claude API request/response structures
type ClaudeRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	Messages    []Message `json:"messages"`
}

type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ClaudeResponse struct {
	Content []ClaudeContent `json:"content"`
}

type ClaudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func NewAIService() *AIService {
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	claudeAPIKey := os.Getenv("CLAUDE_API_KEY")
	model := os.Getenv("AI_MODEL")

	if model == "" {
		model = "openai" // default to OpenAI
	}

	var openaiClient *openai.Client
	if openaiAPIKey != "" {
		openaiClient = openai.NewClient(openaiAPIKey)
	}

	return &AIService{
		openaiClient: openaiClient,
		claudeAPIKey: claudeAPIKey,
		model:        model,
	}
}

func (a *AIService) ReviewCode(diffs []Diff, mrTitle, mrDescription string) (*CodeReview, error) {
	contextStr := a.buildContext(diffs, mrTitle, mrDescription)

	if len(contextStr) > 3000 {
		contextStr = contextStr[:3000] + "\n\n[Code was truncated due to token limit]"
	}

	prompt := `You are an experienced programmer and code reviewer. Analyze the following code changes and perform a detailed code review.

Review guidelines:
1. Check logical and syntactic correctness
2. Pay attention to security (SQL injection, XSS, etc.)
3. Check performance and optimization
4. Evaluate readability and compliance with best practices
5. Check error handling
6. Pay attention to tests

Respond in JSON format:
{
  "summary": "Brief summary of the review",
  "issues": [
    {
      "type": "bug|security|performance|style|best_practice",
      "severity": "low|medium|high|critical",
      "description": "Problem description",
      "file": "file path",
      "line": line_number,
      "code": "code snippet",
      "fix": "suggested fix"
    }
  ],
  "suggestions": ["list of general suggestions"],
  "score": score_1_10
}

Context:
MR Title: ` + mrTitle + `
MR Description: ` + mrDescription + `

Code changes:
` + contextStr

	switch a.model {
	case "claude":
		return a.reviewWithClaude(prompt)
	case "ollama":
		// TODO: implement OLLAMA
		fallthrough
	case "openai":
		fallthrough
	default:
		return a.reviewWithOpenAI(prompt)
	}
}

func (a *AIService) reviewWithOpenAI(prompt string) (*CodeReview, error) {
	if a.openaiClient == nil {
		return nil, fmt.Errorf("OpenAI client is not configured")
	}

	resp, err := a.openaiClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a code review expert. Respond only in JSON format. All responses must be in English.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.3,
			MaxTokens:   2000,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error calling OpenAI API: %v", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI API")
	}

	content := resp.Choices[0].Message.Content
	return a.parseReviewResponse(content)
}

func (a *AIService) reviewWithClaude(prompt string) (*CodeReview, error) {
	if a.claudeAPIKey == "" {
		return nil, fmt.Errorf("claude API key is not configured")
	}

	requestBody := ClaudeRequest{
		Model:       "claude-sonnet-4-20250514",
		MaxTokens:   2000,
		Temperature: 0.3,
		Messages: []Message{
			{
				Role: "user",
				Content: []Content{
					{
						Type: "text",
						Text: "You are a code review expert. Respond only in JSON format. All responses must be in English.\n\n" + prompt,
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.claudeAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error calling Claude API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("claude API error: %s - %s", resp.Status, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return nil, fmt.Errorf("error decoding Claude response: %v", err)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("no response from Claude API")
	}

	content := claudeResp.Content[0].Text
	return a.parseReviewResponse(content)
}

func (a *AIService) buildContext(diffs []Diff, mrTitle, mrDescription string) string {
	var context strings.Builder

	maxDiffs := 5
	if len(diffs) > maxDiffs {
		context.WriteString(fmt.Sprintf("Showing first %d of %d files:\n\n", maxDiffs, len(diffs)))
		diffs = diffs[:maxDiffs]
	}

	for _, diff := range diffs {
		context.WriteString(fmt.Sprintf("\nFile: %s\n", diff.NewPath))
		if diff.NewFile {
			context.WriteString("(NEW FILE)\n")
		} else if diff.DeletedFile {
			context.WriteString("(DELETED FILE)\n")
		} else if diff.RenamedFile {
			context.WriteString(fmt.Sprintf("(RENAMED FROM: %s)\n", diff.OldPath))
		}

		diffContent := diff.Diff
		if len(diffContent) > 1000 {
			diffContent = diffContent[:1000] + "\n\n[Diff was truncated...]"
		}

		context.WriteString("Diff:\n")
		context.WriteString(diffContent)
		context.WriteString("\n")
	}

	return context.String()
}

func (a *AIService) parseReviewResponse(content string) (*CodeReview, error) {
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var review CodeReview
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		logrus.Warnf("Failed to parse AI response: %v. Content: %s", err, content)
		return a.createFallbackReview([]Diff{}), nil
	}

	if review.Summary == "" {
		review.Summary = "Automatic code review"
	}
	if review.Score == 0 {
		review.Score = 5
	}
	if review.Score > 10 {
		review.Score = 10
	}

	return &review, nil
}

func (a *AIService) createFallbackReview(diffs []Diff) *CodeReview {
	return &CodeReview{
		Summary: "Automatic review - basic analysis",
		Issues: []Issue{
			{
				Type:        "best_practice",
				Severity:    "low",
				Description: "A detailed code review is recommended",
				File:        "unknown",
				Line:        0,
				Code:        "",
				Fix:         "Review code manually",
			},
		},
		Suggestions: []string{
			"Check business logic",
			"Verify error handling",
			"Add tests if missing",
		},
		Score: 5,
	}
}
