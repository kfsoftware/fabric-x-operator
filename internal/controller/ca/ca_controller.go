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

package ca

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

// CAReconciler reconciles a CA object
type CAReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=cas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=cas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=cas/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch

const (
	caFinalizer = "finalizer.ca.fabricx.kfsoft.tech"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling CA", "namespace", req.Namespace, "name", req.Name)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in CA reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the CA status to failed
			ca := &fabricxv1alpha1.CA{}
			if err := r.Get(ctx, req.NamespacedName, ca); err == nil {
				panicMsg := fmt.Sprintf("Panic in CA reconciliation: %v", panicErr)
				r.updateCAStatus(ctx, ca, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	// Fetch the CA instance
	ca := &fabricxv1alpha1.CA{}
	err := r.Get(ctx, req.NamespacedName, ca)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			log.Info("CA resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get CA.")
		return ctrl.Result{}, fmt.Errorf("failed to get CA: %w", err)
	}

	// Set initial status if not set
	if ca.Status.Status == "" {
		r.updateCAStatus(ctx, ca, fabricxv1alpha1.PendingStatus, "Initializing CA")
	}

	// Check if the CA instance is marked to be deleted
	if ca.GetDeletionTimestamp() != nil {
		return r.handleDeletion(ctx, ca)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(ca, caFinalizer) {
		controllerutil.AddFinalizer(ca, caFinalizer)
		if err := r.Update(ctx, ca); err != nil {
			errorMsg := fmt.Sprintf("Failed to add finalizer: %v", err)
			log.Error(err, "Failed to add finalizer")
			r.updateCAStatus(ctx, ca, fabricxv1alpha1.FailedStatus, errorMsg)
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// Set defaults
	r.setDefaults(ca)

	// Reconcile the CA
	if err := r.reconcileCA(ctx, ca); err != nil {
		errorMsg := fmt.Sprintf("Failed to reconcile CA: %v", err)
		log.Error(err, "Failed to reconcile CA")
		r.updateCAStatus(ctx, ca, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, fmt.Errorf("failed to reconcile CA: %w", err)
	}

	// Update status to success
	r.updateCAStatus(ctx, ca, fabricxv1alpha1.RunningStatus, "CA reconciled successfully")

	return ctrl.Result{}, nil
}

// setDefaults sets default values for the CA spec
func (r *CAReconciler) setDefaults(ca *fabricxv1alpha1.CA) {
	// Defensive nil checks for nested structs to avoid panics
	if ca == nil {
		return
	}
	// Top-level
	if ca.Spec.Replicas == nil {
		replicas := int32(1)
		ca.Spec.Replicas = &replicas
	}
	if ca.Spec.Image == "" {
		ca.Spec.Image = "hyperledger/fabric-ca:1.5.15"
	}
	if ca.Spec.Version == "" {
		ca.Spec.Version = "1.5.15"
	}
	if ca.Spec.CredentialStore == "" {
		ca.Spec.CredentialStore = fabricxv1alpha1.CredentialStoreKubernetes
	}
	if ca.Spec.Service.ServiceType == "" {
		ca.Spec.Service.ServiceType = corev1.ServiceTypeClusterIP
	}
	if ca.Spec.Storage.AccessMode == "" {
		ca.Spec.Storage.AccessMode = "ReadWriteOnce"
	}
	if ca.Spec.Storage.Size == "" {
		ca.Spec.Storage.Size = "1Gi"
	}
	if ca.Spec.CLRSizeLimit == 0 {
		ca.Spec.CLRSizeLimit = 512000
	}
	if ca.Spec.Database.Type == "" {
		ca.Spec.Database.Type = "sqlite3"
	}
	if ca.Spec.Database.Datasource == "" {
		ca.Spec.Database.Datasource = "/var/hyperledger/fabric-ca/fabric-ca-server.db"
	}
	if ca.Spec.Metrics.Provider == "" {
		ca.Spec.Metrics.Provider = "prometheus"
	}
	if ca.Spec.Metrics.Statsd.Network == "" {
		ca.Spec.Metrics.Statsd.Network = "udp"
	}
	if ca.Spec.Metrics.Statsd.Address == "" {
		ca.Spec.Metrics.Statsd.Address = "127.0.0.1:8125"
	}
	if ca.Spec.Metrics.Statsd.WriteInterval == "" {
		ca.Spec.Metrics.Statsd.WriteInterval = "10s"
	}
	if ca.Spec.Metrics.Statsd.Prefix == "" {
		ca.Spec.Metrics.Statsd.Prefix = "fabric-ca"
	}

	// Defensive nil checks for CA
	if ca.Spec.CA.Name == "" {
		ca.Spec.CA.Name = "ca"
	}
	if ca.Spec.CA.CRL.Expiry == "" {
		ca.Spec.CA.CRL.Expiry = "24h"
	}
	if ca.Spec.CA.Registry.MaxEnrollments == 0 {
		ca.Spec.CA.Registry.MaxEnrollments = -1
	}
	if ca.Spec.CA.CSR.CN == "" {
		ca.Spec.CA.CSR.CN = "ca"
	}
	if ca.Spec.CA.CSR.Names == nil {
		ca.Spec.CA.CSR.Names = []fabricxv1alpha1.FabricCANames{}
	}
	if len(ca.Spec.CA.CSR.Names) == 0 {
		ca.Spec.CA.CSR.Names = []fabricxv1alpha1.FabricCANames{
			{
				C:  "US",
				ST: "North Carolina",
				L:  "",
				O:  "Hyperledger",
				OU: "Fabric",
			},
		}
	}
	if ca.Spec.CA.CSR.CA.Expiry == "" {
		ca.Spec.CA.CSR.CA.Expiry = "131400h"
	}
	if ca.Spec.CA.BCCSP.Default == "" {
		ca.Spec.CA.BCCSP.Default = "SW"
	}
	if ca.Spec.CA.BCCSP.SW.Hash == "" {
		ca.Spec.CA.BCCSP.SW.Hash = "SHA2"
	}
	if ca.Spec.CA.BCCSP.SW.Security == 0 {
		ca.Spec.CA.BCCSP.SW.Security = 256
	}

	// Defensive nil checks for TLSCA
	if ca.Spec.TLSCA.Name == "" {
		ca.Spec.TLSCA.Name = "tlsca"
	}
	if ca.Spec.TLSCA.CRL.Expiry == "" {
		ca.Spec.TLSCA.CRL.Expiry = "24h"
	}
	if ca.Spec.TLSCA.Registry.MaxEnrollments == 0 {
		ca.Spec.TLSCA.Registry.MaxEnrollments = -1
	}
	if ca.Spec.TLSCA.CSR.CN == "" {
		ca.Spec.TLSCA.CSR.CN = "tlsca"
	}
	if ca.Spec.TLSCA.CSR.Names == nil {
		ca.Spec.TLSCA.CSR.Names = []fabricxv1alpha1.FabricCANames{}
	}
	if len(ca.Spec.TLSCA.CSR.Names) == 0 {
		ca.Spec.TLSCA.CSR.Names = []fabricxv1alpha1.FabricCANames{
			{
				C:  "US",
				ST: "North Carolina",
				L:  "",
				O:  "Hyperledger",
				OU: "Fabric",
			},
		}
	}
	if ca.Spec.TLSCA.CSR.CA.Expiry == "" {
		ca.Spec.TLSCA.CSR.CA.Expiry = "131400h"
	}
	if ca.Spec.TLSCA.BCCSP.Default == "" {
		ca.Spec.TLSCA.BCCSP.Default = "SW"
	}
	if ca.Spec.TLSCA.BCCSP.SW.Hash == "" {
		ca.Spec.TLSCA.BCCSP.SW.Hash = "SHA2"
	}
	if ca.Spec.TLSCA.BCCSP.SW.Security == 0 {
		ca.Spec.TLSCA.BCCSP.SW.Security = 256
	}
}

// handleDeletion handles the deletion of CA resources
func (r *CAReconciler) handleDeletion(ctx context.Context, ca *fabricxv1alpha1.CA) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in CA deletion",
				"ca", ca.Name, "namespace", ca.Namespace)

			// Update the CA status to failed
			panicMsg := fmt.Sprintf("Panic in CA deletion: %v", panicErr)
			r.updateCAStatus(ctx, ca, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Handling CA deletion", "ca", ca.Name, "namespace", ca.Namespace)

	// Set status to indicate deletion
	r.updateCAStatus(ctx, ca, fabricxv1alpha1.PendingStatus, "Deleting CA resources")

	if controllerutil.ContainsFinalizer(ca, caFinalizer) {
		// Delete all resources
		if err := r.deleteResources(ctx, ca); err != nil {
			errorMsg := fmt.Sprintf("Failed to delete CA resources: %v", err)
			log.Error(err, "Failed to delete CA resources")
			r.updateCAStatus(ctx, ca, fabricxv1alpha1.FailedStatus, errorMsg)
			return ctrl.Result{}, fmt.Errorf("failed to delete CA resources: %w", err)
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(ca, caFinalizer)
		if err := r.Update(ctx, ca); err != nil {
			errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
			log.Error(err, "Failed to remove finalizer")
			r.updateCAStatus(ctx, ca, fabricxv1alpha1.FailedStatus, errorMsg)
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	log.Info("CA deletion completed successfully", "ca", ca.Name)
	return ctrl.Result{}, nil
}

// reconcileCA reconciles all CA resources
func (r *CAReconciler) reconcileCA(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in CA reconciliation",
				"ca", ca.Name, "namespace", ca.Namespace)

			// Update the CA status to failed
			panicMsg := fmt.Sprintf("Panic in CA reconciliation: %v", panicErr)
			r.updateCAStatus(ctx, ca, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	var reconciliationErrors []string

	// Reconcile ConfigMaps first (deployment needs config)
	if err := r.reconcileConfigMaps(ctx, ca); err != nil {
		errorMsg := fmt.Sprintf("ConfigMaps reconciliation failed: %v", err)
		log.Error(err, "Failed to reconcile ConfigMaps")
		reconciliationErrors = append(reconciliationErrors, errorMsg)
	}

	// Reconcile Secrets (deployment needs TLS and MSP secrets)
	if err := r.reconcileSecrets(ctx, ca); err != nil {
		errorMsg := fmt.Sprintf("Secrets reconciliation failed: %v", err)
		log.Error(err, "Failed to reconcile Secrets")
		reconciliationErrors = append(reconciliationErrors, errorMsg)
	}

	// Reconcile PVC (deployment needs persistent storage)
	if err := r.reconcilePVC(ctx, ca); err != nil {
		errorMsg := fmt.Sprintf("PVC reconciliation failed: %v", err)
		log.Error(err, "Failed to reconcile PVC")
		reconciliationErrors = append(reconciliationErrors, errorMsg)
	}

	// Reconcile Service (deployment can reference service)
	if err := r.reconcileService(ctx, ca); err != nil {
		errorMsg := fmt.Sprintf("Service reconciliation failed: %v", err)
		log.Error(err, "Failed to reconcile Service")
		reconciliationErrors = append(reconciliationErrors, errorMsg)
	}

	// Reconcile Ingress (if enabled)
	if ca.Spec.Ingress != nil && ca.Spec.Ingress.Enabled {
		if err := r.reconcileIngress(ctx, ca); err != nil {
			errorMsg := fmt.Sprintf("Ingress reconciliation failed: %v", err)
			log.Error(err, "Failed to reconcile Ingress")
			reconciliationErrors = append(reconciliationErrors, errorMsg)
		}
	}

	// Reconcile Deployment last (depends on all other resources)
	if err := r.reconcileDeployment(ctx, ca); err != nil {
		errorMsg := fmt.Sprintf("Deployment reconciliation failed: %v", err)
		log.Error(err, "Failed to reconcile Deployment")
		reconciliationErrors = append(reconciliationErrors, errorMsg)
	}

	// Reconcile idemix keys secret (if idemix is enabled)
	// TODO: Replace Job-based extraction with direct pod exec from controller using REST client
	if ca.Spec.Idemix != nil && ca.Spec.Idemix.Curve != "" {
		if err := r.reconcileIdemixKeysJob(ctx, ca); err != nil {
			errorMsg := fmt.Sprintf("Idemix keys Job reconciliation failed: %v", err)
			log.Error(err, "Failed to reconcile idemix keys Job")
			reconciliationErrors = append(reconciliationErrors, errorMsg)
		}
	}

	// If there were any errors, update the status to Failed
	if len(reconciliationErrors) > 0 {
		combinedErrorMsg := strings.Join(reconciliationErrors, "; ")
		log.Error(fmt.Errorf("%s", combinedErrorMsg), "CA reconciliation failed")
		return fmt.Errorf("reconciliation failed")
	}

	// Clear any previous error status
	ca.Status.Status = fabricxv1alpha1.PendingStatus
	ca.Status.Message = ""

	return nil
}

// reconcileConfigMaps reconciles CA ConfigMaps
func (r *CAReconciler) reconcileConfigMaps(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	// Main CA config
	caConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	caConfigTemplate := r.getConfigMapTemplate(ca, fmt.Sprintf("%s-config", ca.Name), map[string]string{
		"ca.yaml": r.generateCAConfig(ctx, ca),
	})
	if err := r.updateConfigMap(ctx, ca, caConfig, caConfigTemplate); err != nil {
		log.Error(err, "Failed to update CA ConfigMap", "name", caConfig.Name)
		return fmt.Errorf("failed to update CA ConfigMap %s: %w", caConfig.Name, err)
	}

	// TLS CA config
	tlsConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config-tls", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	tlsConfigTemplate := r.getConfigMapTemplate(ca, fmt.Sprintf("%s-config-tls", ca.Name), map[string]string{
		"fabric-ca-server-config.yaml": r.generateTLSCAConfig(ctx, ca),
	})
	if err := r.updateConfigMap(ctx, ca, tlsConfig, tlsConfigTemplate); err != nil {
		log.Error(err, "Failed to update TLS CA ConfigMap", "name", tlsConfig.Name)
		return fmt.Errorf("failed to update TLS CA ConfigMap %s: %w", tlsConfig.Name, err)
	}

	// Environment config
	envConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-env", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	envConfigTemplate := r.getConfigMapTemplate(ca, fmt.Sprintf("%s-env", ca.Name), map[string]string{
		"GODEBUG":        "netdns=go",
		"FABRIC_CA_HOME": "/var/hyperledger/fabric-ca",
		"SERVICE_DNS":    "0.0.0.0",
	})
	if err := r.updateConfigMap(ctx, ca, envConfig, envConfigTemplate); err != nil {
		log.Error(err, "Failed to update environment ConfigMap", "name", envConfig.Name)
		return fmt.Errorf("failed to update environment ConfigMap %s: %w", envConfig.Name, err)
	}

	log.Info("ConfigMaps reconciled successfully", "ca", ca.Name)
	return nil
}

// reconcileSecrets reconciles CA Secrets
func (r *CAReconciler) reconcileSecrets(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	// Check if we need to regenerate certificates
	shouldRegenerate := r.shouldRegenerateCertificates(ctx, ca)

	var tlsCert, tlsKey, caCert, caKey, tlscaCert, tlscaKey []byte
	var err error

	if shouldRegenerate {
		log.Info("Regenerating certificates", "ca", ca.Name)

		// Generate TLS certificates for server TLS
		tlsCert, tlsKey, err = r.generateTLSCertificate(ca)
		if err != nil {
			log.Error(err, "Failed to generate TLS certificate", "ca", ca.Name)
			return fmt.Errorf("failed to generate TLS certificate: %w", err)
		}

		// Generate CA certificates for signing
		caCert, caKey, err = r.generateCACertificate(ca)
		if err != nil {
			log.Error(err, "Failed to generate CA certificate", "ca", ca.Name)
			return fmt.Errorf("failed to generate CA certificate: %w", err)
		}

		// Generate TLS CA certificates for TLS CA
		tlscaCert, tlscaKey, err = r.generateTLSCACertificate(ca)
		if err != nil {
			log.Error(err, "Failed to generate TLS CA certificate", "ca", ca.Name)
			return fmt.Errorf("failed to generate TLS CA certificate: %w", err)
		}
	} else {
		log.Info("Using existing certificates", "ca", ca.Name)

		// Use existing certificates if available
		tlsCert, tlsKey, caCert, caKey, tlscaCert, tlscaKey, err = r.getExistingCertificates(ctx, ca)
		if err != nil {
			log.Error(err, "Failed to get existing certificates, regenerating", "ca", ca.Name)

			// If we can't get existing certificates, regenerate them
			tlsCert, tlsKey, err = r.generateTLSCertificate(ca)
			if err != nil {
				log.Error(err, "Failed to generate TLS certificate", "ca", ca.Name)
				return fmt.Errorf("failed to generate TLS certificate: %w", err)
			}
			caCert, caKey, err = r.generateCACertificate(ca)
			if err != nil {
				log.Error(err, "Failed to generate CA certificate", "ca", ca.Name)
				return fmt.Errorf("failed to generate CA certificate: %w", err)
			}
			tlscaCert, tlscaKey, err = r.generateTLSCACertificate(ca)
			if err != nil {
				log.Error(err, "Failed to generate TLS CA certificate", "ca", ca.Name)
				return fmt.Errorf("failed to generate TLS CA certificate: %w", err)
			}
		}
	}

	// TLS crypto material secret (for server TLS)
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-tls-crypto", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	tlsSecretTemplate := r.getSecretTemplate(ca, fmt.Sprintf("%s-tls-crypto", ca.Name), map[string][]byte{
		"tls.crt": tlsCert,
		"tls.key": tlsKey,
	})
	if err := r.updateSecret(ctx, ca, tlsSecret, tlsSecretTemplate); err != nil {
		log.Error(err, "Failed to update TLS crypto secret", "name", tlsSecret.Name)
		return fmt.Errorf("failed to update TLS crypto secret %s: %w", tlsSecret.Name, err)
	}

	// MSP crypto material secret (for CA signing)
	mspSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-msp-crypto", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	mspSecretTemplate := r.getSecretTemplate(ca, fmt.Sprintf("%s-msp-crypto", ca.Name), map[string][]byte{
		"certfile": caCert,
		"keyfile":  caKey,
	})
	if err := r.updateSecret(ctx, ca, mspSecret, mspSecretTemplate); err != nil {
		log.Error(err, "Failed to update MSP crypto secret", "name", mspSecret.Name)
		return fmt.Errorf("failed to update MSP crypto secret %s: %w", mspSecret.Name, err)
	}

	// TLS CA crypto material secret (for TLS CA)
	tlscaSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-tlsca-crypto", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	tlscaSecretTemplate := r.getSecretTemplate(ca, fmt.Sprintf("%s-tlsca-crypto", ca.Name), map[string][]byte{
		"certfile": tlscaCert,
		"keyfile":  tlscaKey,
	})
	if err := r.updateSecret(ctx, ca, tlscaSecret, tlscaSecretTemplate); err != nil {
		log.Error(err, "Failed to update TLS CA crypto secret", "name", tlscaSecret.Name)
		return fmt.Errorf("failed to update TLS CA crypto secret %s: %w", tlscaSecret.Name, err)
	}

	log.Info("Secrets reconciled successfully", "ca", ca.Name)
	return nil
}

