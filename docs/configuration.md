# Configuration reference

## Where settings come from

Every setting is an environment variable. The effective value is taken from the first source that
has it:

1. **UI** – values saved in **Settings**, stored in the SQLite database,
2. **`.env` / environment** – the `.env` file in the working directory or real environment variables,
3. **default** – the built-in default.

- Saving a value in the UI overrides `.env`. **restore from .env** / **restore default** removes the
  override.
- Changes apply immediately: GitLab, Jira, the AI provider and the review settings are rebuilt.
  Reviews in progress finish with the settings they started with. Only `PORT` needs a restart.
- `DB_PATH` can only be set in the environment, because the app needs it to find the database.
- Secrets are never sent to the browser; an empty secret field keeps the current value.

Start from [`env.example`](../env.example).

## GitLab

| Variable | Default | Description |
|---|---|---|
| `GITLAB_URL` | `https://gitlab.com` | Base address of your GitLab |
| `GITLAB_TOKEN` | – | Personal access token with the `api` scope. In draft mode comments are created as the token owner |
| `GITLAB_PROJECTS` | all your projects | Comma separated project IDs or paths shown in the panel |
| `GITLAB_PROJECTS_LIMIT` | `100` | Max. number of projects when `GITLAB_PROJECTS` is empty |
| `GITLAB_PROJECT_ID` | – | Default project for the legacy `/api/v1/merge-requests` routes |

## Jira

| Variable | Default | Description |
|---|---|---|
| `JIRA_URL` | – | Jira address; empty disables the integration |
| `JIRA_EMAIL` | – | Atlassian e-mail for Jira Cloud; empty for Server/Data Center |
| `JIRA_API_TOKEN` | – | API token (Cloud) or Personal Access Token (Server/DC) |
| `JIRA_PROJECT_KEYS` | – | Limit key detection to these projects, e.g. `SHOP,API` |
| `JIRA_ISSUE_KEY_PATTERN` | – | Custom key regex; takes precedence over `JIRA_PROJECT_KEYS` |
| `JIRA_MAX_COMMENTS` | `20` | Latest comments read per issue |
| `JIRA_MAX_CHARS` | `20000` | Max. characters of task context |

## Review

| Variable | Default | Description |
|---|---|---|
| `REVIEW_ENGINE` | `llm` | `llm` (prompt per file to `AI_MODEL`) or `claude-code` (agent on a cloned repository) |
| `REVIEW_LANGUAGE` | `pl` | Language of findings and GitLab comments: `pl`, `en`, `de` (see [Languages](languages.md)) |
| `REVIEW_POST_TO_GITLAB` | `true` | Publish results to GitLab |
| `REVIEW_GITLAB_MODE` | `draft` | `draft` (your pending review) or `comment` (visible immediately) |
| `REVIEW_MIN_SEVERITY` | `low` | Lowest severity posted to GitLab: `low`, `medium`, `high`, `critical` |
| `REVIEW_MAX_FILES` | `30` | Max. files reviewed per MR |
| `REVIEW_IGNORE` | lock files, `vendor/*`, `node_modules/*`, `*.min.js`, `*.min.css`, `*.map`, images, fonts, PDF | Comma separated glob patterns of files to skip |

Patterns in `REVIEW_IGNORE` match the file name (`*.lock`) or the full path (`config/*.yml`).
`dir/*` matches the directory at any depth (`vendor/*` matches `app/vendor/x.php`).

## Claude Code agent

| Variable | Default | Description |
|---|---|---|
| `CLAUDE_CODE_MODEL` | Claude Code default | `opus`, `sonnet` or a full model id |
| `CLAUDE_CODE_MAX_TURNS` | `60` | Max. agent steps |
| `REVIEW_AGENT_TIMEOUT_MIN` | `20` | Timeout of one review, minutes |
| `CLAUDE_CODE_USE_API_KEY` | `false` | `false`: claude.ai login (subscription); `true`: `ANTHROPIC_API_KEY` |
| `ANTHROPIC_API_KEY` | – | Needed only with `CLAUDE_CODE_USE_API_KEY=true` |
| `CLAUDE_CODE_BIN` | `claude` | Path to the Claude Code CLI |
| `REVIEW_WORKDIR` | system cache dir | Repository clones and temporary worktrees |

## LLM models

| Variable | Default | Description |
|---|---|---|
| `AI_MODEL` | `openai` | `openai`, `claude`, `kimi`, `ollama`, `mlx`, `opencode` |
| `OPENAI_API_KEY` | – | OpenAI key |
| `CLAUDE_API_KEY` | – | Anthropic key for the `llm` engine |
| `KIMI_API_KEY` | – | Moonshot AI key |
| `OPENCODE_API_KEY` | – | OpenCode server password |
| `OLLAMA_MODEL` | `llama2` | Ollama model, e.g. `qwen2.5-coder:32b` |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Ollama address |
| `OLLAMA_NUM_CTX` | `32768` | Ollama context window in tokens |
| `MLX_MODEL` | `mlx-community/Qwen2.5-Coder-32B-Instruct-4bit` | MLX model |
| `MLX_BASE_URL` | `http://localhost:8081/v1` | MLX OpenAI-compatible endpoint |
| `MLX_API_KEY` | `mlx-local` | MLX server key |

## Context and limits (`llm` engine)

| Variable | Default | Description |
|---|---|---|
| `REVIEW_CONCURRENCY` | `2` | Parallel model requests; keep low for local models |
| `REVIEW_DOCS_PATHS` | `README.md, README.rst, README, CLAUDE.md, AGENTS.md, CONTRIBUTING.md, ARCHITECTURE.md, CONVENTIONS.md, .cursorrules, .github/copilot-instructions.md, docs, doc` | Files and directories read from the MR branch as project documentation |
| `REVIEW_DOCS_MAX_CHARS` | `12000` | Budget for documentation |
| `REVIEW_FILE_MAX_CHARS` | `15000` | Max. full-file content per file |
| `REVIEW_DIFF_MAX_CHARS` | `12000` | Max. diff per file |
| `REVIEW_TREE_MAX_PATHS` | `300` | Max. repository paths listed as project structure |

## Server

| Variable | Default | Description |
|---|---|---|
| `UI_LANGUAGE` | `pl` | Default interface language: `pl`, `en` |
| `PORT` | `8080` | HTTP port (restart required) |
| `DB_PATH` | `data/codereview.db` | SQLite database; environment only |

## The database

The SQLite file at `DB_PATH` holds settings saved in the UI (including tokens entered there) and all
review results. It is created with `0600` permissions; `data/` is in `.gitignore`. Back it up like
any other file (stop the app or copy it together with the `-wal` file). Deleting it resets the UI
settings to `.env` values and removes the review history.
