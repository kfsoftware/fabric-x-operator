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
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	// Gateway API imports
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

const (
	// OrdererAssemblerFinalizerName is the name of the finalizer used by OrdererAssembler
	OrdererAssemblerFinalizerName = "ordererassembler.fabricx.kfsoft.tech/finalizer"
	// AssemblerServicePort is the port the assembler service listens on
	AssemblerServicePort = 7050
)

// OrdererAssemblerReconciler reconciles a OrdererAssembler object
type OrdererAssemblerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererassemblers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererassemblers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererassemblers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OrdererAssemblerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererAssembler reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the OrdererAssembler status to failed
			ordererAssembler := &fabricxv1alpha1.OrdererAssembler{}
			if err := r.Get(ctx, req.NamespacedName, ordererAssembler); err == nil {
				panicMsg := fmt.Sprintf("Panic in OrdererAssembler reconciliation: %v", panicErr)
				r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var ordererAssembler fabricxv1alpha1.OrdererAssembler
	if err := r.Get(ctx, req.NamespacedName, &ordererAssembler); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the OrdererAssembler is being deleted
	if !ordererAssembler.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &ordererAssembler)
	}

	// Set initial status if not set
	if ordererAssembler.Status.Status == "" {
		r.updateOrdererAssemblerStatus(ctx, &ordererAssembler, fabricxv1alpha1.PendingStatus, "Initializing OrdererAssembler")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &ordererAssembler); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateOrdererAssemblerStatus(ctx, &ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the OrdererAssembler
	if err := r.reconcileOrdererAssembler(ctx, &ordererAssembler); err != nil {
		// The reconcileOrdererAssembler method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if ordererAssembler.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile OrdererAssembler: %v", err)
			r.updateOrdererAssemblerStatus(ctx, &ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile OrdererAssembler")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute to ensure continuous monitoring
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileOrdererAssembler handles the reconciliation of an OrdererAssembler
func (r *OrdererAssemblerReconciler) reconcileOrdererAssembler(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererAssembler reconciliation",
				"ordererAssembler", ordererAssembler.Name, "namespace", ordererAssembler.Namespace)

			// Update the OrdererAssembler status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererAssembler reconciliation: %v", panicErr)
			r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Starting OrdererAssembler reconciliation",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace,
		"bootstrapMode", ordererAssembler.Spec.BootstrapMode)

	// Check bootstrap mode - only deploy when bootstrapMode is "deploy"
	bootstrapMode := ordererAssembler.Spec.BootstrapMode
	if bootstrapMode == "" {
		bootstrapMode = "configure" // Default to configure mode
	}

	// Reconcile based on deployment mode
	switch ordererAssembler.Spec.BootstrapMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, ordererAssembler); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, ordererAssembler); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid bootstrap mode: %s", ordererAssembler.Spec.BootstrapMode)
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid bootstrap mode")
		r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	// Update status to success
	r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.RunningStatus, "OrdererAssembler reconciled successfully")

	log.Info("OrdererAssembler reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *OrdererAssemblerReconciler) reconcileConfigureMode(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererAssembler in configure mode",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace)

	// In configure mode, only create certificates
	if err := r.reconcileCertificates(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("OrdererAssembler certificates created in configure mode")

	log.Info("OrdererAssembler configure mode reconciliation completed")
	return nil
}

// reconcileGenesisBlock creates or updates the genesis block secret for the OrdererAssembler
func (r *OrdererAssemblerReconciler) reconcileGenesisBlock(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	// Check if genesis configuration is provided
	if ordererAssembler.Spec.Genesis.SecretName == "" {
		log.Info("No genesis block configuration found, skipping genesis block reconciliation")
		return nil
	}

	// Verify that the genesis block secret exists
	genesisSecret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: func() string {
			if ordererAssembler.Spec.Genesis.SecretNamespace != "" {
				return ordererAssembler.Spec.Genesis.SecretNamespace
			}
			return ordererAssembler.Namespace
		}(),
		Name: ordererAssembler.Spec.Genesis.SecretName,
	}, genesisSecret)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "Genesis block secret not found",
				"secretName", ordererAssembler.Spec.Genesis.SecretName,
				"secretNamespace", func() string {
					if ordererAssembler.Spec.Genesis.SecretNamespace != "" {
						return ordererAssembler.Spec.Genesis.SecretNamespace
					}
					return ordererAssembler.Namespace
				}())
			return fmt.Errorf("genesis block secret not found: %w", err)
		}
		return fmt.Errorf("failed to get genesis block secret: %w", err)
	}

	// Check if the genesis block data exists in the secret
	genesisKey := ordererAssembler.Spec.Genesis.SecretKey
	if genesisKey == "" {
		genesisKey = "genesis.block" // Default key name
	}

	if _, exists := genesisSecret.Data[genesisKey]; !exists {
		log.Error(fmt.Errorf("genesis block data not found in secret"),
			"Genesis block data not found in secret",
			"secretName", ordererAssembler.Spec.Genesis.SecretName,
			"secretKey", genesisKey)
		return fmt.Errorf("genesis block data not found in secret %s with key %s", ordererAssembler.Spec.Genesis.SecretName, genesisKey)
	}

	log.Info("Genesis block secret verified successfully",
		"secretName", ordererAssembler.Spec.Genesis.SecretName,
		"secretKey", genesisKey)
	return nil
}

