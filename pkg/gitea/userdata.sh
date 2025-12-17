#!/usr/bin/env bash
set -euxo pipefail

# Gitea installation via snap
# Note: Gitea snap is strictly confined, some features may be limited

# Database configuration (injected by deployer)
DB_HOST="{{DB_HOST}}"
DB_PORT="{{DB_PORT}}"
DB_NAME="{{DB_NAME}}"
DB_USER="{{DB_USER}}"
DB_PASS="{{DB_PASS}}"

# S3 Storage configuration (injected by deployer)
S3_ENDPOINT="{{S3_ENDPOINT}}"
S3_ACCESS_KEY="{{S3_ACCESS_KEY}}"
S3_SECRET_KEY="{{S3_SECRET_KEY}}"
S3_BUCKET="{{S3_BUCKET}}"
S3_USE_SSL="{{S3_USE_SSL}}"

# Install snapd if not present
export DEBIAN_FRONTEND=noninteractive
if ! command -v snap &> /dev/null; then
    sudo -E apt-get update
    sudo -E apt-get install -y snapd
fi

# Start snapd and wait for socket to be available
sudo systemctl enable snapd.socket
sudo systemctl start snapd.socket
sudo systemctl enable snapd
sudo systemctl start snapd

# Wait for snapd socket to be ready
echo "Waiting for snapd socket..."
for i in {1..30}; do
    if [ -S /run/snapd.socket ]; then
        echo "snapd socket ready"
        break
    fi
    sleep 1
done

# Install Gitea via snap
sudo snap install gitea

# Wait for snap to initialize and create config directory
sleep 5

# Get Gitea IP
GITEA_IP=$(hostname -I | awk '{print $1}')

# Gitea snap config path
GITEA_CONF_DIR="/var/snap/gitea/common/conf"
GITEA_APP_INI="${GITEA_CONF_DIR}/app.ini"

# Ensure config directory exists
sudo mkdir -p "${GITEA_CONF_DIR}"

# Create app.ini with database and storage pre-configured
# Note: RUN_USER is omitted - let snap handle the user
cat <<EOF | sudo tee "${GITEA_APP_INI}"
APP_NAME = Gitea
RUN_MODE = prod

[server]
DOMAIN = ${GITEA_IP}
HTTP_PORT = 3000
ROOT_URL = http://${GITEA_IP}:3000/
DISABLE_SSH = false
SSH_PORT = 22
LFS_START_SERVER = true

[database]
DB_TYPE = postgres
HOST = ${DB_HOST}:${DB_PORT}
NAME = ${DB_NAME}
USER = ${DB_USER}
PASSWD = ${DB_PASS}
SSL_MODE = disable

[storage]
STORAGE_TYPE = minio
MINIO_ENDPOINT = ${S3_ENDPOINT}
MINIO_ACCESS_KEY_ID = ${S3_ACCESS_KEY}
MINIO_SECRET_ACCESS_KEY = ${S3_SECRET_KEY}
MINIO_BUCKET = ${S3_BUCKET}
MINIO_USE_SSL = ${S3_USE_SSL}

[security]
INSTALL_LOCK = false

[log]
MODE = console
LEVEL = info
EOF

sudo chmod 640 "${GITEA_APP_INI}"

# Restart Gitea to pick up config
sudo snap restart gitea

# Save connection info
cat <<EOF | sudo tee /home/ubuntu/gitea-info.txt
Gitea Instance
==============
Web UI: http://${GITEA_IP}:3000
SSH: ssh://git@${GITEA_IP}:22

Database Configuration (pre-configured):
  Type: PostgreSQL
  Host: ${DB_HOST}:${DB_PORT}
  Database: ${DB_NAME}
  Username: ${DB_USER}
  Password: ${DB_PASS}
  SSL Mode: disable

S3 Storage Configuration (pre-configured):
  Endpoint: ${S3_ENDPOINT}
  Access Key: ${S3_ACCESS_KEY}
  Secret Key: ${S3_SECRET_KEY}
  Bucket: ${S3_BUCKET}
  Use SSL: ${S3_USE_SSL}

Config file: ${GITEA_APP_INI}

First-time Setup:
  1. Open http://${GITEA_IP}:3000 in your browser
  2. Database is pre-configured - just verify the settings
  3. Create your admin account
  4. Complete the setup wizard
EOF

sudo chown ubuntu:ubuntu /home/ubuntu/gitea-info.txt
sudo chmod 600 /home/ubuntu/gitea-info.txt

echo "Gitea installation complete!"
echo "Database and S3 storage pre-configured"
echo "Access the web UI at http://${GITEA_IP}:3000"
echo "Configuration saved to /home/ubuntu/gitea-info.txt"
