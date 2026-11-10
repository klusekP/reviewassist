package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// FileReviewInput is everything the model gets to review a single changed file.
type FileReviewInput struct {
	Diff          Diff
	NumberedDiff  string
	FileContent   string
	OtherFiles    []string
	MRTitle       string
	MRDescription string
	SourceBranch  string
	TaskContext   string
	Project       *ProjectContext

	PreviousFindings string // re-review: findings already reported for this file
}

// TaskReviewInput is used for the MR-level check against the Jira task.
type TaskReviewInput struct {
	MRTitle       string
	MRDescription string
	SourceBranch  string
	TaskContext   string
	Project       *ProjectContext
	Diffs         []Diff
}

const issueJSONSchema = `{
  "summary": "1-3 sentences about this file's change",
  "issues": [
    {
      "type": "bug|security|performance|requirement|best_practice|refactoring|style",
      "severity": "low|medium|high|critical",
      "description": "what is wrong and why it matters (reference the project convention or task requirement if relevant)",
      "file": "%s",
      "line": <number from the left column of the DIFF, must be a line marked with +>,
      "code": "the offending code line(s)",
      "fix": "concrete fix, preferably with a short code example"
    }
  ],
  "suggestions": ["optional general remarks about this file"],
  "score": <1-10>
}`

func (a *AIService) ReviewFileWithContext(cfg *ReviewConfig, in FileReviewInput) (*CodeReview, error) {
	if a.aiProvider == nil {
		return nil, fmt.Errorf("AI provider is not configured")
	}
	d := in.Diff
	var p strings.Builder

	fmt.Fprintf(&p, `You are a senior engineer doing a code review of ONE file in a GitLab merge request of the project described below.

RULES:
1. Review ONLY the changes: lines marked with "+" in the DIFF section. Unchanged code, the full file, docs and structure are context only. Pre-existing problems in unchanged code are out of scope; report a problem only on the "+" line that introduces it, makes it worse or newly depends on it.
2. Use the project documentation, repository structure and the full file content to understand the project's architecture and conventions. Flag changes that violate documented conventions.
3. Use the task description (Jira) to judge whether the change does what it is supposed to do. Flag logic that contradicts the requirements or misses an edge case described in the task or its comments.
4. Focus on real problems: bugs, security, data integrity, error handling, performance, missing tests for new logic, readability problems that matter. Do not nitpick formatting. Do not invent problems - an empty "issues" list is a valid answer.
5. "line" MUST be the number shown in the left column of the DIFF for a line marked with "+". Never use a line that is not in the diff.
6. Write "summary", "description", "fix" and "suggestions" in %s. Keep code, identifiers and file paths unchanged.
7. Go through the security and the code volume / refactoring / performance checklists below for the changed lines.
8. Respond with JSON only, no markdown fences, exactly in this format:
`, cfg.LanguageName())
	fmt.Fprintf(&p, issueJSONSchema, d.NewPath)
	p.WriteString("\n" + securityChecklist + qualityChecklist + severityGuide)

	p.WriteString("\n\n## TASK (Jira)\n")
	if in.TaskContext != "" {
		p.WriteString(in.TaskContext)
	} else {
		p.WriteString("No Jira task linked. Infer intent from the MR title and description.\n")
	}

	fmt.Fprintf(&p, "\n## MERGE REQUEST\nTitle: %s\nBranch: %s\nDescription:\n%s\n", in.MRTitle, in.SourceBranch, orDash(in.MRDescription))

	if in.Project != nil && in.Project.Docs != "" {
		p.WriteString("\n## PROJECT DOCUMENTATION\n")
		p.WriteString(in.Project.Docs)
	}
	if in.Project != nil && in.Project.Structure != "" {
		p.WriteString("\n## REPOSITORY STRUCTURE (partial)\n")
		p.WriteString(truncate(in.Project.Structure, 4000, "..."))
		p.WriteString("\n")
	}
	if len(in.OtherFiles) > 0 {
		p.WriteString("\n## OTHER FILES CHANGED IN THIS MR\n")
		p.WriteString(strings.Join(in.OtherFiles, "\n"))
		p.WriteString("\n")
	}
	p.WriteString(in.PreviousFindings)

	if in.FileContent != "" {
		fmt.Fprintf(&p, "\n## FULL FILE AFTER CHANGE: %s\n", d.NewPath)
		p.WriteString(truncate(numberLines(in.FileContent), cfg.FileMaxChars, "[file truncated]"))
	}

	fmt.Fprintf(&p, "\n## DIFF: %s %s\n", d.NewPath, strings.TrimSpace(a.diffFileTypeLabel(d)))
	p.WriteString("(left column = line number in the new file; \"+\" added, \"-\" removed, blank = unchanged context)\n")
	p.WriteString(truncate(in.NumberedDiff, cfg.DiffMaxChars, "[diff truncated]"))

	logrus.Infof("[AI] Reviewing %s (prompt ~%d chars)", d.NewPath, p.Len())
	review, err := a.aiProvider.Review(context.Background(), p.String())
	if err != nil {
		return nil, err
	}
	for i := range review.Issues {
		review.Issues[i].File = d.NewPath
	}
	return review, nil
}

