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

// OrdererGroupCertServiceInterface defines the interface for certificate provisioning services
type OrdererGroupCertServiceInterface interface {
	ProvisionComponentCertificates(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, componentName string, componentConfig *fabricxv1alpha1.ComponentConfig) error
	CleanupComponentCertificates(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, componentName string) error
	GetCertificateSecretName(ordererGroupName string, componentName string, replicaIndex int, certType string) string
}

// OrdererGroupCertService provides certificate provisioning services for OrdererGroup components
type OrdererGroupCertService struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// NewOrdererGroupCertService creates a new OrdererGroupCertService
func NewOrdererGroupCertService(client client.Client, scheme *runtime.Scheme) *OrdererGroupCertService {
	return &OrdererGroupCertService{
		Client: client,
		Scheme: scheme,
	}
}

// ProvisionComponentCertificates provisions certificates for a specific component
func (s *OrdererGroupCertService) ProvisionComponentCertificates(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
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
			secretName := generateCertificateSecretName(ordererGroup.Name, componentName, certType)
			existingSecret := &corev1.Secret{}
			err := s.Client.Get(ctx, client.ObjectKey{
				Namespace: ordererGroup.Namespace,
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

			// Select the appropriate enrollment configuration based on certType
			var certConfig *fabricxv1alpha1.CertificateConfig
			switch certType {
			case "sign":
				certConfig = componentConfig.Enrollment.Sign
			case "tls":
				certConfig = componentConfig.Enrollment.TLS
			default:
				return fmt.Errorf("unknown certificate type: %s", certType)
			}

			// Create a copy of the cert config to avoid modifying the original
			mergedCertConfig := &fabricxv1alpha1.CertificateConfig{
				CA: certConfig.CA,
			}

			// Use component-specific SANS if available, otherwise use enrollment SANS
			if componentConfig.SANS != nil {
				mergedCertConfig.SANS = componentConfig.SANS
			} else if certConfig.SANS != nil {
				mergedCertConfig.SANS = certConfig.SANS
			}

			// Create certificate request for this specific type and replica
			request := OrdererGroupCertificateRequest{
				ComponentName:    fmt.Sprintf("%s-%d", componentName, replicaIndex),
				ComponentType:    "orderer",
				Namespace:        ordererGroup.Namespace,
				OrdererGroupName: ordererGroup.Name,
				CertConfig:       convertToCertConfig(ordererGroup.Spec.MSPID, mergedCertConfig),
				EnrollmentConfig: convertToEnrollmentConfig(ordererGroup.Spec.MSPID, ordererGroup.Spec.Enrollment),
				CertTypes:        []string{certType}, // Only one cert type per request
			}

			// Provision certificates with client context
			certificates, err := ProvisionOrdererGroupCertificatesWithClient(ctx, s.Client, request)
			if err != nil {
				return fmt.Errorf("failed to provision %s certificates for %s replica %d: %w", certType, componentName, replicaIndex, err)
			}

			// Create Kubernetes secret for this certificate
			if err := s.createCertificateSecret(ctx, ordererGroup, fmt.Sprintf("%s-%d", componentName, replicaIndex), certificates); err != nil {
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
func (s *OrdererGroupCertService) createCertificateSecret(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
	certificates []ComponentCertificateData,
) error {
	// Process each certificate in the slice
	for _, certData := range certificates {
		secretName := generateCertificateSecretName(ordererGroup.Name, componentName, certData.CertType) // Use 0 as default for backward compatibility
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: ordererGroup.Namespace,
				Labels: map[string]string{
					"app":                      "fabric-x",
					"orderergroup":             ordererGroup.Name,
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
		if err := controllerutil.SetControllerReference(ordererGroup, secret, s.Scheme); err != nil {
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
func (s *OrdererGroupCertService) CleanupComponentCertificates(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
) error {
	// Secrets will be automatically deleted by Kubernetes garbage collection
	// due to owner references, so no manual cleanup is needed
	return nil
}

// GetCertificateSecretName returns the name of the certificate secret for a component
func (s *OrdererGroupCertService) GetCertificateSecretName(
	ordererGroupName string,
	componentName string,
	replicaIndex int,
	certType string,
) string {
	return generateCertificateSecretName(ordererGroupName, componentName, certType)
}

// generateCertificateSecretName generates a consistent secret name for certificates
func generateCertificateSecretName(ordererGroupName, componentName, certType string) string {
	// Check if componentName contains an instance index (e.g., "batcher-0", "batcher-1")
	if strings.Contains(componentName, "-") {
		// Extract the base component name and instance index
		parts := strings.SplitN(componentName, "-", 2)
		if len(parts) == 2 {
			baseComponent := parts[0]
			instanceIndex := parts[1]
			return fmt.Sprintf("%s-%s-%s-%s-cert", ordererGroupName, baseComponent, instanceIndex, certType)
		}
	}
	// Fallback to original format for backward compatibility
	return fmt.Sprintf("%s-%s-%s-cert", ordererGroupName, componentName, certType)
}

// convertToCertConfig converts API certificate config to internal format
func convertToCertConfig(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig) *CertificateConfig {
	if apiConfig == nil {
		return nil
	}

	config := &CertificateConfig{
		MSPID: mspID,
	}

	// Add CA configuration if provided
	if apiConfig.CA != nil {
		config.CA = &CACertificateConfig{
			CAHost:       apiConfig.CA.CAHost,
			CAName:       apiConfig.CA.CAName,
			CAPort:       apiConfig.CA.CAPort,
			EnrollID:     apiConfig.CA.EnrollID,
			EnrollSecret: apiConfig.CA.EnrollSecret,
		}

		// Add CATLS configuration if provided
		if apiConfig.CA.CATLS != nil {
			config.CA.CATLS = &CATLSConfig{
				CACert: apiConfig.CA.CATLS.CACert,
			}
			if apiConfig.CA.CATLS.SecretRef != nil {
				config.CA.CATLS.SecretRef = &SecretRef{
					Name:      apiConfig.CA.CATLS.SecretRef.Name,
					Key:       apiConfig.CA.CATLS.SecretRef.Key,
					Namespace: apiConfig.CA.CATLS.SecretRef.Namespace,
				}
			}
		}
	}

	// Add SANS configuration if provided
	if apiConfig.SANS != nil {
		config.SANS = &SANSConfig{
			DNSNames:    apiConfig.SANS.DNSNames,
			IPAddresses: apiConfig.SANS.IPAddresses,
		}
	}

	return config
}

// convertComponentCertConfig converts API component certificate config to internal format
// This function handles ComponentCertificateConfig which only contains SANS
func convertComponentCertConfig(mspID string, apiConfig *fabricxv1alpha1.ComponentCertificateConfig) *CertificateConfig {
	if apiConfig == nil {
		return nil
	}

	config := &CertificateConfig{
		MSPID: mspID,
	}

	// Add SANS configuration if provided
	if apiConfig.SANS != nil {
		config.SANS = &SANSConfig{
			DNSNames:    apiConfig.SANS.DNSNames,
			IPAddresses: apiConfig.SANS.IPAddresses,
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
