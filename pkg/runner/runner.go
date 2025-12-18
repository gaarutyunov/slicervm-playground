package runner

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	sdk "github.com/slicervm/sdk"
)

//go:embed userdata.sh
var userdataTemplate string

const (
	DefaultHostGroup   = "api"
	DefaultVCPU        = 2
	DefaultRAMGB       = 4
	DefaultStorageSize = "100G"
	DefaultVersion     = "0.2.11"
)

type Config struct {
	HostGroup   string
	VCPU        int
	RAMGB       int
	StorageSize string
	SSHKeys     []string
	GitHubUser  string
	Tags        []string
	// Runner configuration
	GiteaURL    string // Gitea instance URL (e.g., http://192.168.137.10:3000)
	RunnerToken string // Registration token from Gitea admin
	RunnerName  string // Optional runner name
	Labels      string // Optional labels (e.g., ubuntu-latest:docker://node:16-bullseye)
	Version     string // act_runner version
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
		Tags:        []string{"runner"},
		Version:     DefaultVersion,
		Labels:      "ubuntu-latest:docker://node:16-bullseye,ubuntu-22.04:docker://node:16-bullseye,ubuntu-20.04:docker://node:16-bullseye",
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

	client := sdk.NewSlicerClient(baseURL, token, "slicer-runner/1.0", nil)

	return &Deployer{
		client: client,
		config: config,
	}, nil
}

func (d *Deployer) Deploy(ctx context.Context) (*sdk.SlicerCreateNodeResponse, error) {
	userdata := generateUserdata(d.config)

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

func generateUserdata(config Config) string {
	userdata := userdataTemplate
	userdata = strings.ReplaceAll(userdata, "{{GITEA_URL}}", config.GiteaURL)
	userdata = strings.ReplaceAll(userdata, "{{RUNNER_TOKEN}}", config.RunnerToken)
	userdata = strings.ReplaceAll(userdata, "{{RUNNER_NAME}}", config.RunnerName)
	userdata = strings.ReplaceAll(userdata, "{{RUNNER_LABELS}}", config.Labels)
	userdata = strings.ReplaceAll(userdata, "{{RUNNER_VERSION}}", config.Version)
	return userdata
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
	return userdataTemplate
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
      gateway: 192.168.142.1/24

  github_user: %s

  image: "ghcr.io/openfaasltd/slicer-systemd:5.10.240-x86_64-latest"

  hypervisor: firecracker

  api:
    port: 8080
    bind_address: "127.0.0.1"
`, config.HostGroup, config.StorageSize, config.VCPU, config.RAMGB,
		config.HostGroup, config.HostGroup, githubUser)
}