func (a *AIService) ReviewTaskCompliance(cfg *ReviewConfig, in TaskReviewInput) (*CodeReview, error) {
	if a.aiProvider == nil {
		return nil, fmt.Errorf("AI provider is not configured")
	}
	var p strings.Builder
	fmt.Fprintf(&p, `You are a senior engineer reviewing a whole GitLab merge request against its task (Jira issue).

Your job:
1. Check whether the changes implement what the task and its comments ask for. List requirements that are missing, partially done, or implemented differently than described.
2. Point out changes that are outside the task's scope.
   Only assess code changed by this MR; do not report on unchanged parts of the project.
3. Point out cross-file problems that a per-file review would miss (e.g. missing migration, missing tests, config not updated, inconsistent naming between layers, docs not updated).
4. Assess security of the change as a whole (checklist below): access control of new endpoints/operations, exposed or writable fields, validation, secrets.
5. Do not repeat line-level nitpicks. Do not invent problems.

Write "summary", "description", "fix" and "suggestions" in %s. Keep code, identifiers and paths unchanged.
Respond with JSON only, no markdown fences:
{
  "summary": "overall assessment: does the MR fulfil the task? 2-5 sentences",
  "issues": [
    {"type": "requirement|scope|architecture|tests|bug", "severity": "low|medium|high|critical",
     "description": "...", "file": "path or empty", "line": 0, "code": "", "fix": "what should be done"}
  ],
  "suggestions": ["..."],
  "score": <1-10, how well the MR fulfils the task with good quality>,
  "verdict": "approve|approve_with_comments|request_changes",
  "security_summary": "explicit security assessment",
  "quality_summary": "assessment of code volume, refactoring needs and performance",
  "requirements": [
    {"requirement": "...", "status": "done|partial|missing|unclear", "evidence": "file:line or explanation"}
  ]
}
`, cfg.LanguageName())
	p.WriteString(reviewGuidelines)

	p.WriteString("\n## TASK (Jira)\n")
	if in.TaskContext != "" {
		p.WriteString(in.TaskContext)
	} else {
		p.WriteString("No Jira task linked. Judge against the MR title and description only.\n")
	}
	fmt.Fprintf(&p, "\n## MERGE REQUEST\nTitle: %s\nBranch: %s\nDescription:\n%s\n", in.MRTitle, in.SourceBranch, orDash(in.MRDescription))
	if in.Project != nil && in.Project.Docs != "" {
		p.WriteString("\n## PROJECT DOCUMENTATION\n")
		p.WriteString(in.Project.Docs)
	}
	if in.Project != nil && in.Project.Structure != "" {
		p.WriteString("\n## REPOSITORY STRUCTURE (partial)\n")
		p.WriteString(in.Project.Structure)
		p.WriteString("\n")
	}

	p.WriteString("\n## ALL CHANGES\n")
	budget := cfg.DiffMaxChars * 3
	perFile := budget / max(1, len(in.Diffs))
	if perFile < 800 {
		perFile = 800
	}
	var changes strings.Builder
	for _, d := range in.Diffs {
		fmt.Fprintf(&changes, "\n### %s %s\n%s\n", d.NewPath, strings.TrimSpace(a.diffFileTypeLabel(d)), truncate(d.Diff, perFile, "[diff truncated]"))
	}
	p.WriteString(truncate(changes.String(), budget, "[remaining changes truncated]"))

	logrus.Infof("[AI] Reviewing MR against task (prompt ~%d chars)", p.Len())
	return a.aiProvider.Review(context.Background(), p.String())
}
