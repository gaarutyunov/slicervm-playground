package k3s

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	sdk "github.com/slicervm/sdk"
)

//go:embed userdata_cp.sh
var userdataCPScript string

//go:embed userdata_agent.sh
var userdataAgentScript string

const (
	DefaultCPHostGroup    = "api"
	DefaultAgentHostGroup = "api"
	DefaultCPVCPU         = 2
	DefaultCPRAMGB        = 4
	DefaultAgentVCPU      = 2
	DefaultAgentRAMGB     = 4
	DefaultStorageSize    = "25G"
	DefaultCPCount        = 3
	DefaultCPCIDR         = "192.168.137.0/24"
	DefaultAgentCIDR      = "192.168.138.0/24"
)

// CPConfig holds configuration for control plane nodes
type CPConfig struct {
	HostGroup   string
	VCPU        int
	RAMGB       int
	StorageSize string
	SSHKeys     []string
	GitHubUser  string
	Tags        []string
	Count       int
	CIDR        string
}

// AgentConfig holds configuration for worker/agent nodes
type AgentConfig struct {
	HostGroup   string
	VCPU        int
	RAMGB       int
	StorageSize string
	SSHKeys     []string
	GitHubUser  string
	Tags        []string
	CIDR        string
	APIPort     int
	TapPrefix   string
	// K3s cluster credentials (embedded in userdata)
	K3sURL   string
	K3sToken string
}

func DefaultCPConfig() CPConfig {
	hostGroup := os.Getenv("K3S_CP_HOST_GROUP")
	if hostGroup == "" {
		hostGroup = DefaultCPHostGroup
	}

	return CPConfig{
		HostGroup:   hostGroup,
		VCPU:        DefaultCPVCPU,
		RAMGB:       DefaultCPRAMGB,
		StorageSize: DefaultStorageSize,
		Tags:        []string{"k3s", "k3s-cp"},
		Count:       DefaultCPCount,
		CIDR:        DefaultCPCIDR,
	}
}

func DefaultAgentConfig() AgentConfig {
	hostGroup := os.Getenv("K3S_AGENT_HOST_GROUP")
	if hostGroup == "" {
		hostGroup = DefaultAgentHostGroup
	}

	return AgentConfig{
		HostGroup:   hostGroup,
		VCPU:        DefaultAgentVCPU,
		RAMGB:       DefaultAgentRAMGB,
		StorageSize: DefaultStorageSize,
		Tags:        []string{"k3s", "k3s-agent"},
		CIDR:        DefaultAgentCIDR,
		APIPort:     8081,
		TapPrefix:   "k3sa",
	}
}

// CPDeployer handles control plane node deployment
type CPDeployer struct {
	client *sdk.SlicerClient
	config CPConfig
}

func NewCPDeployer(client *sdk.SlicerClient, config CPConfig) *CPDeployer {
	return &CPDeployer{
		client: client,
		config: config,
	}
}

