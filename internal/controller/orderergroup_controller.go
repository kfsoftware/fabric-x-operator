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
	"github.com/kfsoftware/fabric-x-operator/internal/controller/ordgroup"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

const (
	// OrdererGroupFinalizerName is the name of the finalizer used by OrdererGroup
	OrdererGroupFinalizerName = "orderergroup.fabricx.kfsoft.tech/finalizer"
)

// OrdererGroupReconciler reconciles a OrdererGroup object
type OrdererGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Component controllers
	ConsenterController *ordgroup.ConsenterController
	AssemblerController *ordgroup.AssemblerController
	RouterController    *ordgroup.RouterController
	BatcherController   *ordgroup.BatcherController
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups/status,verbs=get;update;patch
// +kubebuilder:groups=fabricx.kfsoft.tech,resources=orderergroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

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

	// Reconcile child components
	if err := r.reconcileChildComponents(ctx, ordererGroup); err != nil {
		errorMsg := fmt.Sprintf("Failed to reconcile child components: %v", err)
		log.Error(err, "Failed to reconcile child components")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("failed to reconcile child components: %w", err)
	}

	// Update status to success
	r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.RunningStatus, "OrdererGroup reconciled successfully")

	log.Info("OrdererGroup reconciliation completed successfully")
	return nil
}

// reconcileChildComponents reconciles all child components of the OrdererGroup
func (r *OrdererGroupReconciler) reconcileChildComponents(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	// Reconcile Consenter
	if ordererGroup.Spec.Components.Consenter != nil {
		if err := r.ConsenterController.Reconcile(ctx, ordererGroup, ordererGroup.Spec.Components.Consenter); err != nil {
			return fmt.Errorf("failed to reconcile consenter: %w", err)
		}
		log.Info("Consenter component reconciled successfully")
	}

	// Reconcile Assembler
	if ordererGroup.Spec.Components.Assembler != nil {
		if err := r.AssemblerController.Reconcile(ctx, ordererGroup, ordererGroup.Spec.Components.Assembler); err != nil {
			return fmt.Errorf("failed to reconcile assembler: %w", err)
		}
		log.Info("Assembler component reconciled successfully")
	}

	// Reconcile Router
	if ordererGroup.Spec.Components.Router != nil {
		if err := r.RouterController.Reconcile(ctx, ordererGroup, ordererGroup.Spec.Components.Router); err != nil {
			return fmt.Errorf("failed to reconcile router: %w", err)
		}
		log.Info("Router component reconciled successfully")
	}

	// Reconcile Batchers (multiple batcher instances)
	if len(ordererGroup.Spec.Components.Batchers) > 0 {
		// The batcher controller handles multiple instances internally
		if err := r.BatcherController.Reconcile(ctx, ordererGroup, nil); err != nil {
			return fmt.Errorf("failed to reconcile batchers: %w", err)
		}
		log.Info("Batcher components reconciled successfully")
	}

	log.Info("All child components reconciled successfully")
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

	// Cleanup child components
	if err := r.cleanupChildComponents(ctx, ordererGroup); err != nil {
		errorMsg := fmt.Sprintf("Failed to cleanup child components: %v", err)
		log.Error(err, "Failed to cleanup child components")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
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

// cleanupChildComponents cleans up all child components of the OrdererGroup
func (r *OrdererGroupReconciler) cleanupChildComponents(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	// Cleanup Consenter
	if ordererGroup.Spec.Components.Consenter != nil {
		if err := r.ConsenterController.Cleanup(ctx, ordererGroup, ordererGroup.Spec.Components.Consenter); err != nil {
			log.Error(err, "Failed to cleanup consenter")
		}
	}

	// Cleanup Assembler
	if ordererGroup.Spec.Components.Assembler != nil {
		if err := r.AssemblerController.Cleanup(ctx, ordererGroup, ordererGroup.Spec.Components.Assembler); err != nil {
			log.Error(err, "Failed to cleanup assembler")
		}
	}

	// Cleanup Router
	if ordererGroup.Spec.Components.Router != nil {
		if err := r.RouterController.Cleanup(ctx, ordererGroup, ordererGroup.Spec.Components.Router); err != nil {
			log.Error(err, "Failed to cleanup router")
		}
	}

	// Cleanup Batchers
	if len(ordererGroup.Spec.Components.Batchers) > 0 {
		if err := r.BatcherController.Cleanup(ctx, ordererGroup, nil); err != nil {
			log.Error(err, "Failed to cleanup batchers")
		}
	}

	log.Info("All child components cleaned up successfully")
	return nil
}

// ensureFinalizer ensures the finalizer is present on the OrdererGroup
func (r *OrdererGroupReconciler) ensureFinalizer(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	if !utils.ContainsString(ordererGroup.Finalizers, OrdererGroupFinalizerName) {
		ordererGroup.Finalizers = append(ordererGroup.Finalizers, OrdererGroupFinalizerName)
		return r.Update(ctx, ordererGroup)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererGroup
func (r *OrdererGroupReconciler) removeFinalizer(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	ordererGroup.Finalizers = utils.RemoveString(ordererGroup.Finalizers, OrdererGroupFinalizerName)
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

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize component controllers
	r.ConsenterController = ordgroup.NewConsenterController(r.Client, r.Scheme)
	r.AssemblerController = ordgroup.NewAssemblerController(r.Client, r.Scheme)
	r.RouterController = ordgroup.NewRouterController(r.Client, r.Scheme)
	r.BatcherController = ordgroup.NewBatcherController(r.Client, r.Scheme)

	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererGroup{}).
		Named("orderergroup").
		Complete(r)
}
