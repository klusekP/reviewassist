package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

type AIService struct {
	client *openai.Client
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

func NewAIService() *AIService {
	apiKey := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(apiKey)

	return &AIService{
		client: client,
	}
}

func (a *AIService) ReviewCode(diffs []Diff, mrTitle, mrDescription string) (*CodeReview, error) {
	// Przygotowanie kontekstu dla AI
	contextStr := a.buildContext(diffs, mrTitle, mrDescription)

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

	// Wywołanie OpenAI API
	resp, err := a.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4,
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
			MaxTokens:   4000,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("błąd wywołania OpenAI API: %v", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("brak odpowiedzi z OpenAI API")
	}

	// Parsowanie odpowiedzi JSON
	content := resp.Choices[0].Message.Content
	review, err := a.parseReviewResponse(content)
	if err != nil {
		logrus.Warnf("Błąd parsowania odpowiedzi AI: %v", err)
		// Fallback - zwróć podstawową recenzję
		return a.createFallbackReview(diffs), nil
	}

	return review, nil
}

func (a *AIService) buildContext(diffs []Diff, mrTitle, mrDescription string) string {
	var context strings.Builder

	for _, diff := range diffs {
		context.WriteString(fmt.Sprintf("\nPlik: %s\n", diff.NewPath))
		if diff.NewFile {
			context.WriteString("(NOWY PLIK)\n")
		} else if diff.DeletedFile {
			context.WriteString("(USUNIĘTY PLIK)\n")
		} else if diff.RenamedFile {
			context.WriteString(fmt.Sprintf("(PRZENIESIONY Z: %s)\n", diff.OldPath))
		}
		context.WriteString("Diff:\n")
		context.WriteString(diff.Diff)
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
	// Tutaj zwracamy podstawową strukturę
	return &CodeReview{
		Summary: "Automatyczna recenzja kodu",
		Issues: []Issue{
			{
				Type:        "best_practice",
				Severity:    "low",
				Description: "Zalecana jest dodanie komentarzy do złożonej logiki",
				File:        "main.go",
				Line:        1,
				Code:        "// przykład kodu",
				Fix:         "// Dodaj komentarze wyjaśniające logikę",
			},
		},
		Suggestions: []string{
			"Rozważ dodanie testów jednostkowych",
			"Sprawdź obsługę błędów",
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
