# Integrations

## GitLab

GitLab.com and self-hosted GitLab are supported. Set `GITLAB_URL` to the base address of your
instance (e.g. `https://gitlab.example.com`).

### Personal access token

1. In GitLab open **Preferences → Access tokens** (or **User settings → Access tokens**).
2. Create a token with the **`api`** scope and an expiry date that suits you.
3. Put it in `GITLAB_TOKEN` (`.env`) or in **Settings → GitLab → GitLab token**.
4. Click **Test connection**; it shows `Logged in as <name>`.

The token must be **yours** if you use `REVIEW_GITLAB_MODE=draft`: draft comments are created in the
pending review of the token owner, so with someone else's token they would land in their review.
For `comment` mode a dedicated bot account works well, so comments are clearly authored by the bot.

### Which projects are shown

- `GITLAB_PROJECTS` – comma separated project IDs or paths, e.g. `group/api, group/front, 123456`.
- Empty – all projects you are a member of, up to `GITLAB_PROJECTS_LIMIT` (default 100).
- `GITLAB_PROJECT_ID` – optional default project used only by the legacy
  `/api/v1/merge-requests` routes.

The project list and open MR counts are cached for 5 minutes. The refresh buttons bypass the cache.

## Jira

Jira is optional and **read-only**: the app only reads issues to use them as review context. It
never writes to Jira.

| Jira type | `JIRA_URL` | `JIRA_EMAIL` | `JIRA_API_TOKEN` |
|---|---|---|---|
| Cloud (`*.atlassian.net`) | `https://company.atlassian.net` | your Atlassian e-mail | API token from <https://id.atlassian.com/manage-profile/security/api-tokens> |
| Server / Data Center | your Jira address | empty | Personal Access Token (Bearer) |

Leaving `JIRA_URL` or `JIRA_API_TOKEN` empty disables the integration.

### Issue key detection

Keys are searched in the MR title, source branch and description, and, only if none is found
there, in the commit messages.

- By default any `ABC-123`-like key matches. This can give false matches such as `UTF-8`.
- `JIRA_PROJECT_KEYS=SHOP,API` limits detection to these projects (recommended).
- `JIRA_ISSUE_KEY_PATTERN` – your own regular expression, e.g. `\b(?:KM|SHOP)-\d+\b`. It takes
  precedence over `JIRA_PROJECT_KEYS`. An invalid expression disables Jira (see the log).

### What is read

For each issue: key, summary, type, status, description, parent/epic and the latest
`JIRA_MAX_COMMENTS` comments (oldest first, so later decisions win). The total is limited to
`JIRA_MAX_CHARS` characters. Comments matter: a change of requirements agreed in a comment is taken
into account.

## AI providers (`llm` engine)

Choose the provider with `AI_MODEL`.

| `AI_MODEL` | Settings | Model used |
|---|---|---|
| `openai` | `OPENAI_API_KEY` | `gpt-3.5-turbo` (fixed in code) |
| `claude` | `CLAUDE_API_KEY` | `claude-sonnet-4-5` (fixed in code) |
| `kimi` | `KIMI_API_KEY` (Moonshot AI) | `moonshot-v1-32k` |
| `ollama` | `OLLAMA_BASE_URL` (default `http://localhost:11434`), `OLLAMA_MODEL`, `OLLAMA_NUM_CTX` | `OLLAMA_MODEL` (default `llama2`) |
| `mlx` | `MLX_BASE_URL` (default `http://localhost:8081/v1`), `MLX_MODEL`, `MLX_API_KEY` | any model served by an OpenAI-compatible MLX server |
| `opencode` | `OPENCODE_API_KEY` | an OpenCode server on `http://localhost:4096` |

If the selected provider cannot be initialised (e.g. a missing key), the app logs a warning and falls
back to OpenAI.

### Local models (Ollama, MLX)

- A review prompt with full context takes roughly **10–20k tokens**. Ollama's default context window
  is much smaller and silently truncates the prompt, so keep `OLLAMA_NUM_CTX` at 32768 or more.
- Use a code model, e.g. `qwen2.5-coder:32b` in Ollama or
  `mlx-community/Qwen2.5-Coder-32B-Instruct-4bit` in MLX.
- With small models lower `REVIEW_DIFF_MAX_CHARS`, `REVIEW_FILE_MAX_CHARS` and
  `REVIEW_DOCS_MAX_CHARS`, and keep `REVIEW_CONCURRENCY` at 1–2.
- In Docker, reach Ollama on the host with `OLLAMA_BASE_URL=http://host.docker.internal:11434`.

Example MLX server:

```bash
pip install mlx-lm
mlx_lm.server --model mlx-community/Qwen2.5-Coder-32B-Instruct-4bit --port 8081
```

## Claude Code agent (`claude-code` engine)

1. Install Claude Code and log in: see <https://docs.claude.com/en/docs/claude-code>. Check that
   `claude --version` works in the same shell that starts the app.
2. Set `REVIEW_ENGINE=claude-code`.
3. Optional settings:
   - `CLAUDE_CODE_MODEL` – `opus`, `sonnet` or a full model id; empty = Claude Code default,
   - `CLAUDE_CODE_MAX_TURNS` (default 60) and `REVIEW_AGENT_TIMEOUT_MIN` (default 20),
   - `CLAUDE_CODE_BIN` if `claude` is not in `PATH`,
   - `REVIEW_WORKDIR` – where repository clones and worktrees live (default: the system cache
     directory, e.g. `~/Library/Caches/codereviewAI` on macOS),
   - `CLAUDE_CODE_USE_API_KEY=true` and `ANTHROPIC_API_KEY` to pay per token instead of using your
     claude.ai subscription.
4. In **Settings → Claude Code agent** click **Test connection**; it shows the CLI version and the
   billing mode.

The repository is cloned over HTTPS with your GitLab token. The first review of a project clones the
whole repository; later reviews only fetch.
