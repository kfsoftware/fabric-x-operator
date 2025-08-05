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

// CommitterSpec defines the desired state of Committer.
type CommitterSpec struct {
	// Bootstrap mode: "configure" or "deploy"
	BootstrapMode string `json:"bootstrapMode,omitempty"`

	// MSP ID
	MSPID string `json:"mspid,omitempty"`

	// Common configuration applied to all components
	Common *CommonComponentConfig `json:"common,omitempty"`

	// Genesis block configuration for Sidecar
	Genesis GenesisConfig `json:"genesis"`

	// Component-specific configurations
	Components CommitterComponents `json:"components"`

	// Global enrollment configuration (inherited by components)
	Enrollment *EnrollmentConfig `json:"enrollment,omitempty"`
}

// CommitterComponents defines configurations for each committer component
type CommitterComponents struct {
	// Sidecar configuration
	Sidecar *ComponentConfig `json:"sidecar,omitempty"`

	// Coordinator configuration
	Coordinator *ComponentConfig `json:"coordinator,omitempty"`

	// Verifier Service configuration
	VerifierService *ComponentConfig `json:"verifierService,omitempty"`

	// Validator configuration
	Validator *ComponentConfig `json:"validator,omitempty"`
}

// CommitterStatus defines the observed state of Committer.
type CommitterStatus struct {
	// Status of the Committer
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

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=committer,singular=committer
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Committer is the Schema for the committers API.
type Committer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CommitterSpec   `json:"spec,omitempty"`
	Status CommitterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CommitterList contains a list of Committer.
type CommitterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Committer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Committer{}, &CommitterList{})
}
