package grafana

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
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
	Namespace   = "monitoring"
	ReleaseName = "kube-prometheus-stack"
	RepoName    = "prometheus-community"
	RepoURL     = "https://prometheus-community.github.io/helm-charts"
	ChartName   = "kube-prometheus-stack"
)

// Config holds Grafana stack deployment configuration
type Config struct {
	// Grafana settings
	AdminPassword string
	// Ingress settings
	IngressEnabled   bool
	IngressHost      string
	IngressClassName string
	// TLS settings
	TLSEnabled     bool
	ClusterIssuer  string
	// Prometheus settings
	PrometheusRetentionDays int
	PrometheusStorageSize   string
	// Alertmanager settings
	EnableAlertmanager bool
	// Resource settings
	GrafanaReplicas    int
	PrometheusReplicas int
}

// DefaultConfig returns default Grafana stack configuration with a generated password
func DefaultConfig() Config {
	password, err := GeneratePassword(16)
	if err != nil {
		password = "prom-operator" // Fallback to chart default
	}
	return Config{
		AdminPassword:           password,
		IngressEnabled:          false,
		IngressHost:             "",
		IngressClassName:        "traefik", // K3s default ingress controller
		TLSEnabled:              false,
		ClusterIssuer:           "letsencrypt-prod",
		PrometheusRetentionDays: 10,
		PrometheusStorageSize:   "10Gi",
		EnableAlertmanager:      true,
		GrafanaReplicas:         1,
		PrometheusReplicas:      1,
	}
}

// GeneratePassword generates a random password of the specified length
func GeneratePassword(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}
	// Use URL-safe base64 and trim to desired length
	password := base64.URLEncoding.EncodeToString(bytes)
	// Remove padding and take only the requested length
	password = strings.TrimRight(password, "=")
	if len(password) > length {
		password = password[:length]
	}
	return password, nil
}

// Provisioner handles Grafana stack installation via Helm
type Provisioner struct {
	clientset  *kubernetes.Clientset
	kubeconfig string
}

// NewProvisioner creates a new Grafana stack Provisioner
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

// Install deploys the Grafana stack (kube-prometheus-stack) via Helm
func (p *Provisioner) Install(ctx context.Context, config Config) error {
	settings := cli.New()
	settings.KubeConfig = p.kubeconfig
	settings.SetNamespace(Namespace)

	// Add the Prometheus community repo
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

	// Build grafana config
	grafanaConfig := map[string]interface{}{
		"adminPassword": config.AdminPassword,
		"replicas":      config.GrafanaReplicas,
		"service": map[string]interface{}{
			"type": "NodePort",
		},
		// Dashboard providers configuration
		"dashboardProviders": map[string]interface{}{
			"dashboardproviders.yaml": map[string]interface{}{
				"apiVersion": 1,
				"providers": []map[string]interface{}{
					{
						"name":            "default",
						"orgId":           1,
						"folder":          "",
						"type":            "file",
						"disableDeletion": false,
						"editable":        true,
						"options": map[string]interface{}{
							"path": "/var/lib/grafana/dashboards/default",
						},
					},
				},
			},
		},
		// Dashboards to provision from Grafana.com
		"dashboards": map[string]interface{}{
			"default": map[string]interface{}{
				// Node Exporter Full - comprehensive Linux server monitoring
				// https://grafana.com/grafana/dashboards/1860
				"node-exporter-full": map[string]interface{}{
					"gnetId":     1860,
					"revision":   37,
					"datasource": "Prometheus",
				},
			},
		},
	}

	// Add ingress if enabled
	if config.IngressEnabled && config.IngressHost != "" {
		ingressConfig := map[string]interface{}{
			"enabled":          true,
			"ingressClassName": config.IngressClassName,
			"hosts":            []string{config.IngressHost},
			"path":             "/",
		}

		// Add TLS if enabled
		if config.TLSEnabled {
			ingressConfig["annotations"] = map[string]interface{}{
				"cert-manager.io/cluster-issuer": config.ClusterIssuer,
			}
			ingressConfig["tls"] = []map[string]interface{}{
				{
					"secretName": fmt.Sprintf("%s-tls", config.IngressHost),
					"hosts":      []string{config.IngressHost},
				},
			}
		}

		grafanaConfig["ingress"] = ingressConfig
	}

	// Build prometheus spec
	prometheusSpec := map[string]interface{}{
		"replicas":  config.PrometheusReplicas,
		"retention": fmt.Sprintf("%dd", config.PrometheusRetentionDays),
		// Enable remote write receiver to allow external agents to push metrics
		"enableRemoteWriteReceiver": true,
		"storageSpec": map[string]interface{}{
			"volumeClaimTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"accessModes": []string{"ReadWriteOnce"},
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"storage": config.PrometheusStorageSize,
						},
					},
				},
			},
		},
	}

	// Check if additional scrape config secret exists
	_, err = p.clientset.CoreV1().Secrets(Namespace).Get(ctx, "additional-scrape-configs", metav1.GetOptions{})
	if err == nil {
		// Secret exists, reference it via additionalScrapeConfigsSecret (not additionalScrapeConfigs)
		prometheusSpec["additionalScrapeConfigsSecret"] = map[string]interface{}{
			"name": "additional-scrape-configs",
			"key":  "prometheus-additional.yaml",
		}
	}

	// Build values
	vals := map[string]interface{}{
		"grafana": grafanaConfig,
		"prometheus": map[string]interface{}{
			"prometheusSpec": prometheusSpec,
			"service": map[string]interface{}{
				"type": "NodePort",
			},
		},
		"alertmanager": map[string]interface{}{
			"enabled": config.EnableAlertmanager,
			"service": map[string]interface{}{
				"type": "NodePort",
			},
		},
	}

	if releaseExists {
		// Upgrade existing release
		upgrade := action.NewUpgrade(actionConfig)
		upgrade.Namespace = Namespace
		upgrade.Wait = true
		upgrade.Timeout = 10 * time.Minute

		_, err = upgrade.Run(ReleaseName, chart, vals)
		if err != nil {
			return fmt.Errorf("failed to upgrade grafana stack: %w", err)
		}
	} else {
		// Fresh install
		install := action.NewInstall(actionConfig)
		install.ReleaseName = ReleaseName
		install.Namespace = Namespace
		install.CreateNamespace = true
		install.Wait = true
		install.Timeout = 10 * time.Minute

		_, err = install.Run(chart, vals)
		if err != nil {
			return fmt.Errorf("failed to install grafana stack: %w", err)
		}
	}

	return nil
}

