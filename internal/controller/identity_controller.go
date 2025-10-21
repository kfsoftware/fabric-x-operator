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
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/hyperledger/fabric-ca/api"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
	"github.com/kfsoftware/fabric-x-operator/internal/idemix"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

const identityFinalizer = "identity.fabricx.kfsoft.tech/finalizer"

// IdentityReconciler reconciles an Identity object
type IdentityReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=identities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=identities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=identities/finalizers,verbs=update
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=cas,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *IdentityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling Identity")

	// Fetch the Identity instance
	identity := &fabricxv1alpha1.Identity{}
	if err := r.Get(ctx, req.NamespacedName, identity); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Identity resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Identity")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !identity.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, logger, identity)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(identity, identityFinalizer) {
		controllerutil.AddFinalizer(identity, identityFinalizer)
		if err := r.Update(ctx, identity); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate spec
	if err := r.validateSpec(identity); err != nil {
		return r.updateStatus(ctx, logger, identity, "INVALID", err.Error())
	}

	// Check if identity materials already exist
	if r.outputSecretsExist(ctx, identity) {
		logger.Info("Identity materials already exist, validating")
		if err := r.validateExistingSecrets(ctx, identity); err != nil {
			logger.Error(err, "Existing secrets validation failed")
			return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Secret validation failed: %v", err))
		}
		return r.updateStatus(ctx, logger, identity, "READY", "Identity materials exist and are valid")
	}

	// Create identity materials via enrollment
	if identity.Spec.Enrollment == nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", "Enrollment configuration is required")
	}

	return r.handleEnrollment(ctx, logger, identity)
}

// validateSpec validates the identity specification
func (r *IdentityReconciler) validateSpec(identity *fabricxv1alpha1.Identity) error {
	// Check that Enrollment is specified
	if identity.Spec.Enrollment == nil {
		return errors.New("enrollment configuration is required")
	}

	return nil
}

