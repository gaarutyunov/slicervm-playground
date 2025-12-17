package postgres

import (
	"context"
	"crypto/rand"
	_ "embed"
	"fmt"
	"math/big"
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
	DefaultPort        = 5432
	DefaultDBName      = "app"
	DefaultDBUser      = "app"
)

// alphanumeric characters for password generation (no special chars)
const alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Config struct {
	HostGroup   string
	VCPU        int
	RAMGB       int
	StorageSize string
	SSHKeys     []string
	GitHubUser  string
	Tags        []string
	// PostgreSQL specific
	DBName string
	DBUser string
	DBPass string
}

// Credentials holds the generated PostgreSQL credentials
type Credentials struct {
	DBName string
	DBUser string
	DBPass string
}

// DeployResponse contains VM info and credentials
type DeployResponse struct {
	*sdk.SlicerCreateNodeResponse
	Credentials Credentials
}

// GeneratePassword creates a cryptographically secure random alphanumeric password
func GeneratePassword(length int) (string, error) {
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphanumeric))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random password: %w", err)
		}
		result[i] = alphanumeric[num.Int64()]
	}
	return string(result), nil
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
		Tags:        []string{"postgres"},
		DBName:      DefaultDBName,
		DBUser:      DefaultDBUser,
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

	client := sdk.NewSlicerClient(baseURL, token, "slicer-postgres/1.0", nil)

	return &Deployer{
		client: client,
		config: config,
	}, nil
}

func (d *Deployer) Deploy(ctx context.Context) (*DeployResponse, error) {
	// Generate password if not already set
	password := d.config.DBPass
	if password == "" {
		var err error
		password, err = GeneratePassword(24)
		if err != nil {
			return nil, fmt.Errorf("failed to generate password: %w", err)
		}
	}

	dbName := d.config.DBName
	if dbName == "" {
		dbName = DefaultDBName
	}

	dbUser := d.config.DBUser
	if dbUser == "" {
		dbUser = DefaultDBUser
	}

	// Generate userdata with credentials
	userdata := generateUserdata(dbName, dbUser, password)

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

	resp, err := d.client.CreateNode(ctx, d.config.HostGroup, req)
	if err != nil {
		return nil, err
	}

	return &DeployResponse{
		SlicerCreateNodeResponse: resp,
		Credentials: Credentials{
			DBName: dbName,
			DBUser: dbUser,
			DBPass: password,
		},
	}, nil
}

// generateUserdata replaces placeholders in the userdata template
func generateUserdata(dbName, dbUser, password string) string {
	userdata := userdataTemplate
	userdata = strings.ReplaceAll(userdata, "{{POSTGRES_DB}}", dbName)
	userdata = strings.ReplaceAll(userdata, "{{POSTGRES_USER}}", dbUser)
	userdata = strings.ReplaceAll(userdata, "{{POSTGRES_PASSWORD}}", password)
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
      gateway: 192.168.139.1/24

  github_user: %s

  image: "ghcr.io/openfaasltd/slicer-systemd:5.10.240-x86_64-latest"

  hypervisor: firecracker

  api:
    port: 8080
    bind_address: "127.0.0.1"
`, config.HostGroup, config.StorageSize, config.VCPU, config.RAMGB,
		config.HostGroup, config.HostGroup, githubUser)
}
