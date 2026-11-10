# Web interface

The panel is a single page served at `http://localhost:$PORT`. Labels below are given in English;
the interface is in Polish by default and can be switched with the language button (see
[Languages](languages.md)).

## Layout

```
┌──────────────┬─────────────────────────────────────────────┐
│ Sidebar      │ Main area                                   │
│ - Overview   │ - Overview (dashboard)                      │
│ - Projects   │ - Project: list of merge requests           │
│   tree       │ - Merge request: Review / Changes /         │
│ - In progress│   Description tabs                          │
│ - Recent     │ - Settings                                  │
│ - Search     │                                             │
│ - Status bar │                                             │
└──────────────┴─────────────────────────────────────────────┘
```

## Sidebar

- **Overview** opens the dashboard. The refresh button next to it reloads the project list from
  GitLab, bypassing the 5-minute cache. The gear opens **Settings**.
- **Projects** is a tree of your GitLab projects with the number of open MRs.
  - Click a project name to open its MR list.
  - Click the chevron to expand the project and see its open MRs directly in the tree. Each MR has a
    colored dot with its review status (see [Status dots](#status-dots)).
  - Projects without open MRs are hidden. Use **Show projects without MRs (n)** to show them and
    **Hide projects without MRs** to hide them again.
- **In progress** lists reviews running right now, with the elapsed time.
- **Recent reviews** lists the six most recent reviews.
- **Search** (`Ctrl+K` / `Cmd+K`, `Esc` to clear) filters projects and MRs by project name, MR
  title, `!id`, branch or author.
- The **status bar** at the bottom shows:
  - the review engine and model, with a green dot when the live connection (WebSocket) to the
    server works and a red one when it is reconnecting,
  - `jira` when the Jira integration is enabled,
  - the review comment language (`REVIEW_LANGUAGE`),
  - the interface language button (`PL` / `EN`),
  - the theme switch (dark / light).

Which projects appear is controlled by `GITLAB_PROJECTS` (a list of IDs or paths). When it is empty,
all projects you are a member of are listed, up to `GITLAB_PROJECTS_LIMIT`.

## Overview (dashboard)

- Tiles: number of projects, open MRs, reviews in progress and completed reviews.
- **Projects** table: name and path, default branch, open MRs and last activity. Click a row to open
  the project.
- **Recent reviews** table: verdict, MR, project, score and time. Click a row to open the MR.

If GitLab is unreachable or the token has no access to any project, the dashboard shows the error
with a link to Settings.

## Project: merge request list

- Switch between **Open**, **Merged** and **Closed** merge requests.
- Filter the list by title, `!id`, branch or author.
- **Refresh** reloads the list, **GitLab** opens the project's MR list in GitLab.

Columns:

| Column | Meaning |
|---|---|
| Status | AI review status of the MR (see below) |
| Merge request | Title, `!iid`, *draft* and *conflicts* chips, the first four labels |
| Branches | Source → target branch |
| Author | MR author |
| Score | Score of the last review, 1–10 |
| Findings | Number of findings: red = critical + high, yellow = medium, blue = low |
| Updated | Last update of the MR |

### Status dots

| Dot / label | Meaning |
|---|---|
| purple, pulsing – *analyzing…* | A review is running |
| red – *review error* | The last review failed (details on the MR page) |
| none – *no review* | The MR has never been reviewed |
| green – *OK* | Verdict *approve*: nothing to change |
| yellow – *comments* | Verdict *approve with comments*: only medium/low findings |
| red – *changes needed* | Verdict *request changes*: critical/high findings or missing requirements |
| *… · outdated* | New commits were pushed after the review; consider running it again |

## Merge request page

The header shows the title, `!iid`, project, author, branches, added/removed lines, last update and
chips (*draft*, *conflicts*, *new commits since review*).

Buttons:

- **Run review** / **Re-run review** starts a review. While it runs the button shows *Analyzing…*
  and a progress bar appears:
  - `llm` engine: *file N of M*,
  - `claude-code` engine: the number of files and an indeterminate bar (the agent explores the code
    on its own).
- **GitLab** opens the MR in GitLab.
- **Trash** deletes the stored review results of this MR from the local database. Comments already
  published in GitLab are not touched.

### Review tab

1. **Verdict** with an icon, the review date, the requirements summary, the engine and the score.
   If new commits were pushed since, a warning is shown.
2. **Summary** – overall assessment, followed by the requirements line, the security and quality
   summaries and the GitLab publishing status.
3. **Task requirements** – a checklist built from the Jira task: ✅ done, ⚠️ partial, ❌ missing,
   ❓ to verify, each with evidence (file/line or explanation).
4. **🔁 Re-review** – shown when the review checked findings of a previous review: each previous
   finding is ✅ fixed, ⏳ still open or 👍 explanation accepted, with a link to its GitLab thread.
5. **Security** and **Code volume, quality and performance** – short assessments written even when
   nothing was found.
6. **Findings** – filter by severity, grouped by file (*General MR findings* for findings not tied to
   a line). Each card shows severity, type, location, description, the offending code and a
   **Suggested fix**. Click a card to jump to the line in the **Changes** tab.
7. **Review history** – appears when the MR was reviewed more than once. Click an entry to view an
   older review; *show latest* returns to the newest one.
8. **Suggestions** – general remarks that are not findings.

### Changes tab

- A file tree on the left: file name, number of findings, added and removed lines. Click a file to
  scroll to it.
- A unified diff per file with syntax highlighting. AI findings are shown inline under the line
  they refer to; findings that could not be placed are listed at the end of the file.
- Click a file header to collapse or expand it. Chips mark *new*, *deleted* and renamed files.

### Description tab

The MR description (Markdown) and details: author, branches, merge status (e.g. *ready to merge*,
*needs rebase*, *waiting for CI*), pipeline status, number of comments, labels, creation and update
dates, head commit, and how AI comments are published (draft review, immediate comments or
disabled).

## Settings

Settings are grouped: **GitLab**, **Jira**, **Review**, **Claude Code agent**, **LLM models**,
**Context and limits**, **Server**. The left menu scrolls to a group; a purple dot marks groups with
unsaved changes.

For every setting you see:

- its label, environment variable name and description,
- the **source** of the current value: `UI` (saved in the database), `.env` or `default`,
- a *restart* chip when the change needs an application restart (only `PORT`).

Editing:

- Change a value; the row is highlighted as changed. **undo** reverts the field.
- **restore from .env** / **restore default** removes the UI override; the field shows *will revert to
  .env* until you save.
- Secrets (tokens, API keys) are never sent to the browser. The field shows only the last four
  characters (`set (••••abcd) – type to change`). Leave it empty to keep the current value.
- **Save changes (n)** saves and applies all changes at once; **Discard** drops them.
- **Test connection** (GitLab, Jira, Claude Code agent) checks the **saved** settings, so save first.

A full description of every setting is in the [Configuration reference](configuration.md).

## Notifications

Toasts in the bottom-right corner report: review started, review finished (with the verdict),
review failed, settings saved (or *restart required*), and errors. The browser asks for
confirmation before leaving the page with unsaved settings.