// handleEnrollment handles enrolling with Fabric CA
func (r *IdentityReconciler) handleEnrollment(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity) (ctrl.Result, error) {
	logger.Info("Enrolling identity with Fabric CA")

	enrollment := identity.Spec.Enrollment

	// Debug: Check idemix field
	if enrollment.Idemix != nil {
		logger.Info("Idemix enrollment requested")
		return r.handleIdemixEnrollment(ctx, logger, identity)
	} else {
		logger.Info("Using X.509 enrollment (idemix is nil)")
	}

	// Get CA resource
	ca := &fabricxv1alpha1.CA{}
	caNamespace := enrollment.CARef.Namespace
	if caNamespace == "" {
		caNamespace = identity.Namespace
	}
	err := r.Get(ctx, types.NamespacedName{
		Name:      enrollment.CARef.Name,
		Namespace: caNamespace,
	}, ca)
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to get CA resource: %v", err))
	}

	// Get enrollment secret
	enrollSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      enrollment.EnrollSecretRef.Name,
		Namespace: identity.Namespace,
	}, enrollSecret)
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to get enrollment secret: %v", err))
	}

	enrollPassword, ok := enrollSecret.Data[enrollment.EnrollSecretRef.Key]
	if !ok {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Key %s not found in secret %s", enrollment.EnrollSecretRef.Key, enrollment.EnrollSecretRef.Name))
	}

	// Get TLS cert (server certificate to trust)
	tlsCertSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-tls-crypto", ca.Name),
		Namespace: ca.Namespace,
	}, tlsCertSecret)
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to get TLS certificate: %v", err))
	}

	tlsCACert, ok := tlsCertSecret.Data["tls.crt"]
	if !ok {
		return r.updateStatus(ctx, logger, identity, "FAILED", "TLS certificate not found in secret")
	}

	// Build CA URL
	caURL := fmt.Sprintf("https://%s.%s:7054", ca.Name, ca.Namespace)
	logger.Info("Using CA URL", "url", caURL)

	// Get the actual CA name from the CA spec
	caName := ca.Spec.CA.Name
	if caName == "" {
		caName = "ca" // Default CA name
	}

	// Prepare attributes for enrollment
	var attrs []*api.AttributeRequest
	for _, attr := range enrollment.Attrs {
		attrs = append(attrs, &api.AttributeRequest{
			Name:     attr.Name,
			Optional: !attr.ECert,
		})
	}

	// Perform sign certificate enrollment
	logger.Info("Enrolling for sign certificate", "enrollID", enrollment.EnrollID, "caName", caName)
	signCert, signKey, signCACert, err := certs.EnrollUser(ctx, certs.EnrollUserRequest{
		TLSCert:    string(tlsCACert),
		URL:        caURL,
		Name:       caName,
		MSPID:      identity.Spec.MspID,
		User:       enrollment.EnrollID,
		Secret:     string(enrollPassword),
		Hosts:      []string{},
		CN:         enrollment.EnrollID,
		Profile:    "",
		Attributes: attrs,
	})
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to enroll for sign certificate: %v", err))
	}

	// Convert to PEM
	signCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: signCert.Raw})
	signKeyPEM, err := utils.EncodePrivateKey(signKey)
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to encode sign private key: %v", err))
	}
	signCACertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: signCACert.Raw})

	// Perform TLS certificate enrollment if requested
	var tlsCertPEM, tlsKeyPEM, tlsCACertPEM []byte
	if enrollment.EnrollTLS {
		logger.Info("Enrolling for TLS certificate", "enrollID", enrollment.EnrollID)

		// Get TLS CA ref if specified
		tlsCAName := ca.Name
		tlsCANamespace := ca.Namespace
		if enrollment.TLSCARef != nil {
			tlsCAName = enrollment.TLSCARef.Name
			if enrollment.TLSCARef.Namespace != "" {
				tlsCANamespace = enrollment.TLSCARef.Namespace
			}
		}

		// Get TLS certificate (server certificate to trust)
		tlsCASecret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{
			Name:      fmt.Sprintf("%s-tls-crypto", tlsCAName),
			Namespace: tlsCANamespace,
		}, tlsCASecret)
		if err != nil {
			return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to get TLS certificate for TLS enrollment: %v", err))
		}

		tlsCAForTLS, ok := tlsCASecret.Data["tls.crt"]
		if !ok {
			return r.updateStatus(ctx, logger, identity, "FAILED", "TLS certificate not found for TLS enrollment")
		}

		tlsCAURL := fmt.Sprintf("https://%s.%s:7054", tlsCAName, tlsCANamespace)

		// Get the actual CA name for TLS enrollment
		tlsCAActualName := "ca" // Use default, or get from TLS CA spec if different
		if enrollment.TLSCARef != nil {
			// If a different TLS CA is specified, get its name
			tlsCA := &fabricxv1alpha1.CA{}
			err = r.Get(ctx, types.NamespacedName{
				Name:      tlsCAName,
				Namespace: tlsCANamespace,
			}, tlsCA)
			if err == nil && tlsCA.Spec.CA.Name != "" {
				tlsCAActualName = tlsCA.Spec.CA.Name
			}
		} else {
			// Use same CA name as sign enrollment
			tlsCAActualName = caName
		}

		tlsCert, tlsKey, tlsCACert, err := certs.EnrollUser(ctx, certs.EnrollUserRequest{
			TLSCert:    string(tlsCAForTLS),
			URL:        tlsCAURL,
			Name:       tlsCAActualName,
			MSPID:      identity.Spec.MspID,
			User:       enrollment.EnrollID,
			Secret:     string(enrollPassword),
			Hosts:      []string{},
			CN:         enrollment.EnrollID,
			Profile:    "tls",
			Attributes: attrs,
		})
		if err != nil {
			return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to enroll for TLS certificate: %v", err))
		}

		tlsCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsCert.Raw})
		tlsKeyPEM, err = utils.EncodePrivateKey(tlsKey)
		if err != nil {
			return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to encode TLS private key: %v", err))
		}
		tlsCACertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsCACert.Raw})
	}

	// Create output secrets
	if err := r.createOutputSecrets(ctx, logger, identity, signCertPEM, signKeyPEM, signCACertPEM, tlsCertPEM, tlsKeyPEM, tlsCACertPEM); err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to create output secrets: %v", err))
	}

	// Update status with enrollment time and certificate expiry
	now := metav1.Now()
	certExpiry := &metav1.Time{Time: signCert.NotAfter}
	var tlsCertExpiry *metav1.Time
	if tlsCertPEM != nil {
		tlsCertExpiry = &metav1.Time{Time: signCert.NotAfter}
	}

	return r.updateStatusWithCerts(ctx, logger, identity, "READY", "Identity enrolled successfully with CA", &now, certExpiry, tlsCertExpiry)
}

