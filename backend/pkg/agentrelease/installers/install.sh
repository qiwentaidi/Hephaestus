#!/usr/bin/env bash
# Download released binaries; no source checkout or compiler is required.
set -Eeuo pipefail
umask 077

APP=hephaestus-scan-agent
SERVER="${HEPHAESTUS_SERVER_URL:-}"
VERSION=""
RELEASE_BASE="${HEPHAESTUS_AGENT_RELEASE_BASE:-https://github.com/qiwentaidi/Hephaestus-Agent/releases/download}"
INSTALL_DIR=""
CONFIG_SOURCE=""
ACTION=install
START=1
SERVICE=0
TMP=""
LOCKED=0
SERVICE_NAME=hephaestus-scan-agent

log() { printf '[hephaestus-agent] %s\n' "$*" >&2; }
fail() { log "ERROR: $*"; exit 1; }
valid_config_value() {
  [[ "$1" != *$'\n'* && "$1" != *$'\r'* ]] || return 1
  printf '%s' "$1" | iconv -f UTF-8 -t UTF-8 >/dev/null 2>&1
}
validate_config_file() {
  command -v iconv >/dev/null 2>&1 || fail "install iconv first to validate configuration encoding"
  iconv -f UTF-8 -t UTF-8 "$1" >/dev/null 2>&1 || fail "config.yaml contains invalid UTF-8; correct the input or file encoding before starting the node"
}
usage() {
  cat <<'EOF'
Hephaestus Agent installer (Linux / macOS)
  bash install.sh --server URL [--install-dir DIR] [--config FILE]
  bash install.sh --update --install-dir DIR
  bash install.sh --run --install-dir DIR

Options:
  --version VERSION     Require this version to match the server recommendation.
  --release-base URL    Release download mirror (including /releases/download).
  --no-start            Download/configure without starting the node.
  --service             Run via Linux systemd (requires root; same install directory).
  --config FILE         Copy an existing node config on first install only.

Default install directory: ./hephaestus-agent, including --service.
First installation asks for PostgreSQL credentials if no config is present.
Updates preserve config.yaml, node identity, environment and resources.
Stop a foreground node with Ctrl+C before updating. Managed systemd services
are stopped gracefully and restarted; a failed stop never replaces the binary.
EOF
}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --server|--version|--release-base|--install-dir|--config)
      [[ $# -ge 2 && -n "$2" ]] || fail "missing value for $1"
      case "$1" in
        --server) SERVER="$2";; --version) VERSION="$2";;
        --release-base) RELEASE_BASE="$2";; --install-dir) INSTALL_DIR="$2";;
        --config) CONFIG_SOURCE="$2";;
      esac
      shift 2;;
    --update) ACTION=update; shift;;
    --run) ACTION=run; shift;;
    --no-start) START=0; shift;;
    --service) SERVICE=1; shift;;
    -h|--help) usage; exit 0;;
    *) fail "unknown option: $1";;
  esac
done

if [[ -z "$INSTALL_DIR" ]]; then
  if [[ -f "$(dirname "${BASH_SOURCE[0]}")/.installed-version" ]]; then
    INSTALL_DIR="$(dirname "${BASH_SOURCE[0]}")"
  else
    INSTALL_DIR="$PWD/hephaestus-agent"
  fi
fi
mkdir -p "$INSTALL_DIR"
INSTALL_DIR="$(cd "$INSTALL_DIR" && pwd)"
[[ "$INSTALL_DIR" != *$'\n'* && "$INSTALL_DIR" != *$'\r'* ]] || fail "invalid installation path"
BIN="$INSTALL_DIR/$APP"
cleanup() {
  [[ -z "$TMP" ]] || rm -rf "$TMP"
  [[ "$LOCKED" -eq 0 ]] || rmdir "$INSTALL_DIR/.install-lock" 2>/dev/null || true
}
trap cleanup EXIT

