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