// createOutputSecrets creates secrets for the identity materials
func (r *IdentityReconciler) createOutputSecrets(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity, signCert, signKey, signCACert, tlsCert, tlsKey, tlsCACert []byte) error {
	output := identity.Spec.Output
	namespace := output.Namespace
	if namespace == "" {
		namespace = identity.Namespace
	}

	secrets := map[string][]byte{
		fmt.Sprintf("%s-sign-cert", output.SecretPrefix):   signCert,
		fmt.Sprintf("%s-sign-key", output.SecretPrefix):    signKey,
		fmt.Sprintf("%s-sign-cacert", output.SecretPrefix): signCACert,
	}

	if tlsCert != nil {
		secrets[fmt.Sprintf("%s-tls-cert", output.SecretPrefix)] = tlsCert
	}
	if tlsKey != nil {
		secrets[fmt.Sprintf("%s-tls-key", output.SecretPrefix)] = tlsKey
	}
	if tlsCACert != nil {
		secrets[fmt.Sprintf("%s-tls-cacert", output.SecretPrefix)] = tlsCACert
	}

	// Create secrets
	for secretName, data := range secrets {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Labels:    output.Labels,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"cert.pem": data,
			},
		}

		// Set controller reference
		if err := controllerutil.SetControllerReference(identity, secret, r.Scheme); err != nil {
			return errors.Wrapf(err, "failed to set controller reference for secret %s", secretName)
		}

		// Create or update secret
		if err := r.Create(ctx, secret); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logger.Info("Secret already exists, updating", "secret", secretName)
				if err := r.Update(ctx, secret); err != nil {
					return errors.Wrapf(err, "failed to update secret %s", secretName)
				}
			} else {
				return errors.Wrapf(err, "failed to create secret %s", secretName)
			}
		}
		logger.Info("Created secret", "secret", secretName)
	}

	// Create combined sign secret (cert + key + ca in one secret)
	combinedSignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cert", output.SecretPrefix),
			Namespace: namespace,
			Labels:    output.Labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"cert.pem": signCert,
			"key.pem":  signKey,
			"ca.pem":   signCACert,
		},
	}
	if err := controllerutil.SetControllerReference(identity, combinedSignSecret, r.Scheme); err != nil {
		return errors.Wrapf(err, "failed to set controller reference for combined sign secret")
	}
	if err := r.Create(ctx, combinedSignSecret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("Combined sign secret already exists, updating", "secret", combinedSignSecret.Name)
			if err := r.Update(ctx, combinedSignSecret); err != nil {
				return errors.Wrapf(err, "failed to update combined sign secret")
			}
		} else {
			return errors.Wrapf(err, "failed to create combined sign secret")
		}
	}
	logger.Info("Created combined sign secret", "secret", combinedSignSecret.Name)

	// Create combined TLS secret if TLS certs exist
	if tlsCert != nil && tlsKey != nil && tlsCACert != nil {
		combinedTLSSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-tls-combined", output.SecretPrefix),
				Namespace: namespace,
				Labels:    output.Labels,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"cert.pem": tlsCert,
				"key.pem":  tlsKey,
				"ca.pem":   tlsCACert,
			},
		}
		if err := controllerutil.SetControllerReference(identity, combinedTLSSecret, r.Scheme); err != nil {
			return errors.Wrapf(err, "failed to set controller reference for combined TLS secret")
		}
		if err := r.Create(ctx, combinedTLSSecret); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logger.Info("Combined TLS secret already exists, updating", "secret", combinedTLSSecret.Name)
				if err := r.Update(ctx, combinedTLSSecret); err != nil {
					return errors.Wrapf(err, "failed to update combined TLS secret")
				}
			} else {
				return errors.Wrapf(err, "failed to create combined TLS secret")
			}
		}
		logger.Info("Created combined TLS secret", "secret", combinedTLSSecret.Name)
	}

	// Update status with output secrets
	identity.Status.OutputSecrets = fabricxv1alpha1.IdentityOutputSecrets{
		SignCert:   fmt.Sprintf("%s-sign-cert", output.SecretPrefix),
		SignKey:    fmt.Sprintf("%s-sign-key", output.SecretPrefix),
		SignCACert: fmt.Sprintf("%s-sign-cacert", output.SecretPrefix),
	}
	if tlsCert != nil {
		identity.Status.OutputSecrets.TLSCert = fmt.Sprintf("%s-tls-cert", output.SecretPrefix)
	}
	if tlsKey != nil {
		identity.Status.OutputSecrets.TLSKey = fmt.Sprintf("%s-tls-key", output.SecretPrefix)
	}
	if tlsCACert != nil {
		identity.Status.OutputSecrets.TLSCACert = fmt.Sprintf("%s-tls-cacert", output.SecretPrefix)
	}

	return nil
}


