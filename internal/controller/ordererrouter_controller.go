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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// OrdererRouterFinalizerName is the name of the finalizer used by OrdererRouter
	OrdererRouterFinalizerName = "ordererrouter.fabricx.kfsoft.tech/finalizer"

	// Service and deployment constants
	RouterServicePort = 7150
	RouterTargetPort  = 7150
)

// Helper functions for consistent naming and port configuration

// getServiceName returns the service name for the router
func (r *OrdererRouterReconciler) getServiceName(ordererRouter *fabricxv1alpha1.OrdererRouter) string {
	return utils.GetServiceNameWithSuffix(ordererRouter.Name, "service")
}

// getDeploymentName returns the deployment name for the router
func (r *OrdererRouterReconciler) getDeploymentName(ordererRouter *fabricxv1alpha1.OrdererRouter) string {
	return ordererRouter.Name
}

// getServicePort returns the service port for the router
func (r *OrdererRouterReconciler) getServicePort() int32 {
	return RouterServicePort
}

// getTargetPort returns the target port for the router
func (r *OrdererRouterReconciler) getTargetPort() int {
	return RouterTargetPort
}

// getServiceFQDN returns the fully qualified domain name for the service
func (r *OrdererRouterReconciler) getServiceFQDN(ordererRouter *fabricxv1alpha1.OrdererRouter) string {
	return utils.GetServiceFQDNWithSuffix(ordererRouter.Name, "service", ordererRouter.Namespace)
}

// OrdererRouterReconciler reconciles a OrdererRouter object
type OrdererRouterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererrouters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererrouters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererrouters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OrdererRouterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererRouter reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the OrdererRouter status to failed
			ordererRouter := &fabricxv1alpha1.OrdererRouter{}
			if err := r.Get(ctx, req.NamespacedName, ordererRouter); err == nil {
				panicMsg := fmt.Sprintf("Panic in OrdererRouter reconciliation: %v", panicErr)
				r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var ordererRouter fabricxv1alpha1.OrdererRouter
	if err := r.Get(ctx, req.NamespacedName, &ordererRouter); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the OrdererRouter is being deleted
	if !ordererRouter.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &ordererRouter)
	}

	// Set initial status if not set
	if ordererRouter.Status.Status == "" {
		r.updateOrdererRouterStatus(ctx, &ordererRouter, fabricxv1alpha1.PendingStatus, "Initializing OrdererRouter")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &ordererRouter); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateOrdererRouterStatus(ctx, &ordererRouter, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the OrdererRouter
	if err := r.reconcileOrdererRouter(ctx, &ordererRouter); err != nil {
		// The reconcileOrdererRouter method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if ordererRouter.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile OrdererRouter: %v", err)
			r.updateOrdererRouterStatus(ctx, &ordererRouter, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile OrdererRouter")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute to ensure continuous monitoring
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileOrdererRouter handles the reconciliation of an OrdererRouter
func (r *OrdererRouterReconciler) reconcileOrdererRouter(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererRouter reconciliation",
				"ordererRouter", ordererRouter.Name, "namespace", ordererRouter.Namespace)

			// Update the OrdererRouter status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererRouter reconciliation: %v", panicErr)
			r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Starting OrdererRouter reconciliation",
		"name", ordererRouter.Name,
		"namespace", ordererRouter.Namespace,
		"bootstrapMode", ordererRouter.Spec.BootstrapMode)

	// Check bootstrap mode - only deploy when bootstrapMode is "deploy"
	bootstrapMode := ordererRouter.Spec.BootstrapMode
	if bootstrapMode == "" {
		bootstrapMode = "configure" // Default to configure mode
	}

	// Reconcile based on deployment mode
	switch ordererRouter.Spec.BootstrapMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, ordererRouter); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, ordererRouter); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid bootstrap mode: %s", ordererRouter.Spec.BootstrapMode)
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid bootstrap mode")
		r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	// Update status to success
	r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.RunningStatus, "OrdererRouter reconciled successfully")

	log.Info("OrdererRouter reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *OrdererRouterReconciler) reconcileConfigureMode(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererRouter in configure mode",
		"name", ordererRouter.Name,
		"namespace", ordererRouter.Namespace)

	// In configure mode, only create certificates
	if err := r.reconcileCertificates(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("OrdererRouter certificates created in configure mode")

	log.Info("OrdererRouter configure mode reconciliation completed")
	return nil
}

