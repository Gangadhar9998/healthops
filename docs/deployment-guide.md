# HealthOps Production Deployment Guide

Complete guide for deploying HealthOps from scratch. Covers local dev, Docker, bare-metal production, required MongoDB storage, custom domains, TLS/SSL, notifications, and troubleshooting.

---

## Table of Contents

1. [Quick Start (Development)](#1-quick-start-development)
2. [Production Deployment with Docker](#2-production-deployment-with-docker)
3. [Bare-Metal Production Deployment](#3-bare-metal-production-deployment)
4. [Environment Variables Reference](#4-environment-variables-reference)
5. [Ports Reference](#5-ports-reference)
6. [Configuration File Reference](#6-configuration-file-reference)
7. [Custom Domain & TLS/SSL Setup](#7-custom-domain--tlsssl-setup)
8. [Authentication & User Management](#8-authentication--user-management)
9. [Notification Channels Setup](#9-notification-channels-setup)
10. [SSH Remote Server Monitoring](#10-ssh-remote-server-monitoring)
11. [MySQL Monitoring Setup](#11-mysql-monitoring-setup)
12. [AI Analysis Setup (BYOK)](#12-ai-analysis-setup-byok)
13. [Backup & Recovery](#13-backup--recovery)
14. [Troubleshooting](#14-troubleshooting)
15. [Log Locations & How to Read Them](#15-log-locations--how-to-read-them)
16. [Health Verification Checklist](#16-health-verification-checklist)

---

## 1. Quick Start (Development)

### Prerequisites
- Go 1.23+
- Node.js 20+
- npm
- MongoDB 7+ reachable from the HealthOps backend

### Steps

```bash
# Clone the repo
git clone https://github.com/your-org/healthops.git
cd healthops

# Build frontend
cd frontend && npm install && npm run build && cd ..

# Start backend (serves frontend too)
cd backend && \
  FRONTEND_DIR=../frontend/dist \
  MONGODB_URI=mongodb://localhost:27017 \
  MONGODB_DATABASE=healthops \
  MONGODB_COLLECTION_PREFIX=healthops \
  HEALTHOPS_JWT_SECRET='change-me-at-least-32-characters' \
  HEALTHOPS_AI_ENCRYPTION_KEY='change-me-random-ai-secret' \
  HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD='change-me-admin-password' \
  go run ./cmd/healthops
```

Open http://localhost:8080. Login with `admin` / `admin`.

### Frontend-Only Development (hot reload)

```bash
# Terminal 1: Start backend
cd backend && \
  FRONTEND_DIR=../frontend/dist \
  MONGODB_URI=mongodb://localhost:27017 \
  MONGODB_DATABASE=healthops \
  MONGODB_COLLECTION_PREFIX=healthops \
  HEALTHOPS_JWT_SECRET='change-me-at-least-32-characters' \
  HEALTHOPS_AI_ENCRYPTION_KEY='change-me-random-ai-secret' \
  HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD='change-me-admin-password' \
  go run ./cmd/healthops

# Terminal 2: Start Vite dev server (proxies API to backend)
cd frontend && npm run dev
```

Frontend dev server runs on http://localhost:3000 with hot reload. API calls are proxied to `:8080`.

---

## 2. Production Deployment with Docker

### Option A: Docker Compose (recommended)

```bash
# Clone and start
git clone https://github.com/your-org/healthops.git
cd healthops
cat > .env <<'EOF'
MONGODB_URI=mongodb://mongo:27017
MONGODB_DATABASE=healthops
MONGODB_COLLECTION_PREFIX=healthops
HEALTHOPS_JWT_SECRET=replace-with-at-least-32-random-characters
HEALTHOPS_AI_ENCRYPTION_KEY=replace-with-a-random-ai-encryption-secret
HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD=replace-with-a-strong-admin-password
EOF
docker compose up -d
```

This starts:
- **HealthOps** on port `8080`
- **MongoDB** on port `27017` (localhost only)

### Option B: Docker Compose with overrides

Create a `.env` file in the project root:

```env
# .env
MONGODB_URI=mongodb://mongo:27017
MONGODB_DATABASE=healthops
MONGODB_COLLECTION_PREFIX=healthops
HEALTHOPS_JWT_SECRET=replace-with-at-least-32-random-characters
HEALTHOPS_AI_ENCRYPTION_KEY=replace-with-a-random-ai-encryption-secret
HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD=replace-with-a-strong-admin-password
```

Create `docker-compose.override.yml` for production customizations:

```yaml
services:
  healthops:
    environment:
      - CORS_ORIGIN=https://monitor.yourdomain.com
    ports:
      - "127.0.0.1:8080:8080"  # Only expose to localhost (nginx forwards)
    restart: always

  mongo:
    environment:
      - MONGO_INITDB_ROOT_USERNAME=healthops
      - MONGO_INITDB_ROOT_PASSWORD=your-secure-password
    restart: always
```

Then update `MONGODB_URI` in `.env`:
```env
MONGODB_URI=mongodb://healthops:your-secure-password@mongo:27017/healthops?authSource=admin
```

```bash
docker compose up -d
```

### Option C: Docker only with external MongoDB

```bash
docker build -t healthops .
docker run -d \
  --name healthops \
  -p 8080:8080 \
  -e MONGODB_URI=mongodb://your-mongo-host:27017 \
  -e MONGODB_DATABASE=healthops \
  -e MONGODB_COLLECTION_PREFIX=healthops \
  -e HEALTHOPS_JWT_SECRET=replace-with-at-least-32-random-characters \
  -e HEALTHOPS_AI_ENCRYPTION_KEY=replace-with-a-random-ai-encryption-secret \
  -e HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD=replace-with-a-strong-admin-password \
  healthops
```

### Verify

```bash
curl http://localhost:8080/healthz
# {"success":true,"data":{"status":"ok"}}

curl http://localhost:8080/readyz
# {"success":true,"data":{"status":"ready","checks":4,...}}
```

---

## 3. Bare-Metal Production Deployment

### Build

```bash
cd backend
CGO_ENABLED=0 go build -o healthops ./cmd/healthops

cd ../frontend
npm ci && npm run build
```

### Install

```bash
# Create app directory
sudo mkdir -p /opt/healthops/{config,frontend}

# Copy files
sudo cp backend/healthops /opt/healthops/
sudo cp backend/config/default.json /opt/healthops/config/
sudo cp -r frontend/dist/* /opt/healthops/frontend/

# Create app user
sudo useradd -r -s /usr/sbin/nologin healthops
sudo chown -R healthops:healthops /opt/healthops
```

Create `/etc/healthops/healthops.env` and restrict it to the service user:

```env
MONGODB_URI=mongodb://localhost:27017
MONGODB_DATABASE=healthops
MONGODB_COLLECTION_PREFIX=healthops
HEALTHOPS_JWT_SECRET=replace-with-at-least-32-random-characters
HEALTHOPS_AI_ENCRYPTION_KEY=replace-with-a-random-ai-encryption-secret
HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD=replace-with-a-strong-admin-password
```

```bash
sudo install -d -m 0750 -o root -g healthops /etc/healthops
sudoedit /etc/healthops/healthops.env
sudo chown root:healthops /etc/healthops/healthops.env
sudo chmod 0640 /etc/healthops/healthops.env
```

### Systemd Service

Create `/etc/systemd/system/healthops.service`:

```ini
[Unit]
Description=HealthOps Monitoring Service
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=healthops
Group=healthops
WorkingDirectory=/opt/healthops

ExecStart=/opt/healthops/healthops

# Environment
Environment=CONFIG_PATH=/opt/healthops/config/default.json
Environment=FRONTEND_DIR=/opt/healthops/frontend
Environment=CORS_ORIGIN=https://monitor.yourdomain.com
EnvironmentFile=/etc/healthops/healthops.env

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=healthops

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict

# Restart policy
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable healthops
sudo systemctl start healthops
sudo systemctl status healthops
```

---

## 4. Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `CONFIG_PATH` | `config/default.json` | Path to the checks configuration file |
| `FRONTEND_DIR` | *(not set)* | Path to built frontend `dist/` folder. If unset, no frontend served |
| `MONGODB_URI` | *(not set)* | **Required.** MongoDB connection string for authoritative storage |
| `MONGODB_DATABASE` | `healthops` | **Required.** MongoDB database name |
| `MONGODB_COLLECTION_PREFIX` | `healthops` | **Required.** Prefix for MongoDB collection names |
| `HEALTHOPS_JWT_SECRET` | *(not set)* | **Required.** JWT signing secret; use at least 32 random characters |
| `HEALTHOPS_AI_ENCRYPTION_KEY` | *(not set)* | **Required.** Deployment-managed secret for AI provider key encryption |
| `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` | *(not set)* | **Required on first start.** Bootstraps or rotates the admin password |
| `CORS_ORIGIN` | *(not set)* | Allowed CORS origin for SSE endpoint. If unset, same-origin only |
| `MYSQL_DSN` or check-specific `dsnEnv` | *(not set)* | MySQL DSN for mysql checks. Never logged |

### Obsolete file-store variables

`STATE_PATH`, `DATA_DIR`, `state.json`, and JSONL repository files are legacy file-store implementation details. Do not use them for production deployment, backup, restore, or operational checks. MongoDB is the authoritative required storage backend.

---

## 5. Ports Reference

| Port | Service | Protocol | Notes |
|------|---------|----------|-------|
| `8080` | HealthOps HTTP API + Frontend | HTTP | Main application port. Configurable via `server.addr` in config |
| `3000` | Vite dev server (dev only) | HTTP | Frontend hot-reload. Only used during development |
| `27017` | MongoDB | TCP | Required for HealthOps authoritative storage |
| `3306` | MySQL | TCP | Only if mysql checks are configured |
| `22` | SSH | TCP | Only if SSH remote server checks are configured |

### Changing the Application Port

In `config/default.json`:
```json
{
  "server": {
    "addr": ":9090"
  }
}
```

---

## 6. Configuration File Reference

The config file (`config/default.json`) controls all runtime behavior:

```json
{
  "server": {
    "addr": ":8080",             // Listen address (":8080" = all interfaces on port 8080)
    "readTimeoutSeconds": 10,     // HTTP read timeout
    "writeTimeoutSeconds": 10,    // HTTP write timeout
    "idleTimeoutSeconds": 60      // HTTP keep-alive idle timeout
  },
  "auth": {
    "enabled": false,             // Enable basic auth (legacy; JWT is preferred)
    "username": "admin",
    "password": "changeme"
  },
  "retentionDays": 7,            // How long to keep check results
  "checkIntervalSeconds": 30,    // Default check interval
  "workers": 8,                  // Parallel check execution workers
  "allowCommandChecks": false,   // SECURITY: enable shell command checks
  "servers": [],                 // Remote SSH server definitions
  "checks": []                   // Health check definitions
}
```

### Check Configuration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique check identifier |
| `name` | string | Yes | Display name |
| `type` | string | Yes | `api`, `tcp`, `process`, `command`, `log`, `mysql`, `ssh` |
| `target` | string | Varies | URL (api), hostname (tcp), process name (process) |
| `host` | string | tcp | Hostname for TCP checks |
| `port` | int | tcp | Port number for TCP checks |
| `command` | string | command | Shell command to execute |
| `path` | string | log | File path for log freshness check |
| `freshnessSeconds` | int | log | Max age of log file in seconds |
| `server` | string | No | Server group tag |
| `application` | string | No | Application group tag |
| `tags` | []string | No | Tags for filtering and notification routing |
| `enabled` | bool | No | Default: `true`. Set `false` to disable |
| `expectedStatus` | int | api | Expected HTTP status code (default: 200) |
| `expectedContains` | string | No | Expected substring in response body |
| `timeoutSeconds` | int | No | Check timeout (default: 5) |
| `warningThresholdMs` | int | No | Response time warning threshold in ms |
| `intervalSeconds` | int | No | Per-check interval override |
| `retryCount` | int | No | Retry attempts on failure |
| `retryDelaySeconds` | int | No | Delay between retries |
| `cooldownSeconds` | int | No | Minimum time between alerts for this check |
| `notificationChannelIDs` | []string | No | Specific channel IDs to notify for this check |

---

## 7. Custom Domain & TLS/SSL Setup

HealthOps does not handle TLS directly. Use a reverse proxy (nginx, Caddy, or a cloud load balancer).

### Option A: Nginx + Let's Encrypt (Recommended)

```bash
# Install nginx and certbot
sudo apt install nginx certbot python3-certbot-nginx
```

Create `/etc/nginx/sites-available/healthops`:

```nginx
server {
    listen 80;
    server_name monitor.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE support
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 86400;
    }
}
```

```bash
sudo ln -s /etc/nginx/sites-available/healthops /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx

# Get TLS certificate
sudo certbot --nginx -d monitor.yourdomain.com
```

Set the CORS origin env var to match your domain:
```bash
CORS_ORIGIN=https://monitor.yourdomain.com
```

### Option B: Caddy (auto-TLS)

Create `Caddyfile`:

```
monitor.yourdomain.com {
    reverse_proxy localhost:8080
}
```

```bash
caddy start
```

Caddy automatically provisions and renews TLS certificates.

### Option C: AWS ALB / Cloud Load Balancer

1. Create an ALB with HTTPS listener on port 443
2. Attach an ACM certificate for your domain
3. Forward to target group pointing at HealthOps on port 8080
4. Set `CORS_ORIGIN=https://monitor.yourdomain.com`

### DNS Setup

Point your domain to your server:

```
Type: A
Name: monitor.yourdomain.com
Value: YOUR_SERVER_IP
TTL: 300
```

---

## 8. Authentication & User Management

### Default Credentials

On first start, HealthOps creates a default admin user:
- **Username:** `admin`
- **Password:** `admin`

**Change this immediately in production.**

### Login

```bash
# Get JWT token
TOKEN=$(curl -s http://localhost:8080/api/v1/auth/login \
  -X POST -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

echo $TOKEN
```

### Change Admin Password

```bash
# First get the admin user ID
curl -s http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# Update password (replace USER_ID)
curl -X PUT http://localhost:8080/api/v1/users/admin \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"password":"your-new-secure-password-here"}'
```

### Create Additional Users

```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "ops-viewer",
    "password": "secure-password",
    "displayName": "Ops Viewer",
    "role": "ops"
  }'
```

**Roles:**
- `admin` — Full access (read + write + user management)
- `ops` — Read-only access (dashboard, checks, incidents)

### Password Requirements

- Minimum 8 characters

---

## 9. Notification Channels Setup

All channel configuration is done via the API or the UI (Settings > Notification Channels).

### Create a Slack Channel

```bash
curl -X POST http://localhost:8080/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Slack Alerts",
    "type": "slack",
    "enabled": true,
    "webhookUrl": "https://hooks.slack.com/services/YOUR_WORKSPACE/YOUR_CHANNEL/YOUR_TOKEN",
    "severities": ["critical", "warning"],
    "notifyOnResolve": true
  }'
```

### Create a Discord Channel

```bash
curl -X POST http://localhost:8080/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Discord Alerts",
    "type": "discord",
    "enabled": true,
    "webhookUrl": "https://discord.com/api/webhooks/1234567890/abcdefghij"
  }'
```

### Create a Webhook Channel

```bash
curl -X POST http://localhost:8080/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Custom Webhook",
    "type": "webhook",
    "enabled": true,
    "webhookUrl": "https://your-service.com/webhooks/alerts",
    "headers": {
      "Authorization": "Bearer your-webhook-token",
      "X-Source": "healthops"
    }
  }'
```

### Create an Email Channel

```bash
curl -X POST http://localhost:8080/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Email Alerts",
    "type": "email",
    "enabled": true,
    "email": "oncall@company.com,manager@company.com",
    "smtpHost": "smtp.gmail.com",
    "smtpPort": 587,
    "smtpUser": "alerts@company.com",
    "smtpPass": "your-app-password",
    "fromEmail": "healthops@company.com",
    "severities": ["critical"],
    "notifyOnResolve": true
  }'
```

### Create a Telegram Channel

```bash
curl -X POST http://localhost:8080/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Telegram Ops",
    "type": "telegram",
    "enabled": true,
    "botToken": "123456789:ABCDefGhIjKlMnOpQrStUvWxYz",
    "chatId": "-1001234567890"
  }'
```

### Create a PagerDuty Channel

```bash
curl -X POST http://localhost:8080/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "PagerDuty Critical",
    "type": "pagerduty",
    "enabled": true,
    "routingKey": "your-pagerduty-integration-key",
    "severities": ["critical"]
  }'
```

### Smart Filter Options

Every channel supports these optional filters:

| Filter | Type | Description |
|--------|------|-------------|
| `severities` | []string | Only these severities: `["critical"]`, `["warning","critical"]` |
| `checkIds` | []string | Only alerts from these specific checks |
| `checkTypes` | []string | Only these check types: `["mysql","api"]` |
| `servers` | []string | Only alerts from these servers |
| `tags` | []string | Check must have at least one matching tag |
| `cooldownMinutes` | int | Min time between notifications for same check |
| `minConsecutiveFailures` | int | Require N failures before notifying |
| `notifyOnResolve` | bool | Also notify when incident resolves |

### Test a Channel

```bash
curl -X POST http://localhost:8080/api/v1/notification-channels/CHANNEL_ID/test \
  -H "Authorization: Bearer $TOKEN"
```

### Link Channels to Specific Checks

Add `notificationChannelIDs` to check config:

```json
{
  "id": "prod-api",
  "name": "Production API",
  "type": "api",
  "target": "https://api.example.com/health",
  "notificationChannelIDs": ["channel-id-1", "channel-id-2"]
}
```

---

## 10. SSH Remote Server Monitoring

Monitor remote servers over SSH by defining servers and SSH checks.

### Define a Remote Server

Add to `config/default.json` under `"servers"`:

```json
{
  "servers": [
    {
      "id": "prod-web-1",
      "name": "Production Web Server 1",
      "host": "192.168.1.100",
      "port": 22,
      "user": "monitoring",
      "keyPath": "/opt/healthops/.ssh/id_ed25519",
      "tags": ["production", "web"],
      "enabled": true
    }
  ]
}
```

Or via API:

```bash
curl -X POST http://localhost:8080/api/v1/servers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Web Server",
    "host": "192.168.1.100",
    "port": 22,
    "user": "monitoring",
    "keyPath": "/opt/healthops/.ssh/id_ed25519",
    "tags": ["production"]
  }'
```

### Auth Options

| Method | Config Fields | Notes |
|--------|--------------|-------|
| SSH key file | `"keyPath": "/path/to/key"` | Recommended for production |
| SSH key from env var | `"keyEnv": "SSH_PRIVATE_KEY"` | Good for containers |
| Password | `"password": "pass"` | Not recommended |
| Password from env var | `"passwordEnv": "SSH_PASS"` | Better than hardcoded |

### Add SSH Checks

```json
{
  "id": "prod-web-nginx",
  "name": "Nginx on prod-web-1",
  "type": "process",
  "target": "nginx",
  "serverId": "prod-web-1",
  "intervalSeconds": 30,
  "enabled": true
}
```

### SSH Key Setup

```bash
# Generate a dedicated monitoring key
ssh-keygen -t ed25519 -f /opt/healthops/.ssh/id_ed25519 -N ""

# Copy to target server
ssh-copy-id -i /opt/healthops/.ssh/id_ed25519.pub monitoring@192.168.1.100

# Test connection
ssh -i /opt/healthops/.ssh/id_ed25519 monitoring@192.168.1.100 "hostname"
```

---

## 11. MySQL Monitoring Setup

### Prerequisites
- MySQL server with a monitoring user
- `dsnEnv` env var containing the DSN

### Create MySQL User

```sql
CREATE USER 'healthops'@'%' IDENTIFIED BY 'secure-password';
GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'healthops'@'%';
GRANT SELECT ON performance_schema.* TO 'healthops'@'%';
FLUSH PRIVILEGES;
```

### Set Environment Variable

```bash
export MYSQL_PROD_DSN="healthops:secure-password@tcp(db.example.com:3306)/mysql?parseTime=true&timeout=3s"
```

### Add MySQL Check to Config

```json
{
  "id": "mysql-prod",
  "name": "Production MySQL",
  "type": "mysql",
  "server": "database",
  "mysql": {
    "dsnEnv": "MYSQL_PROD_DSN",
    "connectTimeoutSeconds": 3,
    "queryTimeoutSeconds": 5,
    "processlistLimit": 50,
    "statementLimit": 20,
    "hostUserLimit": 20
  },
  "intervalSeconds": 15,
  "enabled": true,
  "tags": ["mysql", "production"]
}
```

### Built-in MySQL Alert Rules

HealthOps includes 9 default MySQL rules:
1. Connection utilization > 80%
2. Slow queries per second > 1
3. Lock wait timeouts
4. Table lock waits
5. Aborted connections
6. InnoDB row lock waits
7. InnoDB deadlocks
8. Replication lag
9. Thread running spike

---

## 12. AI Analysis Setup (BYOK)

AI auto-analyzes new incidents. Configure via UI (Settings > AI Configuration) or API.

### Add an AI Provider

```bash
curl -X POST http://localhost:8080/api/v1/ai/providers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "openai",
    "type": "openai",
    "apiKey": "sk-your-openai-key",
    "model": "gpt-4",
    "enabled": true
  }'
```

Supported provider types: `openai`, `anthropic`, `google`, `ollama`, `custom`

API keys are AES-256-GCM encrypted at rest in MongoDB. Provide `HEALTHOPS_AI_ENCRYPTION_KEY` from your deployment secret manager and back it up with the rest of the required environment secrets.

### Using Local Ollama

```bash
curl -X POST http://localhost:8080/api/v1/ai/providers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "local-ollama",
    "type": "ollama",
    "baseUrl": "http://localhost:11434",
    "model": "llama3",
    "enabled": true
  }'
```

---

## 13. Backup & Recovery

### What to Back Up

| Path | Content | Critical? |
|------|---------|-----------|
| MongoDB database named by `MONGODB_DATABASE` | Checks, results, users, incidents, alert rules, notification config, AI config, audit data | Yes |
| Deployment secret store / `.env` | `MONGODB_URI`, `MONGODB_DATABASE`, `MONGODB_COLLECTION_PREFIX`, `HEALTHOPS_JWT_SECRET`, `HEALTHOPS_AI_ENCRYPTION_KEY`, `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` | Yes |
| `config/default.json` | Check definitions | Yes |

### Backup Script

Use the repository helper for self-managed MongoDB backups:

```bash
ENV_FILE=/etc/healthops/healthops.env \
BACKUP_DIR=/var/backups/healthops \
scripts/healthops-mongo-backup.sh
```

Schedule it from the repository/deployment directory, then ship the generated
`*.archive.gz` file to encrypted remote storage. Back up the deployment env
file or secret-manager values separately, especially
`HEALTHOPS_AI_ENCRYPTION_KEY` and `HEALTHOPS_JWT_SECRET`.

### Restore

```bash
sudo systemctl stop healthops
sudo install -m 0640 -o root -g healthops /path/to/healthops.env /etc/healthops/healthops.env
set -a
. /etc/healthops/healthops.env
set +a
CONFIRM_RESTORE="$MONGODB_DATABASE" \
  scripts/healthops-mongo-restore.sh /var/backups/healthops/healthops-mongo-healthops-20260509T120000Z.archive.gz
cp /path/to/default.json /opt/healthops/config/default.json
sudo systemctl start healthops
```

---

## 14. Troubleshooting

### Service Won't Start

```bash
# Check if port is already in use
lsof -i :8080

# Kill stale processes
lsof -ti:8080 | xargs kill -9

# Check systemd logs
sudo journalctl -u healthops -f --no-pager

# Check config is valid JSON
python3 -m json.tool < config/default.json

# Try running manually to see errors
cd /opt/healthops && ./healthops 2>&1
```

### Frontend Not Loading

```bash
# Check FRONTEND_DIR is set and contains files
ls -la $FRONTEND_DIR/index.html

# Rebuild frontend if missing
cd frontend && npm ci && npm run build

# Check backend logs for "serving frontend from ..."
sudo journalctl -u healthops | grep "frontend"
```

### Login Returns 401

```bash
# Check if default credentials work
curl -v http://localhost:8080/api/v1/auth/login \
  -X POST -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'

# If password was changed and forgotten, rotate it through the bootstrap env
sudo systemctl stop healthops
sudoedit /etc/healthops/healthops.env
# Set HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD and, for a one-time forced reset:
# HEALTHOPS_BOOTSTRAP_ADMIN_RESET=true
sudo systemctl start healthops
```

### Checks Always Failing

```bash
# Check latest results
curl -s http://localhost:8080/api/v1/results?limit=5 \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# Test the target manually
curl -v https://your-target-url/healthz

# For TCP checks
nc -zv hostname port

# For process checks
ps aux | grep process-name

# For DNS issues
nslookup target-hostname
```

### Rate Limited (429 Too Many Requests)

The API limits to 100 requests per minute per IP. If you hit this:
- Wait 60 seconds and retry
- If monitoring tools are polling too fast, increase their interval
- Health endpoints (`/healthz`, `/readyz`) are exempt from rate limiting

---

## 15. Log Locations & How to Read Them

### Where Are the Logs?

| Deployment | Log Location | Command |
|------------|-------------|---------|
| Systemd | journald | `sudo journalctl -u healthops -f` |
| Docker | Container logs | `docker compose logs -f healthops` |
| Manual run | stdout/stderr | Visible in terminal |

### Log Format

```
healthops 2026/04/19 10:15:30.123456 HTTP listening on :8080
healthops 2026/04/19 10:15:30.234567 storage backend selected mode=mongodb_primary
healthops 2026/04/19 10:15:30.345678 SECURITY WARNING: Authentication is disabled
healthops 2026/04/19 10:15:30.456789 Notification channels initialized
healthops 2026/04/19 10:15:30.567890 User management initialized
```

### What to Look For

#### Startup Problems
```bash
# Filter for fatal/error messages
sudo journalctl -u healthops | grep -i "fatal\|error\|warning\|failed"
```

#### Check Failures
```bash
# Watch check execution in real-time
sudo journalctl -u healthops -f | grep -i "check\|fail\|unhealthy"
```

#### Notification Issues
```bash
# Filter notification-related logs
sudo journalctl -u healthops -f | grep -i "notif\|dispatch\|channel\|webhook\|slack\|email\|telegram"
```

### Notification Did Not Send — Debugging Checklist

1. **Is the channel enabled?**
   ```bash
   curl -s http://localhost:8080/api/v1/notification-channels \
     -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
   ```
   Check `"enabled": true` for your channel.

2. **Do severity filters match?**
   If channel has `"severities": ["critical"]` but the incident is `"warning"`, it won't fire.

3. **Is the channel in cooldown?**
   Check `cooldownMinutes` — if set, the same check won't re-notify within that window.

4. **Was the incident already notified?**
   HealthOps deduplicates: each incident + channel pair only fires once. Check the outbox:
   ```bash
   curl -s http://localhost:8080/api/v1/notifications?limit=20 \
     -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
   ```

5. **Is the webhook URL reachable?**
   ```bash
   # Test the webhook directly
   curl -v -X POST "YOUR_WEBHOOK_URL" \
     -H 'Content-Type: application/json' \
     -d '{"text":"test from healthops"}'
   ```

6. **Are SSRF protections blocking it?**
   Webhook URLs to `localhost`, private IPs (`192.168.x.x`, `10.x.x.x`), and cloud metadata (`169.254.169.254`) are blocked for security. Use public URLs.

7. **Check notification delivery history for errors:**
   ```bash
   curl -s http://localhost:8080/api/v1/notifications?limit=20 \
     -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
   ```
   Look for `"error"` fields in the returned entries.

8. **Is the check linked to the channel?**
   If the check has `notificationChannelIDs`, only those channels receive alerts. If empty, all matching channels fire.

9. **Test the channel directly:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/notification-channels/CHANNEL_ID/test \
     -H "Authorization: Bearer $TOKEN"
   ```

### Audit Log

All API mutations are recorded in the audit log:

```bash
# View recent audit entries through the API or your MongoDB/audit export pipeline.
curl -s http://localhost:8080/api/v1/audit \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
```

### Prometheus Metrics

```bash
curl http://localhost:8080/metrics
```

Key metrics:
- `healthops_check_runs_total` — Total check executions
- `healthops_check_failures_total` — Failed checks
- `healthops_check_duration_seconds` — Check latency histogram
- `healthops_http_requests_total` — API request count
- `healthops_http_request_duration_seconds` — API latency

---

## 16. Health Verification Checklist

Run this after deploying or updating:

```bash
# 1. Service is running
curl -s http://localhost:8080/healthz | python3 -m json.tool

# 2. Service is ready and executing checks
curl -s http://localhost:8080/readyz | python3 -m json.tool

# 3. Auth works
TOKEN=$(curl -s http://localhost:8080/api/v1/auth/login \
  -X POST -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")
echo "Token: $TOKEN"

# 4. Checks are configured
curl -s http://localhost:8080/api/v1/checks \
  -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
data = json.load(sys.stdin)
checks = data.get('data', [])
print(f'{len(checks)} checks configured')
for c in checks:
    status = 'enabled' if c.get('enabled', True) else 'disabled'
    print(f'  [{status}] {c[\"id\"]} ({c[\"type\"]}): {c[\"name\"]}')
"

# 5. Results are being collected
curl -s "http://localhost:8080/api/v1/results?limit=5" \
  -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
data = json.load(sys.stdin)
results = data.get('data', [])
print(f'{len(results)} recent results')
for r in results:
    health = 'OK' if r.get('healthy') else 'FAIL'
    print(f'  [{health}] {r[\"checkId\"]} - {r.get(\"latencyMs\",0):.0f}ms')
"

# 6. Notification channels (if configured)
curl -s http://localhost:8080/api/v1/notification-channels \
  -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
data = json.load(sys.stdin)
channels = data.get('data', [])
print(f'{len(channels)} notification channels')
for ch in channels:
    status = 'enabled' if ch.get('enabled') else 'disabled'
    print(f'  [{status}] {ch[\"name\"]} ({ch[\"type\"]})')
"

# 7. Frontend loads
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/
# Should return 200

# 8. Prometheus metrics
curl -s http://localhost:8080/metrics | head -5
```

---

## Quick Reference Card

```
Start (dev):     cd backend && FRONTEND_DIR=../frontend/dist go run ./cmd/healthops
Start (Docker):  docker compose up -d
Start (systemd): sudo systemctl start healthops

Logs (Docker):   docker compose logs -f healthops
Logs (systemd):  sudo journalctl -u healthops -f
Logs (manual):   (visible in terminal)

Health check:    curl http://localhost:8080/healthz
Dashboard:       http://localhost:8080 (or your domain)
API docs:        backend/docs/api-reference.md
Metrics:         http://localhost:8080/metrics

Default login:   admin / admin
Config file:     config/default.json
Storage:         MongoDB (`MONGODB_URI` / `MONGODB_DATABASE`)
```
