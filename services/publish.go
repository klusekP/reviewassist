package services

import (
	"fmt"
	"strings"

	"codereviewAI/services/ai"

	"github.com/sirupsen/logrus"
)

var severityIcon = map[string]string{"critical": "🔴", "high": "🟠", "medium": "🟡", "low": "🔵"}

type labels struct {
	verdict, security, quality, requirements, reqHeader                                                               string
	verdicts                                                                                                          map[string]string
	header, task, noTask, score, taskFindings, outsideDiff, suggestions, fix, failedFiles, footerDraft, footerComment string

	reReview, reFixed, reOpen, reAccepted, reNew, replyFixed, replyAccepted string
}

func (c *ReviewConfig) labels() labels {
	if strings.HasPrefix(strings.ToLower(c.Language), "pl") || strings.EqualFold(c.Language, "polski") {
		return labels{
			verdict:      "Werdykt",
			security:     "### 🔒 Bezpieczeństwo",
			quality:      "### ⚡ Jakość, ilość kodu i wydajność",
			requirements: "### 📋 Realizacja wymagań z zadania",
			reqHeader:    "| | Wymaganie | Gdzie / uwagi |",
			verdicts: map[string]string{
				VerdictApprove:             "✅ Można zatwierdzić, nie znaleziono problemów",
				VerdictApproveWithComments: "💬 Można zatwierdzić po drobnych poprawkach",
				VerdictRequestChanges:      "⛔ Wymagane zmiany przed merge",
			},
			header:        "## 🤖 AI Code Review",
			task:          "Zadanie",
			noTask:        "Nie znaleziono powiązanego zadania w Jira (brak klucza w tytule, gałęzi lub opisie MR).",
			score:         "Ocena",
			taskFindings:  "### Zgodność z zadaniem i uwagi ogólne",
			outsideDiff:   "### Uwagi, których nie udało się przypiąć do linii",
			suggestions:   "### Sugestie",
			fix:           "Propozycja poprawki",
			failedFiles:   "### Pliki, których nie udało się przeanalizować",
			footerDraft:   "_Komentarze wygenerowane automatycznie jako szkic review. Zweryfikuj je przed opublikowaniem._",
			footerComment: "_Komentarze wygenerowane automatycznie przez AI._",
			reReview:      "### 🔁 Ponowne review (zmiany od `%s`)",
			reFixed:       "poprawione",
			reOpen:        "nadal otwarte",
			reAccepted:    "zaakceptowane",
			reNew:         "nowe uwagi",
			replyFixed:    "✅ **Poprawione** w `%s`",
			replyAccepted: "👍 **Wyjaśnienie przyjęte**, problem pozostawiony świadomie",
		}
	}
	return labels{
		verdict:      "Verdict",
		security:     "### 🔒 Security",
		quality:      "### ⚡ Code volume, refactoring and performance",
		requirements: "### 📋 Task requirements",
		reqHeader:    "| | Requirement | Where / notes |",
		verdicts: map[string]string{
			VerdictApprove:             "✅ Approve, no issues found",
			VerdictApproveWithComments: "💬 Approve after minor fixes",
			VerdictRequestChanges:      "⛔ Changes required before merge",
		},
		header:        "## 🤖 AI Code Review",
		task:          "Task",
		noTask:        "No linked Jira task found (no key in MR title, branch or description).",
		score:         "Score",
		taskFindings:  "### Task compliance and general findings",
		outsideDiff:   "### Findings that could not be attached to a line",
		suggestions:   "### Suggestions",
		fix:           "Suggested fix",
		failedFiles:   "### Files that could not be reviewed",
		footerDraft:   "_Generated automatically as a draft review. Verify before submitting._",
		footerComment: "_Generated automatically by AI._",
		reReview:      "### 🔁 Re-review (changes since `%s`)",
		reFixed:       "fixed",
		reOpen:        "still open",
		reAccepted:    "accepted",
		reNew:         "new findings",
		replyFixed:    "✅ **Fixed** in `%s`",
		replyAccepted: "👍 **Explanation accepted**, issue left intentionally",
	}
}

var requirementIcon = map[string]string{"done": "✅", "partial": "⚠️", "missing": "❌", "unclear": "❓"}

// tableCell makes text safe for a single Markdown table cell.
func tableCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\n", "<br>")
}

