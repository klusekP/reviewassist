package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"codereviewAI/services/ai"
	aiClaude "codereviewAI/services/ai/cloude"
	aiKimi "codereviewAI/services/ai/kimi"
	aiOllama "codereviewAI/services/ai/ollama"
	aiOpenAI "codereviewAI/services/ai/openai"

	"github.com/sirupsen/logrus"
)

type AIService struct {
	aiProvider ai.Ai
}

type (
	CodeReview = ai.CodeReview
	Issue      = ai.Issue
)

func NewAIService() *AIService {
	model := os.Getenv("AI_MODEL")
	if model == "" {
		model = "openai"
	}

	var aiProvider ai.Ai
	var err error

	switch model {
	case "claude":
		claudeAPIKey := os.Getenv("CLAUDE_API_KEY")
		aiProvider, err = aiClaude.NewService(claudeAPIKey)
		if err != nil {
			logrus.Errorf("Failed to create Claude service: %v", err)
			openaiAPIKey := os.Getenv("OPENAI_API_KEY")
			aiProvider, _ = aiOpenAI.NewService(openaiAPIKey)
		}
	case "kimi":
		kimiAPIKey := os.Getenv("KIMI_API_KEY")
		aiProvider, err = aiKimi.NewService(kimiAPIKey)
		if err != nil {
			logrus.Errorf("Failed to create KIMI service: %v", err)
			openaiAPIKey := os.Getenv("OPENAI_API_KEY")
			aiProvider, _ = aiOpenAI.NewService(openaiAPIKey)
		}
	case "ollama":
		ollamaModel := os.Getenv("OLLAMA_MODEL")
		aiProvider, err = aiOllama.NewService(ollamaModel)
		if err != nil {
			logrus.Errorf("Failed to create Ollama service: %v", err)
			openaiAPIKey := os.Getenv("OPENAI_API_KEY")
			aiProvider, _ = aiOpenAI.NewService(openaiAPIKey)
		}
	case "openai":
		fallthrough
	default:
		openaiAPIKey := os.Getenv("OPENAI_API_KEY")
		aiProvider, err = aiOpenAI.NewService(openaiAPIKey)
		if err != nil {
			logrus.Errorf("Failed to create OpenAI service: %v", err)
		}
	}

	return &AIService{
		aiProvider: aiProvider,
	}
}

func (a *AIService) ReviewCode(diffs []Diff, mrTitle, mrDescription string) (*CodeReview, error) {
	logrus.Infof("[AI] ReviewCode: %d diffs, building prompt...", len(diffs))
	if a.aiProvider == nil {
		return a.createFallbackReview(diffs), fmt.Errorf("AI provider is not configured")
	}

	contextStr := a.buildContext(diffs, mrTitle, mrDescription)

	if len(contextStr) > 3000 {
		contextStr = contextStr[:3000] + "\n\n[Code was truncated due to token limit]"
	}

	prompt := `You are an experienced senior software architect and code reviewer. Analyze the following code changes and perform a comprehensive code review with focus on code quality, design principles, and refactoring opportunities.

Review guidelines:
1. Check logical and syntactic correctness
2. Pay attention to security (SQL injection, XSS, etc.)
3. Check performance and optimization
4. Evaluate readability and compliance with best practices
5. Check error handling
6. Pay attention to tests

DESIGN PRINCIPLES ANALYSIS (CRITICAL):
7. SOLID Principles:
   - Single Responsibility Principle (SRP): Each class/function should have one reason to change
   - Open/Closed Principle (OCP): Open for extension, closed for modification
   - Liskov Substitution Principle (LSP): Subtypes must be substitutable for their base types
   - Interface Segregation Principle (ISP): Clients shouldn't depend on interfaces they don't use
   - Dependency Inversion Principle (DIP): Depend on abstractions, not concretions
8. KISS (Keep It Simple, Stupid): Code should be as simple as possible
9. DRY (Don't Repeat Yourself): Avoid code duplication
10. YAGNI (You Aren't Gonna Need It): Don't add functionality until needed
11. Clean Code principles: Meaningful names, small functions, clear structure
12. Design Patterns: Identify opportunities to apply appropriate patterns
13. Test Driven Development (TDD): Write tests before writing code
14. Check directory structure and file naming conventions
15. Use consistent naming conventions for variables, functions, and classes
16. Avoid magic numbers and use constants instead
17. Use appropriate data structures and algorithms
18. Document code clearly and concisely
19. Use comments to explain complex code logic

REFACTORING FOCUS:
- Analyze NEW CODE (lines starting with +) for refactoring opportunities
- Suggest concrete refactorings to improve code quality
- Propose better structure, separation of concerns, and abstraction levels
- Identify code smells: long methods, large classes, duplicated code, feature envy, etc.
- Suggest improvements for maintainability, testability, and extensibility

Respond in JSON format:
{
  "summary": "Brief summary of the review including overall code quality assessment",
  "issues": [
    {
      "type": "bug|security|performance|style|best_practice|refactoring",
      "severity": "low|medium|high|critical",
      "description": "Detailed problem description with explanation of why it violates principles (e.g., 'Violates SRP: function handles both data validation and database operations')",
      "file": "file path",
      "line": line_number,
      "code": "code snippet that needs improvement",
      "fix": "Concrete refactoring suggestion with example code showing how to improve it. For refactoring issues, provide before/after code examples."
    }
  ],
  "suggestions": ["list of general suggestions for overall improvement"],
  "score": score_1_10
}

IMPORTANT:
- Focus especially on NEW CODE (added lines marked with +)
- For each refactoring suggestion, explain which principle it violates and how the refactoring improves it
- Provide concrete, actionable refactoring proposals with code examples
- Prioritize issues that affect code maintainability and extensibility

Context:
MR Title: ` + mrTitle + `
MR Description: ` + mrDescription + `

Code changes:
` + contextStr

	review, err := a.aiProvider.Review(context.Background(), prompt)
	if err != nil {
		logrus.Warnf("Failed to get AI review: %v. Using fallback review.", err)
		return a.createFallbackReview(diffs), nil
	}

	return review, nil
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
