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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
)

// convertToEnrollmentConfig converts API EnrollmentConfig to certs package EnrollmentConfig
func convertToEnrollmentConfig(mspid string, apiEnrollment *fabricxv1alpha1.EnrollmentConfig) *certs.EnrollmentConfig {
	if apiEnrollment == nil {
		return nil
	}

	return &certs.EnrollmentConfig{
		Sign: convertToCertConfig(mspid, apiEnrollment.Sign, "sign"),
		TLS:  convertToCertConfig(mspid, apiEnrollment.TLS, "tls"),
	}
}

// convertToCertConfig converts API CertificateConfig to certs package CertificateConfig
func convertToCertConfig(mspid string, apiCert *fabricxv1alpha1.CertificateConfig, certType string) *certs.CertificateConfig {
	if apiCert == nil {
		return nil
	}

	certConfig := &certs.CertificateConfig{
		MSPID: mspid,
	}

	// Convert CA configuration
	if apiCert.CA != nil {
		certConfig.CA = &certs.CACertificateConfig{
			CAName:       apiCert.CA.CAName,
			CAHost:       apiCert.CA.CAHost,
			CAPort:       apiCert.CA.CAPort,
			EnrollID:     apiCert.CA.EnrollID,
			EnrollSecret: apiCert.CA.EnrollSecret,
		}

		// Convert CA TLS configuration
		if apiCert.CA.CATLS != nil {
			certConfig.CA.CATLS = &certs.CATLSConfig{
				CACert: apiCert.CA.CATLS.CACert,
			}

			// Convert secret reference
			if apiCert.CA.CATLS.SecretRef != nil {
				certConfig.CA.CATLS.SecretRef = &certs.SecretRef{
					Name:      apiCert.CA.CATLS.SecretRef.Name,
					Key:       apiCert.CA.CATLS.SecretRef.Key,
					Namespace: apiCert.CA.CATLS.SecretRef.Namespace,
				}
			}
		}
	}

	// Convert SANS configuration (for TLS certificates)
	if apiCert.SANS != nil {
		certConfig.SANS = &certs.SANSConfig{}
		if len(apiCert.SANS.DNSNames) > 0 {
			certConfig.SANS.DNSNames = apiCert.SANS.DNSNames
		}
		if len(apiCert.SANS.IPAddresses) > 0 {
			certConfig.SANS.IPAddresses = apiCert.SANS.IPAddresses
		}
	}

	return certConfig
}

// createCertificateSecrets creates individual secrets for each certificate type
func (r *EndorserReconciler) createCertificateSecrets(
	ctx context.Context,
	endorser *fabricxv1alpha1.Endorser,
	certificates []certs.ComponentCertificateData,
) error {
	for _, certData := range certificates {
		// Generate secret name based on cert type
		secretName := fmt.Sprintf("%s-%s-cert", endorser.Name, certData.CertType)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: endorser.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"cert.pem": certData.Cert,
				"key.pem":  certData.Key,
			},
		}

		// Add CA certificate if present
		if len(certData.CACert) > 0 {
			secret.Data["ca.pem"] = certData.CACert
		}

		// Set owner reference
		if err := controllerutil.SetControllerReference(endorser, secret, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for %s secret: %w", certData.CertType, err)
		}

		// Create or update secret
		existingSecret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: endorser.Namespace}, existingSecret)
		if err != nil {
			if errors.IsNotFound(err) {
				if err := r.Create(ctx, secret); err != nil {
					return fmt.Errorf("failed to create %s certificate secret: %w", certData.CertType, err)
				}
			} else {
				return fmt.Errorf("failed to get %s certificate secret: %w", certData.CertType, err)
			}
		} else {
			// Update existing secret
			existingSecret.Data = secret.Data
			if err := r.Update(ctx, existingSecret); err != nil {
				return fmt.Errorf("failed to update %s certificate secret: %w", certData.CertType, err)
			}
		}

		// Update status with certificate secret name
		if endorser.Status.CertificateSecrets == nil {
			endorser.Status.CertificateSecrets = make(map[string]string)
		}
		endorser.Status.CertificateSecrets[certData.CertType] = secretName
	}

	return nil
}
