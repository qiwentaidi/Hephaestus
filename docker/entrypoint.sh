#!/bin/sh
set -eu

CONFIG_DIR="${HEPHAESTUS_CONFIG_DIR:-/var/lib/hephaestus}"
RESOURCE_DIR="${HEPHAESTUS_RESOURCE_DIR:-$CONFIG_DIR/config}"
DATA_DIR="$CONFIG_DIR/data"
LOG_DIR="$CONFIG_DIR/logs"
CONFIG_FILE="$CONFIG_DIR/config.yaml"
PORT_VALUE="${PORT:-18181}"
DB_TYPE="${HEPHAESTUS_DB_TYPE:-postgres}"
DB_HOST="${HEPHAESTUS_DB_HOST:-local}"
DB_PORT="${HEPHAESTUS_DB_PORT:-5432}"
DB_USER="${HEPHAESTUS_DB_USER:-postgres}"
DB_NAME="${HEPHAESTUS_DB_NAME:-hephaestus}"
DB_SSLMODE="${HEPHAESTUS_DB_SSLMODE:-disable}"
DB_PASSWORD="${HEPHAESTUS_DB_PASSWORD:-}"
PGDATA="${HEPHAESTUS_PGDATA:-$CONFIG_DIR/pgdata}"
PG_SOCKET_DIR="${HEPHAESTUS_PG_SOCKET_DIR:-/run/postgresql}"
PG_LOG="$LOG_DIR/postgresql.log"
PG_PASSWORD_FILE="$CONFIG_DIR/.pg-password"
JWT_SECRET_FILE="$CONFIG_DIR/.jwt-secret"
EMBEDDED_PG=0

log() {
  printf '[hephaestus] %s\n' "$*"
}

yaml_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

generate_password() {
  head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32
}

case "$(printf '%s' "$DB_TYPE" | tr '[:upper:]' '[:lower:]')" in
  postgres|postgresql) DB_TYPE=postgres ;;
  sqlite|sqlite3) DB_TYPE=sqlite ;;
  *) log "不支持的 HEPHAESTUS_DB_TYPE: $DB_TYPE"; exit 1 ;;
esac