// reconcileCertificates creates or updates certificates for the OrdererRouter
func (r *OrdererRouterReconciler) reconcileCertificates(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if enrollment is configured
	if ordererRouter.Spec.Enrollment == nil {
		log.Info("No enrollment configuration found, skipping certificate creation")
		return nil
	}

	// Generate certificates for each type (each function handles its own existence check)
	var allCertificates []certs.ComponentCertificateData

	// Create sign certificate with component-specific SANS if available
	signCertConfig := &fabricxv1alpha1.CertificateConfig{
		CA: ordererRouter.Spec.Enrollment.Sign.CA,
	}

	signRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    ordererRouter.Name,
		ComponentType:    "router",
		Namespace:        ordererRouter.Namespace,
		OrdererGroupName: ordererRouter.Name, // Using router name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(ordererRouter.Spec.MSPID, signCertConfig, "sign"),
		EnrollmentConfig: r.convertToEnrollmentConfig(ordererRouter.Spec.MSPID, ordererRouter.Spec.Enrollment),
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
		CA: ordererRouter.Spec.Enrollment.TLS.CA,
	}
	// Use component-specific SANS if available, otherwise use enrollment SANS
	if ordererRouter.Spec.SANS != nil {
		tlsCertConfig.SANS = ordererRouter.Spec.SANS
	} else if ordererRouter.Spec.Enrollment.TLS.SANS != nil {
		tlsCertConfig.SANS = ordererRouter.Spec.Enrollment.TLS.SANS
	}

	tlsRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    ordererRouter.Name,
		ComponentType:    "router",
		Namespace:        ordererRouter.Namespace,
		OrdererGroupName: ordererRouter.Name, // Using router name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(ordererRouter.Spec.MSPID, tlsCertConfig, "tls"),
		EnrollmentConfig: r.convertToEnrollmentConfig(ordererRouter.Spec.MSPID, ordererRouter.Spec.Enrollment),
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
		if err := r.createCertificateSecrets(ctx, ordererRouter, allCertificates); err != nil {
			return fmt.Errorf("failed to create certificate secrets: %w", err)
		}
	}

	log.Info("Certificates reconciled successfully", "router", ordererRouter.Name)
	return nil
}

