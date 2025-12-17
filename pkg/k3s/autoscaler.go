package k3s

import (
	"fmt"
	"strings"
)

// NodeGroup represents a Slicer instance that can provision worker nodes
type NodeGroup struct {
	Name        string
	SlicerURL   string
	SlicerToken string
	MinSize     int // 0 means use global default
	MaxSize     int // 0 means use global default
}

// AutoscalerConfig holds configuration for the Cluster Autoscaler
type AutoscalerConfig struct {
	K3sURL         string
	K3sToken       string
	CABundle       string // Optional: path to custom CA bundle
	DefaultMinSize int
	DefaultMaxSize int
	NodeGroups     []NodeGroup
}

func DefaultAutoscalerConfig() AutoscalerConfig {
	return AutoscalerConfig{
		DefaultMinSize: 0,
		DefaultMaxSize: 10,
		NodeGroups:     []NodeGroup{},
	}
}

// GenerateCloudConfig generates the cloud-config.ini file for the Cluster Autoscaler
func GenerateCloudConfig(config AutoscalerConfig) string {
	var sb strings.Builder

	sb.WriteString("[global]\n")
	sb.WriteString(fmt.Sprintf("k3s-url=%s\n", config.K3sURL))
	sb.WriteString(fmt.Sprintf("k3s-token=%s\n", config.K3sToken))

	if config.CABundle != "" {
		sb.WriteString(fmt.Sprintf("ca-bundle=%s\n", config.CABundle))
	}

	sb.WriteString(fmt.Sprintf("default-min-size=%d\n", config.DefaultMinSize))
	sb.WriteString(fmt.Sprintf("default-max-size=%d\n", config.DefaultMaxSize))

	for _, ng := range config.NodeGroups {
		sb.WriteString(fmt.Sprintf("\n[nodegroup \"%s\"]\n", ng.Name))
		sb.WriteString(fmt.Sprintf("slicer-url=%s\n", ng.SlicerURL))
		sb.WriteString(fmt.Sprintf("slicer-token=%s\n", ng.SlicerToken))
		if ng.MinSize > 0 {
			sb.WriteString(fmt.Sprintf("min-size=%d\n", ng.MinSize))
		}
		if ng.MaxSize > 0 {
			sb.WriteString(fmt.Sprintf("max-size=%d\n", ng.MaxSize))
		}
	}

	return sb.String()
}

// HelmValuesConfig holds configuration for the Helm values file
type HelmValuesConfig struct {
	ClusterName              string
	ImageRepository          string
	ImageTag                 string
	ScaleDownEnabled         bool
	ScaleDownDelayAfterAdd   string
	ScaleDownUnneededTime    string
	Expander                 string // random, least-waste, most-pods, price
	LogVerbosity             int
	ExtraVolumes             []ExtraVolume
}

type ExtraVolume struct {
	SecretName string
	MountPath  string
	Key        string
	Path       string
}

func DefaultHelmValuesConfig() HelmValuesConfig {
	return HelmValuesConfig{
		ClusterName:              "k3s-slicer",
		ImageRepository:          "docker.io/openfaasltd/cluster-autoscaler-slicer",
		ImageTag:                 "latest",
		ScaleDownEnabled:         true,
		ScaleDownDelayAfterAdd:   "30s",
		ScaleDownUnneededTime:    "30s",
		Expander:                 "random",
		LogVerbosity:             4,
	}
}