run_node() {
  [[ -x "$BIN" && -f "$INSTALL_DIR/config.yaml" ]] || fail "install/configure the node first"
  validate_config_file "$INSTALL_DIR/config.yaml"
  mkdir "$INSTALL_DIR/.run-lock" 2>/dev/null || fail "node is already running; if it crashed, remove .run-lock after checking the process"
  local child result=0
  trap 'rmdir "$INSTALL_DIR/.run-lock" 2>/dev/null || true; cleanup' EXIT
  cd "$INSTALL_DIR"
  export HEPHAESTUS_CONFIG_DIR="$INSTALL_DIR"
  export HEPHAESTUS_RESOURCE_DIR="$INSTALL_DIR/config"
  # Existing environment overrides are retained; config.yaml is never sourced.
  local supervisor=""
  if "$BIN" help 2>/dev/null | grep -q '^  supervise'; then
    supervisor="$INSTALL_DIR/.agent-supervisor-$$"
    cp -p "$BIN" "$supervisor"
    "$supervisor" supervise --binary "$BIN" &
  else
    "$BIN" &
  fi
  child=$!
  trap 'kill -TERM "$child" 2>/dev/null || true; wait "$child" || true; if [[ -n "$supervisor" ]]; then rm -f "$supervisor"; fi; exit 130' INT TERM
  wait "$child" || result=$?
  if [[ -n "$supervisor" ]]; then rm -f "$supervisor"; fi
  return "$result"
}
if [[ "$ACTION" == run ]]; then run_node; exit $?; fi
mkdir "$INSTALL_DIR/.install-lock" 2>/dev/null || fail "another installer is running"
LOCKED=1
[[ ! -d "$INSTALL_DIR/.run-lock" ]] || fail "stop the foreground node with Ctrl+C before updating"
if [[ -z "$SERVER" && -f "$INSTALL_DIR/.server-url" ]]; then SERVER="$(cat "$INSTALL_DIR/.server-url")"; fi
if [[ -f "$INSTALL_DIR/.release-base" && -z "${HEPHAESTUS_AGENT_RELEASE_BASE:-}" && "$RELEASE_BASE" == https://github.com/qiwentaidi/Hephaestus-Agent/releases/download ]]; then
  RELEASE_BASE="$(cat "$INSTALL_DIR/.release-base")"
fi
[[ "$SERVER" =~ ^https?://[^[:space:]]+$ ]] || fail "provide --server with the node-accessible Hephaestus URL"
[[ "$RELEASE_BASE" =~ ^https?://[^[:space:]]+$ ]] || fail "invalid release download URL"
SERVER="${SERVER%/}"
RELEASE_BASE="${RELEASE_BASE%/}"
case "$(uname -s)" in Linux) OS=linux;; Darwin) OS=darwin;; *) fail "use install.bat on Windows";; esac
case "$(uname -m)" in x86_64|amd64) ARCH=amd64;; arm64|aarch64) ARCH=arm64;; *) fail "unsupported architecture";; esac
[[ "$SERVICE" -eq 0 || ( "$OS" == linux && "$(id -u)" -eq 0 ) ]] || fail "--service requires Linux and root"

