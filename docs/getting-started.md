# Getting started

This guide takes you from a fresh clone to your first published review.

## 1. Requirements

| What | Why | Notes |
|---|---|---|
| Go 1.25+ | Build and run the server | Not needed with Docker |
| GitLab account and personal access token | Read MRs, publish comments | Scope `api`. See [Integrations → GitLab](integrations.md#gitlab) |
| An AI model | Write the review | A cloud API key (OpenAI, Anthropic, Kimi), a local server (Ollama, MLX), or the Claude Code CLI |
| Jira (optional) | Task description used as review context | Cloud or Server / Data Center, read-only |
| `git` and `claude` in `PATH` (optional) | Only for the `claude-code` review engine | Runs locally, not in the Docker image |

## 2. Install

### Option A: run locally

```bash
git clone <repository-url> codereviewAI
cd codereviewAI
go mod download
cp env.example .env
```

Edit `.env` and set at least:

```env
GITLAB_URL=https://gitlab.com          # or your self-hosted GitLab
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxx
AI_MODEL=openai                        # openai | claude | kimi | ollama | mlx | opencode
OPENAI_API_KEY=sk-...                  # the key for the provider you chose
```

Start the server:

```bash
go run .
# or build a binary
go build -o codereviewAI . && ./codereviewAI
```

The log shows the database path and the address of the web interface:

```
INFO Database: data/codereview.db
INFO Web interface available at: http://localhost:8080
```

> You can also start with an almost empty `.env`. If `GITLAB_TOKEN` is missing, the app still starts
> and asks you to configure it in **Settings**.

### Option B: Docker Compose

```bash
cp env.example .env       # fill in the values
docker compose up -d --build
```

- The panel is available on `http://localhost:${PORT:-8080}`.
- Settings changed in the UI and review results are kept in `./data` (mounted volume).
- Ollama running on the host is reachable as `http://host.docker.internal:11434` (already set in
  `docker-compose.yml`).
- The image does not contain `git` or the `claude` CLI, so use the `llm` engine in Docker. To use the
  `claude-code` engine, run the app directly on your machine.

## 3. Check the configuration

1. Open <http://localhost:8080>.
2. Click the gear icon (**Settings**) in the sidebar.
3. In the **GitLab** group click **Test connection**. You should see `Logged in as <your name>`.
4. If you use Jira, test the **Jira** group the same way. For the Claude Code engine, test the
   **Claude Code agent** group: it prints the CLI version and the billing mode.

Values you change in Settings are saved in the database and take effect immediately, without a
restart (except `PORT`). See [Configuration reference](configuration.md).

## 4. Run your first review

1. In the sidebar, click a project. Projects without open merge requests are hidden; use
   **Show projects without MRs** to see them all.
2. Click a merge request in the list.
3. Click **Run review**. A progress bar shows the analysis; you can leave the page, the review keeps
   running on the server and a notification appears when it is done.
4. Read the result on the **Review** tab: verdict, score, summary, task requirements, security and
   quality assessment, and the findings grouped by file. Click a finding to jump to its line on the
   **Changes** tab.

## 5. Publish in GitLab

With the default settings (`REVIEW_POST_TO_GITLAB=true`, `REVIEW_GITLAB_MODE=draft`) the findings
are added to **your pending review** in GitLab and are visible only to you:

1. Open the MR in GitLab (the **GitLab** button on the MR page).
2. Go through the draft comments. Edit or delete the ones you disagree with.
3. Click **Submit review** and optionally choose *Approve* or *Request changes*.

Running the review again replaces the previous AI drafts and leaves your own drafts untouched.
To post comments immediately for everyone, set `REVIEW_GITLAB_MODE=comment`.

## 6. Next steps

- Link Jira so the review checks the task requirements: [Integrations → Jira](integrations.md#jira).
- Pick the review engine that fits you: [How a review works → Engines](how-reviews-work.md#review-engines).
- Switch the interface or comment language: [Languages](languages.md).
