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
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

const (
	// FinalizerName is the name of the finalizer used by OrdererRouter
	FinalizerName = "ordererrouter.fabricx.kfsoft.tech/finalizer"
)

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
		"deploymentMode", ordererRouter.Spec.DeploymentMode)

	// Determine deployment mode
	deploymentMode := ordererRouter.Spec.DeploymentMode
	if deploymentMode == "" {
		deploymentMode = "deploy" // Default to deploy mode
	}

	// Reconcile based on deployment mode
	switch deploymentMode {
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
		errorMsg := fmt.Sprintf("Invalid deployment mode: %s", deploymentMode)
		log.Error(fmt.Errorf(errorMsg), "Invalid deployment mode")
		r.updateOrdererRouterStatus(ctx, ordererRouter, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf(errorMsg)
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

	// In configure mode, only create configuration resources
	// This could include ConfigMaps, Secrets, etc. but no actual deployments

	// TODO: Implement configuration resource creation
	// - Create ConfigMaps for configuration
	// - Create Secrets for certificates
	// - Create any other configuration resources needed

	log.Info("OrdererRouter configure mode reconciliation completed")
	return nil
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *OrdererRouterReconciler) reconcileDeployMode(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererRouter in deploy mode",
		"name", ordererRouter.Name,
		"namespace", ordererRouter.Namespace)

	// In deploy mode, create full deployment resources
	// This includes Deployments, Services, PVCs, etc.

	// TODO: Implement full deployment resource creation
	// - Create Deployment/StatefulSet
	// - Create Service
	// - Create PVC if needed
	// - Create ConfigMaps and Secrets
	// - Create any other deployment resources needed

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
	if !utils.ContainsString(ordererRouter.Finalizers, FinalizerName) {
		ordererRouter.Finalizers = append(ordererRouter.Finalizers, FinalizerName)
		return r.Update(ctx, ordererRouter)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererRouter
func (r *OrdererRouterReconciler) removeFinalizer(ctx context.Context, ordererRouter *fabricxv1alpha1.OrdererRouter) error {
	ordererRouter.Finalizers = utils.RemoveString(ordererRouter.Finalizers, FinalizerName)
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

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererRouterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererRouter{}).
		Named("ordererrouter").
		Complete(r)
}
