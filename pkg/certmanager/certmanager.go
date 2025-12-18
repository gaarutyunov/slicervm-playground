package certmanager

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	Namespace   = "cert-manager"
	ReleaseName = "cert-manager"
	RepoName    = "jetstack"
	RepoURL     = "https://charts.jetstack.io"
	ChartName   = "cert-manager"
)

// Config holds cert-manager deployment configuration
type Config struct {
	// Install CRDs (required for first install)
	InstallCRDs bool
	// Replica count
	ReplicaCount int
	// Enable prometheus metrics
	EnablePrometheus bool
}

// DefaultConfig returns default cert-manager configuration
func DefaultConfig() Config {
	return Config{
		InstallCRDs:      true,
		ReplicaCount:     1,
		EnablePrometheus: true,
	}
}

// Provisioner handles cert-manager installation via Helm
type Provisioner struct {
	clientset     *kubernetes.Clientset
	dynamicClient dynamic.Interface
	kubeconfig    string
}

// NewProvisioner creates a new cert-manager Provisioner
func NewProvisioner(kubeconfig string) (*Provisioner, error) {
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

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	if kubeconfig == "" {
		kubeconfig = loadingRules.GetDefaultFilename()
	}

	return &Provisioner{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		kubeconfig:    kubeconfig,
	}, nil
}

// VerifyClusterConnection checks if we can connect to the cluster
func (p *Provisioner) VerifyClusterConnection(ctx context.Context) error {
	_, err := p.clientset.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to connect to cluster: %w", err)
	}
	return nil
}

// Install deploys cert-manager via Helm
func (p *Provisioner) Install(ctx context.Context, config Config) error {
	settings := cli.New()
	settings.KubeConfig = p.kubeconfig
	settings.SetNamespace(Namespace)

	// Add the jetstack repo
	repoEntry := &repo.Entry{
		Name: RepoName,
		URL:  RepoURL,
	}

	providers := getter.All(settings)

	repoFile := settings.RepositoryConfig
	r, err := repo.NewChartRepository(repoEntry, providers)
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	_, err = r.DownloadIndexFile()
	if err != nil {
		return fmt.Errorf("failed to download repo index: %w", err)
	}

	// Load existing repos or create new file
	f, err := repo.LoadFile(repoFile)
	if err != nil {
		f = repo.NewFile()
	}

	// Add repo if not exists
	if !f.Has(RepoName) {
		f.Update(repoEntry)
		if err := f.WriteFile(repoFile, 0644); err != nil {
			return fmt.Errorf("failed to write repo file: %w", err)
		}
	}

	// Setup action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), Namespace, "secret", func(format string, v ...interface{}) {
		fmt.Printf(format+"\n", v...)
	}); err != nil {
		return fmt.Errorf("failed to initialize helm action config: %w", err)
	}

	// Check if release exists
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	releases, err := histClient.Run(ReleaseName)
	releaseExists := err == nil && len(releases) > 0

	// Locate the chart
	chartPathOpts := action.ChartPathOptions{}
	chartPath, err := chartPathOpts.LocateChart(
		fmt.Sprintf("%s/%s", RepoName, ChartName),
		settings,
	)
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}

	// Load the chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Build values
	vals := map[string]interface{}{
		"installCRDs": config.InstallCRDs,
		"replicaCount": config.ReplicaCount,
		"prometheus": map[string]interface{}{
			"enabled": config.EnablePrometheus,
		},
	}

	if releaseExists {
		// Upgrade existing release
		upgrade := action.NewUpgrade(actionConfig)
		upgrade.Namespace = Namespace
		upgrade.Wait = true
		upgrade.Timeout = 5 * time.Minute

		_, err = upgrade.Run(ReleaseName, chart, vals)
		if err != nil {
			return fmt.Errorf("failed to upgrade cert-manager: %w", err)
		}
	} else {
		// Fresh install
		install := action.NewInstall(actionConfig)
		install.ReleaseName = ReleaseName
		install.Namespace = Namespace
		install.CreateNamespace = true
		install.Wait = true
		install.Timeout = 5 * time.Minute

		_, err = install.Run(chart, vals)
		if err != nil {
			return fmt.Errorf("failed to install cert-manager: %w", err)
		}
	}

	return nil
}

// Uninstall removes cert-manager from the cluster
func (p *Provisioner) Uninstall(ctx context.Context) error {
	settings := cli.New()
	settings.KubeConfig = p.kubeconfig
	settings.SetNamespace(Namespace)

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), Namespace, "secret", func(format string, v ...interface{}) {
		fmt.Printf(format+"\n", v...)
	}); err != nil {
		return fmt.Errorf("failed to initialize helm action config: %w", err)
	}

	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = true
	uninstall.Timeout = 2 * time.Minute

	_, err := uninstall.Run(ReleaseName)
	if err != nil {
		return fmt.Errorf("failed to uninstall cert-manager: %w", err)
	}

	return nil
}

