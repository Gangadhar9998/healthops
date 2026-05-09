# Health Monitoring Service Runbook

**Last Updated:** 2026-04-17

## 1. Startup

### Prerequisites

- **Go 1.19+** installed
- **MongoDB 7+** reachable from HealthOps. MongoDB is required authoritative storage.
- **Basic tools:** `curl`, `ps`, `netstat`

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CONFIG_PATH` | No | `backend/config/default.json` | Path to configuration file |
| `MONGODB_URI` | Yes | - | MongoDB connection string for authoritative storage |
| `MONGODB_DATABASE` | Yes | `healthops` | MongoDB database name |
| `MONGODB_COLLECTION_PREFIX` | Yes | `healthops` | MongoDB collection prefix |
| `HEALTHOPS_JWT_SECRET` | Yes | - | JWT signing secret; use at least 32 random characters |
| `HEALTHOPS_AI_ENCRYPTION_KEY` | Yes | - | Deployment-managed secret for encrypting AI provider keys |
| `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` | Yes on first start | - | Bootstraps or rotates the admin password |

`STATE_PATH`, `DATA_DIR`, `state.json`, and JSONL repository files are obsolete file-store operational controls. Do not use them for deploy, backup, restore, or incident response.

### Start the Service

```bash
# Navigate to backend directory
cd backend

# Start the monitoring service
MONGODB_URI=mongodb://localhost:27017 \
MONGODB_DATABASE=healthops \
MONGODB_COLLECTION_PREFIX=healthops \
HEALTHOPS_JWT_SECRET='change-me-at-least-32-characters' \
HEALTHOPS_AI_ENCRYPTION_KEY='change-me-random-ai-secret' \
HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD='change-me-admin-password' \
go run ./cmd/healthops

# Alternative: Build and run
go build -o healthops ./cmd/healthops
MONGODB_URI=mongodb://localhost:27017 \
MONGODB_DATABASE=healthops \
MONGODB_COLLECTION_PREFIX=healthops \
HEALTHOPS_JWT_SECRET='change-me-at-least-32-characters' \
HEALTHOPS_AI_ENCRYPTION_KEY='change-me-random-ai-secret' \
HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD='change-me-admin-password' \
./healthops
```

### Verify Startup

**Check health endpoint:**
```bash
curl http://localhost:8080/healthz
# Expected: {"status":"ok","success":true}
```

**Check readiness:**
```bash
curl http://localhost:8080/readyz
# Expected: {"success":true,"data":{"status":"ready","checks":<count>,"lastRunAt":null}}
```

**Check service status:**
```bash
# Check if process is running
ps aux | grep healthops

