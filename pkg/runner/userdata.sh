#!/usr/bin/env bash
set -euxo pipefail

# Gitea Runner (act_runner) installation
# Requires Docker for running jobs in containers

# Configuration (injected by deployer)
GITEA_URL="{{GITEA_URL}}"
RUNNER_TOKEN="{{RUNNER_TOKEN}}"
RUNNER_NAME="{{RUNNER_NAME}}"
RUNNER_LABELS="{{RUNNER_LABELS}}"
RUNNER_VERSION="{{RUNNER_VERSION}}"

export DEBIAN_FRONTEND=noninteractive

# Install Docker
sudo -E apt-get update
sudo -E apt-get install -y ca-certificates curl gnupg

sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo -E apt-get update
sudo -E apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# Add ubuntu user to docker group
sudo usermod -aG docker ubuntu

# Start Docker
sudo systemctl enable docker
sudo systemctl start docker

# Download act_runner
RUNNER_DIR="/opt/act_runner"
sudo mkdir -p "${RUNNER_DIR}"
cd "${RUNNER_DIR}"

ARCH=$(uname -m)
case ${ARCH} in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
esac

sudo curl -L -o act_runner "https://dl.gitea.com/act_runner/${RUNNER_VERSION}/act_runner-${RUNNER_VERSION}-linux-${ARCH}"
sudo chmod +x act_runner

# Set runner name to hostname if not specified
if [ -z "${RUNNER_NAME}" ]; then
    RUNNER_NAME=$(hostname)
fi

# Register runner with Gitea
sudo ./act_runner register \
    --instance "${GITEA_URL}" \
    --token "${RUNNER_TOKEN}" \
    --name "${RUNNER_NAME}" \
    --labels "${RUNNER_LABELS}" \
    --no-interactive

# Create systemd service
cat <<EOF | sudo tee /etc/systemd/system/act_runner.service
[Unit]
Description=Gitea Actions Runner
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=root
WorkingDirectory=${RUNNER_DIR}
ExecStart=${RUNNER_DIR}/act_runner daemon
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable act_runner
sudo systemctl start act_runner

# Get runner IP
RUNNER_IP=$(hostname -I | awk '{print $1}')

# Save info
cat <<EOF | sudo tee /home/ubuntu/runner-info.txt
Gitea Runner
============
Gitea Instance: ${GITEA_URL}
Runner Name: ${RUNNER_NAME}
Runner Labels: ${RUNNER_LABELS}
Runner Version: ${RUNNER_VERSION}

Status: sudo systemctl status act_runner
Logs: sudo journalctl -u act_runner -f
Restart: sudo systemctl restart act_runner

Config: ${RUNNER_DIR}/.runner
EOF

sudo chown ubuntu:ubuntu /home/ubuntu/runner-info.txt
sudo chmod 600 /home/ubuntu/runner-info.txt

echo "Gitea Runner installation complete!"
echo "Runner registered with ${GITEA_URL}"
echo "Check status: sudo systemctl status act_runner"