// reconcileCertificates creates or updates certificates for the OrdererAssembler
func (r *OrdererAssemblerReconciler) reconcileCertificates(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	// Check if enrollment is configured
	if ordererAssembler.Spec.Enrollment == nil {
		log.Info("No enrollment configuration found, skipping certificate creation")
		return nil
	}

	// Generate certificates for each type (each function handles its own existence check)
	var allCertificates []certs.ComponentCertificateData

	// Create sign certificate with component-specific SANS if available
	signCertConfig := &fabricxv1alpha1.CertificateConfig{
		CA: ordererAssembler.Spec.Enrollment.Sign.CA,
	}

	signRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    ordererAssembler.Name,
		ComponentType:    "assembler",
		Namespace:        ordererAssembler.Namespace,
		OrdererGroupName: ordererAssembler.Name, // Using assembler name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfigAssembler(ordererAssembler.Spec.MSPID, signCertConfig, "sign"),
		EnrollmentConfig: r.convertToEnrollmentConfigAssembler(ordererAssembler.Spec.MSPID, ordererAssembler.Spec.Enrollment),
	}
	signCertData, err := certs.CreateSignCertificate(ctx, r.Client, signRequest)
	if err != nil {
		return fmt.Errorf("failed to create sign certificate: %w", err)
	}
	if signCertData != nil {
		allCertificates = append(allCertificates, *signCertData)
	}

	// Create TLS certificate with component-specific SANS if available
	tlsCertConfig := &fabricxv1alpha1.CertificateConfig{
		CA: ordererAssembler.Spec.Enrollment.TLS.CA,
	}
	// Use component-specific SANS if available, otherwise use enrollment SANS
	if ordererAssembler.Spec.SANS != nil {
		tlsCertConfig.SANS = ordererAssembler.Spec.SANS
	} else if ordererAssembler.Spec.Enrollment.TLS.SANS != nil {
		tlsCertConfig.SANS = ordererAssembler.Spec.Enrollment.TLS.SANS
	}

	tlsRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    ordererAssembler.Name,
		ComponentType:    "assembler",
		Namespace:        ordererAssembler.Namespace,
		OrdererGroupName: ordererAssembler.Name, // Using assembler name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfigAssembler(ordererAssembler.Spec.MSPID, tlsCertConfig, "tls"),
		EnrollmentConfig: r.convertToEnrollmentConfigAssembler(ordererAssembler.Spec.MSPID, ordererAssembler.Spec.Enrollment),
	}
	tlsCertData, err := certs.CreateTLSCertificate(ctx, r.Client, tlsRequest)
	if err != nil {
		return fmt.Errorf("failed to create TLS certificate: %w", err)
	}
	if tlsCertData != nil {
		allCertificates = append(allCertificates, *tlsCertData)
	}

	// Create Kubernetes secrets for the certificates (only if any were generated)
	if len(allCertificates) > 0 {
		if err := r.createCertificateSecrets(ctx, ordererAssembler, allCertificates); err != nil {
			return fmt.Errorf("failed to create certificate secrets: %w", err)
		}
	}

	log.Info("Certificates reconciled successfully", "assembler", ordererAssembler.Name)
	return nil
}

// convertToCertConfigAssembler converts API certificate config to internal format
func (r *OrdererAssemblerReconciler) convertToCertConfigAssembler(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig, certType string) *certs.CertificateConfig {
	if apiConfig == nil {
		return nil
	}

	config := &certs.CertificateConfig{
		MSPID: mspID,
	}

	// Add CA configuration if provided
	if apiConfig.CA != nil {
		config.CA = &certs.CACertificateConfig{
			CAHost:       apiConfig.CA.CAHost,
			CAName:       apiConfig.CA.CAName,
			CAPort:       apiConfig.CA.CAPort,
			EnrollID:     apiConfig.CA.EnrollID,
			EnrollSecret: apiConfig.CA.EnrollSecret,
		}

		// Add CATLS configuration if provided
		if apiConfig.CA.CATLS != nil {
			config.CA.CATLS = &certs.CATLSConfig{
				CACert: apiConfig.CA.CATLS.CACert,
			}
			if apiConfig.CA.CATLS.SecretRef != nil {
				config.CA.CATLS.SecretRef = &certs.SecretRef{
					Name:      apiConfig.CA.CATLS.SecretRef.Name,
					Key:       apiConfig.CA.CATLS.SecretRef.Key,
					Namespace: apiConfig.CA.CATLS.SecretRef.Namespace,
				}
			}
		}
	}

	// Add SANS configuration if provided
	if certType == "tls" && apiConfig.SANS != nil {
		config.SANS = &certs.SANSConfig{
			DNSNames:    apiConfig.SANS.DNSNames,
			IPAddresses: apiConfig.SANS.IPAddresses,
		}
	}

	return config
}

