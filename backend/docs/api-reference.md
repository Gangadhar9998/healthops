# HealthOps API Reference

> **Version**: v1  
> **Base URL**: `http://localhost:8080`  
> **Content-Type**: `application/json` (unless noted)

---

## Table of Contents

- [Authentication](#authentication)
- [Response Envelope](#response-envelope)
- [Enums & Constants](#enums--constants)
- [Health & Readiness](#health--readiness)
- [Checks](#checks)
- [Runs](#runs)
- [Summary & Results](#summary--results)
- [Dashboard](#dashboard)
- [Incidents](#incidents)
- [Audit Log](#audit-log)
- [Analytics](#analytics)
- [Stats](#stats)
- [Configuration](#configuration)
- [Alert Rules](#alert-rules)
- [Server-Sent Events](#server-sent-events)
- [Auth Info](#auth-info)
- [MySQL Monitoring](#mysql-monitoring)
- [Notifications](#notifications)
- [AI Queue](#ai-queue)
- [BYOK AI Service](#byok-ai-service)
- [Data Export](#data-export)
- [Prometheus Metrics](#prometheus-metrics)

---

## Authentication

| Property | Value |
|----------|-------|
| **Type** | HTTP Basic Auth |
| **Header** | `Authorization: Basic base64(username:password)` |
| **Read endpoints (GET)** | No auth required |
| **Write endpoints (POST/PUT/PATCH/DELETE)** | Auth required when `auth.enabled = true` |
| **401 Response Header** | `WWW-Authenticate: Basic realm="HealthMon"` |

---

## Response Envelope

Every API response follows this envelope:

### Success

```json
{
  "success": true,
  "data": <any>
}
```

### Error

```json
{
  "success": false,
  "error": {
    "code": 400,
    "message": "human-readable error message"
  }
}
```

### Paginated

```json
{
  "success": true,
  "data": {
    "items": [...],
    "total": 100,
    "limit": 50,
    "offset": 0
  }
}
```

---

## Enums & Constants

### Check Types

| Value | Description |
|-------|-------------|
| `api` | HTTP endpoint health check |
| `tcp` | TCP port connectivity check |
| `process` | OS process existence check |
| `command` | Shell command execution check |
| `log` | Log file freshness check |
| `mysql` | MySQL database monitoring |

### Check Result Status

| Value | Description |
|-------|-------------|
| `healthy` | Check passed successfully |
| `warning` | Check passed but exceeded warning threshold |
| `critical` | Check failed |
| `unknown` | Check has not run yet |

### Incident Status

| Value | Description |
|-------|-------------|
| `open` | New incident, not yet acknowledged |
| `acknowledged` | Incident has been seen by an operator |
| `resolved` | Incident has been resolved |

### Severity

| Value | Description |
|-------|-------------|
| `warning` | Degraded but functional |
| `critical` | Service down or severely impacted |

### Notification Status

| Value | Description |
|-------|-------------|
| `pending` | Awaiting delivery |
| `sent` | Successfully delivered |
| `failed` | Delivery failed |

### AI Queue Status

| Value | Description |
|-------|-------------|
| `pending` | Awaiting analysis |
| `processing` | Currently being analyzed |
| `completed` | Analysis complete |
| `failed` | Analysis failed |

### Alert Operators

| Value | Description |
|-------|-------------|
| `equals` | Exact match |
| `not_equals` | Not equal |
| `greater_than` | Greater than |
| `less_than` | Less than |

### Period Values

| Value | Duration |
|-------|----------|
| `24h` | Last 24 hours (default) |
| `7d` | Last 7 days |
| `30d` | Last 30 days |
| `90d` | Last 90 days |

### Interval Values

| Value | Description |
|-------|-------------|
| `1h` | 1-hour buckets (default) |
| `6h` | 6-hour buckets |
| `1d` | 1-day buckets |

---

## Health & Readiness

### GET /healthz

Liveness probe.

**Auth**: No

**Response** `200 OK`

```json
{
  "success": true,
  "data": {
    "status": "ok"
  }
}
```

---

### GET /readyz

Readiness probe with service status.

**Auth**: No

**Response** `200 OK`

```json
{
  "success": true,
  "data": {
    "status": "ready",
    "checks": 12,
    "lastRunAt": "2026-04-19T10:30:00Z"
  }
}
```

---

## Checks

### GET /api/v1/checks

List all configured checks (lightweight view).

**Auth**: No

**Response** `200 OK` — `data`: `CheckListItem[]`

```json
{
  "success": true,
  "data": [
    {
      "id": "api-health",
      "name": "API Health Check",
      "type": "api",
      "server": "prod-1",
      "application": "backend",
      "enabled": true,
      "tags": ["critical", "production"]
    }
  ]
}
```

---

### POST /api/v1/checks

Create a new check.

**Auth**: Yes

**Request Body** — `CheckConfig`

```json
{
  "name": "MySQL Production",
  "type": "mysql",
  "server": "db-prod-1",
  "application": "database",
  "enabled": true,
  "tags": ["production", "database"],
  "mysql": {
    "dsnEnv": "MYSQL_PROD_DSN",
    "connectTimeoutSeconds": 3,
    "queryTimeoutSeconds": 5,
    "processlistLimit": 50,
    "statementLimit": 20,
    "hostUserLimit": 20
  }
}
```

<details>
<summary><strong>Full CheckConfig Schema</strong></summary>

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `id` | string | No | auto-generated | Unique identifier (pattern: `^[a-z0-9-]+$`) |
| `name` | string | **Yes** | — | Human-readable name |
| `type` | string | **Yes** | — | Check type: `api\|tcp\|process\|command\|log\|mysql` |
| `server` | string | No | `"default"` | Server grouping |
| `application` | string | No | — | Application grouping |
| `target` | string | No | — | Target URL (api) or process name (process) |
| `host` | string | No | — | Hostname for tcp checks |
| `port` | int | No | — | Port number for tcp checks |
| `command` | string | No | — | Shell command for command checks |
| `path` | string | No | — | File path for log checks |
| `expectedStatus` | int | No | `200` | Expected HTTP status code |
| `expectedContains` | string | No | — | Expected substring in response body |
| `timeoutSeconds` | int | No | `5` | Check execution timeout |
| `warningThresholdMs` | int | No | — | Response time warning threshold |
| `freshnessSeconds` | int | No | — | Log file freshness threshold |
| `intervalSeconds` | int | No | — | Override global check interval (min: 10) |
| `retryCount` | int | No | `0` | Number of retries on failure |
| `retryDelaySeconds` | int | No | — | Delay between retries (required if retryCount > 0) |
| `cooldownSeconds` | int | No | `0` | Alert cooldown period |
| `enabled` | bool | No | `true` | Whether check is active |
| `tags` | string[] | No | — | Arbitrary tags |
| `metadata` | object | No | — | Key-value metadata |
| `mysql` | object | No | — | MySQL-specific config (required when type=mysql) |

**MySQL Config (`mysql` field)**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `dsnEnv` | string | **Yes** | — | Environment variable name containing DSN |
| `connectTimeoutSeconds` | int | No | `3` | Connection timeout |
| `queryTimeoutSeconds` | int | No | `5` | Query timeout |
| `processlistLimit` | int | No | `50` | Max processlist rows |
| `statementLimit` | int | No | `20` | Max statement analysis rows |
| `hostUserLimit` | int | No | `20` | Max host/user summary rows |

**Validation Rules by Type**

| Type | Required Fields | Extra Rules |
|------|-----------------|-------------|
| `api` | `target` | — |
| `tcp` | `port > 0`, `host` or `target` | — |
| `process` | `target` | — |
| `command` | `command` | `allowCommandChecks` must be `true` in config |
| `log` | `path`, `freshnessSeconds > 0` | — |
| `mysql` | `mysql` block with `dsnEnv` | Defaults: `intervalSeconds=15`, `timeoutSeconds=10` |

</details>

**Response** `201 Created` — `data`: saved `CheckConfig`

---

### GET /api/v1/checks/{id}

Get detailed view of a single check with uptime, recent results, and open incidents.

**Auth**: No

**Response** `200 OK` — `data`: `CheckDetail`

```json
{
  "success": true,
  "data": {
    "config": { /* full CheckConfig */ },
    "latestResult": {
      "id": "result-123",
      "checkId": "api-health",
      "status": "healthy",
      "healthy": true,
      "durationMs": 45,
      "startedAt": "2026-04-19T10:30:00Z",
      "finishedAt": "2026-04-19T10:30:00Z"
    },
    "uptime24h": 99.8,
    "uptime7d": 99.5,
    "avgDurationMs": 42.3,
    "recentResults": [ /* up to 50 CheckResults, newest first */ ],
    "openIncidents": [ /* Incident[] if any */ ]
  }
}
```

**Status Codes**: `200`, `404` (check not found)

---

### PUT /api/v1/checks/{id}

Update an existing check.

**Auth**: Yes  
**Request Body**: `CheckConfig` (path `id` overrides body `id`)  
**Response** `200 OK` — `data`: updated `CheckConfig`

---

### DELETE /api/v1/checks/{id}

Delete a check and remove from scheduler.

**Auth**: Yes  
**Response**: `204 No Content`

---

## Runs

### POST /api/v1/runs

Trigger an immediate check run.

**Auth**: Yes

**Request Body** (optional)

```json
{ "checkId": "api-health" }
```

- If `checkId` provided: runs single check → returns `CheckResult`
- If omitted: runs all checks → returns `RunSummary`

**Response** `202 Accepted`

<details>
<summary><strong>Single Check Response</strong></summary>

```json
{
  "success": true,
  "data": {
    "id": "result-abc",
    "checkId": "api-health",
    "name": "API Health Check",
    "type": "api",
    "server": "prod-1",
    "status": "healthy",
    "healthy": true,
    "message": "",
    "durationMs": 45,
    "startedAt": "2026-04-19T10:30:00Z",
    "finishedAt": "2026-04-19T10:30:00Z",
    "metrics": {
      "statusCode": 200,
      "bodyLength": 1234
    },
    "tags": ["production"]
  }
}
```

</details>

<details>
<summary><strong>Full Run Response — RunSummary</strong></summary>

```json
{
  "success": true,
  "data": {
    "startedAt": "2026-04-19T10:30:00Z",
    "finishedAt": "2026-04-19T10:30:02Z",
    "results": [ /* CheckResult[] */ ],
    "summary": {
      "totalChecks": 12,
      "enabledChecks": 10,
      "healthy": 8,
      "warning": 1,
      "critical": 1,
      "unknown": 0,
      "lastRunAt": "2026-04-19T10:30:02Z",
      "byServer": {
        "prod-1": { "total": 5, "healthy": 4, "warning": 1, "critical": 0, "unknown": 0 }
      },
      "byApplication": {
        "backend": { "total": 3, "healthy": 3, "warning": 0, "critical": 0, "unknown": 0 }
      },
      "latest": [ /* CheckResult[] */ ]
    }
  }
}
```

</details>

---

## Summary & Results

### GET /api/v1/summary

Get current health summary.

**Auth**: No

**Response** `200 OK` — `data`: `Summary`

```json
{
  "success": true,
  "data": {
    "totalChecks": 12,
    "enabledChecks": 10,
    "healthy": 8,
    "warning": 1,
    "critical": 1,
    "unknown": 2,
    "lastRunAt": "2026-04-19T10:30:02Z",
    "byServer": {
      "prod-1": { "total": 5, "healthy": 4, "warning": 1, "critical": 0, "unknown": 0 }
    },
    "byApplication": {
      "backend": { "total": 3, "healthy": 3, "warning": 0, "critical": 0, "unknown": 0 }
    },
    "latest": [ /* CheckResult per check */ ]
  }
}
```

---

### GET /api/v1/results

Get historical check results.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | No | all | Filter by check ID |
| `days` | int | No | `retentionDays` (7) | Time window |

**Response** `200 OK` — `data`: `CheckResult[]`

---

## Dashboard

Optimized read-only endpoints that prefer MongoDB snapshot when available.

### GET /api/v1/dashboard/checks

**Auth**: No — **Response**: `CheckListItem[]`

### GET /api/v1/dashboard/summary

**Auth**: No — **Response**: `Summary`

### GET /api/v1/dashboard/results

**Auth**: No  
**Query Params**: `checkId`, `days` (same as `/api/v1/results`)  
**Response**: `CheckResult[]`

---

## Incidents

### GET /api/v1/incidents

List incidents with filtering and pagination.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `status` | string | No | all | `open\|acknowledged\|resolved` |
| `severity` | string | No | all | `warning\|critical` |
| `checkId` | string | No | all | Filter by check ID |
| `limit` | int | No | `50` | Page size |
| `offset` | int | No | `0` | Page offset |

**Response** `200 OK` — **Paginated**

```json
{
  "success": true,
  "data": {
    "items": [
      {
        "id": "api-health-1713520200",
        "checkId": "api-health",
        "checkName": "API Health Check",
        "type": "api",
        "status": "open",
        "severity": "critical",
        "message": "HTTP 503 Service Unavailable",
        "startedAt": "2026-04-19T10:30:00Z",
        "updatedAt": "2026-04-19T10:31:00Z",
        "resolvedAt": null,
        "acknowledgedAt": null,
        "acknowledgedBy": "",
        "resolvedBy": "",
        "metadata": {
          "ruleId": "api-status-check",
          "ruleName": "API Status Critical"
        }
      }
    ],
    "total": 15,
    "limit": 50,
    "offset": 0
  }
}
```

---

### GET /api/v1/incidents/{id}

Get a single incident.

**Auth**: No  
**Response** `200 OK` — `data`: `Incident`  
**Status Codes**: `200`, `404`, `503` (incident manager not configured)

---

### POST /api/v1/incidents/{id}/acknowledge

Acknowledge an incident.

**Auth**: Yes

**Request Body**

```json
{ "acknowledgedBy": "jane.doe" }
```

> Defaults to `"anonymous"` if empty.

**Response** `200 OK` — `data`: updated `Incident` with `status: "acknowledged"`, `acknowledgedAt`, `acknowledgedBy` set

---

### POST /api/v1/incidents/{id}/resolve

Resolve an incident.

**Auth**: Yes

**Request Body**

```json
{ "resolvedBy": "jane.doe" }
```

> Defaults to `"anonymous"` if empty.

**Response** `200 OK` — `data`: updated `Incident` with `status: "resolved"`, `resolvedAt`, `resolvedBy` set

---

### GET /api/v1/incidents/{id}/snapshots

Get evidence snapshots captured at incident creation.

**Auth**: No

**Response** `200 OK` — `data`: `IncidentSnapshot[]`

```json
{
  "success": true,
  "data": [
    {
      "incidentId": "db-prod-1713520200",
      "snapshotType": "latest_sample",
      "timestamp": "2026-04-19T10:30:00Z",
      "payloadJson": "{\"connections\":950,\"maxConnections\":1000,...}"
    },
    {
      "incidentId": "db-prod-1713520200",
      "snapshotType": "processlist",
      "timestamp": "2026-04-19T10:30:00Z",
      "payloadJson": "[{\"ID\":1,\"USER\":\"root\",...}]"
    }
  ]
}
```

**Snapshot Types**: `latest_sample`, `recent_deltas`, `processlist`, `statement_analysis`, `host_summary`, `user_summary`, `host_cache`

---

## Audit Log

### GET /api/v1/audit

Query audit events.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `action` | string | No | all | e.g. `check.created`, `incident.resolved` |
| `actor` | string | No | all | Username |
| `target` | string | No | all | Target type (e.g. `check`, `incident`) |
| `targetId` | string | No | all | Target ID |
| `startTime` | RFC3339 | No | — | Start of time range |
| `endTime` | RFC3339 | No | — | End of time range |
| `limit` | int | No | `100` | Page size |
| `offset` | int | No | `0` | Page offset |

**Response** `200 OK` — `data`: `AuditEvent[]`

```json
{
  "success": true,
  "data": [
    {
      "id": "evt-abc123",
      "action": "check.created",
      "actor": "admin",
      "target": "check",
      "targetId": "api-health",
      "details": { "name": "API Health Check", "type": "api" },
      "timestamp": "2026-04-19T10:30:00Z"
    }
  ]
}
```

**Audit Actions**: `check.created`, `check.updated`, `check.deleted`, `incident.acknowledged`, `incident.resolved`, `notification.sent`, `notification.failed`, `ai.analysis.completed`, `ai.analysis.failed`, `config.updated`, `alert_rule.created`, `alert_rule.updated`, `alert_rule.deleted`

---

## Analytics

### GET /api/v1/analytics/uptime

Get uptime statistics for checks.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | No | all enabled | Specific check |
| `period` | string | No | `24h` | `24h\|7d\|30d\|90d` |

**Response** `200 OK`

- If `checkId` provided: `data` = single `UptimeStats`
- If omitted: `data` = `UptimeStats[]` for all enabled checks

```json
{
  "checkId": "api-health",
  "checkName": "API Health Check",
  "period": "7d",
  "totalResults": 672,
  "healthyCount": 668,
  "uptimePct": 99.4,
  "avgDurationMs": 42.3,
  "maxDurationMs": 1250,
  "minDurationMs": 12
}
```

---

### GET /api/v1/analytics/response-times

Get response time distribution over time with percentiles.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | **Yes** | — | Check ID |
| `period` | string | No | `24h` | `24h\|7d\|30d\|90d` |
| `interval` | string | No | `1h` | `1h\|6h\|1d` |

**Response** `200 OK` — `data`: `ResponseTimeBucket[]`

```json
[
  {
    "timestamp": "2026-04-19T09:00:00Z",
    "avgDurationMs": 45.2,
    "p50DurationMs": 42.0,
    "p95DurationMs": 120.0,
    "p99DurationMs": 250.0,
    "maxDurationMs": 350,
    "minDurationMs": 12,
    "count": 60
  }
]
```

---

### GET /api/v1/analytics/status-timeline

Get chronological status changes for a check.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | **Yes** | — | Check ID |
| `days` | int | No | `7` | Days to look back |

**Response** `200 OK` — `data`: `StatusTimelineEntry[]`

```json
[
  {
    "timestamp": "2026-04-19T10:30:00Z",
    "status": "critical",
    "durationMs": 5023,
    "message": "HTTP 503 Service Unavailable"
  }
]
```

---

### GET /api/v1/analytics/failure-rate

Get failure rates grouped by server, application, or type.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `period` | string | No | `24h` | `24h\|7d\|30d\|90d` |
| `groupBy` | string | No | `server` | `server\|application\|type` |

**Response** `200 OK` — `data`: `FailureRateEntry[]`

```json
[
  {
    "group": "prod-1",
    "totalResults": 480,
    "failedCount": 12,
    "failureRate": 2.5
  }
]
```

---

### GET /api/v1/analytics/incidents

Get incident analytics with MTTA and MTTR.

**Auth**: No

**Response** `200 OK` — `data`: `IncidentStats`

```json
{
  "total": 45,
  "open": 3,
  "acknowledged": 2,
  "resolved": 40,
  "mttaMinutes": 4.5,
  "mttrMinutes": 32.8,
  "bySeverity": {
    "warning": 30,
    "critical": 15
  }
}
```

---

## Stats

### GET /api/v1/stats/overview

Dashboard hero card data — single call for top-level metrics.

**Auth**: No

**Response** `200 OK` — `data`: `OverviewStats`

```json
{
  "totalChecks": 12,
  "enabledChecks": 10,
  "healthyChecks": 8,
  "activeIncidents": 3,
  "avgUptimePct": 98.7,
  "checksByType": {
    "api": 5,
    "tcp": 2,
    "mysql": 2,
    "process": 1,
    "log": 1,
    "command": 1
  },
  "checksByServer": {
    "prod-1": 5,
    "prod-2": 4,
    "staging": 3
  }
}
```

---

## Configuration

### GET /api/v1/config

Get current runtime configuration (credentials are never exposed).

**Auth**: No

**Response** `200 OK` — `data`: `SafeConfigView`

```json
{
  "success": true,
  "data": {
    "server": {
      "addr": ":8080",
      "readTimeoutSeconds": 10,
      "writeTimeoutSeconds": 10,
      "idleTimeoutSeconds": 60
    },
    "authEnabled": true,
    "retentionDays": 7,
    "checkIntervalSeconds": 60,
    "workers": 8,
    "allowCommandChecks": false,
    "totalChecks": 12
  }
}
```

---

### PUT /api/v1/config

Update runtime configuration.

**Auth**: Yes

**Request Body** — `ConfigUpdate`

```json
{
  "retentionDays": 14,
  "checkIntervalSeconds": 30,
  "workers": 16,
  "allowCommandChecks": true
}
```

All fields are optional. Only provided fields are updated.

| Field | Type | Min | Max | Description |
|-------|------|-----|-----|-------------|
| `retentionDays` | int | 1 | 365 | Data retention period |
| `checkIntervalSeconds` | int | 5 | 3600 | Default check interval |
| `workers` | int | 1 | 100 | Parallel check workers |
| `allowCommandChecks` | bool | — | — | Enable shell command checks |

**Response** `200 OK` — `data`: updated `SafeConfigView`  
**Status Codes**: `200`, `400` (validation failed), `401` (unauthorized)

---

## Alert Rules

### GET /api/v1/alert-rules

List all alert rules.

**Auth**: No

**Response** `200 OK` — `data`: `AlertRule[]`

```json
{
  "success": true,
  "data": [
    {
      "id": "mysql-conn-util-crit",
      "name": "Connection Utilization Critical",
      "enabled": true,
      "checkIds": [],
      "conditions": [
        { "field": "status", "operator": "equals", "value": "critical" }
      ],
      "severity": "critical",
      "channels": [],
      "cooldownMinutes": 5,
      "description": "Connection utilization above 90%",
      "consecutiveBreaches": 2,
      "recoverySamples": 3,
      "thresholdNum": 90,
      "ruleCode": "CONN_UTIL_CRIT"
    }
  ]
}
```

<details>
<summary><strong>Full AlertRule Schema</strong></summary>

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `id` | string | **Yes** | — | Unique rule identifier |
| `name` | string | **Yes** | — | Human-readable name |
| `enabled` | bool | No | `true` | Whether rule is active |
| `checkIds` | string[] | No | `[]` (all checks) | Checks this rule applies to |
| `conditions` | AlertCondition[] | **Yes** | — | Conditions to evaluate (AND logic) |
| `severity` | string | No | `"warning"` | `warning\|critical` |
| `channels` | AlertChannel[] | No | `[]` | Notification channels |
| `cooldownMinutes` | int | No | `5` | Minimum time between alerts |
| `description` | string | No | — | Human-readable description |
| `consecutiveBreaches` | int | No | `1` | Breaches before alert fires |
| `recoverySamples` | int | No | `1` | OK samples before recovery |
| `thresholdNum` | float64 | No | `0` | Numeric threshold for rule codes |
| `ruleCode` | string | No | — | MySQL rule code identifier |

**AlertCondition**

| Field | Type | Description |
|-------|------|-------------|
| `field` | string | Field to evaluate: `status`, `healthy`, `durationMs`, or any metric key |
| `operator` | string | `equals\|not_equals\|greater_than\|less_than` |
| `value` | any | Comparison value (string, bool, or number) |

**AlertChannel**

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Channel type: `webhook`, `email`, etc. |
| `config` | object | Channel-specific configuration |

</details>

---

### POST /api/v1/alert-rules

Create a new alert rule.

**Auth**: Yes  
**Request Body**: `AlertRule`  
**Response** `201 Created` — `data`: created `AlertRule`

---

### PUT /api/v1/alert-rules/{id}

Update an existing alert rule.

**Auth**: Yes  
**Request Body**: `AlertRule` (path `id` overrides body `id`)  
**Response** `200 OK` — `data`: updated `AlertRule`  
**Status Codes**: `200`, `404`

---

### DELETE /api/v1/alert-rules/{id}

Delete an alert rule.

**Auth**: Yes

**Response** `200 OK`

```json
{
  "success": true,
  "data": { "deleted": "mysql-conn-util-crit" }
}
```

**Status Codes**: `200`, `404`

---

## Server-Sent Events

### GET /api/v1/events

Live event stream for real-time dashboard updates.

**Auth**: No  
**Response**: `text/event-stream`

**Headers**

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
Access-Control-Allow-Origin: *
```

**Event Format** (every 5 seconds)

```
event: snapshot
data: {"type":"snapshot","timestamp":"2026-04-19T10:30:00Z","summary":{"totalChecks":12,"enabledChecks":10,"healthy":8,"warning":1,"critical":1,"unknown":2,"lastRunAt":"2026-04-19T10:30:00Z","byServer":{},"byApplication":{},"latest":[]},"activeIncidents":3}

```

**SSEPayload Schema**

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"snapshot"` |
| `timestamp` | RFC3339 | Event timestamp |
| `summary` | Summary | Current health summary |
| `activeIncidents` | int | Count of non-resolved incidents |

---

## Auth Info

### GET /api/v1/auth/me

Get current authentication info.

**Auth**: No (returns info about the caller)

**Response** `200 OK` — `data`: `AuthInfo`

```json
{
  "success": true,
  "data": {
    "username": "admin",
    "authEnabled": true
  }
}
```

> Without credentials: `username` is `"unknown"` (when auth enabled) or `"system"` (when auth disabled)

---

## MySQL Monitoring

### GET /api/v1/mysql/samples

Get raw MySQL metric samples.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | **Yes** | — | MySQL check ID |
| `limit` | int | No | `20` | Max results |

**Response** `200 OK` — `data`: `MySQLSample[]`

```json
[
  {
    "sampleId": "db-prod-1713520200123456789",
    "checkId": "db-prod",
    "timestamp": "2026-04-19T10:30:00Z",
    "connections": 150,
    "maxConnections": 1000,
    "maxUsedConnections": 200,
    "threadsRunning": 5,
    "threadsConnected": 150,
    "threadsCreated": 500,
    "abortedConnects": 10,
    "abortedClients": 2,
    "slowQueries": 45,
    "questions": 1500000,
    "questionsPerSec": 250.5,
    "uptimeSeconds": 86400,
    "innodbRowLockWaits": 100,
    "innodbRowLockTime": 5000,
    "createdTmpDiskTables": 50,
    "createdTmpTables": 200,
    "connectionsRefused": 0
  }
]
```

---

### GET /api/v1/mysql/deltas

Get computed deltas between consecutive MySQL samples.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | **Yes** | — | MySQL check ID |
| `limit` | int | No | `20` | Max results |

**Response** `200 OK` — `data`: `MySQLDelta[]`

```json
[
  {
    "sampleId": "db-prod-1713520200123456789",
    "checkId": "db-prod",
    "intervalSec": 15.03,
    "timestamp": "2026-04-19T10:30:15Z",
    "abortedConnectsDelta": 0,
    "abortedConnectsPerSec": 0.0,
    "slowQueriesDelta": 2,
    "slowQueriesPerSec": 0.13,
    "questionsDelta": 3750,
    "questionsPerSec": 249.5,
    "rowLockWaitsDelta": 5,
    "rowLockWaitsPerSec": 0.33,
    "tmpDiskTablesDelta": 1,
    "tmpDiskTablesPct": 12.5,
    "threadsCreatedDelta": 3,
    "threadsCreatedPerSec": 0.2,
    "connectionsRefusedDelta": 0
  }
]
```

> Counter resets are handled with `max(0, diff)` behavior.

---

### GET /api/v1/mysql/health

Get MySQL health summary card for a check.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | **Yes** | — | MySQL check ID |

**Response** `200 OK` — `data`: `MySQLHealthSummary`

```json
{
  "checkId": "db-prod",
  "timestamp": "2026-04-19T10:30:00Z",
  "connectionUtilPct": 15.0,
  "threadsRunning": 5,
  "queriesPerSec": 249.5,
  "slowQueriesPerSec": 0.13,
  "rowLockWaitsPerSec": 0.33,
  "tmpDiskTablesPct": 12.5,
  "abortedConnectsPerSec": 0.0,
  "uptimeSeconds": 86400,
  "status": "healthy"
}
```

**Status Logic**: `connectionUtilPct > 90%` → `"critical"`, `> 70%` → `"warning"`, else `"healthy"`

---

### GET /api/v1/mysql/timeseries

Get MySQL metrics as time-series data for charting.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | **Yes** | — | MySQL check ID |
| `metric` | string | No | `all` | `all\|connections\|qps\|slow_queries\|threads\|locks` |
| `limit` | int | No | `100` | Max data points |

**Response** `200 OK` — `data`: `MySQLTimeSeriesPoint[]` (sorted ascending by timestamp)

```json
[
  {
    "timestamp": "2026-04-19T10:15:00Z",
    "connectionUtilPct": 14.5,
    "threadsRunning": 4,
    "queriesPerSec": 230.0,
    "slowQueriesPerSec": 0.1,
    "rowLockWaitsPerSec": 0.2,
    "tmpDiskTablesPct": 10.0,
    "connections": 145,
    "maxConnections": 1000
  }
]
```

**Metric Filtering**: When `metric` is not `all`, only relevant fields are populated. Others are zero/omitted.

---

## Notifications

### GET /api/v1/notifications

List pending notifications.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `limit` | int | No | `100` | Max results |

**Response** `200 OK` — `data`: `NotificationEvent[]`

```json
[
  {
    "notificationId": "notif-db-prod-1713520200-1713520200123456789",
    "incidentId": "db-prod-1713520200",
    "channel": "email",
    "payloadJson": "{\"incidentId\":\"db-prod-1713520200\",\"severity\":\"critical\",...}",
    "status": "pending",
    "retryCount": 0,
    "createdAt": "2026-04-19T10:30:00Z"
  }
]
```

---

### POST /api/v1/notifications/{id}/sent

Mark a notification as successfully sent.

**Auth**: Yes  
**Request Body**: none  
**Response** `200 OK`

```json
{ "success": true, "data": { "status": "sent" } }
```

---

### POST /api/v1/notifications/{id}/failed

Mark a notification as failed.

**Auth**: Yes

**Request Body**

```json
{ "reason": "SMTP connection timeout" }
```

**Response** `200 OK`

```json
{ "success": true, "data": { "status": "failed" } }
```

---

### GET /api/v1/notifications/stats

Get notification delivery statistics.

**Auth**: No

**Response** `200 OK` — `data`: `NotificationStats`

```json
{
  "total": 45,
  "pending": 3,
  "sent": 38,
  "failed": 4
}
```

---

## AI Queue

### GET /api/v1/ai/queue

List pending AI analysis queue items.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `limit` | int | No | `100` | Max results |

**Response** `200 OK` — `data`: `AIQueueItem[]`

```json
[
  {
    "incidentId": "db-prod-1713520200",
    "promptVersion": "v1",
    "status": "pending",
    "createdAt": "2026-04-19T10:30:00Z"
  }
]
```

---

### POST /api/v1/ai/queue/{incidentId}/done

Complete AI analysis for an incident.

**Auth**: Yes

**Request Body** — `AIAnalysisResult`

```json
{
  "analysis": "Connection pool exhaustion due to unclosed connections in the payment service.",
  "suggestions": [
    "Increase max_connections to 2000",
    "Fix connection leak in payment-svc",
    "Add connection pool monitoring"
  ],
  "severity": "critical"
}
```

**Response** `200 OK`

```json
{ "success": true, "data": { "status": "completed" } }
```

---

### POST /api/v1/ai/queue/{incidentId}/failed

Mark AI analysis as failed.

**Auth**: Yes

**Request Body**

```json
{ "reason": "OpenAI API rate limit exceeded" }
```

**Response** `200 OK`

```json
{ "success": true, "data": { "status": "failed" } }
```

---

### GET /api/v1/ai/queue/stats

Get AI queue statistics.

**Auth**: No

**Response** `200 OK` — `data`: `AIQueueStats`

```json
{
  "total": 25,
  "pending": 2,
  "processing": 1,
  "completed": 20,
  "failed": 2
}
```

---

## BYOK AI Service

Bring Your Own Key AI layer. Configure AI providers (OpenAI, Anthropic, Google Gemini, Ollama, or any OpenAI-compatible service) from the UI. API keys are AES-256-GCM encrypted at rest and always masked in API responses.

### GET /api/v1/ai/config

Get AI service configuration (API keys masked).

**Auth**: No

**Response** `200 OK` — `data`: `SafeAIConfigView`

```json
{
  "enabled": true,
  "autoAnalyze": true,
  "maxConcurrent": 2,
  "timeoutSeconds": 30,
  "retryCount": 2,
  "retryDelayMs": 1000,
  "defaultPromptId": "incident-analysis-v1",
  "providers": [
    {
      "id": "openai-prod",
      "provider": "openai",
      "name": "Production OpenAI",
      "apiKey": "sk-p...****ef12",
      "baseUrl": "",
      "model": "gpt-4o",
      "maxTokens": 4096,
      "temperature": 0.3,
      "enabled": true,
      "isDefault": true,
      "createdAt": "2026-04-19T10:00:00Z",
      "updatedAt": "2026-04-19T10:00:00Z"
    }
  ],
  "prompts": [
    {
      "id": "incident-analysis-v1",
      "name": "General Incident Analysis",
      "description": "SRE-focused root cause analysis",
      "version": "v1",
      "isDefault": true
    }
  ]
}
```

---

### PUT /api/v1/ai/config

Update AI service settings.

**Auth**: Yes

**Request Body** — partial update (all fields optional)

```json
{
  "enabled": true,
  "autoAnalyze": true,
  "maxConcurrent": 4,
  "timeoutSeconds": 60,
  "retryCount": 3,
  "retryDelayMs": 2000,
  "defaultPromptId": "mysql-analysis-v1"
}
```

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `enabled` | bool | — | Enable/disable AI analysis globally |
| `autoAnalyze` | bool | — | Auto-analyze on incident creation |
| `maxConcurrent` | int | 1–20 | Max concurrent analysis workers |
| `timeoutSeconds` | int | 5–300 | Per-provider request timeout |
| `retryCount` | int | 0–10 | Retries on failure |
| `retryDelayMs` | int | 0–60000 | Delay between retries |
| `defaultPromptId` | string | — | Default prompt template ID |

**Response** `200 OK` — updated `SafeAIConfigView`

---

### GET /api/v1/ai/providers

List all configured AI providers (API keys masked).

**Auth**: No

**Response** `200 OK` — `data`: `SafeAIProviderView[]`

---

### POST /api/v1/ai/providers

Add a new AI provider.

**Auth**: Yes

**Request Body** — `AIProviderConfig`

```json
{
  "id": "anthropic-prod",
  "provider": "anthropic",
  "name": "Production Anthropic",
  "apiKey": "sk-ant-api-key-here",
  "model": "claude-sonnet-4-20250514",
  "maxTokens": 4096,
  "temperature": 0.3,
  "enabled": true,
  "isDefault": false
}
```

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| `id` | string | Yes | unique | Provider identifier |
| `provider` | enum | Yes | `openai\|anthropic\|google\|ollama\|custom` | Provider type |
| `name` | string | Yes | — | Display name |
| `apiKey` | string | Conditional | Required for openai, anthropic, google | API key (encrypted at rest) |
| `baseUrl` | string | Conditional | Required for ollama, custom | API base URL |
| `model` | string | Yes | — | Model identifier |
| `maxTokens` | int | No | 0–128000 | Max response tokens |
| `temperature` | float | No | 0.0–2.0 | Sampling temperature |
| `enabled` | bool | No | — | Enable this provider |
| `isDefault` | bool | No | max 1 default | Use as default provider |

**Provider Types**:

| Type | API Key | Base URL | Notes |
|------|---------|----------|-------|
| `openai` | Required | Optional (default: `https://api.openai.com/v1`) | OpenAI Chat Completions API |
| `anthropic` | Required | Optional (default: `https://api.anthropic.com/v1`) | Anthropic Messages API |
| `google` | Required | Optional (default: `https://generativelanguage.googleapis.com/v1beta`) | Google Gemini API |
| `ollama` | Not needed | Required (default: `http://localhost:11434`) | Local Ollama instance |
| `custom` | Optional | Required | Any OpenAI-compatible API |

**Response** `201 Created` — `SafeAIProviderView` (API key masked)

---

### PUT /api/v1/ai/providers/{id}

Update an existing provider. If `apiKey` is empty or contains `****`, the existing key is preserved.

**Auth**: Yes

**Response** `200 OK` — updated `SafeAIProviderView`

---

### DELETE /api/v1/ai/providers/{id}

Remove a provider.

**Auth**: Yes

**Response** `200 OK`

```json
{ "success": true, "data": { "deleted": "anthropic-prod" } }
```

---

### GET /api/v1/ai/prompts

List all prompt templates (including defaults).

**Auth**: No

**Response** `200 OK` — `data`: `AIPromptTemplate[]`

```json
[
  {
    "id": "incident-analysis-v1",
    "name": "General Incident Analysis",
    "description": "SRE-focused root cause analysis with structured JSON output",
    "systemMsg": "You are a senior SRE engineer...",
    "userMsg": "Analyze this incident: {{.CheckName}} ...",
    "version": "v1",
    "isDefault": true
  },
  {
    "id": "mysql-analysis-v1",
    "name": "MySQL Analysis",
    "description": "MySQL DBA analysis with connection, query, lock, capacity analysis",
    "systemMsg": "You are a senior MySQL DBA...",
    "userMsg": "MySQL check '{{.CheckName}}' triggered...",
    "version": "v1",
    "isDefault": false
  }
]
```

**Template Variables** (available in `systemMsg` and `userMsg`):

| Variable | Description |
|----------|-------------|
| `{{.IncidentID}}` | Incident ID |
| `{{.CheckName}}` | Check name |
| `{{.CheckType}}` | Check type (api, tcp, mysql, etc.) |
| `{{.Severity}}` | Incident severity |
| `{{.Message}}` | Alert message |
| `{{.StartedAt}}` | Incident start time (RFC3339) |
| `{{.Duration}}` | Time since incident started |
| `{{.Evidence}}` | Collected evidence snapshots |
| `{{.RecentResults}}` | Last 10 check results (JSON) |
| `{{.RuleCode}}` | Alert rule code (from metadata) |
| `{{.LatestSample}}` | Latest MySQL sample (if available) |
| `{{.RecentDeltas}}` | Recent MySQL deltas (if available) |
| `{{.ProcessList}}` | MySQL process list (if available) |
| `{{.StatementAnalysis}}` | MySQL statement analysis (if available) |

---

### POST /api/v1/ai/prompts

Create a custom prompt template.

**Auth**: Yes

**Request Body** — `AIPromptTemplate`

```json
{
  "id": "custom-redis-v1",
  "name": "Redis Analysis",
  "description": "Redis-specific incident analysis",
  "systemMsg": "You are a Redis expert...",
  "userMsg": "Redis check '{{.CheckName}}' fired: {{.Message}}",
  "version": "v1",
  "isDefault": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique template ID |
| `name` | string | Yes | Display name |
| `description` | string | No | Template description |
| `systemMsg` | string | No | System prompt (Go template) |
| `userMsg` | string | Yes | User prompt (Go template) |
| `version` | string | No | Version tag |
| `isDefault` | bool | No | Set as default (unsets others) |

**Response** `201 Created` — `AIPromptTemplate`

---

### PUT /api/v1/ai/prompts/{id}

Update a prompt template.

**Auth**: Yes

**Response** `200 OK` — updated `AIPromptTemplate`

---

### DELETE /api/v1/ai/prompts/{id}

Delete a prompt template.

**Auth**: Yes

**Response** `200 OK`

```json
{ "success": true, "data": { "deleted": "custom-redis-v1" } }
```

---

### POST /api/v1/ai/analyze/{incidentId}

Manually trigger AI analysis for a specific incident.

**Auth**: Yes

**Request Body** (optional)

```json
{
  "providerId": "anthropic-prod",
  "promptId": "mysql-analysis-v1"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `providerId` | string | No | Use specific provider (default: default provider) |
| `promptId` | string | No | Use specific prompt template |

**Response** `200 OK` — `data`: `AIAnalysisResult`

```json
{
  "incidentId": "db-prod-1713520200",
  "analysis": "**Root Cause**: Connection pool exhaustion...\n\n**Impact**: Service degradation...",
  "suggestions": [
    "Increase max_connections to 2000",
    "Fix connection leak in payment-svc"
  ],
  "severity": "critical",
  "createdAt": "2026-04-19T10:35:00Z"
}
```

---

### GET /api/v1/ai/health

Check connectivity to all configured AI providers.

**Auth**: No

**Response** `200 OK` — `data`: `ProviderHealth[]`

```json
[
  {
    "id": "openai-prod",
    "provider": "openai",
    "model": "gpt-4o",
    "healthy": true,
    "isDefault": true
  },
  {
    "id": "ollama-local",
    "provider": "ollama",
    "model": "llama3",
    "healthy": false,
    "isDefault": false
  }
]
```

---

### GET /api/v1/ai/results/{incidentId}

Get AI analysis results for a specific incident.

**Auth**: No

**Response** `200 OK` — `data`: `AIAnalysisResult[]`

```json
[
  {
    "incidentId": "db-prod-1713520200",
    "analysis": "**Root Cause**: Memory leak in connection pool...",
    "suggestions": ["Restart service", "Tune pool size"],
    "severity": "critical",
    "createdAt": "2026-04-19T10:35:00Z"
  }
]
```

---

## Data Export

### GET /api/v1/export/mysql/samples

Export MySQL samples as CSV or JSON.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | **Yes** | — | MySQL check ID |
| `limit` | int | No | `100` | Max rows |
| `format` | string | No | `json` | `csv\|json` |

**CSV Response**: `Content-Type: text/csv`, `Content-Disposition: attachment; filename=mysql_samples.csv`

CSV columns: `sampleId, checkId, timestamp, connections, maxConnections, threadsRunning, threadsConnected, slowQueries, questionsPerSec, uptimeSeconds`

**JSON Response**: Same as `GET /api/v1/mysql/samples`

---

### GET /api/v1/export/incidents

Export all incidents as CSV or JSON.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `format` | string | No | `json` | `csv\|json` |

**CSV columns**: `id, checkId, checkName, type, status, severity, message, startedAt, resolvedAt`

---

### GET /api/v1/export/results

Export check results as CSV or JSON.

**Auth**: No

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkId` | string | No | all | Filter by check |
| `days` | int | No | `retentionDays` (7) | Time window |
| `format` | string | No | `json` | `csv\|json` |

**CSV columns**: `id, checkId, name, type, server, application, status, healthy, durationMs, startedAt, finishedAt, message`

---

## Prometheus Metrics

### GET /metrics

Prometheus metrics endpoint.

**Auth**: No  
**Response**: `text/plain` (Prometheus exposition format)

**Available Metrics**

| Metric | Type | Description |
|--------|------|-------------|
| `healthmon_check_runs_total` | counter | Total check runs by check ID and status |
| `healthmon_check_duration_seconds` | histogram | Check execution duration |
| `healthmon_check_failures_total` | counter | Total check failures |
| `healthmon_incidents_total` | counter | Total incidents by severity |
| `healthmon_alerts_triggered_total` | counter | Total alerts triggered |
| `healthmon_http_requests_total` | counter | HTTP requests by method, path, status |
| `healthmon_http_request_duration_seconds` | histogram | HTTP request duration |

---

## Default MySQL Alert Rules

These rules are loaded at startup and can be managed via the Alert Rules API.

| Rule Code | Severity | Threshold | Consecutive Breaches | Recovery Samples | Cooldown |
|-----------|----------|-----------|---------------------|-----------------|----------|
| `CONN_UTIL_WARN` | warning | 70% | 3 | 3 | 15 min |
| `CONN_UTIL_CRIT` | critical | 90% | 2 | 3 | 5 min |
| `MAX_CONN_REFUSED` | critical | any > 0 | 1 | 5 | 5 min |
| `ABORTED_CONNECT_SPIKE` | warning | 5/sec | 3 | 5 | 15 min |
| `THREADS_RUNNING_HIGH` | warning | 50 | 3 | 3 | 10 min |
| `ROW_LOCK_WAITS_HIGH` | warning | 10/sec | 3 | 5 | 15 min |
| `SLOW_QUERY_SPIKE` | warning | 2/sec | 3 | 5 | 15 min |
| `TMP_DISK_PCT_HIGH` | warning | 25% | 5 | 5 | 30 min |
| `THREAD_CREATE_SPIKE` | warning | 10/sec | 3 | 5 | 15 min |

---

## Default Retention Periods

| Data Type | Default Retention |
|-----------|------------------|
| MySQL Samples | 7 days |
| MySQL Deltas | 7 days |
| Incident Snapshots | 30 days |
| Notifications | 14 days |
| AI Queue Items | 30 days |
| Incidents | 90 days |

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CONFIG_PATH` | No | `config/default.json` | Config file location |
| `STATE_PATH` | No | `data/state.json` | Local state file |
| `MONGODB_URI` | No | — | Enable MongoDB mirroring |
| `MONGODB_DATABASE` | No | `healthmon` | MongoDB database name |
| `MONGODB_COLLECTION_PREFIX` | No | `healthmon` | MongoDB collection prefix |
| `{check.mysql.dsnEnv}` | Per check | — | MySQL DSN (never logged) |

---

## Error Codes

| HTTP Status | Meaning |
|-------------|---------|
| `200` | Success |
| `201` | Created |
| `202` | Accepted (async operation started) |
| `204` | No Content (successful delete) |
| `400` | Bad Request (validation error) |
| `401` | Unauthorized (auth required) |
| `404` | Not Found |
| `405` | Method Not Allowed |
| `500` | Internal Server Error |
| `503` | Service Unavailable (component not configured) |

---

## Quick Reference — All 61 Endpoints

| # | Method | Path | Auth | Description |
|---|--------|------|------|-------------|
| 1 | GET | `/healthz` | No | Liveness probe |
| 2 | GET | `/readyz` | No | Readiness probe |
| 3 | GET | `/api/v1/checks` | No | List checks |
| 4 | POST | `/api/v1/checks` | Yes | Create check |
| 5 | GET | `/api/v1/checks/{id}` | No | Check detail |
| 6 | PUT | `/api/v1/checks/{id}` | Yes | Update check |
| 7 | DELETE | `/api/v1/checks/{id}` | Yes | Delete check |
| 8 | POST | `/api/v1/runs` | Yes | Trigger run |
| 9 | GET | `/api/v1/summary` | No | Health summary |
| 10 | GET | `/api/v1/results` | No | Historical results |
| 11 | GET | `/api/v1/dashboard/checks` | No | Dashboard checks |
| 12 | GET | `/api/v1/dashboard/summary` | No | Dashboard summary |
| 13 | GET | `/api/v1/dashboard/results` | No | Dashboard results |
| 14 | GET | `/api/v1/incidents` | No | List incidents (filtered) |
| 15 | GET | `/api/v1/incidents/{id}` | No | Get incident |
| 16 | POST | `/api/v1/incidents/{id}/acknowledge` | Yes | Acknowledge incident |
| 17 | POST | `/api/v1/incidents/{id}/resolve` | Yes | Resolve incident |
| 18 | GET | `/api/v1/incidents/{id}/snapshots` | No | Incident evidence |
| 19 | GET | `/api/v1/audit` | No | Audit log |
| 20 | GET | `/api/v1/analytics/uptime` | No | Uptime stats |
| 21 | GET | `/api/v1/analytics/response-times` | No | Response time percentiles |
| 22 | GET | `/api/v1/analytics/status-timeline` | No | Status timeline |
| 23 | GET | `/api/v1/analytics/failure-rate` | No | Failure rate by group |
| 24 | GET | `/api/v1/analytics/incidents` | No | Incident MTTA/MTTR |
| 25 | GET | `/api/v1/stats/overview` | No | Dashboard overview |
| 26 | GET | `/api/v1/config` | No | Get config |
| 27 | PUT | `/api/v1/config` | Yes | Update config |
| 28 | GET | `/api/v1/alert-rules` | No | List alert rules |
| 29 | POST | `/api/v1/alert-rules` | Yes | Create alert rule |
| 30 | PUT | `/api/v1/alert-rules/{id}` | Yes | Update alert rule |
| 31 | DELETE | `/api/v1/alert-rules/{id}` | Yes | Delete alert rule |
| 32 | GET | `/api/v1/events` | No | SSE live stream |
| 33 | GET | `/api/v1/auth/me` | No | Auth info |
| 34 | GET | `/api/v1/mysql/samples` | No | MySQL samples |
| 35 | GET | `/api/v1/mysql/deltas` | No | MySQL deltas |
| 36 | GET | `/api/v1/mysql/health` | No | MySQL health card |
| 37 | GET | `/api/v1/mysql/timeseries` | No | MySQL time-series |
| 38 | GET | `/api/v1/notifications` | No | List notifications |
| 39 | POST | `/api/v1/notifications/{id}/sent` | Yes | Mark sent |
| 40 | POST | `/api/v1/notifications/{id}/failed` | Yes | Mark failed |
| 41 | GET | `/api/v1/notifications/stats` | No | Notification stats |
| 42 | GET | `/api/v1/ai/queue` | No | AI queue items |
| 43 | POST | `/api/v1/ai/queue/{incidentId}/done` | Yes | Complete analysis |
| 44 | POST | `/api/v1/ai/queue/{incidentId}/failed` | Yes | Fail analysis |
| 45 | GET | `/api/v1/ai/queue/stats` | No | AI queue stats |
| 46 | GET | `/api/v1/ai/config` | No | AI service config (masked) |
| 47 | PUT | `/api/v1/ai/config` | Yes | Update AI settings |
| 48 | GET | `/api/v1/ai/providers` | No | List AI providers |
| 49 | POST | `/api/v1/ai/providers` | Yes | Add AI provider |
| 50 | PUT | `/api/v1/ai/providers/{id}` | Yes | Update AI provider |
| 51 | DELETE | `/api/v1/ai/providers/{id}` | Yes | Delete AI provider |
| 52 | GET | `/api/v1/ai/prompts` | No | List prompt templates |
| 53 | POST | `/api/v1/ai/prompts` | Yes | Create prompt template |
| 54 | PUT | `/api/v1/ai/prompts/{id}` | Yes | Update prompt template |
| 55 | DELETE | `/api/v1/ai/prompts/{id}` | Yes | Delete prompt template |
| 56 | POST | `/api/v1/ai/analyze/{incidentId}` | Yes | Trigger AI analysis |
| 57 | GET | `/api/v1/ai/health` | No | AI provider health |
| 58 | GET | `/api/v1/ai/results/{incidentId}` | No | AI analysis results |
| 59 | GET | `/api/v1/export/mysql/samples` | No | Export MySQL CSV/JSON |
| 60 | GET | `/api/v1/export/incidents` | No | Export incidents CSV/JSON |
| 61 | GET | `/api/v1/export/results` | No | Export results CSV/JSON |
| 62 | GET | `/metrics` | No | Prometheus metrics |
