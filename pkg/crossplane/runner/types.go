package runner

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CRD API constants
const (
	VMGroup      = "vm.slicervm.crossplane.io"
	VMVersion    = "v1alpha1"
	VMKind       = "VM"
	VMResource   = "vms"
	VMAPIVersion = VMGroup + "/" + VMVersion

	ConfigGroup      = "template.crossplane.io"
	ConfigVersion    = "v1alpha1"
	ConfigKind       = "ClusterProviderConfig"
	ConfigResource   = "clusterproviderconfigs"
	ConfigAPIVersion = ConfigGroup + "/" + ConfigVersion
)

// VMParameters are the configurable fields of a Slicer VM.
type VMParameters struct {
	// HostGroup is the host group to create the VM in.
	HostGroup string `json:"hostGroup,omitempty"`
	// CPUs is the number of virtual CPUs for the VM.
	CPUs int `json:"cpus,omitempty"`
	// RAMGB is the amount of RAM in GB for the VM.
	RAMGB int `json:"ramGb,omitempty"`
	// Userdata is the cloud-init userdata script to run on boot.
	Userdata string `json:"userdata,omitempty"`
	// SSHKeys is a list of SSH public keys to add to the VM.
	SSHKeys []string `json:"sshKeys,omitempty"`
	// ImportUser is a GitHub username to import SSH keys from.
	ImportUser string `json:"importUser,omitempty"`
	// Tags are labels to apply to the VM.
	Tags []string `json:"tags,omitempty"`
}

// VMObservation are the observable fields of a Slicer VM.
type VMObservation struct {
	Hostname  string `json:"hostname,omitempty"`
	IP        string `json:"ip,omitempty"`
	State     string `json:"state,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// ProviderConfigReference references a provider config.
type ProviderConfigReference struct {
	Name string `json:"name"`
}

// VMSpec defines the desired state of a VM.
type VMSpec struct {
	ForProvider       VMParameters            `json:"forProvider"`
	ProviderConfigRef ProviderConfigReference `json:"providerConfigRef,omitempty"`
}

// VMStatus represents the observed state of a VM.
type VMStatus struct {
	AtProvider VMObservation `json:"atProvider,omitempty"`
	Conditions []Condition   `json:"conditions,omitempty"`
}

// Condition represents a resource condition.
type Condition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// VM is a managed resource that represents a Slicer VM.
type VM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VMSpec   `json:"spec"`
	Status VMStatus `json:"status,omitempty"`
}

// VMList contains a list of VM.
type VMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VM `json:"items"`
}

// ProviderCredentials required to authenticate.
type ProviderCredentials struct {
	Source    string           `json:"source"`
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// SecretReference references a secret.
type SecretReference struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Key       string `json:"key"`
}

// ProviderConfigSpec defines the provider configuration.
type ProviderConfigSpec struct {
	Credentials ProviderCredentials `json:"credentials"`
	URL         string              `json:"url,omitempty"`
	HostGroup   string              `json:"hostGroup,omitempty"`
}

// ClusterProviderConfig configures the SlicerVM provider.
type ClusterProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ProviderConfigSpec `json:"spec"`
}