// getCertificateExpiry parses a PEM-encoded certificate and returns its expiry time
func (r *IdentityReconciler) getCertificateExpiry(certPEM []byte) (time.Time, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, errors.New("failed to decode PEM certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "failed to parse certificate")
	}

	return cert.NotAfter, nil
}

// outputSecretsExist checks if all output secrets already exist
func (r *IdentityReconciler) outputSecretsExist(ctx context.Context, identity *fabricxv1alpha1.Identity) bool {
	output := identity.Spec.Output
	namespace := output.Namespace
	if namespace == "" {
		namespace = identity.Namespace
	}

	// Check if this is an idemix enrollment
	if identity.Spec.Enrollment != nil && identity.Spec.Enrollment.Idemix != nil && identity.Spec.Enrollment.Idemix.Enabled != nil && *identity.Spec.Enrollment.Idemix.Enabled {
		// For idemix, check for the idemix credential secret
		idemixSecretName := fmt.Sprintf("%s-idemix-cred", output.SecretPrefix)
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: idemixSecretName, Namespace: namespace}, secret); err != nil {
			return false
		}
		return true
	}

	// For X.509, check for the traditional secrets
	secretNames := []string{
		fmt.Sprintf("%s-sign-cert", output.SecretPrefix),
		fmt.Sprintf("%s-sign-key", output.SecretPrefix),
		fmt.Sprintf("%s-sign-cacert", output.SecretPrefix),
	}

	for _, secretName := range secretNames {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
			return false
		}
	}

	return true
}

// validateExistingSecrets validates that existing secrets contain valid data
func (r *IdentityReconciler) validateExistingSecrets(ctx context.Context, identity *fabricxv1alpha1.Identity) error {
	output := identity.Spec.Output
	namespace := output.Namespace
	if namespace == "" {
		namespace = identity.Namespace
	}

	// For idemix, validate the idemix credential secret
	if identity.Spec.Enrollment != nil && identity.Spec.Enrollment.Idemix != nil && identity.Spec.Enrollment.Idemix.Enabled != nil && *identity.Spec.Enrollment.Idemix.Enabled {
		idemixSecretName := fmt.Sprintf("%s-idemix-cred", output.SecretPrefix)
		idemixSecret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: idemixSecretName, Namespace: namespace}, idemixSecret); err != nil {
			return errors.Wrapf(err, "failed to get idemix secret %s", idemixSecretName)
		}

		// Basic validation - check if Cred and Sk fields exist
		if _, ok := idemixSecret.Data["Cred"]; !ok {
			return errors.New("idemix secret missing Cred field")
		}
		if _, ok := idemixSecret.Data["Sk"]; !ok {
			return errors.New("idemix secret missing Sk field")
		}

		return nil
	}

	// For X.509, validate signing cert
	signCertSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-sign-cert", output.SecretPrefix),
		Namespace: namespace,
	}, signCertSecret); err != nil {
		return errors.Wrap(err, "failed to get sign cert secret")
	}

	signCertData, ok := signCertSecret.Data["cert.pem"]
	if !ok || len(signCertData) == 0 {
		return errors.New("sign cert secret missing cert.pem data")
	}

	// Parse to ensure it's valid
	if _, err := r.getCertificateExpiry(signCertData); err != nil {
		return errors.Wrap(err, "invalid sign certificate")
	}

	// Create combined secret if it doesn't exist
	// Fetch individual secrets
	signKeySecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-sign-key", output.SecretPrefix),
		Namespace: namespace,
	}, signKeySecret); err != nil {
		return errors.Wrap(err, "failed to get sign key secret")
	}

	signCACertSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-sign-cacert", output.SecretPrefix),
		Namespace: namespace,
	}, signCACertSecret); err != nil {
		return errors.Wrap(err, "failed to get sign cacert secret")
	}

	// Create combined sign secret
	combinedSecretName := fmt.Sprintf("%s-cert", output.SecretPrefix)
	combinedSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: combinedSecretName, Namespace: namespace}, combinedSecret)
	if err != nil && apierrors.IsNotFound(err) {
		// Create combined secret
		combinedSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      combinedSecretName,
				Namespace: namespace,
				Labels:    output.Labels,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"cert.pem": signCertSecret.Data["cert.pem"],
				"key.pem":  signKeySecret.Data["cert.pem"],
				"ca.pem":   signCACertSecret.Data["cert.pem"],
			},
		}
		if err := controllerutil.SetControllerReference(identity, combinedSecret, r.Scheme); err != nil {
			return errors.Wrapf(err, "failed to set controller reference for combined secret")
		}
		if err := r.Create(ctx, combinedSecret); err != nil {
			return errors.Wrapf(err, "failed to create combined secret %s", combinedSecretName)
		}
	}

	return nil
}

