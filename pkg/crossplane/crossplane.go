package crossplane

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	Namespace     = "crossplane-system"
	ReleaseName   = "crossplane"
	RepoName      = "crossplane-stable"
	RepoURL       = "https://charts.crossplane.io/stable"
	ChartName     = "crossplane"
)

// Config holds Crossplane deployment configuration
type Config struct {
	// Helm values
	ReplicaCount int
	// Feature flags (beta features enabled by default, alpha disabled)
	EnableUsages                bool // Beta
	EnableRealtimeCompositions  bool // Beta
	EnableFunctionResponseCache bool // Alpha
	EnableSignatureVerification bool // Alpha
}

// DefaultConfig returns default Crossplane configuration
func DefaultConfig() Config {
	return Config{
		ReplicaCount:               1,
		EnableUsages:               true,  // Beta - enabled by default
		EnableRealtimeCompositions: true,  // Beta - enabled by default
	}
}

// Provisioner handles Crossplane installation via Helm
type Provisioner struct {
	clientset  *kubernetes.Clientset
	kubeconfig string
}

// NewProvisioner creates a new Crossplane Provisioner
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

	if kubeconfig == "" {
		kubeconfig = loadingRules.GetDefaultFilename()
	}

	return &Provisioner{
		clientset:  clientset,
		kubeconfig: kubeconfig,
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

// Install deploys Crossplane via Helm
func (p *Provisioner) Install(ctx context.Context, config Config) error {
	settings := cli.New()
	settings.KubeConfig = p.kubeconfig
	settings.SetNamespace(Namespace)

	// Add the Crossplane repo
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
	// args must be a list of strings for the Crossplane chart
	var args []string
	if config.EnableUsages {
		args = append(args, "--enable-usages")
	}
	if config.EnableRealtimeCompositions {
		args = append(args, "--enable-realtime-compositions")
	}
	if config.EnableFunctionResponseCache {
		args = append(args, "--enable-function-response-cache")
	}
	if config.EnableSignatureVerification {
		args = append(args, "--enable-signature-verification")
	}

	vals := map[string]interface{}{
		"replicas": config.ReplicaCount,
	}
	if len(args) > 0 {
		vals["args"] = args
	}

	if releaseExists {
		// Upgrade existing release
		upgrade := action.NewUpgrade(actionConfig)
		upgrade.Namespace = Namespace
		upgrade.Wait = true
		upgrade.Timeout = 5 * time.Minute

		_, err = upgrade.Run(ReleaseName, chart, vals)
		if err != nil {
			return fmt.Errorf("failed to upgrade crossplane: %w", err)
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
			return fmt.Errorf("failed to install crossplane: %w", err)
		}
	}

	return nil
}

// Uninstall removes Crossplane from the cluster
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
		return fmt.Errorf("failed to uninstall crossplane: %w", err)
	}

	return nil
}

// GetPods returns the Crossplane pods
func (p *Provisioner) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	podList, err := p.clientset.CoreV1().Pods(Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list crossplane pods: %w", err)
	}
	return podList.Items, nil
}

// GetLogs returns logs from a Crossplane pod
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

// GetDeployments returns Crossplane deployments status
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

// IsInstalled checks if Crossplane is already installed
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