// convertToEnrollmentConfigAssembler converts API enrollment config to internal format
func (r *OrdererAssemblerReconciler) convertToEnrollmentConfigAssembler(mspID string, apiConfig *fabricxv1alpha1.EnrollmentConfig) *certs.EnrollmentConfig {
	if apiConfig == nil {
		return nil
	}

	config := &certs.EnrollmentConfig{}

	if apiConfig.Sign != nil {
		config.Sign = r.convertToCertConfigAssembler(mspID, apiConfig.Sign, "sign")
	}

	if apiConfig.TLS != nil {
		config.TLS = r.convertToCertConfigAssembler(mspID, apiConfig.TLS, "tls")
	}

	return config
}

// createCertificateSecrets creates Kubernetes secrets for certificate data
func (r *OrdererAssemblerReconciler) createCertificateSecrets(
	ctx context.Context,
	ordererAssembler *fabricxv1alpha1.OrdererAssembler,
	certificates []certs.ComponentCertificateData,
) error {
	log := logf.FromContext(ctx)

	// Process each certificate in the slice
	for _, certData := range certificates {
		secretName := fmt.Sprintf("%s-%s-cert", ordererAssembler.Name, certData.CertType)

		// Check if secret already exists
		existingSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: ordererAssembler.Namespace,
		}, existingSecret)

		if err != nil {
			if errors.IsNotFound(err) {
				// Secret doesn't exist, create it
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: ordererAssembler.Namespace,
						Labels: map[string]string{
							"app":                      "fabric-x",
							"ordererassembler":         ordererAssembler.Name,
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
				if err := controllerutil.SetControllerReference(ordererAssembler, secret, r.Scheme); err != nil {
					return fmt.Errorf("failed to set controller reference for secret %s: %w", secretName, err)
				}

				if err := r.Create(ctx, secret); err != nil {
					return fmt.Errorf("failed to create certificate secret %s: %w", secretName, err)
				}

				log.Info("Created certificate secret", "secret", secretName, "certType", certData.CertType)
			} else {
				return fmt.Errorf("failed to check existing certificate secret %s: %w", secretName, err)
			}
		} else {
			// Secret exists, check if it needs to be updated
			needsUpdate := false
			updatedSecret := existingSecret.DeepCopy()

			// Check if certificate data has changed
			if !reflect.DeepEqual(existingSecret.Data["cert.pem"], certData.Cert) ||
				!reflect.DeepEqual(existingSecret.Data["key.pem"], certData.Key) ||
				!reflect.DeepEqual(existingSecret.Data["ca.pem"], certData.CACert) {

				updatedSecret.Data = map[string][]byte{
					"cert.pem": certData.Cert,
					"key.pem":  certData.Key,
					"ca.pem":   certData.CACert,
				}
				needsUpdate = true
			}

			// Check if labels need to be updated
			expectedLabels := map[string]string{
				"app":                      "fabric-x",
				"ordererassembler":         ordererAssembler.Name,
				"certificate-type":         certData.CertType,
				"fabricx.kfsoft.tech/type": "certificate",
			}
			if !reflect.DeepEqual(existingSecret.Labels, expectedLabels) {
				updatedSecret.Labels = expectedLabels
				needsUpdate = true
			}

			if needsUpdate {
				if err := r.Update(ctx, updatedSecret); err != nil {
					return fmt.Errorf("failed to update certificate secret %s: %w", secretName, err)
				}
				log.Info("Updated certificate secret", "secret", secretName, "certType", certData.CertType)
			} else {
				log.Info("Certificate secret already exists and is up to date", "secret", secretName, "certType", certData.CertType)
			}
		}
	}

	return nil
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *OrdererAssemblerReconciler) reconcileDeployMode(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererAssembler in deploy mode",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace)

	// 1. Create/Update certificates first
	if err := r.reconcileCertificates(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// 2. Create/Update genesis block secret
	if err := r.reconcileGenesisBlock(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile genesis block: %w", err)
	}

	// 3. Create/Update ConfigMap for Assembler configuration
	if err := r.reconcileConfigMap(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile ConfigMap: %w", err)
	}

	// 3. Create/Update Service
	if err := r.reconcileService(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}
	// 5. Create/Update PVC if needed
	if err := r.reconcilePVC(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile PVC: %w", err)
	}
	// 4. Create/Update Deployment for Assembler
	if err := r.reconcileDeployment(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile Deployment: %w", err)
	}

	// 6. Create/Update Gateway resources
	if err := r.reconcileGatewayResources(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile Gateway resources: %w", err)
	}

	log.Info("OrdererAssembler deploy mode reconciliation completed")
	return nil
}

// handleDeletion handles the deletion of an OrdererAssembler
func (r *OrdererAssemblerReconciler) handleDeletion(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererAssembler deletion",
				"ordererAssembler", ordererAssembler.Name, "namespace", ordererAssembler.Namespace)

			// Update the OrdererAssembler status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererAssembler deletion: %v", panicErr)
			r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Handling OrdererAssembler deletion",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace)

	// Set status to indicate deletion
	r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.PendingStatus, "Deleting OrdererAssembler resources")

	// Clean up Gateway resources if they exist
	if ordererAssembler.Spec.Ingress != nil && ordererAssembler.Spec.Ingress.Gateway != nil {
		if err := r.cleanupGatewayResources(ctx, ordererAssembler); err != nil {
			log.Error(err, "Failed to cleanup Gateway resources")
		}
	}

	// Clean up deployment resources
	if err := r.cleanupDeploymentResources(ctx, ordererAssembler); err != nil {
		log.Error(err, "Failed to cleanup deployment resources")
	}

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, ordererAssembler); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("OrdererAssembler deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the OrdererAssembler
func (r *OrdererAssemblerReconciler) ensureFinalizer(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	if !utils.ContainsString(ordererAssembler.Finalizers, OrdererAssemblerFinalizerName) {
		ordererAssembler.Finalizers = append(ordererAssembler.Finalizers, OrdererAssemblerFinalizerName)
		return r.Update(ctx, ordererAssembler)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererAssembler
func (r *OrdererAssemblerReconciler) removeFinalizer(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	ordererAssembler.Finalizers = utils.RemoveString(ordererAssembler.Finalizers, OrdererAssemblerFinalizerName)
	return r.Update(ctx, ordererAssembler)
}

// updateOrdererAssemblerStatus updates the OrdererAssembler status with the given status and message
func (r *OrdererAssemblerReconciler) updateOrdererAssemblerStatus(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating OrdererAssembler status",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace,
		"status", status,
		"message", message)

	// Update the status
	ordererAssembler.Status.Status = status
	ordererAssembler.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	ordererAssembler.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, ordererAssembler); err != nil {
		log.Error(err, "Failed to update OrdererAssembler status")
	} else {
		log.Info("OrdererAssembler status updated successfully",
			"name", ordererAssembler.Name,
			"namespace", ordererAssembler.Namespace,
			"status", status,
			"message", message)
	}
}

// reconcileService creates or updates the Service for Assembler
func (r *OrdererAssemblerReconciler) reconcileService(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getServiceName(ordererAssembler),
			Namespace: ordererAssembler.Namespace,
		},
	}

	template := r.getServiceTemplate(ordererAssembler)

	if err := r.updateService(ctx, ordererAssembler, service, template); err != nil {
		log.Error(err, "Failed to update Service", "name", service.Name)
		return fmt.Errorf("failed to update Service %s: %w", service.Name, err)
	}

	log.Info("Service reconciled successfully", "assembler", ordererAssembler.Name)
	return nil
}

