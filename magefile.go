//go:build mage

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	sdk "github.com/slicervm/sdk"

	"github.com/gaarutyunov/slicer/pkg/buildkit"
	"github.com/gaarutyunov/slicer/pkg/crossplane"
	"github.com/gaarutyunov/slicer/pkg/gitea"
	"github.com/gaarutyunov/slicer/pkg/k3s"
	"github.com/gaarutyunov/slicer/pkg/openfaas"
	"github.com/gaarutyunov/slicer/pkg/postgres"
	"github.com/gaarutyunov/slicer/pkg/runner"
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
	fmt.Printf("\nCredentials (save these - password is randomly generated):\n")
	fmt.Printf("  Access Key: %s\n", resp.Credentials.User)
	fmt.Printf("  Secret Key: %s\n", resp.Credentials.Password)

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

// PostgreSQL targets
type Postgres mg.Namespace

// Deploy creates a new PostgreSQL VM
// SSH_KEY_PATH env var specifies an additional SSH public key file (default: ~/.ssh/id_ed25519.pub)
// POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD env vars configure the database (optional)
func (Postgres) Deploy(ctx context.Context) error {
	config := postgres.DefaultConfig()

	if gh := os.Getenv("GITHUB_USER"); gh != "" {
		config.GitHubUser = gh
	}

	if key := loadSSHKey(); key != "" {
		config.SSHKeys = append(config.SSHKeys, key)
	}

	// PostgreSQL specific configuration
	if db := os.Getenv("POSTGRES_DB"); db != "" {
		config.DBName = db
	}
	if user := os.Getenv("POSTGRES_USER"); user != "" {
		config.DBUser = user
	}
	if pass := os.Getenv("POSTGRES_PASSWORD"); pass != "" {
		config.DBPass = pass
	}

	deployer, err := postgres.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	resp, err := deployer.Deploy(ctx)
	if err != nil {
		return fmt.Errorf("failed to deploy postgres: %w", err)
	}

	fmt.Printf("PostgreSQL VM deployed:\n")
	fmt.Printf("  Hostname: %s\n", resp.Hostname)
	fmt.Printf("  IP: %s\n", resp.IP)
	fmt.Printf("  Created: %s\n", resp.CreatedAt)
	fmt.Printf("\nCredentials (save these - password is randomly generated):\n")
	fmt.Printf("  Database: %s\n", resp.Credentials.DBName)
	fmt.Printf("  Username: %s\n", resp.Credentials.DBUser)
	fmt.Printf("  Password: %s\n", resp.Credentials.DBPass)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. SSH: ssh ubuntu@%s\n", resp.IP)
	fmt.Printf("  2. Connect: psql -h %s -U %s -d %s\n", resp.IP, resp.Credentials.DBUser, resp.Credentials.DBName)

	return nil
}

// List shows all PostgreSQL VMs (filtered by "postgres" tag)
func (Postgres) List(ctx context.Context) error {
	config := postgres.DefaultConfig()

	deployer, err := postgres.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list postgres nodes: %w", err)
	}

	printNodeList(nodes, "postgres", "PostgreSQL")
	return nil
}

// Delete removes a PostgreSQL VM by hostname
func (Postgres) Delete(ctx context.Context, hostname string) error {
	config := postgres.DefaultConfig()

	deployer, err := postgres.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if err := deployer.Delete(ctx, hostname); err != nil {
		return fmt.Errorf("failed to delete postgres VM %s: %w", hostname, err)
	}

	fmt.Printf("PostgreSQL VM %s deleted\n", hostname)
	return nil
}

