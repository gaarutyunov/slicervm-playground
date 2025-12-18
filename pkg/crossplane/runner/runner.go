package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gaarutyunov/slicer/pkg/runner"
)

const (
	DefaultNamespace        = "default"
	DefaultProviderConfig   = "default"
	DefaultHostGroup        = "api"
	DefaultVCPU             = 2
	DefaultRAMGB            = 4
	DefaultVersion          = "0.2.11"
	DefaultLabels           = "ubuntu-latest:docker://node:16-bullseye,ubuntu-22.04:docker://node:16-bullseye,ubuntu-20.04:docker://node:16-bullseye"
)

// Config holds configuration for Crossplane runner deployment.
type Config struct {
	// Kubernetes
	Kubeconfig    string
	Namespace     string
	ProviderConfig string

	// VM configuration
	Name        string
	HostGroup   string
	VCPU        int
	RAMGB       int
	SSHKeys     []string
	GitHubUser  string
	Tags        []string

	// Runner configuration
	GiteaURL    string
	RunnerToken string
	RunnerName  string
	Labels      string
	Version     string
}

// DefaultConfig returns default configuration from environment variables.
func DefaultConfig() Config {
	hostGroup := os.Getenv("SLICER_HOST_GROUP")
	if hostGroup == "" {
		hostGroup = DefaultHostGroup
	}

	namespace := os.Getenv("CROSSPLANE_NAMESPACE")
	if namespace == "" {
		namespace = DefaultNamespace
	}

	providerConfig := os.Getenv("CROSSPLANE_PROVIDER_CONFIG")
	if providerConfig == "" {
		providerConfig = DefaultProviderConfig
	}

	return Config{
		Namespace:      namespace,
		ProviderConfig: providerConfig,
		HostGroup:      hostGroup,
		VCPU:           DefaultVCPU,
		RAMGB:          DefaultRAMGB,
		Tags:           []string{"runner", "crossplane"},
		Version:        DefaultVersion,
		Labels:         DefaultLabels,
	}
}

// Deployer handles Crossplane VM deployments for runners.
type Deployer struct {
	client     dynamic.Interface
	kubeconfig string
	config     Config
}

// NewDeployer creates a new Crossplane runner deployer.
func NewDeployer(kubeconfig string, config Config) (*Deployer, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	if kubeconfig == "" {
		kubeconfig = loadingRules.GetDefaultFilename()
	}

	return &Deployer{
		client:     client,
		kubeconfig: kubeconfig,
		config:     config,
	}, nil
}

// vmGVR returns the GroupVersionResource for VM.
func vmGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    VMGroup,
		Version:  VMVersion,
		Resource: VMResource,
	}
}

// Deploy creates a new runner VM via Crossplane.
func (d *Deployer) Deploy(ctx context.Context) (*VM, error) {
	userdata := generateUserdata(d.config)

	// Build the VM spec
	forProvider := map[string]interface{}{
		"hostGroup": d.config.HostGroup,
		"cpus":      d.config.VCPU,
		"ramGb":     d.config.RAMGB,
		"userdata":  userdata,
		"tags":      d.config.Tags,
	}

	spec := map[string]interface{}{
		"forProvider": forProvider,
		"providerConfigRef": map[string]interface{}{
			"name": d.config.ProviderConfig,
			"kind": "ClusterProviderConfig",
		},
	}

	if len(d.config.SSHKeys) > 0 {
		forProvider["sshKeys"] = d.config.SSHKeys
	}

	if d.config.GitHubUser != "" {
		forProvider["importUser"] = d.config.GitHubUser
	}

	// Generate name if not specified
	name := d.config.Name
	if name == "" {
		name = fmt.Sprintf("runner-%s", generateID())
	}

	vm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": VMAPIVersion,
			"kind":       VMKind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": d.config.Namespace,
			},
			"spec": spec,
		},
	}

	created, err := d.client.Resource(vmGVR()).Namespace(d.config.Namespace).Create(ctx, vm, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create VM: %w", err)
	}

	return unstructuredToVM(created)
}

// Delete removes a runner VM.
func (d *Deployer) Delete(ctx context.Context, name string) error {
	err := d.client.Resource(vmGVR()).Namespace(d.config.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete VM: %w", err)
	}
	return nil
}

// Get retrieves a runner VM by name.
func (d *Deployer) Get(ctx context.Context, name string) (*VM, error) {
	obj, err := d.client.Resource(vmGVR()).Namespace(d.config.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get VM: %w", err)
	}
	return unstructuredToVM(obj)
}

// List returns all runner VMs.
func (d *Deployer) List(ctx context.Context) ([]VM, error) {
	// List VMs with runner tag
	list, err := d.client.Resource(vmGVR()).Namespace(d.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "crossplane.io/claim-name",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	var vms []VM
	for _, item := range list.Items {
		vm, err := unstructuredToVM(&item)
		if err != nil {
			continue
		}
		// Filter by runner tag
		if hasTag(vm.Spec.ForProvider.Tags, "runner") {
			vms = append(vms, *vm)
		}
	}

	return vms, nil
}

// ListAll returns all VMs in the namespace.
func (d *Deployer) ListAll(ctx context.Context) ([]VM, error) {
	list, err := d.client.Resource(vmGVR()).Namespace(d.config.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	var vms []VM
	for _, item := range list.Items {
		vm, err := unstructuredToVM(&item)
		if err != nil {
			continue
		}
		vms = append(vms, *vm)
	}

	return vms, nil
}

// unstructuredToVM converts an unstructured object to VM.
func unstructuredToVM(obj *unstructured.Unstructured) (*VM, error) {
	data, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, err
	}

	var vm VM
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil, err
	}

	return &vm, nil
}

// generateUserdata creates the runner bootstrap script.
func generateUserdata(config Config) string {
	// Use the same userdata template as the SDK runner
	userdata := runner.Userdata()
	userdata = strings.ReplaceAll(userdata, "{{GITEA_URL}}", config.GiteaURL)
	userdata = strings.ReplaceAll(userdata, "{{RUNNER_TOKEN}}", config.RunnerToken)
	userdata = strings.ReplaceAll(userdata, "{{RUNNER_NAME}}", config.RunnerName)
	userdata = strings.ReplaceAll(userdata, "{{RUNNER_LABELS}}", config.Labels)
	userdata = strings.ReplaceAll(userdata, "{{RUNNER_VERSION}}", config.Version)
	return userdata
}

// generateID generates a short random ID.
func generateID() string {
	return fmt.Sprintf("%d", metav1.Now().Unix()%100000)
}

// hasTag checks if a tag exists in the list.
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Userdata returns the runner userdata template.
func Userdata() string {
	return runner.Userdata()
}
