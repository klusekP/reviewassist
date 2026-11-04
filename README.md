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

## API Endpoints

### Health Check
```http
GET /api/v1/health
```

### List Merge Requests
```http
GET /api/v1/merge-requests?state=opened
```

### Merge Request Details
```http
GET /api/v1/merge-requests/{id}
```

### Automatic review (local)
```http
POST /api/v1/merge-requests/{id}/review
```

### List projects
```http
GET /api/v1/projects
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