download() {
  local status="" result=0 detail="" resource="${3:-download}" error_log="$2.download-error"
  if command -v curl >/dev/null 2>&1; then
    if status="$(curl -fsSL --retry 2 --connect-timeout 10 --max-time 300 --write-out '%{http_code}' "$1" -o "$2")"; then return 0;
    else result=$?; fi
    case "$result" in
      28) detail="request timed out; check network connectivity and proxy settings, then retry";;
      6) detail="DNS lookup failed; check DNS and proxy settings, then retry";;
      7) detail="cannot connect to the download server; check network connectivity and proxy settings";;
      18|52|55|56) detail="connection interrupted or reset; check network connectivity and proxy settings, then retry";;
      22)
        if [[ "$status" == 404 ]]; then detail="HTTP 404: requested file or endpoint was not found; check the server/version and release address";
        else detail="server returned HTTP $status; retry or check access to the download server"; fi;;
      23) detail="cannot write the downloaded file; check directory permissions and free disk space";;
      35|60) detail="TLS connection or certificate verification failed; check system time, certificates and proxy settings";;
      *) detail="download failed (curl exit $result, HTTP ${status:-unknown}); see the original error above";;
    esac
  elif command -v wget >/dev/null 2>&1; then
    if wget --server-response --timeout=300 -O "$2" "$1" 2>"$error_log"; then rm -f "$error_log"; return 0;
    else result=$?; fi
    status="$(awk '/^[[:space:]]*HTTP\// {code=$2} END {print code}' "$error_log")"
    if [[ "$result" == 8 && "$status" == 404 ]]; then detail="HTTP 404: requested file or endpoint was not found; check the server/version and release address";
    elif [[ "$result" == 8 ]]; then detail="server returned HTTP ${status:-unknown}; retry or check access to the download server";
    elif grep -qi 'timed out\|timeout' "$error_log"; then detail="request timed out; check network connectivity and proxy settings, then retry";
    else detail="download failed (wget exit $result); check network, DNS, proxy settings, directory permissions and disk space"; fi
    tail -n 3 "$error_log" >&2
  else fail "install curl or wget first"; fi
  fail "$resource: $detail"
}
TMP="$(mktemp -d)"
download "$SERVER/api/v1/agent/release?format=text" "$TMP/version" "compatible version query"
RECOMMENDED="$(tr -d '\r\n' < "$TMP/version")"
[[ "$RECOMMENDED" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || fail "invalid server version response"
[[ -z "$VERSION" || "$VERSION" == "$RECOMMENDED" ]] || fail "$VERSION is incompatible; server recommends $RECOMMENDED"
VERSION="$RECOMMENDED"
PACKAGE="hephaestus-agent-${OS}-${ARCH}.zip"
log "Downloading $VERSION ($OS/$ARCH)"
download "$RELEASE_BASE/$VERSION/$PACKAGE" "$TMP/package.zip" "release $VERSION package $PACKAGE"
download "$RELEASE_BASE/$VERSION/SHA256SUMS.txt" "$TMP/checksums" "release $VERSION checksum file"
EXPECTED="$(awk -v name="$PACKAGE" '$2 == name || $2 == "*" name {print $1}' "$TMP/checksums")"
[[ "$EXPECTED" =~ ^[a-fA-F0-9]{64}$ ]] || fail "missing or ambiguous package checksum"
if command -v sha256sum >/dev/null 2>&1; then ACTUAL="$(sha256sum "$TMP/package.zip" | awk '{print $1}')";
else ACTUAL="$(shasum -a 256 "$TMP/package.zip" | awk '{print $1}')"; fi
[[ "$(printf '%s' "$ACTUAL" | tr 'A-F' 'a-f')" == "$(printf '%s' "$EXPECTED" | tr 'A-F' 'a-f')" ]] || fail "checksum mismatch; installed node was not changed"
command -v unzip >/dev/null 2>&1 || fail "install unzip first"
# Extract only the expected root binary; do not allow archive paths to overwrite config.
unzip -p "$TMP/package.zip" "$APP" > "$TMP/$APP" || fail "binary missing from release package"
chmod 0755 "$TMP/$APP"
OUTPUT="$("$TMP/$APP" version)" || fail "binary cannot run on this host; installed node was not changed"
[[ "$OUTPUT" == "Hephaestus scan-agent $VERSION (cli mode)" ]] || fail "binary version does not match release $VERSION"

yaml_string() {
  [[ "$1" != *$'\n'* && "$1" != *$'\r'* ]] || fail "configuration values must be single-line"
  local value="$1" quote="'"
  value="${value//$quote/$quote$quote}"
  printf "'%s'" "$value"
}
ask() {
  local label="$1" fallback="$2" value
  [[ -r /dev/tty ]] || fail "provide --config FILE for non-interactive installation"
  while true; do
    printf '%s [%s]: ' "$label" "$fallback" > /dev/tty
    read -r value < /dev/tty || fail "configuration cancelled"
    REPLY="${value:-$fallback}"
    if valid_config_value "$REPLY"; then return; fi
    log "Invalid $label: use valid UTF-8 text on a single line; please re-enter this field."
  done
}
if [[ ! -f "$INSTALL_DIR/config.yaml" ]]; then
  if [[ -n "$CONFIG_SOURCE" ]]; then
    [[ -f "$CONFIG_SOURCE" ]] || fail "config file does not exist"
    cp "$CONFIG_SOURCE" "$TMP/config.yaml"
  else
    command -v iconv >/dev/null 2>&1 || fail "install iconv first to validate configuration encoding"
    log "Configure the same PostgreSQL database as the main server. Password is not echoed."
    ask 'PostgreSQL host (reachable from this node)' ''; DB_HOST="$REPLY"
    [[ -n "$DB_HOST" ]] || fail "PostgreSQL host is required"
    ask 'PostgreSQL port' 5432; DB_PORT="$REPLY"
    [[ "$DB_PORT" =~ ^[0-9]+$ && "$DB_PORT" -gt 0 && "$DB_PORT" -le 65535 ]] || fail "invalid port"
    ask 'Database user' postgres; DB_USER="$REPLY"
    while true; do
      printf 'Database password: ' > /dev/tty
      read -rs DB_PASSWORD < /dev/tty || fail "configuration cancelled"
      printf '\n' > /dev/tty
      if valid_config_value "$DB_PASSWORD"; then break; fi
      log "Invalid password encoding: use valid UTF-8 text on a single line; please re-enter the password."
    done
    ask 'Database name' hephaestus; DB_NAME="$REPLY"
    ask 'PostgreSQL sslmode (require / verify-full / disable)' disable; DB_SSLMODE="$REPLY"
    case "$DB_SSLMODE" in disable|require|verify-ca|verify-full) ;; *) fail "invalid sslmode";; esac
    ask 'Agent roles (scan / ai / ai,scan)' ai,scan; ROLES="$REPLY"
    case "$ROLES" in scan) ROLE_YAML="['scan']";; ai) ROLE_YAML="['ai']";; ai,scan|scan,ai) ROLE_YAML="['ai', 'scan']";; *) fail "invalid roles";; esac
    cat > "$TMP/config.yaml" <<EOF
