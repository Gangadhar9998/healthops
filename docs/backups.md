# HealthOps Backup and Disaster Recovery

**Audience:** the operator responsible for restoring HealthOps after data loss.

MongoDB is the authoritative required storage backend for HealthOps. File-store artifacts such as `STATE_PATH`, `DATA_DIR`, `state.json`, and JSONL repositories are legacy implementation details and are not valid backup or restore targets.

For deployment layout details see [deployment-guide.md](deployment-guide.md). For operational playbooks after a restore see [runbook.md](runbook.md).

---

## 1. What to back up

Back up these assets together:

| Asset | What it holds | Loss impact |
|---|---|---|
| MongoDB database named by `MONGODB_DATABASE` | Checks, results, users, incidents, alert rules, notification channels, AI configuration, audit data | Authoritative service state. Loss = full reset. |
| Deployment environment secrets | `MONGODB_URI`, `MONGODB_DATABASE`, `MONGODB_COLLECTION_PREFIX`, `HEALTHOPS_JWT_SECRET`, `HEALTHOPS_AI_ENCRYPTION_KEY`, `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` | Cannot safely restart or decrypt/sign runtime data without the same values. |
| `backend/config/default.json` or deployed config seed | First-run check seed and server settings | Needed for clean rebuilds and disaster recovery rehearsals. |

Store environment secrets in a real secret manager when possible. If you keep a `.env` or `/etc/healthops/healthops.env` file, back it up encrypted and restrict restore access.

---

## 2. Backup frequency

| Workload pattern | Frequency | Rationale |
|---|---|---|
| Idle / dev | Daily `mongodump` | RPO 24h is usually fine. |
| Active monitoring with incidents | Hourly `mongodump` plus daily full offsite copy | Limits incident/audit data loss to roughly 1h. |
| MySQL monitoring at 1m intervals | Hourly `mongodump` | MySQL telemetry grows continuously; hourly keeps trend gaps small. |

Keep at least 7 daily snapshots, 4 weekly snapshots, and 12 monthly snapshots before pruning.

---

## 3. Backup script

Run the repository helper on a host that has MongoDB Database Tools installed
and can reach MongoDB. The script loads `ENV_FILE` when set, otherwise it loads
`.env` from the current directory if present, and fails clearly when
`MONGODB_URI` or `MONGODB_DATABASE` is missing.

```bash
ENV_FILE=/etc/healthops/healthops.env \
BACKUP_DIR=/var/backups/healthops \
scripts/healthops-mongo-backup.sh
```

The output is a gzip-compressed `mongodump` archive:
`healthops-mongo-<database>-<timestamp>.archive.gz`.

The helper intentionally backs up MongoDB only. Back up deployment secrets from
your secret manager, `.env`, or `/etc/healthops/healthops.env` separately and
store them encrypted. Without the same `HEALTHOPS_AI_ENCRYPTION_KEY` and
`HEALTHOPS_JWT_SECRET`, restored data may not be usable.

`/etc/cron.d/healthops-backup`:

```cron
# m h dom mon dow user command
0  *  *   *   *   healthops cd /opt/healthops && ENV_FILE=/etc/healthops/healthops.env BACKUP_DIR=/var/backups/healthops scripts/healthops-mongo-backup.sh >> /var/log/healthops-backup.log 2>&1
```

---

## 4. Restore procedure

Restore is a stop / restore MongoDB / restore secrets / start workflow. Do not restore legacy `data/` directories.

```bash
# 1. Stop the service
sudo systemctl stop healthops
# or: docker compose stop healthops

# 2. Fetch the MongoDB archive and the matching secret/config snapshots
aws s3 cp s3://your-bucket/healthops/prod-1/healthops-mongo-healthops-20260509T120000Z.archive.gz /tmp/
# Fetch or decrypt /path/to/healthops.env from your secret backup.
# Fetch /path/to/default.json if you back up the first-run config seed.

# 3. Restore environment secrets before starting the app
sudo install -m 0640 -o root -g healthops /path/to/healthops.env /etc/healthops/healthops.env
set -a
# shellcheck disable=SC1091
. /etc/healthops/healthops.env
set +a

# 4. Restore authoritative MongoDB state
CONFIRM_RESTORE="$MONGODB_DATABASE" \
    scripts/healthops-mongo-restore.sh /tmp/healthops-mongo-healthops-20260509T120000Z.archive.gz

# 5. Restore config seed if present
if [ -f /path/to/default.json ]; then
    sudo install -m 0644 -o healthops -g healthops /path/to/default.json /opt/healthops/config/default.json
fi

# 6. Start and verify
sudo systemctl start healthops
curl -fsS http://127.0.0.1:8080/healthz
TOKEN=$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"<known-good-admin-password>"}' \
  | jq -r '.data.token')
curl -fsS http://127.0.0.1:8080/api/v1/summary -H "Authorization: Bearer $TOKEN" | jq
```

After verification, rotate `HEALTHOPS_BOOTSTRAP_ADMIN_PASSWORD` out of the environment if your production policy removes it after the first start.

---

## 5. Targets

For an internal infrastructure tool:

- **RPO (Recovery Point Objective):** 24 hours by default; hourly snapshots tighten this to 1 hour during active monitoring.
- **RTO (Recovery Time Objective):** 1 hour. Restore is a MongoDB restore plus service restart.

Tighten these only if HealthOps itself is reclassified above "internal-tool" tier, at which point you also need redundancy at the service layer (see [slo.md](slo.md), section 4).
