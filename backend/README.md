# Backend

Go backend for the HealthOps monitoring console. Provides health checks, MySQL monitoring, incident management, alert rules, BYOK AI-powered analysis, and a comprehensive REST API (62 endpoints).

## Run

```bash
cd backend
go run ./cmd/healthmon
```

## Test

```bash
cd backend
go test ./...           # all tests
go test ./... -race      # with race detector
go fmt ./...             # format before committing
```

## Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `CONFIG_PATH` | `config/default.json` | JSON config file |
| `STATE_PATH` | `data/state.json` | Persisted state file |
| `DATA_DIR` | `data/` | JSONL repositories, AI config |
| `MONGODB_URI` | — | Optional MongoDB mirror |
| `MONGODB_DATABASE` | `healthmon` | MongoDB database name |
| `MONGODB_COLLECTION_PREFIX` | `healthmon` | Collection prefix |
| `{check.mysql.dsnEnv}` | — | MySQL DSN per check (never logged) |

MongoDB is best-effort only. The backend keeps running with the local file store if MongoDB is unavailable.

## Key Features

- **Health Checks**: `api`, `tcp`, `process`, `command`, `log`, `mysql` check types
- **MySQL Monitoring**: Collects `SHOW GLOBAL STATUS/VARIABLES`, computes deltas, 9 default alert rules
- **Incidents**: Auto-created from alert rules, acknowledge/resolve lifecycle, evidence snapshots
- **Alert Rules**: Configurable thresholds, cooldowns, consecutive breaches, per-check or global
- **BYOK AI Analysis**: Configure OpenAI/Anthropic/Google/Ollama/Custom providers from the UI. API keys AES-256-GCM encrypted at rest. Auto-analyzes incidents with configurable prompt templates.
- **Analytics**: Uptime, response times, failure rates, incident MTTA/MTTR
- **Export**: CSV/JSON export for MySQL samples, incidents, and results
- **Observability**: Prometheus metrics, audit logging, SSE live events

## API

Full reference: [`docs/api-reference.md`](docs/api-reference.md) (62 endpoints)

**Core**: `/healthz`, `/readyz`, checks CRUD, runs, summary, results, dashboard  
**Incidents**: list, get, acknowledge, resolve, snapshots  
**MySQL**: samples, deltas, health card, time-series  
**BYOK AI**: config, providers, prompts, analyze, health, results  
**More**: alert rules, analytics, audit, notifications, SSE, config, stats, exports, `/metrics`
