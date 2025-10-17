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

// CommitterSidecarSpec defines the desired state of CommitterSidecar.
type CommitterSidecarSpec struct {
	// Bootstrap mode: "configure" or "deploy"
	BootstrapMode string `json:"bootstrapMode,omitempty"`

	// MSP ID
	MSPID string `json:"mspid,omitempty"`

	// Party ID
	PartyID int32 `json:"partyID,omitempty"`

	// Image for committer component
	// +kubebuilder:default="hyperledger/fabric-x-committer"
	Image string `json:"image,omitempty"`

	// ImageTag for committer component
	// +kubebuilder:default="0.1.5"
	ImageTag string `json:"imageTag,omitempty"`

	// Genesis block configuration
	Genesis GenesisConfig `json:"genesis"`

	// Replicas for this component
	Replicas int32 `json:"replicas,omitempty"`

	// Storage configuration
	Storage *StorageConfig `json:"storage,omitempty"`

	// Resources configuration
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

	// Orderer endpoints to connect to (host:port). If empty, controller may fallback to parent Committer config.
	OrdererEndpoints []string `json:"ordererEndpoints,omitempty"`

	// Coordinator (committer) endpoint host and port the sidecar talks to
	CommitterHost string `json:"committerHost,omitempty"`
	CommitterPort int32  `json:"committerPort,omitempty"`

	// Component-specific environment variables
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Component-specific command
	Command []string `json:"command,omitempty"`

	// Component-specific args
	Args []string `json:"args,omitempty"`
}

// CommitterSidecarStatus defines the observed state of CommitterSidecar.
type CommitterSidecarStatus struct {
	// Status of the CommitterSidecar
	Status DeploymentStatus `json:"status,omitempty"`

	// Message describing the current state
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=committersidecar,singular=committersidecar
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CommitterSidecar is the Schema for the committersidecars API.
type CommitterSidecar struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CommitterSidecarSpec   `json:"spec,omitempty"`
	Status CommitterSidecarStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CommitterSidecarList contains a list of CommitterSidecar.
type CommitterSidecarList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CommitterSidecar `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CommitterSidecar{}, &CommitterSidecarList{})
}