# Check port binding
netstat -an | grep 8080
```

## 2. Configuration

### Config File Structure

```json
{
  "server": {
    "addr": ":8080",
    "readTimeoutSeconds": 10,
    "writeTimeoutSeconds": 10,
    "idleTimeoutSeconds": 60
  },
  "auth": {
    "enabled": false,
    "username": "admin",
    "password": "securepassword"
  },
  "retentionDays": 7,
  "checkIntervalSeconds": 60,
  "workers": 8,
  "allowCommandChecks": false,
  "checks": [...]
}
```

### Check Types

#### API Checks
```json
{
  "id": "prod-api",
  "name": "Production API",
  "type": "api",
  "server": "prod-1",
  "application": "medics",
  "target": "https://example.com/health",
  "expectedStatus": 200,
  "timeoutSeconds": 5,
  "warningThresholdMs": 1000,
  "intervalSeconds": 120,
  "retryCount": 3,
  "retryDelaySeconds": 5,
  "cooldownSeconds": 30,
  "enabled": true,
  "tags": ["api", "prod"]
}
```

#### TCP Checks
```json
{
  "id": "database-port",
  "name": "Database Port Check",
  "type": "tcp",
  "server": "prod-1",
  "application": "database",
  "host": "localhost",
  "port": 3306,
  "timeoutSeconds": 5,
  "warningThresholdMs": 500,
  "intervalSeconds": 60,
  "enabled": true
}
```

#### Process Checks
```json
{
  "id": "nginx-process",
  "name": "Nginx Process Check",
  "type": "process",
  "server": "prod-1",
  "application": "webserver",
  "target": "nginx",
  "timeoutSeconds": 5,
  "intervalSeconds": 60,
  "enabled": true
}
```

#### Command Checks
```json
{
  "id": "disk-space",
  "name": "Disk Space Check",
  "type": "command",
  "server": "prod-1",
  "application": "system",
  "command": "df -h / | awk 'NR==2 {print $5}' | sed 's/%//'",
  "expectedStatus": 0,
  "expectedContains": "Use this",
  "timeoutSeconds": 10,
  "intervalSeconds": 300,
  "enabled": true
}
```

**NOTE:** Command checks require `allowCommandChecks: true` in config.

#### Log Checks
```json
{
  "id": "log-rotation",
  "name": "Log Rotation Check",
  "type": "log",
  "server": "prod-1",
  "application": "medics",
  "path": "/var/log/medics/access.log",
  "freshnessSeconds": 3600,
  "timeoutSeconds": 5,
  "intervalSeconds": 300,
  "enabled": true
}
```

### Per-Check Scheduling

Each check can have individual scheduling parameters:

- **`intervalSeconds`** - How often to run the check (default: 60)
- **`retryCount`** - Number of retry attempts on failure (default: 3)
- **`retryDelaySeconds`** - Delay between retries (default: 5)
- **`cooldownSeconds`** - Minimum time between consecutive failures (default: 30)

## 3. Troubleshooting - Failed Checks

### Check Logs for Error Messages

```bash
# Check service logs
sudo journalctl -u healthops -f
# or: docker compose logs -f healthops

# Check audit events through the API or MongoDB audit export
curl -s http://localhost:8080/api/v1/audit \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# Run verbose mode (if available)
go run ./cmd/healthops -v
```

### Common Issues and Solutions

#### **Connection Refused**
```bash
# Test connectivity manually
curl -v https://example.com/health

# Check if target is reachable
ping example.com

# Check firewall rules
sudo iptables -L
```

#### **Timeout Issues**
```bash
# Increase timeout in config
{
  "timeoutSeconds": 30,
  "warningThresholdMs": 5000
}
```

#### **Wrong Status Code**
```bash
# Check actual response
curl -I https://example.com/health

# Adjust expected status in config
{
  "expectedStatus": 200
}
```

#### **Wrong Response Body**
```bash
# Check response content
curl -s https://example.com/health

# Update expectedContains
{
  "expectedContains": "healthy"
}
```

### Test Individual Check Manually

```bash
# Trigger a specific check
curl -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -d '{"checkId": "prod-api"}'

# Check results
curl http://localhost:8080/api/v1/results?checkId=prod-api&days=1
```

### Debug Single Check

```bash
# Create a simple test check
curl -X POST http://localhost:8080/api/v1/checks \
  -H "Content-Type: application/json" \
  -d '{
    "id": "debug-check",
    "name": "Debug Check",
    "type": "api",
    "target": "https://httpbin.org/status/200",
    "expectedStatus": 200,
    "intervalSeconds": 10,
    "enabled": true
  }'

# Watch results in real-time
watch -n 5 curl http://localhost:8080/api/v1/results?checkId=debug-check
```

## 4. Troubleshooting - Failed Alerts

### Check Alert Rules Configuration

```bash
# Get current alert rules (if configured via API)
curl http://localhost:8080/api/v1/alert-rules

# Check if alerts are enabled in config
grep -A 10 "alertRules" config/default.json
```

### Check Cooldown Period

```bash
# Check if cooldown is blocking alerts
{
  "cooldownMinutes": 15,
  "severity": "critical"
}
```

### Test Alert Delivery

```bash
# Check audit log for alert attempts
curl -s http://localhost:8080/api/v1/audit \
  -H "Authorization: Bearer $TOKEN" | jq '.data[] | select(.action | test("alert"))'

