# HealthOps

**Open-source infrastructure monitoring with a modern UI, AI-powered incident analysis, and MySQL deep monitoring.**

HealthOps is a single-binary Go backend + React frontend that monitors your servers, APIs, databases, and services — then alerts you when things go wrong and uses AI to help you fix them.

![Dashboard](docs/screenshots/dashboard.png)

## Why HealthOps?

- **Zero-config start** — Run one command, get a full monitoring dashboard at `localhost:8080`
- **7 check types** — HTTP APIs, TCP ports, processes, commands, logs, MySQL databases, SSH remote servers
- **AI incident analysis** — Bring your own key (OpenAI, Anthropic, Google, Ollama) to auto-analyze incidents
- **Beautiful UI** — React + Tailwind dashboard with real-time SSE updates, charts, and dark mode
- **Single binary** — No external dependencies required. Optional MongoDB mirror and MySQL monitoring
- **62+ API endpoints** — Full REST API for automation and integration
- **Production-ready** — Auth, audit logging, Prometheus metrics, retention cleanup, encrypted AI keys

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
cd backend && FRONTEND_DIR=../frontend/dist go run ./cmd/healthmon
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

### Incident Management

- Auto-created incidents from configurable alert rules
- Full lifecycle: **open → acknowledge → resolve**
- Evidence snapshots captured at incident creation
- MTTA/MTTR analytics

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
│  Auth · Validation · Audit · Prometheus · Alert Rules│
└──────────────────────────────────────────────────────┘
```

## Project Layout

```
healthops/
├── backend/                  # Go backend service
│   ├── cmd/healthmon/        # Main entrypoint
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
| `MYSQL_DSN` | — | MySQL DSN for mysql checks |
| `AUTH_ENABLED` | `false` | Enable HTTP Basic Auth |
| `AUTH_USERNAME` | `admin` | Auth username |
| `AUTH_PASSWORD` | — | Auth password |

### Check Configuration

Checks are defined in `backend/config/default.json`. Each check supports:

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

**Incidents:** list, detail, acknowledge, resolve, evidence snapshots

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

- HTTP Basic Auth for mutating endpoints (optional, enable in config)
- Command checks disabled by default (`allowCommandChecks=false`)
- AI API keys AES-256-GCM encrypted at rest
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
- controlled migration from fragmented scripts/tools into one service

Not ideal if you need:
- multi-tenant SaaS features out of the box
- turnkey managed alerting integrations without customization
- full APM traces/profiling (this is health monitoring, not full observability tracing)
