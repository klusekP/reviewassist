package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"codereviewAI/services/ai"

	"github.com/sirupsen/logrus"
)

type Service struct {
	baseURL string
	model   string
	numCtx  int
}

type OllamaRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Format   string    `json:"format,omitempty"`
	Options  Options   `json:"options"`
}

type Options struct {
	Temperature float64 `json:"temperature"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaResponse struct {
	Message Message `json:"message"`
}

func NewService(model string) (*Service, error) {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	if model == "" {
		model = "llama2"
	}

	// Ollama's default context window (2-4k tokens) silently truncates review prompts
	// that include project docs, the full file and the Jira task.
	numCtx := 32768
	if v, err := strconv.Atoi(os.Getenv("OLLAMA_NUM_CTX")); err == nil && v > 0 {
		numCtx = v
	}

	return &Service{
		baseURL: baseURL,
		model:   model,
		numCtx:  numCtx,
	}, nil
}

func (s *Service) Review(ctx context.Context, prompt string) (*ai.CodeReview, error) {
	logrus.Infof("[Ollama] Sending review request to %s/api/chat (model: %s, num_ctx: %d, prompt ~%d chars)", s.baseURL, s.model, s.numCtx, len(prompt))

	requestBody := OllamaRequest{
		Model:  s.model,
		Stream: false,
		Messages: []Message{
			{
				Role:    "system",
				Content: "You are a code review expert. Respond only in JSON format. Write human-readable text in the language requested in the prompt.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Format:  "json",
		Options: Options{Temperature: 0.2, NumCtx: s.numCtx},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	url := fmt.Sprintf("%s/api/chat", s.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logrus.Errorf("[Ollama] Request failed: %v", err)
		return nil, fmt.Errorf("error calling Ollama API: %v", err)
	}
	defer resp.Body.Close()

	logrus.Infof("[Ollama] Response received: HTTP %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error: %s - %s", resp.Status, string(body))
	}

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("error decoding Ollama response: %v", err)
	}

	content := ollamaResp.Message.Content
	logrus.Infof("[Ollama] Parsing response (~%d chars)", len(content))
	return parseReviewResponse(content)
}

func parseReviewResponse(content string) (*ai.CodeReview, error) {
	content = strings.TrimSpace(content)
	if start, end := strings.Index(content, "{"), strings.LastIndex(content, "}"); start >= 0 && end > start {
		content = content[start : end+1]
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