// shouldRegenerateCertificates determines if certificates should be regenerated
func (r *CAReconciler) shouldRegenerateCertificates(ctx context.Context, ca *fabricxv1alpha1.CA) bool {
	// Check if certificates exist
	tlsSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-tls-crypto", ca.Name),
		Namespace: ca.Namespace,
	}, tlsSecret)

	if err != nil || tlsSecret.Data == nil {
		// Certificates don't exist, need to generate
		return true
	}

	// Check if CA spec has changed in a way that requires new certificates
	// This is a simplified check - in a real implementation you might want to hash the relevant spec fields
	return false
}

// getExistingCertificates retrieves existing certificates from secrets
func (r *CAReconciler) getExistingCertificates(ctx context.Context, ca *fabricxv1alpha1.CA) ([]byte, []byte, []byte, []byte, []byte, []byte, error) {
	// Get TLS certificates
	tlsSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-tls-crypto", ca.Name),
		Namespace: ca.Namespace,
	}, tlsSecret)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to get TLS secret %s: %w", fmt.Sprintf("%s-tls-crypto", ca.Name), err)
	}

	// Get MSP certificates
	mspSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-msp-crypto", ca.Name),
		Namespace: ca.Namespace,
	}, mspSecret)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to get MSP secret %s: %w", fmt.Sprintf("%s-msp-crypto", ca.Name), err)
	}

	// Get TLS CA certificates
	tlscaSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-tlsca-crypto", ca.Name),
		Namespace: ca.Namespace,
	}, tlscaSecret)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to get TLS CA secret %s: %w", fmt.Sprintf("%s-tlsca-crypto", ca.Name), err)
	}

	tlsCert := tlsSecret.Data["tls.crt"]
	tlsKey := tlsSecret.Data["tls.key"]
	caCert := mspSecret.Data["certfile"]
	caKey := mspSecret.Data["keyfile"]
	tlscaCert := tlscaSecret.Data["certfile"]
	tlscaKey := tlscaSecret.Data["keyfile"]

	if tlsCert == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("TLS certificate not found in secret %s", fmt.Sprintf("%s-tls-crypto", ca.Name))
	}
	if tlsKey == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("TLS private key not found in secret %s", fmt.Sprintf("%s-tls-crypto", ca.Name))
	}
	if caCert == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("CA certificate not found in secret %s", fmt.Sprintf("%s-msp-crypto", ca.Name))
	}
	if caKey == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("CA private key not found in secret %s", fmt.Sprintf("%s-msp-crypto", ca.Name))
	}
	if tlscaCert == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("TLS CA certificate not found in secret %s", fmt.Sprintf("%s-tlsca-crypto", ca.Name))
	}
	if tlscaKey == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("TLS CA private key not found in secret %s", fmt.Sprintf("%s-tlsca-crypto", ca.Name))
	}

	return tlsCert, tlsKey, caCert, caKey, tlscaCert, tlscaKey, nil
}

