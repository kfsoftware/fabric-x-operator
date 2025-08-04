package certs

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

// CommitterCertServiceInterface defines the interface for certificate operations
type CommitterCertServiceInterface interface {
	ProvisionComponentCertificates(ctx context.Context, committer *fabricxv1alpha1.Committer, componentName string, componentConfig *fabricxv1alpha1.ComponentConfig) error
	CleanupComponentCertificates(ctx context.Context, committer *fabricxv1alpha1.Committer, componentName string) error
	GetCertificateSecretName(committerName string, componentName string, replicaIndex int, certType string) string
}

// CommitterCertService handles certificate operations for Committer components
type CommitterCertService struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// NewCommitterCertService creates a new CommitterCertService
func NewCommitterCertService(client client.Client, scheme *runtime.Scheme) *CommitterCertService {
	return &CommitterCertService{
		Client: client,
		Scheme: scheme,
	}
}

// ProvisionComponentCertificates provisions certificates for a specific component
func (s *CommitterCertService) ProvisionComponentCertificates(
	ctx context.Context,
	committer *fabricxv1alpha1.Committer,
	componentName string,
	componentConfig *fabricxv1alpha1.ComponentConfig,
) error {
	log := logf.FromContext(ctx)

	// Get the number of replicas for this component
	replicas := componentConfig.Replicas
	if replicas <= 0 {
		replicas = 1 // Default to 1 replica if not specified
	}

	log.Info("Provisioning certificates for component",
		"component", componentName,
		"replicas", replicas)

	// Handle each certificate type separately
	certTypes := []string{"sign", "tls"}

	// Generate certificates for each replica
	for replicaIndex := 0; replicaIndex < int(replicas); replicaIndex++ {
		for _, certType := range certTypes {
			// Check if certificate secret already exists for this replica
			secretName := generateCommitterCertificateSecretName(committer.Name, componentName, certType)
			existingSecret := &corev1.Secret{}
			err := s.Client.Get(ctx, client.ObjectKey{
				Namespace: committer.Namespace,
				Name:      secretName,
			}, existingSecret)

			// If secret exists and has the required data, skip certificate generation
			if err == nil && existingSecret.Data != nil {
				if _, hasCert := existingSecret.Data["cert.pem"]; hasCert {
					if _, hasKey := existingSecret.Data["key.pem"]; hasKey {
						if _, hasCA := existingSecret.Data["ca.pem"]; hasCA {
							log.Info("Certificate secret already exists, skipping generation",
								"secret", secretName, "component", componentName, "replica", replicaIndex, "certType", certType)
							continue
						}
					}
				}
			}

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

			// Create certificate request for this specific type and replica
			request := OrdererGroupCertificateRequest{
				ComponentName:    fmt.Sprintf("%s-%d", componentName, replicaIndex),
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
				return fmt.Errorf("failed to provision %s certificates for %s replica %d: %w", certType, componentName, replicaIndex, err)
			}

			// Create Kubernetes secret for this certificate
			if err := s.createCertificateSecret(ctx, committer, fmt.Sprintf("%s-%d", componentName, replicaIndex), certificates); err != nil {
				return fmt.Errorf("failed to create certificate secret for %s replica %d: %w", componentName, replicaIndex, err)
			}

			log.Info("Successfully provisioned certificates",
				"component", componentName,
				"replica", replicaIndex,
				"certType", certType,
				"secret", secretName)
		}
	}

	return nil
}

// createCertificateSecret creates a Kubernetes secret for certificate data
func (s *CommitterCertService) createCertificateSecret(
	ctx context.Context,
	committer *fabricxv1alpha1.Committer,
	componentName string,
	certificates []ComponentCertificateData,
) error {
	// Process each certificate in the slice
	for _, certData := range certificates {
		secretName := generateCommitterCertificateSecretName(committer.Name, componentName, certData.CertType) // Use 0 as default for backward compatibility
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: committer.Namespace,
				Labels: map[string]string{
					"app":                      "fabric-x",
					"committer":                committer.Name,
					"component":                componentName,
					"certificate-type":         certData.CertType,
					"fabricx.kfsoft.tech/type": "certificate",
				},
			},
			Data: map[string][]byte{
				"cert.pem": certData.Cert,
				"key.pem":  certData.Key,
				"ca.pem":   certData.CACert,
			},
		}

		// Set the controller reference
		if err := controllerutil.SetControllerReference(committer, secret, s.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for secret %s: %w", secretName, err)
		}

		if err := s.Client.Create(ctx, secret); err != nil {
			// If secret already exists, update it
			if strings.Contains(err.Error(), "already exists") {
				if err := s.Client.Update(ctx, secret); err != nil {
					return fmt.Errorf("failed to update certificate secret %s: %w", secretName, err)
				}
			} else {
				return fmt.Errorf("failed to create certificate secret %s: %w", secretName, err)
			}
		}
	}

	return nil
}

// CleanupComponentCertificates removes certificate secrets for a component
// This method is now deprecated as secrets will be automatically cleaned up via owner references
func (s *CommitterCertService) CleanupComponentCertificates(
	ctx context.Context,
	committer *fabricxv1alpha1.Committer,
	componentName string,
) error {
	// Secrets will be automatically deleted by Kubernetes garbage collection
	// due to owner references, so no manual cleanup is needed
	return nil
}

// GetCertificateSecretName returns the name of the certificate secret for a component
func (s *CommitterCertService) GetCertificateSecretName(
	committerName string,
	componentName string,
	replicaIndex int,
	certType string,
) string {
	return generateCommitterCertificateSecretName(committerName, componentName, certType)
}

// generateCommitterCertificateSecretName generates a consistent secret name for committer certificates
func generateCommitterCertificateSecretName(committerName, componentName, certType string) string {
	return fmt.Sprintf("%s-%s-%s-cert", committerName, componentName, certType)
}
