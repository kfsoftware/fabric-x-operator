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
)

// SecretKeyNSSelector defines a secret key reference
type SecretKeyNSSelector struct {
	// Secret name
	Name string `json:"name"`

	// Secret key
	Key string `json:"key"`

	// Namespace of the secret
	Namespace string `json:"namespace"`
}

// OrdererOrganization represents an organization with certificates stored in secrets
type OrdererOrganization struct {
	// Name of the organization
	Name string `json:"name"`

	// MSP ID for the organization
	MSPID string `json:"mspId"`

	// Signing CA certificate reference
	SignCACertRef SecretKeyNSSelector `json:"signCaCertRef"`

	// TLS CA certificate reference
	TLSCACertRef SecretKeyNSSelector `json:"tlsCaCertRef"`

	// Admin certificate reference (optional)
	AdminCertRef *SecretKeyNSSelector `json:"adminCertRef,omitempty"`

	// Orderer endpoints in the format: "id=<partyID>,<type>,<host>:<port>"
	// Example: "id=1,broadcast,orderergroup-party1-router-service.default.svc.cluster.local:7150"
	// Types: "broadcast" (router), "deliver" (assembler)
	// +optional
	Endpoints []string `json:"endpoints,omitempty"`

	// Router configuration for this organization
	Router *RouterConfig `json:"router,omitempty"`

	// Batcher configurations for this organization
	Batchers []BatcherConfig `json:"batchers,omitempty"`

	// Consenter configuration for this organization
	Consenter *ConsenterConfig `json:"consenter,omitempty"`

	// Assembler configuration for this organization
	Assembler *AssemblerConfig `json:"assembler,omitempty"`
}

// RouterConfig represents router configuration for an organization
type RouterConfig struct {
	// Host address of the router
	Host string `json:"host"`

	// Port number for the router
	Port int32 `json:"port"`

	// Party ID for this router
	PartyID int32 `json:"partyID"`

	// Signing certificate reference
	SignCertRef SecretKeyNSSelector `json:"signCertRef"`

	// TLS certificate reference
	TLSCertRef SecretKeyNSSelector `json:"tlsCertRef"`
}

// BatcherConfig represents batcher configuration for an organization
type BatcherConfig struct {
	// Shard ID for this batcher
	ShardID int32 `json:"shardID"`

	// Host address of the batcher
	Host string `json:"host"`

	// Port number for the batcher
	Port int32 `json:"port"`

	// Signing certificate reference
	SignCertRef SecretKeyNSSelector `json:"signCertRef"`

	// TLS certificate reference
	TLSCertRef SecretKeyNSSelector `json:"tlsCertRef"`
}

// ConsenterConfig represents consenter configuration for an organization
type ConsenterConfig struct {
	// Host address of the consenter
	Host string `json:"host"`

	// Port number for the consenter
	Port int32 `json:"port"`

	// Signing certificate reference
	SignCertRef SecretKeyNSSelector `json:"signCertRef"`

	// TLS certificate reference
	TLSCertRef SecretKeyNSSelector `json:"tlsCertRef"`
}

// AssemblerConfig represents assembler configuration for an organization
type AssemblerConfig struct {
	// Host address of the assembler
	Host string `json:"host"`

	// Port number for the assembler
	Port int32 `json:"port"`

	// TLS certificate reference
	TLSCertRef SecretKeyNSSelector `json:"tlsCertRef"`
}

// OrdererNode represents a specific orderer node for consensus
type OrdererNode struct {
	// Unique identifier for the orderer node
	ID int `json:"id"`

	// Host address of the orderer node
	Host string `json:"host"`

	// Port number for the orderer node
	Port int `json:"port"`

	// MSP ID of the organization this orderer belongs to
	MSPID string `json:"mspId"`

	// Client TLS certificate reference
	ClientTLSCertRef SecretKeyNSSelector `json:"clientTlsCertRef"`

	// Server TLS certificate reference
	ServerTLSCertRef SecretKeyNSSelector `json:"serverTlsCertRef"`

	// Identity certificate reference
	IdentityRef SecretKeyNSSelector `json:"identityRef"`
}

// ApplicationOrganization represents an application organization
type ApplicationOrganization struct {
	// Name of the organization
	Name string `json:"name"`

	// MSP ID for the organization
	MSPID string `json:"mspId"`

	// Signing CA certificate reference
	SignCACertRef SecretKeyNSSelector `json:"signCaCertRef"`

	// TLS CA certificate reference
	TLSCACertRef SecretKeyNSSelector `json:"tlsCaCertRef"`

	// Admin certificate reference (optional)
	// +optional
	AdminCertRef *SecretKeyNSSelector `json:"adminCertRef,omitempty"`
}