// createCertificateSecrets creates Kubernetes secrets for certificate data
func (r *OrdererRouterReconciler) createCertificateSecrets(
	ctx context.Context,
	ordererRouter *fabricxv1alpha1.OrdererRouter,
	certificates []certs.ComponentCertificateData,
) error {
	log := logf.FromContext(ctx)

	// Process each certificate in the slice
	for _, certData := range certificates {
		secretName := fmt.Sprintf("%s-%s-cert", ordererRouter.Name, certData.CertType)

		// Check if secret already exists
		existingSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: ordererRouter.Namespace,
		}, existingSecret)

		if err != nil {
			if errors.IsNotFound(err) {
				// Secret doesn't exist, create it
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: ordererRouter.Namespace,
						Labels: map[string]string{
							"app":                      "fabric-x",
							"ordererrouter":            ordererRouter.Name,
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
				if err := controllerutil.SetControllerReference(ordererRouter, secret, r.Scheme); err != nil {
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
				"ordererrouter":            ordererRouter.Name,
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

// convertRouterCertConfig converts API certificate config to internal format for router
func (r *OrdererRouterReconciler) convertToCertConfig(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig, certType string) *certs.CertificateConfig {
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

// convertToEnrollmentConfig converts API enrollment config to internal format
func (r *OrdererRouterReconciler) convertToEnrollmentConfig(mspID string, apiConfig *fabricxv1alpha1.EnrollmentConfig) *certs.EnrollmentConfig {
	if apiConfig == nil {
		return nil
	}

	config := &certs.EnrollmentConfig{}

	if apiConfig.Sign != nil {
		config.Sign = r.convertToCertConfig(mspID, apiConfig.Sign, "sign")
	}

	if apiConfig.TLS != nil {
		config.TLS = r.convertToCertConfig(mspID, apiConfig.TLS, "tls")
	}

	return config
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *OrdererRouterReconciler) reconcileDeployMode(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererRouter in deploy mode",
		"name", ordererRouter.Name,
		"namespace", ordererRouter.Namespace,
		"partyID", ordererRouter.Spec.PartyID,
		"bootstrapMode", ordererRouter.Spec.BootstrapMode)

	// Check if bootstrap mode is set to deploy
	if ordererRouter.Spec.BootstrapMode != "deploy" {
		log.Info("Bootstrap mode is not 'deploy', skipping deployment resources",
			"bootstrapMode", ordererRouter.Spec.BootstrapMode)
		return nil
	}

	// 1. Create/Update certificates first
	if err := r.reconcileCertificates(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// 2. Create/Update Service for Router
	if err := r.reconcileService(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile service: %w", err)
	}

	// 3. Create/Update Gateway resources
	if err := r.reconcileGatewayResources(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile Gateway resources: %w", err)
	}

	// 4. Create/Update ConfigMap for Router configuration
	if err := r.reconcileConfigMap(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile configmap: %w", err)
	}

	// 5. Create/Update Deployment for Router
	if err := r.reconcileDeployment(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile deployment: %w", err)
	}

	// 6. Create/Update PVC if needed
	if err := r.reconcilePVC(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile PVC: %w", err)
	}

	// 7. Create/Update Ingress if configured
	if err := r.reconcileIngress(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile ingress: %w", err)
	}

	log.Info("OrdererRouter deploy mode reconciliation completed")
	return nil
}

// handleDeletion handles the deletion of an OrdererRouter
func (r *OrdererRouterReconciler) handleDeletion(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererRouter deletion",
				"ordererRouter", ordererRouter.Name, "namespace", ordererRouter.Namespace)

			// Update the OrdererRouter status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererRouter deletion: %v", panicErr)
			r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Handling OrdererRouter deletion",
		"name", ordererRouter.Name,
		"namespace", ordererRouter.Namespace)

	// Set status to indicate deletion
	r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.PendingStatus, "Deleting OrdererRouter resources")

	// Clean up Gateway resources if they exist
	if ordererRouter.Spec.Ingress != nil && ordererRouter.Spec.Ingress.Gateway != nil {
		if err := r.cleanupGatewayResources(ctx, ordererRouter); err != nil {
			log.Error(err, "Failed to cleanup Gateway resources")
		}
	}

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, ordererRouter); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("OrdererRouter deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the OrdererRouter
func (r *OrdererRouterReconciler) ensureFinalizer(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	if !utils.ContainsString(ordererRouter.Finalizers, OrdererRouterFinalizerName) {
		ordererRouter.Finalizers = append(ordererRouter.Finalizers, OrdererRouterFinalizerName)
		return r.Update(ctx, ordererRouter)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererRouter
func (r *OrdererRouterReconciler) removeFinalizer(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	ordererRouter.Finalizers = utils.RemoveString(ordererRouter.Finalizers, OrdererRouterFinalizerName)
	return r.Update(ctx, ordererRouter)
}

// updateOrdererRouterStatus updates the OrdererRouter status with the given status and message
func (r *OrdererRouterReconciler) updateOrdererRouterStatus(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating OrdererRouter status",
		"name", ordererRouter.Name,
		"namespace", ordererRouter.Namespace,
		"status", status,
		"message", message)

	// Update the status
	ordererRouter.Status.Status = status
	ordererRouter.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	ordererRouter.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, ordererRouter); err != nil {
		log.Error(err, "Failed to update OrdererRouter status")
	} else {
		log.Info("OrdererRouter status updated successfully",
			"name", ordererRouter.Name,
			"namespace", ordererRouter.Namespace,
			"status", status,
			"message", message)
	}
}

// reconcileConfigMap creates or updates the ConfigMap for Router configuration
func (r *OrdererRouterReconciler) reconcileConfigMap(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ordererRouter.Name),
			Namespace: ordererRouter.Namespace,
		},
	}

	// Determine TLS enabled flag
	tlsEnabled := false
	if ordererRouter.Spec.TLS != nil {
		tlsEnabled = ordererRouter.Spec.TLS.Enabled
	}

	// Prepare template data
	templateData := utils.RouterTemplateData{
		Name:       ordererRouter.Name,
		PartyID:    ordererRouter.Spec.PartyID,
		MSPID:      ordererRouter.Spec.MSPID,
		Port:       r.getServicePort(),
		TLSEnabled: tlsEnabled,
	}

	// Execute the template using the shared utility
	configContent, err := utils.ExecuteTemplateWithValidation(utils.RouterConfigTemplate, templateData)
	if err != nil {
		return fmt.Errorf("failed to execute router config template: %w", err)
	}

	template := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ordererRouter.Name),
			Namespace: ordererRouter.Namespace,
		},
		Data: map[string]string{
			"node_config.yaml": configContent,
		},
	}

	if err := r.updateConfigMap(ctx, ordererRouter, configMap, template); err != nil {
		log.Error(err, "Failed to update ConfigMap", "name", configMap.Name)
		return fmt.Errorf("failed to update ConfigMap %s: %w", configMap.Name, err)
	}

	log.Info("ConfigMap reconciled successfully", "router", ordererRouter.Name)
	return nil
}