func (r *OrdererAssemblerReconciler) getServiceName(ordererAssembler *fabricxv1alpha1.OrdererAssembler) string {
	return utils.GetServiceName(ordererAssembler.Name)
}

// getServicePort returns the service port for the assembler
func (r *OrdererAssemblerReconciler) getServicePort() int32 {
	return AssemblerServicePort
}

// getServiceTemplate returns the service template for the OrdererAssembler
func (r *OrdererAssemblerReconciler) getServiceTemplate(ordererAssembler *fabricxv1alpha1.OrdererAssembler) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getServiceName(ordererAssembler),
			Namespace: ordererAssembler.Namespace,
			Labels: map[string]string{
				"app":              "fabric-x",
				"ordererassembler": ordererAssembler.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":              "fabric-x",
				"ordererassembler": ordererAssembler.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:        "assembler",
					Protocol:    corev1.ProtocolTCP,
					Port:        7050,
					TargetPort:  intstr.FromInt32(7050),
					AppProtocol: func() *string { s := "h2c"; return &s }(),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// updateService updates a Service with the given template
func (r *OrdererAssemblerReconciler) updateService(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler, service *corev1.Service, template *corev1.Service) error {
	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererAssembler, template, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for service: %w", err)
	}

	// Try to create the Service
	if err := r.Create(ctx, template); err != nil {
		// If Service already exists, update it
		if strings.Contains(err.Error(), "already exists") {
			if err := r.Update(ctx, template); err != nil {
				return fmt.Errorf("failed to update Service: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create Service: %w", err)
		}
	}

	return nil
}

// reconcileGatewayGateway creates or updates the Gateway API Route for Assembler
func (r *OrdererAssemblerReconciler) reconcileGatewayGateway(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	// Check if Gateway configuration is provided
	if ordererAssembler.Spec.Ingress == nil || ordererAssembler.Spec.Ingress.Gateway == nil {
		log.Info("No Gateway configuration found, skipping route creation")
		return nil
	}

	// Determine if TLS is enabled
	tlsEnabled := false
	if ordererAssembler.Spec.TLS != nil {
		tlsEnabled = ordererAssembler.Spec.TLS.Enabled
	}

	gatewayConfig := ordererAssembler.Spec.Ingress.Gateway

	if tlsEnabled {
		// Create TLSRoute for TLS-enabled assemblers (uses SNI-based routing on port 443)
		log.Info("TLS enabled, creating TLSRoute", "assembler", ordererAssembler.Name)
		return ReconcileTLSRoute(ctx, r.Client, RouteConfig{
			Name:           fmt.Sprintf("%s-tlsroute", ordererAssembler.Name),
			Namespace:      ordererAssembler.Namespace,
			Hostnames:      gatewayConfig.Hosts,
			ServiceName:    r.getServiceName(ordererAssembler),
			ServicePort:    r.getServicePort(),
			IngressGateway: gatewayConfig.IngressGateway,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererassembler":         ordererAssembler.Name,
				"fabricx.kfsoft.tech/type": "tlsroute",
			},
			Owner:  ordererAssembler,
			Scheme: r.Scheme,
		})
	} else {
		// Create HTTPRoute for non-TLS assemblers (uses Host header-based routing on port 80)
		// gRPC uses HTTP/2, so HTTPRoute can route by hostname
		log.Info("TLS disabled, creating HTTPRoute for hostname-based routing on port 80", "assembler", ordererAssembler.Name)
		return ReconcileHTTPRoute(ctx, r.Client, RouteConfig{
			Name:           fmt.Sprintf("%s-httproute", ordererAssembler.Name),
			Namespace:      ordererAssembler.Namespace,
			Hostnames:      gatewayConfig.Hosts,
			ServiceName:    r.getServiceName(ordererAssembler),
			ServicePort:    r.getServicePort(),
			IngressGateway: gatewayConfig.IngressGateway,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererassembler":         ordererAssembler.Name,
				"fabricx.kfsoft.tech/type": "httproute",
			},
			Owner:  ordererAssembler,
			Scheme: r.Scheme,
		})
	}
}

