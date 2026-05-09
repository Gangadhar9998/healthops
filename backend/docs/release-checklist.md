# Production Release Checklist (Execution)

## How To Use
1. Execute each command in order.
2. Mark checkbox only after command output is validated.
3. Record links or logs in the `Evidence` field.
4. Release gate is blocked until all required sections pass.

## 1) Backend Verification
- [x] `go test ./...` passes
Evidence: 2026-04-18 `ok  	medics-health-check/backend/internal/monitoring	46.026s`

- [x] `go test -race ./internal/monitoring -run 'Test(AllMutatingEndpointsRequireAuth|E2E_FullIncidentLifecycle|POSTChecksContract|MetricsEndpoint)$' -count=1` passes
Evidence: 2026-04-18 `ok  	medics-health-check/backend/internal/monitoring	1.796s`

- [x] Contract tests pass:
`go test ./internal/monitoring -run 'Test(GET|POST|PUT|DELETE|ResponseEnvelope|ErrorResponses|FieldType|IncidentEndpointsUnAvailable|AuditEndpointsUnAvailable)' -count=1`
Evidence: 2026-04-18 `ok  	medics-health-check/backend/internal/monitoring	0.520s`

- [x] E2E core flows pass:
`go test ./internal/monitoring -run 'TestE2E_(FullIncidentLifecycle|AlertDeduplication|CooldownEnforcement|RecoveryAutoResolve|AuditTrail|AuthEnforcement)$' -count=1`
Evidence: 2026-04-18 `ok  	medics-health-check/backend/internal/monitoring	1.364s`

## 2) Security Verification
- [x] Security audit suite passes:
`go test ./internal/monitoring -run 'Test(AllMutatingEndpointsRequireAuth|InvalidAuthRejected|ValidAuthAccepted|ReadEndpointsBypassAuth|NoSecretsInAPIResponses|InputValidation|TimingAttackResistance|SecurityHeaders|RateLimitingStatus|CSRFProtectionStatus|SecretsInLogsStatus)$' -count=1`
Evidence: 2026-04-18 `ok  	medics-health-check/backend/internal/monitoring	0.409s`

- [x] Mutating APIs return `401` without valid auth.
Evidence: 2026-04-18 live check on auth-enabled instance: `POST /api/v1/checks` -> `401` (without auth), `201` (with `admin:secret`).

- [x] Command checks disabled by default (`allowCommandChecks=false`).
Evidence: `backend/config/default.json` contains `"allowCommandChecks": false`.

- [x] Security report reviewed: `backend/docs/security-audit.md`
Evidence: Reviewed during final gate execution on 2026-04-18.

## 3) Load & Performance Verification
- [x] Query load scenario passes:
`go run ./cmd/loadtest -scenario=query -duration=20s -checks=20 -queries=20 -workers=10 -memory-growth-max=400 -goroutine-limit=5000`
Evidence: 2026-04-18 PASS, p95=2ms, failures=0, queries=1817.

- [ ] Scheduler scenario passes target thresholds for your environment.
Evidence: 2026-04-18 FAIL, `scheduler lag p95 (16m38.921784s) exceeds threshold (5s)`.

- [ ] Memory scenario (>=30m) shows no leak trend.
Evidence: 2026-04-18 INCONCLUSIVE/FAIL. Long-run memory scenario showed severe execution stalls and did not complete in expected wall-clock cadence.

- [x] Load report updated: `backend/docs/load-test-report.md`
Evidence: Updated with final gate rerun outcomes on 2026-04-18.

## 4) Deployment Readiness
- [ ] Docker build passes (`Dockerfile`)
Evidence: 2026-04-18 FAIL (environment): Docker daemon unavailable (`/Users/sai/.docker/run/docker.sock`).

- [ ] `docker-compose.yml` boot test passes
Evidence: 2026-04-18 FAIL (environment): Cannot connect to Docker daemon.

- [x] `.env.example` validated against runtime config
Evidence: Environment docs cover required MongoDB and secret keys (`MONGODB_URI`, `MONGODB_DATABASE`, `MONGODB_COLLECTION_PREFIX`, `HEALTHOPS_JWT_SECRET`, `HEALTHOPS_AI_ENCRYPTION_KEY`, `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD`).

- [x] Runbook reviewed: `docs/runbook.md`
Evidence: Reviewed during gate run on 2026-04-18.

## 5) Release Sign-Off
- [ ] Engineering sign-off
Name / Date:

- [ ] QA sign-off
Name / Date:

- [ ] Security sign-off
Name / Date:

## Release Gate
- Required sections for production cut: `1`, `2`, `3`, `5`
- Any unchecked required item blocks release.
