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

// CommitterCertService provides certificate provisioning services for Committer components
type CommitterCertService struct {
	Client client.Client
}

// NewCommitterCertService creates a new certificate service for Committer components
func NewCommitterCertService(client client.Client) *CommitterCertService {
	return &CommitterCertService{
		Client: client,
	}
}

// ProvisionComponentCertificates provisions certificates for a specific component
func (s *CommitterCertService) ProvisionComponentCertificates(
	ctx context.Context,
	committer *fabricxv1alpha1.Committer,
	componentName string,
	componentConfig *fabricxv1alpha1.ComponentConfig,
) error {
	// Handle each certificate type separately
	certTypes := []string{"sign", "tls"}

	for _, certType := range certTypes {
		// Get enrollment parameters based on certificate type
		var enrollID, enrollSecret string

		if committer.Spec.Enrollment != nil {
			if certType == "sign" && committer.Spec.Enrollment.Sign != nil {
				enrollID = committer.Spec.Enrollment.Sign.EnrollID
				enrollSecret = committer.Spec.Enrollment.Sign.EnrollSecret
			} else if certType == "tls" && committer.Spec.Enrollment.TLS != nil {
				enrollID = committer.Spec.Enrollment.TLS.EnrollID
				enrollSecret = committer.Spec.Enrollment.TLS.EnrollSecret
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
			ComponentType:    "committer",
			Namespace:        committer.Namespace,
			OrdererGroupName: committer.Name, // Using OrdererGroupName field for committer name
			CertConfig:       convertToCertConfig(committer.Spec.MSPID, componentConfig.Certificates),
			EnrollmentConfig: convertToEnrollmentConfig(committer.Spec.MSPID, committer.Spec.Enrollment),
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
			if err := s.createCertificateSecret(ctx, committer, componentName, certData); err != nil {
				return fmt.Errorf("failed to create certificate secret for %s %s: %w", componentName, certData.CertType, err)
			}
		}
	}

	return nil
}

// createCertificateSecret creates a Kubernetes secret containing certificate data
func (s *CommitterCertService) createCertificateSecret(
	ctx context.Context,
	committer *fabricxv1alpha1.Committer,
	componentName string,
	certData ComponentCertificateData,
) error {
	secretName := generateCommitterCertificateSecretName(committer.Name, componentName, certData.CertType)

	// Check if secret already exists
	existingSecret := &corev1.Secret{}
	err := s.Client.Get(ctx, client.ObjectKey{
		Namespace: committer.Namespace,
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
			Namespace: committer.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "fabric-x-operator",
				"app.kubernetes.io/component": componentName,
				"app.kubernetes.io/part-of":   "committer",
				"fabric-x/certificate-type":   certData.CertType,
				"fabric-x/committer":          committer.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: committer.APIVersion,
					Kind:       committer.Kind,
					Name:       committer.Name,
					UID:        committer.UID,
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
func (s *CommitterCertService) CleanupComponentCertificates(
	ctx context.Context,
	committer *fabricxv1alpha1.Committer,
	componentName string,
) error {
	certTypes := []string{"sign", "tls"}

	for _, certType := range certTypes {
		secretName := generateCommitterCertificateSecretName(committer.Name, componentName, certType)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: committer.Namespace,
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
func (s *CommitterCertService) GetCertificateSecretName(
	committerName string,
	componentName string,
	certType string,
) string {
	return generateCommitterCertificateSecretName(committerName, componentName, certType)
}

// generateCommitterCertificateSecretName generates a consistent secret name for committer certificates
func generateCommitterCertificateSecretName(committerName, componentName, certType string) string {
	return fmt.Sprintf("%s-%s-%s-cert", committerName, componentName, certType)
}
