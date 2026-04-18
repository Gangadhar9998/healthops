# Health Check

Monitoring system for app servers, APIs, processes, logs, and short-term health trends.

## Layout

- `backend/` Go service for health checks, alerting, and JSON APIs
- `frontend/` reserved for the monitoring UI

## Backend

```bash
cd backend
go run ./cmd/healthmon
```

The backend exposes:

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/checks`
- `POST /api/v1/checks`
- `POST /api/v1/runs`
- `GET /api/v1/summary`
- `GET /api/v1/results`

## Configuration

- Edit `backend/config/default.json` to define servers, APIs, processes, and log-heartbeat checks.
- Set `CONFIG_PATH` or `STATE_PATH` if you want to override the defaults.
- Set `MONGODB_URI` if you want mirrored persistence in MongoDB. The backend stays up if MongoDB is unreachable because the local file store remains active.
- The dashboard UI can read from `/api/v1/dashboard/checks`, `/api/v1/dashboard/summary`, and `/api/v1/dashboard/results`.
