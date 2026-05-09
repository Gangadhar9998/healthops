# MySQL Monitoring Migration Spec (Replace DB-Side Pack)

## Status
- Proposed
- Date: 2026-04-18
- Owner: Backend

## 1. Decision
We will retire the separate MySQL SQL-pack application and move the same core capability into `healthops`.

This implementation must be:
- Modular at component boundaries
- Generic at interfaces where reuse is realistic
- Explicitly not overengineered

## 2. Scope
This migration must deliver feature parity for v1 with the current MySQL pack behavior:
1. 15-second MySQL sampling per monitored DB target
2. Raw sample persistence and interval delta persistence
3. Rule evaluation with consecutive breach and recovery streak logic
4. Incident open/close with deduplication
5. Incident evidence snapshots at open time
6. Notification outbox rows for external sender workers
7. Optional AI analysis queue + result storage
8. Retention cleanup for samples/incidents/queues

## 3. Non-Goals
1. No in-DB MySQL events/procedures in the target design
2. No auto-remediation actions (`KILL`, `FLUSH HOSTS`, `SET GLOBAL`, restart actions)
3. No plugin system or generic rule DSL engine in v1
4. No distributed queue system in v1

## 4. Architecture (Minimal-Generic)
We keep package root `backend/internal/monitoring` and add focused modules.

### 4.1 New module files
1. `mysql_models.go`
2. `mysql_collector.go`
3. `mysql_repository.go`
4. `mysql_rules.go`
5. `mysql_incident_evidence.go`
6. `notification_outbox.go`
7. `ai_queue.go`
8. `retention_jobs.go`
9. `mysql_api.go`
10. `mysql_*_test.go` (matching each module)

### 4.2 Interfaces (generic enough, not abstract-heavy)
1. `MySQLSampler`
   - `Collect(ctx context.Context, check CheckConfig) (MySQLSample, error)`
2. `MySQLMetricsRepository`
   - `AppendSample(sample MySQLSample) (sampleID string, err error)`
   - `ComputeAndAppendDelta(sampleID string) (MySQLDelta, error)`
   - `LatestSample(checkID string) (MySQLSample, error)`
   - `RecentSamples(checkID string, limit int) ([]MySQLSample, error)`
   - `RecentDeltas(checkID string, limit int) ([]MySQLDelta, error)`
3. `IncidentSnapshotRepository`
   - `SaveSnapshots(incidentID string, snaps []IncidentSnapshot) error`
4. `NotificationOutboxRepository`
   - `Enqueue(evt NotificationEvent) error`
   - `ListPending(limit int) ([]NotificationEvent, error)`
   - `MarkSent(id string) error`
   - `MarkFailed(id string, reason string) error`
5. `AIQueueRepository`
   - `Enqueue(incidentID string, promptVersion string) error`
   - `ClaimPending(limit int) ([]AIQueueItem, error)`
   - `Complete(incidentID string, result AIAnalysisResult) error`
   - `Fail(incidentID string, reason string) error`

## 5. Data Model

### 5.1 Config additions
Add to [types.go](/Users/sai/dev/work/medics-health-check/backend/internal/monitoring/types.go):
1. New check type: `mysql`
2. `CheckConfig.MySQL *MySQLCheckConfig`
3. `MySQLCheckConfig` fields:
   - `DSNEnv string` (required)
   - `ConnectTimeoutSeconds int` (default 3)
   - `QueryTimeoutSeconds int` (default 5)
   - `ProcesslistLimit int` (default 50)
   - `StatementLimit int` (default 20)
   - `HostUserLimit int` (default 20)

Validation rules:
1. `Type=mysql` requires `MySQL != nil`
2. `DSNEnv` must be non-empty and present in environment at runtime
3. Do not log DSN values

### 5.2 New structs
1. `MySQLSample`
   - `SampleID, CheckID, Timestamp`
   - Raw counters and gauges equivalent to current SQL pack fields
2. `MySQLDelta`
   - `SampleID, CheckID, IntervalSec`
   - Delta + per-second fields equivalent to current SQL pack behavior
3. `IncidentSnapshot`
   - `IncidentID, SnapshotType, Timestamp, PayloadJSON`
4. `NotificationEvent`
   - `NotificationID, IncidentID, Channel, PayloadJSON, Status, RetryCount, LastError, CreatedAt, SentAt`
5. `AIQueueItem` and `AIAnalysisResult`

### 5.3 Persistence Strategy
Use MongoDB-backed repositories for v1:
1. `{prefix}_mysql_samples`
2. `{prefix}_mysql_deltas`
3. `{prefix}_incident_snapshots`
4. `{prefix}_notification_outbox`
5. `{prefix}_ai_queue`
6. `{prefix}_ai_results`