func formatIssue(l labels, is ai.Issue, withLocation bool) string {
	var b strings.Builder
	icon := severityIcon[strings.ToLower(is.Severity)]
	fmt.Fprintf(&b, "%s **[%s] %s**", icon, is.Severity, is.Type)
	if withLocation && is.File != "" {
		if line := issueLine(is.Line); line > 0 {
			fmt.Fprintf(&b, " · `%s:%d`", is.File, line)
		} else {
			fmt.Fprintf(&b, " · `%s`", is.File)
		}
	}
	fmt.Fprintf(&b, "\n\n%s", strings.TrimSpace(is.Description))
	if fix := strings.TrimSpace(is.Fix); fix != "" {
		fmt.Fprintf(&b, "\n\n**%s:**\n%s", l.fix, fix)
	}
	return b.String()
}

// publishToGitLab posts inline comments for each finding and one summary note.
// Returns a short human-readable status line.
func (r *ReviewService) publishToGitLab(mr *MergeRequest, refs *DiffRefs, results []fileResult, taskReview *ai.CodeReview, jiraIssues []*JiraIssue, rr *reReview) (string, error) {
	cfg := r.config
	l := cfg.labels()
	mode := cfg.GitLabMode
	if mode != "comment" {
		mode = "draft"
		if n, err := r.gitlabService.DeleteBotDrafts(mr.IID); err != nil {
			logrus.Warnf("[GitLab] Cannot clean previous drafts: %v", err)
		} else if n > 0 {
			logrus.Infof("[GitLab] Removed %d previous AI draft(s) from MR %d", n, mr.IID)
		}
	}

	inline, failed := 0, 0
	var outside []ai.Issue
	var failedFiles []string

	for _, res := range results {
		if res.failed {
			failedFiles = append(failedFiles, res.diff.NewPath)
			continue
		}
		for _, is := range res.review.Issues {
			if !cfg.SeverityAllowed(is.Severity) || strings.TrimSpace(is.Description) == "" {
				continue
			}
			newLine, oldLine, ok := res.lines.ResolveLine(issueLine(is.Line))
			if !ok {
				// not on a line changed by this MR - out of scope
				logrus.Infof("[GitLab] Skipping finding outside changed lines %s:%v: %s", is.File, is.Line, truncate(is.Description, 120, "…"))
				continue
			}
			err := r.gitlabService.PublishNote(mr.IID, refs, InlineNote{
				Body:    formatIssue(l, is, false),
				OldPath: res.diff.OldPath,
				NewPath: res.diff.NewPath,
				NewLine: newLine,
				OldLine: oldLine,
			}, mode)
			if err != nil {
				// GitLab rejects positions it cannot map - keep the finding in the summary instead
				logrus.Warnf("[GitLab] Inline note on %s:%d failed: %v", res.diff.NewPath, newLine, err)
				failed++
				outside = append(outside, is)
				continue
			}
			inline++
		}
	}

	// Re-review: answer and resolve threads of fixed / accepted findings
	resolved := 0
	if rr != nil && rr.source == "gitlab" {
		for _, f := range rr.findings {
			if f.DiscussionID == "" || f.Status == ai.FindingOpen {
				continue
			}
			body := l.replyAccepted
			if f.Status == ai.FindingFixed {
				body = fmt.Sprintf(l.replyFixed, shortSha(refs.HeadSha))
			}
			if f.Comment != "" {
				body += "\n\n" + f.Comment
			}
			if err := r.gitlabService.ReplyToThread(mr.IID, f.DiscussionID, body, true, mode); err != nil {
				logrus.Warnf("[GitLab] Reply to thread %s failed: %v", f.DiscussionID, err)
				continue
			}
			resolved++
		}
	}

	// Summary note
	var b strings.Builder
	b.WriteString(l.header + "\n\n")
	if rr != nil {
		fu := rr.followup()
		fmt.Fprintf(&b, l.reReview+"\n\n", shortSha(rr.sinceSHA))
		fmt.Fprintf(&b, "✅ %s: **%d** · ⏳ %s: **%d** · 👍 %s: **%d** · 🆕 %s: **%d**\n\n",
			l.reFixed, fu.Fixed, l.reOpen, fu.Open, l.reAccepted, fu.Accepted, l.reNew, fu.New)
		for _, it := range fu.Items {
			if it.Status != ai.FindingOpen {
				continue
			}
			loc := fmt.Sprintf("`%s:%d`", it.File, it.Line)
			if it.URL != "" {
				loc = fmt.Sprintf("[%s](%s)", loc, it.URL)
			}
			desc := strings.SplitN(strings.TrimSpace(it.Description), "\n", 2)[0]
			fmt.Fprintf(&b, "- ⏳ %s **[%s]** %s %s\n", severityIcon[it.Severity], it.Severity, loc, truncate(desc, 200, "…"))
		}
		b.WriteString("\n")
	}
	if len(jiraIssues) > 0 {
		for _, ji := range jiraIssues {
			fmt.Fprintf(&b, "**%s:** [%s](%s) %s (%s)\n\n", l.task, ji.Key, ji.URL, ji.Summary, ji.Status)
		}
	} else if r.jira != nil {
		b.WriteString(l.noTask + "\n\n")
	}
	if taskReview != nil {
		if v, ok := l.verdicts[taskReview.Verdict]; ok {
			fmt.Fprintf(&b, "**%s:** %s\n\n", l.verdict, v)
		}
		fmt.Fprintf(&b, "%s\n\n**%s:** %d/10\n\n", strings.TrimSpace(taskReview.Summary), l.score, taskReview.Score)
		if len(taskReview.Requirements) > 0 {
			fmt.Fprintf(&b, "%s\n\n%s\n|---|---|---|\n", l.requirements, l.reqHeader)
			for _, rq := range taskReview.Requirements {
				fmt.Fprintf(&b, "| %s | %s | %s |\n", requirementIcon[strings.ToLower(rq.Status)], tableCell(rq.Requirement), tableCell(rq.Evidence))
			}
			b.WriteString("\n")
		}
		if sec := strings.TrimSpace(taskReview.SecuritySummary); sec != "" {
			fmt.Fprintf(&b, "%s\n\n%s\n\n", l.security, sec)
		}
		if q := strings.TrimSpace(taskReview.QualitySummary); q != "" {
			fmt.Fprintf(&b, "%s\n\n%s\n\n", l.quality, q)
		}
		var taskIssues []ai.Issue
		for _, is := range taskReview.Issues {
			if cfg.SeverityAllowed(is.Severity) && strings.TrimSpace(is.Description) != "" {
				taskIssues = append(taskIssues, is)
			}
		}
		if len(taskIssues) > 0 {
			b.WriteString(l.taskFindings + "\n\n")
			for _, is := range taskIssues {
				b.WriteString("- " + strings.ReplaceAll(formatIssue(l, is, true), "\n", "\n  ") + "\n")
			}
			b.WriteString("\n")
		}
	}
	if len(outside) > 0 {
		b.WriteString(l.outsideDiff + "\n\n")
		for _, is := range outside {
			b.WriteString("- " + strings.ReplaceAll(formatIssue(l, is, true), "\n", "\n  ") + "\n")
		}
		b.WriteString("\n")
	}
	if taskReview != nil && len(taskReview.Suggestions) > 0 {
		b.WriteString(l.suggestions + "\n\n")
		for _, s := range taskReview.Suggestions {
			b.WriteString("- " + s + "\n")
		}
		b.WriteString("\n")
	}
	if len(failedFiles) > 0 {
		b.WriteString(l.failedFiles + "\n\n")
		for _, f := range failedFiles {
			b.WriteString("- `" + f + "`\n")
		}
		b.WriteString("\n")
	}
	if mode == "draft" {
		b.WriteString(l.footerDraft)
	} else {
		b.WriteString(l.footerComment)
	}

	if err := r.gitlabService.PublishNote(mr.IID, refs, InlineNote{Body: b.String()}, mode); err != nil {
		return "", fmt.Errorf("summary note: %v", err)
	}

	status := fmt.Sprintf("GitLab (%s): %d komentarzy w kodzie + podsumowanie", mode, inline)
	if resolved > 0 {
		status += fmt.Sprintf(", %d wątków z poprzedniego review zamkniętych", resolved)
	}
	if failed > 0 {
		status += fmt.Sprintf(", %d przeniesionych do podsumowania", failed)
	}
	if mode == "draft" {
		status += ". Otwórz MR w GitLab → „Submit review”, aby je opublikować."
	}
	logrus.Infof("[GitLab] MR %d: %s", mr.IID, status)
	return status, nil
}
