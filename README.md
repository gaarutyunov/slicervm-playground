# Slicer Experiments

Experiments and automation tooling for [SlicerVM](https://slicervm.com/) - a lightweight VM orchestrator built on Firecracker.

## Overview

This repository contains Mage targets for deploying various workloads on Slicer VMs:

- **BuildKit** - Remote container image builder
- **OpenFaaS Edge** - Serverless functions platform
- **RustFS** - High-performance S3-compatible object storage

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

## VM Specifications

| Workload | vCPU | RAM | Storage |
|----------|------|-----|---------|
| BuildKit | 4 | 8 GB | 25 GB |
| OpenFaaS | 2 | 4 GB | 25 GB |
| RustFS | 2 | 4 GB | 25 GB |

## License

MIT