Rules:
1. Startup fails when MongoDB is missing or unavailable.
2. Repository queries use indexed `checkId`, `incidentId`, `status`, and `timestamp` fields.
3. Retention pruning runs against MongoDB collections.

## 6. Rule Engine Changes
Extend existing alert rule behavior to include streak semantics.

### 6.1 Rule fields to add
In `AlertRule`:
1. `ConsecutiveBreaches int`
2. `RecoverySamples int`
3. `ThresholdNum float64`
4. `RuleCode string`

In `AlertState` (new persisted state object):
1. `RuleCode`
2. `CheckID`
3. `Status` (`OK` or `OPEN`)
4. `BreachStreak`
5. `RecoveryStreak`
6. `OpenIncidentID`

### 6.2 Required v1 rules
Implement these rule codes exactly:
1. `CONN_UTIL_WARN`
2. `CONN_UTIL_CRIT`
3. `MAX_CONN_REFUSED`
4. `ABORTED_CONNECT_SPIKE`
5. `THREADS_RUNNING_HIGH`
6. `ROW_LOCK_WAITS_HIGH`
7. `SLOW_QUERY_SPIKE`
8. `TMP_DISK_PCT_HIGH`
9. `THREAD_CREATE_SPIKE`

Thresholds are seeded from config defaults matching current SQL pack starter values.

## 7. Incident Evidence Capture
On incident open (and only on open):
1. Save `latest_sample`
2. Save `recent_deltas` (last 20)
3. Save `processlist`
4. Save `statement_analysis`
5. Save `host_summary`
6. Save `user_summary`
7. Save `host_cache`

Implementation notes:
1. Query sources are `performance_schema` and `sys`
2. Any evidence query failure must not block incident creation
3. Evidence failure must be attached as snapshot payload with error metadata

## 8. API Additions
Add endpoints in [service.go](/Users/sai/dev/work/medics-health-check/backend/internal/monitoring/service.go) via `mysql_api.go` handlers.

Read-only:
1. `GET /api/v1/mysql/samples?checkId=...&limit=...`
2. `GET /api/v1/mysql/deltas?checkId=...&limit=...`
3. `GET /api/v1/incidents/{id}/snapshots`
4. `GET /api/v1/notifications?status=pending&limit=...`
5. `GET /api/v1/ai/queue?status=pending&limit=...`

Mutating (auth required):
1. `POST /api/v1/notifications/{id}/sent`
2. `POST /api/v1/notifications/{id}/failed`
3. `POST /api/v1/ai/queue/{incidentId}/done`
4. `POST /api/v1/ai/queue/{incidentId}/failed`

## 9. Scheduler & Runtime
1. Existing scheduler remains source of timing
2. Each `mysql` check runs every `intervalSeconds` (default 15 for mysql checks)
3. On each run:
   - collect sample
   - persist sample
   - compute + persist delta
   - evaluate mysql rules
   - process incident transitions
   - enqueue outbox (+ optional ai queue)
4. Retention purge runs daily from existing app scheduler, not MySQL events

## 10. Security Requirements
1. DSN comes only from env var referenced by `DSNEnv`
2. API responses must never include raw DSN, passwords, or auth headers
3. Audit logs for all mutating queue endpoints
4. Read endpoints remain read-only and safe without auth if global config keeps GET public

## 11. Implementation Plan (Exact Tasks)

### Task M1: Config and types
- Add `mysql` check type and `MySQLCheckConfig`
- Add validation/defaults
- Acceptance:
  - invalid mysql config rejected
  - missing env var rejected at check execution with safe error message

### Task M2: MySQL collector
- Implement collector for required status/variable/sys snapshots
- Acceptance:
  - successful sample has all required core fields
  - collection timeout handled cleanly

### Task M3: Sample + delta repository
- Implement MongoDB-backed sample/delta repository
- Acceptance:
  - append and latest/recent reads are deterministic
  - delta handles counter reset with `max(0, diff)` behavior

### Task M4: Rule streak engine
- Add rule state persistence and streak transitions
- Acceptance:
  - incidents open only after configured consecutive breaches
  - incidents close only after configured recovery streak

### Task M5: Incident evidence snapshots
- Add snapshot repository and capture pipeline on open
- Acceptance:
  - all 7 snapshot types saved when available
  - partial failure still opens incident

### Task M6: Notification outbox
- Add outbox repository and enqueue on incident open
- Acceptance:
  - one outbox event per incident open
  - idempotent sender status updates

### Task M7: AI queue/result (optional but built-in)
- Add queue/result repositories and endpoints
- Acceptance:
  - queue item created only when `enableAI=true`
  - repeated queue inserts for same incident are prevented