// Logs shows serial console logs for a PostgreSQL VM
func (Postgres) Logs(ctx context.Context, hostname string) error {
	config := postgres.DefaultConfig()

	deployer, err := postgres.NewDeployerFromEnv(config)
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

// Userdata prints the PostgreSQL userdata script
func (Postgres) Userdata() {
	fmt.Println(postgres.Userdata())
}

// YAML generates a Slicer config YAML for PostgreSQL
func (Postgres) YAML() error {
	githubUser := os.Getenv("GITHUB_USER")
	if githubUser == "" {
		return fmt.Errorf("GITHUB_USER environment variable is required")
	}

	config := postgres.DefaultConfig()
	fmt.Println(postgres.GenerateYAML(config, githubUser))
	return nil
}

// Gitea targets
type Gitea mg.Namespace

// Deploy creates a new Gitea VM with snap installation
// Required env vars: GITEA_DB_PASS, GITEA_S3_ACCESS_KEY, GITEA_S3_SECRET_KEY
// Optional env vars: GITEA_DB_HOST (auto-detected from postgres VM), GITEA_S3_ENDPOINT (auto-detected from rustfs VM)
func (Gitea) Deploy(ctx context.Context) error {
	config := gitea.DefaultConfig()

	if gh := os.Getenv("GITHUB_USER"); gh != "" {
		config.GitHubUser = gh
	}

	if key := loadSSHKey(); key != "" {
		config.SSHKeys = append(config.SSHKeys, key)
	}

	// Database host - auto-detect from postgres VM if not specified
	dbHost := os.Getenv("GITEA_DB_HOST")
	if dbHost == "" {
		// Try to find a running postgres VM
		pgConfig := postgres.DefaultConfig()
		pgDeployer, err := postgres.NewDeployerFromEnv(pgConfig)
		if err != nil {
			return fmt.Errorf("failed to create postgres deployer: %w", err)
		}
		nodes, err := pgDeployer.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list postgres nodes: %w", err)
		}
		// Find first postgres VM
		for _, node := range nodes {
			if hasTag(node.Tags, "postgres") {
				dbHost = node.IP
				// Strip CIDR suffix
				if idx := strings.Index(dbHost, "/"); idx != -1 {
					dbHost = dbHost[:idx]
				}
				fmt.Printf("Auto-detected PostgreSQL host: %s\n", dbHost)
				break
			}
		}
		if dbHost == "" {
			return fmt.Errorf("no postgres VM found; deploy one with 'mage postgres:deploy' or set GITEA_DB_HOST")
		}
	}
	config.DBHost = dbHost

	dbPass := os.Getenv("GITEA_DB_PASS")
	if dbPass == "" {
		return fmt.Errorf("GITEA_DB_PASS environment variable is required")
	}
	config.DBPass = dbPass

	// Optional database config
	if port := os.Getenv("GITEA_DB_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &config.DBPort)
	}
	if name := os.Getenv("GITEA_DB_NAME"); name != "" {
		config.DBName = name
	}
	if user := os.Getenv("GITEA_DB_USER"); user != "" {
		config.DBUser = user
	}

	// S3 Storage - auto-detect from rustfs VM if not specified
	s3Endpoint := os.Getenv("GITEA_S3_ENDPOINT")
	if s3Endpoint == "" {
		// Try to find a running rustfs VM
		rustfsConfig := rustfs.DefaultConfig()
		rustfsDeployer, err := rustfs.NewDeployerFromEnv(rustfsConfig)
		if err != nil {
			return fmt.Errorf("failed to create rustfs deployer: %w", err)
		}
		nodes, err := rustfsDeployer.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list rustfs nodes: %w", err)
		}
		// Find first rustfs VM
		for _, node := range nodes {
			if hasTag(node.Tags, "rustfs") {
				s3Host := node.IP
				// Strip CIDR suffix
				if idx := strings.Index(s3Host, "/"); idx != -1 {
					s3Host = s3Host[:idx]
				}
				s3Endpoint = fmt.Sprintf("%s:9000", s3Host)
				fmt.Printf("Auto-detected RustFS endpoint: %s\n", s3Endpoint)
				break
			}
		}
		if s3Endpoint == "" {
			return fmt.Errorf("no rustfs VM found; deploy one with 'mage rustfs:deploy' or set GITEA_S3_ENDPOINT")
		}
	}
	config.S3Endpoint = s3Endpoint

	s3AccessKey := os.Getenv("GITEA_S3_ACCESS_KEY")
	if s3AccessKey == "" {
		return fmt.Errorf("GITEA_S3_ACCESS_KEY environment variable is required")
	}
	config.S3AccessKey = s3AccessKey

	s3SecretKey := os.Getenv("GITEA_S3_SECRET_KEY")
	if s3SecretKey == "" {
		return fmt.Errorf("GITEA_S3_SECRET_KEY environment variable is required")
	}
	config.S3SecretKey = s3SecretKey

	// Optional S3 config
	if bucket := os.Getenv("GITEA_S3_BUCKET"); bucket != "" {
		config.S3Bucket = bucket
	}
	if ssl := os.Getenv("GITEA_S3_USE_SSL"); ssl == "true" {
		config.S3UseSSL = true
	}

	deployer, err := gitea.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	resp, err := deployer.Deploy(ctx)
	if err != nil {
		return fmt.Errorf("failed to deploy gitea: %w", err)
	}

	// Strip CIDR suffix from IP
	ip := resp.IP
	if idx := strings.Index(ip, "/"); idx != -1 {
		ip = ip[:idx]
	}

	fmt.Printf("Gitea VM deployed:\n")
	fmt.Printf("  Hostname: %s\n", resp.Hostname)
	fmt.Printf("  IP: %s\n", ip)
	fmt.Printf("  Created: %s\n", resp.CreatedAt)
	fmt.Printf("\nDatabase configured:\n")
	fmt.Printf("  Host: %s\n", config.DBHost)
	fmt.Printf("  Database: %s\n", config.DBName)
	fmt.Printf("  User: %s\n", config.DBUser)
	fmt.Printf("\nS3 Storage configured:\n")
	fmt.Printf("  Endpoint: %s\n", config.S3Endpoint)
	fmt.Printf("  Bucket: %s\n", config.S3Bucket)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. SSH: ssh ubuntu@%s\n", ip)
	fmt.Printf("  2. Web UI: http://%s:3000\n", ip)
	fmt.Printf("  3. Complete setup wizard in browser\n")
	fmt.Printf("  4. Configure S3 storage in app.ini (see /home/ubuntu/gitea-info.txt)\n")

	return nil
}

