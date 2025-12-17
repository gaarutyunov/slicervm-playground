#!/usr/bin/env bash
set -euxo pipefail

# K3s Agent node bootstrap script
# Automatically joins the K3s cluster using embedded credentials

K3S_URL="{{K3S_URL}}"
K3S_TOKEN="{{K3S_TOKEN}}"

# Ensure required packages are available
apt-get update -qq
apt-get install -y -qq curl ca-certificates

# Create directory for k3s
mkdir -p /etc/rancher/k3s

# Install k3s agent and join the cluster
curl -sfL https://get.k3s.io | K3S_URL="${K3S_URL}" K3S_TOKEN="${K3S_TOKEN}" sh -s - agent

echo "K3s agent installed and joined cluster"
