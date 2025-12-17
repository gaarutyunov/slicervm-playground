#!/usr/bin/env bash
set -euxo pipefail

# K3s Control Plane node preparation script
# k3sup will install K3s after the VM is ready

# Ensure required packages are available
apt-get update -qq
apt-get install -y -qq curl ca-certificates

# Create directory for k3s
mkdir -p /etc/rancher/k3s

# The control plane setup will be completed via k3sup from the workstation:
# k3sup-pro plan --user ubuntu ./devices.json
# k3sup-pro apply