// reconcileGatewayVirtualService is no longer needed with Gateway API - using TLSRoute only
func (r *OrdererAssemblerReconciler) reconcileGatewayVirtualService(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	// With Gateway API, we only need TLSRoute - no separate VirtualService
	return nil
}

// reconcileGatewayResources creates or updates Istio Gateway and VirtualService resources
func (r *OrdererAssemblerReconciler) reconcileGatewayResources(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	// Check if Istio native configuration is provided
	if ordererAssembler.Spec.Ingress != nil && ordererAssembler.Spec.Ingress.Istio != nil {
		log.Info("Istio native configuration found, creating VirtualService and DestinationRule")
		return r.reconcileIstioNativeResources(ctx, ordererAssembler)
	}

	// Check if Gateway API configuration is provided
	if ordererAssembler.Spec.Ingress == nil || ordererAssembler.Spec.Ingress.Gateway == nil {
		log.Info("No Gateway or Istio configuration found, skipping ingress resources")
		return nil
	}

	// Reconcile Gateway API resources (TLSRoute/HTTPRoute)
	if err := r.reconcileGatewayGateway(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile Gateway API route: %w", err)
	}

	log.Info("Gateway resources reconciled successfully")
	return nil
}

// reconcileIstioNativeResources creates or updates Istio VirtualService and DestinationRule
func (r *OrdererAssemblerReconciler) reconcileIstioNativeResources(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)
	istioConfig := ordererAssembler.Spec.Ingress.Istio

	// Prepare resource config
	resourceConfig := IstioResourceConfig{
		Name:        fmt.Sprintf("%s-vs", ordererAssembler.Name),
		Namespace:   ordererAssembler.Namespace,
		Hosts:       istioConfig.Hosts,
		ServiceName: r.getServiceName(ordererAssembler),
		ServicePort: 7050,
		Gateway:     istioConfig.Gateway,
		EnableHTTP2: istioConfig.EnableHTTP2,
		Labels: map[string]string{
			"app":                      "fabric-x",
			"ordererassembler":         ordererAssembler.Name,
			"fabricx.kfsoft.tech/type": "istio",
		},
		Owner:  ordererAssembler,
		Scheme: r.Scheme,
	}

	// Reconcile VirtualService
	if err := ReconcileIstioVirtualService(ctx, r.Client, resourceConfig); err != nil {
		log.Error(err, "Failed to reconcile VirtualService")
		return fmt.Errorf("failed to reconcile VirtualService: %w", err)
	}

	// Reconcile DestinationRule (for HTTP/2 support)
	resourceConfig.Name = fmt.Sprintf("%s-dr", ordererAssembler.Name)
	if err := ReconcileIstioDestinationRule(ctx, r.Client, resourceConfig); err != nil {
		log.Error(err, "Failed to reconcile DestinationRule")
		return fmt.Errorf("failed to reconcile DestinationRule: %w", err)
	}

	log.Info("Istio native resources reconciled successfully", "assembler", ordererAssembler.Name)
	return nil
}

