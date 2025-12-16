#!/usr/bin/env bash
set -euxo pipefail

# RustFS Installation Script for Slicer VMs
# Based on https://github.com/rustfs/rustfs documentation

# Configuration
RUSTFS_DATA_DIR="/data/rustfs"
RUSTFS_LOG_DIR="/var/log/rustfs"
RUSTFS_PORT=9000
RUSTFS_CONSOLE_PORT=9001
RUSTFS_ACCESS_KEY="rustfsadmin"
RUSTFS_SECRET_KEY="rustfsadmin"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        ARCH="x86_64"
        ;;
    aarch64)
        ARCH="aarch64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Create rustfs system user
useradd -r -s /sbin/nologin rustfs || true

# Create required directories
mkdir -p /opt/rustfs
mkdir -p ${RUSTFS_DATA_DIR}/vol1
mkdir -p ${RUSTFS_DATA_DIR}/vol2
mkdir -p ${RUSTFS_LOG_DIR}
mkdir -p /etc/rustfs

# Set directory permissions
chown -R rustfs:rustfs /opt/rustfs ${RUSTFS_DATA_DIR} ${RUSTFS_LOG_DIR} /etc/rustfs
chmod 755 /opt/rustfs ${RUSTFS_DATA_DIR} ${RUSTFS_LOG_DIR}

# Download latest RustFS binary
RELEASE_URL="https://github.com/rustfs/rustfs/releases/latest/download/rustfs-linux-${ARCH}-musl.tar.gz"
echo "Downloading RustFS from ${RELEASE_URL}..."
curl -fsSL "${RELEASE_URL}" -o /tmp/rustfs.tar.gz
tar -xzf /tmp/rustfs.tar.gz -C /tmp
mv /tmp/rustfs /usr/local/bin/rustfs
chmod +x /usr/local/bin/rustfs
rm -f /tmp/rustfs.tar.gz

# Create systemd service file
cat > /etc/systemd/system/rustfs.service << 'EOF'
[Unit]
Description=RustFS Object Storage Server
Documentation=https://rustfs.com/docs/
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
NotifyAccess=main
User=rustfs
Group=rustfs

WorkingDirectory=/opt/rustfs

Environment=RUSTFS_ACCESS_KEY=rustfsadmin
Environment=RUSTFS_SECRET_KEY=rustfsadmin

ExecStart=/usr/local/bin/rustfs \
    --address 0.0.0.0:9000 \
    --volumes /data/rustfs/vol1,/data/rustfs/vol2 \
    --console-enable

StandardOutput=append:/var/log/rustfs/rustfs.log
StandardError=append:/var/log/rustfs/rustfs-err.log

LimitNOFILE=1048576
LimitNPROC=32768
TasksMax=infinity

Restart=always
RestartSec=10s

TimeoutStartSec=30s
TimeoutStopSec=30s

NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectClock=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictRealtime=true
ReadWritePaths=/data/rustfs /var/log/rustfs

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable rustfs
systemctl start rustfs

echo "RustFS installation complete!"
echo "  API endpoint: http://$(hostname -I | awk '{print $1}'):${RUSTFS_PORT}"
echo "  Console: http://$(hostname -I | awk '{print $1}'):${RUSTFS_CONSOLE_PORT}"
echo "  Credentials: ${RUSTFS_ACCESS_KEY} / ${RUSTFS_SECRET_KEY}"
