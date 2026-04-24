# Repository Guidelines

## Project Structure & Module Organization
- `backend/` is the Go service. `backend/cmd/healthops/main.go` is the entrypoint.
- `backend/internal/monitoring/` holds config, store, runner, HTTP handlers, MySQL monitoring, AI BYOK layer, incidents, analytics, and alert rules.
- `backend/config/default.json` is the **first-run seed** for runtime defaults and any starter checks. After the first run, the persisted state (`backend/data/state.json`, optionally mirrored to MongoDB) is the single source of truth — checks are managed via the API/UI and survive restarts. Edits to `default.json` are ignored once state exists.
- `backend/data/` stores the file-backed state: `state.json`, JSONL repositories, AI config, encryption keys.
- `backend/docs/` holds API reference, migration specs, security audit, and release checklist.
- `frontend/` is reserved for the monitoring UI and should stay separate from backend code.
- `docs/` contains architectural decision records (ADRs) and the operational runbook.

## Build, Test, and Development Commands
- `cd backend && go run ./cmd/healthops` starts the monitoring API and scheduler.
- `cd backend && go test ./...` runs backend tests once they are added.
- `go fmt ./...` should be run from `backend/` before committing Go changes.
- `go test ./...` and `go run ./cmd/healthops` assume you are inside the backend module directory.

## Coding Style & Naming Conventions
- Use standard Go formatting and keep package names short, lowercase, and domain-oriented.
- Prefer small packages with clear boundaries such as `internal/monitoring`.
- Name JSON fields and config keys in lower camel case to match the API payloads.
- Keep new UI files under `frontend/` and backend code under `backend/` only.

## Testing Guidelines
- Add Go tests next to the code they cover, for example `backend/internal/monitoring/config_test.go`.
- Test types: unit tests, contract tests (`contract_test.go`), E2E tests (`e2e_test.go`, `mysql_e2e_test.go`), race tests (`mysql_race_test.go`), security tests (`security_audit_test.go`).
- Focus tests on config validation, result retention, check execution, API handlers, AI config encryption, and incident lifecycle.
- For manual verification, hit `/healthz`, `/api/v1/summary`, and `GET /api/v1/checks` after starting the service.

## Commit & Pull Request Guidelines
- Keep commits short, imperative, and scoped to a single backend or frontend change.
- Pull requests should explain the feature, the impact on checks/configuration, and how it was validated.
- Include screenshots for UI work and sample JSON for API changes when useful.

## Security & Configuration Tips
- Do not hardcode credentials, webhook URLs, or passwords in source files.
- Keep secrets in environment variables or deployment config, never in `backend/config/default.json`.
- AI provider API keys are AES-256-GCM encrypted at rest in `data/ai_config.json` and always masked in API responses.
- MySQL DSNs are read from environment variables referenced by `dsnEnv` in check config.
- Review config changes carefully because they directly control alerting and monitoring scope.
