package kimi

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
	client *http.Client
}

type KimiRequest struct {
	Model       string        `json:"model"`
	Messages    []KimiMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type KimiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type KimiResponse struct {
	Choices []KimiChoice `json:"choices"`
}

type KimiChoice struct {
	Message KimiMessage `json:"message"`
}

func NewService(apiKey string) (*Service, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("KIMI API key is required")
	}
	return &Service{
		apiKey: apiKey,
		client: &http.Client{},
	}, nil
}

func (s *Service) Review(ctx context.Context, prompt string) (*ai.CodeReview, error) {
	requestBody := KimiRequest{
		Model: "moonshot-v1-32k",
		Messages: []KimiMessage{
			{
				Role:    "system",
				Content: "You are a code review expert. Respond only in JSON format. All responses must be in English.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.3,
		MaxTokens:   4000,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.moonshot.cn/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error calling KIMI API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("KIMI API error: %s - %s", resp.Status, string(body))
	}

	var kimiResp KimiResponse
	if err := json.NewDecoder(resp.Body).Decode(&kimiResp); err != nil {
		return nil, fmt.Errorf("error decoding KIMI response: %v", err)
	}

	if len(kimiResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from KIMI API")
	}

	content := kimiResp.Choices[0].Message.Content
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
		return nil, fmt.Errorf("failed to parse AI response: %v", err)
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