// cleanupGatewayResources cleans up Gateway API TLSRoute resources
func (r *OrdererAssemblerReconciler) cleanupGatewayResources(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	// Check if Gateway configuration is provided
	if ordererAssembler.Spec.Ingress == nil || ordererAssembler.Spec.Ingress.Gateway == nil {
		return nil
	}

	// Use shared helper to delete TLSRoute
	return DeleteTLSRoute(ctx, r.Client, fmt.Sprintf("%s-tlsroute", ordererAssembler.Name), ordererAssembler.Namespace)
}

// reconcileConfigMap creates or updates the ConfigMap for Assembler
func (r *OrdererAssemblerReconciler) reconcileConfigMap(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ordererAssembler.Name),
			Namespace: ordererAssembler.Namespace,
		},
	}

	template := r.getConfigMapTemplate(ctx, ordererAssembler)

	if err := r.updateConfigMap(ctx, ordererAssembler, configMap, template); err != nil {
		log.Error(err, "Failed to update ConfigMap", "name", configMap.Name)
		return fmt.Errorf("failed to update ConfigMap %s: %w", configMap.Name, err)
	}

	log.Info("ConfigMap reconciled successfully", "assembler", ordererAssembler.Name)
	return nil
}

// getConfigMapTemplate returns the configmap template for the OrdererAssembler
func (r *OrdererAssemblerReconciler) getConfigMapTemplate(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ordererAssembler.Name),
			Namespace: ordererAssembler.Namespace,
			Labels: map[string]string{
				"app":              "fabric-x",
				"ordererassembler": ordererAssembler.Name,
			},
		},
		Data: map[string]string{
			"assembler.yaml": r.generateAssemblerConfig(ctx, ordererAssembler),
		},
	}
}

// generateAssemblerConfig generates the assembler configuration
func (r *OrdererAssemblerReconciler) generateAssemblerConfig(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) string {
	log := logf.FromContext(ctx)

	// Determine TLS enabled flag
	tlsEnabled := false
	if ordererAssembler.Spec.TLS != nil {
		tlsEnabled = ordererAssembler.Spec.TLS.Enabled
	}

	// Create template data for assembler
	templateData := utils.AssemblerTemplateData{
		Name:       ordererAssembler.Name,
		PartyID:    ordererAssembler.Spec.PartyID,
		MSPID:      ordererAssembler.Spec.MSPID,
		Port:       7050, // Assembler port
		TLSEnabled: tlsEnabled,
	}

	// Execute the template
	config, err := utils.ExecuteTemplate(utils.AssemblerConfigTemplate, templateData)
	if err != nil {
		log.Error(err, "Failed to generate Assembler configuration", "name", ordererAssembler.Name)
		// Fallback to basic config if template execution fails
		return fmt.Sprintf(`
# Assembler Configuration for %s
assembler:
  name: %s
  namespace: %s
  mspId: %s
  partyId: %d
  port: 7050
  tls:
    enabled: true
    certFile: /etc/hyperledger/fabricx/assembler/tls/server.crt
    keyFile: /etc/hyperledger/fabricx/assembler/tls/server.key
    caFile: /etc/hyperledger/fabricx/assembler/tls/ca.crt
`, ordererAssembler.Name, ordererAssembler.Name, ordererAssembler.Namespace, ordererAssembler.Spec.MSPID, ordererAssembler.Spec.PartyID)
	}

	return config
}

