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

// IdentitySpec defines the desired state of Identity
type IdentitySpec struct {
	// Type of identity
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=client;peer;orderer;admin;user
	Type string `json:"type"`

	// MSPID for this identity
	// +kubebuilder:validation:Required
	MspID string `json:"mspID"`

	// Enrollment configuration - used to enroll a new identity with a CA
	// +kubebuilder:validation:Required
	Enrollment *IdentityEnrollment `json:"enrollment"`

	// Output configuration - where to store the identity materials
	// +kubebuilder:validation:Required
	Output IdentityOutput `json:"output"`
}

// IdentityEnrollment defines enrollment configuration for creating a new identity
type IdentityEnrollment struct {
	// Reference to the Fabric CA to use for enrollment
	// +kubebuilder:validation:Required
	CARef IdentityCARef `json:"caRef"`

	// Enrollment ID (username)
	// +kubebuilder:validation:Required
	EnrollID string `json:"enrollID"`

	// Enrollment secret (password) reference
	// +kubebuilder:validation:Required
	EnrollSecretRef SecretKeyNSSelector `json:"enrollSecretRef"`

	// Affiliation for the identity
	// +optional
	Affiliation string `json:"affiliation,omitempty"`

	// Attributes for the identity
	// +optional
	Attrs []IdentityAttribute `json:"attrs,omitempty"`

	// Whether to also enroll for TLS certificates
	// +optional
	// +kubebuilder:default=true
	EnrollTLS bool `json:"enrollTLS,omitempty"`

	// TLS CA reference (if different from signing CA)
	// +optional
	TLSCARef *IdentityCARef `json:"tlsCARef,omitempty"`
}

// IdentityCARef references a Fabric CA instance
type IdentityCARef struct {
	// Name of the CA custom resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the CA (optional, defaults to identity's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// TLS CA certificate reference for connecting to the CA
	// +optional
	CACertRef *SecretKeyNSSelector `json:"caCertRef,omitempty"`
}

// IdentityAttribute defines an attribute for an identity
type IdentityAttribute struct {
	// Name of the attribute
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Value of the attribute
	// +kubebuilder:validation:Required
	Value string `json:"value"`

	// Whether this attribute should be included in the enrollment certificate
	// +optional
	// +kubebuilder:default=false
	ECert bool `json:"ecert,omitempty"`
}

// IdentityOutput defines where to store the identity materials
type IdentityOutput struct {
	// Secret name prefix for storing identity materials
	// Will create secrets: <prefix>-sign-cert, <prefix>-sign-key, <prefix>-tls-cert, <prefix>-tls-key
	// +kubebuilder:validation:Required
	SecretPrefix string `json:"secretPrefix"`

	// Namespace for output secrets (optional, defaults to identity's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Additional labels to apply to output secrets
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// IdentityStatus defines the observed state of Identity
type IdentityStatus struct {
	// Status of the identity
	// +optional
	Status string `json:"status,omitempty"`

	// Message describing the current state
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Enrollment time (when identity was enrolled or imported)
	// +optional
	EnrollmentTime *metav1.Time `json:"enrollmentTime,omitempty"`

	// Certificate expiration time
	// +optional
	CertificateExpiry *metav1.Time `json:"certificateExpiry,omitempty"`

	// TLS certificate expiration time
	// +optional
	TLSCertificateExpiry *metav1.Time `json:"tlsCertificateExpiry,omitempty"`

	// Output secrets created
	// +optional
	OutputSecrets IdentityOutputSecrets `json:"outputSecrets,omitempty"`
}

// IdentityOutputSecrets tracks the secrets created for this identity
type IdentityOutputSecrets struct {
	// Sign certificate secret name
	// +optional
	SignCert string `json:"signCert,omitempty"`

	// Sign key secret name
	// +optional
	SignKey string `json:"signKey,omitempty"`

	// Sign CA certificate secret name
	// +optional
	SignCACert string `json:"signCACert,omitempty"`

	// TLS certificate secret name
	// +optional
	TLSCert string `json:"tlsCert,omitempty"`

	// TLS key secret name
	// +optional
	TLSKey string `json:"tlsKey,omitempty"`

	// TLS CA certificate secret name
	// +optional
	TLSCACert string `json:"tlsCACert,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=id,singular=identity
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="MSPID",type="string",JSONPath=".spec.mspID"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Expiry",type="date",JSONPath=".status.certificateExpiry"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Identity is the Schema for the identities API.
// It represents a Fabric identity (user, peer, orderer, admin) with its cryptographic materials.
type Identity struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IdentitySpec   `json:"spec,omitempty"`
	Status IdentityStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IdentityList contains a list of Identity.
type IdentityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Identity `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Identity{}, &IdentityList{})
}
