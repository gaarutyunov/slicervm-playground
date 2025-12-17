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
	"github.com/gaarutyunov/slicer/pkg/k3s"
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


