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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecretKeyRef references a key in a Secret
type SecretKeyRef struct {
	// Name of the secret
	Name string `json:"name"`

	// Namespace of the secret
	Namespace string `json:"namespace"`

	// Key in the secret
	Key string `json:"key"`
}

// NamespaceTLS defines TLS configuration for orderer connection
type NamespaceTLS struct {
	// Enabled indicates whether TLS is enabled for the orderer connection
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// CACert is the reference to the CA certificate (only used if Enabled is true)
	CACert *SecretKeyRef `json:"caCert,omitempty"`
}

// NamespaceSpec defines the desired state of Namespace.
type NamespaceSpec struct {
	// Name is the namespace ID
	Name string `json:"name"`

	// Orderer endpoint to broadcast the namespace transaction
	Orderer string `json:"orderer"`

	// TLS configuration for orderer connection
	TLS *NamespaceTLS `json:"tls,omitempty"`

	// MSPID is the MSP identifier
	MSPID string `json:"mspID"`

	// Identity references the MSP identity to use for signing
	Identity SecretKeyRef `json:"identity"`

	// Channel is the channel name to deploy the namespace to
	Channel string `json:"channel,omitempty"`

	// VerificationKeyPath is optional path to verification key (if not using default MSP signer)
	VerificationKeyPath string `json:"verificationKeyPath,omitempty"`

	// Version is the namespace version (use -1 for new namespace, >= 0 for updates)
	// +kubebuilder:default=-1
	Version int `json:"version,omitempty"`
}

// NamespaceStatus defines the observed state of Namespace.
type NamespaceStatus struct {
	// Status of the namespace deployment
	Status string `json:"status,omitempty"`

	// Message provides details about the status
	Message string `json:"message,omitempty"`

	// TxID is the transaction ID of the deployed namespace
	TxID string `json:"txID,omitempty"`

	// Conditions represent the latest available observations of the namespace's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cns
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`,priority=1
// +kubebuilder:printcolumn:name="MSPID",type=string,JSONPath=`.spec.mspID`
// +kubebuilder:printcolumn:name="Channel",type=string,JSONPath=`.spec.channel`
// +kubebuilder:printcolumn:name="TxID",type=string,JSONPath=`.status.txID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChainNamespace is the Schema for the chainnamespaces API.
type ChainNamespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceSpec   `json:"spec,omitempty"`
	Status NamespaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChainNamespaceList contains a list of ChainNamespace.
type ChainNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChainNamespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChainNamespace{}, &ChainNamespaceList{})
}
