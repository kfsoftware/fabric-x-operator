/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CommonComponentConfig provides shared configuration for all components
type CommonComponentConfig struct {
	// Replicas for this component
	Replicas int32 `json:"replicas,omitempty"`

	// Storage configuration
	Storage *StorageConfig `json:"storage,omitempty"`

	// Resources configuration
	// +kubebuilder:validation:Optional
	// +nullable
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Security context
	SecurityContext *SecurityContext `json:"securityContext,omitempty"`

	// Pod annotations
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// Pod labels
	PodLabels map[string]string `json:"podLabels,omitempty"`

	// Volumes
	Volumes []Volume `json:"volumes,omitempty"`

	// Affinity
	Affinity *Affinity `json:"affinity,omitempty"`

	// Volume mounts
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty"`

	// Image pull secrets
	ImagePullSecrets []ImagePullSecret `json:"imagePullSecrets,omitempty"`

	// Tolerations
	Tolerations []Toleration `json:"tolerations,omitempty"`
}

// Volume defines a Kubernetes volume
type Volume struct {
	// Volume name
	Name string `json:"name"`

	// Volume source
	VolumeSource VolumeSource `json:"volumeSource"`
}

// VolumeSource defines the source of a volume
type VolumeSource struct {
	// Empty directory
	EmptyDir *EmptyDirVolumeSource `json:"emptyDir,omitempty"`

	// Config map
	ConfigMap *ConfigMapVolumeSource `json:"configMap,omitempty"`

	// Secret
	Secret *SecretVolumeSource `json:"secret,omitempty"`

	// Persistent volume claim
	PersistentVolumeClaim *PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`

	// Host path
	HostPath *HostPathVolumeSource `json:"hostPath,omitempty"`
}

// EmptyDirVolumeSource represents an empty directory
type EmptyDirVolumeSource struct {
	// Medium
	Medium string `json:"medium,omitempty"`

	// Size limit
	SizeLimit string `json:"sizeLimit,omitempty"`
}

// ConfigMapVolumeSource represents a ConfigMap
type ConfigMapVolumeSource struct {
	// ConfigMap name
	Name string `json:"name"`

	// Items to include
	Items []KeyToPath `json:"items,omitempty"`

	// Default mode
	DefaultMode *int32 `json:"defaultMode,omitempty"`
}

// SecretVolumeSource represents a Secret
type SecretVolumeSource struct {
	// Secret name
	SecretName string `json:"secretName"`

	// Items to include
	Items []KeyToPath `json:"items,omitempty"`

	// Default mode
	DefaultMode *int32 `json:"defaultMode,omitempty"`
}

// PersistentVolumeClaimVolumeSource represents a PVC
type PersistentVolumeClaimVolumeSource struct {
	// Claim name
	ClaimName string `json:"claimName"`

	// Read only
	ReadOnly bool `json:"readOnly,omitempty"`
}

// HostPathVolumeSource represents a host path
type HostPathVolumeSource struct {
	// Path
	Path string `json:"path"`

	// Type
	Type string `json:"type,omitempty"`
}

// KeyToPath defines a key to path mapping
type KeyToPath struct {
	// Key
	Key string `json:"key"`

	// Path
	Path string `json:"path"`

	// Mode
	Mode *int32 `json:"mode,omitempty"`
}

// VolumeMount defines a volume mount
type VolumeMount struct {
	// Mount path
	MountPath string `json:"mountPath"`

	// Volume name
	Name string `json:"name"`

	// Read only
	ReadOnly bool `json:"readOnly,omitempty"`

	// Sub path
	SubPath string `json:"subPath,omitempty"`
}

// Affinity defines pod affinity
type Affinity struct {
	// Node affinity
	NodeAffinity *NodeAffinity `json:"nodeAffinity,omitempty"`

	// Pod affinity
	PodAffinity *PodAffinity `json:"podAffinity,omitempty"`

	// Pod anti-affinity
	PodAntiAffinity *PodAntiAffinity `json:"podAntiAffinity,omitempty"`
}

// NodeAffinity defines node affinity
type NodeAffinity struct {
	// Required during scheduling ignored during execution
	RequiredDuringSchedulingIgnoredDuringExecution *NodeSelector `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`

	// Preferred during scheduling ignored during execution
	PreferredDuringSchedulingIgnoredDuringExecution []PreferredSchedulingTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// NodeSelector defines a node selector
type NodeSelector struct {
	// Node selector terms
	NodeSelectorTerms []NodeSelectorTerm `json:"nodeSelectorTerms"`
}

// NodeSelectorTerm defines a node selector term
type NodeSelectorTerm struct {
	// Match expressions
	MatchExpressions []NodeSelectorRequirement `json:"matchExpressions,omitempty"`

	// Match fields
	MatchFields []NodeSelectorRequirement `json:"matchFields,omitempty"`
}

// NodeSelectorRequirement defines a node selector requirement
type NodeSelectorRequirement struct {
	// Key
	Key string `json:"key"`

	// Operator
	Operator string `json:"operator"`

	// Values
	Values []string `json:"values,omitempty"`
}

// PreferredSchedulingTerm defines a preferred scheduling term
type PreferredSchedulingTerm struct {
	// Weight
	Weight int32 `json:"weight"`

	// Preference
	Preference NodeSelectorTerm `json:"preference"`
}

// PodAffinity defines pod affinity
type PodAffinity struct {
	// Required during scheduling ignored during execution
	RequiredDuringSchedulingIgnoredDuringExecution []PodAffinityTerm `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`

	// Preferred during scheduling ignored during execution
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// PodAntiAffinity defines pod anti-affinity
type PodAntiAffinity struct {
	// Required during scheduling ignored during execution
	RequiredDuringSchedulingIgnoredDuringExecution []PodAffinityTerm `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`

	// Preferred during scheduling ignored during execution
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// PodAffinityTerm defines a pod affinity term
type PodAffinityTerm struct {
	// Label selector
	LabelSelector *LabelSelector `json:"labelSelector,omitempty"`

	// Namespaces
	Namespaces []string `json:"namespaces,omitempty"`

	// Topology key
	TopologyKey string `json:"topologyKey"`
}

// WeightedPodAffinityTerm defines a weighted pod affinity term
type WeightedPodAffinityTerm struct {
	// Weight
	Weight int32 `json:"weight"`

	// Pod affinity term
	PodAffinityTerm PodAffinityTerm `json:"podAffinityTerm"`
}

// LabelSelector defines a label selector
type LabelSelector struct {
	// Match labels
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// Match expressions
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// LabelSelectorRequirement defines a label selector requirement
type LabelSelectorRequirement struct {
	// Key
	Key string `json:"key"`

	// Operator
	Operator string `json:"operator"`

	// Values
	Values []string `json:"values,omitempty"`
}

// ImagePullSecret defines an image pull secret
type ImagePullSecret struct {
	// Secret name
	Name string `json:"name"`
}

// Toleration defines a toleration
type Toleration struct {
	// Key
	Key string `json:"key,omitempty"`

	// Operator
	Operator string `json:"operator,omitempty"`

	// Value
	Value string `json:"value,omitempty"`

	// Effect
	Effect string `json:"effect,omitempty"`

	// Toleration seconds
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty"`
}

