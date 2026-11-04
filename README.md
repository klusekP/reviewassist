# Simple Code Review AI

AI application for automatic code review of GitLab Merge Requests. The application analyzes code changes using OpenAI GPT-4 and displays review results in a web interface.

## Features

- 🔍 **Automatic code analysis** - AI analyzes changes in Merge Requests
- 🤖 **Intelligent comments** - Displays detailed comments for specific code lines
- 🛡️ **Security** - Checks for potential security issues
- ⚡ **Performance** - Analyzes code optimization
- 📝 **Best Practices** - Checks compliance with good practices
- 📊 **Quality assessment** - Generates code quality score (1-10)
- 💾 **Local storage** - Review results are saved locally in the browser
- 🔒 **Privacy** - Comments are not automatically sent to GitLab

## Requirements

- Go 1.25+
- GitLab Personal Access Token
- AI API Key (OpenAI, Claude, KIMI) or local server (Ollama/MLX)

## Installation

1. **Clone the repository:**
```bash
git clone <repository-url>
cd codereviewAI
```

2. **Install dependencies:**
```bash
go mod tidy
```

3. **Configure environment variables:**
```bash
cp env.example .env
```

Edit the `.env` file and add your API keys:
```env
GITLAB_TOKEN=your_gitlab_personal_access_token
GITLAB_URL=https://gitlab.com
GITLAB_PROJECT_ID=123456
OPENAI_API_KEY=your_openai_api_key
CLAUDE_API_KEY=your_claude_api_key
KIMI_API_KEY=your_kimi_api_key
AI_MODEL=openai  # Options: openai, claude, kimi, ollama, mlx
MLX_API_KEY=mlx-local
MLX_MODEL=mlx-community/Qwen2.5-Coder-32B-Instruct-4bit
MLX_BASE_URL=http://localhost:8081/v1
PORT=8080
```

4. **Run the application:**
```bash
go run main.go
```

## GitLab Configuration

1. **Create Personal Access Token:**
   - Go to GitLab → Settings → Access Tokens
   - Create a token with permissions: `api`, `read_user`, `read_repository`
   - Copy the token to the `GITLAB_TOKEN` variable

2. **Find Project ID:**
   - Go to your project in GitLab
   - Project ID is located on the project's main page
   - Add to the `GITLAB_PROJECT_ID` variable

## AI Model Configuration

The application supports multiple AI providers. Set the `AI_MODEL` environment variable to choose your provider:

