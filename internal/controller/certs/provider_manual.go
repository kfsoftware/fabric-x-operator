package certs

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ManualProvider implements the CertificateProvider interface for manually provided certificates
type ManualProvider struct{}

// NewManualProvider creates a new manual certificate provider
func NewManualProvider() *ManualProvider {
	return &ManualProvider{}
}

// Name returns the provider name
func (p *ManualProvider) Name() string {
	return "manual"
}

// ProvisionSignCertificate retrieves a signing certificate from a Kubernetes secret
func (p *ManualProvider) ProvisionSignCertificate(ctx context.Context, req SignCertificateRequest) (*CertificateData, error) {
	manualConfig, ok := req.Config.(*ManualConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type for Manual provider, expected *ManualConfig")
	}

	if manualConfig.SecretRef == nil {
		return nil, fmt.Errorf("secretRef is required for manual certificate provider")
	}

	return p.getCertificateFromSecret(ctx, req.K8sClient, manualConfig, "sign")
}

// ProvisionTLSCertificate retrieves a TLS certificate from a Kubernetes secret
func (p *ManualProvider) ProvisionTLSCertificate(ctx context.Context, req TLSCertificateRequest) (*CertificateData, error) {
	manualConfig, ok := req.Config.(*ManualConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type for Manual provider, expected *ManualConfig")
	}

	if manualConfig.SecretRef == nil {
		return nil, fmt.Errorf("secretRef is required for manual certificate provider")
	}

	return p.getCertificateFromSecret(ctx, req.K8sClient, manualConfig, "tls")
}

// getCertificateFromSecret retrieves certificate data from a Kubernetes secret
func (p *ManualProvider) getCertificateFromSecret(
	ctx context.Context,
	k8sClient client.Client,
	config *ManualConfig,
	certType string,
) (*CertificateData, error) {
	// Get the secret
	secret := &corev1.Secret{}
	secretNamespace := config.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = "default"
	}

	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      config.SecretRef.Name,
		Namespace: secretNamespace,
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate secret: %w", err)
	}

	// Determine the keys to use
	certKey := config.CertKey
	if certKey == "" {
		certKey = "cert.pem"
	}

	keyKey := config.KeyKey
	if keyKey == "" {
		keyKey = "key.pem"
	}

	caKey := config.CAKey
	if caKey == "" {
		caKey = "ca.pem"
	}

	// Extract the certificate data
	certData := &CertificateData{
		Type: certType,
	}

	// Get certificate
	cert, ok := secret.Data[certKey]
	if !ok {
		return nil, fmt.Errorf("certificate key %q not found in secret %s/%s", certKey, secretNamespace, config.SecretRef.Name)
	}
	certData.Certificate = cert

	// Get private key
	key, ok := secret.Data[keyKey]
	if !ok {
		return nil, fmt.Errorf("private key %q not found in secret %s/%s", keyKey, secretNamespace, config.SecretRef.Name)
	}
	certData.PrivateKey = key

	// Get CA certificate (optional)
	if caCert, ok := secret.Data[caKey]; ok {
		certData.CACertificate = caCert
	}

	return certData, nil
}