// StorageConfig defines storage requirements
type StorageConfig struct {
	// PVC name to use (if empty, will be auto-generated)
	PVCName string `json:"pvcName,omitempty"`

	// Storage class to use
	StorageClass string `json:"storageClass,omitempty"`

	// Storage size
	Size string `json:"size"`

	// Access mode
	AccessMode string `json:"accessMode,omitempty"`
}

// ResourceConfig defines resource requirements
type ResourceConfig struct {
	// CPU requests
	CPURequests string `json:"cpuRequests,omitempty"`

	// CPU limits
	CPULimits string `json:"cpuLimits,omitempty"`

	// Memory requests
	MemoryRequests string `json:"memoryRequests,omitempty"`

	// Memory limits
	MemoryLimits string `json:"memoryLimits,omitempty"`
}

// SecurityContext defines security settings
type SecurityContext struct {
	// Run as user ID
	RunAsUser *int64 `json:"runAsUser,omitempty"`

	// Run as group ID
	RunAsGroup *int64 `json:"runAsGroup,omitempty"`

	// Read-only root filesystem
	ReadOnlyRootFilesystem *bool `json:"readOnlyRootFilesystem,omitempty"`
}

// IngressConfig defines ingress configuration
type IngressConfig struct {
	// Istio-specific configuration
	Istio *IstioConfig `json:"istio,omitempty"`
}

// IstioConfig defines Istio ingress configuration
type IstioConfig struct {
	// Hosts for this component
	Hosts []string `json:"hosts"`

	// Port number
	Port int32 `json:"port"`

	// Ingress gateway name
	IngressGateway string `json:"ingressGateway"`

	// TLS configuration
	TLS *TLSConfig `json:"tls,omitempty"`
}

// TLSConfig defines TLS settings
type TLSConfig struct {
	// Secret name containing TLS certificate
	SecretName string `json:"secretName,omitempty"`

	// TLS enabled
	Enabled bool `json:"enabled,omitempty"`
}

// CertificateConfig defines certificate enrollment configuration
type CertificateConfig struct {
	// CA host
	CAHost string `json:"cahost,omitempty"`

	// CA name
	CAName string `json:"caname,omitempty"`

	// CA port
	CAPort int32 `json:"caport,omitempty"`

	// CA TLS configuration
	CATLS *CATLSConfig `json:"catls,omitempty"`

	// Enrollment ID
	EnrollID string `json:"enrollid,omitempty"`

	// Enrollment secret
	EnrollSecret string `json:"enrollsecret,omitempty"`
}