// updateConfigMap updates a ConfigMap with the given template
func (r *OrdererRouterReconciler) updateConfigMap(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter, configMap *corev1.ConfigMap, template *corev1.ConfigMap) error {
	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererRouter, template, r.Scheme); err != nil {
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

// reconcileService creates or updates the Service for Router
func (r *OrdererRouterReconciler) reconcileService(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getServiceName(ordererRouter),
			Namespace: ordererRouter.Namespace,
		},
	}

	template := r.getServiceTemplate(ordererRouter)

	if err := r.updateService(ctx, ordererRouter, service, template); err != nil {
		log.Error(err, "Failed to update Service", "name", service.Name)
		return fmt.Errorf("failed to update Service %s: %w", service.Name, err)
	}

	log.Info("Service reconciled successfully", "router", ordererRouter.Name)
	return nil
}

// getServiceTemplate returns the service template for the OrdererRouter
func (r *OrdererRouterReconciler) getServiceTemplate(ordererRouter *fabricxv1alpha1.OrdererRouter) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getServiceName(ordererRouter),
			Namespace: ordererRouter.Namespace,
			Labels: map[string]string{
				"app":           "fabric-x",
				"ordererrouter": ordererRouter.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":           "fabric-x",
				"ordererrouter": ordererRouter.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:        "router",
					Protocol:    corev1.ProtocolTCP,
					Port:        r.getServicePort(),
					TargetPort:  intstr.FromInt(r.getTargetPort()),
					AppProtocol: func() *string { s := "h2c"; return &s }(),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// updateService updates a Service with the given template
func (r *OrdererRouterReconciler) updateService(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter, service *corev1.Service, template *corev1.Service) error {
	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererRouter, template, r.Scheme); err != nil {
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

// reconcileDeployment creates or updates the Deployment for Router
func (r *OrdererRouterReconciler) reconcileDeployment(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getDeploymentName(ordererRouter),
			Namespace: ordererRouter.Namespace,
		},
	}

	template := r.getDeploymentTemplate(ctx, ordererRouter)

	if err := r.updateDeployment(ctx, ordererRouter, deployment, template); err != nil {
		log.Error(err, "Failed to update Deployment", "name", deployment.Name)
		return fmt.Errorf("failed to update Deployment %s: %w", deployment.Name, err)
	}

	log.Info("Deployment reconciled successfully", "router", ordererRouter.Name)
	return nil
}

