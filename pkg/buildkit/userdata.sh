#!/usr/bin/env bash
set -euxo pipefail

# BuildKit installation and configuration script
# This script installs buildkitd, configures the buildkit group, and creates a systemd service

# Install buildkit
arkade system install buildkitd

# Add a buildkit group
sudo groupadd buildkit

# Add ubuntu user to buildkit group
sudo usermod -aG buildkit ubuntu

# Systemd service for buildkit (daemonized under systemd)
cat <<'EOF' | sudo tee /etc/systemd/system/buildkitd.service > /dev/null
[Unit]
Description=BuildKit Daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/buildkitd --addr unix:///run/buildkit/buildkitd.sock --group buildkit
Restart=always
User=root

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now buildkitd
