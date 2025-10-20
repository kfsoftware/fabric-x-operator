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

const (
	caEnrollmentFinalizer = "finalizer.caenrollment.fabricx.kfsoft.tech"
)

// CAEnrollmentReconciler reconciles a CAEnrollment object
type CAEnrollmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=caenrollments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=caenrollments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=caenrollments/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CAEnrollmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling CAEnrollment", "namespace", req.Namespace, "name", req.Name)

	// Fetch the CAEnrollment instance
	enrollment := &fabricxv1alpha1.CAEnrollment{}
	err := r.Get(ctx, req.NamespacedName, enrollment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("CAEnrollment resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get CAEnrollment.")
		return ctrl.Result{}, fmt.Errorf("failed to get CAEnrollment: %w", err)
	}

	// Set initial status if not set
	if enrollment.Status.Status == "" {
		r.updateStatus(ctx, enrollment, "Pending", "Initializing identity enrollment")
	}

	// Check if the enrollment is marked to be deleted
	if enrollment.GetDeletionTimestamp() != nil {
		return r.handleDeletion(ctx, enrollment)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(enrollment, caEnrollmentFinalizer) {
		controllerutil.AddFinalizer(enrollment, caEnrollmentFinalizer)
		if err := r.Update(ctx, enrollment); err != nil {
			log.Error(err, "Failed to add finalizer")
			r.updateStatus(ctx, enrollment, "Failed", fmt.Sprintf("Failed to add finalizer: %v", err))
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// Check if output secret already exists
	outputSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      enrollment.Spec.OutputSecretName,
		Namespace: enrollment.Namespace,
	}, outputSecret)

	if err == nil {
		// Secret already exists, check if it needs renewal
		if r.needsRenewal(enrollment, outputSecret) {
			log.Info("Certificate needs renewal, re-enrolling")
		} else {
			log.Info("Identity already enrolled and certificate is valid")
			r.updateStatus(ctx, enrollment, "Ready", "Identity enrolled successfully")
			return ctrl.Result{RequeueAfter: 24 * time.Hour}, nil
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get output secret")
		return ctrl.Result{}, fmt.Errorf("failed to get output secret: %w", err)
	}

	// Perform enrollment
	if err := r.enrollIdentity(ctx, enrollment); err != nil {
		errorMsg := fmt.Sprintf("Failed to enroll identity: %v", err)
		log.Error(err, "Failed to enroll identity")
		r.updateStatus(ctx, enrollment, "Failed", errorMsg)
		return ctrl.Result{}, fmt.Errorf("failed to enroll identity: %w", err)
	}

	// Update status to ready
	r.updateStatus(ctx, enrollment, "Ready", "Identity enrolled successfully")

	// Requeue to check certificate expiry
	return ctrl.Result{RequeueAfter: 24 * time.Hour}, nil
}

// enrollIdentity performs the enrollment with Fabric CA and creates the output secret
func (r *CAEnrollmentReconciler) enrollIdentity(ctx context.Context, enrollment *fabricxv1alpha1.CAEnrollment) error {
	log := logf.FromContext(ctx)

	// Get CA service
	ca := &fabricxv1alpha1.CA{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      enrollment.Spec.CAName,
		Namespace: enrollment.Spec.CANamespace,
	}, ca)
	if err != nil {
		return errors.Wrap(err, "failed to get CA resource")
	}

	// Get enrollment secret
	enrollSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      enrollment.Spec.EnrollSecretRef.Name,
		Namespace: enrollment.Namespace,
	}, enrollSecret)
	if err != nil {
		return errors.Wrap(err, "failed to get enrollment secret")
	}

	enrollSecretValue, ok := enrollSecret.Data[enrollment.Spec.EnrollSecretRef.Key]
	if !ok {
		return fmt.Errorf("key %s not found in secret %s", enrollment.Spec.EnrollSecretRef.Key, enrollment.Spec.EnrollSecretRef.Name)
	}

	// Get TLS CA cert from CA
	tlsCACertSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-tlsca-crypto", ca.Name),
		Namespace: ca.Namespace,
	}, tlsCACertSecret)
	if err != nil {
		return errors.Wrap(err, "failed to get TLS CA certificate secret")
	}

	tlsCACert, ok := tlsCACertSecret.Data["certfile"]
	if !ok {
		return fmt.Errorf("certfile not found in TLS CA cert secret")
	}

	// Build CA URL
	caURL := fmt.Sprintf("https://%s.%s:7054", ca.Name, ca.Namespace)
	log.Info("Using CA URL", "url", caURL)

	// Prepare attributes for enrollment
	var attrs []*api.AttributeRequest
	for _, attr := range enrollment.Spec.Attrs {
		attrs = append(attrs, &api.AttributeRequest{
			Name:     attr.Name,
			Optional: !attr.ECert,
		})
	}

	// Perform enrollment using existing helper
	userCert, userKey, rootCert, err := certs.EnrollUser(ctx, certs.EnrollUserRequest{
		TLSCert:    string(tlsCACert),
		URL:        caURL,
		Name:       enrollment.Spec.CAName,
		MSPID:      enrollment.Spec.CAName, // Use CA name as MSPID
		User:       enrollment.Spec.EnrollID,
		Secret:     string(enrollSecretValue),
		Hosts:      []string{},
		CN:         enrollment.Spec.EnrollID,
		Profile:    "",
		Attributes: attrs,
	})
	if err != nil {
		return errors.Wrap(err, "failed to enroll identity with CA")
	}

	log.Info("Successfully enrolled identity", "enrollID", enrollment.Spec.EnrollID)

	// Convert private key to PEM
	keyPEM, err := utils.EncodePrivateKey(userKey)
	if err != nil {
		return errors.Wrap(err, "failed to convert private key to PEM")
	}

	// Convert certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: userCert.Raw,
	})

	// Convert root cert to PEM
	rootCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: rootCert.Raw,
	})

	// Parse certificate to get expiry time
	certExpiry := &userCert.NotAfter

	// Create or update output secret
	outputSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      enrollment.Spec.OutputSecretName,
			Namespace: enrollment.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"key.pem":  keyPEM,
			"cert.pem": certPEM,
			"ca.pem":   rootCertPEM,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(enrollment, outputSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "failed to set controller reference on output secret")
	}

	// Create or update the secret
	existingSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      enrollment.Spec.OutputSecretName,
		Namespace: enrollment.Namespace,
	}, existingSecret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new secret
			if err := r.Create(ctx, outputSecret); err != nil {
				return errors.Wrap(err, "failed to create output secret")
			}
			log.Info("Created output secret", "secret", enrollment.Spec.OutputSecretName)
		} else {
			return errors.Wrap(err, "failed to check if output secret exists")
		}
	} else {
		// Update existing secret
		existingSecret.Data = outputSecret.Data
		if err := r.Update(ctx, existingSecret); err != nil {
			return errors.Wrap(err, "failed to update output secret")
		}
		log.Info("Updated output secret", "secret", enrollment.Spec.OutputSecretName)
	}

	// Update status with enrollment details
	now := metav1.Now()
	enrollment.Status.EnrollmentTime = &now
	enrollment.Status.EnrollmentCount++

	if certExpiry != nil {
		expiryTime := metav1.NewTime(*certExpiry)
		enrollment.Status.CertificateExpiry = &expiryTime
	}

	return nil
}

