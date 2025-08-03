package certs

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

// OrdererGroupCertService provides certificate provisioning services for OrdererGroup components
type OrdererGroupCertService struct {
	Client client.Client
}

// NewOrdererGroupCertService creates a new certificate service for OrdererGroup components
func NewOrdererGroupCertService(client client.Client) *OrdererGroupCertService {
	return &OrdererGroupCertService{
		Client: client,
	}
}

// ProvisionComponentCertificates provisions certificates for a specific component
func (s *OrdererGroupCertService) ProvisionComponentCertificates(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
	componentConfig *fabricxv1alpha1.ComponentConfig,
) error {
	// Handle each certificate type separately
	certTypes := []string{"sign", "tls"}

	for _, certType := range certTypes {
		// Get enrollment parameters based on certificate type
		var enrollID, enrollSecret string

		if ordererGroup.Spec.Enrollment != nil {
			if certType == "sign" && ordererGroup.Spec.Enrollment.Sign != nil {
				enrollID = ordererGroup.Spec.Enrollment.Sign.EnrollID
				enrollSecret = ordererGroup.Spec.Enrollment.Sign.EnrollSecret
			} else if certType == "tls" && ordererGroup.Spec.Enrollment.TLS != nil {
				enrollID = ordererGroup.Spec.Enrollment.TLS.EnrollID
				enrollSecret = ordererGroup.Spec.Enrollment.TLS.EnrollSecret
			}
		}

		// Fallback to component-specific enrollment if global enrollment is not available
		if enrollID == "" && componentConfig.Certificates != nil {
			enrollID = componentConfig.Certificates.EnrollID
			enrollSecret = componentConfig.Certificates.EnrollSecret
		}

		// Create certificate request for this specific type
		request := OrdererGroupCertificateRequest{
			ComponentName:    componentName,
			ComponentType:    "orderer",
			Namespace:        ordererGroup.Namespace,
			OrdererGroupName: ordererGroup.Name,
			CertConfig:       convertToCertConfig(ordererGroup.Spec.MSPID, componentConfig.Certificates),
			EnrollmentConfig: convertToEnrollmentConfig(ordererGroup.Spec.MSPID, ordererGroup.Spec.Enrollment),
			CertTypes:        []string{certType}, // Only one cert type per request
			EnrollID:         enrollID,
			EnrollSecret:     enrollSecret,
		}

		// Provision certificates with client context
		certificates, err := ProvisionOrdererGroupCertificatesWithClient(ctx, s.Client, request)
		if err != nil {
			return fmt.Errorf("failed to provision %s certificates for component %s: %w", certType, componentName, err)
		}

		// Create Kubernetes secrets for each certificate
		for _, certData := range certificates {
			if err := s.createCertificateSecret(ctx, ordererGroup, componentName, certData); err != nil {
				return fmt.Errorf("failed to create certificate secret for %s %s: %w", componentName, certData.CertType, err)
			}
		}
	}

	return nil
}

// createCertificateSecret creates a Kubernetes secret containing certificate data
func (s *OrdererGroupCertService) createCertificateSecret(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
	certData ComponentCertificateData,
) error {
	secretName := generateCertificateSecretName(ordererGroup.Name, componentName, certData.CertType)

	// Check if secret already exists
	existingSecret := &corev1.Secret{}
	err := s.Client.Get(ctx, client.ObjectKey{
		Namespace: ordererGroup.Namespace,
		Name:      secretName,
	}, existingSecret)

	if err == nil {
		// Secret exists, update it
		existingSecret.Data = map[string][]byte{
			"cert.pem": certData.Cert,
			"key.pem":  certData.Key,
			"ca.pem":   certData.CACert,
		}
		return s.Client.Update(ctx, existingSecret)
	}

	// Create new secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ordererGroup.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "fabric-x-operator",
				"app.kubernetes.io/component": componentName,
				"app.kubernetes.io/part-of":   "orderergroup",
				"fabric-x/certificate-type":   certData.CertType,
				"fabric-x/orderergroup":       ordererGroup.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: ordererGroup.APIVersion,
					Kind:       ordererGroup.Kind,
					Name:       ordererGroup.Name,
					UID:        ordererGroup.UID,
				},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"cert.pem": certData.Cert,
			"key.pem":  certData.Key,
			"ca.pem":   certData.CACert,
		},
	}

	return s.Client.Create(ctx, secret)
}

// CleanupComponentCertificates removes certificate secrets for a component
func (s *OrdererGroupCertService) CleanupComponentCertificates(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
) error {
	certTypes := []string{"sign", "tls"}

	for _, certType := range certTypes {
		secretName := generateCertificateSecretName(ordererGroup.Name, componentName, certType)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: ordererGroup.Namespace,
			},
		}

		if err := s.Client.Delete(ctx, secret); err != nil {
			// Ignore not found errors
			if !strings.Contains(err.Error(), "not found") {
				return fmt.Errorf("failed to delete certificate secret %s: %w", secretName, err)
			}
		}
	}

	return nil
}

// GetCertificateSecretName returns the name of the certificate secret for a component
func (s *OrdererGroupCertService) GetCertificateSecretName(
	ordererGroupName string,
	componentName string,
	certType string,
) string {
	return generateCertificateSecretName(ordererGroupName, componentName, certType)
}

// generateCertificateSecretName generates a consistent secret name for certificates
func generateCertificateSecretName(ordererGroupName, componentName, certType string) string {
	return fmt.Sprintf("%s-%s-%s-cert", ordererGroupName, componentName, certType)
}

// convertToCertConfig converts API certificate config to internal format
func convertToCertConfig(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig) *CertificateConfig {
	if apiConfig == nil {
		return nil
	}

	config := &CertificateConfig{
		CAHost:       apiConfig.CAHost,
		CAName:       apiConfig.CAName,
		CAPort:       apiConfig.CAPort,
		EnrollID:     apiConfig.EnrollID,
		EnrollSecret: apiConfig.EnrollSecret,
		MSPID:        mspID,
		CATLS: &CATLSConfig{
			CACert: apiConfig.CATLS.CACert,
		},
	}

	if apiConfig.CATLS != nil {
		config.CATLS = &CATLSConfig{
			CACert: apiConfig.CATLS.CACert,
		}

		if apiConfig.CATLS.SecretRef != nil {
			config.CATLS.SecretRef = &SecretRef{
				Name:      apiConfig.CATLS.SecretRef.Name,
				Key:       apiConfig.CATLS.SecretRef.Key,
				Namespace: apiConfig.CATLS.SecretRef.Namespace,
			}
		}
	}

	return config
}

// convertToEnrollmentConfig converts API enrollment config to internal format
func convertToEnrollmentConfig(mspID string, apiConfig *fabricxv1alpha1.EnrollmentConfig) *EnrollmentConfig {
	if apiConfig == nil {
		return nil
	}

	config := &EnrollmentConfig{}

	if apiConfig.Sign != nil {
		config.Sign = convertToCertConfig(mspID, apiConfig.Sign)
	}

	if apiConfig.TLS != nil {
		config.TLS = convertToCertConfig(mspID, apiConfig.TLS)
	}

	return config
}