// reconcilePVC reconciles the PersistentVolumeClaim
func (r *CAReconciler) reconcilePVC(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	template := r.getPVCTemplate(ca)

	// Log the template size for debugging
	if len(template.Spec.Resources.Requests) > 0 {
		log.Info("PVC template size", "size", template.Spec.Resources.Requests[corev1.ResourceStorage])
	}

	if err := r.updatePVC(ctx, ca, pvc, template); err != nil {
		log.Error(err, "Failed to update PVC", "name", pvc.Name)
		return fmt.Errorf("failed to update PVC %s: %w", pvc.Name, err)
	}

	log.Info("PVC reconciled successfully", "ca", ca.Name)
	return nil
}

// reconcileDeployment reconciles the CA Deployment
func (r *CAReconciler) reconcileDeployment(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	template := r.GetDeploymentTemplate(ctx, ca)
	if err := r.updateDeployment(ctx, ca, deployment, template); err != nil {
		log.Error(err, "Failed to update Deployment", "name", deployment.Name)
		return fmt.Errorf("failed to update Deployment %s: %w", deployment.Name, err)
	}

	log.Info("Deployment reconciled successfully", "ca", ca.Name)
	return nil
}

// reconcileService reconciles the CA Service
func (r *CAReconciler) reconcileService(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	template := r.getServiceTemplate(ca)
	if err := r.updateService(ctx, ca, service, template); err != nil {
		log.Error(err, "Failed to update Service", "name", service.Name)
		return fmt.Errorf("failed to update Service %s: %w", service.Name, err)
	}

	log.Info("Service reconciled successfully", "ca", ca.Name)
	return nil
}

