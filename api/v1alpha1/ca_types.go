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

// DeploymentStatus represents the status of a deployment
type DeploymentStatus string

const (
	PendingStatus        DeploymentStatus = "PENDING"
	FailedStatus         DeploymentStatus = "FAILED"
	RunningStatus        DeploymentStatus = "RUNNING"
	UnknownStatus        DeploymentStatus = "UNKNOWN"
	UpdatingVersion      DeploymentStatus = "UPDATING_VERSION"
	UpdatingCertificates DeploymentStatus = "UPDATING_CERTIFICATES"
)

// CredentialStore represents the type of credential store
type CredentialStore string

const (
	CredentialStoreKubernetes = "kubernetes"
	CredentialStoreVault      = "vault"
)

// FabricCANames represents certificate subject names
type FabricCANames struct {
	C  string `json:"C,omitempty"`
	ST string `json:"ST,omitempty"`
	O  string `json:"O,omitempty"`
	L  string `json:"L,omitempty"`
	OU string `json:"OU,omitempty"`
}

// FabricCAIdentity represents a CA identity
type FabricCAIdentity struct {
	Name        string                `json:"name"`
	Pass        string                `json:"pass"`
	Type        string                `json:"type"`
	Affiliation string                `json:"affiliation"`
	Attrs       FabricCAIdentityAttrs `json:"attrs"`
}

// FabricCAIdentityAttrs represents identity attributes
type FabricCAIdentityAttrs struct {
	RegistrarRoles string `json:"hf.Registrar.Roles,omitempty"`
	DelegateRoles  string `json:"hf.Registrar.DelegateRoles,omitempty"`
	Attributes     string `json:"hf.Registrar.Attributes,omitempty"`
	Revoker        bool   `json:"hf.Revoker,omitempty"`
	IntermediateCA bool   `json:"hf.IntermediateCA,omitempty"`
	GenCRL         bool   `json:"hf.GenCRL,omitempty"`
	AffiliationMgr bool   `json:"hf.AffiliationMgr,omitempty"`
}

// FabricCACRL represents CRL configuration
type FabricCACRL struct {
	Expiry string `json:"expiry,omitempty"`
}

// FabricCACSR represents certificate signing request configuration
type FabricCACSR struct {
	CN    string          `json:"cn,omitempty"`
	Hosts []string        `json:"hosts,omitempty"`
	Names []FabricCANames `json:"names,omitempty"`
	CA    FabricCACSRCA   `json:"ca,omitempty"`
}

// FabricCACSRCA represents CA CSR configuration
type FabricCACSRCA struct {
	Expiry     string `json:"expiry,omitempty"`
	PathLength int    `json:"pathLength,omitempty"`
}

// FabricCASigning represents signing configuration
type FabricCASigning struct {
	Default  FabricCASigningDefault  `json:"default,omitempty"`
	Profiles FabricCASigningProfiles `json:"profiles,omitempty"`
}

// FabricCASigningDefault represents default signing configuration
type FabricCASigningDefault struct {
	Expiry string   `json:"expiry,omitempty"`
	Usage  []string `json:"usage,omitempty"`
}

// FabricCASigningProfiles represents signing profiles
type FabricCASigningProfiles struct {
	CA  FabricCASigningSignProfile `json:"ca,omitempty"`
	TLS FabricCASigningTLSProfile  `json:"tls,omitempty"`
}

// FabricCASigningSignProfile represents a signing profile
type FabricCASigningSignProfile struct {
	Usage        []string                             `json:"usage,omitempty"`
	Expiry       string                               `json:"expiry,omitempty"`
	CAConstraint FabricCASigningSignProfileConstraint `json:"caconstraint,omitempty"`
}

// FabricCASigningSignProfileConstraint represents CA constraint
type FabricCASigningSignProfileConstraint struct {
	IsCA       bool `json:"isca,omitempty"`
	MaxPathLen int  `json:"maxpathlen,omitempty"`
}

// FabricCASigningTLSProfile represents TLS signing profile
type FabricCASigningTLSProfile struct {
	Usage  []string `json:"usage,omitempty"`
	Expiry string   `json:"expiry,omitempty"`
}