// updateConfigMap updates a ConfigMap with the given template
func (r *OrdererAssemblerReconciler) updateConfigMap(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler, configMap *corev1.ConfigMap, template *corev1.ConfigMap) error {
	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererAssembler, template, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for configmap: %w", err)
	}

	// Try to create the ConfigMap
	if err := r.Create(ctx, template); err != nil {
		// If ConfigMap already exists, update it
		if strings.Contains(err.Error(), "already exists") {
			if err := r.Update(ctx, template); err != nil {
				return fmt.Errorf("failed to update ConfigMap: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create ConfigMap: %w", err)
		}
	}

	return nil
}

// reconcileDeployment creates or updates the Deployment for Assembler
func (r *OrdererAssemblerReconciler) reconcileDeployment(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ordererAssembler.Name,
			Namespace: ordererAssembler.Namespace,
		},
	}

	template := r.getDeploymentTemplate(ctx, ordererAssembler)

	if err := r.updateDeployment(ctx, ordererAssembler, deployment, template); err != nil {
		log.Error(err, "Failed to update Deployment", "name", deployment.Name)
		return fmt.Errorf("failed to update Deployment %s: %w", deployment.Name, err)
	}

	log.Info("Deployment reconciled successfully", "assembler", ordererAssembler.Name)
	return nil
}

// getDeploymentTemplate returns the deployment template for the OrdererAssembler
func (r *OrdererAssemblerReconciler) getDeploymentTemplate(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) *appsv1.Deployment {
	replicas := int32(1)
	if ordererAssembler.Spec.Replicas > 0 {
		replicas = ordererAssembler.Spec.Replicas
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ordererAssembler.Name,
			Namespace: ordererAssembler.Namespace,
			Labels: map[string]string{
				"app":              "fabric-x",
				"ordererassembler": ordererAssembler.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":              "fabric-x",
					"ordererassembler": ordererAssembler.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":              "fabric-x",
						"ordererassembler": ordererAssembler.Name,
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "setup-msp",
							Image: "busybox:1.35",
							Command: []string{
								"/bin/sh",
								"-c",
								fmt.Sprintf(
									`mkdir -p %s/signcerts && `+
										"mkdir -p %s/keystore && "+
										"mkdir -p %s/cacerts && "+
										"mkdir -p %s && "+
										"cp /sign-certs/cert.pem %s/signcerts/ && "+
										"cp /sign-certs/key.pem %s/keystore/sign-privateKey.pem && "+
										"cp /sign-certs/ca.pem %s/cacerts/ && "+
										"cp /tls-certs/cert.pem %s/server.crt && "+
										"cp /tls-certs/key.pem %s/server.key && "+
										"cp /tls-certs/ca.pem %s/ca.crt",
									"/var/hyperledger/msp", "/var/hyperledger/msp", "/var/hyperledger/msp", "/etc/hyperledger/fabricx/assembler/tls",
									"/var/hyperledger/msp", "/var/hyperledger/msp", "/var/hyperledger/msp",
									"/etc/hyperledger/fabricx/assembler/tls", "/etc/hyperledger/fabricx/assembler/tls", "/etc/hyperledger/fabricx/assembler/tls",
								),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sign-certs",
									ReadOnly:  true,
									MountPath: "/sign-certs",
								},
								{
									Name:      "tls-certs",
									ReadOnly:  true,
									MountPath: "/tls-certs",
								},
								{
									Name:      "shared-msp",
									MountPath: "/var/hyperledger/msp",
								},
								{
									Name:      "shared-tls",
									MountPath: "/etc/hyperledger/fabricx/assembler/tls",
								},
							},
						},
						{
							Name:  "setup-genesis",
							Image: "busybox:1.35",
							Command: []string{
								"/bin/sh",
								"-c",
								fmt.Sprintf("cp /genesis-block/genesis.block %s/genesis.block", "/etc/hyperledger/fabricx/assembler/genesis"),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "genesis-block",
									ReadOnly:  true,
									MountPath: "/genesis-block",
								},
								{
									Name:      "shared-genesis",
									MountPath: "/etc/hyperledger/fabricx/assembler/genesis",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "assembler",
							Image: fmt.Sprintf("%s:%s",
								func() string {
									if ordererAssembler.Spec.Image != "" {
										return ordererAssembler.Spec.Image
									}
									return "hyperledger/fabric-x-orderer"
								}(),
								func() string {
									if ordererAssembler.Spec.ImageTag != "" {
										return ordererAssembler.Spec.ImageTag
									}
									return "0.0.19"
								}()),
							Args: []string{
								"assembler",
								"--config",
								"/etc/hyperledger/assembler/assembler.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "assembler",
									ContainerPort: 7050,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "ASSEMBLER_CONFIG",
									Value: "/etc/hyperledger/assembler/assembler.yaml",
								},
								{
									Name:  "ASSEMBLER_NAME",
									Value: ordererAssembler.Name,
								},
								{
									Name:  "ASSEMBLER_NAMESPACE",
									Value: ordererAssembler.Namespace,
								},
								{
									Name:  "MSP_ID",
									Value: ordererAssembler.Spec.MSPID,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/hyperledger/assembler",
								},
								{
									Name:      "shared-msp",
									MountPath: "/var/hyperledger/msp",
								},
								{
									Name:      "shared-tls",
									MountPath: "/etc/hyperledger/fabricx/assembler/tls",
								},
								{
									Name:      "shared-genesis",
									MountPath: "/etc/hyperledger/fabricx/assembler/genesis",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", ordererAssembler.Name),
									},
								},
							},
						},
						{
							Name: "sign-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-sign-cert", ordererAssembler.Name),
								},
							},
						},
						{
							Name: "tls-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-tls-cert", ordererAssembler.Name),
								},
							},
						},
						{
							Name: "shared-msp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "shared-tls",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "shared-genesis",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "genesis-block",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: ordererAssembler.Spec.Genesis.SecretName,
									Items: []corev1.KeyToPath{
										{
											Key: func() string {
												if ordererAssembler.Spec.Genesis.SecretKey != "" {
													return ordererAssembler.Spec.Genesis.SecretKey
												}
												return "genesis.block"
											}(),
											Path: "genesis.block",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// updateDeployment updates a Deployment with the given template
func (r *OrdererAssemblerReconciler) updateDeployment(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler, deployment *appsv1.Deployment, template *appsv1.Deployment) error {
	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererAssembler, template, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for deployment: %w", err)
	}

	// Try to create the Deployment
	if err := r.Create(ctx, template); err != nil {
		// If Deployment already exists, update it
		if strings.Contains(err.Error(), "already exists") {
			if err := r.Update(ctx, template); err != nil {
				return fmt.Errorf("failed to update Deployment: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create Deployment: %w", err)
		}
	}

	return nil
}

// reconcilePVC creates or updates the PVC for Assembler
func (r *OrdererAssemblerReconciler) reconcilePVC(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	// Check if storage is configured
	if ordererAssembler.Spec.Storage == nil {
		log.Info("No storage configuration found, skipping PVC creation")
		return nil
	}

	pvcName := fmt.Sprintf("%s-data-pvc", ordererAssembler.Name)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: ordererAssembler.Namespace,
			Labels: map[string]string{
				"app":              "fabric-x",
				"ordererassembler": ordererAssembler.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(ordererAssembler.Spec.Storage.Size),
				},
			},
			StorageClassName: &ordererAssembler.Spec.Storage.StorageClass,
		},
	}

	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererAssembler, pvc, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for PVC: %w", err)
	}

	if err := r.Create(ctx, pvc); err != nil {
		// If PVC already exists, update it
		if strings.Contains(err.Error(), "already exists") {
			if err := r.Update(ctx, pvc); err != nil {
				return fmt.Errorf("failed to update PVC: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create PVC: %w", err)
		}
	}

	log.Info("PVC reconciled successfully", "assembler", ordererAssembler.Name)
	return nil
}

