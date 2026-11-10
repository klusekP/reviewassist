# REST API

All endpoints are under `/api/v1` and return JSON. Errors have the form `{"error": "message"}`.
Messages are localized by the `Accept-Language` header (`pl`, `en`), falling back to `UI_LANGUAGE`.

> The API has no authentication. Do not expose it to untrusted networks; see [Security](security.md).

## Health and configuration

```http
GET /api/v1/health
```
`{"status": "OK", "message": "Code Review AI is running correctly"}`

```http
GET /api/v1/config
```
Non-secret settings used by the panel: `default_project_id`, `gitlab_url`, `engine`, `model`,
`language`, `ui_language`, `ui_languages`, `post_to_gitlab`, `gitlab_mode`, `jira_enabled`,
`db_path`.

## Projects and merge requests

```http
GET /api/v1/projects[?refresh=1]
```
Projects with open MR counts. Cached for 5 minutes; `refresh=1` bypasses the cache.

```http
GET /api/v1/projects/{pid}/merge-requests?state=opened|merged|closed
```
`{"project_id": pid, "merge_requests": [...], "count": n, "running": {"<iid>": {...}}}` – `running` lists reviews in
progress in this project.

```http
GET /api/v1/projects/{pid}/merge-requests/{iid}
```
`{"merge_request": {...}, "diffs": [...], "diff_count": n, "running": {...}|null}`

```http
POST /api/v1/projects/{pid}/merge-requests/{iid}/review
```
Starts a review in the background and returns `202 Accepted` immediately with `status: "started"`,
`project_id`, `mr_id`, `mr_title`, `engine` and `total_files`.
Returns `409 Conflict` when a review of this MR is already running. The result is delivered over the
WebSocket and stored in the database.

```bash
curl -X POST -H 'Accept-Language: en' http://localhost:8080/api/v1/projects/42/merge-requests/7/review
```

### Legacy routes (default project `GITLAB_PROJECT_ID`)

```http
GET  /api/v1/merge-requests?state=opened
GET  /api/v1/merge-requests/{iid}
POST /api/v1/merge-requests/{iid}/review
```

## Stored reviews

```http
GET    /api/v1/reviews[?project_id=&limit=500]                  # latest review of each MR
GET    /api/v1/projects/{pid}/merge-requests/{iid}/reviews      # history, newest first
DELETE /api/v1/projects/{pid}/merge-requests/{iid}/reviews      # {"deleted": n}
POST   /api/v1/reviews/import                                   # move results kept in the browser by old versions
```

A review record contains `id`, `project_id`, `iid`, `sha`, `title`, `verdict`, `score`, `engine`,
`created_at` and `review`:

```json
{
  "summary": "…",
  "verdict": "approve_with_comments",
  "score": 7,
  "issues": [
    {
      "type": "security",
      "severity": "high",
      "description": "…",
      "file": "src/Controller/OrderController.php",
      "line": 42,
      "code": "…",
      "fix": "…"
    }
  ],
  "requirements": [{"requirement": "…", "status": "done", "evidence": "…"}],
  "security_summary": "…",
  "quality_summary": "…",
  "suggestions": ["…"],
  "followup": {"source": "gitlab", "since_sha": "…", "fixed": 1, "open": 0, "accepted": 0, "new": 2, "items": []}
}
```

## Settings

```http
GET  /api/v1/settings
```
`{"groups": [...], "settings": [...], "db_path": "…"}`. Labels and descriptions are localized;
secret values are masked (`••••abcd`).

```http
PUT  /api/v1/settings
Content-Type: application/json

{"values": {"REVIEW_LANGUAGE": "en", "REVIEW_MAX_FILES": "20"}, "reset": ["REVIEW_MIN_SEVERITY"]}
```
Validates, saves and applies the values. `reset` removes UI overrides. An empty secret keeps the
current value. The response repeats `GET` plus `restart_required` (keys that need a restart).

```http
POST /api/v1/settings/test/{gitlab|jira|claude}
```
`{"ok": true, "message": "Logged in as …"}` – checks the saved settings.

## WebSocket

```
GET /api/v1/ws
```

The panel listens for review events:

| `type` | Fields | Meaning |
|---|---|---|
| `review_part` | `project_id`, `mr_id`, `file`, `file_index`, `total_files`, `review` | One file reviewed (`llm` engine) |
| `review` | `project_id`, `mr_id`, `review` | Review finished |
| `review_error` | `project_id`, `mr_id`, `error` | Review failed |