// FabricCAItemConf represents CA item configuration
type FabricCAItemConf struct {
	Name         string                   `json:"name"`
	CFG          FabricCAItemCFG          `json:"cfg,omitempty"`
	CSR          FabricCACSR              `json:"csr,omitempty"`
	CRL          FabricCACRL              `json:"crl,omitempty"`
	Registry     FabricCAItemRegistry     `json:"registry,omitempty"`
	Intermediate FabricCAItemIntermediate `json:"intermediate,omitempty"`
	Affiliations []FabricCAAffiliation    `json:"affiliations,omitempty"`
	BCCSP        FabricCAItemBCCSP        `json:"bccsp,omitempty"`
	Signing      FabricCASigning          `json:"signing,omitempty"`
}

// FabricCAItemCFG represents CA item configuration
type FabricCAItemCFG struct {
	Identities   FabricCAItemCFGIdentities   `json:"identities,omitempty"`
	Affiliations FabricCAItemCFGAffiliations `json:"affiliations,omitempty"`
}

// FabricCAItemCFGIdentities represents identities configuration
type FabricCAItemCFGIdentities struct {
	AllowRemove bool `json:"allowremove,omitempty"`
}

// FabricCAItemCFGAffiliations represents affiliations configuration
type FabricCAItemCFGAffiliations struct {
	AllowRemove bool `json:"allowremove,omitempty"`
}

// FabricCAItemRegistry represents registry configuration
type FabricCAItemRegistry struct {
	MaxEnrollments int                `json:"maxenrollments,omitempty"`
	Identities     []FabricCAIdentity `json:"identities,omitempty"`
}

// FabricCAItemIntermediate represents intermediate CA configuration
type FabricCAItemIntermediate struct {
	ParentServer FabricCAItemIntermediateParentServer `json:"parentserver,omitempty"`
}

// FabricCAItemIntermediateParentServer represents parent server configuration
type FabricCAItemIntermediateParentServer struct {
	URL    string `json:"url,omitempty"`
	CAName string `json:"caname,omitempty"`
}

// FabricCAAffiliation represents affiliation configuration
type FabricCAAffiliation struct {
	Name        string   `json:"name,omitempty"`
	Departments []string `json:"departments,omitempty"`
}

// FabricCAItemBCCSP represents BCCSP configuration
type FabricCAItemBCCSP struct {
	Default string              `json:"default,omitempty"`
	SW      FabricCAItemBCCSPSW `json:"sw,omitempty"`
}

// FabricCAItemBCCSPSW represents software BCCSP configuration
type FabricCAItemBCCSPSW struct {
	Hash     string `json:"hash,omitempty"`
	Security int    `json:"security,omitempty"`
}

// FabricCATLS represents TLS configuration
type FabricCATLS struct {
	Subject FabricCANames `json:"subject,omitempty"`

	// Domain names for TLS certificate generation
	Domains []string `json:"domains,omitempty"`
}

// FabricCAIngress represents ingress configuration
type FabricCAIngress struct {
	// Enabled flag to enable/disable ingress
	Enabled bool `json:"enabled,omitempty"`

	// Istio-specific configuration
	Istio *FabricCAIstioConfig `json:"istio,omitempty"`
}

// FabricCAIstioConfig defines Istio ingress configuration
type FabricCAIstioConfig struct {
	// Hosts for this CA
	Hosts []string `json:"hosts"`

	// Port number
	Port int32 `json:"port"`

	// Ingress gateway name
	IngressGateway string `json:"ingressGateway"`

	// TLS configuration
	TLS *FabricCATLSConfig `json:"tls,omitempty"`
}

// FabricCATLSConfig defines TLS settings
type FabricCATLSConfig struct {
	// Secret name containing TLS certificate
	SecretName string `json:"secretName,omitempty"`

	// TLS enabled
	Enabled bool `json:"enabled,omitempty"`
}

// FabricCADatabase represents database configuration
type FabricCADatabase struct {
	Type       string `json:"type,omitempty"`
	Datasource string `json:"datasource,omitempty"`
}

// FabricCASpecService represents service configuration
type FabricCASpecService struct {
	ServiceType corev1.ServiceType `json:"type,omitempty"`
}

// FabricCAMetrics represents metrics configuration
type FabricCAMetrics struct {
	Provider string                `json:"provider,omitempty"`
	Statsd   FabricCAMetricsStatsd `json:"statsd,omitempty"`
}

