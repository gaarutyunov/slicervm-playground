package buildkit

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	sdk "github.com/slicervm/sdk"
)

//go:embed userdata.sh
var userdataScript string

const (
	DefaultHostGroup   = "api"
	DefaultVCPU        = 4
	DefaultRAMGB       = 8
	DefaultStorageSize = "25G"
)

type Config struct {
	HostGroup   string
	VCPU        int
	RAMGB       int
	StorageSize string
	SSHKeys     []string
	GitHubUser  string
	Tags        []string
}

func DefaultConfig() Config {
	hostGroup := os.Getenv("SLICER_HOST_GROUP")
	if hostGroup == "" {
		hostGroup = DefaultHostGroup
	}

	return Config{
		HostGroup:   hostGroup,
		VCPU:        DefaultVCPU,
		RAMGB:       DefaultRAMGB,
		StorageSize: DefaultStorageSize,
		Tags:        []string{"buildkit"},
	}
}

type Deployer struct {
	client *sdk.SlicerClient
	config Config
}

func NewDeployer(client *sdk.SlicerClient, config Config) *Deployer {
	return &Deployer{
		client: client,
		config: config,
	}
}

func NewDeployerFromEnv(config Config) (*Deployer, error) {
	baseURL := os.Getenv("SLICER_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}

	token := os.Getenv("SLICER_TOKEN")

	client := sdk.NewSlicerClient(baseURL, token, "slicer-buildkit/1.0", nil)

	return &Deployer{
		client: client,
		config: config,
	}, nil
}

func (d *Deployer) Deploy(ctx context.Context) (*sdk.SlicerCreateNodeResponse, error) {
	req := sdk.SlicerCreateNodeRequest{
		RamGB:    d.config.RAMGB,
		CPUs:     d.config.VCPU,
		Userdata: userdataScript,
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

func (d *Deployer) Delete(ctx context.Context, hostname string) error {
	_, err := d.client.DeleteVM(ctx, d.config.HostGroup, hostname)
	return err
}

func (d *Deployer) List(ctx context.Context) ([]sdk.SlicerNode, error) {
	return d.client.GetHostGroupNodes(ctx, d.config.HostGroup)
}

func (d *Deployer) Logs(ctx context.Context, hostname string, lines int) (string, error) {
	resp, err := d.client.GetVMLogs(ctx, hostname, lines)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func Userdata() string {
	return userdataScript
}

func GenerateYAML(config Config, githubUser string) string {
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
      tap_prefix: %stap
      gateway: 192.168.138.1/24

  github_user: %s

  image: "ghcr.io/openfaasltd/slicer-systemd:5.10.240-x86_64-latest"

  hypervisor: firecracker

  api:
    port: 8080
    bind_address: "127.0.0.1"
`, config.HostGroup, config.StorageSize, config.VCPU, config.RAMGB,
		config.HostGroup, config.HostGroup, githubUser)
}
