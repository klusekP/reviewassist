package ai

import (
	"context"
)

type Issue struct {
	Type        string      `json:"type"`
	Severity    string      `json:"severity"`
	Description string      `json:"description"`
	File        string      `json:"file"`
	Line        interface{} `json:"line"`
	Code        string      `json:"code"`
	Fix         string      `json:"fix"`
}

// Requirement is one requirement from the task with its implementation status.
type Requirement struct {
	Requirement string `json:"requirement"`
	Status      string `json:"status"`   // done | partial | missing | unclear
	Evidence    string `json:"evidence"` // file:line or explanation
}

type CodeReview struct {
	Summary         string        `json:"summary"`
	Issues          []Issue       `json:"issues"`
	Suggestions     []string      `json:"suggestions"`
	Score           int           `json:"score"`
	Verdict         string        `json:"verdict,omitempty"`          // approve | approve_with_comments | request_changes
	SecuritySummary string        `json:"security_summary,omitempty"` // explicit security assessment
	QualitySummary  string        `json:"quality_summary,omitempty"`  // code volume, refactoring, performance
	Requirements    []Requirement `json:"requirements,omitempty"`     // task requirements checklist
}

type Ai interface {
	Review(ctx context.Context, prompt string) (*CodeReview, error)
}
