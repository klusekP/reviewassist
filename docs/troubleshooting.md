# Troubleshooting and FAQ

Start with the server log: every component logs with a prefix (`[GitLab]`, `[Jira]`, `[Review]`,
`[ReReview]`, `[Settings]`, `[Store]`). The **Test connection** buttons in Settings check GitLab,
Jira and Claude Code with the saved settings.

## Connection problems

**"Could not connect to GitLab" / no projects in the sidebar**
- Check `GITLAB_URL` (base address only, no `/api/v4`) and that the token has the `api` scope and has
  not expired.
- Click **Test connection** in **Settings → GitLab**.
- With `GITLAB_PROJECTS` set, check the IDs/paths; with it empty, you only see projects you are a
  member of.

**Projects are visible but hidden in the sidebar**
Projects without open MRs are hidden. Click **Show projects without MRs** or search with `Ctrl+K`.

**Open MR counts or the project list are stale**
They are cached for 5 minutes. Use a refresh button.

**Jira test fails**
- Cloud: `JIRA_EMAIL` must be your Atlassian e-mail and `JIRA_API_TOKEN` an API token (not your
  password).
- Server / Data Center: leave `JIRA_EMAIL` empty and use a Personal Access Token.
- `HTTP 401/403` means wrong credentials or missing permissions to browse the project.

**The review ignores the Jira task**
- The summary says *No linked Jira task found*: put the issue key in the MR title, branch name or
  description.
- Keys are case sensitive (`SHOP-123`). Set `JIRA_PROJECT_KEYS` if other text such as `UTF-8` is
  detected instead.
- An invalid `JIRA_ISSUE_KEY_PATTERN` disables Jira; check the log.

## Review problems

**"Review already running"**
Only one review per MR runs at a time. Wait for it to finish; the sidebar shows it under
*In progress*.

**The review is slow**
- `llm` engine: increase `REVIEW_CONCURRENCY` for cloud models, lower `REVIEW_MAX_FILES`.
- `claude-code` engine: the first review of a project clones the whole repository. Lower
  `CLAUDE_CODE_MAX_TURNS` or use a faster `CLAUDE_CODE_MODEL`.

**"Review of this file failed" / files listed as not reviewed**
The model returned an error or invalid JSON for that file. Common causes: rate limits, a too small
context window, a model that does not follow JSON instructions. Use a stronger model, lower the
`REVIEW_*_MAX_CHARS` limits, or lower `REVIEW_CONCURRENCY`.

**Local model gives poor or empty reviews**
Ollama's default context window truncates the prompt. Keep `OLLAMA_NUM_CTX` at 32768 or more, use a
coder model (e.g. `qwen2.5-coder:32b`), and lower the `REVIEW_*_MAX_CHARS` limits for small models.

**The agent fails to start (`claude-code`)**
- `Cannot run "claude"`: install Claude Code or set `CLAUDE_CODE_BIN` to its full path. The app must
  see the same `PATH` as your shell.
- Not logged in: run `claude` once in a terminal and log in, or set
  `CLAUDE_CODE_USE_API_KEY=true` with `ANTHROPIC_API_KEY`.
- In Docker the engine is not available; use `llm` or run the app locally.
- Timeouts: raise `REVIEW_AGENT_TIMEOUT_MIN`.

**Too many or too trivial findings**
Set `REVIEW_MIN_SEVERITY=medium` to post only medium and higher in GitLab (the panel still shows
all). Add generated or vendored paths to `REVIEW_IGNORE`.

**A finding refers to old code**
The review only comments on lines added by the MR; findings far from the changes are dropped. If
you still see one, it was probably placed on the nearest changed line. Delete the draft in GitLab
before submitting.

## GitLab publishing

**I don't see the comments in GitLab**
In `draft` mode they are in **your pending review** and visible only to the token owner. Open the MR
and look for *Submit review* / pending comments. If the token belongs to another user, the drafts
are in their pending review.

**Some findings are only in the summary note**
GitLab refuses inline comments on positions it cannot map (e.g. some renamed or very large files);
those findings are moved to *Findings that could not be attached to a line*.

**Re-running the review duplicated comments**
In `draft` mode previous AI drafts are deleted before each run. In `comment` mode published comments
stay; a re-review checks unresolved AI threads, resolves fixed ones and does not repeat open ones.

**"Publishing to GitLab failed"**
The review is still saved in the panel. Check the token scope (`api`) and that you have at least
Developer access to the project.

## Settings and data

**A change in `.env` has no effect**
A value saved in the UI overrides `.env`. In Settings the source shows `UI`; click
**restore from .env**.

**Changed `PORT` but the app still listens on the old port**
`PORT` needs a restart.

**Where are my reviews stored? Can I reset everything?**
In the SQLite file at `DB_PATH` (default `data/codereview.db`). Stop the app and delete the file to
reset UI settings and review history.

**The interface is in Polish**
Click the `PL` button in the sidebar status bar, or set `UI_LANGUAGE=en`. See [Languages](languages.md).

## FAQ

**Does the app approve or merge MRs?** No. It only adds comments; approving, requesting changes
and merging stay with you.

**Can it review GitHub or Bitbucket?** Not yet; only GitLab is supported. GitHub support is
planned, see the [roadmap](../ROADMAP.md#-github-support).

**Can reviews start automatically when an MR is opened or updated?** Not yet; reviews are started
from the panel or the API. Webhooks and a CI job are planned, see the
[roadmap](../ROADMAP.md#-automatic-review-flow).

**Does it work with self-hosted GitLab and Jira?** Yes; set `GITLAB_URL` and `JIRA_URL` to your
instances.

**Which engine should I use?** `claude-code` for the best quality when you have Claude Code;
`llm` with a local model when code must stay on your machine; `llm` with a cloud model as a simple
middle ground.