# Manual alert test
curl -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -d '{"checkId": "prod-api"}'
```

### Verify Channel Configuration

#### Email Alerts
```json
{
  "type": "email",
  "config": {
    "smtp": {
      "host": "smtp.gmail.com",
      "port": 587,
      "username": "alerts@company.com",
      "password": "app-password"
    },
    "from": "health-monitor@company.com",
    "to": ["admin@company.com", "ops@company.com"]
  }
}
```

#### Webhook Alerts
```json
{
  "type": "webhook",
  "config": {
    "url": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL",
    "method": "POST",
    "headers": {
      "Content-Type": "application/json",
      "Authorization": "Bearer token"
    }
  }
}
```

## 5. Troubleshooting - Incidents

### Check Incident Status

```bash
# List all incidents
curl http://localhost:8080/api/v1/incidents

# Get specific incident
curl http://localhost:8080/api/v1/incidents/<incident-id>
```

### Manual Incident Management

**Acknowledge an incident:**
```bash
curl -X PATCH http://localhost:8080/api/v1/incidents/<incident-id> \
  -H "Content-Type: application/json" \
  -d '{
    "status": "acknowledged",
    "acknowledgedBy": "admin@company.com"
  }'
```

**Resolve an incident:**
```bash
curl -X PATCH http://localhost:8080/api/v1/incidents/<incident-id> \
  -H "Content-Type: application/json" \
  -d '{
    "status": "resolved",
    "resolvedBy": "admin@company.com",
    "message": "Issue resolved"
  }'
```

### Auto-Resolve on Recovery

The service automatically resolves incidents when the underlying check recovers:

```json
{
  "autoResolve": true,
  "resolutionThreshold": 3
}
```

## 6. Backup and Restore

### Authoritative Storage Location

- **MongoDB URI:** `MONGODB_URI`
- **MongoDB database:** `MONGODB_DATABASE`
- **Collection prefix:** `MONGODB_COLLECTION_PREFIX`
- **Config file:** `backend/config/default.json`
- **Deployment secrets:** `HEALTHOPS_JWT_SECRET`, `HEALTHOPS_AI_ENCRYPTION_KEY`, `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD`

### Backup Procedure

```bash
ENV_FILE=/etc/healthops/healthops.env \
BACKUP_DIR=/var/backups/healthops \
scripts/healthops-mongo-backup.sh
```

Back up `/etc/healthops/healthops.env` or the equivalent secret-manager values
separately in encrypted storage.

### Restore Procedure

```bash
# Stop the service
pkill healthops

# Restore deployment environment secrets
sudo install -m 0640 -o root -g healthops /path/to/healthops.env /etc/healthops/healthops.env
set -a
. /etc/healthops/healthops.env
set +a

# Restore authoritative MongoDB state
CONFIRM_RESTORE="$MONGODB_DATABASE" \
  scripts/healthops-mongo-restore.sh /var/backups/healthops/healthops-mongo-healthops-20260509T120000Z.archive.gz

# Restore config seed if needed
cp /path/to/config.json backend/config/default.json