func NewCPDeployerFromEnv(config CPConfig) (*CPDeployer, error) {
	baseURL := os.Getenv("SLICER_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}

	token := os.Getenv("SLICER_TOKEN")

	client := sdk.NewSlicerClient(baseURL, token, "slicer-k3s-cp/1.0", nil)

	return &CPDeployer{
		client: client,
		config: config,
	}, nil
}

func (d *CPDeployer) Deploy(ctx context.Context) (*sdk.SlicerCreateNodeResponse, error) {
	req := sdk.SlicerCreateNodeRequest{
		RamGB:    d.config.RAMGB,
		CPUs:     d.config.VCPU,
		Userdata: userdataCPScript,
	}

	if len(d.config.SSHKeys) > 0 {
		req.SSHKeys = d.config.SSHKeys
	}

	if d.config.GitHubUser != "" {
		req.ImportUser = d.config.GitHubUser
	}

	if len(d.config.Tags) > 0 {
		req.Tags = d.config.Tags
	}

	return d.client.CreateNode(ctx, d.config.HostGroup, req)
}

func (d *CPDeployer) Delete(ctx context.Context, hostname string) error {
	_, err := d.client.DeleteVM(ctx, d.config.HostGroup, hostname)
	return err
}

func (d *CPDeployer) List(ctx context.Context) ([]sdk.SlicerNode, error) {
	return d.client.GetHostGroupNodes(ctx, d.config.HostGroup)
}

func (d *CPDeployer) Logs(ctx context.Context, hostname string, lines int) (string, error) {
	resp, err := d.client.GetVMLogs(ctx, hostname, lines)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// AgentDeployer handles worker/agent node deployment
type AgentDeployer struct {
	client *sdk.SlicerClient
	config AgentConfig
}

func NewAgentDeployer(client *sdk.SlicerClient, config AgentConfig) *AgentDeployer {
	return &AgentDeployer{
		client: client,
		config: config,
	}
}

func NewAgentDeployerFromEnv(config AgentConfig) (*AgentDeployer, error) {
	baseURL := os.Getenv("SLICER_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}

	token := os.Getenv("SLICER_TOKEN")

	client := sdk.NewSlicerClient(baseURL, token, "slicer-k3s-agent/1.0", nil)

	return &AgentDeployer{
		client: client,
		config: config,
	}, nil
}

func (d *AgentDeployer) Deploy(ctx context.Context) (*sdk.SlicerCreateNodeResponse, error) {
	// Prepare userdata with embedded K3s credentials
	userdata := PrepareAgentUserdata(d.config.K3sURL, d.config.K3sToken)

	req := sdk.SlicerCreateNodeRequest{
		RamGB:    d.config.RAMGB,
		CPUs:     d.config.VCPU,
		Userdata: userdata,
	}

	if len(d.config.SSHKeys) > 0 {
		req.SSHKeys = d.config.SSHKeys
	}

	if d.config.GitHubUser != "" {
		req.ImportUser = d.config.GitHubUser
	}

	if len(d.config.Tags) > 0 {
		req.Tags = d.config.Tags
	}

	return d.client.CreateNode(ctx, d.config.HostGroup, req)
}

// PrepareAgentUserdata returns the agent userdata script with K3s credentials embedded
func PrepareAgentUserdata(k3sURL, k3sToken string) string {
	userdata := userdataAgentScript
	userdata = strings.ReplaceAll(userdata, "{{K3S_URL}}", k3sURL)
	userdata = strings.ReplaceAll(userdata, "{{K3S_TOKEN}}", k3sToken)
	return userdata
}

func (d *AgentDeployer) Delete(ctx context.Context, hostname string) error {
	_, err := d.client.DeleteVM(ctx, d.config.HostGroup, hostname)
	return err
}

func (d *AgentDeployer) List(ctx context.Context) ([]sdk.SlicerNode, error) {
	return d.client.GetHostGroupNodes(ctx, d.config.HostGroup)
}

func (d *AgentDeployer) Logs(ctx context.Context, hostname string, lines int) (string, error) {
	resp, err := d.client.GetVMLogs(ctx, hostname, lines)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func UserdataCP() string {
	return userdataCPScript
}

func UserdataAgent() string {
	return userdataAgentScript
}

func GenerateCPYAML(config CPConfig, githubUser string) string {
	return fmt.Sprintf(`config:
  host_groups:
  - name: %s
    storage: image
    storage_size: %s
    count: %d
    vcpu: %d
    ram_gb: %d
    network:
      bridge: br%s0
      tap_prefix: %stap
      gateway: %s

  github_user: %s

  image: "ghcr.io/openfaasltd/slicer-systemd:5.10.240-x86_64-latest"

  hypervisor: firecracker

  api:
    port: 8080
    bind_address: "127.0.0.1"
`, config.HostGroup, config.StorageSize, config.Count, config.VCPU, config.RAMGB,
		config.HostGroup, config.HostGroup, gatewayFromCIDR(config.CIDR), githubUser)
}

func GenerateAgentYAML(config AgentConfig, githubUser string) string {
	return fmt.Sprintf(`config:
  host_groups:
  - name: %s
    storage: image
    storage_size: %s
    count: 0
    vcpu: %d
    ram_gb: %d
    network:
      bridge: br%s0
      tap_prefix: %s
      gateway: %s

  github_user: %s

  image: "ghcr.io/openfaasltd/slicer-systemd:5.10.240-x86_64-latest"

  hypervisor: firecracker

  api:
    port: %d
    bind_address: "0.0.0.0"

  ssh:
    port: 0
    find_keys: false
`, config.HostGroup, config.StorageSize, config.VCPU, config.RAMGB,
		config.HostGroup, config.TapPrefix, gatewayFromCIDR(config.CIDR),
		githubUser, config.APIPort)
}

// gatewayFromCIDR converts CIDR like 192.168.137.0/24 to gateway format 192.168.137.1/24
func gatewayFromCIDR(cidr string) string {
	// Simple implementation: replace .0/ with .1/
	if len(cidr) > 0 {
		for i := len(cidr) - 1; i >= 0; i-- {
			if cidr[i] == '/' {
				// Find the last octet
				for j := i - 1; j >= 0; j-- {
					if cidr[j] == '.' {
						return cidr[:j+1] + "1" + cidr[i:]
					}
				}
			}
		}
	}
	return cidr
}
