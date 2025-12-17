package k3s

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	AutoscalerNamespace    = "kube-system"
	AutoscalerSecretName   = "cluster-autoscaler-cloud-config"
	AutoscalerReleaseName  = "slicer-cluster-autoscaler"
	AutoscalerRepoName     = "autoscaler"
	AutoscalerRepoURL      = "https://kubernetes.github.io/autoscaler"
	AutoscalerChartName    = "cluster-autoscaler"
	AutoscalerClusterRole  = "slicer-cluster-autoscaler"
)

// Provisioner handles Kubernetes resource provisioning for the autoscaler
type Provisioner struct {
	clientset  *kubernetes.Clientset
	kubeconfig string
}

// NewProvisioner creates a new Provisioner using standard kubeconfig loading
// Uses KUBECONFIG env var, then ~/.kube/config, following kubectl conventions
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

	// Resolve the actual kubeconfig path for later use
	if kubeconfig == "" {
		kubeconfig = loadingRules.GetDefaultFilename()
	}

	return &Provisioner{
		clientset:  clientset,
		kubeconfig: kubeconfig,
	}, nil
}

// GetK3sURLFromKubeconfig extracts the server URL from the current kubeconfig context
// Uses standard client-go loading rules (KUBECONFIG env var, then ~/.kube/config)
func GetK3sURLFromKubeconfig(kubeconfigPath string) (string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Get current context
	currentContext := rawConfig.CurrentContext
	if currentContext == "" {
		return "", fmt.Errorf("no current context set in kubeconfig")
	}

	context, exists := rawConfig.Contexts[currentContext]
	if !exists {
		return "", fmt.Errorf("context %q not found in kubeconfig", currentContext)
	}

	cluster, exists := rawConfig.Clusters[context.Cluster]
	if !exists {
		return "", fmt.Errorf("cluster %q not found in kubeconfig", context.Cluster)
	}

	if cluster.Server == "" {
		return "", fmt.Errorf("server URL is empty for cluster %q", context.Cluster)
	}

	return cluster.Server, nil
}

const (
	// K3sTokenSecretName is the name of the secret storing the K3s node join token
	K3sTokenSecretName = "k3s-node-token"
	// K3sTokenSecretKey is the key within the secret
	K3sTokenSecretKey = "token"
)

// GetK3sToken fetches the K3s node-token from the cluster secret
// The secret must be created manually with:
//   kubectl create secret generic k3s-node-token -n kube-system --from-file=token=/var/lib/rancher/k3s/server/node-token
func (p *Provisioner) GetK3sToken(ctx context.Context) (string, error) {
	secret, err := p.clientset.CoreV1().Secrets(AutoscalerNamespace).Get(ctx, K3sTokenSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", fmt.Errorf("secret %q not found in namespace %q; create it with:\n  kubectl create secret generic %s -n %s --from-file=%s=/var/lib/rancher/k3s/server/node-token",
				K3sTokenSecretName, AutoscalerNamespace, K3sTokenSecretName, AutoscalerNamespace, K3sTokenSecretKey)
		}
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	token, ok := secret.Data[K3sTokenSecretKey]
	if !ok {
		return "", fmt.Errorf("secret %q does not contain key %q", K3sTokenSecretName, K3sTokenSecretKey)
	}

	// Trim whitespace (token file often has trailing newline)
	return strings.TrimSpace(string(token)), nil
}

// SimplifiedConfig holds the simplified configuration for single-API setup
type SimplifiedConfig struct {
	// K3s settings
	K3sURL   string // K3s API URL (e.g., https://192.168.137.7:6443)
	K3sToken string // K3s join token from /var/lib/rancher/k3s/server/node-token

	// Slicer settings (single API for both CP and agents)
	SlicerURL   string // Slicer API URL (e.g., http://127.0.0.1:8080)
	SlicerToken string // Slicer auth token

	// Autoscaler settings
	NodeGroupName string // Name of the nodegroup (e.g., "api")
	MinSize       int    // Minimum number of agent nodes
	MaxSize       int    // Maximum number of agent nodes
}