// List shows all Gitea VMs (filtered by "gitea" tag)
func (Gitea) List(ctx context.Context) error {
	config := gitea.DefaultConfig()

	deployer, err := gitea.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list gitea nodes: %w", err)
	}

	printNodeList(nodes, "gitea", "Gitea")
	return nil
}

// Delete removes a Gitea VM by hostname
func (Gitea) Delete(ctx context.Context, hostname string) error {
	config := gitea.DefaultConfig()

	deployer, err := gitea.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if err := deployer.Delete(ctx, hostname); err != nil {
		return fmt.Errorf("failed to delete gitea VM %s: %w", hostname, err)
	}

	fmt.Printf("Gitea VM %s deleted\n", hostname)
	return nil
}

// Logs shows serial console logs for a Gitea VM
func (Gitea) Logs(ctx context.Context, hostname string) error {
	config := gitea.DefaultConfig()

	deployer, err := gitea.NewDeployerFromEnv(config)
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

// Userdata prints the Gitea userdata script
func (Gitea) Userdata() {
	fmt.Println(gitea.Userdata())
}

// YAML generates a Slicer config YAML for Gitea
func (Gitea) YAML() error {
	githubUser := os.Getenv("GITHUB_USER")
	if githubUser == "" {
		return fmt.Errorf("GITHUB_USER environment variable is required")
	}

	config := gitea.DefaultConfig()
	fmt.Println(gitea.GenerateYAML(config, githubUser))
	return nil
}

// Runner targets for Gitea Actions Runner
type Runner mg.Namespace

// Deploy creates a new Gitea Runner VM
// Required env vars: RUNNER_TOKEN (from Gitea admin/actions/runners)
// Optional env vars: GITEA_URL (auto-detected from gitea VM), RUNNER_NAME, RUNNER_LABELS, RUNNER_VERSION
func (Runner) Deploy(ctx context.Context) error {
	config := runner.DefaultConfig()

	if gh := os.Getenv("GITHUB_USER"); gh != "" {
		config.GitHubUser = gh
	}

	if key := loadSSHKey(); key != "" {
		config.SSHKeys = append(config.SSHKeys, key)
	}

	// Gitea URL - auto-detect from gitea VM if not specified
	giteaURL := os.Getenv("GITEA_URL")
	if giteaURL == "" {
		// Try to find a running gitea VM
		giteaConfig := gitea.DefaultConfig()
		giteaDeployer, err := gitea.NewDeployerFromEnv(giteaConfig)
		if err != nil {
			return fmt.Errorf("failed to create gitea deployer: %w", err)
		}
		nodes, err := giteaDeployer.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list gitea nodes: %w", err)
		}
		// Find first gitea VM
		for _, node := range nodes {
			if hasTag(node.Tags, "gitea") {
				giteaHost := node.IP
				// Strip CIDR suffix
				if idx := strings.Index(giteaHost, "/"); idx != -1 {
					giteaHost = giteaHost[:idx]
				}
				giteaURL = fmt.Sprintf("http://%s:3000", giteaHost)
				fmt.Printf("Auto-detected Gitea URL: %s\n", giteaURL)
				break
			}
		}
		if giteaURL == "" {
			return fmt.Errorf("no gitea VM found; deploy one with 'mage gitea:deploy' or set GITEA_URL")
		}
	}
	config.GiteaURL = giteaURL

	// Runner token is required
	runnerToken := os.Getenv("RUNNER_TOKEN")
	if runnerToken == "" {
		return fmt.Errorf("RUNNER_TOKEN environment variable is required (get it from %s/admin/actions/runners)", giteaURL)
	}
	config.RunnerToken = runnerToken

	// Optional config
	if name := os.Getenv("RUNNER_NAME"); name != "" {
		config.RunnerName = name
	}
	if labels := os.Getenv("RUNNER_LABELS"); labels != "" {
		config.Labels = labels
	}
	if version := os.Getenv("RUNNER_VERSION"); version != "" {
		config.Version = version
	}

	deployer, err := runner.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	resp, err := deployer.Deploy(ctx)
	if err != nil {
		return fmt.Errorf("failed to deploy runner: %w", err)
	}

	// Strip CIDR suffix from IP
	ip := resp.IP
	if idx := strings.Index(ip, "/"); idx != -1 {
		ip = ip[:idx]
	}

	fmt.Printf("Gitea Runner VM deployed:\n")
	fmt.Printf("  Hostname: %s\n", resp.Hostname)
	fmt.Printf("  IP: %s\n", ip)
	fmt.Printf("  Created: %s\n", resp.CreatedAt)
	fmt.Printf("\nRunner configured:\n")
	fmt.Printf("  Gitea URL: %s\n", config.GiteaURL)
	fmt.Printf("  Labels: %s\n", config.Labels)
	fmt.Printf("  Version: %s\n", config.Version)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. SSH: ssh ubuntu@%s\n", ip)
	fmt.Printf("  2. Check status: sudo systemctl status act_runner\n")
	fmt.Printf("  3. View logs: sudo journalctl -u act_runner -f\n")

	return nil
}

