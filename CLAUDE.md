# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Medics Health Check is a monitoring system for app servers, APIs, processes, logs, and short-term health trends. The project is built with a Go backend and a planned frontend UI.

**Current architecture:** Go service (`backend/`) handles health checks, alerting, and JSON APIs. Frontend (`frontend/`) is reserved for the future monitoring UI.

## Common Commands

All Go commands must be run from the `backend/` directory:

```bash
# Run the monitoring service (API + scheduler)
cd backend && go run ./cmd/healthops

# Run tests
cd backend && go test ./...

# Run specific test
cd backend && go test ./internal/monitoring -run TestDashboard

# Format code before committing
cd backend && go fmt ./...
```

## Architecture

### Backend Structure (`backend/`)

- **`cmd/healthops/main.go`** - Service entrypoint. Loads config, initializes stores, repositories, AI service, and starts the HTTP server and check scheduler.
- **`internal/monitoring/`** - Core monitoring package:
  - `config.go` - Config loading, validation, defaults.
  - `types.go` - Core types: `Config`, `CheckConfig`, `CheckResult`, `State`, `Summary`, `Store`/`Mirror` interfaces, `Incident`, `AIAnalysisResult`.
  - `store.go` - `FileStore` implementation (local JSON state file with atomic writes).
  - `hybrid_store.go` - `HybridStore` wrapping `FileStore` with optional MongoDB mirror.
  - `mongo.go` - `MongoMirror` implementation.
  - `runner.go` - `Runner` executes checks in parallel. Supports: `api`, `tcp`, `process`, `command`, `log`, `mysql`.
  - `service.go` - `Service` HTTP handlers and scheduler. Wires MySQL API handler and AI API handler.
  - `incident_manager.go` - Incident lifecycle (create, acknowledge, resolve). Fires `OnIncidentCreated` callback for AI enqueue.
  - `incident_repository.go` - In-memory incident repository.
  - `alert_rules.go` - Alert rule engine with configurable thresholds, cooldowns, and consecutive-breach logic.
  - `scheduler.go` - `CheckScheduler` for periodic check execution.
  - `auth.go` - HTTP Basic Auth middleware.
  - `audit.go` - `AuditLogger` with actor extraction.
  - `metrics.go` - Prometheus metrics collector.
  - `validation.go` - Input validation helpers.
  - `dashboard.go` - Dashboard snapshot read model.

#### MySQL Monitoring (`mysql_*.go`)
  - `mysql_models.go` - `MySQLSample`, `MySQLDelta`, snapshot types.
  - `mysql_collector.go` - `LiveMySQLSampler` collects `SHOW GLOBAL STATUS/VARIABLES`, computes deltas.
  - `mysql_repository.go` - `FileMySQLRepository` with JSONL-backed sample/delta storage.
  - `mysql_rules.go` - `MySQLRuleEngine` with 9 default alert rules (connection utilization, slow queries, locks, etc.).
  - `mysql_incident_evidence.go` - `IncidentSnapshotRepository` for evidence collection.
  - `mysql_api.go` - `MySQLAPIHandler` for samples, deltas, health card, time-series, AI queue, notifications.
  - `mysql_analytics.go` - Analytics extensions (uptime, response times, failure rate, incident MTTA/MTTR).

#### BYOK AI Layer (`ai_*.go`)
  - `ai_config.go` - `AIConfigStore` with AES-256-GCM encrypted API keys, provider/prompt config, safe masked views.
  - `ai_provider.go` - `AIProvider` interface + implementations: OpenAI, Anthropic, Google Gemini, Ollama, Custom (OpenAI-compatible).
  - `ai_service.go` - `AIService` orchestrator: background worker, queue processing, prompt template rendering, retry with fallback.
  - `ai_api.go` - `AIAPIHandler` for BYOK config, provider CRUD, prompt CRUD, on-demand analysis, health check, results.
  - `ai_queue.go` - `FileAIQueue` with dedup, claim, complete/fail lifecycle.

#### Supporting Infrastructure
  - `notification_outbox.go` - `FileNotificationOutbox` for alert delivery tracking.
  - `retention_jobs.go` - `RetentionJob` with daily pruning for snapshots, notifications, AI queue.
  - `analytics.go` - General analytics (uptime, response times, status timeline).
  - `frontend_api.go` - Frontend-optimized endpoints (SSE events, config, stats, auth, exports).
  - `api_types.go` - `APIResponse` envelope, pagination, error formatting.

### Storage Architecture

The service uses a hybrid storage approach:
1. **Local file store** (`backend/data/state.json`) - Always works, primary source of truth
2. **MongoDB mirror** - Optional best-effort sync via `MONGODB_URI` env var

Read operations (especially dashboard endpoints) prefer Mongo if the data is fresh and non-stale. Writes always update the local file first, then attempt to sync to Mongo asynchronously with a 5-second timeout. If Mongo is down, the service continues operating normally.