// GenerateHelmValues generates the values-slicer.yaml file for Helm deployment
func GenerateHelmValues(config HelmValuesConfig) string {
	var sb strings.Builder

	sb.WriteString("fullnameOverride: slicer-cluster-autoscaler\n\n")

	sb.WriteString("image:\n")
	sb.WriteString(fmt.Sprintf("  repository: %s\n", config.ImageRepository))
	sb.WriteString(fmt.Sprintf("  tag: %s\n", config.ImageTag))
	sb.WriteString("\n")

	sb.WriteString("cloudProvider: slicer\n\n")

	sb.WriteString("autoDiscovery:\n")
	sb.WriteString(fmt.Sprintf("  clusterName: %s\n", config.ClusterName))
	sb.WriteString("\n")

	sb.WriteString("extraVolumeSecrets:\n")
	sb.WriteString("  cluster-autoscaler-cloud-config:\n")
	sb.WriteString("    name: cluster-autoscaler-cloud-config\n")
	sb.WriteString("    mountPath: /etc/slicer/\n")
	sb.WriteString("    items:\n")
	sb.WriteString("      - key: cloud-config\n")
	sb.WriteString("        path: cloud-config\n")

	// Add any extra volumes (like CA bundles)
	for _, vol := range config.ExtraVolumes {
		sb.WriteString(fmt.Sprintf("  %s:\n", vol.SecretName))
		sb.WriteString(fmt.Sprintf("    name: %s\n", vol.SecretName))
		sb.WriteString(fmt.Sprintf("    mountPath: %s\n", vol.MountPath))
		sb.WriteString("    items:\n")
		sb.WriteString(fmt.Sprintf("      - key: %s\n", vol.Key))
		sb.WriteString(fmt.Sprintf("        path: %s\n", vol.Path))
	}
	sb.WriteString("\n")

	sb.WriteString("extraArgs:\n")
	sb.WriteString("  cloud-config: /etc/slicer/cloud-config\n")
	sb.WriteString("  logtostderr: true\n")
	sb.WriteString("  stderrthreshold: info\n")
	sb.WriteString(fmt.Sprintf("  v: %d\n", config.LogVerbosity))
	sb.WriteString(fmt.Sprintf("  scale-down-enabled: %t\n", config.ScaleDownEnabled))
	sb.WriteString(fmt.Sprintf("  scale-down-delay-after-add: \"%s\"\n", config.ScaleDownDelayAfterAdd))
	sb.WriteString(fmt.Sprintf("  scale-down-unneeded-time: \"%s\"\n", config.ScaleDownUnneededTime))
	sb.WriteString("  expendable-pods-priority-cutoff: -10\n")
	sb.WriteString(fmt.Sprintf("  expander: %s\n", config.Expander))

	return sb.String()
}

// GenerateTestDeployment generates a test deployment YAML for testing autoscaling
func GenerateTestDeployment(name string, replicas int) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
spec:
  replicas: %d
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      terminationGracePeriodSeconds: 0
      containers:
        - name: %s
          image: docker.io/library/busybox:latest
          command: ["sleep", "infinity"]
          imagePullPolicy: IfNotPresent
          resources:
            requests:
              cpu: 50m
              memory: 50Mi
`, name, replicas, name, name, name)
}

// GenerateRoutesScript generates the add-routes.sh script for network routing
func GenerateRoutesScript(routes []string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("set -euo pipefail\n\n")

	for _, route := range routes {
		sb.WriteString(fmt.Sprintf("ip route add %s || true\n", route))
	}

	return sb.String()
}

// GenerateRoutesService generates the systemd service for persistent routes
func GenerateRoutesService() string {
	return `[Unit]
Description=Add slicer routes
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/root/add-routes.sh
RemainAfterExit=true
User=root

[Install]
WantedBy=multi-user.target
`
}

// InstallCommands returns the shell commands needed to deploy the autoscaler
func InstallCommands(cloudConfigPath, valuesPath string) []string {
	return []string{
		"# Create the cloud-config secret",
		fmt.Sprintf("kubectl create secret generic -n kube-system cluster-autoscaler-cloud-config --from-file=cloud-config=%s", cloudConfigPath),
		"",
		"# Add the autoscaler Helm repo",
		"helm repo add autoscaler https://kubernetes.github.io/autoscaler",
		"",
		"# Install/upgrade the cluster autoscaler",
		fmt.Sprintf("helm upgrade --install slicer-cluster-autoscaler autoscaler/cluster-autoscaler --namespace=kube-system --values=%s", valuesPath),
		"",
		"# Patch the ClusterRole to allow node deletion",
		`kubectl patch clusterrole/slicer-cluster-autoscaler --type='json' -p='[{"op": "add", "path": "/rules/4/verbs/-", "value": "delete"}]'`,
	}
}