// reconcileIngress reconciles the CA Ingress
func (r *CAReconciler) reconcileIngress(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	template := r.getIngressTemplate(ca)
	if err := r.updateIngress(ctx, ca, ingress, template); err != nil {
		log.Error(err, "Failed to update Ingress", "name", ingress.Name)
		return fmt.Errorf("failed to update Ingress %s: %w", ingress.Name, err)
	}

	log.Info("Ingress reconciled successfully", "ca", ca.Name)
	return nil
}

// generateCAConfig generates the CA configuration using Go templates
func (r *CAReconciler) generateCAConfig(ctx context.Context, ca *fabricxv1alpha1.CA) string {
	// Ensure defaults are set before generating config
	r.setDefaults(ca)

	// Prepare data for template
	data := ConfigData{
		Debug:        ca.Spec.Debug,
		CLRSizeLimit: ca.Spec.CLRSizeLimit,
		Database: struct {
			Type       string
			Datasource string
		}{
			Type:       ca.Spec.Database.Type,
			Datasource: ca.Spec.Database.Datasource,
		},
		Metrics: struct {
			Provider string
			Statsd   struct {
				Network       string
				Address       string
				WriteInterval string
				Prefix        string
			}
		}{
			Provider: ca.Spec.Metrics.Provider,
			Statsd: struct {
				Network       string
				Address       string
				WriteInterval string
				Prefix        string
			}{
				Network:       ca.Spec.Metrics.Statsd.Network,
				Address:       ca.Spec.Metrics.Statsd.Address,
				WriteInterval: ca.Spec.Metrics.Statsd.WriteInterval,
				Prefix:        ca.Spec.Metrics.Statsd.Prefix,
			},
		},
		CA:    ca.Spec.CA,
		TLSCA: ca.Spec.TLSCA,
	}

	config, err := GenerateConfigFromTemplate(CAConfigTemplate, data)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to generate CA configuration")
		// Fallback to a simple configuration if template fails
		return fmt.Sprintf(`version: "1.4.9"
port: 7054
debug: %t
tls:
  enabled: true
  certfile: /var/hyperledger/tls/secret/tls.crt
  keyfile: /var/hyperledger/tls/secret/tls.key
  clientauth:
    type: noclientcert
ca:
  name: %s
  keyfile: /var/hyperledger/fabric-ca/msp-secret/keyfile
  certfile: /var/hyperledger/fabric-ca/msp-secret/certfile
db:
  type: %s
  datasource: %s`, ca.Spec.Debug, ca.Spec.CA.Name, ca.Spec.Database.Type, ca.Spec.Database.Datasource)
	}

	return config
}

// generateTLSCAConfig generates the TLS CA configuration using Go templates
func (r *CAReconciler) generateTLSCAConfig(ctx context.Context, ca *fabricxv1alpha1.CA) string {
	// Ensure defaults are set before generating config
	r.setDefaults(ca)

	// Prepare data for template
	data := ConfigData{
		Debug:        ca.Spec.Debug,
		CLRSizeLimit: ca.Spec.CLRSizeLimit,
		Database: struct {
			Type       string
			Datasource string
		}{
			Type:       ca.Spec.Database.Type,
			Datasource: ca.Spec.Database.Datasource,
		},
		Metrics: struct {
			Provider string
			Statsd   struct {
				Network       string
				Address       string
				WriteInterval string
				Prefix        string
			}
		}{
			Provider: ca.Spec.Metrics.Provider,
			Statsd: struct {
				Network       string
				Address       string
				WriteInterval string
				Prefix        string
			}{
				Network:       ca.Spec.Metrics.Statsd.Network,
				Address:       ca.Spec.Metrics.Statsd.Address,
				WriteInterval: ca.Spec.Metrics.Statsd.WriteInterval,
				Prefix:        ca.Spec.Metrics.Statsd.Prefix,
			},
		},
		CA:    ca.Spec.CA,
		TLSCA: ca.Spec.TLSCA,
	}

	config, err := GenerateConfigFromTemplate(TLSConfigTemplate, data)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to generate TLS CA configuration")
		// Fallback to a simple configuration if template fails
		return fmt.Sprintf(`tls:
  certfile: /var/hyperledger/fabric-ca/msp-tls-secret/certfile
  keyfile: /var/hyperledger/fabric-ca/msp-tls-secret/keyfile
  clientauth:
    type: noclientcert
    enabled: true
ca:
  name: %s
  keyfile: /var/hyperledger/fabric-ca/msp-tls-secret/keyfile
  certfile: /var/hyperledger/fabric-ca/msp-tls-secret/certfile
db:
  type: %s
  datasource: %s`, ca.Spec.TLSCA.Name, ca.Spec.Database.Type, ca.Spec.Database.Datasource)
	}

	return config
}

// generateTLSCertificate generates TLS certificate and key
func (r *CAReconciler) generateTLSCertificate(ca *fabricxv1alpha1.CA) ([]byte, []byte, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ECDSA private key: %w", err)
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Get IP addresses and DNS names
	ips := []net.IP{net.ParseIP("127.0.0.1")}
	dnsNames := []string{}

	// Add domains from TLS configuration
	if ca.Spec.TLS.Domains != nil {
		dnsNames = append(dnsNames, ca.Spec.TLS.Domains...)
	}

	// Add hosts from spec
	for _, host := range ca.Spec.Hosts {
		if ip := net.ParseIP(host); ip != nil {
			ips = append(ips, ip)
		} else {
			dnsNames = append(dnsNames, host)
		}
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{ca.Spec.TLS.Subject.O},
			Country:            []string{ca.Spec.TLS.Subject.C},
			Locality:           []string{ca.Spec.TLS.Subject.L},
			OrganizationalUnit: []string{ca.Spec.TLS.Subject.OU},
			StreetAddress:      []string{ca.Spec.TLS.Subject.ST},
		},
		NotBefore:             time.Now().AddDate(0, 0, -1),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           ips,
		DNSNames:              dnsNames,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create TLS certificate: %w", err)
	}

	// Encode certificate
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	// Encode private key
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	return certPEM, keyPEM, nil
}

// generateCACertificate generates CA certificate and key
func (r *CAReconciler) generateCACertificate(ca *fabricxv1alpha1.CA) ([]byte, []byte, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ECDSA private key: %w", err)
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	if len(ca.Spec.CA.CSR.Names) == 0 {
		return nil, nil, fmt.Errorf("no certificate names specified in CA CSR configuration")
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{ca.Spec.CA.CSR.Names[0].O},
			Country:            []string{ca.Spec.CA.CSR.Names[0].C},
			Locality:           []string{ca.Spec.CA.CSR.Names[0].L},
			OrganizationalUnit: []string{ca.Spec.CA.CSR.Names[0].OU},
			CommonName:         ca.Spec.CA.CSR.CN,
		},
		NotBefore:             time.Now().AddDate(0, 0, -1),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		SubjectKeyId:          r.computeSKI(privateKey),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Encode certificate
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	// Encode private key
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	return certPEM, keyPEM, nil
}

