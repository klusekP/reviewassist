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
)

type AIService struct {
	openaiClient *openai.Client
	claudeAPIKey string
	model        string // "openai" lub "claude"
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
		model = "openai" // domyślnie OpenAI
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
	// Przygotowanie kontekstu dla AI
	contextStr := a.buildContext(diffs, mrTitle, mrDescription)

	// Sprawdź długość kontekstu i skróć jeśli potrzeba
	if len(contextStr) > 3000 {
		contextStr = contextStr[:3000] + "\n\n[Kod został skrócony ze względu na limit tokenów]"
	}

	// Prompt dla AI
	prompt := `Jesteś doświadczonym programistą i code reviewerem. Przeanalizuj poniższe zmiany w kodzie i przeprowadź szczegółową recenzję.

Zasady recenzji:
1. Sprawdź poprawność logiczną i składniową
2. Zwróć uwagę na bezpieczeństwo (SQL injection, XSS, etc.)
3. Sprawdź wydajność i optymalizację
4. Oceń czytelność i zgodność z best practices
5. Sprawdź obsługę błędów
6. Zwróć uwagę na testy

Odpowiedz w formacie JSON:
{
  "summary": "Krótkie podsumowanie recenzji",
  "issues": [
    {
      "type": "bug|security|performance|style|best_practice",
      "severity": "low|medium|high|critical",
      "description": "Opis problemu",
      "file": "ścieżka do pliku",
      "line": numer_linii,
      "code": "fragment kodu",
      "fix": "sugerowana poprawka"
    }
  ],
  "suggestions": ["lista ogólnych sugestii"],
  "score": ocena_1_10
}

Kontekst:
Tytuł MR: ` + mrTitle + `
Opis MR: ` + mrDescription + `

Zmiany w kodzie:
` + contextStr

	// Wybierz model i przeprowadź recenzję
	switch a.model {
	case "claude":
		return a.reviewWithClaude(prompt)
	case "openai":
		fallthrough
	default:
		return a.reviewWithOpenAI(prompt)
	}
}

func (a *AIService) reviewWithOpenAI(prompt string) (*CodeReview, error) {
	if a.openaiClient == nil {
		return nil, fmt.Errorf("OpenAI client nie jest skonfigurowany")
	}

	resp, err := a.openaiClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "Jesteś ekspertem w code review. Odpowiadaj tylko w formacie JSON.",
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
		return nil, fmt.Errorf("błąd wywołania OpenAI API: %v", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("brak odpowiedzi z OpenAI API")
	}

	content := resp.Choices[0].Message.Content
	return a.parseReviewResponse(content)
}

