# Code Review AI – User documentation

Code Review AI reviews GitLab merge requests with an AI model. It reads the linked Jira task and the
project's documentation, comments on the changed lines, and publishes the findings to GitLab as a
draft review that you check and submit yourself.

## Contents

| Document | What you will find |
|---|---|
| [Getting started](getting-started.md) | Requirements, installation (local and Docker), first run, first review |
| [Web interface](user-guide.md) | Every screen of the panel: sidebar, dashboard, MR list, MR details, settings |
| [How a review works](how-reviews-work.md) | Review flow, engines, scope rules, verdicts, severities, re-reviews, publishing to GitLab |
| [Integrations](integrations.md) | GitLab token, Jira (Cloud and Server), AI providers (OpenAI, Claude, Kimi, Ollama, MLX, OpenCode), Claude Code agent |
| [Configuration reference](configuration.md) | Every setting with its default value, and how `.env`, the UI and the database interact |
| [Languages](languages.md) | Interface language and review language, adding a new translation |
| [REST API](api.md) | HTTP endpoints and the WebSocket used by the panel |
| [Security and privacy](security.md) | What data goes where, how secrets are stored, agent sandboxing, deployment advice |
| [Troubleshooting and FAQ](troubleshooting.md) | Common problems and their fixes |

## In one minute

```bash
git clone <repository-url> codereviewAI && cd codereviewAI
cp env.example .env          # set at least GITLAB_URL and GITLAB_TOKEN
go run .                     # or: docker compose up -d
```

Open <http://localhost:8080>, pick a project in the sidebar, open a merge request and click
**Run review**. Then open the MR in GitLab, check the draft comments and click **Submit review**.

## License

The project is released under the [MIT License](../LICENSE).
