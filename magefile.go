//go:build mage

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	sdk "github.com/slicervm/sdk"

	"github.com/gaarutyunov/slicer/pkg/buildkit"
	"github.com/gaarutyunov/slicer/pkg/openfaas"
	"github.com/gaarutyunov/slicer/pkg/rustfs"
	"github.com/magefile/mage/mg"
)

// loadSSHKey reads an SSH public key from SSH_KEY_PATH env var or default location
func loadSSHKey() string {
	keyPath := os.Getenv("SSH_KEY_PATH")
	if keyPath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			keyPath = home + "/.ssh/id_ed25519.pub"
		}
	}
	if keyPath != "" {
		if keyData, err := os.ReadFile(keyPath); err == nil {
			return strings.TrimSpace(string(keyData))
		}
	}
	return ""
}

// hasTag checks if a tag exists in a list of tags
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// printNodeList prints nodes filtered by tag
func printNodeList(nodes []sdk.SlicerNode, tag, label string) {
	var filtered []sdk.SlicerNode
	for _, node := range nodes {
		if hasTag(node.Tags, tag) {
			filtered = append(filtered, node)
		}
	}

	if len(filtered) == 0 {
		fmt.Printf("No %s VMs found\n", label)
		return
	}

	fmt.Printf("%s VMs (%d):\n", label, len(filtered))
	for _, node := range filtered {
		tags := " [" + strings.Join(node.Tags, ", ") + "]"
		fmt.Printf("  - %s (%s)%s created %s\n", node.Hostname, node.IP, tags, node.CreatedAt)
	}
}

type Buildkit mg.Namespace

// Deploy creates a new BuildKit VM
// SSH_KEY_PATH env var specifies an additional SSH public key file (default: ~/.ssh/id_ed25519.pub)
func (Buildkit) Deploy(ctx context.Context) error {
	config := buildkit.DefaultConfig()

	if gh := os.Getenv("GITHUB_USER"); gh != "" {
		config.GitHubUser = gh
	}

	if key := loadSSHKey(); key != "" {
		config.SSHKeys = append(config.SSHKeys, key)
	}

	deployer, err := buildkit.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	resp, err := deployer.Deploy(ctx)
	if err != nil {
		return fmt.Errorf("failed to deploy buildkit: %w", err)
	}

	fmt.Printf("BuildKit VM deployed:\n")
	fmt.Printf("  Hostname: %s\n", resp.Hostname)
	fmt.Printf("  IP: %s\n", resp.IP)
	fmt.Printf("  Created: %s\n", resp.CreatedAt)

	return nil
}

// List shows all BuildKit VMs (filtered by "buildkit" tag)
func (Buildkit) List(ctx context.Context) error {
	config := buildkit.DefaultConfig()

	deployer, err := buildkit.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list buildkit nodes: %w", err)
	}

	printNodeList(nodes, "buildkit", "BuildKit")
	return nil
}

// Delete removes a BuildKit VM by hostname
func (Buildkit) Delete(ctx context.Context, hostname string) error {
	config := buildkit.DefaultConfig()

	deployer, err := buildkit.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if err := deployer.Delete(ctx, hostname); err != nil {
		return fmt.Errorf("failed to delete buildkit VM %s: %w", hostname, err)
	}

	fmt.Printf("BuildKit VM %s deleted\n", hostname)
	return nil
}

// Logs shows serial console logs for a BuildKit VM
func (Buildkit) Logs(ctx context.Context, hostname string) error {
	config := buildkit.DefaultConfig()

	deployer, err := buildkit.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	logs, err := deployer.Logs(ctx, hostname, 50)
	if err != nil {
		return fmt.Errorf("failed to get logs for %s: %w", hostname, err)
	}

	fmt.Println(logs)
	return nil
}

// Userdata prints the BuildKit userdata script
func (Buildkit) Userdata() {
	fmt.Println(buildkit.Userdata())
}

// YAML generates a Slicer config YAML for BuildKit
func (Buildkit) YAML() error {
	githubUser := os.Getenv("GITHUB_USER")
	if githubUser == "" {
		return fmt.Errorf("GITHUB_USER environment variable is required")
	}

	config := buildkit.DefaultConfig()
	fmt.Println(buildkit.GenerateYAML(config, githubUser))
	return nil
}

// OpenFaaS targets
type Openfaas mg.Namespace

// Deploy creates a new OpenFaaS Edge VM
// SSH_KEY_PATH env var specifies an additional SSH public key file (default: ~/.ssh/id_ed25519.pub)
func (Openfaas) Deploy(ctx context.Context) error {
	config := openfaas.DefaultConfig()

	if gh := os.Getenv("GITHUB_USER"); gh != "" {
		config.GitHubUser = gh
	}

	if key := loadSSHKey(); key != "" {
		config.SSHKeys = append(config.SSHKeys, key)
	}

	deployer, err := openfaas.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	resp, err := deployer.Deploy(ctx)
	if err != nil {
		return fmt.Errorf("failed to deploy openfaas: %w", err)
	}

	fmt.Printf("OpenFaaS Edge VM deployed:\n")
	fmt.Printf("  Hostname: %s\n", resp.Hostname)
	fmt.Printf("  IP: %s\n", resp.IP)
	fmt.Printf("  Created: %s\n", resp.CreatedAt)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. SSH: ssh ubuntu@%s\n", resp.IP)
	fmt.Printf("  2. Activate faasd with a license\n")
	fmt.Printf("  3. Gateway: http://%s:8080\n", resp.IP)

	return nil
}