// generateTLSCACertificate generates TLS CA certificate and key
func (r *CAReconciler) generateTLSCACertificate(ca *fabricxv1alpha1.CA) ([]byte, []byte, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ECDSA private key: %w", err)
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	if len(ca.Spec.TLSCA.CSR.Names) == 0 {
		return nil, nil, fmt.Errorf("no certificate names specified in TLS CA CSR configuration")
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{ca.Spec.TLSCA.CSR.Names[0].O},
			Country:            []string{ca.Spec.TLSCA.CSR.Names[0].C},
			Locality:           []string{ca.Spec.TLSCA.CSR.Names[0].L},
			OrganizationalUnit: []string{ca.Spec.TLSCA.CSR.Names[0].OU},
			CommonName:         ca.Spec.TLSCA.CSR.CN,
		},
		NotBefore:             time.Now().AddDate(0, 0, -1),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		SubjectKeyId:          r.computeSKI(privateKey),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create TLS CA certificate: %w", err)
	}

	// Encode certificate
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	// Encode private key
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	return certPEM, keyPEM, nil
}

// computeSKI computes the Subject Key Identifier
func (r *CAReconciler) computeSKI(privKey *ecdsa.PrivateKey) []byte {
	// Convert ECDSA public key to ECDH format for modern marshaling
	ecdhKey, err := ecdh.P256().NewPublicKey(elliptic.MarshalCompressed(privKey.Curve, privKey.X, privKey.Y))
	if err != nil {
		// Use compressed marshaling as fallback
		raw := elliptic.MarshalCompressed(privKey.Curve, privKey.X, privKey.Y)
		hash := sha256.Sum256(raw)
		return hash[:]
	}

	hash := sha256.Sum256(ecdhKey.Bytes())
	return hash[:]
}

// ComputeConfigMapHash computes a hash of the ConfigMap data for triggering pod restarts
func (r *CAReconciler) ComputeConfigMapHash(ctx context.Context, ca *fabricxv1alpha1.CA) (string, error) {
	// Get the main CA ConfigMap
	caConfigMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-config", ca.Name),
		Namespace: ca.Namespace,
	}, caConfigMap)
	if err != nil {
		return "", fmt.Errorf("failed to get CA ConfigMap: %w", err)
	}

	// Get the TLS CA ConfigMap
	tlsConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-config-tls", ca.Name),
		Namespace: ca.Namespace,
	}, tlsConfigMap)
	if err != nil {
		return "", fmt.Errorf("failed to get TLS ConfigMap: %w", err)
	}

	// Create a combined data structure for hashing
	configData := struct {
		CAConfig  map[string]string `json:"caConfig"`
		TLSConfig map[string]string `json:"tlsConfig"`
	}{
		CAConfig:  caConfigMap.Data,
		TLSConfig: tlsConfigMap.Data,
	}

	// Marshal to JSON for consistent hashing
	jsonData, err := json.Marshal(configData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config data: %w", err)
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%x", hash), nil
}

// updateCAStatus updates the CA status with the given status and message
func (r *CAReconciler) updateCAStatus(ctx context.Context, ca *fabricxv1alpha1.CA, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating CA status",
		"name", ca.Name,
		"namespace", ca.Namespace,
		"status", status,
		"message", message)

	// Update the status
	ca.Status.Status = status
	ca.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	ca.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, ca); err != nil {
		log.Error(err, "Failed to update CA status")
	} else {
		log.Info("CA status updated successfully",
			"name", ca.Name,
			"namespace", ca.Namespace,
			"status", status,
			"message", message)
	}
}

// updateStatus updates the CA status (legacy method)
func (r *CAReconciler) updateStatus(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	// If status is already set to Failed, preserve it
	if ca.Status.Status == fabricxv1alpha1.FailedStatus {
		return r.Status().Update(ctx, ca)
	}

	// Get deployment status
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: ca.Name, Namespace: ca.Namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			ca.Status.Status = fabricxv1alpha1.PendingStatus
			ca.Status.Message = "Deployment not found"
		} else {
			ca.Status.Status = fabricxv1alpha1.FailedStatus
			ca.Status.Message = fmt.Sprintf("Failed to get deployment: %v", err)
			return err
		}
	} else {
		if deployment.Status.ReadyReplicas > 0 {
			ca.Status.Status = fabricxv1alpha1.RunningStatus
			ca.Status.Message = "CA is running successfully"
		} else {
			ca.Status.Status = fabricxv1alpha1.PendingStatus
			ca.Status.Message = "Deployment is not ready"
		}
	}

	// Get service to determine node port
	service := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: ca.Name, Namespace: ca.Namespace}, service)
	if err == nil && len(service.Spec.Ports) > 0 {
		ca.Status.NodePort = int(service.Spec.Ports[0].NodePort)
	}

	return r.Status().Update(ctx, ca)
}

// deleteResources deletes all CA resources
func (r *CAReconciler) deleteResources(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)
	var deletionErrors []string

	// Delete deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
		errorMsg := fmt.Sprintf("Failed to delete Deployment %s: %v", ca.Name, err)
		log.Error(err, "Failed to delete Deployment", "name", ca.Name)
		deletionErrors = append(deletionErrors, errorMsg)
	}

	// Delete service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
		errorMsg := fmt.Sprintf("Failed to delete Service %s: %v", ca.Name, err)
		log.Error(err, "Failed to delete Service", "name", ca.Name)
		deletionErrors = append(deletionErrors, errorMsg)
	}

	// Delete ingress (if exists)
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	if err := r.Delete(ctx, ingress); err != nil && !errors.IsNotFound(err) {
		errorMsg := fmt.Sprintf("Failed to delete Ingress %s: %v", ca.Name, err)
		log.Error(err, "Failed to delete Ingress", "name", ca.Name)
		deletionErrors = append(deletionErrors, errorMsg)
	}

	// Delete PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
		errorMsg := fmt.Sprintf("Failed to delete PVC %s: %v", ca.Name, err)
		log.Error(err, "Failed to delete PVC", "name", ca.Name)
		deletionErrors = append(deletionErrors, errorMsg)
	}

	// Delete ConfigMaps
	configMaps := []string{
		fmt.Sprintf("%s-config", ca.Name),
		fmt.Sprintf("%s-config-tls", ca.Name),
		fmt.Sprintf("%s-env", ca.Name),
	}
	for _, name := range configMaps {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ca.Namespace,
			},
		}
		if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
			errorMsg := fmt.Sprintf("Failed to delete ConfigMap %s: %v", name, err)
			log.Error(err, "Failed to delete ConfigMap", "name", name)
			deletionErrors = append(deletionErrors, errorMsg)
		}
	}

	// Delete Secrets
	secrets := []string{
		fmt.Sprintf("%s-tls-crypto", ca.Name),
		fmt.Sprintf("%s-msp-crypto", ca.Name),
		fmt.Sprintf("%s-tlsca-crypto", ca.Name),
	}
	for _, name := range secrets {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ca.Namespace,
			},
		}
		if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
			errorMsg := fmt.Sprintf("Failed to delete Secret %s: %v", name, err)
			log.Error(err, "Failed to delete Secret", "name", name)
			deletionErrors = append(deletionErrors, errorMsg)
		}
	}

	// If there were any errors, return them
	if len(deletionErrors) > 0 {
		combinedErrorMsg := strings.Join(deletionErrors, "; ")
		log.Error(fmt.Errorf("%s", combinedErrorMsg), "CA resource deletion failed", "ca", ca.Name)
		return fmt.Errorf("failed to delete CA resources")
	}

	log.Info("All CA resources deleted successfully", "ca", ca.Name)
	return nil
}