// InternalApplicationOrg represents an internal application organization
type InternalApplicationOrg struct {
	// Signing CA certificate reference
	SignCACertRef SecretKeyNSSelector `json:"signCaCertRef"`

	// TLS CA certificate reference
	TLSCACertRef SecretKeyNSSelector `json:"tlsCaCertRef"`

	// Admin certificate reference (optional)
	AdminCertRef *SecretKeyNSSelector `json:"adminCertRef,omitempty"`
}

// ExternalApplicationOrg represents an external application organization
type ExternalApplicationOrg struct {
	// Signing CA certificate reference
	SignCACertRef SecretKeyNSSelector `json:"signCaCertRef"`

	// TLS CA certificate reference
	TLSCACertRef SecretKeyNSSelector `json:"tlsCaCertRef"`

	// Admin certificate reference
	AdminCertRef *SecretKeyNSSelector `json:"adminCertRef,omitempty"`
}

// PartyConfig represents a party configuration for SharedConfig
type PartyConfig struct {
	// Party ID for this party
	PartyID int32 `json:"partyID"`

	// CA certificates references
	CACerts []SecretKeyNSSelector `json:"caCerts"`

	// TLS CA certificates references
	TLSCACerts []SecretKeyNSSelector `json:"tlsCaCerts"`

	// Router configuration for this party
	RouterConfig *PartyRouterConfig `json:"routerConfig"`

	// Batcher configurations for this party
	BatchersConfig []PartyBatcherConfig `json:"batchersConfig"`

	// Consenter configuration for this party
	ConsenterConfig *PartyConsenterConfig `json:"consenterConfig"`

	// Assembler configuration for this party
	AssemblerConfig *PartyAssemblerConfig `json:"assemblerConfig"`
}

// PartyRouterConfig represents router configuration for a party
type PartyRouterConfig struct {
	// Host address of the router
	Host string `json:"host"`

	// Port number for the router
	Port int32 `json:"port"`

	// TLS certificate reference
	TLSCert SecretKeyNSSelector `json:"tlsCert"`
}

// PartyBatcherConfig represents batcher configuration for a party
type PartyBatcherConfig struct {
	// Shard ID for this batcher
	ShardID int32 `json:"shardID"`

	// Host address of the batcher
	Host string `json:"host"`

	// Port number for the batcher
	Port int32 `json:"port"`

	// Signing certificate reference
	SignCert SecretKeyNSSelector `json:"signCert"`

	// TLS certificate reference
	TLSCert SecretKeyNSSelector `json:"tlsCert"`
}

// PartyConsenterConfig represents consenter configuration for a party
type PartyConsenterConfig struct {
	// Host address of the consenter
	Host string `json:"host"`

	// Port number for the consenter
	Port int32 `json:"port"`

	// Signing certificate reference
	SignCert SecretKeyNSSelector `json:"signCert"`

	// TLS certificate reference
	TLSCert SecretKeyNSSelector `json:"tlsCert"`
}

// PartyAssemblerConfig represents assembler configuration for a party
type PartyAssemblerConfig struct {
	// Host address of the assembler
	Host string `json:"host"`

	// Port number for the assembler
	Port int32 `json:"port"`

	// TLS certificate reference
	TLSCert SecretKeyNSSelector `json:"tlsCert"`
}

// ConsensusConfig represents consensus configuration
type ConsensusConfig struct {
	// SmartBFT configuration
	SmartBFT *SmartBFTConfig `json:"smartBFT"`
}

// SmartBFTConfig represents SmartBFT consensus configuration
type SmartBFTConfig struct {
	// Self ID for this node
	SelfID int32 `json:"selfID"`

	// Request batch max count
	RequestBatchMaxCount int32 `json:"requestBatchMaxCount"`

	// Request batch max bytes
	RequestBatchMaxBytes int32 `json:"requestBatchMaxBytes"`

	// Request batch max interval
	RequestBatchMaxInterval string `json:"requestBatchMaxInterval"`

	// Incoming message buffer size
	IncomingMessageBufferSize int32 `json:"incomingMessageBufferSize"`

	// Request pool size
	RequestPoolSize int32 `json:"requestPoolSize"`

	// Request forward timeout
	RequestForwardTimeout string `json:"requestForwardTimeout"`

	// Request complain timeout
	RequestComplainTimeout string `json:"requestComplainTimeout"`

	// Request auto remove timeout
	RequestAutoRemoveTimeout string `json:"requestAutoRemoveTimeout"`

	// View change resend interval
	ViewChangeResendInterval string `json:"viewChangeResendInterval"`

	// View change timeout
	ViewChangeTimeout string `json:"viewChangeTimeout"`

	// Leader heartbeat timeout
	LeaderHeartbeatTimeout string `json:"leaderHeartbeatTimeout"`

	// Leader heartbeat count
	LeaderHeartbeatCount int32 `json:"leaderHeartbeatCount"`

	// Number of ticks behind before syncing
	NumOfTicksBehindBeforeSyncing int32 `json:"numOfTicksBehindBeforeSyncing"`

	// Collect timeout
	CollectTimeout string `json:"collectTimeout"`

	// Sync on start
	SyncOnStart bool `json:"syncOnStart"`

	// Speed up view change
	SpeedUpViewChange bool `json:"speedUpViewChange"`

	// Leader rotation
	LeaderRotation bool `json:"leaderRotation"`

	// Decisions per leader
	DecisionsPerLeader int32 `json:"decisionsPerLeader"`

	// Request max bytes
	RequestMaxBytes int32 `json:"requestMaxBytes"`

	// Request pool submit timeout
	RequestPoolSubmitTimeout string `json:"requestPoolSubmitTimeout"`
}

