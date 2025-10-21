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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EndorserSpec defines the desired state of Endorser.
type EndorserSpec struct {
	// Bootstrap mode: "configure" or "deploy"
	BootstrapMode string `json:"bootstrapMode,omitempty"`

	// MSP ID for the endorser
	MSPID string `json:"mspid,omitempty"`

	// Common configuration
	Common *CommonComponentConfig `json:"common,omitempty"`

	// Core configuration for the endorser
	Core EndorserCoreConfig `json:"core"`

	// Enrollment configuration for certificates
	Enrollment *EnrollmentConfig `json:"enrollment,omitempty"`

	// Subject Alternative Names (SANS) for TLS certificates
	// This overrides SANS from enrollment configuration if specified
	SANS *SANSConfig `json:"sans,omitempty"`

	// Ingress configuration
	Ingress *IngressConfig `json:"ingress,omitempty"`

	// Image to use for the endorser
	Image string `json:"image,omitempty"`

	// Image version/tag
	Version string `json:"version,omitempty"`

	// Command to run in the container (overrides image's ENTRYPOINT)
	Command []string `json:"command,omitempty"`

	// Args to pass to the container (overrides image's CMD)
	Args []string `json:"args,omitempty"`

	// Ports to expose on the endorser pod and service
	// +optional
	Ports []EndorserPort `json:"ports,omitempty"`
}

// EndorserPort defines a port to expose on the endorser
type EndorserPort struct {
	// Name of the port (e.g., "grpc", "http", "metrics")
	Name string `json:"name"`

	// Port number to expose
	Port int32 `json:"port"`

	// Protocol (TCP or UDP), defaults to TCP
	// +optional
	// +kubebuilder:default=TCP
	Protocol string `json:"protocol,omitempty"`
}

// EndorserCoreConfig represents the typed structure of the core.yaml file
type EndorserCoreConfig struct {
	// Logging configuration
	Logging *LoggingConfig `json:"logging,omitempty"`

	// FSC (Fabric Smart Client) configuration
	FSC FSCConfig `json:"fsc"`

	// Fabric configuration
	Fabric *FabricConfig `json:"fabric,omitempty"`

	// Token configuration
	Token *TokenConfig `json:"token,omitempty"`
}

// LoggingConfig defines logging settings
type LoggingConfig struct {
	// Log level spec (e.g., "info", "debug", "warn", "error")
	Spec string `json:"spec,omitempty"`

	// Log format string
	Format string `json:"format,omitempty"`
}

// FSCConfig defines Fabric Smart Client configuration
type FSCConfig struct {
	// Node ID (e.g., "endorser1")
	ID string `json:"id"`

	// Identity configuration
	Identity FSCIdentity `json:"identity"`

	// P2P configuration
	P2P FSCP2PConfig `json:"p2p"`

	// Persistence configuration
	Persistences map[string]PersistenceConfig `json:"persistences,omitempty"`

	// Endpoint configuration
	Endpoint *EndpointConfig `json:"endpoint,omitempty"`
}

// FSCIdentity defines the identity configuration
type FSCIdentity struct {
	// Certificate configuration
	Cert CertFileConfig `json:"cert"`

	// Private key configuration
	Key KeyFileConfig `json:"key"`
}

