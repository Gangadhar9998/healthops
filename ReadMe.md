# HealthOps

**Open-source infrastructure monitoring with a modern UI, AI-powered incident analysis, and MySQL deep monitoring.**

HealthOps is a single-binary Go backend + React frontend that monitors your servers, APIs, databases, and services — then alerts you when things go wrong and uses AI to help you fix them.

## Demo

<div align="center">

https://github.com/user-attachments/assets/healthops-demo.mp4

*Dashboard → Health Checks → Incidents → Analytics → AI Analysis → MySQL Monitoring → Dark Mode*

</div>

<details>
<summary>📸 Screenshots</summary>

![Dashboard](docs/screenshots/dashboard.png)

</details>

## Why HealthOps?

- **Zero-config start** — Run one command, get a full monitoring dashboard at `localhost:8080`
- **7 check types** — HTTP APIs, TCP ports, processes, commands, logs, MySQL databases, SSH remote servers
- **AI incident analysis** — Bring your own key (OpenAI, Anthropic, Google, Ollama) to auto-analyze incidents
- **Beautiful UI** — React + Tailwind dashboard with real-time SSE updates, charts, and dark mode
- **Single binary** — No external dependencies required. Optional MongoDB mirror and MySQL monitoring
- **62+ API endpoints** — Full REST API for automation and integration
- **Production-ready** — JWT auth, user management, notification channels, audit logging, Prometheus metrics, retention cleanup, encrypted AI keys

## Screenshots

| Dashboard | Health Checks | Incidents |
|-----------|--------------|-----------|
| ![Dashboard](docs/screenshots/dashboard.png) | ![Checks](docs/screenshots/checks.png) | ![Incidents](docs/screenshots/incidents.png) |

| Servers | Analytics | Settings |
|---------|-----------|----------|
| ![Servers](docs/screenshots/servers.png) | ![Analytics](docs/screenshots/analytics.png) | ![Settings](docs/screenshots/settings.png) |

| AI Analysis | MySQL Monitoring |
|-------------|-----------------|
| ![AI Analysis](docs/screenshots/ai-analysis.png) | ![MySQL](docs/screenshots/mysql.png) |

## Quick Start

### Option 1: Run locally (Go + Node.js required)

```bash
# Build frontend
cd frontend && npm install && npm run build && cd ..

# Start the backend (serves frontend too)
cd backend && FRONTEND_DIR=../frontend/dist go run ./cmd/healthops
```