// updateStatus updates the identity status
func (r *IdentityReconciler) updateStatus(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity, status, message string) (ctrl.Result, error) {
	// Refetch the identity to avoid conflict errors
	fresh := &fabricxv1alpha1.Identity{}
	if err := r.Get(ctx, types.NamespacedName{Name: identity.Name, Namespace: identity.Namespace}, fresh); err != nil {
		logger.Error(err, "Failed to refetch Identity for status update")
		return ctrl.Result{}, err
	}

	// Update status on the fresh object
	fresh.Status.Status = status
	fresh.Status.Message = message

	if err := r.Status().Update(ctx, fresh); err != nil {
		logger.Error(err, "Failed to update Identity status")
		return ctrl.Result{}, err
	}

	logger.Info("Updated Identity status", "status", status, "message", message)
	return ctrl.Result{}, nil
}

// updateStatusWithTime updates the identity status including enrollment time
func (r *IdentityReconciler) updateStatusWithTime(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity, status, message string, enrollmentTime *metav1.Time) (ctrl.Result, error) {
	// Refetch the identity to avoid conflict errors
	fresh := &fabricxv1alpha1.Identity{}
	if err := r.Get(ctx, types.NamespacedName{Name: identity.Name, Namespace: identity.Namespace}, fresh); err != nil {
		logger.Error(err, "Failed to refetch Identity for status update")
		return ctrl.Result{}, err
	}

	// Update status on the fresh object
	fresh.Status.Status = status
	fresh.Status.Message = message
	fresh.Status.EnrollmentTime = enrollmentTime

	if err := r.Status().Update(ctx, fresh); err != nil {
		logger.Error(err, "Failed to update Identity status")
		return ctrl.Result{}, err
	}

	logger.Info("Updated Identity status with enrollment time", "status", status, "message", message)
	return ctrl.Result{}, nil
}

// updateStatusWithCerts updates the identity status including enrollment time and certificate expiry
func (r *IdentityReconciler) updateStatusWithCerts(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity, status, message string, enrollmentTime, certExpiry, tlsCertExpiry *metav1.Time) (ctrl.Result, error) {
	// Refetch the identity to avoid conflict errors
	fresh := &fabricxv1alpha1.Identity{}
	if err := r.Get(ctx, types.NamespacedName{Name: identity.Name, Namespace: identity.Namespace}, fresh); err != nil {
		logger.Error(err, "Failed to refetch Identity for status update")
		return ctrl.Result{}, err
	}

	// Update status on the fresh object
	fresh.Status.Status = status
	fresh.Status.Message = message
	fresh.Status.EnrollmentTime = enrollmentTime
	fresh.Status.CertificateExpiry = certExpiry
	fresh.Status.TLSCertificateExpiry = tlsCertExpiry

	if err := r.Status().Update(ctx, fresh); err != nil {
		logger.Error(err, "Failed to update Identity status")
		return ctrl.Result{}, err
	}

	logger.Info("Updated Identity status with certificate expiry", "status", status, "message", message)
	return ctrl.Result{}, nil
}