### Task M8: API handlers
- Add new read + mutating endpoints
- Acceptance:
  - envelope contract unchanged
  - auth enforced on mutating endpoints

### Task M9: Retention job
- Add daily cleanup for samples, deltas, closed incidents, queue/results
- Acceptance:
  - configurable retention windows are respected

### Task M10: Seed defaults and docs
- Add mysql v1 alert default thresholds to config/docs
- Acceptance:
  - fresh startup loads default mysql rules

## 12. Test Plan

### 12.1 Unit tests
Files:
1. `mysql_collector_test.go`
2. `mysql_repository_test.go`
3. `mysql_rules_test.go`
4. `mysql_incident_evidence_test.go`
5. `notification_outbox_test.go`
6. `ai_queue_test.go`

Must cover:
1. Counter-delta math and reset behavior
2. Breach and recovery streak transitions
3. Dedup (single open incident per `ruleCode+checkID`)
4. Snapshot capture success and partial failures
5. Outbox and AI queue idempotency

### 12.2 Contract tests
Extend `contract_test.go` for new endpoints:
1. Response envelope
2. Status codes
3. Field types
4. Auth on mutating endpoints

### 12.3 E2E tests
Add:
1. `TestE2E_MySQLIncidentLifecycle`
2. `TestE2E_MySQLRuleStreaks`
3. `TestE2E_MySQLEvidenceCapture`
4. `TestE2E_NotificationOutboxFlow`
5. `TestE2E_AIQueueFlow`

### 12.4 Race and load tests
1. Race tests for collector+scheduler+outbox concurrency
2. Add `loadtest` mysql scenario with pass/fail thresholds

## 13. Test Gates (Blocking)
Run from `backend/`.

### Gate G1: Build + all tests
- Command: `go test ./...`
- Pass condition: exit code 0

### Gate G2: MySQL unit suite
- Command: `go test ./internal/monitoring -run 'TestMySQL(Collector|Repository|Delta|Rules|Evidence|Outbox|AIQueue)' -count=1`
- Pass condition: exit code 0

### Gate G3: API contract suite
- Command: `go test ./internal/monitoring -run 'Test(MySQLSamplesEndpointContract|MySQLDeltasEndpointContract|IncidentSnapshotsEndpointContract|NotificationsEndpointContract|AIQueueEndpointContract|MySQLMutatingEndpointsRequireAuth)' -count=1`
- Pass condition: exit code 0

### Gate G4: E2E mysql lifecycle
- Command: `go test ./internal/monitoring -run 'TestE2E_(MySQLIncidentLifecycle|MySQLRuleStreaks|MySQLEvidenceCapture|NotificationOutboxFlow|AIQueueFlow)$' -count=1`
- Pass condition: exit code 0

### Gate G5: Race safety
- Command: `go test -race ./internal/monitoring -run 'Test(MySQLCollectorConcurrent|MySQLSchedulerConcurrent|MySQLOutboxConcurrent)$' -count=1`
- Pass condition: exit code 0

### Gate G6: Load safety
- Command: `go run ./cmd/loadtest -scenario=mysql -duration=20m -checks=3 -workers=20 -memory-growth-max=40 -goroutine-limit=2000 -query-latency-max=250ms`
- Pass conditions:
  - no panic
  - no deadlock
  - memory growth <= 40%
  - goroutines <= 2000
  - p95 API read latency <= 250ms

### Gate G7: Security regression
- Command: `go test ./internal/monitoring -run 'Test(NoSecretsInAPIResponses|MySQLDSNRedaction|AllMutatingEndpointsRequireAuth|InputValidation)' -count=1`
- Pass condition: exit code 0

### Gate G8: Final production checklist
- Command group:
  1. `go test ./...`
  2. `go test -race ./internal/monitoring -count=1`
  3. `go run ./cmd/loadtest -scenario=mysql -duration=20m ...`
- Pass condition: all green

## 14. Cutover Plan (No Parallel Legacy Pack Maintenance)
1. Deploy new healthops build with mysql checks configured
2. Verify Gates G1-G8 in target environment
3. Disable old db-side events:
   - `ALTER EVENT dbmon.ev_collect_sample DISABLE;`
   - `ALTER EVENT dbmon.ev_evaluate_alerts DISABLE;`
   - `ALTER EVENT dbmon.ev_purge_old_data DISABLE;`
4. Keep old schema read-only for 7 days for forensics
5. Remove old pack objects after 7 days if no gaps found

## 15. Definition of Done
Migration is complete only when:
1. All gates G1-G8 pass
2. At least one real mysql alert opens and auto-resolves correctly in non-prod
3. Evidence snapshots are present and queryable via API
4. Outbox events are sent by external notifier and acknowledged
5. Legacy MySQL pack events are disabled
