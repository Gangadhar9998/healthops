# MongoDB-Only Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make HealthOps a Docker-first MongoDB-backed application with no runtime JSON/JSONL/file persistence fallbacks.

**Architecture:** MongoDB is the required authoritative persistence layer for checks, results, incidents, audit events, users, servers, notification channels, notification outbox, AI config, AI jobs/results, incident snapshots, MySQL samples/deltas, and server metrics. Backend startup fails fast when MongoDB or required secrets are missing. Static config remains only as a first-run seed source for checks/servers and as process configuration.

**Tech Stack:** Go, MongoDB Go Driver v2, net/http, React/Vite frontend, Docker Compose.

---

### Task 1: Fix Baseline Parse Failure

**Files:**
- Modify: `backend/internal/util/jsonl/jsonl_chaos_test.go`

- [ ] **Step 1: Write or identify failing command**

Run: `cd backend && go test ./internal/util/jsonl`

Expected before fix: FAIL because `jsonl_chaos_test.go` has duplicate `package jsonl`.

- [ ] **Step 2: Minimal implementation**

Remove the second duplicate `package jsonl` line at the top of `backend/internal/util/jsonl/jsonl_chaos_test.go`.

- [ ] **Step 3: Verify**

Run: `cd backend && go test ./internal/util/jsonl`

Expected after fix: PASS.

### Task 2: Add Required Mongo Store For Checks And Results

**Files:**
- Create: `backend/internal/monitoring/mongo_store.go`
- Create: `backend/internal/monitoring/mongo_store_test.go`
- Modify: `backend/internal/monitoring/service.go`

- [ ] **Step 1: Write failing store tests**

Add tests using a fake `Mirror` implementation that verify:
- `NewMongoStore` rejects a nil mirror.
- initial seed checks are synced when Mongo state is empty.
- `Snapshot` returns cloned state from Mongo.
- `UpsertCheck`, `DeleteCheck`, `AppendResults`, and `SetLastRun` mutate Mongo state without touching files.

- [ ] **Step 2: Implement MongoStore**

Implement a `MongoStore` that satisfies `monitoring.Store` by reading state through `Mirror.ReadState`, mutating an in-memory copy per operation, pruning results, and persisting with `Mirror.SyncState`. It must not depend on `FileStore` or local paths.

- [ ] **Step 3: Update degraded-mode detection**

