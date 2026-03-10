package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"codereviewAI/services/ai"
)

type Service struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type MessageResponse struct {
	Info struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	} `json:"info"`
	Parts []ResponsePart `json:"parts"`
}

type ResponsePart struct {
	Type     string `json:"type"`
	Content  string `json:"content,omitempty"`
	Text     string `json:"text,omitempty"`
	Language string `json:"language,omitempty"`
}

type MessageRequest struct {
	Parts   []RequestPart `json:"parts"`
	NoReply bool          `json:"noReply"`
}

type RequestPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func NewService(apiKey string) (*Service, error) {
	return &Service{
		baseURL:  "http://localhost:4096",
		username: "opencode",
		password: apiKey,
		client:   &http.Client{},
	}, nil
}

func (s *Service) Review(ctx context.Context, prompt string) (*ai.CodeReview, error) {
	sessionID, err := s.createSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	reqBody := MessageRequest{
		Parts: []RequestPart{
			{Type: "text", Text: prompt},
		},
		NoReply: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/session/"+sessionID+"/message", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.SetBasicAuth(s.username, s.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to OpenCode server: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenCode server returned status %d: %s", resp.StatusCode, string(body))
	}

	var msgResp MessageResponse
	if err := json.Unmarshal(body, &msgResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v, body: %s", err, string(body))
	}

	var content strings.Builder
	for _, part := range msgResp.Parts {
		if part.Type == "text" {
			if part.Content != "" {
				content.WriteString(part.Content)
			} else if part.Text != "" {
				content.WriteString(part.Text)
			}
		}
	}

	if content.Len() == 0 {
		return nil, fmt.Errorf("no response from OpenCode")
	}

	return parseReviewResponse(content.String())
}

func (s *Service) createSession(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/session", nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(s.username, s.password)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create session: %s", string(body))
	}

	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", err
	}

	return session.ID, nil
}

func parseReviewResponse(content string) (*ai.CodeReview, error) {
	content = strings.TrimSpace(content)

	re := regexp.MustCompile(`\{[\s\S]*\}`)
	match := re.FindString(content)
	if match == "" {
		return &ai.CodeReview{
			Summary:     content,
			Score:       5,
			Issues:      []ai.Issue{},
			Suggestions: []string{"Could not extract JSON from response"},
		}, nil
	}

	content = match

	var review ai.CodeReview
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %v", err)
	}

	for i := range review.Issues {
		switch v := review.Issues[i].Line.(type) {
		case float64:
			review.Issues[i].Line = int(v)
		case string:
			var line int
			fmt.Sscanf(v, "%d", &line)
			review.Issues[i].Line = line
		}
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