database:
  type: postgres
  host: $(yaml_string "$DB_HOST")
  port: $(yaml_string "$DB_PORT")
  user: $(yaml_string "$DB_USER")
  password: $(yaml_string "$DB_PASSWORD")
  dbname: $(yaml_string "$DB_NAME")
  sslmode: $(yaml_string "$DB_SSLMODE")
agent:
  id: ''
  roles: $ROLE_YAML
  aiConcurrency: 4
  scanConcurrency: 3
  server:
    address: $(yaml_string "$SERVER")
EOF
    unset DB_PASSWORD
  fi
  validate_config_file "$TMP/config.yaml"
  install -m 0600 "$TMP/config.yaml" "$INSTALL_DIR/config.yaml"
fi
validate_config_file "$INSTALL_DIR/config.yaml"
mkdir -p "$INSTALL_DIR/config"
# Copy the installer for repeatable updates, including when launched from a pipe.
if [[ -f "${BASH_SOURCE[0]}" && "${BASH_SOURCE[0]}" != "$INSTALL_DIR/install.sh" ]]; then
  cp "${BASH_SOURCE[0]}" "$TMP/install.sh"
else
  download "$SERVER/api/v1/agent/install.sh" "$TMP/install.sh" "updater script"
fi
bash -n "$TMP/install.sh"