// List shows all OpenFaaS VMs (filtered by "openfaas" tag)
func (Openfaas) List(ctx context.Context) error {
	config := openfaas.DefaultConfig()

	deployer, err := openfaas.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list openfaas nodes: %w", err)
	}

	printNodeList(nodes, "openfaas", "OpenFaaS")
	return nil
}

// Delete removes an OpenFaaS VM by hostname
func (Openfaas) Delete(ctx context.Context, hostname string) error {
	config := openfaas.DefaultConfig()

	deployer, err := openfaas.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if err := deployer.Delete(ctx, hostname); err != nil {
		return fmt.Errorf("failed to delete openfaas VM %s: %w", hostname, err)
	}

	fmt.Printf("OpenFaaS VM %s deleted\n", hostname)
	return nil
}

// Logs shows serial console logs for an OpenFaaS VM
func (Openfaas) Logs(ctx context.Context, hostname string) error {
	config := openfaas.DefaultConfig()

	deployer, err := openfaas.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	logs, err := deployer.Logs(ctx, hostname, 50)
	if err != nil {
		return fmt.Errorf("failed to get logs for %s: %w", hostname, err)
	}

	fmt.Println(logs)
	return nil
}

// Userdata prints the OpenFaaS Edge userdata script
func (Openfaas) Userdata() {
	fmt.Println(openfaas.Userdata())
}

// YAML generates a Slicer config YAML for OpenFaaS Edge
func (Openfaas) YAML() error {
	githubUser := os.Getenv("GITHUB_USER")
	if githubUser == "" {
		return fmt.Errorf("GITHUB_USER environment variable is required")
	}

	config := openfaas.DefaultConfig()
	fmt.Println(openfaas.GenerateYAML(config, githubUser))
	return nil
}

// RustFS targets
type Rustfs mg.Namespace

// Deploy creates a new RustFS VM
// SSH_KEY_PATH env var specifies an additional SSH public key file (default: ~/.ssh/id_ed25519.pub)
func (Rustfs) Deploy(ctx context.Context) error {
	config := rustfs.DefaultConfig()

	if gh := os.Getenv("GITHUB_USER"); gh != "" {
		config.GitHubUser = gh
	}

	if key := loadSSHKey(); key != "" {
		config.SSHKeys = append(config.SSHKeys, key)
	}

	deployer, err := rustfs.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	resp, err := deployer.Deploy(ctx)
	if err != nil {
		return fmt.Errorf("failed to deploy rustfs: %w", err)
	}

	fmt.Printf("RustFS VM deployed:\n")
	fmt.Printf("  Hostname: %s\n", resp.Hostname)
	fmt.Printf("  IP: %s\n", resp.IP)
	fmt.Printf("  Created: %s\n", resp.CreatedAt)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. SSH: ssh ubuntu@%s\n", resp.IP)
	fmt.Printf("  2. API endpoint: http://%s:9000\n", resp.IP)
	fmt.Printf("  3. Console: http://%s:9001\n", resp.IP)
	fmt.Printf("  4. Credentials: rustfsadmin / rustfsadmin\n")

	return nil
}

// List shows all RustFS VMs (filtered by "rustfs" tag)
func (Rustfs) List(ctx context.Context) error {
	config := rustfs.DefaultConfig()

	deployer, err := rustfs.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list rustfs nodes: %w", err)
	}

	printNodeList(nodes, "rustfs", "RustFS")
	return nil
}

// Delete removes a RustFS VM by hostname
func (Rustfs) Delete(ctx context.Context, hostname string) error {
	config := rustfs.DefaultConfig()

	deployer, err := rustfs.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if err := deployer.Delete(ctx, hostname); err != nil {
		return fmt.Errorf("failed to delete rustfs VM %s: %w", hostname, err)
	}

	fmt.Printf("RustFS VM %s deleted\n", hostname)
	return nil
}

// Logs shows serial console logs for a RustFS VM
func (Rustfs) Logs(ctx context.Context, hostname string) error {
	config := rustfs.DefaultConfig()

	deployer, err := rustfs.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	logs, err := deployer.Logs(ctx, hostname, 50)
	if err != nil {
		return fmt.Errorf("failed to get logs for %s: %w", hostname, err)
	}

	fmt.Println(logs)
	return nil
}

// Userdata prints the RustFS userdata script
func (Rustfs) Userdata() {
	fmt.Println(rustfs.Userdata())
}

// YAML generates a Slicer config YAML for RustFS
func (Rustfs) YAML() error {
	githubUser := os.Getenv("GITHUB_USER")
	if githubUser == "" {
		return fmt.Errorf("GITHUB_USER environment variable is required")
	}

	config := rustfs.DefaultConfig()
	fmt.Println(rustfs.GenerateYAML(config, githubUser))
	return nil
}