// CertFileConfig defines certificate file configuration
type CertFileConfig struct {
	// File path to certificate (will be managed by secret)
	File string `json:"file,omitempty"`

	// Secret reference for certificate
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// KeyFileConfig defines private key file configuration
type KeyFileConfig struct {
	// File path to private key (will be managed by secret)
	File string `json:"file,omitempty"`

	// Secret reference for private key
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// FSCP2PConfig defines P2P configuration
type FSCP2PConfig struct {
	// Listen address (e.g., "/ip4/0.0.0.0/tcp/9301")
	ListenAddress string `json:"listenAddress"`

	// P2P type (e.g., "websocket")
	Type string `json:"type,omitempty"`

	// P2P options
	Opts *P2POptions `json:"opts,omitempty"`
}

// P2POptions defines P2P options
type P2POptions struct {
	// Routing configuration
	Routing *RoutingConfig `json:"routing,omitempty"`
}

// RoutingConfig defines routing configuration
type RoutingConfig struct {
	// Path to routing configuration file
	Path string `json:"path,omitempty"`

	// Inline routing configuration as raw JSON
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Inline *runtime.RawExtension `json:"inline,omitempty"`
}

// PersistenceConfig defines persistence configuration
type PersistenceConfig struct {
	// Persistence type (e.g., "sqlite", "postgres")
	Type string `json:"type"`

	// Persistence options
	Opts map[string]string `json:"opts,omitempty"`
}

// EndpointConfig defines endpoint resolver configuration
type EndpointConfig struct {
	// List of endpoint resolvers
	Resolvers []EndpointResolver `json:"resolvers,omitempty"`
}

// EndpointResolver defines a single endpoint resolver
type EndpointResolver struct {
	// Resolver name (e.g., "issuer", "endorser1")
	Name string `json:"name"`

	// Identity configuration
	Identity *ResolverIdentity `json:"identity,omitempty"`

	// Addresses for different protocols
	Addresses map[string]string `json:"addresses,omitempty"`
}

// ResolverIdentity defines identity for resolver
type ResolverIdentity struct {
	// Identity ID
	ID string `json:"id,omitempty"`

	// Path to identity certificate
	Path string `json:"path,omitempty"`

	// Secret reference for identity
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// FabricConfig defines Hyperledger Fabric configuration
type FabricConfig struct {
	// Enable Fabric integration
	Enabled bool `json:"enabled,omitempty"`

	// Default Fabric configuration
	Default *FabricDefaultConfig `json:"default,omitempty"`
}

// FabricDefaultConfig defines default Fabric configuration
type FabricDefaultConfig struct {
	// Driver type (e.g., "fabricx")
	Driver string `json:"driver,omitempty"`

	// Default MSP ID
	DefaultMSP string `json:"defaultMSP,omitempty"`

	// MSP configurations
	MSPs []MSPConfig `json:"msps,omitempty"`

	// TLS configuration
	TLS *FabricTLSConfig `json:"tls,omitempty"`

	// Peer configurations
	Peers []PeerConfig `json:"peers,omitempty"`

	// Query service configurations
	QueryService []QueryServiceConfig `json:"queryService,omitempty"`

	// Channel configurations
	Channels []ChannelConfig `json:"channels,omitempty"`
}

// MSPConfig defines MSP configuration
type MSPConfig struct {
	// MSP ID (e.g., "user", "endorser")
	ID string `json:"id"`

	// MSP type (e.g., "bccsp")
	MSPType string `json:"mspType,omitempty"`

	// MSP ID for Fabric (e.g., "Org1MSP")
	MSPID string `json:"mspID"`

	// Path to MSP directory
	Path string `json:"path,omitempty"`

	// Secret reference for MSP
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// FabricTLSConfig defines TLS configuration for Fabric
type FabricTLSConfig struct {
	// Enable TLS
	Enabled bool `json:"enabled,omitempty"`
}

// PeerConfig defines peer configuration
type PeerConfig struct {
	// Peer address (e.g., "peer0.org1.example.com:7051")
	Address string `json:"address"`

	// Usage type (e.g., "delivery", "endorsement")
	Usage string `json:"usage,omitempty"`

	// TLS root certificate file
	TLSRootCertFile string `json:"tlsRootCertFile,omitempty"`

	// Secret reference for TLS certificate
	TLSRootCertSecretRef *SecretRef `json:"tlsRootCertSecretRef,omitempty"`
}

// QueryServiceConfig defines query service configuration
type QueryServiceConfig struct {
	// Service address (e.g., "localhost:5500")
	Address string `json:"address"`
}

// ChannelConfig defines channel configuration
type ChannelConfig struct {
	// Channel name
	Name string `json:"name"`

	// Is this the default channel
	Default bool `json:"default,omitempty"`
}

// TokenConfig defines token service configuration
type TokenConfig struct {
	// Enable token service
	Enabled bool `json:"enabled,omitempty"`

	// Token Management Service (TMS) configurations
	TMS map[string]TMSConfig `json:"tms,omitempty"`
}

// TMSConfig defines Token Management Service configuration
type TMSConfig struct {
	// Network name
	Network string `json:"network,omitempty"`

	// Channel name
	Channel string `json:"channel,omitempty"`

	// Namespace
	Namespace string `json:"namespace,omitempty"`

	// Driver type (e.g., "zkatdlog")
	Driver string `json:"driver,omitempty"`

	// Wallets configuration (pass-through to core.yaml)
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Wallets map[string]string `json:"wallets,omitempty"`

	// Services configuration
	Services *TMSServices `json:"services,omitempty"`

	// Public parameters configuration
	PublicParameters *PublicParametersConfig `json:"publicParameters,omitempty"`
}

// PublicParametersConfig defines public parameters configuration
type PublicParametersConfig struct {
	// Path to public parameters file
	Path string `json:"path,omitempty"`
}

// TMSServices defines TMS services configuration
type TMSServices struct {
	// Network service configuration
	Network *NetworkServiceConfig `json:"network,omitempty"`
}

// NetworkServiceConfig defines network service configuration
type NetworkServiceConfig struct {
	// Fabric-specific configuration
	Fabric *FabricNetworkConfig `json:"fabric,omitempty"`
}

// FabricNetworkConfig defines Fabric network configuration
type FabricNetworkConfig struct {
	// FSC endorsement configuration
	FSCEndorsement *FSCEndorsementConfig `json:"fsc_endorsement,omitempty"`
}

// FSCEndorsementConfig defines FSC endorsement configuration
type FSCEndorsementConfig struct {
	// Is this node an endorser
	Endorser bool `json:"endorser,omitempty"`

	// Endorser identity ID
	ID string `json:"id,omitempty"`

	// Endorsement policy
	Policy *EndorsementPolicy `json:"policy,omitempty"`

	// List of endorsers
	Endorsers []string `json:"endorsers,omitempty"`
}

// EndorsementPolicy defines endorsement policy
type EndorsementPolicy struct {
	// Policy type (e.g., "all", "any", "majority")
	Type string `json:"type,omitempty"`

	// Threshold for majority policies
	Threshold int `json:"threshold,omitempty"`
}

// EndorserStatus defines the observed state of Endorser.
type EndorserStatus struct {
	// Deployment status
	Status DeploymentStatus `json:"status,omitempty"`

	// Status message
	Message string `json:"message,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Core config secret name
	CoreConfigSecretName string `json:"coreConfigSecretName,omitempty"`

	// Certificate secret names
	CertificateSecrets map[string]string `json:"certificateSecrets,omitempty"`

	// Service endpoint
	ServiceEndpoint string `json:"serviceEndpoint,omitempty"`

	// P2P endpoint
	P2PEndpoint string `json:"p2pEndpoint,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Endorser is the Schema for the endorsers API.
type Endorser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndorserSpec   `json:"spec,omitempty"`
	Status EndorserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndorserList contains a list of Endorser.
type EndorserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Endorser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Endorser{}, &EndorserList{})
}
