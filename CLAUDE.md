# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go project for managing Slicer VMs using the Slicer SDK. The project provides automation tooling to recreate environments from scratch and create VMs with presets. A Magefile is used to manage VM lifecycle operations.

**Documentation reference**: The `docs.slicervm.com/` directory contains the official Slicer documentation (cloned locally). Key references:
- `docs/examples/buildkit.md` - BuildKit VM deployment pattern
- `docs/reference/api.md` - REST API endpoints
- `docs/tasks/userdata.md` - VM bootstrap scripts

## Build Commands

```bash
# List all mage targets
mage -l

# BuildKit targets
mage buildkit:deploy              # Create a new BuildKit VM
mage buildkit:list                # List all BuildKit VMs
mage buildkit:delete <hostname>   # Delete a BuildKit VM
mage buildkit:logs <hostname>     # Show serial console logs
mage buildkit:userdata            # Print the userdata script
mage buildkit:yaml                # Generate Slicer config YAML

# OpenFaaS Edge targets
mage openfaas:deploy              # Create a new OpenFaaS Edge VM
mage openfaas:list                # List all OpenFaaS VMs
mage openfaas:delete <hostname>   # Delete an OpenFaaS VM
mage openfaas:logs <hostname>     # Show serial console logs
mage openfaas:userdata            # Print the userdata script
mage openfaas:yaml                # Generate Slicer config YAML

# RustFS (S3-compatible storage) targets
mage rustfs:deploy                # Create a new RustFS VM
mage rustfs:list                  # List all RustFS VMs
mage rustfs:delete <hostname>     # Delete a RustFS VM
mage rustfs:logs <hostname>       # Show serial console logs
mage rustfs:userdata              # Print the userdata script

# PostgreSQL targets
mage postgres:deploy              # Create a new PostgreSQL VM (Gitea-ready)
mage postgres:list                # List all PostgreSQL VMs
mage postgres:delete <hostname>   # Delete a PostgreSQL VM
mage postgres:logs <hostname>     # Show serial console logs
mage postgres:userdata            # Print the userdata script
mage postgres:yaml                # Generate Slicer config YAML

# Gitea targets
mage gitea:deploy                 # Create a new Gitea VM (requires postgres, rustfs)
mage gitea:list                   # List all Gitea VMs
mage gitea:delete <hostname>      # Delete a Gitea VM
mage gitea:logs <hostname>        # Show serial console logs
mage gitea:userdata               # Print the userdata script
mage gitea:yaml                   # Generate Slicer config YAML

# Gitea Runner targets
mage runner:deploy                # Create a new Gitea Actions Runner VM
mage runner:list                  # List all Runner VMs
mage runner:delete <hostname>     # Delete a Runner VM
mage runner:logs <hostname>       # Show serial console logs
mage runner:userdata              # Print the userdata script
mage runner:yaml                  # Generate Slicer config YAML

# K3s Kubernetes cluster targets
mage k3s:deploy                   # Create a new K3s cluster (control plane + workers)
mage k3s:list                     # List all K3s VMs
mage k3s:deleteCluster            # Delete entire K3s cluster
mage k3s:logsCP                   # Show control plane logs
mage k3s:scaleWorkers <count>     # Scale worker nodes

# Crossplane targets
mage crossplane:install           # Install Crossplane in K3s cluster
mage crossplane:uninstall         # Uninstall Crossplane

# Run tests
go test ./...

# Run a single test
go test -run TestName ./path/to/package
```

## Architecture

### Slicer SDK Usage

The project uses `github.com/slicervm/sdk` for programmatic VM management:

```go
import sdk "github.com/slicervm/sdk"

// Create client
client := sdk.NewSlicerClient(baseURL, token, "user-agent", nil)

// Create VM in a host group
client.CreateNode(ctx, "hostgroup-name", sdk.SlicerCreateNodeRequest{
    RamGB:    4,
    CPUs:     2,
    Userdata: "#!/bin/bash\n...",
})

// Delete VM
client.DeleteVM(ctx, "hostgroup-name", "hostname")
```

### Package Structure

- `pkg/buildkit/` - BuildKit VM deployment with embedded userdata script
- `pkg/openfaas/` - OpenFaaS Edge VM deployment with embedded userdata script
- `pkg/rustfs/` - RustFS (S3-compatible storage) VM deployment
- `pkg/postgres/` - PostgreSQL VM deployment (configured for Gitea)
- `pkg/gitea/` - Gitea VM deployment via snap with pre-configured database/S3
- `pkg/runner/` - Gitea Actions Runner VM deployment with Docker + act_runner
- `pkg/k3s/` - K3s Kubernetes cluster deployment (control plane + workers)
- `pkg/crossplane/` - Crossplane installation using Helm Go SDK
- `magefile.go` - Mage targets that import packages for easier testing