// DefaultSimplifiedConfig returns default configuration values
func DefaultSimplifiedConfig() SimplifiedConfig {
	return SimplifiedConfig{
		NodeGroupName: "api",
		MinSize:       0,
		MaxSize:       10,
	}
}

// GenerateSimplifiedCloudConfig generates the cloud-config.ini for single-API setup
func GenerateSimplifiedCloudConfig(config SimplifiedConfig) string {
	return fmt.Sprintf(`[global]
k3s-url=%s
k3s-token=%s
default-min-size=%d
default-max-size=%d

[nodegroup "%s"]
slicer-url=%s
slicer-token=%s
min-size=%d
max-size=%d
`, config.K3sURL, config.K3sToken, config.MinSize, config.MaxSize,
		config.NodeGroupName, config.SlicerURL, config.SlicerToken, config.MinSize, config.MaxSize)
}

// CreateCloudConfigSecret creates or updates the cloud-config secret in kube-system
func (p *Provisioner) CreateCloudConfigSecret(ctx context.Context, cloudConfig string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AutoscalerSecretName,
			Namespace: AutoscalerNamespace,
		},
		StringData: map[string]string{
			"cloud-config": cloudConfig,
		},
	}

	existing, err := p.clientset.CoreV1().Secrets(AutoscalerNamespace).Get(ctx, AutoscalerSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = p.clientset.CoreV1().Secrets(AutoscalerNamespace).Create(ctx, secret, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create secret: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get existing secret: %w", err)
	}

	// Update existing secret
	existing.StringData = secret.StringData
	_, err = p.clientset.CoreV1().Secrets(AutoscalerNamespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	return nil
}

// PatchClusterRole adds delete permission to the autoscaler ClusterRole
func (p *Provisioner) PatchClusterRole(ctx context.Context) error {
	clusterRole, err := p.clientset.RbacV1().ClusterRoles().Get(ctx, AutoscalerClusterRole, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ClusterRole: %w", err)
	}

	// Find the rule that needs the delete verb (typically the nodes rule at index 4)
	for i, rule := range clusterRole.Rules {
		for _, resource := range rule.Resources {
			if resource == "nodes" {
				hasDelete := false
				for _, verb := range rule.Verbs {
					if verb == "delete" {
						hasDelete = true
						break
					}
				}
				if !hasDelete {
					clusterRole.Rules[i].Verbs = append(rule.Verbs, "delete")
				}
			}
		}
	}

	_, err = p.clientset.RbacV1().ClusterRoles().Update(ctx, clusterRole, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ClusterRole: %w", err)
	}

	return nil
}

// HelmConfig holds Helm deployment configuration
type HelmConfig struct {
	ImageRepository        string
	ImageTag               string
	ScaleDownEnabled       bool
	ScaleDownDelayAfterAdd string
	ScaleDownUnneededTime  string
	Expander               string
	LogVerbosity           int
}

// DefaultHelmConfig returns default Helm configuration
func DefaultHelmConfig() HelmConfig {
	return HelmConfig{
		ImageRepository:        "docker.io/welteki/cluster-autoscaler-slicer",
		ImageTag:               "latest",
		ScaleDownEnabled:       true,
		ScaleDownDelayAfterAdd: "30s",
		ScaleDownUnneededTime:  "30s",
		Expander:               "random",
		LogVerbosity:           4,
	}
}

// InstallAutoscaler deploys the cluster autoscaler via Helm
func (p *Provisioner) InstallAutoscaler(ctx context.Context, helmConfig HelmConfig) error {
	settings := cli.New()
	settings.KubeConfig = p.kubeconfig
	settings.SetNamespace(AutoscalerNamespace)

	// Add the autoscaler repo
	repoEntry := &repo.Entry{
		Name: AutoscalerRepoName,
		URL:  AutoscalerRepoURL,
	}

	// Get getter providers for HTTP/HTTPS
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
	if !f.Has(AutoscalerRepoName) {
		f.Update(repoEntry)
		if err := f.WriteFile(repoFile, 0644); err != nil {
			return fmt.Errorf("failed to write repo file: %w", err)
		}
	}

	// Setup action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), AutoscalerNamespace, "secret", func(format string, v ...interface{}) {
		fmt.Printf(format+"\n", v...)
	}); err != nil {
		return fmt.Errorf("failed to initialize helm action config: %w", err)
	}

	// Check if release exists and its status
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	releases, err := histClient.Run(AutoscalerReleaseName)

	releaseExists := err == nil && len(releases) > 0

	// Locate the chart first
	chartPathOpts := action.ChartPathOptions{}
	chartPath, err := chartPathOpts.LocateChart(
		fmt.Sprintf("%s/%s", AutoscalerRepoName, AutoscalerChartName),
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
		"fullnameOverride": "slicer-cluster-autoscaler",
		"cloudProvider":    "slicer",
		"image": map[string]interface{}{
			"repository": helmConfig.ImageRepository,
			"tag":        helmConfig.ImageTag,
		},
		"autoDiscovery": map[string]interface{}{
			"clusterName": "k3s-slicer",
		},
		"extraVolumeSecrets": map[string]interface{}{
			AutoscalerSecretName: map[string]interface{}{
				"name":      AutoscalerSecretName,
				"mountPath": "/etc/slicer/",
				"items": []map[string]interface{}{
					{
						"key":  "cloud-config",
						"path": "cloud-config",
					},
				},
			},
		},
		"extraArgs": map[string]interface{}{
			"cloud-config":               "/etc/slicer/cloud-config",
			"logtostderr":                true,
			"stderrthreshold":            "info",
			"v":                          helmConfig.LogVerbosity,
			"scale-down-enabled":         helmConfig.ScaleDownEnabled,
			"scale-down-delay-after-add": helmConfig.ScaleDownDelayAfterAdd,
			"scale-down-unneeded-time":   helmConfig.ScaleDownUnneededTime,
			"expendable-pods-priority-cutoff": -10,
			"expander":                   helmConfig.Expander,
		},
	}

	if releaseExists {
		// Upgrade existing release
		upgrade := action.NewUpgrade(actionConfig)
		upgrade.Namespace = AutoscalerNamespace
		upgrade.Wait = true
		upgrade.Timeout = 5 * time.Minute

		_, err = upgrade.Run(AutoscalerReleaseName, chart, vals)
		if err != nil {
			return fmt.Errorf("failed to upgrade autoscaler: %w", err)
		}
	} else {
		// Fresh install
		install := action.NewInstall(actionConfig)
		install.ReleaseName = AutoscalerReleaseName
		install.Namespace = AutoscalerNamespace
		install.Wait = true
		install.Timeout = 5 * time.Minute

		_, err = install.Run(chart, vals)
		if err != nil {
			return fmt.Errorf("failed to install autoscaler: %w", err)
		}
	}

	return nil
}

