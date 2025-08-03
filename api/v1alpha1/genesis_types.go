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

// InternalOrganization represents an organization that uses Fabric CA
type InternalOrganization struct {
	// Name of the organization
	Name string `json:"name"`

	// MSP ID for the organization
	MSPID string `json:"mspId"`

	// Reference to the Fabric CA
	CAReference CAReference `json:"caReference"`

	// Admin identity name in the CA
	AdminIdentity string `json:"adminIdentity,omitempty"`

	// Orderer identity name in the CA (if this is an orderer org)
	OrdererIdentity string `json:"ordererIdentity,omitempty"`
}

// CAReference represents a reference to a Fabric CA
type CAReference struct {
	// Name of the CA resource
	Name string `json:"name"`

	// Namespace of the CA resource
	Namespace string `json:"namespace"`
}

// ExternalOrganization represents an organization with externally provided certificates
type ExternalOrganization struct {
	// Name of the organization
	Name string `json:"name"`

	// MSP ID for the organization
	MSPID string `json:"mspId"`

	// Signing certificate (base64 encoded)
	SignCert string `json:"signCert"`

	// TLS certificate (base64 encoded)
	TLSCert string `json:"tlsCert"`

	// Admin certificate (base64 encoded)
	AdminCert string `json:"adminCert"`

	// Orderer certificate (base64 encoded, if this is an orderer org)
	OrdererCert string `json:"ordererCert,omitempty"`
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

	// Client TLS certificate (base64 encoded)
	ClientTLSCert string `json:"clientTlsCert"`

	// Server TLS certificate (base64 encoded)
	ServerTLSCert string `json:"serverTlsCert"`

	// Identity certificate (base64 encoded)
	Identity string `json:"identity"`
}

// GenesisSpec defines the desired state of Genesis.
type GenesisSpec struct {
	// Config template reference
	ConfigTemplate ConfigTemplateReference `json:"configTemplate"`

	// Internal organizations (using Fabric CA)
	InternalOrgs []InternalOrganization `json:"internalOrgs,omitempty"`

	// External organizations (with provided certificates)
	ExternalOrgs []ExternalOrganization `json:"externalOrgs,omitempty"`

	// Specific orderer nodes for consensus
	OrdererNodes []OrdererNode `json:"ordererNodes,omitempty"`

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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=genesis,singular=genesis
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Genesis is the Schema for the geneses API.
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
