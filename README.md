# Code Review AI

AI code review for GitLab merge requests. It reads the linked Jira task and your project's
documentation, comments on the changed lines, checks security, code quality and task requirements,
and publishes the findings to GitLab as a **draft review** that you check and submit yourself.

## Features

- 🔍 **Context-aware review** – the Jira task (description and comments), project docs
  (`README`, `CLAUDE.md`, `docs/`…), repository structure and full content of the changed files.
- 🎯 **Scoped to the MR** – comments only on added lines; pre-existing problems are out of scope.
- 🛡️ **Mandatory checklists** – security (access control, IDOR, injection, data exposure…), code
  volume / refactoring / performance, and task requirements (done / partial / missing).
- ✅ **Clear verdict** – approve, approve with comments, or request changes, plus a 1–10 score.
- 💬 **GitLab draft review** – inline comments and a summary note in your pending review, or
  regular comments posted immediately.
- 🔁 **Re-reviews** – previous findings are checked: fixed and explained ones are resolved, open ones
  are not repeated.
- 🤖 **Two engines** – one prompt per file to OpenAI, Claude, Kimi, Ollama, MLX or OpenCode, or the
  [Claude Code](https://docs.claude.com/en/docs/claude-code) agent exploring a checkout of the MR
  in read-only mode.
- 🖥️ **Web panel** – projects tree, MR lists, diff with inline AI comments, review history, settings
  editable in the UI (stored in SQLite), dark and light theme.
- 🌍 **Languages** – interface in Polish or English, review comments in Polish, English or German.

## Quick start

```bash
git clone <repository-url> codereviewAI
cd codereviewAI
cp env.example .env     # set GITLAB_URL, GITLAB_TOKEN, AI_MODEL and its API key
go run .                # requires Go 1.25+
# or
docker compose up -d --build
```

Open <http://localhost:8080>, choose a project and a merge request, and click **Run review**.
Then open the MR in GitLab, check the draft comments and click **Submit review**.

## Documentation

Full user documentation is in [`docs/`](docs/README.md):

- [Getting started](docs/getting-started.md)
- [Web interface](docs/user-guide.md)
- [How a review works](docs/how-reviews-work.md)
- [Integrations: GitLab, Jira, AI providers, Claude Code](docs/integrations.md)
- [Configuration reference](docs/configuration.md)
- [Languages](docs/languages.md)
- [REST API](docs/api.md)
- [Security and privacy](docs/security.md)
- [Troubleshooting and FAQ](docs/troubleshooting.md)

## Project structure

```
codereviewAI/
├── main.go                    # HTTP server and routes
├── services/
│   ├── review.go              # review orchestration and MR endpoints
│   ├── agent.go, repo.go      # Claude Code engine: clone, worktree, headless agent
│   ├── ai.go, ai_prompts.go   # llm engine: provider selection and prompts
│   ├── review_guidelines.go   # checklists, severity guide, verdict
│   ├── followup.go            # re-review of previous findings
│   ├── publish.go             # GitLab inline comments and summary note
│   ├── gitlab*.go, jira.go    # integrations
│   ├── settings.go, store.go  # settings and SQLite storage
│   ├── runtime.go             # settings API, live reload
│   ├── i18n/                  # backend translations
│   └── ai/                    # AI provider clients
├── static/                    # web panel (Alpine.js): index.html, js/app.js, js/i18n.js, css/
├── docs/                      # user documentation
└── env.example                # configuration template
```

## Development

```bash
go test ./...                      # unit tests
go test -tags agent ./services/    # integration test with the real Claude Code CLI
go vet ./...
```

Contributions are welcome. Please run the tests and keep both languages complete when changing
texts (see [Languages → Adding a translation](docs/languages.md#adding-a-translation)).

## License

[MIT](LICENSE) © 2026 Piotr Stachowski
