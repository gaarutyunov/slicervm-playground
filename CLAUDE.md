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

- `pkg/buildkit/` - BuildKit VM deployment package with embedded userdata script
- `pkg/openfaas/` - OpenFaaS Edge VM deployment package with embedded userdata script
- `magefile.go` - Mage targets that import packages for easier testing

Mage namespaces expose targets for:
- VM lifecycle (create, delete, list)
- Preset deployments (BuildKit, OpenFaaS Edge)
- Environment recreation

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

- `SLICER_URL` - Slicer API base URL (default: `http://127.0.0.1:8080`)
- `SLICER_TOKEN` - Auth token from `/var/lib/slicer/auth/token`
- `SLICER_HOST_GROUP` - Host group for VM operations (default: `api`)
- `GITHUB_USER` - GitHub username for SSH key import (used by `buildkit:deploy` and `buildkit:yaml`)

## Key SDK Functions

| Function | Purpose |
|----------|---------|
| `GetHostGroups(ctx)` | List all host groups |
| `CreateNode(ctx, group, req)` | Create VM in host group |
| `DeleteVM(ctx, group, hostname)` | Delete VM |
| `GetVMLogs(ctx, hostname, lines)` | Get serial console logs |
| `CpToVM(ctx, vm, local, remote, uid, gid)` | Copy files to VM |