// List shows all Runner VMs (filtered by "runner" tag)
func (Runner) List(ctx context.Context) error {
	config := runner.DefaultConfig()

	deployer, err := runner.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list runner nodes: %w", err)
	}

	printNodeList(nodes, "runner", "Runner")
	return nil
}

// Delete removes a Runner VM by hostname
func (Runner) Delete(ctx context.Context, hostname string) error {
	config := runner.DefaultConfig()

	deployer, err := runner.NewDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if err := deployer.Delete(ctx, hostname); err != nil {
		return fmt.Errorf("failed to delete runner VM %s: %w", hostname, err)
	}

	fmt.Printf("Runner VM %s deleted\n", hostname)
	return nil
}

// Logs shows serial console logs for a Runner VM
func (Runner) Logs(ctx context.Context, hostname string) error {
	config := runner.DefaultConfig()

	deployer, err := runner.NewDeployerFromEnv(config)
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

// Userdata prints the Runner userdata script
func (Runner) Userdata() {
	fmt.Println(runner.Userdata())
}

// YAML generates a Slicer config YAML for Runner
func (Runner) YAML() error {
	githubUser := os.Getenv("GITHUB_USER")
	if githubUser == "" {
		return fmt.Errorf("GITHUB_USER environment variable is required")
	}

	config := runner.DefaultConfig()
	fmt.Println(runner.GenerateYAML(config, githubUser))
	return nil
}

// Crossplane targets for Kubernetes control plane
type Crossplane mg.Namespace

// Install deploys Crossplane to the Kubernetes cluster via Helm
// Uses KUBECONFIG env var or ~/.kube/config
func (Crossplane) Install(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := crossplane.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	fmt.Println("Verifying cluster connection...")
	if err := provisioner.VerifyClusterConnection(ctx); err != nil {
		return fmt.Errorf("cluster connection failed: %w", err)
	}
	fmt.Println("Connected to cluster")

	// Check if already installed
	installed, err := provisioner.IsInstalled(ctx)
	if err != nil {
		return fmt.Errorf("failed to check installation status: %w", err)
	}
	if installed {
		fmt.Println("Crossplane is already installed, upgrading...")
	} else {
		fmt.Println("Installing Crossplane...")
	}

	config := crossplane.DefaultConfig()
	if err := provisioner.Install(ctx, config); err != nil {
		return fmt.Errorf("failed to install crossplane: %w", err)
	}

	fmt.Println("\nCrossplane installed successfully!")
	fmt.Println("Use 'mage crossplane:status' to check status")
	fmt.Println("Use 'mage crossplane:logs' to view logs")
	return nil
}

// Uninstall removes Crossplane from the Kubernetes cluster
func (Crossplane) Uninstall(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := crossplane.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	fmt.Println("Uninstalling Crossplane...")
	if err := provisioner.Uninstall(ctx); err != nil {
		return fmt.Errorf("failed to uninstall crossplane: %w", err)
	}

	fmt.Println("Crossplane uninstalled successfully!")
	return nil
}

// Status shows the status of Crossplane deployments and pods
func (Crossplane) Status(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := crossplane.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	// Check if installed
	installed, err := provisioner.IsInstalled(ctx)
	if err != nil {
		return fmt.Errorf("failed to check installation status: %w", err)
	}
	if !installed {
		fmt.Println("Crossplane is not installed")
		return nil
	}

	// Get deployments
	deployments, err := provisioner.GetDeployments(ctx)
	if err != nil {
		return fmt.Errorf("failed to get deployments: %w", err)
	}

	fmt.Printf("Crossplane Deployments (%d):\n", len(deployments))
	for _, d := range deployments {
		fmt.Printf("  - %s: %d/%d ready\n", d.Name, d.ReadyReplicas, d.Replicas)
	}

	// Get pods
	pods, err := provisioner.GetPods(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	fmt.Printf("\nCrossplane Pods (%d):\n", len(pods))
	for _, pod := range pods {
		ready := 0
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				ready++
			}
		}
		fmt.Printf("  - %s [%s] %d/%d ready\n", pod.Name, pod.Status.Phase, ready, len(pod.Status.ContainerStatuses))
	}

	return nil
}

