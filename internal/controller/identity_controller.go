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
	"time"

	"github.com/go-logr/logr"
	"github.com/hyperledger/fabric-ca/api"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
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

	// Update status
	now := metav1.Now()
	identity.Status.EnrollmentTime = &now
	identity.Status.CertificateExpiry = &metav1.Time{Time: signCert.NotAfter}
	if tlsCertPEM != nil {
		identity.Status.TLSCertificateExpiry = &metav1.Time{Time: signCert.NotAfter}
	}

	return r.updateStatus(ctx, logger, identity, "READY", "Identity enrolled successfully with CA")
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

	// Validate signing cert
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

	return nil
}

// updateStatus updates the identity status
func (r *IdentityReconciler) updateStatus(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity, status, message string) (ctrl.Result, error) {
	identity.Status.Status = status
	identity.Status.Message = message

	if err := r.Status().Update(ctx, identity); err != nil {
		logger.Error(err, "Failed to update Identity status")
		return ctrl.Result{}, err
	}

	logger.Info("Updated Identity status", "status", status, "message", message)
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

// SetupWithManager sets up the controller with the Manager.
func (r *IdentityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.Identity{}).
		Owns(&corev1.Secret{}).
		Named("identity").
		Complete(r)
}
