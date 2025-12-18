# Slicer Experiments

Experiments and automation tooling for [SlicerVM](https://slicervm.com/) - a lightweight VM orchestrator built on Firecracker.

## Overview

This repository contains Mage targets for deploying various workloads on Slicer VMs:

- **BuildKit** - Remote container image builder
- **OpenFaaS Edge** - Serverless functions platform
- **RustFS** - High-performance S3-compatible object storage
- **PostgreSQL** - Database server (configured for Gitea)
- **Gitea** - Self-hosted Git service with Actions support
- **Gitea Runner** - CI/CD runner for Gitea Actions
- **K3s** - Autoscaling Kubernetes cluster with cluster-autoscaler
- **Crossplane** - Kubernetes-native infrastructure management
- **Grafana Stack** - Prometheus, Grafana, and Alertmanager for monitoring
- **cert-manager** - Automatic TLS certificate management

## Prerequisites

- Go 1.21+
- [Mage](https://magefile.org/) build tool
- Running Slicer instance
- Slicer API token

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SLICER_URL` | Slicer API endpoint | `http://127.0.0.1:8080` |
| `SLICER_TOKEN` | Authentication token | - |
| `SLICER_HOST_GROUP` | Host group for VMs | `api` |
| `GITHUB_USER` | GitHub username for SSH key import | - |
| `SSH_KEY_PATH` | Path to SSH public key | `~/.ssh/id_ed25519.pub` |

## Usage

### List Available Targets

```bash
mage -l
```

### BuildKit

```bash
mage buildkit:deploy              # Create a new BuildKit VM
mage buildkit:list                # List all BuildKit VMs
mage buildkit:delete <hostname>   # Delete a BuildKit VM
mage buildkit:logs <hostname>     # Show serial console logs
```

### OpenFaaS Edge

```bash
mage openfaas:deploy              # Create a new OpenFaaS Edge VM
mage openfaas:list                # List all OpenFaaS VMs
mage openfaas:delete <hostname>   # Delete an OpenFaaS VM
mage openfaas:logs <hostname>     # Show serial console logs
```

### RustFS

```bash
mage rustfs:deploy                # Create a new RustFS VM
mage rustfs:list                  # List all RustFS VMs
mage rustfs:delete <hostname>     # Delete a RustFS VM
mage rustfs:logs <hostname>       # Show serial console logs
```

### PostgreSQL

```bash
mage postgres:deploy              # Create a new PostgreSQL VM
mage postgres:list                # List all PostgreSQL VMs
mage postgres:delete <hostname>   # Delete a PostgreSQL VM
mage postgres:logs <hostname>     # Show serial console logs
```

### Gitea Stack

Deploy a complete Gitea instance with PostgreSQL database, S3 storage, and CI/CD runner.

#### 1. Deploy Dependencies

```bash
# Deploy RustFS for S3 storage (note the access/secret keys)
mage rustfs:deploy

# Deploy PostgreSQL database (note the password)
mage postgres:deploy
```

#### 2. Deploy Gitea

```bash
GITEA_DB_PASS=<postgres-password> \
GITEA_S3_ACCESS_KEY=<rustfs-access-key> \
GITEA_S3_SECRET_KEY=<rustfs-secret-key> \
mage gitea:deploy
```

#### 3. Complete Setup

1. Open Gitea web UI (http://<gitea-ip>:3000)
2. Database and S3 storage are pre-configured
3. Create your admin account
4. Get runner token from Admin → Actions → Runners

#### 4. Deploy Runner

```bash
RUNNER_TOKEN=<token-from-gitea> mage runner:deploy
```

#### Gitea Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GITEA_DB_PASS` | PostgreSQL password | (required) |
| `GITEA_DB_HOST` | PostgreSQL host | (auto-detected) |
| `GITEA_S3_ACCESS_KEY` | RustFS access key | (required) |
| `GITEA_S3_SECRET_KEY` | RustFS secret key | (required) |
| `GITEA_S3_ENDPOINT` | S3 endpoint | (auto-detected) |
| `RUNNER_TOKEN` | Runner registration token | (required) |
| `GITEA_URL` | Gitea instance URL | (auto-detected) |

### K3s (Autoscaling Kubernetes)

Deploy an autoscaling K3s cluster on Slicer VMs.

#### Initial Setup

1. Deploy a control plane node:
```bash
mage k3s:deployCP
```

2. Install K3s on the control plane using [k3sup](https://github.com/alexellis/k3sup):
```bash
mage k3s:devices > devices.json
k3sup-pro plan --user ubuntu ./devices.json
k3sup-pro apply
```

3. Create the K3s node token secret (required for agent nodes and autoscaler):
```bash
# From your workstation (requires SSH access to control plane)
ssh ubuntu@<control-plane-ip> 'sudo cat /var/lib/rancher/k3s/server/node-token' | \
  kubectl create secret generic k3s-node-token -n kube-system --from-file=token=/dev/stdin
```

#### Deploy Agent Nodes

```bash
mage k3s:deployAgent              # Deploy agent (auto-joins cluster)
mage k3s:nodes                    # List K8s nodes
```

#### Cluster Autoscaler

```bash
mage k3s:autoscalerInstall        # Install cluster autoscaler
mage k3s:autoscalerStatus         # Check autoscaler status
mage k3s:autoscalerLogs           # View autoscaler logs
mage k3s:autoscalerUninstall      # Remove autoscaler
```

#### Other K3s Commands

```bash
mage k3s:listCP                   # List control plane VMs
mage k3s:listAgents               # List agent VMs
mage k3s:logsCP <hostname>        # Show CP serial logs
mage k3s:deleteCP <hostname>      # Delete control plane VM
mage k3s:deleteAgent <hostname>   # Delete agent VM
```

### Crossplane

Install Crossplane on your K3s cluster for Kubernetes-native infrastructure management.

```bash
mage crossplane:install           # Install Crossplane via Helm
mage crossplane:uninstall         # Uninstall Crossplane
```

### Grafana Stack (Prometheus + Grafana + Alertmanager)

Deploy the kube-prometheus-stack for monitoring your K3s cluster and external hosts.

```bash
mage grafana:install              # Install monitoring stack
mage grafana:uninstall            # Uninstall monitoring stack
mage grafana:status               # Show pod/deployment status
mage grafana:services             # Show service endpoints
mage grafana:password             # Get Grafana admin password
mage grafana:logs [pod-name]      # View pod logs
```

#### Accessing Grafana

```bash
# Get credentials and endpoints
mage grafana:password
mage grafana:services

# Access Grafana at http://<k3s-node-ip>:<grafana-nodeport>
# Default user: admin
```

#### Pre-installed Dashboards

- **Node Exporter Full** (ID 1860) - Comprehensive Linux server monitoring (CPU, memory, disk, network)
- Built-in Kubernetes dashboards for cluster monitoring

#### Monitoring External Hosts

Prometheus accepts remote write at `http://<k3s-node-ip>:30090/api/v1/write`.

Install Grafana Alloy on your external host:

```bash
# Install Alloy
curl -fsSL https://apt.grafana.com/gpg.key | gpg --dearmor -o /etc/apt/keyrings/grafana.gpg
echo "deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main" > /etc/apt/sources.list.d/grafana.list
apt update && apt install -y alloy

# Configure /etc/alloy/config.alloy
cat > /etc/alloy/config.alloy << 'EOF'
prometheus.exporter.unix "host" {
  include_exporter_metrics = true
}

prometheus.scrape "node" {
  targets    = prometheus.exporter.unix.host.targets
  forward_to = [prometheus.remote_write.k3s.receiver]
}

prometheus.remote_write "k3s" {
  endpoint {
    url = "http://<k3s-node-ip>:30090/api/v1/write"
  }
  external_labels = {
    instance = "my-server",
    job      = "node-exporter",
  }
}
EOF

systemctl enable --now alloy
```

#### Grafana Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GRAFANA_PASSWORD` | Admin password | (auto-generated) |
| `GRAFANA_INGRESS_HOST` | Enable ingress with hostname | - |
| `GRAFANA_TLS` | Enable TLS via cert-manager | `false` |
| `PROMETHEUS_RETENTION` | Data retention in days | `10` |
| `PROMETHEUS_STORAGE` | Storage size | `10Gi` |

### cert-manager

Install cert-manager for automatic TLS certificate management.

```bash
mage certManager:install          # Install cert-manager
mage certManager:uninstall        # Uninstall cert-manager
mage certManager:status           # Show status
mage certManager:clusterIssuer    # Create Let's Encrypt ClusterIssuer
mage certManager:clusterIssuerList # List ClusterIssuers
```

#### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ACME_EMAIL` | Email for Let's Encrypt | (required for ClusterIssuer) |
| `ACME_STAGING` | Use staging server | `false` |

## VM Specifications

| Workload | vCPU | RAM | Storage |
|----------|------|-----|---------|
| BuildKit | 2 | 4 GB | 25 GB |
| OpenFaaS | 2 | 4 GB | 25 GB |
| RustFS | 2 | 4 GB | 25 GB |
| PostgreSQL | 2 | 4 GB | 25 GB |
| Gitea | 2 | 4 GB | 25 GB |
| Runner | 2 | 4 GB | 25 GB |
| K3s CP | 2 | 4 GB | 25 GB |
| K3s Agent | 2 | 4 GB | 25 GB |

## License

MIT
