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

// CAEnrollmentSpec defines the desired state of CAEnrollment
type CAEnrollmentSpec struct {
	// Name of the Fabric CA instance to use for enrollment
	// +kubebuilder:validation:Required
	CAName string `json:"caName"`

	// Namespace of the Fabric CA instance
	// +kubebuilder:validation:Required
	CANamespace string `json:"caNamespace"`

	// Enrollment ID (username) for this identity
	// +kubebuilder:validation:Required
	EnrollID string `json:"enrollID"`

	// Enrollment secret (password) reference from a secret
	// +kubebuilder:validation:Required
	EnrollSecretRef CAEnrollmentSecretRef `json:"enrollSecretRef"`

	// Type of identity (e.g., "client", "peer", "orderer", "admin")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=client;peer;orderer;admin
	Type string `json:"type"`

	// Affiliation for the identity (optional)
	// +optional
	Affiliation string `json:"affiliation,omitempty"`

	// Attributes for the identity (optional)
	// +optional
	Attrs []CAEnrollmentAttr `json:"attrs,omitempty"`

	// Maximum number of enrollments (0 for unlimited)
	// +optional
	// +kubebuilder:default=0
	MaxEnrollments int `json:"maxEnrollments,omitempty"`

	// Output secret name where the private key and certificate will be stored
	// +kubebuilder:validation:Required
	OutputSecretName string `json:"outputSecretName"`

	// TLS configuration for connecting to Fabric CA
	// +optional
	TLS *CAEnrollmentTLS `json:"tls,omitempty"`
}

// CAEnrollmentSecretRef defines a reference to a specific key in a secret
type CAEnrollmentSecretRef struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key in the secret
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// CAEnrollmentAttr defines an attribute for a CA enrollment
type CAEnrollmentAttr struct {
	// Name of the attribute
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Value of the attribute
	// +kubebuilder:validation:Required
	Value string `json:"value"`

	// Whether this attribute should be included in the enrollment certificate (ECert)
	// +optional
	// +kubebuilder:default=false
	ECert bool `json:"ecert,omitempty"`
}

// CAEnrollmentTLS defines TLS configuration for connecting to Fabric CA
type CAEnrollmentTLS struct {
	// TLS CA certificate reference from a secret
	// +optional
	CACertRef *CAEnrollmentSecretRef `json:"caCertRef,omitempty"`

	// Skip TLS certificate verification
	// +optional
	// +kubebuilder:default=false
	Insecure bool `json:"insecure,omitempty"`
}

// CAEnrollmentStatus defines the observed state of CAEnrollment
type CAEnrollmentStatus struct {
	// Status of the identity enrollment
	// +optional
	Status string `json:"status,omitempty"`

	// Message describing the current state
	// +optional
	Message string `json:"message,omitempty"`

	// Enrollment timestamp
	// +optional
	EnrollmentTime *metav1.Time `json:"enrollmentTime,omitempty"`

	// Certificate expiration time
	// +optional
	CertificateExpiry *metav1.Time `json:"certificateExpiry,omitempty"`

	// Number of times this identity has been enrolled
	// +optional
	EnrollmentCount int `json:"enrollmentCount,omitempty"`

	// Conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=caenroll,singular=caenrollment
// +kubebuilder:printcolumn:name="CA",type="string",JSONPath=".spec.caName"
// +kubebuilder:printcolumn:name="EnrollID",type="string",JSONPath=".spec.enrollID"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CAEnrollment is the Schema for the caenrollments API.
type CAEnrollment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CAEnrollmentSpec   `json:"spec,omitempty"`
	Status CAEnrollmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CAEnrollmentList contains a list of CAEnrollment.
type CAEnrollmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CAEnrollment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CAEnrollment{}, &CAEnrollmentList{})
}