// getDeploymentTemplate returns the deployment template for the OrdererRouter
func (r *OrdererRouterReconciler) getDeploymentTemplate(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) *appsv1.Deployment {
	replicas := int32(1)
	if ordererRouter.Spec.Replicas > 0 {
		replicas = ordererRouter.Spec.Replicas
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getDeploymentName(ordererRouter),
			Namespace: ordererRouter.Namespace,
			Labels: map[string]string{
				"app":           "fabric-x",
				"ordererrouter": ordererRouter.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":           "fabric-x",
					"ordererrouter": ordererRouter.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":           "fabric-x",
						"ordererrouter": ordererRouter.Name,
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
										"cp /tls-certs/ca.pem %s/ca.crt && "+
										`cat > %s/config.yaml <<'EOF'
NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: orderer
EOF`,
									"/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/tls",
									"/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/msp",
									"/etc/hyperledger/fabricx/router/tls", "/etc/hyperledger/fabricx/router/tls", "/etc/hyperledger/fabricx/router/tls",
									"/etc/hyperledger/fabricx/router/msp",
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
									MountPath: "/etc/hyperledger/fabricx/router/msp",
								},
								{
									Name:      "shared-tls",
									MountPath: "/etc/hyperledger/fabricx/router/tls",
								},
							},
						},
						{
							Name:  "setup-genesis",
							Image: "busybox:1.35",
							Command: []string{
								"/bin/sh",
								"-c",
								fmt.Sprintf("cp /genesis-block/genesis.block %s/genesis.block", "/etc/hyperledger/fabricx/router/genesis"),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "genesis-block",
									ReadOnly:  true,
									MountPath: "/genesis-block",
								},
								{
									Name:      "shared-genesis",
									MountPath: "/etc/hyperledger/fabricx/router/genesis",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "router",
							Image: fmt.Sprintf("%s:%s",
								func() string {
									if ordererRouter.Spec.Image != "" {
										return ordererRouter.Spec.Image
									}
									return "hyperledger/fabric-x-orderer"
								}(),
								func() string {
									if ordererRouter.Spec.ImageTag != "" {
										return ordererRouter.Spec.ImageTag
									}
									return "0.0.24"
								}()),
							Args: []string{
								"router",
								"--config=/etc/hyperledger/fabricx/router/config/node_config.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "router",
									ContainerPort: int32(r.getTargetPort()),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									ReadOnly:  true,
									MountPath: "/etc/hyperledger/fabricx/router/config",
								},
								{
									Name:      "shared-msp",
									MountPath: "/etc/hyperledger/fabricx/router/msp",
								},
								{
									Name:      "shared-tls",
									MountPath: "/etc/hyperledger/fabricx/router/tls",
								},
								{
									Name:      "shared-genesis",
									MountPath: "/etc/hyperledger/fabricx/router/genesis",
								},
								{
									Name:      "router-store",
									MountPath: "/etc/hyperledger/fabricx/router/store",
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
										Name: fmt.Sprintf("%s-config", ordererRouter.Name),
									},
								},
							},
						},
						{
							Name: "sign-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-sign-cert", ordererRouter.Name),
								},
							},
						},
						{
							Name: "tls-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-tls-cert", ordererRouter.Name),
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
							Name: "router-store",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "genesis-block",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: ordererRouter.Spec.Genesis.SecretName,
									Items: []corev1.KeyToPath{
										{
											Key:  ordererRouter.Spec.Genesis.SecretKey,
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
func (r *OrdererRouterReconciler) updateDeployment(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter, deployment *appsv1.Deployment, template *appsv1.Deployment) error {
	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererRouter, template, r.Scheme); err != nil {
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

// reconcilePVC creates or updates the PVC for Router
func (r *OrdererRouterReconciler) reconcilePVC(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if storage is configured
	if ordererRouter.Spec.Storage == nil {
		log.Info("No storage configuration found, skipping PVC creation")
		return nil
	}

	pvcName := fmt.Sprintf("%s-data-pvc", ordererRouter.Name)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: ordererRouter.Namespace,
			Labels: map[string]string{
				"app":           "fabric-x",
				"ordererrouter": ordererRouter.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(ordererRouter.Spec.Storage.Size),
				},
			},
			StorageClassName: func() *string {
				if ordererRouter.Spec.Storage.StorageClass != "" {
					return &ordererRouter.Spec.Storage.StorageClass
				}
				return nil
			}(),
		},
	}

	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererRouter, pvc, r.Scheme); err != nil {
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

	log.Info("PVC reconciled successfully", "router", ordererRouter.Name)
	return nil
}