// UninstallAutoscaler removes the cluster autoscaler
func (p *Provisioner) UninstallAutoscaler(ctx context.Context) error {
	settings := cli.New()
	settings.KubeConfig = p.kubeconfig
	settings.SetNamespace(AutoscalerNamespace)

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), AutoscalerNamespace, "secret", func(format string, v ...interface{}) {
		fmt.Printf(format+"\n", v...)
	}); err != nil {
		return fmt.Errorf("failed to initialize helm action config: %w", err)
	}

	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = true
	uninstall.Timeout = 2 * time.Minute

	_, err := uninstall.Run(AutoscalerReleaseName)
	if err != nil {
		return fmt.Errorf("failed to uninstall autoscaler: %w", err)
	}

	return nil
}

// DeleteCloudConfigSecret removes the cloud-config secret
func (p *Provisioner) DeleteCloudConfigSecret(ctx context.Context) error {
	err := p.clientset.CoreV1().Secrets(AutoscalerNamespace).Delete(ctx, AutoscalerSecretName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}

// GetNodes returns all nodes in the cluster
func (p *Provisioner) GetNodes(ctx context.Context) ([]corev1.Node, error) {
	nodeList, err := p.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	return nodeList.Items, nil
}

// GetAutoscalerPods returns the autoscaler pods
func (p *Provisioner) GetAutoscalerPods(ctx context.Context) ([]corev1.Pod, error) {
	podList, err := p.clientset.CoreV1().Pods(AutoscalerNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/instance=slicer-cluster-autoscaler",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list autoscaler pods: %w", err)
	}
	return podList.Items, nil
}