// GetPods returns the cert-manager pods
func (p *Provisioner) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	podList, err := p.clientset.CoreV1().Pods(Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list cert-manager pods: %w", err)
	}
	return podList.Items, nil
}

// GetLogs returns logs from a cert-manager pod
func (p *Provisioner) GetLogs(ctx context.Context, podName string, tailLines int64) (string, error) {
	opts := &corev1.PodLogOptions{
		TailLines: &tailLines,
	}

	req := p.clientset.CoreV1().Pods(Namespace).GetLogs(podName, opts)
	logs, err := req.DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	return string(logs), nil
}

// GetDeployments returns cert-manager deployments status
func (p *Provisioner) GetDeployments(ctx context.Context) ([]DeploymentStatus, error) {
	deployments, err := p.clientset.AppsV1().Deployments(Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	var statuses []DeploymentStatus
	for _, d := range deployments.Items {
		statuses = append(statuses, DeploymentStatus{
			Name:            d.Name,
			Replicas:        d.Status.Replicas,
			ReadyReplicas:   d.Status.ReadyReplicas,
			UpdatedReplicas: d.Status.UpdatedReplicas,
		})
	}
	return statuses, nil
}

// DeploymentStatus represents the status of a deployment
type DeploymentStatus struct {
	Name            string
	Replicas        int32
	ReadyReplicas   int32
	UpdatedReplicas int32
}

// IsInstalled checks if cert-manager is already installed
func (p *Provisioner) IsInstalled(ctx context.Context) (bool, error) {
	_, err := p.clientset.CoreV1().Namespaces().Get(ctx, Namespace, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check namespace: %w", err)
	}

	// Check if there are any pods in the namespace
	pods, err := p.GetPods(ctx)
	if err != nil {
		return false, err
	}

	return len(pods) > 0, nil
}

// ClusterIssuerConfig holds configuration for creating a ClusterIssuer
type ClusterIssuerConfig struct {
	Name          string
	Email         string
	Server        string // ACME server URL
	IngressClass  string
	IsProduction  bool
}

// DefaultClusterIssuerConfig returns default Let's Encrypt production config
func DefaultClusterIssuerConfig() ClusterIssuerConfig {
	return ClusterIssuerConfig{
		Name:         "letsencrypt-prod",
		Server:       "https://acme-v02.api.letsencrypt.org/directory",
		IngressClass: "traefik",
		IsProduction: true,
	}
}

// StagingClusterIssuerConfig returns Let's Encrypt staging config (for testing)
func StagingClusterIssuerConfig() ClusterIssuerConfig {
	return ClusterIssuerConfig{
		Name:         "letsencrypt-staging",
		Server:       "https://acme-staging-v02.api.letsencrypt.org/directory",
		IngressClass: "traefik",
		IsProduction: false,
	}
}

// ClusterIssuerYAML generates the ClusterIssuer YAML manifest
func ClusterIssuerYAML(config ClusterIssuerConfig) string {
	return fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: %s
spec:
  acme:
    server: %s
    email: %s
    privateKeySecretRef:
      name: %s
    solvers:
    - http01:
        ingress:
          class: %s
`, config.Name, config.Server, config.Email, config.Name, config.IngressClass)
}

// clusterIssuerGVR is the GroupVersionResource for ClusterIssuer
var clusterIssuerGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "clusterissuers",
}

// CreateClusterIssuer creates a ClusterIssuer in the cluster
func (p *Provisioner) CreateClusterIssuer(ctx context.Context, config ClusterIssuerConfig) error {
	issuer := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "ClusterIssuer",
			"metadata": map[string]interface{}{
				"name": config.Name,
			},
			"spec": map[string]interface{}{
				"acme": map[string]interface{}{
					"server": config.Server,
					"email":  config.Email,
					"privateKeySecretRef": map[string]interface{}{
						"name": config.Name,
					},
					"solvers": []interface{}{
						map[string]interface{}{
							"http01": map[string]interface{}{
								"ingress": map[string]interface{}{
									"class": config.IngressClass,
								},
							},
						},
					},
				},
			},
		},
	}

	// Check if it already exists
	existing, err := p.dynamicClient.Resource(clusterIssuerGVR).Get(ctx, config.Name, metav1.GetOptions{})
	if err == nil {
		// Update existing
		issuer.SetResourceVersion(existing.GetResourceVersion())
		_, err = p.dynamicClient.Resource(clusterIssuerGVR).Update(ctx, issuer, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ClusterIssuer: %w", err)
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check ClusterIssuer: %w", err)
	}

	// Create new
	_, err = p.dynamicClient.Resource(clusterIssuerGVR).Create(ctx, issuer, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ClusterIssuer: %w", err)
	}

	return nil
}

// GetClusterIssuers lists all ClusterIssuers
func (p *Provisioner) GetClusterIssuers(ctx context.Context) ([]string, error) {
	list, err := p.dynamicClient.Resource(clusterIssuerGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list ClusterIssuers: %w", err)
	}

	var names []string
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	return names, nil
}