### Check Types

Defined in `backend/config/default.json`:

- **`api`** - HTTP endpoint checks. Validates status code, optional response body substring, latency threshold
- **`tcp`** - Port connectivity checks. Can check if port is open and measure latency
- **`process`** - Process existence checks via `ps` command. Matches by keyword in command line
- **`command`** - Custom shell command execution. Validates exit code and optional output contains
- **`log`** - Log file freshness checks. Validates file modification time is within threshold
- **`mysql`** - MySQL health checks via DSN. Collects `SHOW GLOBAL STATUS/VARIABLES`, computes deltas, triggers alert rules

Each check can be disabled via `"enabled": false`, grouped by `server`/`application` tags, and has configurable timeout and warning thresholds.

### API Endpoints (62 total)

See `backend/docs/api-reference.md` for the full reference with request/response examples.

**Core:** `/healthz`, `/readyz`, checks CRUD, runs, summary, results, dashboard  
**Incidents:** list, get, acknowledge, resolve, snapshots  
**MySQL:** samples, deltas, health card, time-series  
**Analytics:** uptime, response times, status timeline, failure rate, incident MTTA/MTTR  
**Alert Rules:** CRUD for configurable alert rules  
**BYOK AI:** config, provider CRUD, prompt CRUD, on-demand analysis, provider health, results  
**Infrastructure:** audit log, notifications, AI queue, SSE events, config, stats, auth, exports, Prometheus metrics

### Configuration

**Config file** (`backend/config/default.json`) — *seed only on first run*; once `data/state.json` exists, it is the single source of truth and config edits are ignored:
```json
{
  "server": {
    "addr": ":8080",
    "readTimeoutSeconds": 10,
    "writeTimeoutSeconds": 10,
    "idleTimeoutSeconds": 60
  },
  "retentionDays": 7,
  "checkIntervalSeconds": 60,
  "workers": 8,
  "checks": [...]
}
```

**Environment variables:**
- `CONFIG_PATH` - Override config file location
- `STATE_PATH` - Override local state file location
- `DATA_DIR` - Override data directory for JSONL stores
- `MONGODB_URI` - Enable MongoDB mirroring (optional)
- `MONGODB_DATABASE` - Mongo database name (default: `healthops`)
- `MONGODB_COLLECTION_PREFIX` - Mongo collection prefix (default: `healthops`)
- `{check.mysql.dsnEnv}` - MySQL DSN per check (never logged)

The `backend/config/default.json` provides the initial seed checks for the very first run. After that, all checks (including MySQL, SSH, etc.) are managed exclusively via the API (`/api/v1/checks`) or UI and persisted in `data/state.json` (with optional MongoDB mirror). To add a check at runtime, `POST /api/v1/checks` — do not edit `default.json` (the change will be ignored on restart).

## Development Guidelines

### Code Organization
- Place new backend code in `backend/internal/monitoring/` or new `internal/` packages as appropriate
- Keep frontend code in `frontend/` (reserved for future UI)

### Testing
- Tests are located next to the code they cover (e.g., `service_test.go`, `ai_service_test.go`)
- The `fakeStore` mock in `service_test.go` shows the pattern for Store interface mocking
- Run `go test ./...` before committing changes
- Test coverage includes: unit, contract, E2E, race condition (`-race`), and security tests
- For API verification, start the service and hit `/healthz`, `/api/v1/summary`, and `GET /api/v1/checks`

### Adding New Check Types
To add a new check type:
1. Add the type string to the validate switch in `config.go`
2. Add a `run{Type}` method in `runner.go` that executes the check and populates `result.Metrics`
3. Add a case for the new type in `executeCheck` in `runner.go`
4. (Optional) Add an example check to `backend/config/default.json` for first-run seeding only — runtime checks should be created via the API
5. If the check type needs custom alert rules, add them in a `{type}_rules.go` file

### BYOK AI Integration
- AI config is stored in `data/ai_config.json` with API keys AES-256-GCM encrypted
- Encryption key is auto-generated at `data/.ai_enc_key` on first run
- `IncidentManager.OnIncidentCreated` callback auto-enqueues AI analysis when enabled
- Background worker polls the AI queue every 5s, claims items, and processes via configured provider
- Prompt templates use Go `text/template` with incident/evidence context variables

### Important Patterns

**Immutability:** The store uses a clone-on-write pattern. `Update()` accepts a mutator function that receives a copy of the state, and the change is applied atomically via a temp file + rename operation.

**State pruning:** Old results are automatically pruned based on `retentionDays` config. The `pruneResults` function in `store.go` handles this.

**Dashboard read model:** Dashboard endpoints return a `DashboardSnapshot` which includes both raw state and a pre-computed summary. This is optimized for UI rendering.
