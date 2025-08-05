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
	// FinalizerName is the name of the finalizer used by OrdererConsenter
	FinalizerName = "ordererconsenter.fabricx.kfsoft.tech/finalizer"
)

// OrdererConsenterReconciler reconciles a OrdererConsenter object
type OrdererConsenterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererconsenters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererconsenters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererconsenters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OrdererConsenterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererConsenter reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the OrdererConsenter status to failed
			ordererConsenter := &fabricxv1alpha1.OrdererConsenter{}
			if err := r.Get(ctx, req.NamespacedName, ordererConsenter); err == nil {
				panicMsg := fmt.Sprintf("Panic in OrdererConsenter reconciliation: %v", panicErr)
				r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var ordererConsenter fabricxv1alpha1.OrdererConsenter
	if err := r.Get(ctx, req.NamespacedName, &ordererConsenter); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the OrdererConsenter is being deleted
	if !ordererConsenter.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &ordererConsenter)
	}

	// Set initial status if not set
	if ordererConsenter.Status.Status == "" {
		r.updateOrdererConsenterStatus(ctx, &ordererConsenter, fabricxv1alpha1.PendingStatus, "Initializing OrdererConsenter")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &ordererConsenter); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateOrdererConsenterStatus(ctx, &ordererConsenter, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the OrdererConsenter
	if err := r.reconcileOrdererConsenter(ctx, &ordererConsenter); err != nil {
		// The reconcileOrdererConsenter method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if ordererConsenter.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile OrdererConsenter: %v", err)
			r.updateOrdererConsenterStatus(ctx, &ordererConsenter, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile OrdererConsenter")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileOrdererConsenter handles the reconciliation of an OrdererConsenter
func (r *OrdererConsenterReconciler) reconcileOrdererConsenter(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererConsenter reconciliation",
				"ordererConsenter", ordererConsenter.Name, "namespace", ordererConsenter.Namespace)

			// Update the OrdererConsenter status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererConsenter reconciliation: %v", panicErr)
			r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Starting OrdererConsenter reconciliation",
		"name", ordererConsenter.Name,
		"namespace", ordererConsenter.Namespace,
		"deploymentMode", ordererConsenter.Spec.DeploymentMode)

	// Determine deployment mode
	deploymentMode := ordererConsenter.Spec.DeploymentMode
	if deploymentMode == "" {
		deploymentMode = "deploy" // Default to deploy mode
	}

	// Reconcile based on deployment mode
	switch deploymentMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, ordererConsenter); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, ordererConsenter); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid deployment mode: %s", deploymentMode)
		log.Error(fmt.Errorf(errorMsg), "Invalid deployment mode")
		r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf(errorMsg)
	}

	// Update status to success
	r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.RunningStatus, "OrdererConsenter reconciled successfully")

	log.Info("OrdererConsenter reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *OrdererConsenterReconciler) reconcileConfigureMode(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererConsenter in configure mode",
		"name", ordererConsenter.Name,
		"namespace", ordererConsenter.Namespace)

	// In configure mode, only create configuration resources
	// This could include ConfigMaps, Secrets, etc. but no actual deployments

	// TODO: Implement configuration resource creation
	// - Create ConfigMaps for configuration
	// - Create Secrets for certificates
	// - Create any other configuration resources needed

	log.Info("OrdererConsenter configure mode reconciliation completed")
	return nil
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *OrdererConsenterReconciler) reconcileDeployMode(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererConsenter in deploy mode",
		"name", ordererConsenter.Name,
		"namespace", ordererConsenter.Namespace)

	// In deploy mode, create full deployment resources
	// This includes Deployments, Services, PVCs, etc.

	// TODO: Implement full deployment resource creation
	// - Create Deployment/StatefulSet
	// - Create Service
	// - Create PVC if needed
	// - Create ConfigMaps and Secrets
	// - Create any other deployment resources needed

	log.Info("OrdererConsenter deploy mode reconciliation completed")
	return nil
}

// handleDeletion handles the deletion of an OrdererConsenter
func (r *OrdererConsenterReconciler) handleDeletion(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererConsenter deletion",
				"ordererConsenter", ordererConsenter.Name, "namespace", ordererConsenter.Namespace)

			// Update the OrdererConsenter status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererConsenter deletion: %v", panicErr)
			r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Handling OrdererConsenter deletion",
		"name", ordererConsenter.Name,
		"namespace", ordererConsenter.Namespace)

	// Set status to indicate deletion
	r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.PendingStatus, "Deleting OrdererConsenter resources")

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, ordererConsenter); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("OrdererConsenter deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the OrdererConsenter
func (r *OrdererConsenterReconciler) ensureFinalizer(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) error {
	if !utils.ContainsString(ordererConsenter.Finalizers, FinalizerName) {
		ordererConsenter.Finalizers = append(ordererConsenter.Finalizers, FinalizerName)
		return r.Update(ctx, ordererConsenter)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererConsenter
func (r *OrdererConsenterReconciler) removeFinalizer(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) error {
	ordererConsenter.Finalizers = utils.RemoveString(ordererConsenter.Finalizers, FinalizerName)
	return r.Update(ctx, ordererConsenter)
}

// updateOrdererConsenterStatus updates the OrdererConsenter status with the given status and message
func (r *OrdererConsenterReconciler) updateOrdererConsenterStatus(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating OrdererConsenter status",
		"name", ordererConsenter.Name,
		"namespace", ordererConsenter.Namespace,
		"status", status,
		"message", message)

	// Update the status
	ordererConsenter.Status.Status = status
	ordererConsenter.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	ordererConsenter.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, ordererConsenter); err != nil {
		log.Error(err, "Failed to update OrdererConsenter status")
	} else {
		log.Info("OrdererConsenter status updated successfully",
			"name", ordererConsenter.Name,
			"namespace", ordererConsenter.Namespace,
			"status", status,
			"message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererConsenterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererConsenter{}).
		Named("ordererconsenter").
		Complete(r)
}
