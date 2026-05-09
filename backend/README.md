# Backend

Go backend for the HealthOps monitoring console. Provides health checks, MySQL monitoring, incident management, alert rules, BYOK AI-powered analysis, and a comprehensive REST API (62 endpoints).

## Run

```bash
cd backend
MONGODB_URI=mongodb://localhost:27017 \
MONGODB_DATABASE=healthops \
MONGODB_COLLECTION_PREFIX=healthops \
HEALTHOPS_JWT_SECRET='change-me-at-least-32-characters' \
HEALTHOPS_AI_ENCRYPTION_KEY='change-me-random-ai-secret' \
HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD='change-me-admin-password' \
go run ./cmd/healthops
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
| `MONGODB_URI` | — | **Required.** MongoDB connection string for authoritative storage |
| `MONGODB_DATABASE` | `healthops` | **Required.** MongoDB database name |
| `MONGODB_COLLECTION_PREFIX` | `healthops` | **Required.** Collection prefix |
| `HEALTHOPS_JWT_SECRET` | — | **Required.** JWT signing secret; use at least 32 random characters |
| `HEALTHOPS_AI_ENCRYPTION_KEY` | — | **Required.** Deployment-managed secret for encrypting AI provider keys |
| `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` | — | **Required on first start.** Bootstraps or rotates the admin password |
| `{check.mysql.dsnEnv}` | — | MySQL DSN per check (never logged) |

MongoDB is the required authoritative storage backend for production and Compose deployments. Legacy file-store variables such as `STATE_PATH` and `DATA_DIR`, plus direct `state.json`/JSONL operational workflows, are obsolete and should not be used for deployment runbooks.

## Key Features

- **Health Checks**: `api`, `tcp`, `process`, `command`, `log`, `mysql`, `ssh` check types
- **Server Management**: Add remote servers, SSH-based health checks for process/command/connectivity
- **MySQL Monitoring**: Collects `SHOW GLOBAL STATUS/VARIABLES`, computes deltas, 9 default alert rules
- **Incidents**: Auto-created from alert rules, acknowledge/resolve lifecycle, evidence snapshots
- **Alert Rules**: 5 default rules out of the box. Configurable thresholds, cooldowns, consecutive breaches, per-check or global. Persisted in MongoDB.
- **JWT Authentication**: Token-based auth with admin/viewer roles. Default credentials: `admin` / `admin`
- **User Management**: Create, update, delete users with role-based access control
- **Notification Channels**: 6 channel types (email, Slack, Discord, Telegram, webhooks, PagerDuty) with smart filters (severity, check IDs, check types, servers, tags). Professional HTML email templates with incident stats and dashboard links. Incident-level deduplication prevents alert storms.
- **BYOK AI Analysis**: Configure OpenAI/Anthropic/Google/Ollama/Custom providers from the UI. API keys AES-256-GCM encrypted at rest. Auto-analyzes incidents with configurable prompt templates.
- **Analytics**: Uptime, response times, failure rates, incident MTTA/MTTR
- **Export**: CSV/JSON export for MySQL samples, incidents, and results
- **Observability**: Prometheus metrics, audit logging, SSE live events

## API

Full reference: [`docs/api-reference.md`](docs/api-reference.md) (62 endpoints)

**Core**: `/healthz`, `/readyz`, checks CRUD, runs, summary, results, dashboard  
**Auth & Users**: login, user CRUD, role management  
**Incidents**: list, get, acknowledge, resolve, snapshots  
**Notifications**: channel CRUD, toggle, test, smart filters  
**MySQL**: samples, deltas, health card, time-series  
**BYOK AI**: config, providers, prompts, analyze, health, results  
**More**: alert rules, analytics, audit, SSE, config, stats, exports, `/metrics`