// GetDeploymentTemplate returns a deployment template based on the CA spec
func (r *CAReconciler) GetDeploymentTemplate(ctx context.Context, ca *fabricxv1alpha1.CA) *appsv1.Deployment {
	// Compute ConfigMap hash for triggering pod restarts
	configHash, err := r.ComputeConfigMapHash(ctx, ca)
	if err != nil {
		// Log the error but continue with deployment creation
		logf.FromContext(ctx).Error(err, "Failed to compute ConfigMap hash, continuing without hash")
		configHash = "unknown"
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ca.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "ca",
					"release": ca.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: func() map[string]string {
						labels := map[string]string{
							"app":     "ca",
							"release": ca.Name,
						}
						// Merge with custom pod labels if specified
						for k, v := range ca.Spec.PodLabels {
							labels[k] = v
						}
						return labels
					}(),
					Annotations: func() map[string]string {
						annotations := make(map[string]string)
						// Copy existing annotations
						for k, v := range ca.Spec.PodAnnotations {
							annotations[k] = v
						}
						// Add ConfigMap hash annotation
						annotations["checksum/config"] = configHash
						return annotations
					}(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "ca",
							Image: fmt.Sprintf("%s:%s", ca.Spec.Image, ca.Spec.Version),
							Command: func() []string {
								// Build the command with optional idemix flag
								startCmd := "fabric-ca-server start"

								// Add idemix flag if configured
								if ca.Spec.Idemix != nil && ca.Spec.Idemix.Curve != "" {
									startCmd += fmt.Sprintf(" --idemix.curve %s", ca.Spec.Idemix.Curve)
								}

								baseCmd := fmt.Sprintf(`mkdir -p $FABRIC_CA_HOME
cp /var/hyperledger/ca_config/ca.yaml $FABRIC_CA_HOME/fabric-ca-server-config.yaml
cp /var/hyperledger/ca_config_tls/fabric-ca-server-config.yaml $FABRIC_CA_HOME/fabric-ca-server-config-tls.yaml

ls -l $FABRIC_CA_HOME

echo ">\033[0;35m fabric-ca-server start \033[0m"
%s`, startCmd)

								return []string{"sh", "-c", baseCmd}
							}(),
							Ports: []corev1.ContainerPort{
								{
									Name:          "ca-port",
									ContainerPort: 7054,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "operations-port",
									ContainerPort: 9443,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/cainfo",
										Port:   intstr.FromInt(7054),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    10,
								SuccessThreshold: 1,
								FailureThreshold: 3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/cainfo",
										Port:   intstr.FromInt(7054),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    10,
								SuccessThreshold: 1,
								FailureThreshold: 3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/hyperledger",
								},
								{
									Name:      "ca-config",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/ca_config",
								},
								{
									Name:      "ca-config-tls",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/ca_config_tls",
								},
								{
									Name:      "tls-secret",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/tls/secret",
								},
								{
									Name:      "msp-cryptomaterial",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/fabric-ca/msp-secret",
								},
								{
									Name:      "msp-tls-cryptomaterial",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/fabric-ca/msp-tls-secret",
								},
							},
							Resources: ca.Spec.Resources,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: ca.Name,
								},
							},
						},
						{
							Name: "ca-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", ca.Name),
									},
								},
							},
						},
						{
							Name: "ca-config-tls",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config-tls", ca.Name),
									},
								},
							},
						},
						{
							Name: "tls-secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-tls-crypto", ca.Name),
								},
							},
						},
						{
							Name: "msp-cryptomaterial",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-msp-crypto", ca.Name),
								},
							},
						},
						{
							Name: "msp-tls-cryptomaterial",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-tlsca-crypto", ca.Name),
								},
							},
						},
					},
				},
			},
		},
	}

	// Set environment variables
	deployment.Spec.Template.Spec.Containers[0].Env = ca.Spec.Env

	// Set image pull secrets
	deployment.Spec.Template.Spec.ImagePullSecrets = ca.Spec.ImagePullSecrets

	// Set node selector
	if ca.Spec.NodeSelector != nil && len(ca.Spec.NodeSelector.NodeSelectorTerms) > 0 {
		nodeSelector := make(map[string]string)
		for _, term := range ca.Spec.NodeSelector.NodeSelectorTerms {
			for _, req := range term.MatchExpressions {
				if req.Operator == corev1.NodeSelectorOpIn && len(req.Values) > 0 {
					nodeSelector[req.Key] = req.Values[0]
				}
			}
		}
		deployment.Spec.Template.Spec.NodeSelector = nodeSelector
	} else {
		// Clear node selector if not specified
		deployment.Spec.Template.Spec.NodeSelector = nil
	}

	// Set affinity
	deployment.Spec.Template.Spec.Affinity = ca.Spec.Affinity

	// Set tolerations
	deployment.Spec.Template.Spec.Tolerations = ca.Spec.Tolerations

	return deployment
}

// getServiceTemplate returns a service template based on the CA spec
func (r *CAReconciler) getServiceTemplate(ca *fabricxv1alpha1.CA) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: ca.Spec.Service.ServiceType,
			Ports: []corev1.ServicePort{
				{
					Port:       7054,
					TargetPort: intstr.FromInt(7054),
					Protocol:   corev1.ProtocolTCP,
					Name:       "https",
				},
				{
					Port:       9443,
					TargetPort: intstr.FromInt(9443),
					Protocol:   corev1.ProtocolTCP,
					Name:       "operations",
				},
			},
			Selector: map[string]string{
				"app":     "ca",
				"release": ca.Name,
			},
		},
	}
}

// getPVCTemplate returns a PVC template based on the CA spec
func (r *CAReconciler) getPVCTemplate(ca *fabricxv1alpha1.CA) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode(ca.Spec.Storage.AccessMode),
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(ca.Spec.Storage.Size),
				},
			},
		},
	}

	if ca.Spec.Storage.StorageClass != "" && ca.Spec.Storage.StorageClass != "-" {
		pvc.Spec.StorageClassName = &ca.Spec.Storage.StorageClass
	} else {
		// Clear storage class if not specified or set to "-"
		pvc.Spec.StorageClassName = nil
	}

	return pvc
}

// getConfigMapTemplate returns a configmap template based on the CA spec
func (r *CAReconciler) getConfigMapTemplate(ca *fabricxv1alpha1.CA, name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ca.Namespace,
		},
		Data: data,
	}
}

// getSecretTemplate returns a secret template based on the CA spec
func (r *CAReconciler) getSecretTemplate(ca *fabricxv1alpha1.CA, name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ca.Namespace,
		},
		Data: data,
	}
}

// updateDeployment updates a deployment with template data
func (r *CAReconciler) updateDeployment(ctx context.Context, ca *fabricxv1alpha1.CA, deployment *appsv1.Deployment, template *appsv1.Deployment) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ca, deployment, r.Scheme); err != nil {
			return err
		}

		// Update deployment spec
		deployment.Spec = template.Spec

		// Update metadata
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			deployment.Labels[k] = v
		}
		if deployment.Annotations == nil {
			deployment.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			deployment.Annotations[k] = v
		}

		return nil
	})

	return err
}

// updateService updates a service with template data
func (r *CAReconciler) updateService(ctx context.Context, ca *fabricxv1alpha1.CA, service *corev1.Service, template *corev1.Service) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ca, service, r.Scheme); err != nil {
			return err
		}

		// Update service spec
		service.Spec = template.Spec

		// Update metadata
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			service.Labels[k] = v
		}
		if service.Annotations == nil {
			service.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			service.Annotations[k] = v
		}

		return nil
	})

	return err
}

