# Security and privacy

## Deployment model

Code Review AI is designed as a **personal tool running on your own machine** (or on a server
reachable only by you or your team):

- The web panel and the REST API have **no authentication**. Anyone who can reach the port can read
  your projects and merge requests through your GitLab token, start reviews and change settings.
- Run it on `localhost` or behind a VPN / reverse proxy with authentication. Do not expose it to the
  internet.
- Stop the app when you are not using it if it runs on a shared machine.

## Where your data goes

| Data | Sent to |
|---|---|
| MR diffs, changed files, project documentation and structure | The AI provider in `AI_MODEL` (`llm` engine) or Anthropic through Claude Code (`claude-code` engine) |
| Jira task description and comments | Same AI provider, as review context |
| Findings and summaries | GitLab (when `REVIEW_POST_TO_GITLAB=true`) and the local database |
| Tokens and API keys | Only to the service they belong to |

If your code must not leave your network, use a local model (`ollama` or `mlx`) and keep Jira and
GitLab on-premises. Check your AI provider's data retention and training policy before reviewing
private code with a cloud model.

## Secrets

- Tokens can be set in `.env` / environment variables or in **Settings**. Values from Settings are
  stored **in plain text** in the SQLite database (`DB_PATH`), created with `0600` permissions.
- The API never returns secrets; the panel shows only the last four characters.
- `.env` and `data/` are listed in `.gitignore`. Never commit them.
- The GitLab token is passed to `git` as a one-off HTTP header, so it is not written to
  `.git/config`, clone URLs or logs.
- Use a GitLab token with an expiry date. `api` scope is required to publish comments; if you only
  want to read, set `REVIEW_POST_TO_GITLAB=false` (the token still needs read access to the API).

## The Claude Code agent

The code under review is **untrusted input**: an MR can contain text crafted to manipulate an AI
agent (prompt injection). The `claude-code` engine limits what the agent can do:

- only read-only tools: `Read`, `Grep`, `Glob` and `git diff/log/show/blame/status`,
- no file editing, no other shell commands, no web access,
- GitLab and Jira tokens, `OPENAI_API_KEY`, `CLAUDE_API_KEY`, `KIMI_API_KEY`, `OPENCODE_API_KEY`
  and `MLX_API_KEY` are removed from its environment, and `ANTHROPIC_API_KEY` too unless
  `CLAUDE_CODE_USE_API_KEY=true`,
- it works in a temporary worktree that is deleted after the review,
- the application, not the agent, publishes the comments.

Even so, a manipulated review could contain misleading findings. That is one reason the default
publishing mode is `draft`: a human checks every comment before it is visible to others.

## AI output

AI reviews can miss real problems and report false ones. Treat them as a second pair of eyes, not as
an approval gate. The verdict is never milder than the reported findings justify, but the absence
of findings does not prove the code is correct or secure.

## Reporting a vulnerability

Please report security issues privately to the maintainer instead of opening a public issue.