# Start the service
cd backend && go run ./cmd/healthops
```

### Automated Backup Script

```bash
# /etc/cron.d/healthops-backup
0  *  *   *   *   healthops cd /opt/healthops && ENV_FILE=/etc/healthops/healthops.env BACKUP_DIR=/var/backups/healthops scripts/healthops-mongo-backup.sh >> /var/log/healthops-backup.log 2>&1
```

## 7. Performance Tuning

### Worker Count Tuning

```json
{
  "workers": 16,
  "checkIntervalSeconds": 60
}
```

**Recommendations:**
- **CPU cores:** Set workers to 2x CPU cores
- **Network checks:** More workers for frequent checks
- **Disk I/O:** Fewer workers for heavy disk checks

### Interval Tuning

```json
{
  "checkIntervalSeconds": 30,
  "retentionDays": 14
}
```

**Best practices:**
- **Critical checks:** 30-60 seconds
- **Warning checks:** 300-600 seconds
- **System checks:** 60-120 seconds

### Retention Tuning

```json
{
  "retentionDays": 30,
  "cleanupIntervalHours": 24
}
```

**Storage recommendations:**
- **Development:** 1-3 days
- **Production:** 7-30 days
- **Compliance:** 90+ days

### MongoDB Storage Considerations

```json
{
  "MONGODB_URI": "mongodb://localhost:27017/healthops",
  "MONGODB_DATABASE": "healthops",
  "MONGODB_COLLECTION_PREFIX": "healthops"
}
```

**Performance tips:**
- Use connection pooling
- Index check IDs and timestamps
- Consider sharding for large deployments
- Set appropriate read preferences

## 8. Security

### Authentication Setup

```json
{
  "auth": {
    "enabled": true,
    "username": "admin",
    "password": "securepassword123"
  }
}
```

### Environment Variables for Auth

```bash
export AUTH_USERNAME=admin
export AUTH_PASSWORD=securepassword123
```

### Command Checks Security

```json
{
  "allowCommandChecks": false
}
```

**Security warnings:**
- Command checks execute arbitrary shell commands
- Only enable for trusted configurations
- Review all command checks regularly
- Use sudo restrictions if possible

### TLS Configuration

```json
{
  "server": {
    "addr": ":8443",
    "tls": {
      "certFile": "cert/server.crt",
      "keyFile": "cert/server.key"
    }
  }
}
```

### Firewall Recommendations

```bash
# Allow HTTP access
sudo ufw allow 8080/tcp

# Allow HTTPS access
sudo ufw allow 8443/tcp

# Restrict access to specific IPs (optional)
sudo ufw allow from 192.168.1.0/24 to any port 8080
```

## 9. Monitoring

### Metrics Endpoint

```bash
# Access Prometheus metrics
curl http://localhost:8080/debug/vars

# Access service metrics (if configured)
curl http://localhost:8080/api/v1/metrics
```

### Key Metrics to Watch

- **`healthops_checks_total`** - Total checks executed
- **`healthops_checks_failed_total`** - Failed checks
- **`healthops_incidents_total`** - Total incidents
- **`healthops_alerts_triggered_total`** - Alerts triggered
- **`healthops_last_run_timestamp_seconds`** - Last run timestamp

### Alerting Recommendations

```json
{
  "alerting": {
    "cpu_usage_percent": {
      "threshold": 80,
      "duration": "5m"
    },
    "memory_usage_percent": {
      "threshold": 85,
      "duration": "10m"
    },
    "check_failure_rate": {
      "threshold": 10,
      "duration": "5m"
    }
  }
}
```

### Log Monitoring

```bash
# Monitor service logs
tail -f /var/log/health-monitor/service.log

# Monitor access logs
tail -f /var/log/health-monitor/access.log

# Monitor error logs
grep ERROR /var/log/health-monitor/service.log
```

## 10. Common Errors

### "Check Not Found"

```bash
# Error: Check ID not found
# Solution: Verify correct check ID
curl http://localhost:8080/api/v1/checks
```

**Fix:**
```bash
# List available checks
curl http://localhost:8080/api/v1/checks

# Use correct check ID from the list
curl http://localhost:8080/api/v1/runs \
  -d '{"checkId": "correct-check-id"}'
```

### "Unauthorized"

```bash
# Error: 401 Unauthorized
# Solution: Check authentication
curl -u admin:password http://localhost:8080/api/v1/checks
```

**Fix:**
```bash
# Enable auth in config
{
  "auth": {
    "enabled": true,
    "username": "admin",
    "password": "password123"
  }
}

