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
	// FinalizerName is the name of the finalizer used by OrdererBatcher
	FinalizerName = "ordererbatcher.fabricx.kfsoft.tech/finalizer"
)

// OrdererBatcherReconciler reconciles a OrdererBatcher object
type OrdererBatcherReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererbatchers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererbatchers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererbatchers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OrdererBatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererBatcher reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the OrdererBatcher status to failed
			ordererBatcher := &fabricxv1alpha1.OrdererBatcher{}
			if err := r.Get(ctx, req.NamespacedName, ordererBatcher); err == nil {
				panicMsg := fmt.Sprintf("Panic in OrdererBatcher reconciliation: %v", panicErr)
				r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var ordererBatcher fabricxv1alpha1.OrdererBatcher
	if err := r.Get(ctx, req.NamespacedName, &ordererBatcher); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the OrdererBatcher is being deleted
	if !ordererBatcher.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &ordererBatcher)
	}

	// Set initial status if not set
	if ordererBatcher.Status.Status == "" {
		r.updateOrdererBatcherStatus(ctx, &ordererBatcher, fabricxv1alpha1.PendingStatus, "Initializing OrdererBatcher")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &ordererBatcher); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateOrdererBatcherStatus(ctx, &ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the OrdererBatcher
	if err := r.reconcileOrdererBatcher(ctx, &ordererBatcher); err != nil {
		// The reconcileOrdererBatcher method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if ordererBatcher.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile OrdererBatcher: %v", err)
			r.updateOrdererBatcherStatus(ctx, &ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile OrdererBatcher")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileOrdererBatcher handles the reconciliation of an OrdererBatcher
func (r *OrdererBatcherReconciler) reconcileOrdererBatcher(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererBatcher reconciliation",
				"ordererBatcher", ordererBatcher.Name, "namespace", ordererBatcher.Namespace)

			// Update the OrdererBatcher status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererBatcher reconciliation: %v", panicErr)
			r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Starting OrdererBatcher reconciliation",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace,
		"deploymentMode", ordererBatcher.Spec.DeploymentMode,
		"shardID", ordererBatcher.Spec.ShardID)

	// Determine deployment mode
	deploymentMode := ordererBatcher.Spec.DeploymentMode
	if deploymentMode == "" {
		deploymentMode = "deploy" // Default to deploy mode
	}

	// Reconcile based on deployment mode
	switch deploymentMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, ordererBatcher); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, ordererBatcher); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid deployment mode: %s", deploymentMode)
		log.Error(fmt.Errorf(errorMsg), "Invalid deployment mode")
		r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf(errorMsg)
	}

	// Update status to success
	r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.RunningStatus, "OrdererBatcher reconciled successfully")

	log.Info("OrdererBatcher reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *OrdererBatcherReconciler) reconcileConfigureMode(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererBatcher in configure mode",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace)

	// In configure mode, only create configuration resources
	// This could include ConfigMaps, Secrets, etc. but no actual deployments

	// TODO: Implement configuration resource creation
	// - Create ConfigMaps for configuration
	// - Create Secrets for certificates
	// - Create any other configuration resources needed

	log.Info("OrdererBatcher configure mode reconciliation completed")
	return nil
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *OrdererBatcherReconciler) reconcileDeployMode(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererBatcher in deploy mode",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace)

	// In deploy mode, create full deployment resources
	// This includes Deployments, Services, PVCs, etc.

	// TODO: Implement full deployment resource creation
	// - Create Deployment/StatefulSet
	// - Create Service
	// - Create PVC if needed
	// - Create ConfigMaps and Secrets
	// - Create any other deployment resources needed

	log.Info("OrdererBatcher deploy mode reconciliation completed")
	return nil
}

// handleDeletion handles the deletion of an OrdererBatcher
func (r *OrdererBatcherReconciler) handleDeletion(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererBatcher deletion",
				"ordererBatcher", ordererBatcher.Name, "namespace", ordererBatcher.Namespace)

			// Update the OrdererBatcher status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererBatcher deletion: %v", panicErr)
			r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Handling OrdererBatcher deletion",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace)

	// Set status to indicate deletion
	r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.PendingStatus, "Deleting OrdererBatcher resources")

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, ordererBatcher); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("OrdererBatcher deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the OrdererBatcher
func (r *OrdererBatcherReconciler) ensureFinalizer(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	if !utils.ContainsString(ordererBatcher.Finalizers, FinalizerName) {
		ordererBatcher.Finalizers = append(ordererBatcher.Finalizers, FinalizerName)
		return r.Update(ctx, ordererBatcher)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererBatcher
func (r *OrdererBatcherReconciler) removeFinalizer(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	ordererBatcher.Finalizers = utils.RemoveString(ordererBatcher.Finalizers, FinalizerName)
	return r.Update(ctx, ordererBatcher)
}

// updateOrdererBatcherStatus updates the OrdererBatcher status with the given status and message
func (r *OrdererBatcherReconciler) updateOrdererBatcherStatus(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating OrdererBatcher status",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace,
		"status", status,
		"message", message)

	// Update the status
	ordererBatcher.Status.Status = status
	ordererBatcher.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	ordererBatcher.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, ordererBatcher); err != nil {
		log.Error(err, "Failed to update OrdererBatcher status")
	} else {
		log.Info("OrdererBatcher status updated successfully",
			"name", ordererBatcher.Name,
			"namespace", ordererBatcher.Namespace,
			"status", status,
			"message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererBatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererBatcher{}).
		Named("ordererbatcher").
		Complete(r)
}
