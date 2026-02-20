package cloude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"codereviewAI/services/ai"
)

type Service struct {
	apiKey string
}

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

func NewService(apiKey string) (*Service, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Claude API key is required")
	}
	return &Service{
		apiKey: apiKey,
	}, nil
}

func (s *Service) Review(ctx context.Context, prompt string) (*ai.CodeReview, error) {
	requestBody := ClaudeRequest{
		Model:       "claude-sonnet-4-5-20250929",
		MaxTokens:   8192, // code review JSON can be long; 2k was causing truncated "unexpected end of JSON input"
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

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
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
	return parseReviewResponse(content)
}

func parseReviewResponse(content string) (*ai.CodeReview, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	if content == "" {
		return nil, fmt.Errorf("failed to parse AI response: empty response")
	}

	var review ai.CodeReview
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %v (response may be truncated; try increasing max_tokens)", err)
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