MANAGED=0
PREVIOUS_VERSION=""
if [[ -f "$INSTALL_DIR/.installed-version" ]]; then PREVIOUS_VERSION="$(cat "$INSTALL_DIR/.installed-version")"; fi
if [[ -f "$INSTALL_DIR/.service-name" ]]; then
  [[ "$(cat "$INSTALL_DIR/.service-name")" == "$SERVICE_NAME" ]] || fail "unknown managed service"
  MANAGED=1
  [[ "$(id -u)" -eq 0 ]] || fail "update this systemd installation with sudo"
  SERVICE_COMMAND="$(systemctl show "$SERVICE_NAME" -p ExecStart --value)"
  [[ "$SERVICE_COMMAND" == *"$BIN"* || "$SERVICE_COMMAND" == *"$INSTALL_DIR/run.sh"* ]] || fail "service points to a different installation"
  systemctl stop "$SERVICE_NAME" || fail "service did not acknowledge stop; binary was not replaced"
  STATE="$(systemctl show "$SERVICE_NAME" -p ActiveState --value)"
  [[ "$STATE" == inactive ]] || fail "service has not stopped; binary was not replaced"
fi
if [[ -f "$BIN" ]]; then cp -p "$BIN" "$INSTALL_DIR/$APP.previous"; fi
install -m 0755 "$TMP/$APP" "$INSTALL_DIR/.$APP.new"
mv -f "$INSTALL_DIR/.$APP.new" "$BIN"
install -m 0755 "$TMP/install.sh" "$INSTALL_DIR/install.sh"
printf '%s\n' "$SERVER" > "$INSTALL_DIR/.server-url"
printf '%s\n' "$RELEASE_BASE" > "$INSTALL_DIR/.release-base"
printf '%s\n' "$VERSION" > "$INSTALL_DIR/.installed-version"
cat > "$INSTALL_DIR/update.sh" <<'EOF'
#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
exec bash ./install.sh --update --install-dir "$PWD" "$@"
EOF
cat > "$INSTALL_DIR/run.sh" <<'EOF'
#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
exec bash ./install.sh --run --install-dir "$PWD" "$@"
EOF
chmod 0755 "$INSTALL_DIR/update.sh" "$INSTALL_DIR/run.sh"
if [[ "$SERVICE" -eq 1 ]]; then
  command -v systemctl >/dev/null 2>&1 || fail "systemd is required for --service"
  # systemd quoting: spaces and quotes in paths remain literal.
  UNIT_DIR="${INSTALL_DIR//\\/\\\\}"; UNIT_DIR="${UNIT_DIR//\"/\\\"}"; UNIT_DIR="${UNIT_DIR//%/%%}"
  cat > "/etc/systemd/system/$SERVICE_NAME.service" <<EOF
[Unit]
Description=Hephaestus Agent
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
WorkingDirectory="$UNIT_DIR"
Environment="HEPHAESTUS_CONFIG_DIR=$UNIT_DIR"
Environment="HEPHAESTUS_RESOURCE_DIR=$UNIT_DIR/config"
ExecStart="$UNIT_DIR/run.sh"
Restart=on-failure
RestartSec=5
TimeoutStopSec=90
SendSIGKILL=no
[Install]
WantedBy=multi-user.target
EOF
  printf '%s\n' "$SERVICE_NAME" > "$INSTALL_DIR/.service-name"
  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME"
  MANAGED=1
fi
log "Installed $VERSION. Configuration and resources are preserved in $INSTALL_DIR."
log "Next update: bash '$INSTALL_DIR/update.sh'"
if [[ "$START" -eq 1 ]]; then
  if [[ "$MANAGED" -eq 1 ]]; then
    if ! systemctl start "$SERVICE_NAME" || ! systemctl is-active --quiet "$SERVICE_NAME"; then
      if [[ -f "$INSTALL_DIR/$APP.previous" ]]; then
        mv -f "$INSTALL_DIR/$APP.previous" "$BIN"
        if [[ -n "$PREVIOUS_VERSION" ]]; then printf '%s\n' "$PREVIOUS_VERSION" > "$INSTALL_DIR/.installed-version";
        else rm -f "$INSTALL_DIR/.installed-version"; fi
        systemctl start "$SERVICE_NAME" || true
      fi
      fail "new service failed to start; previous binary restored when available; inspect journalctl -u $SERVICE_NAME"
    fi
    log "Service started; confirm the node heartbeat in Agent management."
  else
    rmdir "$INSTALL_DIR/.install-lock"; LOCKED=0
    run_node
  fi
fi