// updatePVC updates a PVC with template data
func (r *CAReconciler) updatePVC(ctx context.Context, ca *fabricxv1alpha1.CA, pvc *corev1.PersistentVolumeClaim, template *corev1.PersistentVolumeClaim) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ca, pvc, r.Scheme); err != nil {
			return err
		}

		// For new PVCs, copy the entire spec from template
		if len(pvc.Spec.AccessModes) == 0 {
			// This is a new PVC, copy the entire spec
			pvc.Spec = template.Spec
			logf.FromContext(ctx).Info("Creating new PVC with template spec",
				"pvc", pvc.Name, "accessModes", template.Spec.AccessModes, "size", template.Spec.Resources.Requests[corev1.ResourceStorage])
		} else {
			// This is an existing PVC, handle updates carefully - some fields are immutable
			if pvc.Spec.StorageClassName != nil && template.Spec.StorageClassName != nil {
				// Only update storage class if it's actually different
				if *pvc.Spec.StorageClassName != *template.Spec.StorageClassName {
					// Storage class cannot be changed after creation
					// Log a warning but don't fail
					logf.FromContext(ctx).Info("Storage class cannot be changed for existing PVC",
						"pvc", pvc.Name, "old", *pvc.Spec.StorageClassName, "new", *template.Spec.StorageClassName)
				}
			}

			// Handle storage size updates
			if len(template.Spec.Resources.Requests) > 0 {
				newSize := template.Spec.Resources.Requests[corev1.ResourceStorage]

				// If PVC doesn't have size set yet, set it
				if len(pvc.Spec.Resources.Requests) == 0 {
					pvc.Spec.Resources.Requests = corev1.ResourceList{
						corev1.ResourceStorage: newSize,
					}
					logf.FromContext(ctx).Info("Setting initial PVC storage size",
						"pvc", pvc.Name, "size", newSize)
				} else {
					// Check if we need to update the size
					currentSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
					if newSize.Cmp(currentSize) != 0 {
						// Size is different, check if it's an increase
						if newSize.Cmp(currentSize) > 0 {
							// Only increase size, never decrease
							pvc.Spec.Resources.Requests[corev1.ResourceStorage] = newSize
							logf.FromContext(ctx).Info("Increasing PVC storage size",
								"pvc", pvc.Name, "old", currentSize, "new", newSize)
						} else {
							// Log warning for size decrease attempt
							logf.FromContext(ctx).Info("Cannot decrease PVC storage size",
								"pvc", pvc.Name, "current", currentSize, "requested", newSize)
						}
					} else {
						logf.FromContext(ctx).Info("PVC storage size unchanged",
							"pvc", pvc.Name, "size", currentSize)
					}
				}
			}

			// Update access modes if they're different
			if !r.accessModesEqual(pvc.Spec.AccessModes, template.Spec.AccessModes) {
				// Access modes cannot be changed after creation
				logf.FromContext(ctx).Info("Access modes cannot be changed for existing PVC",
					"pvc", pvc.Name, "current", pvc.Spec.AccessModes, "requested", template.Spec.AccessModes)
			}
		}

		// Update metadata
		if pvc.Labels == nil {
			pvc.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			pvc.Labels[k] = v
		}
		if pvc.Annotations == nil {
			pvc.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			pvc.Annotations[k] = v
		}

		return nil
	})

	// Handle PVC resize errors - treat them as failures
	if err != nil {
		// Check if it's a PVC resize error
		if r.isPVCResizeError(err) {
			logf.FromContext(ctx).Error(err, "PVC resize failed - this is a reconciliation error",
				"pvc", pvc.Name, "error", err.Error())
			// Return the error to mark the CA as failed
			return fmt.Errorf("PVC resize failed: %v", err)
		}
	}

	return err
}

// updateConfigMap updates a ConfigMap with template data
func (r *CAReconciler) updateConfigMap(ctx context.Context, ca *fabricxv1alpha1.CA, configMap *corev1.ConfigMap, template *corev1.ConfigMap) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ca, configMap, r.Scheme); err != nil {
			return err
		}

		// Check if ConfigMap is immutable
		if configMap.Immutable != nil && *configMap.Immutable {
			// Cannot update immutable ConfigMap - need to delete and recreate
			logf.FromContext(ctx).Info("ConfigMap is immutable, will delete and recreate", "configmap", configMap.Name)
			if err := r.Delete(ctx, configMap); err != nil {
				return err
			}
			// Create new ConfigMap with template data
			newConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMap.Name,
					Namespace: configMap.Namespace,
				},
				Data: template.Data,
			}
			if err := controllerutil.SetControllerReference(ca, newConfigMap, r.Scheme); err != nil {
				return err
			}
			return r.Create(ctx, newConfigMap)
		}

		// Normal update for mutable ConfigMap
		configMap.Data = template.Data
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			configMap.Labels[k] = v
		}
		if configMap.Annotations == nil {
			configMap.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			configMap.Annotations[k] = v
		}

		return nil
	})

	return err
}

// updateSecret updates a Secret with template data
func (r *CAReconciler) updateSecret(ctx context.Context, ca *fabricxv1alpha1.CA, secret *corev1.Secret, template *corev1.Secret) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ca, secret, r.Scheme); err != nil {
			return err
		}

		// Check if Secret is immutable
		if secret.Immutable != nil && *secret.Immutable {
			// Cannot update immutable Secret - need to delete and recreate
			logf.FromContext(ctx).Info("Secret is immutable, will delete and recreate", "secret", secret.Name)
			if err := r.Delete(ctx, secret); err != nil {
				return err
			}
			// Create new Secret with template data
			newSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				},
				Data: template.Data,
			}
			if err := controllerutil.SetControllerReference(ca, newSecret, r.Scheme); err != nil {
				return err
			}
			return r.Create(ctx, newSecret)
		}

		// Normal update for mutable Secret
		secret.Data = template.Data
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			secret.Labels[k] = v
		}
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			secret.Annotations[k] = v
		}

		return nil
	})

	return err
}

// accessModesEqual compares two access mode slices
func (r *CAReconciler) accessModesEqual(a, b []corev1.PersistentVolumeAccessMode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// isPVCResizeError checks if the error is related to PVC resize restrictions
func (r *CAReconciler) isPVCResizeError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	return strings.Contains(errMsg, "only dynamically provisioned pvc can be resized") ||
		strings.Contains(errMsg, "storageclass that provisions the pvc must support resize") ||
		strings.Contains(errMsg, "forbidden") && strings.Contains(errMsg, "resize")
}

// getIngressTemplate returns an ingress template based on the CA spec
func (r *CAReconciler) getIngressTemplate(ca *fabricxv1alpha1.CA) *networkingv1.Ingress {
	if ca.Spec.Ingress == nil || !ca.Spec.Ingress.Enabled || ca.Spec.Ingress.Gateway == nil {
		return nil
	}

	gateway := ca.Spec.Ingress.Gateway
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{},
		},
	}

	// Add rules for each host
	for _, host := range gateway.Hosts {
		rule := networkingv1.IngressRule{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
							Path:     "/",
							PathType: &[]networkingv1.PathType{networkingv1.PathTypePrefix}[0],
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: ca.Name,
									Port: networkingv1.ServiceBackendPort{
										Number: int32(7054),
									},
								},
							},
						},
					},
				},
			},
		}
		ingress.Spec.Rules = append(ingress.Spec.Rules, rule)
	}

	// Add TLS configuration if enabled
	if gateway.TLS != nil && gateway.TLS.Enabled {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      gateway.Hosts,
				SecretName: gateway.TLS.SecretName,
			},
		}
	}

	return ingress
}

// updateIngress updates an Ingress with template data
func (r *CAReconciler) updateIngress(ctx context.Context, ca *fabricxv1alpha1.CA, ingress *networkingv1.Ingress, template *networkingv1.Ingress) error {
	if template == nil {
		// If template is nil, delete the ingress
		if err := r.Delete(ctx, ingress); err != nil && !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ingress, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ca, ingress, r.Scheme); err != nil {
			return err
		}

		// Update ingress spec
		ingress.Spec = template.Spec

		// Update metadata
		if ingress.Labels == nil {
			ingress.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			ingress.Labels[k] = v
		}
		if ingress.Annotations == nil {
			ingress.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			ingress.Annotations[k] = v
		}

		return nil
	})

	return err
}