// GetAutoscalerLogs returns logs from the autoscaler pod
func (p *Provisioner) GetAutoscalerLogs(ctx context.Context, tailLines int64) (string, error) {
	pods, err := p.GetAutoscalerPods(ctx)
	if err != nil {
		return "", err
	}

	if len(pods) == 0 {
		return "", fmt.Errorf("no autoscaler pods found")
	}

	opts := &corev1.PodLogOptions{
		TailLines: &tailLines,
	}

	req := p.clientset.CoreV1().Pods(AutoscalerNamespace).GetLogs(pods[0].Name, opts)
	logs, err := req.DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	return string(logs), nil
}

// VerifyClusterConnection checks if we can connect to the cluster
func (p *Provisioner) VerifyClusterConnection(ctx context.Context) error {
	_, err := p.clientset.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to connect to cluster: %w", err)
	}
	return nil
}

const (
	StressTestDeploymentName      = "autoscaler-stress-test"
	StressTestDeploymentNamespace = "default"
)

// CreateStressTestDeployment creates a busybox deployment for stress testing the autoscaler
func (p *Provisioner) CreateStressTestDeployment(ctx context.Context, replicas int32) error {
	labels := map[string]string{"app": StressTestDeploymentName}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      StressTestDeploymentName,
			Namespace: StressTestDeploymentNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: ptr(int64(0)),
					Containers: []corev1.Container{
						{
							Name:            "sleep",
							Image:           "docker.io/library/busybox:latest",
							Command:         []string{"sleep", "infinity"},
							ImagePullPolicy: corev1.PullIfNotPresent,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("50Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := p.clientset.AppsV1().Deployments(StressTestDeploymentNamespace).Get(ctx, StressTestDeploymentName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = p.clientset.AppsV1().Deployments(StressTestDeploymentNamespace).Create(ctx, deployment, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create deployment: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Update existing deployment
	_, err = p.clientset.AppsV1().Deployments(StressTestDeploymentNamespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}
	return nil
}

// ScaleStressTestDeployment scales the stress test deployment
func (p *Provisioner) ScaleStressTestDeployment(ctx context.Context, replicas int32) error {
	scale, err := p.clientset.AppsV1().Deployments(StressTestDeploymentNamespace).GetScale(ctx, StressTestDeploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment scale: %w", err)
	}

	scale.Spec.Replicas = replicas
	_, err = p.clientset.AppsV1().Deployments(StressTestDeploymentNamespace).UpdateScale(ctx, StressTestDeploymentName, scale, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to scale deployment: %w", err)
	}
	return nil
}

// DeleteStressTestDeployment removes the stress test deployment
func (p *Provisioner) DeleteStressTestDeployment(ctx context.Context) error {
	err := p.clientset.AppsV1().Deployments(StressTestDeploymentNamespace).Delete(ctx, StressTestDeploymentName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}
	return nil
}

// GetStressTestStatus returns the status of the stress test deployment
func (p *Provisioner) GetStressTestStatus(ctx context.Context) (ready, total int32, err error) {
	deployment, err := p.clientset.AppsV1().Deployments(StressTestDeploymentNamespace).Get(ctx, StressTestDeploymentName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("failed to get deployment: %w", err)
	}
	return deployment.Status.ReadyReplicas, deployment.Status.Replicas, nil
}

func ptr[T any](v T) *T {
	return &v
}

// Ensure rbacv1 import is used
var _ = rbacv1.ClusterRole{}