Mage namespaces expose targets for:
- VM lifecycle (create, delete, list, logs)
- Preset deployments (BuildKit, OpenFaaS, RustFS, PostgreSQL, Gitea, Runner)
- Kubernetes cluster management (K3s, Crossplane)
- Auto-detection of dependent VMs (postgres→gitea, rustfs→gitea, gitea→runner)

### VM Configuration Pattern

VMs are configured via YAML files with host groups:

```yaml
config:
  host_groups:
  - name: buildkit
    vcpu: 4
    ram_gb: 8
    storage_size: 25G
    userdata_file: ./buildkit.sh
  github_user: <your-github-username>
  image: "ghcr.io/openfaasltd/slicer-systemd:5.10.240-x86_64-latest"
  hypervisor: firecracker
```

### Userdata Scripts

Bootstrap scripts go in userdata files and run on first boot. Example pattern for BuildKit:

```bash
#!/usr/bin/env bash
set -euxo pipefail
arkade system install buildkitd
sudo groupadd buildkit
sudo usermod -aG buildkit ubuntu
# ... systemd service setup
```

## Environment Variables

### Core
- `SLICER_URL` - Slicer API base URL (default: `http://127.0.0.1:8080`)
- `SLICER_TOKEN` - Auth token from `/var/lib/slicer/auth/token`
- `SLICER_HOST_GROUP` - Host group for VM operations (default: `api`)
- `GITHUB_USER` - GitHub username for SSH key import

### Gitea Deployment
- `GITEA_DB_PASS` - PostgreSQL password (required)
- `GITEA_DB_HOST` - PostgreSQL host (auto-detected from postgres VM)
- `GITEA_DB_PORT` - PostgreSQL port (default: 5432)
- `GITEA_DB_NAME` - Database name (default: giteadb)
- `GITEA_DB_USER` - Database user (default: gitea)
- `GITEA_S3_ACCESS_KEY` - RustFS/MinIO access key (required)
- `GITEA_S3_SECRET_KEY` - RustFS/MinIO secret key (required)
- `GITEA_S3_ENDPOINT` - S3 endpoint (auto-detected from rustfs VM)
- `GITEA_S3_BUCKET` - S3 bucket name (default: gitea)

### Runner Deployment
- `RUNNER_TOKEN` - Gitea runner registration token (required, from Gitea admin)
- `GITEA_URL` - Gitea instance URL (auto-detected from gitea VM)
- `RUNNER_NAME` - Runner name (default: hostname)
- `RUNNER_LABELS` - Runner labels (default: ubuntu-latest, ubuntu-22.04, ubuntu-20.04)
- `RUNNER_VERSION` - act_runner version (default: 0.2.11)

## Deployment Workflows

### Gitea Stack (PostgreSQL + RustFS + Gitea + Runner)

```bash
# 1. Deploy RustFS for S3 storage
mage rustfs:deploy
# Note the access key and secret key from output

# 2. Deploy PostgreSQL database
mage postgres:deploy
# Note the password from output

# 3. Deploy Gitea (auto-detects postgres and rustfs)
GITEA_DB_PASS=<postgres-password> \
GITEA_S3_ACCESS_KEY=<rustfs-access-key> \
GITEA_S3_SECRET_KEY=<rustfs-secret-key> \
mage gitea:deploy

# 4. Complete Gitea setup in browser, get runner token from admin panel

# 5. Deploy Runner (auto-detects gitea)
RUNNER_TOKEN=<token-from-gitea-admin> mage runner:deploy
```

### K3s + Crossplane

```bash
# 1. Deploy K3s cluster
mage k3s:deploy

# 2. Install Crossplane
mage crossplane:install
```

## Key SDK Functions

| Function | Purpose |
|----------|---------|
| `GetHostGroups(ctx)` | List all host groups |
| `CreateNode(ctx, group, req)` | Create VM in host group |
| `DeleteVM(ctx, group, hostname)` | Delete VM |
| `GetVMLogs(ctx, hostname, lines)` | Get serial console logs |
| `CpToVM(ctx, vm, local, remote, uid, gid)` | Copy files to VM |
