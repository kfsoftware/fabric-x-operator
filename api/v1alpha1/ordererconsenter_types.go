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

// OrdererConsenterSpec defines the desired state of OrdererConsenter.
type OrdererConsenterSpec struct {
	// Bootstrap mode: "configure" or "deploy"
	// When set to "configure", only configuration resources are created
	// When set to "deploy", full deployment resources are created
	BootstrapMode string `json:"bootstrapMode,omitempty"`

	// MSP ID for this consenter
	MSPID string `json:"mspid,omitempty"`

	// Party ID for this consenter
	PartyID int32 `json:"partyID,omitempty"`

	// Consenter ID for this consenter instance
	ConsenterID int32 `json:"consenterID,omitempty"`

	// Genesis block configuration
	Genesis GenesisConfig `json:"genesis"`

	// Replicas for this consenter
	Replicas int32 `json:"replicas,omitempty"`

	// Storage configuration
	Storage *StorageConfig `json:"storage,omitempty"`

	// Storage class name for PVC
	StorageClassName string `json:"storageClassName,omitempty"`

	// PVC access modes
	PVCAccessModes []string `json:"pvcAccessModes,omitempty"`

	// PVC storage size
	PVCStorageSize string `json:"pvcStorageSize,omitempty"`

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

	// Component-specific enrollment configuration
	Enrollment *EnrollmentConfig `json:"enrollment,omitempty"`

	// Component-specific SANS configuration (overrides enrollment SANS)
	SANS *SANSConfig `json:"sans,omitempty"`

	// Component-specific endpoints
	Endpoints []string `json:"endpoints,omitempty"`

	// Component-specific environment variables
	Env []EnvVar `json:"env,omitempty"`

	// Component-specific command
	Command []string `json:"command,omitempty"`

	// Component-specific args
	Args []string `json:"args,omitempty"`
}

// OrdererConsenterStatus defines the observed state of OrdererConsenter.
type OrdererConsenterStatus struct {
	// Status of the OrdererConsenter
	Status DeploymentStatus `json:"status,omitempty"`

	// Message describing the current state
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Overall phase
	Phase string `json:"phase,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ordererconsenter,singular=ordererconsenter
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OrdererConsenter is the Schema for the ordererconsenters API.
type OrdererConsenter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrdererConsenterSpec   `json:"spec,omitempty"`
	Status OrdererConsenterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrdererConsenterList contains a list of OrdererConsenter.
type OrdererConsenterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrdererConsenter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OrdererConsenter{}, &OrdererConsenterList{})
}
