package ai

import "context"

type Issue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Code        string `json:"code"`
	Fix         string `json:"fix"`
}

type CodeReview struct {
	Summary     string   `json:"summary"`
	Issues      []Issue  `json:"issues"`
	Suggestions []string `json:"suggestions"`
	Score       int      `json:"score"`
}

type Ai interface {
	Review(ctx context.Context, prompt string) (*CodeReview, error)
}
