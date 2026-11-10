package services

import (
	"fmt"
	"strings"

	"codereviewAI/services/ai"
	"codereviewAI/services/i18n"

	"github.com/sirupsen/logrus"
)

var severityIcon = map[string]string{"critical": "🔴", "high": "🟠", "medium": "🟡", "low": "🔵"}

type labels struct {
	lang                                                                                                              string
	verdict, security, quality, requirements, reqHeader                                                               string
	verdicts                                                                                                          map[string]string
	header, task, noTask, score, taskFindings, outsideDiff, suggestions, fix, failedFiles, footerDraft, footerComment string

	reReview, reFixed, reOpen, reAccepted, reNew, replyFixed, replyAccepted string
}

func (c *ReviewConfig) labels() labels {
	lang := c.Lang()
	t := func(key string) string { return i18n.T(lang, "publish."+key) }
	return labels{
		lang:         lang,
		verdict:      t("verdict"),
		security:     t("security"),
		quality:      t("quality"),
		requirements: t("requirements"),
		reqHeader:    t("reqHeader"),
		verdicts: map[string]string{
			VerdictApprove:             t("verdict." + VerdictApprove),
			VerdictApproveWithComments: t("verdict." + VerdictApproveWithComments),
			VerdictRequestChanges:      t("verdict." + VerdictRequestChanges),
		},
		header:        t("header"),
		task:          t("task"),
		noTask:        t("noTask"),
		score:         t("score"),
		taskFindings:  t("taskFindings"),
		outsideDiff:   t("outsideDiff"),
		suggestions:   t("suggestions"),
		fix:           t("fix"),
		failedFiles:   t("failedFiles"),
		footerDraft:   t("footerDraft"),
		footerComment: t("footerComment"),
		reReview:      t("reReview"),
		reFixed:       t("reFixed"),
		reOpen:        t("reOpen"),
		reAccepted:    t("reAccepted"),
		reNew:         t("reNew"),
		replyFixed:    t("replyFixed"),
		replyAccepted: t("replyAccepted"),
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

	status := i18n.T(l.lang, "publish.status", mode, inline)
	if resolved > 0 {
		status += i18n.T(l.lang, "publish.status.resolved", resolved)
	}
	if failed > 0 {
		status += i18n.T(l.lang, "publish.status.moved", failed)
	}
	if mode == "draft" {
		status += i18n.T(l.lang, "publish.status.draft")
	}
	logrus.Infof("[GitLab] MR %d: %s", mr.IID, status)
	return status, nil
}