// Uninstall removes the Grafana stack from the cluster
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
	uninstall.Timeout = 5 * time.Minute

	_, err := uninstall.Run(ReleaseName)
	if err != nil {
		return fmt.Errorf("failed to uninstall grafana stack: %w", err)
	}

	return nil
}

// GetPods returns the monitoring namespace pods
func (p *Provisioner) GetPods(ctx context.Context) ([]corev1.Pod, error) {
	podList, err := p.clientset.CoreV1().Pods(Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list monitoring pods: %w", err)
	}
	return podList.Items, nil
}

// GetLogs returns logs from a monitoring pod
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

// GetDeployments returns monitoring deployments status
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

// GetStatefulSets returns monitoring statefulsets status (Prometheus, Alertmanager)
func (p *Provisioner) GetStatefulSets(ctx context.Context) ([]StatefulSetStatus, error) {
	statefulsets, err := p.clientset.AppsV1().StatefulSets(Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list statefulsets: %w", err)
	}

	var statuses []StatefulSetStatus
	for _, s := range statefulsets.Items {
		statuses = append(statuses, StatefulSetStatus{
			Name:          s.Name,
			Replicas:      s.Status.Replicas,
			ReadyReplicas: s.Status.ReadyReplicas,
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

// StatefulSetStatus represents the status of a statefulset
type StatefulSetStatus struct {
	Name          string
	Replicas      int32
	ReadyReplicas int32
}

// IsInstalled checks if the Grafana stack is already installed
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

// GetServices returns the services in the monitoring namespace
func (p *Provisioner) GetServices(ctx context.Context) ([]ServiceInfo, error) {
	services, err := p.clientset.CoreV1().Services(Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var infos []ServiceInfo
	for _, svc := range services.Items {
		info := ServiceInfo{
			Name: svc.Name,
			Type: string(svc.Spec.Type),
		}
		if len(svc.Spec.Ports) > 0 {
			info.Port = svc.Spec.Ports[0].Port
			info.NodePort = svc.Spec.Ports[0].NodePort
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// ServiceInfo represents service information
type ServiceInfo struct {
	Name     string
	Type     string
	Port     int32
	NodePort int32
}

// ScrapeTarget represents an external Prometheus scrape target
type ScrapeTarget struct {
	JobName  string
	Targets  []string
	Labels   map[string]string
}

// CreateAdditionalScrapeConfig creates a secret with additional scrape configs for Prometheus
func (p *Provisioner) CreateAdditionalScrapeConfig(ctx context.Context, targets []ScrapeTarget) error {
	// Build the scrape config YAML
	var scrapeConfigs string
	for _, target := range targets {
		scrapeConfigs += fmt.Sprintf("- job_name: '%s'\n", target.JobName)
		scrapeConfigs += "  static_configs:\n"
		scrapeConfigs += "    - targets:\n"
		for _, t := range target.Targets {
			scrapeConfigs += fmt.Sprintf("        - '%s'\n", t)
		}
		if len(target.Labels) > 0 {
			scrapeConfigs += "      labels:\n"
			for k, v := range target.Labels {
				scrapeConfigs += fmt.Sprintf("        %s: '%s'\n", k, v)
			}
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "additional-scrape-configs",
			Namespace: Namespace,
		},
		StringData: map[string]string{
			"prometheus-additional.yaml": scrapeConfigs,
		},
	}

	// Check if secret exists
	existing, err := p.clientset.CoreV1().Secrets(Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if err == nil {
		// Update existing
		secret.ResourceVersion = existing.ResourceVersion
		_, err = p.clientset.CoreV1().Secrets(Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update scrape config secret: %w", err)
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check scrape config secret: %w", err)
	}

	// Create new
	_, err = p.clientset.CoreV1().Secrets(Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create scrape config secret: %w", err)
	}

	return nil
}

// GetAdditionalScrapeConfig returns the current additional scrape config
func (p *Provisioner) GetAdditionalScrapeConfig(ctx context.Context) (string, error) {
	secret, err := p.clientset.CoreV1().Secrets(Namespace).Get(ctx, "additional-scrape-configs", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get scrape config secret: %w", err)
	}

	return string(secret.Data["prometheus-additional.yaml"]), nil
}

// GetGrafanaPassword retrieves the Grafana admin password from the secret
func (p *Provisioner) GetGrafanaPassword(ctx context.Context) (string, error) {
	secretName := fmt.Sprintf("%s-grafana", ReleaseName)
	secret, err := p.clientset.CoreV1().Secrets(Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get grafana secret: %w", err)
	}

	password, ok := secret.Data["admin-password"]
	if !ok {
		return "", fmt.Errorf("admin-password not found in secret")
	}

	return string(password), nil
}
