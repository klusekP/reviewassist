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

	// Re-review: model's verdict on findings of the previous review (input from the model).
	PreviousFindings []FindingStatus `json:"previous_findings,omitempty"`
	// Re-review result attached by the app (output for the UI and history).
	Followup *Followup `json:"followup,omitempty"`
}

// Statuses of a finding from a previous review.
const (
	FindingFixed    = "fixed"    // problem no longer present in the current code
	FindingOpen     = "open"     // still present
	FindingAccepted = "accepted" // left as is, the author's explanation is valid
)

// FindingStatus is the model's assessment of one previous finding after the author's changes.
type FindingStatus struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Line    int    `json:"line"`    // current line of the problem in the new file (0 if fixed/unknown)
	Comment string `json:"comment"` // short justification
}

// FollowupItem is one previous finding with its re-review status.
type FollowupItem struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Severity    string `json:"severity"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Comment     string `json:"comment,omitempty"`
	URL         string `json:"url,omitempty"`
}

// Followup summarises how findings of the previous review were addressed.
type Followup struct {
	Source   string         `json:"source"` // gitlab (open bot threads) | history (last stored review)
	SinceSHA string         `json:"since_sha"`
	Fixed    int            `json:"fixed"`
	Open     int            `json:"open"`
	Accepted int            `json:"accepted"`
	New      int            `json:"new"`
	Items    []FollowupItem `json:"items"`
}

type Ai interface {
	Review(ctx context.Context, prompt string) (*CodeReview, error)
}