// reconcileIdemixServiceAccount creates a service account with necessary RBAC for the idemix keys extraction Job
func (r *CAReconciler) reconcileIdemixServiceAccount(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	// Create ServiceAccount
	saName := fmt.Sprintf("%s-idemix-extractor", ca.Name)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: ca.Namespace,
		},
	}

	if err := controllerutil.SetControllerReference(ca, sa, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for service account: %w", err)
	}

	existingSA := &corev1.ServiceAccount{}
	saNamespacedName := types.NamespacedName{Name: saName, Namespace: ca.Namespace}
	if err := r.Get(ctx, saNamespacedName, existingSA); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating idemix extractor service account", "serviceAccount", saName)
			if err := r.Create(ctx, sa); err != nil {
				return fmt.Errorf("failed to create service account: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get service account: %w", err)
		}
	}

	// Create Role
	roleName := fmt.Sprintf("%s-idemix-extractor", ca.Name)
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: ca.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "create", "update", "patch"},
			},
		},
	}

	if err := controllerutil.SetControllerReference(ca, role, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for role: %w", err)
	}

	existingRole := &rbacv1.Role{}
	roleNamespacedName := types.NamespacedName{Name: roleName, Namespace: ca.Namespace}
	if err := r.Get(ctx, roleNamespacedName, existingRole); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating idemix extractor role", "role", roleName)
			if err := r.Create(ctx, role); err != nil {
				return fmt.Errorf("failed to create role: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get role: %w", err)
		}
	}

	// Create RoleBinding
	rbName := fmt.Sprintf("%s-idemix-extractor", ca.Name)
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbName,
			Namespace: ca.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: ca.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
	}

	if err := controllerutil.SetControllerReference(ca, rb, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for role binding: %w", err)
	}

	existingRB := &rbacv1.RoleBinding{}
	rbNamespacedName := types.NamespacedName{Name: rbName, Namespace: ca.Namespace}
	if err := r.Get(ctx, rbNamespacedName, existingRB); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating idemix extractor role binding", "roleBinding", rbName)
			if err := r.Create(ctx, rb); err != nil {
				return fmt.Errorf("failed to create role binding: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get role binding: %w", err)
		}
	}

	return nil
}

// reconcileIdemixKeysJob creates a Job to extract idemix issuer keys from the CA pod to a Kubernetes secret
// TODO: This should be replaced with direct pod exec using REST client instead of a Job with kubectl
func (r *CAReconciler) reconcileIdemixKeysJob(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	log := logf.FromContext(ctx)

	// First, ensure service account and RBAC are in place
	if err := r.reconcileIdemixServiceAccount(ctx, ca); err != nil {
		return fmt.Errorf("failed to reconcile idemix service account: %w", err)
	}

	// Check if the deployment is ready
	deployment := &appsv1.Deployment{}
	deploymentName := types.NamespacedName{
		Name:      ca.Name,
		Namespace: ca.Namespace,
	}
	if err := r.Get(ctx, deploymentName, deployment); err != nil {
		log.Info("CA deployment not found, skipping idemix keys Job", "deployment", deploymentName)
		return nil
	}

	// Wait for at least one ready replica
	if deployment.Status.ReadyReplicas == 0 {
		log.Info("CA deployment not ready yet, skipping idemix keys Job", "deployment", deploymentName)
		return nil
	}

	// Define secret name
	secretName := fmt.Sprintf("%s-idemix-issuer-keys", ca.Name)

	// Check if secret already exists - if so, no need to create Job
	existingSecret := &corev1.Secret{}
	secretNamespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: ca.Namespace,
	}
	if err := r.Get(ctx, secretNamespacedName, existingSecret); err == nil {
		log.Info("Idemix issuer keys secret already exists, skipping Job creation", "secret", secretName)
		return nil
	}

	// Create the Job to extract keys
	jobName := fmt.Sprintf("%s-extract-idemix-keys", ca.Name)
	job := &batchv1.Job{}
	jobNamespacedName := types.NamespacedName{
		Name:      jobName,
		Namespace: ca.Namespace,
	}

	// Check if job already exists
	if err := r.Get(ctx, jobNamespacedName, job); err == nil {
		if job.Status.Succeeded > 0 {
			log.Info("Idemix keys extraction Job already completed", "job", jobName)
			return nil
		}
		// Job exists but hasn't completed yet
		log.Info("Idemix keys extraction Job still running", "job", jobName)
		return nil
	}

	// Create the Job template
	backoffLimit := int32(3)
	ttlSecondsAfterFinished := int32(600) // 10 minutes
	jobTemplate := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ca.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "fabric-ca",
				"app.kubernetes.io/instance":   ca.Name,
				"app.kubernetes.io/component":  "idemix-keys-extractor",
				"app.kubernetes.io/managed-by": "fabric-x-operator",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: fmt.Sprintf("%s-idemix-extractor", ca.Name),
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "extract-idemix-keys",
							Image: "bitnami/kubectl:latest",
							Command: []string{
								"/bin/bash",
								"-c",
								fmt.Sprintf(`
set -e

# Get the CA pod name
CA_POD=$(kubectl get pods -n %s -l app=ca,release=%s -o jsonpath='{.items[0].metadata.name}')

if [ -z "$CA_POD" ]; then
  echo "Error: CA pod not found"
  exit 1
fi

echo "Found CA pod: $CA_POD"

# Create temporary directory
TMP_DIR=$(mktemp -d)

# Extract issuer keys from CA pod
echo "Extracting IssuerPublicKey..."
kubectl exec -n %s $CA_POD -- cat /etc/hyperledger/fabric-ca-server/IssuerPublicKey > $TMP_DIR/IssuerPublicKey

echo "Extracting IssuerRevocationPublicKey..."
kubectl exec -n %s $CA_POD -- cat /etc/hyperledger/fabric-ca-server/IssuerRevocationPublicKey > $TMP_DIR/IssuerRevocationPublicKey

echo "Extracting IssuerSecretKey..."
kubectl exec -n %s $CA_POD -- cat /etc/hyperledger/fabric-ca-server/msp/keystore/IssuerSecretKey > $TMP_DIR/IssuerSecretKey

echo "Extracting IssuerRevocationPrivateKey..."
kubectl exec -n %s $CA_POD -- cat /etc/hyperledger/fabric-ca-server/msp/keystore/IssuerRevocationPrivateKey > $TMP_DIR/RevocationKey

# Create or update the secret
if kubectl get secret -n %s %s &>/dev/null; then
  echo "Updating existing secret..."
  kubectl create secret generic -n %s %s \
    --from-file=IssuerPublicKey=$TMP_DIR/IssuerPublicKey \
    --from-file=IssuerRevocationPublicKey=$TMP_DIR/IssuerRevocationPublicKey \
    --from-file=IssuerSecretKey=$TMP_DIR/IssuerSecretKey \
    --from-file=RevocationKey=$TMP_DIR/RevocationKey \
    --dry-run=client -o yaml | kubectl apply -f -
else
  echo "Creating new secret..."
  kubectl create secret generic -n %s %s \
    --from-file=IssuerPublicKey=$TMP_DIR/IssuerPublicKey \
    --from-file=IssuerRevocationPublicKey=$TMP_DIR/IssuerRevocationPublicKey \
    --from-file=IssuerSecretKey=$TMP_DIR/IssuerSecretKey \
    --from-file=RevocationKey=$TMP_DIR/RevocationKey
fi

echo "Idemix issuer keys extracted successfully!"

# Cleanup
rm -rf $TMP_DIR
`, ca.Namespace, ca.Name, ca.Namespace, ca.Namespace, ca.Namespace, ca.Namespace, ca.Namespace, secretName, ca.Namespace, secretName, ca.Namespace, secretName),
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(ca, jobTemplate, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for idemix keys Job: %w", err)
	}

	// Create or update the Job
	if err := r.Get(ctx, jobNamespacedName, job); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating idemix keys extraction Job", "job", jobName)
			if err := r.Create(ctx, jobTemplate); err != nil {
				return fmt.Errorf("failed to create idemix keys Job: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get idemix keys Job: %w", err)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.CA{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.Ingress{}).
		Named("ca").
		Complete(r)
}