// reconcileIngress creates or updates the Ingress for Router
func (r *OrdererRouterReconciler) reconcileIngress(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if ingress is configured
	if ordererRouter.Spec.Ingress == nil {
		log.Info("No ingress configuration found, skipping ingress creation")
		return nil
	}

	// TODO: Implement ingress creation based on the IngressConfig
	// This would create Istio Gateway and VirtualService resources

	log.Info("Ingress reconciliation completed", "router", ordererRouter.Name)
	return nil
}

// reconcileGatewayGateway creates or updates the Gateway API Route for Router
func (r *OrdererRouterReconciler) reconcileGatewayGateway(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if Gateway configuration is provided
	if ordererRouter.Spec.Ingress == nil || ordererRouter.Spec.Ingress.Gateway == nil {
		log.Info("No Gateway configuration found, skipping route creation")
		return nil
	}

	// Determine if TLS is enabled
	tlsEnabled := false
	if ordererRouter.Spec.TLS != nil {
		tlsEnabled = ordererRouter.Spec.TLS.Enabled
	}

	gatewayConfig := ordererRouter.Spec.Ingress.Gateway

	if tlsEnabled {
		// Create TLSRoute for TLS-enabled routers (uses SNI-based routing on port 443)
		log.Info("TLS enabled, creating TLSRoute", "router", ordererRouter.Name)
		return ReconcileTLSRoute(ctx, r.Client, RouteConfig{
			Name:           fmt.Sprintf("%s-tlsroute", ordererRouter.Name),
			Namespace:      ordererRouter.Namespace,
			Hostnames:      gatewayConfig.Hosts,
			ServiceName:    r.getServiceName(ordererRouter),
			ServicePort:    r.getServicePort(),
			IngressGateway: gatewayConfig.IngressGateway,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererrouter":            ordererRouter.Name,
				"fabricx.kfsoft.tech/type": "tlsroute",
			},
			Owner:  ordererRouter,
			Scheme: r.Scheme,
		})
	} else {
		// Create HTTPRoute for non-TLS routers (uses Host header-based routing on port 80)
		// gRPC uses HTTP/2, so HTTPRoute can route by hostname
		log.Info("TLS disabled, creating HTTPRoute for hostname-based routing on port 80", "router", ordererRouter.Name)
		return ReconcileHTTPRoute(ctx, r.Client, RouteConfig{
			Name:           fmt.Sprintf("%s-httproute", ordererRouter.Name),
			Namespace:      ordererRouter.Namespace,
			Hostnames:      gatewayConfig.Hosts,
			ServiceName:    r.getServiceName(ordererRouter),
			ServicePort:    r.getServicePort(),
			IngressGateway: gatewayConfig.IngressGateway,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererrouter":            ordererRouter.Name,
				"fabricx.kfsoft.tech/type": "httproute",
			},
			Owner:  ordererRouter,
			Scheme: r.Scheme,
		})
	}
}

