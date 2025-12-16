#!/bin/bash
#
# RustFS Installation Script (Non-interactive)
# Based on https://rustfs.com/install_rustfs.sh
set -euo pipefail

apt-get update && apt-get install -y unzip

# --- Functions ---
err() { echo -e "\033[1;31m[ERROR]\033[0m $1" >&2; exit 1; }
info() { echo -e "\033[1;32m[INFO]\033[0m $1"; }

# --- Global Variables ---
RUSTFS_SERVICE_FILE="/usr/lib/systemd/system/rustfs.service"
RUSTFS_CONFIG_FILE="/etc/default/rustfs"
RUSTFS_BIN_PATH="/usr/local/bin/rustfs"
LOG_DIR="/var/logs/rustfs"
DOWNLOAD_CMD=""
PKG_GNU=""
PKG_MUSL=""
USE_MUSL=1
PORT_CMD=""

# --- Configuration (predefined values) ---
RUSTFS_PORT=9000
CONSOLE_PORT=9001
RUSTFS_VOLUME="/data/rustfs0"

# --- Pre-flight Checks ---
run_preflight_checks() {
    if [[ $EUID -ne 0 ]]; then
      err "This script must be run as root."
    fi

    REQUIRED_CMDS=(unzip systemctl mktemp grep sort find)
    PORT_CHECK_CMDS=(lsof netstat ss)
    DOWNLOAD_CMDS=(wget curl)
    MISSING_CMDS=()

    for cmd in "${REQUIRED_CMDS[@]}"; do
      command -v "$cmd" >/dev/null 2>&1 || MISSING_CMDS+=("$cmd")
    done
    for cmd in "${PORT_CHECK_CMDS[@]}"; do
      if command -v "$cmd" >/dev/null 2>&1; then PORT_CMD="$cmd"; break; fi
    done
    for cmd in "${DOWNLOAD_CMDS[@]}"; do
      if command -v "$cmd" >/dev/null 2>&1; then DOWNLOAD_CMD="$cmd"; break; fi
    done
    [[ ${#MISSING_CMDS[@]} -ne 0 ]] && err "Missing commands: ${MISSING_CMDS[*]}"
    [[ -z "$PORT_CMD" ]] && err "No port check command found (lsof/netstat/ss)"
    [[ -z "$DOWNLOAD_CMD" ]] && err "No download command found (wget/curl)"
    info "All required commands are present."

    [[ "$(uname -s)" != "Linux" ]] && err "This script is only for Linux."
    ARCH=$(uname -m)
    case "$ARCH" in
      x86_64)
        PKG_GNU="https://dl.rustfs.com/artifacts/rustfs/release/rustfs-linux-x86_64-gnu-latest.zip"
        PKG_MUSL="https://dl.rustfs.com/artifacts/rustfs/release/rustfs-linux-x86_64-musl-latest.zip"
        ;;
      aarch64)
        PKG_GNU="https://dl.rustfs.com/artifacts/rustfs/release/rustfs-linux-aarch64-gnu-latest.zip"
        PKG_MUSL="https://dl.rustfs.com/artifacts/rustfs/release/rustfs-linux-aarch64-musl-latest.zip"
        ;;
      *) err "Unsupported CPU architecture: $ARCH";;
    esac
    info "OS and architecture check passed: $ARCH."
    info "Defaulting to MUSL build for maximum compatibility."
}

# --- Download and Install Binary ---
download_and_install_binary() {
    info "Starting download and installation of RustFS binary..."
    ORIG_DIR=$(pwd)
    TMP_DIR=$(mktemp -d) || err "Failed to create temp dir."
    cd "$TMP_DIR" || err "Failed to enter temp dir."

    local PKG_URL
    if [[ $USE_MUSL -eq 1 ]]; then
      PKG_URL="$PKG_MUSL"
      info "Using MUSL build."
    else
      PKG_URL="$PKG_GNU"
      info "Using GNU build."
    fi

    info "Downloading RustFS package from $PKG_URL..."
    if [[ "$DOWNLOAD_CMD" == "wget" ]]; then
      wget -O rustfs.zip "$PKG_URL" || err "Download failed."
    else
      curl -L -o rustfs.zip "$PKG_URL" || err "Download failed."
    fi

    unzip rustfs.zip || err "Failed to unzip package."
    RUSTFS_BIN_FOUND=$(find . -type f -name rustfs | head -n1)
    [[ -z "$RUSTFS_BIN_FOUND" ]] && err "rustfs binary not found in package."

    cp "$RUSTFS_BIN_FOUND" "$RUSTFS_BIN_PATH" || err "Failed to copy binary to $RUSTFS_BIN_PATH."
    chmod +x "$RUSTFS_BIN_PATH" || err "Failed to set execute permission."

    cd "$ORIG_DIR" >/dev/null || true
    rm -rf "$TMP_DIR"
    info "RustFS binary installed successfully."
}