// Logs shows logs from a Crossplane pod
// Usage: mage crossplane:logs [pod-name]
// If no pod name is given, shows logs from the first crossplane pod
func (Crossplane) Logs(ctx context.Context, podName string) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := crossplane.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	// If no pod name provided, get the first crossplane pod
	if podName == "" {
		pods, err := provisioner.GetPods(ctx)
		if err != nil {
			return fmt.Errorf("failed to get pods: %w", err)
		}
		if len(pods) == 0 {
			return fmt.Errorf("no crossplane pods found")
		}
		// Find the main crossplane pod
		for _, pod := range pods {
			if strings.HasPrefix(pod.Name, "crossplane-") && !strings.Contains(pod.Name, "rbac") {
				podName = pod.Name
				break
			}
		}
		if podName == "" {
			podName = pods[0].Name
		}
	}

	fmt.Printf("Logs from pod %s:\n\n", podName)
	logs, err := provisioner.GetLogs(ctx, podName, 100)
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	fmt.Println(logs)
	return nil
}

// K3s targets for autoscaling Kubernetes cluster
type K3s mg.Namespace

// DeployCP creates a new K3s control plane node
// SSH_KEY_PATH env var specifies an additional SSH public key file (default: ~/.ssh/id_ed25519.pub)
func (K3s) DeployCP(ctx context.Context) error {
	config := k3s.DefaultCPConfig()

	if gh := os.Getenv("GITHUB_USER"); gh != "" {
		config.GitHubUser = gh
	}

	if key := loadSSHKey(); key != "" {
		config.SSHKeys = append(config.SSHKeys, key)
	}

	deployer, err := k3s.NewCPDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	resp, err := deployer.Deploy(ctx)
	if err != nil {
		return fmt.Errorf("failed to deploy k3s control plane: %w", err)
	}

	fmt.Printf("K3s Control Plane VM deployed:\n")
	fmt.Printf("  Hostname: %s\n", resp.Hostname)
	fmt.Printf("  IP: %s\n", resp.IP)
	fmt.Printf("  Created: %s\n", resp.CreatedAt)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Deploy all control plane nodes (total: 3 recommended)\n")
	fmt.Printf("  2. Export devices: sudo -E slicer vm list --json > devices.json\n")
	fmt.Printf("  3. Install k3sup-pro: curl -sSL https://get.k3sup.dev | PRO=true sudo -E sh\n")
	fmt.Printf("  4. Plan & apply: k3sup-pro plan --user ubuntu ./devices.json && k3sup-pro apply\n")

	return nil
}

// DeployAgent creates a new K3s agent/worker node
// K3s URL is loaded from kubeconfig, token from cluster secret (k3s-node-token)
func (K3s) DeployAgent(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")
	config := k3s.DefaultAgentConfig()

	// Load K3s URL from kubeconfig
	k3sURL, err := k3s.GetK3sURLFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get K3s URL from kubeconfig: %w", err)
	}
	config.K3sURL = k3sURL

	// Load K3s token from cluster secret
	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}
	k3sToken, err := provisioner.GetK3sToken(ctx)
	if err != nil {
		return err
	}
	config.K3sToken = k3sToken

	if gh := os.Getenv("GITHUB_USER"); gh != "" {
		config.GitHubUser = gh
	}

	if key := loadSSHKey(); key != "" {
		config.SSHKeys = append(config.SSHKeys, key)
	}

	deployer, err := k3s.NewAgentDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	fmt.Printf("Deploying K3s agent with:\n")
	fmt.Printf("  K3s URL: %s\n", k3sURL)

	resp, err := deployer.Deploy(ctx)
	if err != nil {
		return fmt.Errorf("failed to deploy k3s agent: %w", err)
	}

	fmt.Printf("\nK3s Agent VM deployed:\n")
	fmt.Printf("  Hostname: %s\n", resp.Hostname)
	fmt.Printf("  IP: %s\n", resp.IP)
	fmt.Printf("  Created: %s\n", resp.CreatedAt)
	fmt.Printf("\nThe agent will automatically join the K3s cluster.\n")

	return nil
}