# Set environment variables
export AUTH_USERNAME=admin
export AUTH_PASSWORD=password123
```

### "Command Checks Disabled"

```bash
# Error: Command checks disabled for security
# Solution: Enable command checks in config
```

**Fix:**
```json
{
  "allowCommandChecks": true,
  "commandChecks": [
    {
      "id": "safe-command",
      "name": "Safe Command",
      "type": "command",
      "command": "df -h",
      "expectedStatus": 0,
      "enabled": true
    }
  ]
}
```

### "Timeout"

```bash
# Error: Check timeout after X seconds
# Solution: Increase timeout or fix connectivity
```

**Fix:**
```json
{
  "timeoutSeconds": 30,
  "warningThresholdMs": 5000
}
```

### "Connection Refused"

```bash
# Error: connection refused
# Solution: Check target service and network
```

**Fix:**
```bash
# Test connectivity
ping target-host
nc -zv target-host port

# Check service status
systemctl status target-service
```

### "Database Connection Failed"

```bash
# Error: MongoDB connection failed
# Solution: Check MongoDB configuration
```

**Fix:**
```bash
# Test MongoDB connection
mongosh --uri="$MONGODB_URI"

# Check MongoDB status
sudo systemctl status mongod
```

### "File Not Found"

```bash
# Error: Log file not found
# Solution: Verify file path and permissions
```

**Fix:**
```bash
# Check file existence
ls -la /path/to/log/file.log

# Check permissions
ls -ld /path/to/log/

# Fix permissions
sudo chmod 644 /path/to/log/file.log
```

### "Invalid Configuration"

```bash
# Error: Invalid configuration file
# Solution: Validate JSON syntax
```

**Fix:**
```bash
# Validate JSON syntax
jq . config/default.json

# Check configuration
go run ./cmd/healthops --validate-config
```

## Emergency Procedures

### Service Won't Start

```bash
# Check for existing process
pkill healthops

# Check port availability
lsof -i :8080

# Check MongoDB connectivity
mongosh "$MONGODB_URI" --eval 'db.adminCommand({ping: 1})'
```

### MongoDB Data Corruption

```bash
# Restore the last known-good MongoDB dump
CONFIRM_RESTORE="$MONGODB_DATABASE" \
  scripts/healthops-mongo-restore.sh backup/last-good/healthops-mongo-healthops-20260509T120000Z.archive.gz
