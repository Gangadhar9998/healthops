# Security Audit Report

## Scope
- Project: `medics-health-check/backend`
- Audit date: 2026-04-18
- Scope focus: API auth enforcement, input validation, secrets exposure, command execution controls, auditability.

## Evidence Commands

```bash
cd backend

# Security-focused unit suite
go test ./internal/monitoring -run 'Test(AllMutatingEndpointsRequireAuth|InvalidAuthRejected|ValidAuthAccepted|ReadEndpointsBypassAuth|NoSecretsInAPIResponses|InputValidation|TimingAttackResistance|SecurityHeaders|RateLimitingStatus|CSRFProtectionStatus|SecretsInLogsStatus)$' -count=1

# E2E auth and lifecycle checks
go test ./internal/monitoring -run 'TestE2E_(FullIncidentLifecycle|AlertDeduplication|CooldownEnforcement|RecoveryAutoResolve|AuditTrail|AuthEnforcement)$' -count=1

# Contract checks for API envelope/errors
go test ./internal/monitoring -run 'Test(GET|POST|PUT|DELETE|ResponseEnvelope|ErrorResponses|FieldType|IncidentEndpointsUnAvailable|AuditEndpointsUnAvailable)' -count=1

# Spot race check on critical paths
go test -race ./internal/monitoring -run 'Test(AllMutatingEndpointsRequireAuth|E2E_FullIncidentLifecycle|POSTChecksContract|MetricsEndpoint)$' -count=1
```

## Results Summary
- `PASS`: Mutating endpoint auth enforcement (`POST/PUT/PATCH/DELETE` now rejected without valid auth).
- `PASS`: Invalid credentials rejected with `401` and `WWW-Authenticate` challenge.
- `PASS`: Read-only endpoints accessible without auth when configured.
- `PASS`: Command checks disabled by default and gated by config (`allowCommandChecks=false`).
- `PASS`: Input validation paths verified for malformed/invalid payloads.
- `PASS`: Audit trail written for key mutating actions (`check.*`, `incident.*`).
- `PASS`: Constant-time credential comparison used for basic auth checks.

## Key Fixes Applied During Audit Closure
1. Added hard auth enforcement in mutating handlers (defense in depth, independent of middleware chain).
2. Added auth middleware to `Run()` handler chain.
3. Removed permissive security test behavior that previously logged gaps without failing.
4. Fixed scheduler reschedule behavior to avoid scheduling when scheduler is not running (prevented runaway background activity in tests).

## Residual Risks / Follow-Ups
- Authentication mode is Basic Auth. Recommended next hardening for production internet exposure:
  - move to token/JWT or mTLS,
  - rotate credentials via secret manager,
  - add brute-force protections.
- Add explicit rate limiting middleware if external/untrusted network access is expected.
- Add strict security headers policy (HSTS/CSP/etc.) when deployed behind reverse proxy.

## Audit Verdict
- For current single-tenant internal v1 scope: **Acceptable with noted follow-ups**.
- No critical blocker found for internal deployment assuming network perimeter controls are in place.

## Production Hardening Checklist

Apply these before exposing the service to any untrusted network:

- [ ] **Set `HEALTHOPS_REQUIRE_PROD_AUTH=true`** in the service environment. The
      process refuses to start if it detects the default admin password
      or `allowCommandChecks=true` (RCE risk).
- [ ] **Set `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD`** to a strong, unique password
      before first start. For Mongo-backed deployments this is the only way
      production mode can verify that the admin password has been rotated away
      from any default.
- [ ] **Do NOT enable `allowCommandChecks`** in `backend/config/default.json`
      or via the API in production. Shell command checks execute arbitrary
      commands and are an RCE risk if a check is mis-authored or an attacker
      gains write access to the config.
- [ ] **Run behind a TLS-terminating reverse proxy** (nginx, Caddy, AWS ALB,
      Cloudflare). The Go service speaks plain HTTP; never expose it directly
      to the internet.
- [ ] **Verify the login endpoint rate limit** is in place. The
      `/api/v1/auth/login` route is wrapped by a per-IP limiter (5 req/min) on
      top of the global 100 req/min limit to mitigate credential stuffing.
- [ ] Rotate `HEALTHOPS_JWT_SECRET` and `HEALTHOPS_AI_ENCRYPTION_KEY` through
      the deployment secret manager on a documented cadence.
- [ ] Audit the MongoDB audit collection regularly for unexpected mutating
      actions.