// ListCP shows all K3s control plane VMs (filtered by "k3s-cp" tag)
func (K3s) ListCP(ctx context.Context) error {
	config := k3s.DefaultCPConfig()

	deployer, err := k3s.NewCPDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list k3s control plane nodes: %w", err)
	}

	printNodeList(nodes, "k3s-cp", "K3s Control Plane")
	return nil
}

// ListAgents shows all K3s agent VMs (filtered by "k3s-agent" tag)
func (K3s) ListAgents(ctx context.Context) error {
	config := k3s.DefaultAgentConfig()

	deployer, err := k3s.NewAgentDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list k3s agent nodes: %w", err)
	}

	printNodeList(nodes, "k3s-agent", "K3s Agent")
	return nil
}

// DeleteCP removes a K3s control plane VM by hostname
func (K3s) DeleteCP(ctx context.Context, hostname string) error {
	config := k3s.DefaultCPConfig()

	deployer, err := k3s.NewCPDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if err := deployer.Delete(ctx, hostname); err != nil {
		return fmt.Errorf("failed to delete k3s control plane VM %s: %w", hostname, err)
	}

	fmt.Printf("K3s Control Plane VM %s deleted\n", hostname)
	return nil
}

// DeleteAgent removes a K3s agent VM by hostname
func (K3s) DeleteAgent(ctx context.Context, hostname string) error {
	config := k3s.DefaultAgentConfig()

	deployer, err := k3s.NewAgentDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if err := deployer.Delete(ctx, hostname); err != nil {
		return fmt.Errorf("failed to delete k3s agent VM %s: %w", hostname, err)
	}

	fmt.Printf("K3s Agent VM %s deleted\n", hostname)
	return nil
}

