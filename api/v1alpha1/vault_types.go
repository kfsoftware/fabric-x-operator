package v1alpha1

type VaultBackend string

const (
	VaultBackendKV  VaultBackend = "kv"
	VaultBackendPKI VaultBackend = "pki"
)

// VaultPKICertificateRequest defines the configuration for requesting certificates from Vault's PKI backend
type VaultPKICertificateRequest struct {

	// PKI is the PKI backend to use for certificate generation
	// +kubebuilder:validation:Required
	PKI string `json:"pki"`

	// Role is the PKI role to use for certificate generation
	// +kubebuilder:validation:Required
	Role string `json:"role"`

	// TTL is the requested time-to-live of the certificate
	// +optional
	// +kubebuilder:default:="8760h"
	TTL string `json:"ttl,omitempty"`

	// UserIDs are optional user identifiers that can be included in the certificate
	// +optional
	// +nullable
	// +kubebuilder:default:={}
	UserIDs []string `json:"userIDs,omitempty"`
}

type VaultSpecConf struct {
	// URL of the Vault server
	URL string `json:"url"`
	// Token for direct authentication to Vault
	// +optional
	// +nullable
	TokenSecretRef *VaultSecretRef `json:"tokenSecretRef,omitempty"`
	// Role for Kubernetes auth method
	// +optional
	// +nullable
	Role string `json:"role"`

	// Path in Vault where secrets are stored
	// +optional
	// +nullable
	Path string `json:"path,omitempty"`

	// Backend type in Vault (e.g., "kv", "pki")
	// +kubebuilder:default:="kv"
	// +optional
	Backend VaultBackend `json:"backend,omitempty"`

	// Version of KV backend (1 or 2)
	// +kubebuilder:default:=2
	// +optional
	KVVersion int `json:"kvVersion,omitempty"`

	// Path to the secret in Vault
	// +optional
	// +nullable
	SecretIdSecretRef *VaultSecretRef `json:"secretIdSecretRef,omitempty"`
	// Kubernetes service account token path for auth
	// +optional
	// +nullable
	ServiceAccountTokenPath string `json:"serviceAccountTokenPath"`
	// Kubernetes auth mount path
	// +kubebuilder:default:="kubernetes"
	// +optional
	// +nullable
	AuthPath string `json:"authPath"`
	// Server Certificate for TLS authentication
	// +optional
	// +nullable
	ServerCert string `json:"serverCert"`
	// Server Name for TLS authentication
	// +optional
	ServerName string `json:"serverName"`

	// Client certificate for TLS authentication
	// +optional
	ClientCert string `json:"clientCert"`
	// Client key for TLS authentication
	// +optional
	ClientKeySecretRef *VaultSecretRef `json:"clientKey"`
	// CA certificate for TLS verification
	// +optional
	CACert string `json:"caCert"`
	// Skip TLS verification
	// +kubebuilder:default:=false
	TLSSkipVerify bool `json:"tlsSkipVerify"`
	// Timeout for Vault operations
	// +kubebuilder:default:="30s"
	Timeout string `json:"timeout"`
	// Maximum number of retries for Vault operations
	// +kubebuilder:default:=2
	MaxRetries int `json:"maxRetries"`
}

type VaultSecretRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
}