// getCertificateExpiry parses the certificate and returns its expiry time
func (r *CAEnrollmentReconciler) getCertificateExpiry(certPEM []byte) (*time.Time, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse X.509 certificate")
	}

	return &cert.NotAfter, nil
}

// needsRenewal checks if the certificate needs renewal
func (r *CAEnrollmentReconciler) needsRenewal(enrollment *fabricxv1alpha1.CAEnrollment, secret *corev1.Secret) bool {
	certPEM, ok := secret.Data["cert.pem"]
	if !ok {
		return true
	}

	expiry, err := r.getCertificateExpiry(certPEM)
	if err != nil {
		return true
	}

	// Renew if certificate expires in less than 30 days
	renewalThreshold := time.Now().Add(30 * 24 * time.Hour)
	return expiry.Before(renewalThreshold)
}

// handleDeletion handles the deletion of the CAEnrollment resource
func (r *CAEnrollmentReconciler) handleDeletion(ctx context.Context, enrollment *fabricxv1alpha1.CAEnrollment) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(enrollment, caEnrollmentFinalizer) {
		// Delete output secret
		outputSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      enrollment.Spec.OutputSecretName,
				Namespace: enrollment.Namespace,
			},
		}

		if err := r.Delete(ctx, outputSecret); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete output secret")
			return ctrl.Result{}, fmt.Errorf("failed to delete output secret: %w", err)
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(enrollment, caEnrollmentFinalizer)
		if err := r.Update(ctx, enrollment); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// updateStatus updates the status of the CAEnrollment resource
func (r *CAEnrollmentReconciler) updateStatus(ctx context.Context, enrollment *fabricxv1alpha1.CAEnrollment, status, message string) {
	log := logf.FromContext(ctx)

	enrollment.Status.Status = status
	enrollment.Status.Message = message

	if err := r.Status().Update(ctx, enrollment); err != nil {
		log.Error(err, "Failed to update CAEnrollment status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CAEnrollmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.CAEnrollment{}).
		Owns(&corev1.Secret{}).
		Named("caenrollment").
		Complete(r)
}
