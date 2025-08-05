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

// OrdererAssemblerSpec defines the desired state of OrdererAssembler.
type OrdererAssemblerSpec struct {
	// Deployment mode: "configure" or "deploy"
	// When set to "configure", only configuration resources are created
	// When set to "deploy", full deployment resources are created
	DeploymentMode string `json:"deploymentMode,omitempty"`

	// MSP ID for this assembler
	MSPID string `json:"mspid,omitempty"`

	// Party ID for this assembler
	PartyID int32 `json:"partyID,omitempty"`

	// Genesis block configuration
	Genesis GenesisConfig `json:"genesis"`

	// Replicas for this assembler
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

// OrdererAssemblerStatus defines the observed state of OrdererAssembler.
type OrdererAssemblerStatus struct {
	// Status of the OrdererAssembler
	Status DeploymentStatus `json:"status,omitempty"`

	// Message describing the current state
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Overall phase
	Phase string `json:"phase,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ordererassembler,singular=ordererassembler
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OrdererAssembler is the Schema for the ordererassemblers API.
type OrdererAssembler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrdererAssemblerSpec   `json:"spec,omitempty"`
	Status OrdererAssemblerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrdererAssemblerList contains a list of OrdererAssembler.
type OrdererAssemblerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrdererAssembler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OrdererAssembler{}, &OrdererAssemblerList{})
}
