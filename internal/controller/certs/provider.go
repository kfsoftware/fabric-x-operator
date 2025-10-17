package certs

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CertificateProvider is the interface that all certificate providers must implement
// This allows plugging in different certificate management systems (Fabric CA, AWS KMS, Vault, etc.)
type CertificateProvider interface {
	// ProvisionSignCertificate provisions a signing certificate and key
	// Returns the certificate data or an error
	ProvisionSignCertificate(ctx context.Context, req SignCertificateRequest) (*CertificateData, error)

	// ProvisionTLSCertificate provisions a TLS certificate and key with optional SANs
	// Returns the certificate data or an error
	ProvisionTLSCertificate(ctx context.Context, req TLSCertificateRequest) (*CertificateData, error)

	// Name returns the provider name for logging/debugging
	Name() string
}

// CertificateData represents the result of certificate provisioning
type CertificateData struct {
	// Certificate in PEM format
	Certificate []byte

	// Private key in PEM format
	PrivateKey []byte

	// CA certificate chain in PEM format
	CACertificate []byte

	// Type of certificate (sign, tls)
	Type string
}

// SignCertificateRequest contains parameters for provisioning a signing certificate
type SignCertificateRequest struct {
	// Kubernetes client for accessing secrets
	K8sClient client.Client

	// MSP ID for the organization
	MSPID string

	// Component name (for cert CN)
	ComponentName string

	// Provider-specific configuration (unmarshaled from CertificateConfig)
	Config interface{}
}

// TLSCertificateRequest contains parameters for provisioning a TLS certificate
type TLSCertificateRequest struct {
	// Kubernetes client for accessing secrets
	K8sClient client.Client

	// MSP ID for the organization
	MSPID string

	// Component name (for cert CN)
	ComponentName string

	// DNS names for Subject Alternative Names
	DNSNames []string

	// IP addresses for Subject Alternative Names
	IPAddresses []string

	// Provider-specific configuration (unmarshaled from CertificateConfig)
	Config interface{}
}

// ProviderConfig is embedded in CertificateConfig and specifies which provider to use
type ProviderConfig struct {
	// Type specifies the certificate provider type
	// Valid values: "fabric-ca", "vault", "manual"
	Type string `json:"type"`

	// FabricCA contains Fabric CA specific configuration
	FabricCA *FabricCAConfig `json:"fabricCA,omitempty"`

	// Vault contains Hashicorp Vault specific configuration
	Vault *VaultConfig `json:"vault,omitempty"`

	// Manual contains manual certificate management configuration
	Manual *ManualConfig `json:"manual,omitempty"`
}

// FabricCAConfig contains Fabric CA specific configuration
type FabricCAConfig struct {
	// CA server hostname
	CAHost string `json:"caHost"`

	// CA server port
	CAPort int `json:"caPort"`

	// CA name (for multi-CA servers)
	CAName string `json:"caName,omitempty"`

	// Enrollment ID
	EnrollID string `json:"enrollID"`

	// Enrollment secret
	EnrollSecret string `json:"enrollSecret"`

	// CA TLS certificate configuration
	CATLS *CATLSConfig `json:"caTLS,omitempty"`
}

// VaultConfig contains Hashicorp Vault specific configuration
type VaultConfig struct {
	// Vault server address (e.g., "https://vault.example.com:8200")
	Address string `json:"address"`

	// PKI mount path (e.g., "pki" or "fabric-pki")
	PKIPath string `json:"pkiPath"`

	// Role name for certificate issuance
	Role string `json:"role"`

	// Authentication method: "kubernetes" or "token"
	AuthMethod string `json:"authMethod"`

	// Kubernetes service account for auth (if using kubernetes auth)
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Vault token secret reference (if using token auth)
	TokenSecretRef *SecretRef `json:"tokenSecretRef,omitempty"`

	// Vault namespace (for Vault Enterprise)
	Namespace string `json:"namespace,omitempty"`

	// Certificate TTL (e.g., "8760h" for 1 year)
	TTL string `json:"ttl,omitempty"`
}

// ManualConfig contains configuration for manually provided certificates
type ManualConfig struct {
	// Secret reference containing the certificate
	SecretRef *SecretRef `json:"secretRef"`

	// Key in the secret for the certificate (default: "cert.pem")
	CertKey string `json:"certKey,omitempty"`

	// Key in the secret for the private key (default: "key.pem")
	KeyKey string `json:"keyKey,omitempty"`

	// Key in the secret for the CA certificate (default: "ca.pem")
	CAKey string `json:"caKey,omitempty"`
}
