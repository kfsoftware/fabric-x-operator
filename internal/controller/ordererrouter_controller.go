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
	"strings"

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

	// Istio imports
	istioapinetworkingv1alpha3 "istio.io/api/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"

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
	return fmt.Sprintf("%s-service", ordererRouter.Name)
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
	return fmt.Sprintf("%s.%s.svc.cluster.local", r.getServiceName(ordererRouter), ordererRouter.Namespace)
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
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete

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

	return ctrl.Result{}, nil
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

	// Check if certificates are configured
	if ordererRouter.Spec.Certificates == nil {
		log.Info("No certificate configuration found, skipping certificate creation")
		return nil
	}

	// Create certificate request for this router instance
	request := certs.OrdererGroupCertificateRequest{
		ComponentName:    ordererRouter.Name,
		ComponentType:    "router",
		Namespace:        ordererRouter.Namespace,
		OrdererGroupName: ordererRouter.Name, // Using router name as orderer group name for individual instances
		CertConfig:       r.convertRouterCertConfig(ordererRouter.Spec.MSPID, ordererRouter.Spec.Certificates),
		EnrollmentConfig: nil, // Individual routers don't have global enrollment config
		CertTypes:        []string{"sign", "tls"},
		EnrollID:         ordererRouter.Spec.Certificates.EnrollID,
		EnrollSecret:     ordererRouter.Spec.Certificates.EnrollSecret,
	}

	// Provision certificates using the certificate service
	certificates, err := certs.ProvisionOrdererGroupCertificatesWithClient(ctx, r.Client, request)
	if err != nil {
		return fmt.Errorf("failed to provision certificates: %w", err)
	}

	// Create Kubernetes secrets for the certificates
	if err := r.createCertificateSecrets(ctx, ordererRouter, certificates); err != nil {
		return fmt.Errorf("failed to create certificate secrets: %w", err)
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

		if err := r.Client.Create(ctx, secret); err != nil {
			// If secret already exists, update it
			if strings.Contains(err.Error(), "already exists") {
				if err := r.Client.Update(ctx, secret); err != nil {
					return fmt.Errorf("failed to update certificate secret %s: %w", secretName, err)
				}
			} else {
				return fmt.Errorf("failed to create certificate secret %s: %w", secretName, err)
			}
		}

		log.Info("Created certificate secret", "secret", secretName, "certType", certData.CertType)
	}

	return nil
}

