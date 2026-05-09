# Production Deployment Guide

HealthOps is MongoDB-first and Docker-friendly. MongoDB is the authoritative
store for checks, results, incidents, audit events, users, notification state,
AI config, MySQL telemetry, and server metrics. Runtime JSON/JSONL state files
are not supported deployment targets.

## Required Environment

| Variable | Required | Purpose |
| --- | --- | --- |
| `MONGODB_URI` | Yes | MongoDB connection string. Startup fails if MongoDB cannot be reached. |
| `MONGODB_DATABASE` | Yes | Database name, for example `healthops`. |
| `MONGODB_COLLECTION_PREFIX` | Yes | Collection prefix, for example `healthops`. |
| `HEALTHOPS_JWT_SECRET` | Yes | JWT signing secret, at least 32 bytes. |
| `HEALTHOPS_AI_ENCRYPTION_KEY` | Yes | AI provider encryption key, at least 32 bytes. |
| `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` | Yes | Strong admin bootstrap password. |
| `HEALTHOPS_BOOTSTRAP_ADMIN_EMAIL` | No | Defaults to `admin@healthops.local`. |
| `HEALTHOPS_BOOTSTRAP_ADMIN_RESET` | No | Set `true` only when intentionally resetting admin. |
| `CONFIG_PATH` | No | First-run seed config. Defaults to `backend/config/default.json` or `config/default.json`. |
| `FRONTEND_DIR` | No | Built frontend assets. Defaults to `frontend/dist`. |

`STATE_PATH`, `DATA_DIR`, `state.json`, `.jsonl` queues, and local encryption
key files are obsolete. Do not mount or back them up as application state.

## Docker Compose

Use the bundled `docker-compose.yml` as the default topology: one HealthOps
container plus one MongoDB container with a Mongo volume.

```bash
export HEALTHOPS_JWT_SECRET="$(openssl rand -base64 48)"
export HEALTHOPS_AI_ENCRYPTION_KEY="$(openssl rand -base64 48)"
export HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD="$(openssl rand -base64 32)"
export MONGODB_URI="mongodb://mongo:27017"
export MONGODB_DATABASE="healthops"
export MONGODB_COLLECTION_PREFIX="healthops"

docker compose up -d
```

For production, bind the app port to localhost and put TLS in front with nginx,
Caddy, ALB, Cloudflare, or your platform ingress.

## Binary Deployments

Run MongoDB as a managed service or a hardened host service, then start the Go
binary with the required environment above.

```ini
[Service]
Type=simple
User=healthops
Group=healthops
WorkingDirectory=/var/lib/healthops
ExecStart=/usr/local/bin/healthops
Restart=on-failure
RestartSec=5s
Environment=CONFIG_PATH=/etc/healthops/config.json
Environment=FRONTEND_DIR=/opt/healthops/frontend-dist
EnvironmentFile=/etc/healthops/healthops.env
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
```

Keep secrets in `/etc/healthops/healthops.env` with mode `0600`.

## First Run

On first start, HealthOps seeds checks from `CONFIG_PATH` only if MongoDB has no
checks yet. After that, checks, servers, users, notification channels, alert
rules, and AI config are managed through the API/UI and persisted in MongoDB.

## Backups

Back up MongoDB using `mongodump`, snapshots, or your managed Mongo backup
feature. Include the secret values from your secret manager in disaster
recovery procedures, especially `HEALTHOPS_AI_ENCRYPTION_KEY`.

The repository includes lightweight helper scripts for self-managed MongoDB
backups:

```bash
ENV_FILE=/etc/healthops/healthops.env \
BACKUP_DIR=/var/backups/healthops \
scripts/healthops-mongo-backup.sh

ENV_FILE=/etc/healthops/healthops.env \
CONFIRM_RESTORE=healthops \
scripts/healthops-mongo-restore.sh /var/backups/healthops/healthops-mongo-healthops-20260509T120000Z.archive.gz
```

See [backups.md](backups.md) for restore order and secret-handling notes.

## Smoke Tests

```bash
BASE=https://healthops.example.com
curl -fsS "$BASE/healthz"
curl -fsS "$BASE/readyz"
curl -fsS "$BASE/api/v1/checks"
curl -fsS "$BASE/metrics" | head
```