// reconcileGatewayVirtualService is no longer needed with Gateway API - using TLSRoute only
func (r *OrdererRouterReconciler) reconcileGatewayVirtualService(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	// With Gateway API, we only need TLSRoute - no separate VirtualService
	return nil
}

// reconcileGatewayResources creates or updates Istio Gateway and VirtualService resources
func (r *OrdererRouterReconciler) reconcileGatewayResources(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if Istio native configuration is provided
	if ordererRouter.Spec.Ingress != nil && ordererRouter.Spec.Ingress.Istio != nil {
		log.Info("Istio native configuration found, creating VirtualService and DestinationRule")
		return r.reconcileIstioNativeResources(ctx, ordererRouter)
	}

	// Check if Gateway API configuration is provided
	if ordererRouter.Spec.Ingress == nil || ordererRouter.Spec.Ingress.Gateway == nil {
		log.Info("No Gateway or Istio configuration found, skipping ingress resources")
		return nil
	}

	// Reconcile Gateway API resources (TLSRoute/HTTPRoute)
	if err := r.reconcileGatewayGateway(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile Gateway API route: %w", err)
	}

	log.Info("Gateway resources reconciled successfully")
	return nil
}

// reconcileIstioNativeResources creates or updates Istio VirtualService and DestinationRule
func (r *OrdererRouterReconciler) reconcileIstioNativeResources(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)
	istioConfig := ordererRouter.Spec.Ingress.Istio

	// Prepare resource config
	resourceConfig := IstioResourceConfig{
		Name:        fmt.Sprintf("%s-vs", ordererRouter.Name),
		Namespace:   ordererRouter.Namespace,
		Hosts:       istioConfig.Hosts,
		ServiceName: r.getServiceName(ordererRouter),
		ServicePort: r.getServicePort(),
		Gateway:     istioConfig.Gateway,
		EnableHTTP2: istioConfig.EnableHTTP2,
		Labels: map[string]string{
			"app":                      "fabric-x",
			"ordererrouter":            ordererRouter.Name,
			"fabricx.kfsoft.tech/type": "istio",
		},
		Owner:  ordererRouter,
		Scheme: r.Scheme,
	}

	// Reconcile VirtualService
	if err := ReconcileIstioVirtualService(ctx, r.Client, resourceConfig); err != nil {
		log.Error(err, "Failed to reconcile VirtualService")
		return fmt.Errorf("failed to reconcile VirtualService: %w", err)
	}

	// Reconcile DestinationRule (for HTTP/2 support)
	resourceConfig.Name = fmt.Sprintf("%s-dr", ordererRouter.Name)
	if err := ReconcileIstioDestinationRule(ctx, r.Client, resourceConfig); err != nil {
		log.Error(err, "Failed to reconcile DestinationRule")
		return fmt.Errorf("failed to reconcile DestinationRule: %w", err)
	}

	log.Info("Istio native resources reconciled successfully", "router", ordererRouter.Name)
	return nil
}

// cleanupGatewayResources cleans up Gateway API TLSRoute resources
func (r *OrdererRouterReconciler) cleanupGatewayResources(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	// Check if Gateway configuration is provided
	if ordererRouter.Spec.Ingress == nil || ordererRouter.Spec.Ingress.Gateway == nil {
		return nil
	}

	// Use shared helper to delete TLSRoute
	return DeleteTLSRoute(ctx, r.Client, fmt.Sprintf("%s-tlsroute", ordererRouter.Name), ordererRouter.Namespace)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererRouterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register Gateway API types with the scheme
	if err := gatewayv1.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Gateway API v1 to scheme: %w", err)
	}
	if err := gatewayv1alpha2.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Gateway API v1alpha2 to scheme: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererRouter{}).
		Named("ordererrouter").
		Complete(r)
}