Open [http://localhost:8080](http://localhost:8080) — that's it.

### Option 2: Docker Compose (recommended)

```bash
docker compose up -d
```

This starts HealthOps + MongoDB. Open [http://localhost:8080](http://localhost:8080).

### Option 2b: Docker Compose with demo targets

Try HealthOps with realistic monitoring targets (nginx, MySQL, Redis, echo server) — all pre-configured:

```bash
docker compose -f docker-compose.yml -f docker-compose.demo.yml up -d
```

### Option 3: Docker only

```bash
docker build -t healthops .
docker run -p 8080:8080 healthops
```

### Verify it's running

```bash
curl http://localhost:8080/healthz
# {"success":true,"data":{"status":"ok"}}
```

## Features

### Health Check Types

| Type | What it monitors | Example |
|------|-----------------|---------|
| `api` | HTTP/HTTPS endpoints | REST APIs, health endpoints, websites |
| `tcp` | Port connectivity | Database ports, service ports |
| `process` | Running processes | nginx, node, postgres |
| `command` | Shell command output | Custom scripts, disk checks |
| `log` | Log file freshness | App logs, system logs |
| `mysql` | MySQL database health | Connections, queries, replication |
| `ssh` | Remote server checks | SSH-based remote monitoring |

Each check supports: custom intervals, retries, cooldowns, timeout, and warning thresholds.

### Authentication & User Management

- **JWT token-based authentication** — Login via `/api/v1/auth/login`, receive a signed JWT
- **User management** — Create, update, delete users with admin/viewer roles
- **Role-based access** — Admins can mutate; viewers are read-only
- **Default credentials** — `admin` / `admin` (change on first login)

### Notification Channels

Multi-channel alerting with smart filtering:
- **6 channel types** — Email, Slack, Discord, Telegram, Webhooks, PagerDuty
- **Smart filters** — Route alerts by severity, specific checks, check types, servers, and tags
- **Professional HTML emails** — Enterprise-grade email template with severity-coded headers, incident stats, and dashboard links (with plain text fallback)
- **Deduplication** — Incident-level dedup prevents duplicate notifications for the same incident + channel; cooldown per check
- **Test notifications** — Send test alerts from the UI before going live
- **Resolution alerts** — Optionally notify when incidents resolve

### Incident Management

- Auto-created incidents from configurable alert rules
- Full lifecycle: **open → acknowledge → resolve**
- Evidence snapshots captured at incident creation
- MTTA/MTTR analytics

### Alert Rules

- **5 default alert rules** out of the box (critical check failure, warning check failure, high failure rate, extended downtime, response time degradation)
- Configurable thresholds, cooldowns, and consecutive-breach logic
- Per-check or global scope
- File-persisted at `data/alert_rules.json`

### AI-Powered Analysis (BYOK)

Bring your own API key from any provider:
- **OpenAI** (GPT-4, GPT-3.5)
- **Anthropic** (Claude)
- **Google** (Gemini)
- **Ollama** (local models)
- **Custom** (any OpenAI-compatible API)

AI auto-analyzes new incidents with configurable prompt templates. API keys are AES-256-GCM encrypted at rest.

### MySQL Deep Monitoring

- Live `SHOW GLOBAL STATUS/VARIABLES` collection
- Delta computation for rate metrics
- 9 built-in alert rules (connection utilization, slow queries, lock waits, replication lag, etc.)
- Dedicated sub-pages: connections, queries, threads, server stats

### Server Management

- Add and manage remote servers
- SSH-based health checks (process, command, connectivity)
- Server grouping and tagging

### Analytics & Observability

- **Uptime tracking** per check with 7-day trends
- **Response time charts** with percentile breakdowns
- **Failure rate** analysis
- **Prometheus metrics** at `/metrics` (check runs, failures, durations, HTTP requests)
- **Audit logging** for all mutations
- **SSE real-time events** for live dashboard updates
- **CSV/JSON export** for incidents and results

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   React Frontend                     │
│        (Vite + TypeScript + Tailwind + Recharts)     │
└──────────────────────┬──────────────────────────────┘
                       │ REST API + SSE
┌──────────────────────┴──────────────────────────────┐
│                   Go Backend                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐ │
│  │Scheduler │ │ Runner   │ │ Incident │ │  AI    │ │
│  │(per-check│ │(7 types) │ │ Manager  │ │Service │ │
│  │ timers)  │ │          │ │          │ │(BYOK)  │ │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └───┬────┘ │
│       │             │            │            │      │
│  ┌────┴─────────────┴────────────┴────────────┴───┐  │
│  │              Hybrid Store                       │  │
│  │    (File-based primary + optional MongoDB)      │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  JWT Auth · Users · Notifications · Alert Rules      │
│  Audit · Prometheus · Validation · Deduplication     │
└──────────────────────────────────────────────────────┘
```

## Documentation

| Document | Description |
|----------|-------------|
| [Deployment Guide](docs/deployment-guide.md) | **Start here** — Complete setup for dev, Docker, and production. Covers ports, env vars, TLS/SSL, domains, notifications, SSH, MySQL, troubleshooting, and log debugging |
| [API Reference](backend/docs/api-reference.md) | Full reference for all 62+ REST endpoints |
| [Operational Runbook](docs/runbook.md) | Day-to-day operations, backup/restore, performance tuning |
| [Architecture Decisions](docs/) | ADRs for scope, persistence, auth, incidents |

## Project Layout

```
healthops/
├── backend/                  # Go backend service
│   ├── cmd/healthops/        # Main entrypoint
│   ├── cmd/loadtest/         # Load testing tool
│   ├── internal/monitoring/  # Core modules (50+ files)
│   ├── config/default.json   # Default check configuration
│   └── docs/                 # API reference, specs, runbook
├── frontend/                 # React + TypeScript + Vite
│   └── src/
│       ├── pages/            # 11 pages + 4 MySQL sub-pages
│       ├── components/       # Reusable UI components
│       ├── api/              # API client modules
│       └── hooks/            # SSE, export hooks
├── docker/                   # Docker configs & MySQL init
├── docs/                     # ADRs, runbook, plans
├── docker-compose.yml        # Full stack: backend + Mongo + MySQL
├── Dockerfile                # Multi-stage build (frontend + backend)
└── ReadMe.md                 # This file
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CONFIG_PATH` | `config/default.json` | Check configuration file |
| `STATE_PATH` | `data/state.json` | Persisted state file |
| `DATA_DIR` | `data/` | Data directory for JSONL repos |
| `FRONTEND_DIR` | — | Path to frontend dist folder |
| `MONGODB_URI` | — | Optional MongoDB connection |
| `MONGODB_DATABASE` | `healthops` | MongoDB database name |
| `MONGODB_COLLECTION_PREFIX` | `healthops` | MongoDB collection prefix |
| `CORS_ORIGIN` | — | Allowed CORS origin (for custom domains) |
| `MYSQL_DSN` / check `dsnEnv` | — | MySQL DSN for mysql checks |

See the [Deployment Guide](docs/deployment-guide.md) for full details on all configuration options, notification setup, SSH servers, TLS, and troubleshooting.

### Check Configuration

Checks are seeded from `backend/config/default.json` on the very first run, then managed via the API (`/api/v1/checks`) and persisted in `data/state.json`. Edits to `default.json` are ignored once state exists. Each check supports:

```json
{
  "id": "my-api",
  "name": "My API",
  "type": "api",
  "target": "https://api.example.com/healthz",
  "intervalSeconds": 30,
  "timeoutSeconds": 10,
  "retryCount": 2,
  "retryDelaySeconds": 5,
  "cooldownSeconds": 60,
  "warningThresholdMs": 500,
  "server": "prod-server-1",
  "tags": ["production", "critical"]
}
```

## API Overview

HealthOps exposes 62+ REST endpoints. Full reference: [`backend/docs/api-reference.md`](backend/docs/api-reference.md)

**Core:** `/healthz`, `/readyz`, checks CRUD, manual runs, summary, results, dashboard

**Auth & Users:** login, user CRUD, role management

**Incidents:** list, detail, acknowledge, resolve, evidence snapshots

**Notifications:** channel CRUD, toggle, test, smart filters

**MySQL:** samples, deltas, health card, time-series, AI questions

**AI (BYOK):** config, providers, prompts, analyze, health check, results queue

**More:** alert rules, analytics (uptime/response-times/failure-rate/incidents), audit log, SSE events, Prometheus metrics, CSV exports

## Testing

```bash
cd backend
go test ./...           # All tests
go test ./... -race     # With race detector
go fmt ./...            # Format check
```

Load testing:

```bash
cd backend
go run ./cmd/loadtest -scenario=query -duration=2m
```

## Security

- **JWT authentication** with role-based access control (admin/viewer)
- User management with secure password handling
- Command checks disabled by default (`allowCommandChecks=false`)
- AI API keys AES-256-GCM encrypted at rest
- Notification deduplication prevents alert storms
- Input validation on all API endpoints
- Secrets in env vars, never in config files

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.23, `net/http` stdlib |
| Frontend | React 19, TypeScript, Vite 6, Tailwind CSS |
| Charts | Recharts |
| State | TanStack React Query |
| Storage | File-based (primary) + MongoDB (optional mirror) |
| Monitoring | Prometheus client, SSE |
| Container | Docker multi-stage build |

## Contributing

1. Fork the repo
2. Create a feature branch
3. Run tests: `cd backend && go test ./...`
4. Submit a PR

## License

Open source. See repository for details.