// LogsCP shows serial console logs for a K3s control plane VM
func (K3s) LogsCP(ctx context.Context, hostname string) error {
	config := k3s.DefaultCPConfig()

	deployer, err := k3s.NewCPDeployerFromEnv(config)
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

// LogsAgent shows serial console logs for a K3s agent VM
func (K3s) LogsAgent(ctx context.Context, hostname string) error {
	config := k3s.DefaultAgentConfig()

	deployer, err := k3s.NewAgentDeployerFromEnv(config)
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

// UserdataCP prints the K3s control plane userdata script
func (K3s) UserdataCP() {
	fmt.Println(k3s.UserdataCP())
}

// UserdataAgent prints the K3s agent userdata script
func (K3s) UserdataAgent() {
	fmt.Println(k3s.UserdataAgent())
}

// YAMLCP generates a Slicer config YAML for K3s control plane
func (K3s) YAMLCP() error {
	githubUser := os.Getenv("GITHUB_USER")
	if githubUser == "" {
		return fmt.Errorf("GITHUB_USER environment variable is required")
	}

	config := k3s.DefaultCPConfig()
	fmt.Println(k3s.GenerateCPYAML(config, githubUser))
	return nil
}

// YAMLAgent generates a Slicer config YAML for K3s agent host group
func (K3s) YAMLAgent() error {
	githubUser := os.Getenv("GITHUB_USER")
	if githubUser == "" {
		return fmt.Errorf("GITHUB_USER environment variable is required")
	}

	config := k3s.DefaultAgentConfig()
	fmt.Println(k3s.GenerateAgentYAML(config, githubUser))
	return nil
}

// Devices outputs control plane VMs as JSON for k3sup (devices.json format)
func (K3s) Devices(ctx context.Context) error {
	config := k3s.DefaultCPConfig()

	deployer, err := k3s.NewCPDeployerFromEnv(config)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	nodes, err := deployer.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list k3s control plane nodes: %w", err)
	}

	// Filter by k3s-cp tag
	var cpNodes []sdk.SlicerNode
	for _, node := range nodes {
		if hasTag(node.Tags, "k3s-cp") {
			cpNodes = append(cpNodes, node)
		}
	}

	// Convert to k3sup devices format
	type Device struct {
		Hostname  string `json:"hostname"`
		IP        string `json:"ip"`
		CreatedAt string `json:"created_at,omitempty"`
	}

	devices := make([]Device, 0, len(cpNodes))
	for _, node := range cpNodes {
		ip := node.IP
		// Strip CIDR suffix if present (e.g., "192.168.137.7/24" -> "192.168.137.7")
		if idx := strings.Index(ip, "/"); idx != -1 {
			ip = ip[:idx]
		}
		devices = append(devices, Device{
			Hostname:  node.Hostname,
			IP:        ip,
			CreatedAt: node.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	output, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

// Nodes lists all K8s nodes from the cluster (requires KUBECONFIG or ~/.kube/config)
func (K3s) Nodes(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	nodes, err := provisioner.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	fmt.Printf("K8s Nodes (%d):\n", len(nodes))
	for _, node := range nodes {
		ready := "NotReady"
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				ready = "Ready"
				break
			}
		}
		roles := []string{}
		for label := range node.Labels {
			if strings.HasPrefix(label, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(label, "node-role.kubernetes.io/")
				roles = append(roles, role)
			}
		}
		roleStr := "worker"
		if len(roles) > 0 {
			roleStr = strings.Join(roles, ",")
		}
		fmt.Printf("  - %s [%s] %s\n", node.Name, roleStr, ready)
	}
	return nil
}

// AutoscalerConfig prints the generated cloud-config.ini for the autoscaler
// Requires: SLICER_TOKEN environment variable
// K3S_URL from kubeconfig, K3S_TOKEN from cluster secret (k3s-node-token)
func (K3s) AutoscalerConfig(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")
	config := k3s.DefaultSimplifiedConfig()

	// K3s URL from kubeconfig
	k3sURL, err := k3s.GetK3sURLFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get K3s URL from kubeconfig: %w", err)
	}
	config.K3sURL = k3sURL

	// K3s token from cluster secret
	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}
	k3sToken, err := provisioner.GetK3sToken(ctx)
	if err != nil {
		return err
	}
	config.K3sToken = k3sToken

	// Slicer settings
	slicerURL := os.Getenv("SLICER_URL")
	if slicerURL == "" {
		slicerURL = "http://127.0.0.1:8080"
	}
	config.SlicerURL = slicerURL

	slicerToken := os.Getenv("SLICER_TOKEN")
	if slicerToken == "" {
		return fmt.Errorf("SLICER_TOKEN environment variable is required")
	}
	config.SlicerToken = slicerToken

	// Optional settings
	if ng := os.Getenv("K3S_NODEGROUP"); ng != "" {
		config.NodeGroupName = ng
	}
	if minSize := os.Getenv("K3S_MIN_SIZE"); minSize != "" {
		fmt.Sscanf(minSize, "%d", &config.MinSize)
	}
	if maxSize := os.Getenv("K3S_MAX_SIZE"); maxSize != "" {
		fmt.Sscanf(maxSize, "%d", &config.MaxSize)
	}

	fmt.Printf("# K3s URL: %s\n", k3sURL)
	fmt.Println(k3s.GenerateSimplifiedCloudConfig(config))
	return nil
}

// AutoscalerInstall deploys the cluster autoscaler to the K8s cluster
// Requires: SLICER_TOKEN environment variable
// K3S_URL from kubeconfig, K3S_TOKEN from cluster secret (k3s-node-token)
func (K3s) AutoscalerInstall(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	// Verify cluster connection
	fmt.Println("Verifying cluster connection...")
	if err := provisioner.VerifyClusterConnection(ctx); err != nil {
		return fmt.Errorf("cluster connection failed: %w", err)
	}
	fmt.Println("Connected to cluster")

	// Build cloud config
	config := k3s.DefaultSimplifiedConfig()

	// K3s URL from kubeconfig
	k3sURL, err := k3s.GetK3sURLFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get K3s URL from kubeconfig: %w", err)
	}
	config.K3sURL = k3sURL
	fmt.Printf("K3s URL: %s\n", k3sURL)

	// K3s token from cluster secret
	k3sToken, err := provisioner.GetK3sToken(ctx)
	if err != nil {
		return err
	}
	config.K3sToken = k3sToken
	fmt.Println("K3s token loaded from secret")

	slicerURL := os.Getenv("SLICER_URL")
	if slicerURL == "" {
		slicerURL = "http://127.0.0.1:8080"
	}
	config.SlicerURL = slicerURL

	slicerToken := os.Getenv("SLICER_TOKEN")
	if slicerToken == "" {
		return fmt.Errorf("SLICER_TOKEN environment variable is required")
	}
	config.SlicerToken = slicerToken

	if ng := os.Getenv("K3S_NODEGROUP"); ng != "" {
		config.NodeGroupName = ng
	}

	// Create cloud-config secret for autoscaler
	fmt.Println("Creating cloud-config secret...")
	cloudConfig := k3s.GenerateSimplifiedCloudConfig(config)
	if err := provisioner.CreateCloudConfigSecret(ctx, cloudConfig); err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}
	fmt.Println("Secret created")

	// Install autoscaler via Helm
	fmt.Println("Installing cluster autoscaler via Helm...")
	helmConfig := k3s.DefaultHelmConfig()
	if err := provisioner.InstallAutoscaler(ctx, helmConfig); err != nil {
		return fmt.Errorf("failed to install autoscaler: %w", err)
	}
	fmt.Println("Autoscaler installed")

	// Patch ClusterRole
	fmt.Println("Patching ClusterRole for node deletion...")
	if err := provisioner.PatchClusterRole(ctx); err != nil {
		// Non-fatal, autoscaler may work without it initially
		fmt.Printf("Warning: failed to patch ClusterRole: %v\n", err)
	} else {
		fmt.Println("ClusterRole patched")
	}

	fmt.Println("\nCluster Autoscaler installed successfully!")
	fmt.Println("Use 'mage k3s:autoscalerStatus' to check status")
	fmt.Println("Use 'mage k3s:autoscalerLogs' to view logs")
	return nil
}