// BatchingConfig represents batching configuration
type BatchingConfig struct {
	// Batch timeouts configuration
	BatchTimeouts *BatchTimeoutsConfig `json:"batchTimeouts"`

	// Batch size configuration
	BatchSize *BatchSizeConfig `json:"batchSize"`

	// Request max bytes
	RequestMaxBytes int32 `json:"requestMaxBytes"`
}

// BatchTimeoutsConfig represents batch timeouts configuration
type BatchTimeoutsConfig struct {
	// Batch creation timeout
	BatchCreationTimeout string `json:"batchCreationTimeout"`

	// First strike threshold
	FirstStrikeThreshold string `json:"firstStrikeThreshold"`

	// Second strike threshold
	SecondStrikeThreshold string `json:"secondStrikeThreshold"`

	// Auto remove timeout
	AutoRemoveTimeout string `json:"autoRemoveTimeout"`
}

// BatchSizeConfig represents batch size configuration
type BatchSizeConfig struct {
	// Max message count
	MaxMessageCount int32 `json:"maxMessageCount"`

	// Absolute max bytes
	AbsoluteMaxBytes int32 `json:"absoluteMaxBytes"`

	// Preferred max bytes
	PreferredMaxBytes int32 `json:"preferredMaxBytes"`
}

// GenesisSpec defines the desired state of Genesis.
type GenesisSpec struct {
	// Channel ID for the genesis block
	// +kubebuilder:validation:Required
	ChannelID string `json:"channelID"`

	// Config template reference
	ConfigTemplate ConfigTemplateReference `json:"configTemplate"`

	// OrdererOrganizations (with certificates stored in secrets)
	OrdererOrganizations []OrdererOrganization `json:"ordererOrganizations,omitempty"`

	// Application organizations (can be internal or external)
	ApplicationOrgs []ApplicationOrganization `json:"applicationOrgs,omitempty"`

	// Specific consenter nodes for consensus
	Consenters []OrdererNode `json:"consenters,omitempty"`

	// Parties for SharedConfig
	Parties []PartyConfig `json:"parties,omitempty"`

	// Consensus configuration
	ConsensusConfig *ConsensusConfig `json:"consensusConfig,omitempty"`

	// Batching configuration
	BatchingConfig *BatchingConfig `json:"batchingConfig,omitempty"`

	// Meta namespace verification CA certificate reference (deprecated, no longer used)
	// +optional
	MetaNamespaceCA *SecretKeyNSSelector `json:"metaNamespaceCA,omitempty"`

	// Output configuration
	Output GenesisOutput `json:"output"`
}

// ConfigTemplateReference represents a reference to a config template
type ConfigTemplateReference struct {
	// Name of the ConfigMap containing the template
	ConfigMapName string `json:"configMapName"`

	// Key in the ConfigMap containing the template
	Key string `json:"key"`
}

// GenesisOutput defines the output configuration for the genesis block
type GenesisOutput struct {
	// Name of the Secret to store the genesis block
	SecretName string `json:"secretName"`

	// Key in the Secret to store the genesis block
	BlockKey string `json:"blockKey"`
}

// GenesisStatus defines the observed state of Genesis.
type GenesisStatus struct {
	// Status of the genesis block generation
	Status string `json:"status,omitempty"`

	// Message describing the current state
	Message string `json:"message,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=genesis,singular=genesis
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Genesis is the Schema for the genesis API.
type Genesis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GenesisSpec   `json:"spec,omitempty"`
	Status GenesisStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GenesisList contains a list of Genesis.
type GenesisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Genesis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Genesis{}, &GenesisList{})
}