// convertRouterCertConfig converts API certificate config to internal format for router
func (r *OrdererRouterReconciler) convertRouterCertConfig(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig) *certs.CertificateConfig {
	if apiConfig == nil {
		return nil
	}

	config := &certs.CertificateConfig{
		CAHost:       apiConfig.CAHost,
		CAName:       apiConfig.CAName,
		CAPort:       apiConfig.CAPort,
		EnrollID:     apiConfig.EnrollID,
		EnrollSecret: apiConfig.EnrollSecret,
		MSPID:        mspID,
	}

	if apiConfig.CATLS != nil {
		config.CATLS = &certs.CATLSConfig{
			CACert: apiConfig.CATLS.CACert,
		}

		if apiConfig.CATLS.SecretRef != nil {
			config.CATLS.SecretRef = &certs.SecretRef{
				Name:      apiConfig.CATLS.SecretRef.Name,
				Key:       apiConfig.CATLS.SecretRef.Key,
				Namespace: apiConfig.CATLS.SecretRef.Namespace,
			}
		}
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

	// 3. Create/Update Istio resources
	if err := r.reconcileIstioResources(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile Istio resources: %w", err)
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

	// Clean up Istio resources if they exist
	if ordererRouter.Spec.Ingress != nil && ordererRouter.Spec.Ingress.Istio != nil {
		if err := r.cleanupIstioResources(ctx, ordererRouter); err != nil {
			log.Error(err, "Failed to cleanup Istio resources")
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

	// Prepare template data
	templateData := utils.RouterTemplateData{
		Name:    ordererRouter.Name,
		PartyID: ordererRouter.Spec.PartyID,
		MSPID:   ordererRouter.Spec.MSPID,
		Port:    r.getServicePort(),
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
	if err := r.Client.Create(ctx, template); err != nil {
		// If ConfigMap already exists, update it
		if strings.Contains(err.Error(), "already exists") {
			if err := r.Client.Update(ctx, template); err != nil {
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
					Name:       "router",
					Protocol:   corev1.ProtocolTCP,
					Port:       r.getServicePort(),
					TargetPort: intstr.FromInt(r.getTargetPort()),
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
	if err := r.Client.Create(ctx, template); err != nil {
		// If Service already exists, update it
		if strings.Contains(err.Error(), "already exists") {
			if err := r.Client.Update(ctx, template); err != nil {
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
										"cp /tls-certs/ca.pem %s/ca.crt",
									"/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/tls",
									"/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/msp", "/etc/hyperledger/fabricx/router/msp",
									"/etc/hyperledger/fabricx/router/tls", "/etc/hyperledger/fabricx/router/tls", "/etc/hyperledger/fabricx/router/tls",
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
							Name:  "router",
							Image: "hyperledger/fabric-x-orderer:0.0.17",
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
	if err := r.Client.Create(ctx, template); err != nil {
		// If Deployment already exists, update it
		if strings.Contains(err.Error(), "already exists") {
			if err := r.Client.Update(ctx, template); err != nil {
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
			StorageClassName: &ordererRouter.Spec.Storage.StorageClass,
		},
	}

	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererRouter, pvc, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for PVC: %w", err)
	}

	if err := r.Client.Create(ctx, pvc); err != nil {
		// If PVC already exists, update it
		if strings.Contains(err.Error(), "already exists") {
			if err := r.Client.Update(ctx, pvc); err != nil {
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

// reconcileIstioGateway creates or updates the Istio Gateway for Router
func (r *OrdererRouterReconciler) reconcileIstioGateway(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if Istio configuration is provided
	if ordererRouter.Spec.Ingress == nil || ordererRouter.Spec.Ingress.Istio == nil {
		log.Info("No Istio configuration found, skipping Gateway creation")
		return nil
	}

	istioConfig := ordererRouter.Spec.Ingress.Istio
	gatewayName := fmt.Sprintf("%s-gateway", ordererRouter.Name)

	// Create Gateway resource template
	gatewayTemplate := &istionetworkingv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: ordererRouter.Namespace,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererrouter":            ordererRouter.Name,
				"fabricx.kfsoft.tech/type": "gateway",
			},
		},
		Spec: istioapinetworkingv1alpha3.Gateway{
			Selector: map[string]string{
				"istio": istioConfig.IngressGateway,
			},
			Servers: []*istioapinetworkingv1alpha3.Server{
				{
					Port: &istioapinetworkingv1alpha3.Port{
						Number:   uint32(istioConfig.Port),
						Name:     "tls",
						Protocol: "TLS",
					},
					Hosts: istioConfig.Hosts,
					Tls: &istioapinetworkingv1alpha3.ServerTLSSettings{
						Mode: istioapinetworkingv1alpha3.ServerTLSSettings_PASSTHROUGH,
					},
				},
			},
		},
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(ordererRouter, gatewayTemplate, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for Gateway: %w", err)
	}

	// Check if Gateway already exists
	existingGateway := &istionetworkingv1beta1.Gateway{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      gatewayName,
		Namespace: ordererRouter.Namespace,
	}, existingGateway)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new Gateway
			if err := r.Client.Create(ctx, gatewayTemplate); err != nil {
				return fmt.Errorf("failed to create Gateway: %w", err)
			}
			log.Info("Created Istio Gateway", "gateway", gatewayName)
		} else {
			return fmt.Errorf("failed to get existing Gateway: %w", err)
		}
	} else {
		// Update existing Gateway - always update to ensure it's current
		existingGateway.Spec = gatewayTemplate.Spec
		existingGateway.Labels = gatewayTemplate.Labels
		if err := r.Client.Update(ctx, existingGateway); err != nil {
			return fmt.Errorf("failed to update Gateway: %w", err)
		}
		log.Info("Updated Istio Gateway", "gateway", gatewayName)
	}

	log.Info("Istio Gateway reconciled successfully", "gateway", gatewayName)
	return nil
}

// reconcileIstioVirtualService creates or updates the Istio VirtualService for Router
func (r *OrdererRouterReconciler) reconcileIstioVirtualService(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if Istio configuration is provided
	if ordererRouter.Spec.Ingress == nil || ordererRouter.Spec.Ingress.Istio == nil {
		log.Info("No Istio configuration found, skipping VirtualService creation")
		return nil
	}

	istioConfig := ordererRouter.Spec.Ingress.Istio
	virtualServiceName := fmt.Sprintf("%s-virtualservice", ordererRouter.Name)
	gatewayName := fmt.Sprintf("%s-gateway", ordererRouter.Name)

	// Create VirtualService resource template
	virtualServiceTemplate := &istionetworkingv1beta1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtualServiceName,
			Namespace: ordererRouter.Namespace,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererrouter":            ordererRouter.Name,
				"fabricx.kfsoft.tech/type": "virtualservice",
			},
		},
		Spec: istioapinetworkingv1alpha3.VirtualService{
			Hosts:    istioConfig.Hosts,
			Gateways: []string{gatewayName},
			Tls: []*istioapinetworkingv1alpha3.TLSRoute{
				{
					Match: []*istioapinetworkingv1alpha3.TLSMatchAttributes{
						{
							Port:     uint32(istioConfig.Port),
							SniHosts: istioConfig.Hosts,
						},
					},
					Route: []*istioapinetworkingv1alpha3.RouteDestination{
						{
							Destination: &istioapinetworkingv1alpha3.Destination{
								Host: r.getServiceFQDN(ordererRouter),
								Port: &istioapinetworkingv1alpha3.PortSelector{
									Number: uint32(r.getServicePort()),
								},
							},
							Weight: 100,
						},
					},
				},
			},
		},
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(ordererRouter, virtualServiceTemplate, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for VirtualService: %w", err)
	}

	// Check if VirtualService already exists
	existingVirtualService := &istionetworkingv1beta1.VirtualService{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      virtualServiceName,
		Namespace: ordererRouter.Namespace,
	}, existingVirtualService)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new VirtualService
			if err := r.Client.Create(ctx, virtualServiceTemplate); err != nil {
				return fmt.Errorf("failed to create VirtualService: %w", err)
			}
			log.Info("Created Istio VirtualService", "virtualService", virtualServiceName)
		} else {
			return fmt.Errorf("failed to get existing VirtualService: %w", err)
		}
	} else {
		// Update existing VirtualService - always update to ensure it's current
		existingVirtualService.Spec = virtualServiceTemplate.Spec
		existingVirtualService.Labels = virtualServiceTemplate.Labels
		if err := r.Client.Update(ctx, existingVirtualService); err != nil {
			return fmt.Errorf("failed to update VirtualService: %w", err)
		}
		log.Info("Updated Istio VirtualService", "virtualService", virtualServiceName)
	}

	log.Info("Istio VirtualService reconciled successfully", "virtualService", virtualServiceName)
	return nil
}

// reconcileIstioResources creates or updates Istio Gateway and VirtualService resources
func (r *OrdererRouterReconciler) reconcileIstioResources(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if Istio configuration is provided
	if ordererRouter.Spec.Ingress == nil || ordererRouter.Spec.Ingress.Istio == nil {
		log.Info("No Istio configuration found, skipping Istio resources")
		return nil
	}

	// Reconcile Gateway
	if err := r.reconcileIstioGateway(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile Istio Gateway: %w", err)
	}

	// Reconcile VirtualService
	if err := r.reconcileIstioVirtualService(ctx, ordererRouter); err != nil {
		return fmt.Errorf("failed to reconcile Istio VirtualService: %w", err)
	}

	log.Info("Istio resources reconciled successfully")
	return nil
}

// cleanupIstioResources cleans up Istio Gateway and VirtualService resources
func (r *OrdererRouterReconciler) cleanupIstioResources(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	// Check if Istio configuration is provided
	if ordererRouter.Spec.Ingress == nil || ordererRouter.Spec.Ingress.Istio == nil {
		log.Info("No Istio configuration found, skipping Istio resources cleanup")
		return nil
	}

	gatewayName := fmt.Sprintf("%s-gateway", ordererRouter.Name)
	virtualServiceName := fmt.Sprintf("%s-virtualservice", ordererRouter.Name)

	// Delete Gateway
	gateway := &istionetworkingv1beta1.Gateway{}
	gateway.SetName(gatewayName)
	gateway.SetNamespace(ordererRouter.Namespace)

	if err := r.Client.Delete(ctx, gateway); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete Istio Gateway", "name", gatewayName)
	} else {
		log.Info("Deleted Istio Gateway", "name", gatewayName)
	}

	// Delete VirtualService
	virtualService := &istionetworkingv1beta1.VirtualService{}
	virtualService.SetName(virtualServiceName)
	virtualService.SetNamespace(ordererRouter.Namespace)

	if err := r.Client.Delete(ctx, virtualService); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete Istio VirtualService", "name", virtualServiceName)
	} else {
		log.Info("Deleted Istio VirtualService", "name", virtualServiceName)
	}

	log.Info("Istio resources cleanup completed")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererRouterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register Istio types with the scheme
	if err := istionetworkingv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Istio networking v1beta1 to scheme: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererRouter{}).
		Named("ordererrouter").
		Complete(r)
}
