# HealthOps

HealthOps is a backend-first monitoring and incident platform for infrastructure and application health checks.

It is designed for teams that need a practical internal reliability tool: fast to run, easy to configure, and stable under continuous checks.

## What This Project Actually Is

HealthOps is a Go service that:
- stores and schedules health checks
- executes checks on a recurring interval
- exposes health and monitoring APIs
- evaluates alert rules
- exposes Prometheus metrics
- supports audit logging for mutating actions

This is not just a ping tool. It is an operational control-plane for service checks, health summaries, and incident-oriented monitoring workflows.

## What Is Great About It

1. Operationally simple
- Single backend service with file-first persistence
- Optional MongoDB mirror, but local persistence remains the source of resilience
- Easy local and Docker startup

2. Built for production behavior, not demo behavior
- Per-check scheduling (`interval`, `retry`, `retry delay`, `cooldown`)
- Input validation and API envelope consistency
- Auth gating for mutating APIs
- Scheduler, API, and metrics paths are test-covered

3. Good reliability ergonomics
- Prometheus metrics endpoint (`/metrics`)
- Dashboard-focused summary/read endpoints
- Structured audit events
- Load test suite for scheduler/query/memory scenarios

4. Extensible without being overengineered
- Clear module boundaries under `backend/internal/monitoring`
- Domain types for checks/results/incidents/alerts/audit
- Feature evolution documented in backend docs

## Core Use Cases

1. Service/API health monitoring
- Check HTTP APIs for status and content
- Track latency thresholds and warnings

2. Network and host-level availability checks
- TCP port reachability checks
- Process presence checks
- Log freshness heartbeat checks

3. Reliability dashboards for operations
- Aggregate health by server and application
- Inspect latest results and retention-window history

4. Incident operations (backend primitives)
- Incident lifecycle model exists (open/acknowledge/resolve)
- Audit trail support for check and incident mutations

5. Controlled migration away from DB-side MySQL monitoring pack
- MySQL replacement plan is already specified in:
  - `backend/docs/mysql-migration-spec.md`

## Current Capability Snapshot

### Check types
- `api`
- `tcp`
- `process`
- `command` (disabled by default unless explicitly enabled in config)
- `log`

### Main API endpoints
- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/checks`
- `POST /api/v1/checks`
- `PUT /api/v1/checks/{id}`
- `PATCH /api/v1/checks/{id}`
- `DELETE /api/v1/checks/{id}`
- `POST /api/v1/runs`
- `GET /api/v1/summary`
- `GET /api/v1/results`
- `GET /api/v1/dashboard/checks`
- `GET /api/v1/dashboard/summary`
- `GET /api/v1/dashboard/results`
- `GET /api/v1/incidents`
- `GET /api/v1/incidents/{id}`
- `POST /api/v1/incidents/{id}/acknowledge`
- `POST /api/v1/incidents/{id}/resolve`
- `GET /api/v1/audit`
- `GET /metrics`

## High-Level Architecture

```text
Checks config
   -> Scheduler (per-check timers)
      -> Runner (executes check by type)
         -> Store (file, optional Mongo mirror)
            -> Summary + dashboard API
            -> Alert evaluation
               -> Incident lifecycle
                  -> Audit trail

HTTP API
   -> validation/auth middleware
   -> envelope response contracts

Prometheus endpoint
   -> runtime/service metrics
```

## Project Layout

- `backend/`: Go service and operational docs
- `backend/cmd/healthmon`: runtime entrypoint
- `backend/cmd/loadtest`: load/perf validation tool
- `backend/internal/monitoring`: core monitoring, scheduling, API, rules, metrics, audit modules
- `backend/config/default.json`: base check configuration
- `backend/data/`: persisted runtime state
- `frontend/`: reserved UI workspace
- `docker-compose.yml`: local stack with backend + Mongo

## Quick Start

### 1) Run locally

```bash
cd backend
go run ./cmd/healthmon
```

### 2) Verify service

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/api/v1/summary
curl http://localhost:8080/api/v1/checks
```

### 3) Run tests

```bash
cd backend
go test ./...
```

### 4) Run load tests

```bash
cd backend
go run ./cmd/loadtest -scenario=query -duration=2m
```

## Configuration

Primary runtime config is JSON-based (`backend/config/default.json`).

Important points:
- per-check scheduling is configured at check level
- command checks are blocked unless `allowCommandChecks=true`
- default persistence is local file state
- MongoDB mirror is enabled only when `MONGODB_URI` is provided

Common runtime env vars:
- `CONFIG_PATH`
- `STATE_PATH`
- `MONGODB_URI`
- `MONGODB_DATABASE`
- `MONGODB_COLLECTION_PREFIX`

## Security & Safety Notes

- Mutating APIs require auth when auth is enabled in config
- Request body and query guards are implemented for API hardening
- Do not store secrets in config files
- Keep `allowCommandChecks=false` unless strictly required in trusted environments

## Deployment

### Docker

```bash
docker build -t healthops .
docker compose up -d
```

Service endpoints:
- HealthOps API: `http://localhost:8080`
- MongoDB: `mongodb://localhost:27017`

## Quality and Verification

HealthOps includes:
- unit and contract tests across monitoring modules
- e2e incident/auth/audit path tests
- load testing for scheduler/query/memory behavior
- release and security docs under `backend/docs/`

Useful docs:
- `backend/docs/release-checklist.md`
- `backend/docs/security-audit.md`
- `backend/docs/load-test-report.md`
- `backend/docs/mysql-migration-spec.md`

## Roadmap Direction

Immediate roadmap focus:
1. Replace external MySQL SQL-pack monitoring with native HealthOps MySQL module
2. Keep communication and AI queueing generic (incident-centric), while data collection remains source-specific
3. Maintain strong test gates before production cutovers

## Who Should Use This

Use HealthOps if you want:
- an internal reliability backend you can own and extend
- practical API-first monitoring with incident-friendly data
- controlled migration from fragmented scripts/tools into one service

Not ideal if you need:
- multi-tenant SaaS features out of the box
- turnkey managed alerting integrations without customization
- full APM traces/profiling (this is health monitoring, not full observability tracing)
