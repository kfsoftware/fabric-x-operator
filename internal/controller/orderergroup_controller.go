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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/ordgroup"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

const (
	// FinalizerName is the name of the finalizer used by OrdererGroup
	FinalizerName = "orderergroup.fabricx.kfsoft.tech/finalizer"
)

// OrdererGroupReconciler reconciles a OrdererGroup object
type OrdererGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Component controllers
	AssemblerController *ordgroup.AssemblerController
	BatcherController   *ordgroup.BatcherController
	RouterController    *ordgroup.RouterController
	ConsenterController *ordgroup.ConsenterController

	// Certificate service
	CertService certs.OrdererGroupCertServiceInterface
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OrdererGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererGroup reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the OrdererGroup status to failed
			ordererGroup := &fabricxv1alpha1.OrdererGroup{}
			if err := r.Get(ctx, req.NamespacedName, ordererGroup); err == nil {
				panicMsg := fmt.Sprintf("Panic in OrdererGroup reconciliation: %v", panicErr)
				r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var ordererGroup fabricxv1alpha1.OrdererGroup
	if err := r.Get(ctx, req.NamespacedName, &ordererGroup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the OrdererGroup is being deleted
	if !ordererGroup.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &ordererGroup)
	}

	// Set initial status if not set
	if ordererGroup.Status.Status == "" {
		r.updateOrdererGroupStatus(ctx, &ordererGroup, fabricxv1alpha1.PendingStatus, "Initializing OrdererGroup")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &ordererGroup); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateOrdererGroupStatus(ctx, &ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the OrdererGroup
	if err := r.reconcileOrdererGroup(ctx, &ordererGroup); err != nil {
		// The reconcileOrdererGroup method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if ordererGroup.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile OrdererGroup: %v", err)
			r.updateOrdererGroupStatus(ctx, &ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile OrdererGroup")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileOrdererGroup handles the reconciliation of an OrdererGroup
func (r *OrdererGroupReconciler) reconcileOrdererGroup(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererGroup reconciliation",
				"ordererGroup", ordererGroup.Name, "namespace", ordererGroup.Namespace)

			// Update the OrdererGroup status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererGroup reconciliation: %v", panicErr)
			r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Starting OrdererGroup reconciliation",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace)

	// Get merged configurations for each component
	consenterConfig := r.getMergedComponentConfig(ordererGroup, "consenter", ordererGroup.Spec.Components.Consenter)
	assemblerConfig := r.getMergedComponentConfig(ordererGroup, "assembler", ordererGroup.Spec.Components.Assembler)
	routerConfig := r.getMergedComponentConfig(ordererGroup, "router", ordererGroup.Spec.Components.Router)

	log.Info("Reconciling OrdererGroup components",
		"consenter", consenterConfig.Replicas,
		"batchers", len(ordererGroup.Spec.Components.Batchers),
		"assembler", assemblerConfig.Replicas,
		"router", routerConfig.Replicas)

	// Reconcile each component (certificates are handled by individual component controllers)
	if err := r.ConsenterController.Reconcile(ctx, ordererGroup, consenterConfig); err != nil {
		errorMsg := fmt.Sprintf("Failed to reconcile consenter: %v", err)
		log.Error(err, "Failed to reconcile Consenter")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("failed to reconcile consenter: %w", err)
	}

	// Reconcile batchers (multiple batcher instances)
	if err := r.BatcherController.Reconcile(ctx, ordererGroup, nil); err != nil {
		errorMsg := fmt.Sprintf("Failed to reconcile batchers: %v", err)
		log.Error(err, "Failed to reconcile Batchers")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("failed to reconcile batchers: %w", err)
	}

	if err := r.AssemblerController.Reconcile(ctx, ordererGroup, assemblerConfig); err != nil {
		errorMsg := fmt.Sprintf("Failed to reconcile assembler: %v", err)
		log.Error(err, "Failed to reconcile Assembler")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("failed to reconcile assembler: %w", err)
	}

	if err := r.RouterController.Reconcile(ctx, ordererGroup, routerConfig); err != nil {
		errorMsg := fmt.Sprintf("Failed to reconcile router: %v", err)
		log.Error(err, "Failed to reconcile Router")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("failed to reconcile router: %w", err)
	}

	// Update status to success
	r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.RunningStatus, "OrdererGroup reconciled successfully")

	log.Info("OrdererGroup reconciliation completed successfully")
	return nil
}

// handleDeletion handles the deletion of an OrdererGroup
func (r *OrdererGroupReconciler) handleDeletion(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererGroup deletion",
				"ordererGroup", ordererGroup.Name, "namespace", ordererGroup.Namespace)

			// Update the OrdererGroup status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererGroup deletion: %v", panicErr)
			r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Handling OrdererGroup deletion",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace)

	// Set status to indicate deletion
	r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.PendingStatus, "Deleting OrdererGroup components")

	// Get merged configurations for cleanup
	consenterConfig := r.getMergedComponentConfig(ordererGroup, "consenter", ordererGroup.Spec.Components.Consenter)
	assemblerConfig := r.getMergedComponentConfig(ordererGroup, "assembler", ordererGroup.Spec.Components.Assembler)
	routerConfig := r.getMergedComponentConfig(ordererGroup, "router", ordererGroup.Spec.Components.Router)

	// Clean up each component (certificates are handled by individual component controllers)
	if err := r.ConsenterController.Cleanup(ctx, ordererGroup, consenterConfig); err != nil {
		errorMsg := fmt.Sprintf("Failed to cleanup consenter: %v", err)
		log.Error(err, "Failed to cleanup Consenter")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, fmt.Errorf("failed to cleanup consenter: %w", err)
	}

	// Clean up batchers (multiple batcher instances)
	if err := r.BatcherController.Cleanup(ctx, ordererGroup, nil); err != nil {
		errorMsg := fmt.Sprintf("Failed to cleanup batchers: %v", err)
		log.Error(err, "Failed to cleanup Batchers")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, fmt.Errorf("failed to cleanup batchers: %w", err)
	}

	if err := r.AssemblerController.Cleanup(ctx, ordererGroup, assemblerConfig); err != nil {
		errorMsg := fmt.Sprintf("Failed to cleanup assembler: %v", err)
		log.Error(err, "Failed to cleanup Assembler")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, fmt.Errorf("failed to cleanup assembler: %w", err)
	}

	if err := r.RouterController.Cleanup(ctx, ordererGroup, routerConfig); err != nil {
		errorMsg := fmt.Sprintf("Failed to cleanup router: %v", err)
		log.Error(err, "Failed to cleanup Router")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, fmt.Errorf("failed to cleanup router: %w", err)
	}

	// Remove finalizer
	if err := r.removeFinalizer(ctx, ordererGroup); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("OrdererGroup deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the OrdererGroup
func (r *OrdererGroupReconciler) ensureFinalizer(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	if !utils.ContainsString(ordererGroup.Finalizers, FinalizerName) {
		ordererGroup.Finalizers = append(ordererGroup.Finalizers, FinalizerName)
		return r.Update(ctx, ordererGroup)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererGroup
func (r *OrdererGroupReconciler) removeFinalizer(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	ordererGroup.Finalizers = utils.RemoveString(ordererGroup.Finalizers, FinalizerName)
	return r.Update(ctx, ordererGroup)
}

// updateOrdererGroupStatus updates the OrdererGroup status with the given status and message
func (r *OrdererGroupReconciler) updateOrdererGroupStatus(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating OrdererGroup status",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace,
		"status", status,
		"message", message)

	// Update the status
	ordererGroup.Status.Status = status
	ordererGroup.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	ordererGroup.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, ordererGroup); err != nil {
		log.Error(err, "Failed to update OrdererGroup status")
	} else {
		log.Info("OrdererGroup status updated successfully",
			"name", ordererGroup.Name,
			"namespace", ordererGroup.Namespace,
			"status", status,
			"message", message)
	}
}

// updateStatus updates the OrdererGroup status (legacy method)
func (r *OrdererGroupReconciler) updateStatus(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// This is now handled by updateOrdererGroupStatus
	return nil
}

// getMergedComponentConfig merges common configuration with component-specific configuration
func (r *OrdererGroupReconciler) getMergedComponentConfig(
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
	componentConfig *fabricxv1alpha1.ComponentConfig,
) *fabricxv1alpha1.ComponentConfig {

	// Start with common configuration
	merged := &fabricxv1alpha1.ComponentConfig{
		CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
			Replicas: 1, // Default replicas
		},
	}

	// Apply common configuration if specified
	if ordererGroup.Spec.Common != nil {
		merged.CommonComponentConfig = *ordererGroup.Spec.Common
	}

	// Apply component-specific configuration if specified
	if componentConfig != nil {
		// Merge replicas (component-specific overrides common)
		if componentConfig.Replicas > 0 {
			merged.Replicas = componentConfig.Replicas
		}

		// Merge storage (component-specific overrides common)
		if componentConfig.Storage != nil {
			merged.Storage = componentConfig.Storage
		}

		// Merge resources (component-specific overrides common)
		if componentConfig.Resources != nil {
			merged.Resources = componentConfig.Resources
		}

		// Merge security context (component-specific overrides common)
		if componentConfig.SecurityContext != nil {
			merged.SecurityContext = componentConfig.SecurityContext
		}

		// Merge pod annotations (component-specific overrides common)
		if componentConfig.PodAnnotations != nil {
			merged.PodAnnotations = componentConfig.PodAnnotations
		}

		// Merge pod labels (component-specific overrides common)
		if componentConfig.PodLabels != nil {
			merged.PodLabels = componentConfig.PodLabels
		}

		// Merge volumes (component-specific overrides common)
		if componentConfig.Volumes != nil {
			merged.Volumes = componentConfig.Volumes
		}

		// Merge affinity (component-specific overrides common)
		if componentConfig.Affinity != nil {
			merged.Affinity = componentConfig.Affinity
		}

		// Merge volume mounts (component-specific overrides common)
		if componentConfig.VolumeMounts != nil {
			merged.VolumeMounts = componentConfig.VolumeMounts
		}

		// Merge image pull secrets (component-specific overrides common)
		if componentConfig.ImagePullSecrets != nil {
			merged.ImagePullSecrets = componentConfig.ImagePullSecrets
		}

		// Merge tolerations (component-specific overrides common)
		if componentConfig.Tolerations != nil {
			merged.Tolerations = componentConfig.Tolerations
		}

		// Copy component-specific fields
		merged.Ingress = componentConfig.Ingress
		merged.Certificates = componentConfig.Certificates
		merged.Endpoints = componentConfig.Endpoints
		merged.Env = componentConfig.Env
		merged.Command = componentConfig.Command
		merged.Args = componentConfig.Args
	}

	// Merge enrollment configuration
	if ordererGroup.Spec.Enrollment != nil {
		if merged.Certificates == nil {
			merged.Certificates = &fabricxv1alpha1.CertificateConfig{}
		}

		// Apply global enrollment settings if component doesn't have specific ones
		if merged.Certificates.CAHost == "" && ordererGroup.Spec.Enrollment.Sign != nil {
			merged.Certificates.CAHost = ordererGroup.Spec.Enrollment.Sign.CAHost
			merged.Certificates.CAPort = ordererGroup.Spec.Enrollment.Sign.CAPort
			merged.Certificates.CATLS = ordererGroup.Spec.Enrollment.Sign.CATLS
			merged.Certificates.EnrollID = ordererGroup.Spec.Enrollment.Sign.EnrollID
			merged.Certificates.EnrollSecret = ordererGroup.Spec.Enrollment.Sign.EnrollSecret
		}
	}

	return merged
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize component controllers
	r.AssemblerController = ordgroup.NewAssemblerController(r.Client, r.Scheme)
	r.BatcherController = ordgroup.NewBatcherController(r.Client, r.Scheme)
	r.RouterController = ordgroup.NewRouterController(r.Client, r.Scheme)
	r.ConsenterController = ordgroup.NewConsenterController(r.Client, r.Scheme)

	// Initialize certificate service
	r.CertService = certs.NewOrdererGroupCertService(r.Client, r.Scheme)

	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererGroup{}).
		Named("orderergroup").
		Complete(r)
}