Change service degraded-mode setup to use a small interface for Mongo health instead of type-asserting `*HybridStore`.

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/monitoring -run 'TestMongoStore|TestNewService'`.

### Task 3: Add Mongo Repositories For Runtime JSONL Domains

**Files:**
- Create: `backend/internal/monitoring/mongo_incident_repository.go`
- Create: `backend/internal/monitoring/mongo_snapshot_repository.go`
- Create: `backend/internal/monitoring/mongo_server_metrics_repository.go`
- Create: `backend/internal/monitoring/mongo_runtime_repositories_test.go`
- Create: `backend/internal/monitoring/notify/mongo_outbox.go`
- Create: `backend/internal/monitoring/notify/mongo_outbox_test.go`
- Create: `backend/internal/monitoring/ai/mongo_queue.go`
- Create: `backend/internal/monitoring/ai/mongo_queue_test.go`
- Create: `backend/internal/monitoring/mysql/mongo_repository.go`
- Create: `backend/internal/monitoring/mysql/mongo_repository_test.go`

- [ ] **Step 1: Write failing tests**

For each repository, test CRUD/append/list/mark/prune behavior with fake collection abstractions or Mongo integration helpers already used in the repo.

- [ ] **Step 2: Implement repositories**

Implement Mongo-backed replacements for:
- `IncidentRepository`
- `IncidentSnapshotRepository`
- `ServerMetricsRepository` behavior
- `notify.NotificationOutboxRepository`
- `ai.FileAIQueue` behavior through a repository-compatible type
- `monitoring.MySQLMetricsRepository`

- [ ] **Step 3: Verify targeted packages**

Run:
- `cd backend && go test ./internal/monitoring -run 'TestMongo.*Incident|TestMongo.*Snapshot|TestMongo.*ServerMetrics'`
- `cd backend && go test ./internal/monitoring/notify -run TestMongo`
- `cd backend && go test ./internal/monitoring/ai -run TestMongo`
- `cd backend && go test ./internal/monitoring/mysql -run TestMongo`

### Task 4: Rewrite Startup To Require MongoDB

**Files:**
- Modify: `backend/cmd/healthops/main.go`
- Modify: `backend/internal/monitoring/users.go`
- Modify: `backend/internal/monitoring/ai/config.go`
- Modify: `backend/internal/monitoring/ai/repositories/config_store_adapter.go`
- Modify: `backend/internal/monitoring/service.go`

- [ ] **Step 1: Write failing startup tests where practical**

Add tests or narrow unit coverage that verify required env validation:
- missing `MONGODB_URI` fails before service start
- missing `HEALTHOPS_JWT_SECRET` fails
- missing `HEALTHOPS_AI_ENCRYPTION_KEY` fails when AI config encryption is used

- [ ] **Step 2: Require MongoDB**

Replace `FileStore`/`HybridStore` startup selection with:
- read `MONGODB_URI`, `MONGODB_DATABASE`, `MONGODB_COLLECTION_PREFIX`
- connect once with timeout
- create `MongoMirror`
- create `MongoStore`
- seed config checks/servers only when Mongo collections are empty
- fail fast on errors

- [ ] **Step 3: Wire Mongo runtime repositories**

Replace runtime calls to:
- `NewFileNotificationOutbox`
- `NewFileAIQueue`
- `NewFileSnapshotRepository`
- `mysql.NewFileMySQLRepository`
- `NewMemoryIncidentRepository`
- `NewServerMetricsRepository`
- `NewFileAuditRepository`
- `NewNotificationChannelStore`
- file-based AI config store

with Mongo-backed repositories.

- [ ] **Step 4: Make secrets environment-backed**

Make JWT secret and AI encryption key come from environment, not local files.

### Task 5: Remove Runtime File/JSONL Paths From Product Docs And Docker

**Files:**
- Modify: `backend/README.md`
- Modify: `ReadMe.md`
- Modify: `docs/deployment-guide.md`
- Modify: `docs/backups.md`
- Modify: `docs/runbook.md`
- Modify: `docker-compose.yml`
- Modify: `Dockerfile`
- Modify: `backend/config/default.json`

- [ ] **Step 1: Update docs**

Document MongoDB as required and remove `STATE_PATH`, `DATA_DIR`, JSONL state, and file fallback language from production instructions.

- [ ] **Step 2: Update Docker/Compose**

Make Compose provide required Mongo and secrets. Remove app data volume if no runtime file state remains.

- [ ] **Step 3: Verify frontend build unaffected**

Run: `cd frontend && npm run build`.

### Task 6: Delete Or Quarantine File Persistence Implementations

**Files:**
- Delete or move out of runtime build after tests are replaced:
  - `backend/internal/monitoring/store.go`
  - `backend/internal/monitoring/hybrid_store.go`
  - file-backed repo constructors and tests for JSON/JSONL stores
  - `backend/internal/util/jsonl` if no longer referenced

- [ ] **Step 1: Remove references**

Run: `rg 'NewFile|FileStore|HybridStore|jsonl|STATE_PATH|DATA_DIR|state.json|\\.jsonl' backend` and remove runtime references.

- [ ] **Step 2: Keep tests meaningful**

Replace file-store tests with Mongo-store tests rather than deleting coverage.

- [ ] **Step 3: Verify full backend**

Run: `cd backend && go test ./...`.

### Task 7: Final Review And Verification

**Files:**
- All changed files

- [ ] **Step 1: Run formatting**

Run: `cd backend && go fmt ./...`.

- [ ] **Step 2: Run backend tests**

Run: `cd backend && go test ./...`.

- [ ] **Step 3: Run frontend build**

Run: `cd frontend && npm run build`.

- [ ] **Step 4: Run code review**

Dispatch final reviewers for architecture consistency, security, and operational docs.
