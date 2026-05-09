#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf '%s\n' \
    'Usage: healthops-mongo-backup.sh' \
    '' \
    'Creates a gzip-compressed mongodump archive for the HealthOps MongoDB database.' \
    '' \
    'Environment:' \
    '  MONGODB_URI       Required unless loaded from ENV_FILE or .env.' \
    '  MONGODB_DATABASE  Required unless loaded from ENV_FILE or .env.' \
    '  ENV_FILE          Optional env file to source before running. If unset, .env is' \
    '                    loaded when present in the current directory.' \
    '  BACKUP_DIR        Directory for backup archives. Default: ./backups' \
    '  BACKUP_PREFIX     Archive name prefix. Default: healthops-mongo' \
    '' \
    'Example:' \
    '  ENV_FILE=/etc/healthops/healthops.env \' \
    '  BACKUP_DIR=/var/backups/healthops \' \
    '  scripts/healthops-mongo-backup.sh'
}

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

load_env() {
  local env_file="${ENV_FILE:-}"

  if [[ -z "$env_file" && -f .env ]]; then
    env_file=".env"
  fi

  if [[ -n "$env_file" ]]; then
    [[ -f "$env_file" ]] || fail "ENV_FILE does not exist: $env_file"
    set -a
    # shellcheck disable=SC1090
    . "$env_file"
    set +a
  fi
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi
if [[ "$#" -ne 0 ]]; then
  usage >&2
  exit 2
fi

load_env

: "${MONGODB_URI:?MONGODB_URI is required}"
: "${MONGODB_DATABASE:?MONGODB_DATABASE is required}"

need_cmd chmod
need_cmd date
need_cmd mktemp
need_cmd mkdir
need_cmd mongodump
need_cmd mv
need_cmd rm
need_cmd tr

backup_dir="${BACKUP_DIR:-./backups}"
backup_prefix="${BACKUP_PREFIX:-healthops-mongo}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
safe_database="$(printf '%s' "$MONGODB_DATABASE" | LC_ALL=C tr -c 'A-Za-z0-9_.-' '_')"
archive="${backup_dir}/${backup_prefix}-${safe_database}-${timestamp}.archive.gz"
tmp_archive=""

cleanup() {
  if [[ -n "$tmp_archive" && -f "$tmp_archive" ]]; then
    rm -f "$tmp_archive"
  fi
}
trap cleanup EXIT

mkdir -p "$backup_dir"
[[ ! -e "$archive" ]] || fail "backup archive already exists: $archive"

tmp_archive="$(mktemp "${archive}.tmp.XXXXXX")"

printf 'Starting MongoDB backup for database %s\n' "$MONGODB_DATABASE"
mongodump \
  --uri="$MONGODB_URI" \
  --db="$MONGODB_DATABASE" \
  --archive="$tmp_archive" \
  --gzip

chmod 0600 "$tmp_archive"
mv "$tmp_archive" "$archive"
tmp_archive=""

printf 'Backup complete: %s\n' "$archive"
