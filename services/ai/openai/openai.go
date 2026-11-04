package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"codereviewAI/services/ai"

	"github.com/sashabaranov/go-openai"
)

type Service struct {
	client *openai.Client
}

func NewService(apiKey string) (*Service, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}
	return &Service{
		client: openai.NewClient(apiKey),
	}, nil
}

func (s *Service) Review(ctx context.Context, prompt string) (*ai.CodeReview, error) {
	resp, err := s.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a code review expert. Respond only in JSON format. Write human-readable text in the language requested in the prompt.",
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
	return parseReviewResponse(content)
}

func parseReviewResponse(content string) (*ai.CodeReview, error) {
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

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
