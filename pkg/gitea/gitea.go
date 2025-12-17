package gitea

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
	DefaultStorageSize = "25G"
	DefaultHTTPPort    = 3000
	DefaultSSHPort     = 2222
)

type Config struct {
	HostGroup   string
	VCPU        int
	RAMGB       int
	StorageSize string
	SSHKeys     []string
	GitHubUser  string
	Tags        []string
	// Database connection (external PostgreSQL)
	DBHost string
	DBPort int
	DBName string
	DBUser string
	DBPass string
	// S3 Storage (RustFS/MinIO)
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool
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
		Tags:        []string{"gitea"},
		DBPort:      5432,
		DBName:      "giteadb",
		DBUser:      "gitea",
		S3Bucket:    "gitea",
		S3UseSSL:    false,
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

	client := sdk.NewSlicerClient(baseURL, token, "slicer-gitea/1.0", nil)

	return &Deployer{
		client: client,
		config: config,
	}, nil
}

func (d *Deployer) Deploy(ctx context.Context) (*sdk.SlicerCreateNodeResponse, error) {
	// Generate userdata with database config
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

// generateUserdata replaces placeholders in the userdata template
func generateUserdata(config Config) string {
	userdata := userdataTemplate
	userdata = strings.ReplaceAll(userdata, "{{DB_HOST}}", config.DBHost)
	userdata = strings.ReplaceAll(userdata, "{{DB_PORT}}", fmt.Sprintf("%d", config.DBPort))
	userdata = strings.ReplaceAll(userdata, "{{DB_NAME}}", config.DBName)
	userdata = strings.ReplaceAll(userdata, "{{DB_USER}}", config.DBUser)
	userdata = strings.ReplaceAll(userdata, "{{DB_PASS}}", config.DBPass)
	// S3 storage
	userdata = strings.ReplaceAll(userdata, "{{S3_ENDPOINT}}", config.S3Endpoint)
	userdata = strings.ReplaceAll(userdata, "{{S3_ACCESS_KEY}}", config.S3AccessKey)
	userdata = strings.ReplaceAll(userdata, "{{S3_SECRET_KEY}}", config.S3SecretKey)
	userdata = strings.ReplaceAll(userdata, "{{S3_BUCKET}}", config.S3Bucket)
	s3UseSSL := "false"
	if config.S3UseSSL {
		s3UseSSL = "true"
	}
	userdata = strings.ReplaceAll(userdata, "{{S3_USE_SSL}}", s3UseSSL)
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
      gateway: 192.168.141.1/24

  github_user: %s

  image: "ghcr.io/openfaasltd/slicer-systemd:5.10.240-x86_64-latest"

  hypervisor: firecracker

  api:
    port: 8080
    bind_address: "127.0.0.1"
`, config.HostGroup, config.StorageSize, config.VCPU, config.RAMGB,
		config.HostGroup, config.HostGroup, githubUser)
}
