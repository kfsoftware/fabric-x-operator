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

// CommitterSpec defines the desired state of Committer.
type CommitterSpec struct {
	// Bootstrap mode: "configure" or "deploy"
	BootstrapMode string `json:"bootstrapMode,omitempty"`

	// MSP ID
	MSPID string `json:"mspid,omitempty"`

	// Image for committer components
	// +kubebuilder:default="hyperledger/fabric-x-committer"
	Image string `json:"image,omitempty"`

	// ImageTag for committer components
	// +kubebuilder:default="0.1.5"
	ImageTag string `json:"imageTag,omitempty"`

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
	Validator *ValidatorComponentConfig `json:"validator,omitempty"`

	// Query Service configuration
	QueryService *ComponentConfig `json:"queryService,omitempty"`

	// Orderer endpoints shared by sidecar (host:port)
	OrdererEndpoints []string `json:"ordererEndpoints,omitempty"`

	// Committer coordinator endpoint exposed to sidecar (host and port)
	CommitterHost string `json:"committerHost,omitempty"`
	CommitterPort int32  `json:"committerPort,omitempty"`

	// Coordinator Verifier endpoints (host:port) used by coordinator
	CoordinatorVerifierEndpoints []string `json:"coordinatorVerifierEndpoints,omitempty"`

	// Coordinator Validator/Committer endpoints (host:port) used by coordinator
	CoordinatorValidatorCommitterEndpoints []string `json:"coordinatorValidatorCommitterEndpoints,omitempty"`
}

// ValidatorComponentConfig provides validator-specific configuration with PostgreSQL support
type ValidatorComponentConfig struct {
	// Inherit from common config
	CommonComponentConfig `json:",inline"`

	// Component-specific ingress configuration
	Ingress *IngressConfig `json:"ingress,omitempty"`

	// Component-specific enrollment configuration
	Enrollment *EnrollmentConfig `json:"enrollment,omitempty"`

	// Component-specific SANS configuration (overrides enrollment SANS)
	SANS *SANSConfig `json:"sans,omitempty"`

	// Component-specific endpoints
	Endpoints []string `json:"endpoints,omitempty"`

	// Component-specific environment variables
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Component-specific command
	Command []string `json:"command,omitempty"`

	// Component-specific args
	Args []string `json:"args,omitempty"`

	// PostgreSQL configuration
	PostgreSQL *PostgreSQLConfig `json:"postgresql,omitempty"`
}

// PostgreSQLConfig defines PostgreSQL database configuration
type PostgreSQLConfig struct {
	// Database host
	Host string `json:"host,omitempty"`

	// Database port
	Port int32 `json:"port,omitempty"`

	// Database name
	Database string `json:"database,omitempty"`

	// Database username
	Username string `json:"username,omitempty"`

	// Database password secret reference
	PasswordSecret *SecretRef `json:"passwordSecret,omitempty"`

	// Maximum number of connections
	MaxConnections int32 `json:"maxConnections,omitempty"`

	// Minimum number of connections
	MinConnections int32 `json:"minConnections,omitempty"`

	// Load balance setting
	LoadBalance bool `json:"loadBalance,omitempty"`

	// Retry configuration
	Retry *PostgreSQLRetryConfig `json:"retry,omitempty"`
}

// PostgreSQLRetryConfig defines retry configuration for PostgreSQL
type PostgreSQLRetryConfig struct {
	// Maximum elapsed time for retries
	MaxElapsedTime string `json:"maxElapsedTime,omitempty"`
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