// CATLSConfig defines CA TLS configuration
type CATLSConfig struct {
	// CA certificate (base64 encoded)
	CACert string `json:"cacert,omitempty"`

	// Secret reference for CA certificate
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// SecretRef defines a reference to a Kubernetes secret
type SecretRef struct {
	// Secret name
	Name string `json:"name"`

	// Secret key
	Key string `json:"key"`

	// Secret namespace
	Namespace string `json:"namespace,omitempty"`
}

// ComponentConfig provides component-specific configuration with inheritance
type ComponentConfig struct {
	// Inherit from common config
	CommonComponentConfig `json:",inline"`

	// Component-specific ingress configuration
	Ingress *IngressConfig `json:"ingress,omitempty"`

	// Component-specific certificates
	Certificates *CertificateConfig `json:"certificates,omitempty"`

	// Component-specific endpoints
	Endpoints []string `json:"endpoints,omitempty"`

	// Component-specific environment variables
	Env []EnvVar `json:"env,omitempty"`

	// Component-specific command
	Command []string `json:"command,omitempty"`

	// Component-specific args
	Args []string `json:"args,omitempty"`
}

// EnvVar defines an environment variable
type EnvVar struct {
	// Variable name
	Name string `json:"name"`

	// Variable value
	Value string `json:"value,omitempty"`

	// Value from secret
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty"`
}

// EnvVarSource defines the source of an environment variable
type EnvVarSource struct {
	// Secret key reference
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// SecretKeySelector defines a secret key reference
type SecretKeySelector struct {
	// Secret name
	Name string `json:"name"`

	// Secret key
	Key string `json:"key"`
}

// GenesisConfig defines genesis block configuration
type GenesisConfig struct {
	// Secret name containing genesis block
	SecretName string `json:"secretName"`

	// Secret key containing genesis block
	SecretKey string `json:"secretKey"`

	// Secret namespace
	SecretNamespace string `json:"secretNamespace,omitempty"`
}

// OrdererGroupSpec defines the desired state of OrdererGroup.
type OrdererGroupSpec struct {
	// Bootstrap mode: "configure" or "deploy"
	BootstrapMode string `json:"bootstrapMode,omitempty"`

	// MSP ID
	MSPID string `json:"mspid,omitempty"`

	// Party ID for this orderer group
	PartyID int32 `json:"partyID,omitempty"`

	// Common configuration applied to all components
	Common *CommonComponentConfig `json:"common,omitempty"`

	// Genesis block configuration
	Genesis GenesisConfig `json:"genesis"`

	// Component-specific configurations
	Components OrdererComponents `json:"components"`

	// Global enrollment configuration (inherited by components)C
	Enrollment *EnrollmentConfig `json:"enrollment,omitempty"`
}

// EnrollmentConfig defines enrollment configuration
type EnrollmentConfig struct {
	// Sign certificate configuration
	Sign *CertificateConfig `json:"sign,omitempty"`

	// TLS certificate configuration
	TLS *CertificateConfig `json:"tls,omitempty"`
}

// BatcherInstance defines a single batcher instance configuration
type BatcherInstance struct {
	// Inherit from common config
	CommonComponentConfig `json:",inline"`

	// Shard ID for this batcher instance
	ShardID int32 `json:"shardID"`

	// Component-specific ingress configuration
	Ingress *IngressConfig `json:"ingress,omitempty"`

	// Component-specific certificates
	Certificates *CertificateConfig `json:"certificates,omitempty"`

	// Component-specific endpoints
	Endpoints []string `json:"endpoints,omitempty"`

	// Component-specific environment variables
	Env []EnvVar `json:"env,omitempty"`

	// Component-specific command
	Command []string `json:"command,omitempty"`

	// Component-specific args
	Args []string `json:"args,omitempty"`
}

// OrdererComponents defines configurations for each component
type OrdererComponents struct {
	// Consenter configuration
	Consenter *ComponentConfig `json:"consenter,omitempty"`

	// Batcher configurations - can have multiple batcher instances
	Batchers []BatcherInstance `json:"batchers,omitempty"`

	// Assembler configuration
	Assembler *ComponentConfig `json:"assembler,omitempty"`

	// Router configuration
	Router *ComponentConfig `json:"router,omitempty"`
}

// OrdererGroupStatus defines the observed state of OrdererGroup.
type OrdererGroupStatus struct {
	// Status of the OrdererGroup
	Status DeploymentStatus `json:"status,omitempty"`

	// Message describing the current state
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Component statuses
	ComponentStatuses map[string]ComponentStatus `json:"componentStatuses,omitempty"`

	// Overall phase
	Phase string `json:"phase,omitempty"`
}

// ComponentStatus defines the status of a component
type ComponentStatus struct {
	// Ready status
	Ready bool `json:"ready"`

	// Replicas ready
	ReplicasReady int32 `json:"replicasReady"`

	// Total replicas
	ReplicasTotal int32 `json:"replicasTotal"`

	// Last update time
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=orderergroup,singular=orderergroup
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OrdererGroup is the Schema for the orderergroups API.
type OrdererGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrdererGroupSpec   `json:"spec,omitempty"`
	Status OrdererGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrdererGroupList contains a list of OrdererGroup.
type OrdererGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrdererGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OrdererGroup{}, &OrdererGroupList{})
}