```

### High CPU Usage

```bash
# Reduce worker count
# Lower check intervals
# Enable rate limiting
```

### Memory Issues

```bash
# Reduce retention period
# Implement result pruning
# Review MongoDB indexes and query plans
```

## Support

For emergency issues, contact the operations team:
- **Email:** ops@company.com
- **Slack:** #health-monitor-alerts
- **PagerDuty:** Health Monitoring Service

## Related Documentation

- [Architecture Overview](../docs/ARCHITECTURE.md)
- [API Reference](../docs/API.md)
- [Configuration Guide](../docs/CONFIGURATION.md)
- [Troubleshooting Guide](../docs/TROUBLESHOOTING.md)

---

## Production Hardening Checks (Pre-Deploy)

Run this checklist before every deploy to a production-facing environment.
Mirrors and extends [`backend/docs/security-audit.md`](../backend/docs/security-audit.md).

- [ ] `HEALTHOPS_REQUIRE_PROD_AUTH=true` is set in the service environment.
      The process refuses to start otherwise if defaults are detected.
- [ ] `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` is set to a strong, unique
      password for the first start, then **removed from the environment file
      after the admin password has been rotated through the UI**.
- [ ] `MONGODB_URI`, `MONGODB_DATABASE`, and `MONGODB_COLLECTION_PREFIX` are
      set and point at the production MongoDB deployment.
- [ ] `HEALTHOPS_JWT_SECRET` and `HEALTHOPS_AI_ENCRYPTION_KEY` are set from
      the deployment secret manager and included in encrypted secret backups.
- [ ] `allowCommandChecks` is `false` in `config/default.json` and not enabled
      via the API. Shell-command checks are an RCE surface.
- [ ] The service is fronted by a TLS-terminating reverse proxy. The Go
      process binds to `127.0.0.1` (binary install) or to a private docker
      network (Compose). Port 8080 is NOT exposed to the public internet.
      Reverse-proxy headers in place: HSTS, `X-Frame-Options: DENY`,
      `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin`.
      See [deployment.md §4.4](deployment.md).
- [ ] MongoDB backups and deployment-secret backups are scheduled and a test
      restore has been rehearsed — see [backups.md](backups.md).
- [ ] Login rate limit is in place (`/api/v1/auth/login` is wrapped by a
      per-IP 5 req/min limiter on top of the global 100 req/min limit).
      Verified with: `for i in 1 2 3 4 5 6; do curl -i .../api/v1/auth/login -d '{}' -H 'Content-Type: application/json'; done`
      — request 6 must return `429`.
- [ ] JWT signing secret (`HEALTHOPS_JWT_SECRET`) and AI encryption key
      (`HEALTHOPS_AI_ENCRYPTION_KEY`) rotation cadence is documented and scheduled.
      See [`backend/docs/ai-key-rotation.md`](../backend/docs/ai-key-rotation.md).
- [ ] Audit log destination is verified. The Mongo audit collection or API
      export is being shipped to long-term storage.
- [ ] Prometheus scrape is configured against `/metrics` and the burn-rate
      alerts in [slo.md §3](slo.md) are loaded into your alerting platform.
- [ ] Smoke tests in [deployment.md §7](deployment.md) pass against the
      public URL.
- [ ] Rollback plan is rehearsed: previous binary/image is on disk, last
      known-good MongoDB backup and secret snapshot are restorable per
      [backups.md §4](backups.md).

---

## Common Production Incidents

Five short playbooks for the most likely production failures.

### A. `/healthz` returning 503

**Symptoms.** Liveness probes failing, reverse proxy returns 502/503,
`up{job="healthops"} == 0` in Prometheus.

**First steps:**
```bash
sudo systemctl status healthops
journalctl -u healthops -n 200 --no-pager
# or: docker compose logs --tail 200 healthops
```

**Common causes and fixes:**
1. **Service crashed on startup due to prod-mode gate.** Logs contain
   `HEALTHOPS_REQUIRE_PROD_AUTH=true: refusing to start ...`. Fix the named
   condition (rotate default password, set `allowCommandChecks=false`, set
   `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD`) and restart.
2. **MongoDB unavailable or full.** Check MongoDB disk and health with
   `mongosh "$MONGODB_URI" --eval 'db.adminCommand({ping: 1})'` and the
   MongoDB host's disk metrics. Restore MongoDB service before restarting
   HealthOps.
3. **Port already bound.** `sudo lsof -i :8080`. Kill the stray process or
   change the bind in config.
4. **Bad config seed.** Only relevant before MongoDB contains initialized
   state. `jq . /etc/healthops/config.json` to validate.

**Verify recovery:**
```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
```

### B. MongoDB unreachable

**Symptoms.** Logs contain MongoDB connection, ping, or repository errors;
the dashboard may fail to load or return stale/incomplete data. Treat this as
a primary storage incident.

**First steps:**
```bash
# Is Mongo up?
mongosh "$MONGODB_URI" --eval 'db.adminCommand({ping: 1})'