### OpenAI Configuration
1. **Create OpenAI account:**
   - Go to [OpenAI](https://platform.openai.com/)
   - Create an account and add funds
   - Generate API Key
   - Add to the `OPENAI_API_KEY` variable
   - Set `AI_MODEL=openai`

### Claude Configuration
1. **Create Anthropic account:**
   - Go to [Anthropic](https://console.anthropic.com/)
   - Create an account and add funds
   - Generate API Key
   - Add to the `CLAUDE_API_KEY` variable
   - Set `AI_MODEL=claude`

### KIMI Configuration
1. **Create KIMI (Moonshot AI) account:**
   - Go to [Moonshot AI](https://platform.moonshot.cn/)
   - Create an account
   - Generate API Key
   - Add to the `KIMI_API_KEY` variable
   - Set `AI_MODEL=kimi`
   - Model used: `moonshot-v1-32k` (KIMI k2.5 Free)

### Ollama Configuration
1. **Install Ollama locally:**
   - Follow instructions at [Ollama](https://ollama.ai/)
   - Set `OLLAMA_MODEL` to your preferred model
   - Set `AI_MODEL=ollama`

### MLX Server Configuration
1. **Run local MLX server with OpenAI-compatible API:**
   - Set `AI_MODEL=mlx`
   - Optional auth token: `MLX_API_KEY` (default: `mlx-local`)
   - Set model in `MLX_MODEL`
   - Set endpoint in `MLX_BASE_URL` (default: `http://localhost:8081/v1`)

## Context-aware review (GitLab + Jira)

When you click **Review** on an MR (or call `POST /api/v1/merge-requests/{id}/review`), the app:

1. **Reads the task from Jira**: it finds issue keys (e.g. `SHOP-123`) in the MR title, source branch and description, and fetches the summary, description, status, parent/epic and the latest comments (read-only).
2. **Reads project context from the MR branch**: documentation (`REVIEW_DOCS_PATHS`: README, CLAUDE.md, AGENTS.md, docs/…), repository structure and the full content of each changed file.
3. **Reviews each changed file**, looking only at the added lines (`+`). The model gets the diff with line numbers, so comments land on the right lines.
4. **Reviews the whole MR against the task**: missing requirements, out-of-scope changes, missing tests or migrations.
5. **Publishes the findings to GitLab** in `REVIEW_LANGUAGE` (default: Polish):
   - inline comments on the changed lines, plus one summary note,
   - `REVIEW_GITLAB_MODE=draft` (default): the comments are **draft notes** in your pending review, visible only to you. Open the MR in GitLab, edit or delete them, then click **Submit review** and optionally choose *Request changes* or *Approve*,
   - re-running the review replaces the previous AI drafts and leaves your own drafts untouched,
   - findings that can't be anchored to a diff line go into the summary note.

### Review engines

| `REVIEW_ENGINE` | How it works | Best for |
|---|---|---|
| `llm` (default) | The app builds the context itself (docs, full file, numbered diff) and sends one prompt per file plus one MR-level prompt to `AI_MODEL`. | Local or cheap models (Ollama, MLX) |
| `claude-code` | The app clones the GitLab repo, fetches the MR source and target branches, and checks out the MR head in a temporary `git worktree`. Then [Claude Code](https://docs.claude.com/en/docs/claude-code) runs headless (`claude -p`) in that directory. It reads CLAUDE.md and the docs itself, and explores callers, tests and related code. | The best review quality |

The `claude-code` engine:
- needs `git` and `claude` in `PATH`. It runs locally, not in the Docker image.
- restricts the agent to read-only tools (`Read`, `Grep`, `Glob`, `git diff/log/show/blame`). It can't edit files, run other commands, or access the web. The MR code is untrusted input.
- hides the GitLab and Jira tokens and the other API keys from the agent. The app itself publishes the comments.
- uses your claude.ai login (subscription) by default, and also hides `ANTHROPIC_API_KEY` from the agent. Set `CLAUDE_CODE_USE_API_KEY=true` to pay per token with the API key instead.
- keeps the bare clone in `REVIEW_WORKDIR`. The first run clones the whole repo; later runs only fetch. The worktree is deleted after each review.

### Jira configuration

| Jira type | Variables |
|---|---|
| Cloud (`*.atlassian.net`) | `JIRA_URL`, `JIRA_EMAIL`, `JIRA_API_TOKEN` ([create token](https://id.atlassian.com/manage-profile/security/api-tokens)) |
| Server / Data Center | `JIRA_URL`, `JIRA_API_TOKEN` (Personal Access Token), `JIRA_EMAIL` empty |

Set `JIRA_PROJECT_KEYS=SHOP,API` to avoid false matches such as `UTF-8`. Without Jira, the review still works and uses the MR title and description as the task.

### GitLab token

The token needs the `api` scope. In `draft` mode the comments are created as the token owner, so use **your own** personal access token if you want them to appear in your pending review.

### Local models

Review prompts with full context take roughly 10–20k tokens. For Ollama, `OLLAMA_NUM_CTX` (default 32768) sets the context window; Ollama's default is much smaller and would silently truncate the prompt. With small models, lower `REVIEW_*_MAX_CHARS`.

## Web interface

Open `http://localhost:$PORT`. The layout is inspired by [Cezar](https://github.com/open-mercato/cezar):

- **Left sidebar**: a tree of your GitLab projects with the number of open MRs. Click a project to see its MR list; expand it (chevron) to see its MRs directly in the tree. Projects without open MRs are hidden behind "Pokaż projekty bez MR". The sidebar also shows reviews in progress and recent reviews. Search with `Ctrl+K`.
- **Project view**: a table of MRs (open / merged / closed) with the AI review status, branches, author, score and finding counts.
- **MR view**: the *Review* tab shows the verdict, summary, task requirements, security, quality and findings; click a finding to jump to the line in *Changes*. The *Changes* tab shows a unified diff with AI comments inline. The *Opis* tab shows the MR description and merge status. Action buttons: run or re-run the review, open the MR in GitLab, clear the local result.
- Dark and light themes. Results are kept in browser `localStorage`. The server tracks running reviews, so the "analiza…" status survives a page reload.

Which projects appear: `GITLAB_PROJECTS` (IDs or paths), or all projects the token user is a member of (`GITLAB_PROJECTS_LIMIT`). Open MRs are counted with one group-level request per top-level group, and the result is cached for 5 minutes (the refresh button bypasses the cache).

## Settings and database (SQLite)

All settings from `.env` can be changed in the web interface (**gear icon → Ustawienia**), grouped by GitLab, Jira, Review, Claude Code agent, LLM models, context limits and server.

- Values saved in the UI are stored in SQLite (`DB_PATH`, default `data/codereview.db`) and **override `.env`**. Each field shows where its value comes from (`UI`, `.env` or default). "Przywróć z .env" removes the UI override.
- Changes apply immediately without a restart: GitLab, Jira, the AI provider and the review settings are rebuilt, and reviews in progress keep running. Only `PORT` needs a restart.
- Tokens are never sent back to the browser. The UI shows only the last 4 characters; leave the field empty to keep the current token.
- The GitLab, Jira and Claude Code groups have a **Testuj połączenie** button that checks the saved settings.
- The same database stores **review results and their history**. They no longer live in the browser's `localStorage`, so they are visible from any computer. Results kept in the browser by older versions are moved into the database automatically on first launch.
- The database file holds tokens entered in the UI. It is created with `0600` permissions and `data/` is in `.gitignore`. In Docker it is mounted as a volume (`./data`).

`DB_PATH` is the only setting that can't be changed in the UI, because the app needs it to find the database.

## API Endpoints

### Health Check
```http
GET /api/v1/health
```

### UI configuration
```http
GET /api/v1/config
```

### Projects (with open MR counts)
```http
GET /api/v1/projects[?refresh=1]
```

### Merge requests of a project
```http
GET  /api/v1/projects/{pid}/merge-requests?state=opened|merged|closed
GET  /api/v1/projects/{pid}/merge-requests/{iid}
POST /api/v1/projects/{pid}/merge-requests/{iid}/review
```
The legacy routes below use the default project (`GITLAB_PROJECT_ID`).

### List Merge Requests
```http
GET /api/v1/merge-requests?state=opened
```

### Merge Request Details
```http
GET /api/v1/merge-requests/{id}
```

### Settings
```http
GET  /api/v1/settings                      # secrets masked
PUT  /api/v1/settings                      # {"values": {"KEY": "value"}, "reset": ["KEY"]}
POST /api/v1/settings/test/{gitlab|jira|claude}
```

### Stored reviews
```http
GET    /api/v1/reviews[?project_id=]                         # latest review per MR
GET    /api/v1/projects/{pid}/merge-requests/{iid}/reviews   # history
DELETE /api/v1/projects/{pid}/merge-requests/{iid}/reviews
POST   /api/v1/reviews/import                                # move browser results into the DB
```

### Automatic review (local)
```http
POST /api/v1/merge-requests/{id}/review
```


## Usage Example

1. **Get list of open Merge Requests:**
```bash
curl http://localhost:8080/api/v1/merge-requests
```

2. **Perform automatic review:**
```bash
curl -X POST http://localhost:8080/api/v1/merge-requests/123/review
```

## Project Structure

```
codereviewAI/
├── main.go              # Main application file
├── services/
│   ├── gitlab.go        # GitLab API service
│   ├── ai.go           # OpenAI AI service
│   └── review.go       # Review service
├── static/
│   └── index.html      # Web interface
├── go.mod              # Go dependencies
├── env.example         # Configuration example
└── README.md           # Documentation
```

## How it works

1. **Data retrieval:** Application connects to GitLab API and retrieves Merge Request with diff
2. **AI analysis:** Code is analyzed by OpenAI GPT-4 with change context
3. **Comment generation:** AI generates detailed comments for specific lines
4. **Result display:** Comments are displayed in web interface and saved locally

## Types of issues detected by AI

- 🐛 **Bugs** - Logical and syntax problems
- 🛡️ **Security** - SQL injection, XSS, etc.
- ⚡ **Performance** - Optimization issues
- 📝 **Style** - Coding convention compliance
- ✅ **Best Practices** - Good practice compliance

## Severity levels

- **Low** - Minor suggestions and improvements
- **Medium** - Issues worth fixing
- **High** - Important issues requiring attention
- **Critical** - Critical security or functionality issues

## Web interface features

- **MR browsing:** List of all open Merge Requests
- **Automatic review:** One-click AI analysis
- **Local storage:** Review results are saved in the browser
- **Statistics:** Number of reviewed MRs, average score, number of issues
- **Responsive design:** Works on mobile devices

## Development

### Adding new features

1. **New analysis types** - Add new categories in `services/ai.go`
2. **Additional endpoints** - Extend `services/review.go`
3. **Integrations** - Add support for other platforms (GitHub, Bitbucket)
4. **Database** - Add persistent storage for review results

### Testing

```bash
# Run tests
go test ./...

# Test with sample data
curl http://localhost:8080/api/v1/health
```

## License

MIT License

## Support

In case of problems:
1. Check application logs
2. Verify environment variable configuration
3. Check GitLab token permissions
4. Verify OpenAI API balance

## Roadmap

- [ ] GitHub support
- [ ] Review rules configuration
- [ ] Review history
- [ ] Code quality statistics