# --- Installation Logic ---
install_rustfs() {
    info "Starting RustFS installation..."

    if [ -f "$RUSTFS_BIN_PATH" ]; then
        err "RustFS appears to be already installed."
    fi

    # Port checks
    local PORT_OCCUPIED=0
    case "$PORT_CMD" in
      lsof) lsof -i :$RUSTFS_PORT >/dev/null 2>&1 && PORT_OCCUPIED=1 ;;
      netstat) netstat -ltn | grep -q ":$RUSTFS_PORT[[:space:]]" && PORT_OCCUPIED=1 ;;
      ss) ss -ltn | grep -q ":$RUSTFS_PORT[[:space:]]" && PORT_OCCUPIED=1 ;;
    esac
    [[ $PORT_OCCUPIED -eq 1 ]] && err "Port $RUSTFS_PORT is already in use."
    info "Port $RUSTFS_PORT is available."

    PORT_OCCUPIED=0
    case "$PORT_CMD" in
      lsof) lsof -i :$CONSOLE_PORT >/dev/null 2>&1 && PORT_OCCUPIED=1 ;;
      netstat) netstat -ltn | grep -q ":$CONSOLE_PORT[[:space:]]" && PORT_OCCUPIED=1 ;;
      ss) ss -ltn | grep -q ":$CONSOLE_PORT[[:space:]]" && PORT_OCCUPIED=1 ;;
    esac
    [[ $PORT_OCCUPIED -eq 1 ]] && err "Port $CONSOLE_PORT is already in use."
    info "Port $CONSOLE_PORT is available."

    # Data directory
    [[ ! -d "$RUSTFS_VOLUME" ]] && mkdir -p "$RUSTFS_VOLUME" || true
    [[ ! -d "$RUSTFS_VOLUME" ]] && err "Failed to create directory $RUSTFS_VOLUME."
    info "Data directory ready: $RUSTFS_VOLUME."

    # Log directory
    [[ ! -d "$LOG_DIR" ]] && mkdir -p "$LOG_DIR" || true
    [[ ! -d "$LOG_DIR" ]] && err "Failed to create log directory $LOG_DIR."
    info "Log directory ready: $LOG_DIR."

    download_and_install_binary

    # systemd Service File
    cat <<EOF > "$RUSTFS_SERVICE_FILE" || err "Failed to write systemd service file."
[Unit]
Description=RustFS Object Storage Server
Documentation=https://rustfs.com/docs/
After=network-online.target
Wants=network-online.target
[Service]
Type=notify
NotifyAccess=main
User=root
Group=root
WorkingDirectory=/usr/local
EnvironmentFile=-$RUSTFS_CONFIG_FILE
ExecStart=$RUSTFS_BIN_PATH  \$RUSTFS_VOLUMES
LimitNOFILE=1048576
LimitNPROC=32768
TasksMax=infinity
Restart=always
RestartSec=10s
OOMScoreAdjust=-1000
SendSIGKILL=no
TimeoutStartSec=30s
TimeoutStopSec=30s
NoNewPrivileges=true
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectClock=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictRealtime=true
StandardOutput=append:$LOG_DIR/rustfs.log
StandardError=append:$LOG_DIR/rustfs-err.log
[Install]
WantedBy=multi-user.target
EOF
    info "systemd service file created at $RUSTFS_SERVICE_FILE."

    # RustFS Config File
    cat <<EOF > "$RUSTFS_CONFIG_FILE" || err "Failed to write config file."
RUSTFS_ACCESS_KEY=rustfsadmin
RUSTFS_SECRET_KEY=rustfsadmin
RUSTFS_VOLUMES="$RUSTFS_VOLUME"
RUSTFS_ADDRESS=":$RUSTFS_PORT"
RUSTFS_CONSOLE_ADDRESS=":$CONSOLE_PORT"
RUSTFS_CONSOLE_ENABLE=true
RUSTFS_OBS_LOGGER_LEVEL=error
RUSTFS_OBS_LOG_DIRECTORY="$LOG_DIR/"
EOF
    info "RustFS config file created at $RUSTFS_CONFIG_FILE."

    systemctl daemon-reload || err "systemctl daemon-reload failed."
    systemctl enable rustfs || err "systemctl enable rustfs failed."
    systemctl start rustfs || err "systemctl start rustfs failed."
    info "RustFS service enabled and started."

    echo "RustFS has been installed and started successfully!"
    echo "Service port: $RUSTFS_PORT, Console port: $CONSOLE_PORT, Data directory: $RUSTFS_VOLUME"
}

# --- Main ---
run_preflight_checks
install_rustfs