// handleDeletion handles the deletion of an identity
func (r *IdentityReconciler) handleDeletion(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(identity, identityFinalizer) {
		logger.Info("Cleaning up Identity resources")

		// Secrets will be deleted automatically due to controller reference (ownerReferences)
		// No additional cleanup needed

		// Remove finalizer
		controllerutil.RemoveFinalizer(identity, identityFinalizer)
		if err := r.Update(ctx, identity); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// handleIdemixEnrollment handles idemix enrollment with Fabric CA
func (r *IdentityReconciler) handleIdemixEnrollment(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity) (ctrl.Result, error) {
	logger.Info("Enrolling identity with Fabric CA using idemix")

	enrollment := identity.Spec.Enrollment

	// Determine which CA to use for idemix enrollment
	caRef := enrollment.CARef
	if enrollment.Idemix.CARef != nil {
		caRef = *enrollment.Idemix.CARef
	}

	// Get CA resource
	ca := &fabricxv1alpha1.CA{}
	caNamespace := caRef.Namespace
	if caNamespace == "" {
		caNamespace = identity.Namespace
	}
	err := r.Get(ctx, types.NamespacedName{
		Name:      caRef.Name,
		Namespace: caNamespace,
	}, ca)
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to get CA resource: %v", err))
	}

	// Get enrollment secret
	enrollSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      enrollment.EnrollSecretRef.Name,
		Namespace: identity.Namespace,
	}, enrollSecret)
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to get enrollment secret: %v", err))
	}

	enrollPassword, ok := enrollSecret.Data[enrollment.EnrollSecretRef.Key]
	if !ok {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Key %s not found in secret %s", enrollment.EnrollSecretRef.Key, enrollment.EnrollSecretRef.Name))
	}

	// Get TLS cert (server certificate to trust)
	tlsCertSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-tls-crypto", ca.Name),
		Namespace: ca.Namespace,
	}, tlsCertSecret)
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to get TLS certificate: %v", err))
	}

	tlsCACert, ok := tlsCertSecret.Data["tls.crt"]
	if !ok {
		return r.updateStatus(ctx, logger, identity, "FAILED", "TLS certificate not found in secret")
	}

	// Build CA URL
	caURL := fmt.Sprintf("https://%s.%s:7054", ca.Name, ca.Namespace)
	logger.Info("Using CA URL for idemix enrollment", "url", caURL)

	// Get the actual CA name from the CA spec
	caName := ca.Spec.CA.Name
	if caName == "" {
		caName = "ca" // Default CA name
	}

	// Create temporary MSP directory for enrollment
	tmpMSPDir, err := os.MkdirTemp("", "idemix-msp-*")
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to create temp MSP directory: %v", err))
	}
	defer os.RemoveAll(tmpMSPDir)

	// Save TLS cert to temp file for enrollment
	tlsCertPath := filepath.Join(tmpMSPDir, "ca-cert.pem")
	if err := os.WriteFile(tlsCertPath, tlsCACert, 0644); err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to write TLS cert: %v", err))
	}

	// Perform idemix enrollment
	logger.Info("Performing idemix enrollment", "enrollID", enrollment.EnrollID, "caName", caName)
	enrollReq := idemix.EnrollmentRequest{
		CAURL:        caURL,
		CAName:       caName,
		EnrollID:     enrollment.EnrollID,
		EnrollSecret: string(enrollPassword),
		CACertPath:   tlsCertPath,
		MSPDir:       tmpMSPDir,
	}

	enrollResp, err := idemix.Enroll(enrollReq)
	if err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to enroll with idemix: %v", err))
	}

	logger.Info("Idemix enrollment successful", "enrollmentID", enrollResp.SignerConfig.GetEnrollmentID())

	// Get the CA name for fetching issuer public keys
	caNameForKeys := enrollment.CARef.Name
	if enrollment.Idemix.CARef != nil {
		caNameForKeys = enrollment.Idemix.CARef.Name
	}

	// Create idemix credential secret (will fetch CA issuer keys from CA secret)
	if err := r.createIdemixSecret(ctx, logger, identity, enrollResp, caNameForKeys); err != nil {
		return r.updateStatus(ctx, logger, identity, "FAILED", fmt.Sprintf("Failed to create idemix secret: %v", err))
	}

	// Update status with enrollment time
	now := metav1.Now()
	return r.updateStatusWithTime(ctx, logger, identity, "READY", "Identity enrolled successfully with idemix", &now)
}