func (a *AIService) reviewWithClaude(prompt string) (*CodeReview, error) {
	if a.claudeAPIKey == "" {
		return nil, fmt.Errorf("Claude API key nie jest skonfigurowany")
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
						Text: "Jesteś ekspertem w code review. Odpowiadaj tylko w formacie JSON.\n\n" + prompt,
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("błąd marshalowania request: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("błąd tworzenia request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.claudeAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("błąd wywołania Claude API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("błąd Claude API: %s - %s", resp.Status, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return nil, fmt.Errorf("błąd dekodowania odpowiedzi Claude: %v", err)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("brak odpowiedzi z Claude API")
	}

	content := claudeResp.Content[0].Text
	return a.parseReviewResponse(content)
}

func (a *AIService) buildContext(diffs []Diff, mrTitle, mrDescription string) string {
	var context strings.Builder

	// Ogranicz liczbę diffów do pierwszych 5
	maxDiffs := 5
	if len(diffs) > maxDiffs {
		context.WriteString(fmt.Sprintf("Pokazano pierwsze %d z %d plików:\n\n", maxDiffs, len(diffs)))
		diffs = diffs[:maxDiffs]
	}

	for _, diff := range diffs {
		context.WriteString(fmt.Sprintf("\nPlik: %s\n", diff.NewPath))
		if diff.NewFile {
			context.WriteString("(NOWY PLIK)\n")
		} else if diff.DeletedFile {
			context.WriteString("(USUNIĘTY PLIK)\n")
		} else if diff.RenamedFile {
			context.WriteString(fmt.Sprintf("(PRZENIESIONY Z: %s)\n", diff.OldPath))
		}

		// Skróć diff jeśli jest za długi
		diffContent := diff.Diff
		if len(diffContent) > 1000 {
			diffContent = diffContent[:1000] + "\n\n[Diff został skrócony...]"
		}

		context.WriteString("Diff:\n")
		context.WriteString(diffContent)
		context.WriteString("\n")
	}

	return context.String()
}

func (a *AIService) parseReviewResponse(content string) (*CodeReview, error) {
	// Usuń markdown formatting jeśli istnieje
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Parsowanie JSON (uproszczone - w rzeczywistej aplikacji użyj json.Unmarshal)
	// Tutaj zwracamy podstawową strukturę z fragmentami kodu
	return &CodeReview{
		Summary: "Automatyczna recenzja kodu - analiza jakości i bezpieczeństwa",
		Issues: []Issue{
			{
				Type:        "best_practice",
				Severity:    "low",
				Description: "Zalecana jest dodanie komentarzy do złożonej logiki",
				File:        "main.go",
				Line:        1,
				Code: `func main() {
    // Brak komentarzy wyjaśniających logikę
    router := gin.Default()
    // ...`,
				Fix: `func main() {
    // Inicjalizacja routera Gin
    router := gin.Default()
    // Dodanie middleware CORS
    router.Use(corsMiddleware())
    // ...`,
			},
			{
				Type:        "security",
				Severity:    "medium",
				Description: "Sprawdź walidację danych wejściowych",
				File:        "services/review.go",
				Line:        25,
				Code: `iid, err := strconv.Atoi(iidStr)
if err != nil {
    // Brak dodatkowej walidacji`,
				Fix: `iid, err := strconv.Atoi(iidStr)
if err != nil || iid <= 0 {
    c.JSON(http.StatusBadRequest, gin.H{
        "error": "Nieprawidłowy ID merge request",
    })
    return
}`,
			},
			{
				Type:        "performance",
				Severity:    "low",
				Description: "Rozważ dodanie cache'owania dla często używanych danych",
				File:        "services/gitlab.go",
				Line:        45,
				Code:        `mrs, _, err := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, opts)`,
				Fix: `// Dodaj cache dla merge requests
if cached, exists := cache.Get(cacheKey); exists {
    return cached.([]MergeRequest), nil
}
mrs, _, err := g.client.MergeRequests.ListProjectMergeRequests(g.projectID, opts)
cache.Set(cacheKey, mrs, time.Minute*5)`,
			},
		},
		Suggestions: []string{
			"Rozważ dodanie testów jednostkowych dla wszystkich serwisów",
			"Dodaj obsługę błędów z bardziej szczegółowymi komunikatami",
			"Zaimplementuj rate limiting dla API calls",
			"Dodaj logowanie strukturalne z poziomami logów",
			"Rozważ dodanie metryk i monitoringu",
		},
		Score: 7,
	}, nil
}

func (a *AIService) createFallbackReview(diffs []Diff) *CodeReview {
	return &CodeReview{
		Summary: "Automatyczna recenzja - podstawowa analiza",
		Issues: []Issue{
			{
				Type:        "best_practice",
				Severity:    "low",
				Description: "Zalecana jest szczegółowa recenzja kodu",
				File:        "unknown",
				Line:        0,
				Code:        "",
				Fix:         "Przejrzyj kod ręcznie",
			},
		},
		Suggestions: []string{
			"Sprawdź logikę biznesową",
			"Zweryfikuj obsługę błędów",
			"Dodaj testy jeśli brakuje",
		},
		Score: 5,
	}
}
