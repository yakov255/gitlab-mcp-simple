# GitLab MCP Server

MCP server для GitLab на Go (библиотека [mcp-golang](https://github.com/metoro-io/mcp-golang)). Позволяет AI-агенту диагностировать упавшие CI/CD пайплайны в merge request'ах.

## Tools

### `get_mr_failed_jobs`

Получает список упавших джоб из последнего пайплайна merge request'а.

| Parameter | Type | Description |
|-----------|------|-------------|
| `project_id` | string | GitLab project path (`raketa/raketa`) или numeric ID (`335`) |
| `mr_iid` | integer | Merge Request IID (например `35063`) |

**Пример ответа:**
```
MR: Draft: #27237 Обновление пакетов в Core
Branch: 27237-core-update
Pipeline #2250943 — failed
Pipeline URL: https://raketa.dev/raketa/raketa/-/pipelines/2250943

Failed jobs (6):

  [12463873] test:phpstan-core-8.2 (test)
       Status: failed
       Reason: script_failure
       URL: https://raketa.dev/raketa/raketa/-/jobs/12463873
  ...
Use get_job_log with a job_id to see the logs.
```

### `get_job_log`

Получает часть лога указанной джобы с поддержкой пагинации.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `project_id` | string | — | GitLab project path или numeric ID |
| `job_id` | integer | — | Job ID |
| `tail` | boolean | `false` | Вернуть последние N строк (N=limit). Полезно чтобы увидеть конец лога, не зная его длины |
| `offset` | integer | `0` | С какой строки начать (0-based). Игнорируется при `tail=true` |
| `limit` | integer | `100` | Сколько строк вернуть (max `500`). При `tail=true` — сколько строк с конца |

**Пример ответа:**
```
Job 12463873 — showing lines 0-19 (filtered) of 907 raw lines
Total filtered lines: 898

2026-05-22T12:39:46.014246Z 00O Running with gitlab-runner 18.11.3
2026-05-22T12:39:46.014259Z 00O   on node-13.ci.raketa.online-docker ...
...
```

## Usage

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxx",
        "GITLAB_BASE_URL": "https://gitlab.example.com"
      }
    }
  }
}
```

`GITLAB_BASE_URL` опционален, по умолчанию `https://gitlab.com`.

## Workflow для AI-агента

1. **`get_mr_failed_jobs`** — узнать какие джобы упали и их `job_id`
2. **`get_job_log(tail=true, limit=100)`** — прочитать последние строки лога, где обычно видна ошибка
3. **`get_job_log(offset=0, limit=200)`** — прочитать начало лога если нужно
4. **`get_job_log(offset=200, limit=200)`** — продолжить если нужно

Так агент контролирует потребление контекста и не перегружает модель огромными логами.

## Build

```bash
go build -o gitlab-mcp-server .
```