// AutoscalerUninstall removes the cluster autoscaler from the K8s cluster
func (K3s) AutoscalerUninstall(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	fmt.Println("Uninstalling cluster autoscaler...")
	if err := provisioner.UninstallAutoscaler(ctx); err != nil {
		return fmt.Errorf("failed to uninstall autoscaler: %w", err)
	}
	fmt.Println("Autoscaler uninstalled")

	fmt.Println("Deleting cloud-config secret...")
	if err := provisioner.DeleteCloudConfigSecret(ctx); err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	fmt.Println("Secret deleted")

	fmt.Println("\nCluster Autoscaler removed successfully!")
	return nil
}

// AutoscalerStatus shows the status of the cluster autoscaler pods
func (K3s) AutoscalerStatus(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	pods, err := provisioner.GetAutoscalerPods(ctx)
	if err != nil {
		return fmt.Errorf("failed to get autoscaler pods: %w", err)
	}

	if len(pods) == 0 {
		fmt.Println("No autoscaler pods found. Is it installed?")
		return nil
	}

	fmt.Printf("Autoscaler Pods (%d):\n", len(pods))
	for _, pod := range pods {
		ready := 0
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				ready++
			}
		}
		fmt.Printf("  - %s [%s] %d/%d ready\n", pod.Name, pod.Status.Phase, ready, len(pod.Status.ContainerStatuses))
	}
	return nil
}

// AutoscalerLogs shows logs from the cluster autoscaler
func (K3s) AutoscalerLogs(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	logs, err := provisioner.GetAutoscalerLogs(ctx, 100)
	if err != nil {
		return fmt.Errorf("failed to get autoscaler logs: %w", err)
	}

	fmt.Println(logs)
	return nil
}

// AutoscalerStressTest creates a test deployment to trigger autoscaling
// Usage: mage k3s:autoscalerStressTest 100
func (K3s) AutoscalerStressTest(ctx context.Context, replicas int) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	fmt.Printf("Creating stress test deployment with %d replicas...\n", replicas)
	if err := provisioner.CreateStressTestDeployment(ctx, int32(replicas)); err != nil {
		return fmt.Errorf("failed to create stress test: %w", err)
	}

	fmt.Println("Stress test deployment created")
	fmt.Println("Monitor with: kubectl get pods -w")
	fmt.Println("             mage k3s:nodes")
	fmt.Println("             mage k3s:autoscalerLogs")
	fmt.Println("Clean up with: mage k3s:autoscalerStressTestCleanup")
	return nil
}

// AutoscalerStressTestScale scales the stress test deployment
// Usage: mage k3s:autoscalerStressTestScale 200
func (K3s) AutoscalerStressTestScale(ctx context.Context, replicas int) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	fmt.Printf("Scaling stress test deployment to %d replicas...\n", replicas)
	if err := provisioner.ScaleStressTestDeployment(ctx, int32(replicas)); err != nil {
		return fmt.Errorf("failed to scale stress test: %w", err)
	}

	fmt.Println("Stress test deployment scaled")
	return nil
}

// AutoscalerStressTestStatus shows the status of the stress test deployment
func (K3s) AutoscalerStressTestStatus(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	ready, total, err := provisioner.GetStressTestStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stress test status: %w", err)
	}

	if total == 0 {
		fmt.Println("No stress test deployment found")
		return nil
	}

	fmt.Printf("Stress test: %d/%d pods ready\n", ready, total)
	return nil
}

// AutoscalerStressTestCleanup removes the stress test deployment
func (K3s) AutoscalerStressTestCleanup(ctx context.Context) error {
	kubeconfig := os.Getenv("KUBECONFIG")

	provisioner, err := k3s.NewProvisioner(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	fmt.Println("Deleting stress test deployment...")
	if err := provisioner.DeleteStressTestDeployment(ctx); err != nil {
		return fmt.Errorf("failed to delete stress test: %w", err)
	}

	fmt.Println("Stress test deployment deleted")
	fmt.Println("Note: Autoscaler will scale down nodes after cooldown period")
	return nil
}


