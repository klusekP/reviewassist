# Roadmap

Planned work, roughly in priority order. Items are proposals, not promises; dates are not set.
Ideas and pull requests are welcome – open an issue to discuss a bigger change first.

Status: 🔜 next · 📋 planned · 💡 idea

## 🔜 Security hardening (before the first public release)

- Restrict CORS to the panel's own origin instead of `*`.
- Optional access token for the panel and API (`APP_TOKEN`), required for changing settings and
  starting reviews.
- Bind to `127.0.0.1` by default; listening on all interfaces only on request.

## 🔜 Automatic review flow

Run reviews without clicking **Run review**.

- **GitLab webhook** endpoint (`POST /api/v1/webhooks/gitlab`) with secret token verification:
  review on *MR opened*, *new commits pushed* and *marked ready* (draft → ready).
- Rules per project: which events trigger a review, target branches, labels that enable or skip it
  (e.g. `ai-review`, `skip-ai-review`), ignore draft MRs, max MR size.
- Debounce: one review after a burst of pushes, cancel an outdated running review when a newer
  commit arrives.
- Queue with a concurrency limit and retries, visible in the panel.
- Automatic re-review on push, using the existing follow-up of previous findings.
- **CI mode**: a CLI command (`codereviewAI review --project … --mr …`) and a ready-to-use GitLab CI
  job, with an optional exit code based on the verdict (fail the pipeline on *request changes*).
- Automatic mode defaults to `comment` publishing or a dedicated bot token, because draft reviews
  are only visible to the token owner.
- Notifications when an automatic review finishes (Slack / Teams / e-mail webhook).

## 📋 GitHub support

- Provider abstraction for the code host (GitLab today), so reviews, publishing and re-reviews work
  the same on both.
- GitHub pull requests: list repositories and PRs, diffs, review comments on lines, one summary
  comment.
- Publish as a **pending review** (equivalent of GitLab drafts) or a submitted review with
  *Comment* / *Request changes*.
- Re-review using review threads: reply and resolve fixed or accepted findings.
- Authentication with a personal access token (fine-grained) and later a **GitHub App**.
- GitHub webhooks and a GitHub Actions workflow for the automatic review flow.
- GitHub Enterprise Server via a configurable base URL.
- Task context from GitHub Issues linked to the PR (`Fixes #123`), in addition to Jira.

## 📋 Review quality

- Configurable model for every provider (e.g. `OPENAI_MODEL`, `CLAUDE_MODEL`) instead of fixed
  models.
- Project review rules: a `.codereview.yml` (or a section in `CLAUDE.md`) with project-specific
  checklists, ignored paths and severity overrides.
- Feedback loop: mark findings as useful / false positive in the panel and use it to tune prompts.
- Cost and token usage per review.

## 💡 Ideas

- Bitbucket and Gitea / Forgejo support.
- Other trackers as task context: Linear, YouTrack, GitHub / GitLab issues.
- Statistics: findings per project, type and severity over time; average score; time to fix.
- Multi-user mode with logins and per-user tokens.
- More interface languages.

## Done

- ✅ Context-aware review with Jira task, project documentation and full file content.
- ✅ Claude Code agent engine with read-only tools.
- ✅ GitLab draft reviews and summary note.
- ✅ Re-review of previous findings (fixed / open / accepted).
- ✅ Settings editable in the UI, stored in SQLite, applied without restart.
- ✅ Review history in the database.
- ✅ Interface in Polish and English; review comments in Polish, English and German.