// cleanupDeploymentResources cleans up deployment resources (ConfigMap, Deployment, PVC)
func (r *OrdererAssemblerReconciler) cleanupDeploymentResources(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	configMapName := fmt.Sprintf("%s-config", ordererAssembler.Name)
	deploymentName := ordererAssembler.Name
	pvcName := fmt.Sprintf("%s-data-pvc", ordererAssembler.Name)

	// Delete ConfigMap
	configMap := &corev1.ConfigMap{}
	configMap.SetName(configMapName)
	configMap.SetNamespace(ordererAssembler.Namespace)

	if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete ConfigMap", "name", configMapName)
		return err
	} else {
		log.Info("Deleted ConfigMap", "name", configMapName)
	}

	// Delete Deployment
	deployment := &appsv1.Deployment{}
	deployment.SetName(deploymentName)
	deployment.SetNamespace(ordererAssembler.Namespace)

	if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete Deployment", "name", deploymentName)
		return err
	} else {
		log.Info("Deleted Deployment", "name", deploymentName)
	}

	// Delete PVC
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.SetName(pvcName)
	pvc.SetNamespace(ordererAssembler.Namespace)

	if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete PVC", "name", pvcName)
		return err
	} else {
		log.Info("Deleted PVC", "name", pvcName)
	}

	log.Info("Deployment resources cleanup completed")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererAssemblerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register Gateway API types with the scheme
	if err := gatewayv1.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Gateway API v1 to scheme: %w", err)
	}
	if err := gatewayv1alpha2.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Gateway API v1alpha2 to scheme: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererAssembler{}).
		Named("ordererassembler").
		Complete(r)
}