# Does the API still respond?
curl -fsS http://127.0.0.1:8080/api/v1/summary -H "Authorization: Bearer $TOKEN"
```

**Action:**
- If Mongo is down for a known reason (planned restart, network
  maintenance), keep HealthOps in maintenance mode until MongoDB is healthy.
- If Mongo is down unexpectedly, restore MongoDB service or fail over to the
  standby MongoDB deployment, then restart HealthOps if repository errors
  persist.
- Do not unset `MONGODB_URI`; MongoDB is required.

Restore MongoDB from the latest HealthOps `mongodump` only when the primary
database is lost or corrupted.

### C. MongoDB state corruption

**Symptoms.** Service starts but returns inconsistent data (missing checks,
wrong incident state, broken user records) or MongoDB reports collection/index
corruption.

**First steps:**
```bash
sudo systemctl stop healthops
# Inspect MongoDB metadata and recent documents
mongosh "$MONGODB_URI" --eval 'db.adminCommand({ping: 1})'
mongosh "$MONGODB_URI/$MONGODB_DATABASE" --eval 'db.getCollectionNames()'
```

**Action:**
1. Snapshot the current MongoDB database for forensic analysis.
2. Restore MongoDB from the most recent known-good dump per
   [backups.md §4](backups.md).
3. Start the service and run smoke tests from [deployment-guide.md](deployment-guide.md).
4. File a follow-up to investigate the host crash / OOM / disk fault that
   caused the corruption.

### D. Login rate-limit triggered for legitimate user

**Symptoms.** A real operator reports `429 Too Many Requests` from
`/api/v1/auth/login`. The `/api/v1/auth/login` route is rate-limited at
5 req/min per IP on top of the global 100 req/min limit.

**Triage:**
```bash
# Check audit + access logs for the source IP
curl -fsS http://127.0.0.1:8080/api/v1/audit -H "Authorization: Bearer $TOKEN" \
  | jq '.data[] | select(.action == "auth.login")' | tail -20
sudo tail -200 /var/log/nginx/access.log | grep '/auth/login'
```

**Possible causes and actions:**
1. **User typed the password wrong 5+ times.** Wait 60 seconds and try
   again. No service action required.
2. **NAT / shared IP** — many users behind the same egress IP exhausting the
   per-IP limit. Confirm via the access log. If sustained, the operator
   should authenticate from a distinct network, or you may temporarily
   relax the limit (requires a code change — do not do this casually).
3. **Brute-force attempt against a real account.** The audit log shows many
   failed `auth.login` events for the same username from the offending IP.
   Block the IP at the reverse proxy or upstream firewall, force-rotate the
   targeted account's password, review for credential reuse.

The limiter is in-process and resets on service restart; do not restart
just to clear it for one user — the hostile traffic returns immediately.

### E. AI provider failures

**Symptoms.** AI result records show growing `failure` entries; the provider
health endpoint reports degraded; users report missing AI summaries on
incidents.

**First steps:**
```bash
# Check provider health
curl -fsS http://127.0.0.1:8080/api/v1/ai/health -H "Authorization: Bearer $TOKEN"

# Inspect recent results through the API or MongoDB analytics collection
curl -fsS http://127.0.0.1:8080/api/v1/ai/results -H "Authorization: Bearer $TOKEN" \
  | jq '.data[] | {provider, status, error}'

# Inspect the queue depth
curl -fsS http://127.0.0.1:8080/api/v1/ai/queue -H "Authorization: Bearer $TOKEN" | jq
```

**Common causes and fixes:**
1. **Expired or revoked API key.** Provider returns 401/403. Update the key
   via the AI config UI; the key is re-encrypted with
   `HEALTHOPS_AI_ENCRYPTION_KEY` on save. Do not edit MongoDB documents by
   hand — stored provider keys are encrypted.
2. **Rate-limit / quota exhausted.** Provider returns 429. The service
   retries with backoff and falls back to the next configured provider.
   If no fallback is set, configure one. Otherwise wait out the quota window
   or upgrade the plan.
3. **Network egress blocked.** Curl the provider endpoint from the host;
   if it fails, fix the egress firewall.
4. **Queue backed up.** A long backlog usually follows a provider outage.
   The background worker drains it once the provider returns. If the backlog
   is no longer relevant, clear it through the supported API/admin workflow
   for your deployment; do not edit MongoDB queue documents by hand.

AI is a non-critical augmentation. None of these incidents should page
on-call unless they coincide with another incident. Route as tickets.

---

## Related Documentation

- [Production deployment guide](deployment.md)
- [Service Level Objectives](slo.md)
- [Backups and disaster recovery](backups.md)
- [Security audit](../backend/docs/security-audit.md)
- [AI key rotation](../backend/docs/ai-key-rotation.md)