# An existing config.yaml is authoritative. This keeps older SQLite volumes
# working and avoids starting an unused embedded PostgreSQL cluster.
CONFIG_DB_TYPE=""
CONFIG_DB_HOST=""
CONFIG_DB_PASSWORD=""
if [ -f "$CONFIG_FILE" ]; then
  CONFIG_DB_TYPE=$(awk '
    /^database:[[:space:]]*$/ { in_db = 1; next }
    in_db && /^[^[:space:]]/ { in_db = 0 }
    in_db && /^[[:space:]]*type:/ { sub(/^[^:]*:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }
  ' "$CONFIG_FILE")
  CONFIG_DB_HOST=$(awk '
    /^database:[[:space:]]*$/ { in_db = 1; next }
    in_db && /^[^[:space:]]/ { in_db = 0 }
    in_db && /^[[:space:]]*host:/ { sub(/^[^:]*:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }
  ' "$CONFIG_FILE")
  CONFIG_DB_PASSWORD=$(awk '
    /^database:[[:space:]]*$/ { in_db = 1; next }
    in_db && /^[^[:space:]]/ { in_db = 0 }
    in_db && /^[[:space:]]*password:/ { sub(/^[^:]*:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }
  ' "$CONFIG_FILE")
fi
if [ -n "$CONFIG_DB_TYPE" ]; then
  DB_TYPE="$(printf '%s' "$CONFIG_DB_TYPE" | tr '[:upper:]' '[:lower:]')"
  case "$DB_TYPE" in
    postgres|postgresql) DB_TYPE=postgres ;;
    sqlite|sqlite3) DB_TYPE=sqlite ;;
  esac
fi
if [ -n "$CONFIG_DB_HOST" ]; then
  DB_HOST="$CONFIG_DB_HOST"
fi

mkdir -p "$CONFIG_DIR" "$RESOURCE_DIR" "$DATA_DIR" "$LOG_DIR"

if [ "$DB_TYPE" = postgres ]; then
  case "$(printf '%s' "$DB_HOST" | tr '[:upper:]' '[:lower:]')" in
    ""|local|127.0.0.1|localhost|::1)
      if [ "${HEPHAESTUS_EMBEDDED_POSTGRES:-1}" != "0" ]; then
        EMBEDDED_PG=1
        DB_HOST=127.0.0.1
      fi
      ;;
  esac
fi

if [ "$EMBEDDED_PG" = 1 ]; then
  if [ -s "$PG_PASSWORD_FILE" ]; then
    DB_PASSWORD="$(cat "$PG_PASSWORD_FILE")"
  elif [ -n "$CONFIG_DB_PASSWORD" ]; then
    DB_PASSWORD="$CONFIG_DB_PASSWORD"
  elif [ -z "$DB_PASSWORD" ]; then
    DB_PASSWORD="$(generate_password)"
  fi
  if [ ! -s "$PG_PASSWORD_FILE" ]; then
    umask 077
    printf '%s\n' "$DB_PASSWORD" > "$PG_PASSWORD_FILE"
    chmod 600 "$PG_PASSWORD_FILE"
  fi
fi

# Keep server-side JWTs valid across container recreations unless the operator
# supplied an explicit secret.
if [ -z "${HEPHAESTUS_JWT_SECRET:-}" ]; then
  if [ -s "$JWT_SECRET_FILE" ]; then
    HEPHAESTUS_JWT_SECRET="$(cat "$JWT_SECRET_FILE")"
  else
    HEPHAESTUS_JWT_SECRET="$(generate_password)"
    (umask 077; printf '%s\n' "$HEPHAESTUS_JWT_SECRET" > "$JWT_SECRET_FILE")
    chmod 600 "$JWT_SECRET_FILE"
  fi
  export HEPHAESTUS_JWT_SECRET
fi

start_embedded_postgres() {
  log "初始化/启动内置 PostgreSQL (port=$DB_PORT, data=$PGDATA)"
  mkdir -p "$PGDATA" "$PG_SOCKET_DIR"
  chown postgres:postgres "$PGDATA" "$PG_SOCKET_DIR"
  chmod 700 "$PGDATA"
  chmod 775 "$PG_SOCKET_DIR"
  touch "$PG_LOG"
  chown postgres:postgres "$PG_LOG"

  if [ ! -f "$PGDATA/PG_VERSION" ]; then
    chown -R postgres:postgres "$PGDATA"
    pwfile="$(mktemp /tmp/hephaestus-pg-pw.XXXXXX)"
    # Write before handing the file to postgres: afterwards only a process with
    # root's DAC override can write to it, which breaks hardened/rootless runs.
    (umask 077; printf '%s\n' "$DB_PASSWORD" > "$pwfile")
    chown postgres:postgres "$pwfile"
    chmod 600 "$pwfile"
    if ! su-exec postgres initdb \
      -D "$PGDATA" \
      -U "$DB_USER" \
      --auth-local=trust \
      --auth-host=scram-sha-256 \
      --pwfile="$pwfile" \
      --encoding=UTF8 \
      --locale=C >>"$PG_LOG" 2>&1; then
      rm -f "$pwfile"
      log "PostgreSQL 初始化失败，最近日志："
      tail -n 40 "$PG_LOG" || true
      return 1
    fi
    rm -f "$pwfile"
    cat >>"$PGDATA/postgresql.conf" <<EOF
listen_addresses = '127.0.0.1'
port = $DB_PORT
unix_socket_directories = '$PG_SOCKET_DIR'
max_connections = 100
shared_buffers = 128MB
EOF
  fi

  if ! su-exec postgres pg_ctl -D "$PGDATA" -l "$PG_LOG" -o "-p $DB_PORT" -w start; then
    log "PostgreSQL 启动失败，最近日志："
    tail -n 40 "$PG_LOG" || true
    return 1
  fi

  i=0
  until pg_isready -h 127.0.0.1 -p "$DB_PORT" -U "$DB_USER" >/dev/null 2>&1; do
    i=$((i + 1))
    if [ "$i" -ge 60 ]; then
      log "等待 PostgreSQL 就绪超时，最近日志："
      tail -n 40 "$PG_LOG" || true
      return 1
    fi
    sleep 1
  done
  log "PostgreSQL 已就绪"
}

stop_embedded_postgres() {
  if [ "$EMBEDDED_PG" = 1 ] && [ -f "$PGDATA/PG_VERSION" ]; then
    su-exec postgres pg_ctl -D "$PGDATA" -m fast -w stop >/dev/null 2>&1 || true
  fi
}

write_config() {
  if [ -f "$CONFIG_FILE" ]; then
    log "使用已有配置 $CONFIG_FILE"
    return
  fi

  log "生成初始配置 $CONFIG_FILE"
  if [ "$DB_TYPE" = postgres ]; then
    cat >"$CONFIG_FILE" <<EOF
web:
  port: ${PORT_VALUE}
database:
  type: postgres
  host: "$(yaml_escape "$DB_HOST")"
  port: "$(yaml_escape "$DB_PORT")"
  user: "$(yaml_escape "$DB_USER")"
  password: "$(yaml_escape "$DB_PASSWORD")"
  dbname: "$(yaml_escape "$DB_NAME")"
  sslmode: "$(yaml_escape "$DB_SSLMODE")"
EOF
  else
    cat >"$CONFIG_FILE" <<EOF
web:
  port: ${PORT_VALUE}
database:
  type: sqlite
  path: ${DATA_DIR}/hephaestus.db
EOF
  fi
  chmod 600 "$CONFIG_FILE"
}

APP_PID=""
SHUTTING_DOWN=0

shutdown() {
  SHUTTING_DOWN=1
  if [ -n "$APP_PID" ] && kill -0 "$APP_PID" 2>/dev/null; then
    kill -TERM "$APP_PID" 2>/dev/null || true
  fi
}

trap shutdown INT TERM

if [ "$EMBEDDED_PG" = 1 ]; then
  start_embedded_postgres
fi
write_config

export HEPHAESTUS_DEPLOYMENT="${HEPHAESTUS_DEPLOYMENT:-docker}"
export HEPHAESTUS_CONTAINER=1

log "启动 Hephaestus Web 服务 (port=$PORT_VALUE, db=$DB_TYPE)"
/opt/hephaestus/hephaestus-web "$@" &
APP_PID=$!

STATUS=0
wait "$APP_PID" || STATUS=$?
if [ "$STATUS" -gt 128 ]; then
  # A signal interrupted wait before the child finished closing down.
  wait "$APP_PID" || STATUS=$?
fi
APP_PID=""

if [ "$SHUTTING_DOWN" = 1 ]; then
  STATUS=0
fi

stop_embedded_postgres
log "Hephaestus 已退出 (code=$STATUS)"
exit "$STATUS"