// FabricCAMetricsStatsd represents statsd metrics configuration
type FabricCAMetricsStatsd struct {
	Network       string `json:"network,omitempty"`
	Address       string `json:"address,omitempty"`
	WriteInterval string `json:"writeInterval,omitempty"`
	Prefix        string `json:"prefix,omitempty"`
}

// FabricCAStorage represents storage configuration
type FabricCAStorage struct {
	StorageClass string `json:"storageClass,omitempty"`
	AccessMode   string `json:"accessMode,omitempty"`
	Size         string `json:"size,omitempty"`
}

// FabricCAVault represents Vault configuration
type FabricCAVault struct {
	Vault FabricCAVaultConfig `json:"vault,omitempty"`
}

// FabricCAVaultConfig represents Vault configuration details
type FabricCAVaultConfig struct {
	// Add Vault-specific configuration fields as needed
}

// CASpec defines the desired state of CA
type CASpec struct {
	// Pod annotations
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// Pod labels
	PodLabels map[string]string `json:"podLabels,omitempty"`

	// Affinity configuration
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Image pull secrets
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Node selector
	NodeSelector *corev1.NodeSelector `json:"nodeSelector,omitempty"`

	// Service monitor configuration
	ServiceMonitor *FabricCAServiceMonitor `json:"serviceMonitor,omitempty"`

	// Database configuration
	Database FabricCADatabase `json:"db,omitempty"`

	// Hosts for the Fabric CA
	Hosts []string `json:"hosts,omitempty"`

	// Service configuration
	Service FabricCASpecService `json:"service,omitempty"`

	// Ingress configuration
	Ingress *FabricCAIngress `json:"ingress,omitempty"`

	// Image
	Image string `json:"image,omitempty"`

	// Version
	Version string `json:"version,omitempty"`

	// Replicas
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Debug mode
	Debug bool `json:"debug,omitempty"`

	// CRL size limit
	CLRSizeLimit int `json:"clrSizeLimit,omitempty"`

	// Metrics configuration
	Metrics FabricCAMetrics `json:"metrics,omitempty"`

	// Storage configuration
	Storage FabricCAStorage `json:"storage,omitempty"`

	// TLS configuration
	TLS FabricCATLS `json:"tls,omitempty"`

	// CA configuration
	CA FabricCAItemConf `json:"ca,omitempty"`

	// TLS CA configuration
	TLSCA FabricCAItemConf `json:"tlsca,omitempty"`

	// CORS configuration
	Cors FabricCACors `json:"cors,omitempty"`

	// Credential store
	CredentialStore CredentialStore `json:"credentialStore,omitempty"`

	// Vault configuration
	Vault FabricCAVault `json:"vault,omitempty"`

	// Environment variables
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// FabricCAServiceMonitor represents service monitor configuration
type FabricCAServiceMonitor struct {
	Enabled           bool              `json:"enabled,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Interval          string            `json:"interval,omitempty"`
	ScrapeTimeout     string            `json:"scrapeTimeout,omitempty"`
	Scheme            string            `json:"scheme,omitempty"`
	Relabelings       []string          `json:"relabelings,omitempty"`
	TargetLabels      []string          `json:"targetLabels,omitempty"`
	MetricRelabelings []string          `json:"metricRelabelings,omitempty"`
	SampleLimit       int               `json:"sampleLimit,omitempty"`
}

// FabricCACors represents CORS configuration
type FabricCACors struct {
	Enabled bool     `json:"enabled,omitempty"`
	Origins []string `json:"origins,omitempty"`
}

// CAStatus defines the observed state of CA
type CAStatus struct {
	// Status of the CA
	Status DeploymentStatus `json:"status,omitempty"`

	// Message describing the current state
	Message string `json:"message,omitempty"`

	// Node port
	NodePort int `json:"nodePort,omitempty"`

	// TLS Certificate to connect to the CA
	TlsCert string `json:"tlsCert,omitempty"`

	// Root certificate for Sign certificates generated by CA
	CACert string `json:"caCert,omitempty"`

	// Root certificate for TLS certificates generated by CA
	TLSCACert string `json:"tlsCACert,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ca,singular=ca
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CA is the Schema for the cas API
type CA struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CASpec   `json:"spec,omitempty"`
	Status CAStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CAList contains a list of CA
type CAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CA `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CA{}, &CAList{})
}
