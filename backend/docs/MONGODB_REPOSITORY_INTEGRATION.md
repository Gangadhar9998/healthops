# MongoDB Repository Integration

HealthOps now treats MongoDB as the required authoritative persistence layer.
Startup fails when MongoDB or required secrets are missing; the application does
not fall back to JSON, JSONL, or local data files.

## Collections

The configured `MONGODB_COLLECTION_PREFIX` is prepended to all collections.

| Domain | Collection |
| --- | --- |
| Checks | `{prefix}_checks` |
| Results | `{prefix}_results` |
| Dashboard snapshot | `{prefix}_dashboard` |
| State metadata | `{prefix}_state` |
| Users | `{prefix}_users` |
| Servers | `{prefix}_servers` |
| Alert rules | `{prefix}_alert_rules` |
| Audit events | `{prefix}_audit_events` |
| Incidents | `{prefix}_incidents` |
| Incident snapshots | `{prefix}_incident_snapshots` |
| Notification channels | `{prefix}_notification_channels` |
| Notification outbox | `{prefix}_notification_outbox` |
| AI providers | `{prefix}_ai_config` |
| AI queue/results | `{prefix}_ai_queue`, `{prefix}_ai_results` |
| MySQL telemetry | `{prefix}_mysql_samples`, `{prefix}_mysql_deltas` |
| MySQL rule state | `{prefix}_mysql_rule_states` |
| Server metrics | `{prefix}_server_metrics` |

## Runtime Wiring

`backend/cmd/healthops/main.go` creates a Mongo client at startup, verifies it
with `Ping`, and wires Mongo-backed repositories into the service, runner,
notification dispatcher, AI service, and retention job.

Required environment:

| Variable | Required |
| --- | --- |
| `MONGODB_URI` | Yes |
| `MONGODB_DATABASE` | Yes |
| `MONGODB_COLLECTION_PREFIX` | Yes |
| `HEALTHOPS_JWT_SECRET` | Yes, at least 32 bytes |
| `HEALTHOPS_AI_ENCRYPTION_KEY` | Yes, at least 32 bytes |
| `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` | Yes |

`CONFIG_PATH` is still supported as a first-run seed for checks and process
configuration. Once MongoDB contains checks, MongoDB is authoritative.
