# How a review works

## The flow

When you click **Run review** (or call `POST /api/v1/projects/{pid}/merge-requests/{iid}/review`):

1. **Select files.** The MR diff is loaded from GitLab. Deleted files, binary files and files
   matching `REVIEW_IGNORE` (lock files, `vendor/`, `node_modules/`, minified files, images, fonts…)
   are skipped. At most `REVIEW_MAX_FILES` files are reviewed.
2. **Find the task.** Jira issue keys (e.g. `SHOP-123`) are searched in the MR title, source branch
   and description. Only when none is found there, commit messages are searched too (merge commits
   often mention unrelated issues). The summary, description, status, parent/epic and the latest
   comments of each issue become the *task context*. Without Jira, the MR title and description are
   used as the task.
3. **Load previous findings** for a [re-review](#re-reviews), if the MR was reviewed before.
4. **Review** with the configured [engine](#review-engines).
5. **Apply the scope rules** and compute the [final verdict](#verdict-and-score).
6. **Publish to GitLab** (when `REVIEW_POST_TO_GITLAB=true`), see [Publishing](#publishing-to-gitlab).
7. **Store** the result in the database and push it to the open browsers.

Only one review of the same MR can run at a time; starting a second one returns *Review already
running*.

## Review engines

Set with `REVIEW_ENGINE`.

### `llm` (default)

The application builds the context itself and sends prompts to the provider selected by `AI_MODEL`:

- **One prompt per changed file** containing: the numbered diff, the full new content of the file,
  the list of other changed files, the MR title and description, the task context, the project
  documentation (`REVIEW_DOCS_PATHS`) and the repository structure.
- **One MR-level prompt** that compares the whole change with the task: missing requirements,
  out-of-scope changes, missing tests or migrations, plus the security and quality summaries.

Files are reviewed in parallel (`REVIEW_CONCURRENCY`). Context sizes are limited by
`REVIEW_DIFF_MAX_CHARS`, `REVIEW_FILE_MAX_CHARS`, `REVIEW_DOCS_MAX_CHARS` and
`REVIEW_TREE_MAX_PATHS`. Best for local or cheap models.

### `claude-code`

The application clones the repository (a bare clone in `REVIEW_WORKDIR`, later only fetched),
checks out the MR head in a temporary `git worktree`, and runs [Claude Code](https://docs.claude.com/en/docs/claude-code)
headless in that directory. The agent reads `CLAUDE.md` and the documentation itself and explores
callers, tests and related code before writing the review. The worktree is deleted afterwards.

- Read-only tools only: `Read`, `Grep`, `Glob` and `git diff/log/show/blame/status`. No editing, no
  other shell commands, no web access.
- GitLab and Jira tokens and other API keys are removed from the agent's environment.
- Billing: your claude.ai login (subscription) by default; set `CLAUDE_CODE_USE_API_KEY=true` to pay
  per token with `ANTHROPIC_API_KEY`.
- Limits: `CLAUDE_CODE_MAX_TURNS` agent steps and `REVIEW_AGENT_TIMEOUT_MIN` minutes.
- Requires `git` and `claude` in `PATH`; not available in the Docker image.

Best review quality, slower and more expensive than a single prompt.

## What the model checks

Every review goes through three mandatory checklists:

- **Security** – access control and IDOR, injection, XSS, mass assignment and data exposure, input
  validation, secrets in code or logs, file/network issues (path traversal, uploads, SSRF, XXE,
  unsafe deserialization), authentication and sessions (CSRF, CORS, JWT, weak randomness), abuse and
  availability (unbounded queries, N+1, missing rate limits, ReDoS), data integrity (races,
  transactions, constraints), suspicious dependencies.
- **Code volume, refactoring and performance** – unnecessary code and over-engineering, duplication
  (including re-implementing what already exists in the project), simpler alternatives, long or
  mixed-responsibility functions, logic in the wrong layer, algorithmic complexity, wrong data
  structures, inefficient queries.
- **Task compliance** – whether the code does what the Jira task and its comments ask for.

The model is told to report real problems only and that an empty list of findings is a valid answer.

### Finding types

`bug`, `security`, `performance`, `requirement`, `best_practice`, `refactoring`, `style`.

### Severities

| Severity | Used for |
|---|---|
| **critical** | Exploitable without authentication or by any user with severe impact (data leak, account takeover, RCE, price manipulation), or data loss/corruption in normal use |
| **high** | Security issue exploitable by an authenticated user (IDOR, privilege escalation, data exposure), a bug breaking the main flow, or a task requirement that is not implemented |
| **medium** | Edge-case bug, missing validation or error handling, missing tests for new business logic, clear violation of project conventions, noticeable performance problem, duplicated existing logic |
| **low** | Readability, naming, simplification and refactoring suggestions, minor performance improvements, defence in depth |

Only findings at or above `REVIEW_MIN_SEVERITY` are posted to GitLab; the panel always shows all of
them.

## Scope rules

The review is about **this MR's changes**, not the whole codebase:

- A finding must point to an **added line** (`+`) of the diff. Line numbers that are slightly off
  are snapped to the nearest changed line.
- Findings about unchanged code far from any change, or about files the MR did not touch, are
  dropped.
- General findings (no file, or a file without a line) stay as MR-level findings.

## Verdict and score

The review ends with one of three verdicts:

| Verdict | When |
|---|---|
| ✅ **approve** | Nothing worth changing |
| 💬 **approve with comments** | Only medium/low findings, or a requirement only partially done |
| ⛔ **request changes** | Any critical or high finding, or a missing requirement |

The application recomputes the verdict from the findings and requirements and never makes it milder
than the model's own verdict: a model that says *approve* while reporting a high finding is
overruled. The **score** (1–10) is the model's overall assessment.

## Task requirements

When a task is linked, the MR-level review returns a checklist of the task's requirements, each
marked **done**, **partial**, **missing** or **unclear** with evidence (where in the code, or why it
is missing). The summary contains a line such as *Requirements: 3/5 done, 1 partial, 1 missing*.

## Re-reviews

When you run a review again after new commits, the previous findings are checked instead of being
repeated:

- **Source of previous findings:** the unresolved AI threads in GitLab (when publishing is enabled
  and threads exist), otherwise the findings of the last stored review of an older commit. Running
  the review again on the same commit is a plain re-run.
- Each previous finding gets a status:
  - **fixed** – the problem is no longer in the code (also when the code was removed),
  - **open** – still present, possibly moved,
  - **accepted** – still present, but the author replied in the GitLab thread with a valid
    technical reason. A reply without a valid reason is not enough.
- New findings that repeat a still-open one are dropped, so the same comment is not posted twice.
- In GitLab, threads of fixed and accepted findings get a reply (*Fixed in `abc1234`* /
  *Explanation accepted*) and are resolved. Open ones are listed in the summary note.

## Publishing to GitLab

| `REVIEW_GITLAB_MODE` | Effect |
|---|---|
| `draft` (default) | Comments are added to **your pending review** (as the token owner), visible only to you until you click **Submit review** in GitLab. Before each run the previous AI drafts are deleted; your own drafts stay. |
| `comment` | Comments are posted immediately as regular discussions, visible to everyone. |

What is published:

- **Inline comments** on the changed lines: severity icon, severity, type, description and the
  suggested fix.
- **One summary note** with: re-review status, linked Jira task(s), verdict, summary and score,
  requirements table, security and quality assessments, MR-level findings, findings that GitLab
  refused to attach to a line, suggestions, files that could not be reviewed, and a footer saying
  the comments were generated by AI.

The panel shows a status line in the summary, e.g.
*GitLab (draft): 7 inline comments + summary, 2 threads from the previous review resolved. Open the
MR in GitLab → "Submit review" to publish them.*

Set `REVIEW_POST_TO_GITLAB=false` to keep results only in the panel.

## Where results are stored

Every finished review is saved in SQLite (`DB_PATH`, default `data/codereview.db`) with the commit
SHA, verdict, score and engine. The history of all reviews of an MR is kept and visible in the panel.
Deleting results in the panel removes them from the database only.