// createIdemixSecret creates a secret containing all idemix credential information
// Each field from SignerConfig is stored as a separate key in the Kubernetes secret
func (r *IdentityReconciler) createIdemixSecret(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity, enrollResp *idemix.EnrollmentResponse, caName string) error {
	output := identity.Spec.Output
	namespace := output.Namespace
	if namespace == "" {
		namespace = identity.Namespace
	}

	secretName := fmt.Sprintf("%s-idemix-cred", output.SecretPrefix)

	// Prepare secret data - one key per SignerConfig field
	secretData := make(map[string][]byte)

	// Individual fields from SignerConfig (matching JSON structure)
	secretData["Cred"] = enrollResp.SignerConfig.GetCred()
	secretData["Sk"] = enrollResp.SignerConfig.GetSk()
	secretData["enrollment_id"] = []byte(enrollResp.SignerConfig.GetEnrollmentID())
	secretData["credential_revocation_information"] = enrollResp.SignerConfig.GetCredentialRevocationInformation()
	secretData["curveID"] = []byte(enrollResp.SignerConfig.CurveID)
	secretData["revocation_handle"] = []byte(enrollResp.SignerConfig.RevocationHandle)

	// Optional fields (may be empty)
	if ouIdentifier := enrollResp.SignerConfig.GetOrganizationalUnitIdentifier(); ouIdentifier != "" {
		secretData["organizational_unit_identifier"] = []byte(ouIdentifier)
	}

	// Role as string representation
	secretData["role"] = []byte(fmt.Sprintf("%d", enrollResp.SignerConfig.GetRole()))

	// Add all files from idemix config directory
	files, err := os.ReadDir(enrollResp.IdemixConfigPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read idemix config directory")
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filePath := filepath.Join(enrollResp.IdemixConfigPath, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error(err, "Failed to read idemix config file", "file", file.Name())
			continue
		}
		// Store the original SignerConfig file (replace / with -)
		secretData[fmt.Sprintf("user-%s", file.Name())] = content
	}

	// Fetch CA issuer public keys from CA secret
	logger.Info("Fetching CA issuer keys from CA secret", "caName", caName)
	caSecretName := fmt.Sprintf("%s-idemix-issuer-keys", caName)
	caSecret := &corev1.Secret{}
	// CA is in the same namespace as the identity
	caNamespace := identity.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: caSecretName, Namespace: caNamespace}, caSecret); err != nil {
		logger.Info("Failed to get CA issuer keys secret, skipping CA keys", "secret", caSecretName, "error", err)
	} else {
		logger.Info("Found CA issuer keys secret", "secret", caSecretName)
		// Add IssuerPublicKey and IssuerRevocationPublicKey (the public keys needed by identities)
		if issuerPubKey, ok := caSecret.Data["IssuerPublicKey"]; ok {
			logger.Info("Adding IssuerPublicKey to identity secret", "size", len(issuerPubKey))
			secretData["IssuerPublicKey"] = issuerPubKey
		}
		if revocationPubKey, ok := caSecret.Data["IssuerRevocationPublicKey"]; ok {
			logger.Info("Adding IssuerRevocationPublicKey to identity secret", "size", len(revocationPubKey))
			secretData["IssuerRevocationPublicKey"] = revocationPubKey
		}
	}

	// Create secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    output.Labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(identity, secret, r.Scheme); err != nil {
		return errors.Wrapf(err, "failed to set controller reference for secret %s", secretName)
	}

	// Create or update secret
	if err := r.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("Idemix secret already exists, updating", "secret", secretName)
			if err := r.Update(ctx, secret); err != nil {
				return errors.Wrapf(err, "failed to update secret %s", secretName)
			}
		} else {
			return errors.Wrapf(err, "failed to create secret %s", secretName)
		}
	}
	logger.Info("Created idemix secret", "secret", secretName)

	// Update status with output secret
	identity.Status.OutputSecrets.IdemixCred = secretName

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IdentityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.Identity{}).
		Owns(&corev1.Secret{}).
		Named("identity").
		Complete(r)
}
