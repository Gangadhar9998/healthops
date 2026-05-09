#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf '%s\n' \
    'Usage: healthops-mongo-restore.sh <backup.archive.gz>' \
    '' \
    'Restores a HealthOps MongoDB archive created by healthops-mongo-backup.sh.' \
    'By default this drops collections in the configured database before restore.' \
    '' \
    'Environment:' \
    '  MONGODB_URI          Required unless loaded from ENV_FILE or .env.' \
    '  MONGODB_DATABASE     Required unless loaded from ENV_FILE or .env.' \
    '  ENV_FILE             Optional env file to source before running. If unset,' \
    '                       .env is loaded when present in the current directory.' \
    '  CONFIRM_RESTORE      Must equal MONGODB_DATABASE unless RESTORE_DROP=false.' \
    '  RESTORE_DROP         Set to false to restore without --drop. Default: true.' \
    '' \
    'Example:' \
    '  ENV_FILE=/etc/healthops/healthops.env \' \
    '  CONFIRM_RESTORE=healthops \' \
    '  scripts/healthops-mongo-restore.sh /var/backups/healthops/healthops-mongo-healthops-20260509T120000Z.archive.gz'
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
if [[ "$#" -ne 1 ]]; then
  usage >&2
  exit 2
fi

archive="$1"
[[ -f "$archive" ]] || fail "backup archive does not exist: $archive"

load_env

: "${MONGODB_URI:?MONGODB_URI is required}"
: "${MONGODB_DATABASE:?MONGODB_DATABASE is required}"

restore_drop="${RESTORE_DROP:-true}"
case "$restore_drop" in
  true | false) ;;
  *) fail "RESTORE_DROP must be true or false" ;;
esac

if [[ "$restore_drop" != "false" && "${CONFIRM_RESTORE:-}" != "$MONGODB_DATABASE" ]]; then
  fail "set CONFIRM_RESTORE=$MONGODB_DATABASE to allow destructive restore with --drop"
fi

need_cmd mongorestore

args=(
  --uri="$MONGODB_URI"
  --archive="$archive"
  --gzip
  --nsInclude="${MONGODB_DATABASE}.*"
)

if [[ "$restore_drop" != "false" ]]; then
  args+=(--drop)
fi

printf 'Restoring MongoDB database %s from %s\n' "$MONGODB_DATABASE" "$archive"
mongorestore "${args[@]}"
printf 'Restore complete for database %s\n' "$MONGODB_DATABASE"
