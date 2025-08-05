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
	// FinalizerName is the name of the finalizer used by OrdererGroup
	FinalizerName = "orderergroup.fabricx.kfsoft.tech/finalizer"
)

// OrdererGroupReconciler reconciles a OrdererGroup object
type OrdererGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups/status,verbs=get;update;patch
// +kubebuilder:groups=fabricx.kfsoft.tech,resources=orderergroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererbatchers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererassemblers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererconsenters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererrouters,verbs=get;list;watch;create;update;patch;delete

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

	// Reconcile child CRDs
	if err := r.reconcileChildCRDs(ctx, ordererGroup); err != nil {
		errorMsg := fmt.Sprintf("Failed to reconcile child CRDs: %v", err)
		log.Error(err, "Failed to reconcile child CRDs")
		r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("failed to reconcile child CRDs: %w", err)
	}

	// Update status to success
	r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.RunningStatus, "OrdererGroup reconciled successfully")

	log.Info("OrdererGroup reconciliation completed successfully")
	return nil
}

// reconcileChildCRDs creates or updates the child CRDs for the OrdererGroup
func (r *OrdererGroupReconciler) reconcileChildCRDs(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	// Reconcile Consenter
	if ordererGroup.Spec.Components.Consenter != nil {
		if err := r.reconcileConsenter(ctx, ordererGroup); err != nil {
			return fmt.Errorf("failed to reconcile consenter: %w", err)
		}
	}

	// Reconcile Assembler
	if ordererGroup.Spec.Components.Assembler != nil {
		if err := r.reconcileAssembler(ctx, ordererGroup); err != nil {
			return fmt.Errorf("failed to reconcile assembler: %w", err)
		}
	}

	// Reconcile Router
	if ordererGroup.Spec.Components.Router != nil {
		if err := r.reconcileRouter(ctx, ordererGroup); err != nil {
			return fmt.Errorf("failed to reconcile router: %w", err)
		}
	}

	// Reconcile Batchers (multiple batcher instances)
	if len(ordererGroup.Spec.Components.Batchers) > 0 {
		if err := r.reconcileBatchers(ctx, ordererGroup); err != nil {
			return fmt.Errorf("failed to reconcile batchers: %w", err)
		}
	}

	log.Info("Child CRDs reconciled successfully")
	return nil
}

// reconcileConsenter creates or updates the OrdererConsenter CRD
func (r *OrdererGroupReconciler) reconcileConsenter(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	consenterName := fmt.Sprintf("%s-consenter", ordererGroup.Name)
	consenter := &fabricxv1alpha1.OrdererConsenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consenterName,
			Namespace: ordererGroup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: ordererGroup.APIVersion,
					Kind:       ordererGroup.Kind,
					Name:       ordererGroup.Name,
					UID:        ordererGroup.UID,
					Controller: &[]bool{true}[0],
				},
			},
		},
		Spec: fabricxv1alpha1.OrdererConsenterSpec{
			MSPID:   ordererGroup.Spec.MSPID,
			PartyID: ordererGroup.Spec.PartyID,
		},
	}

	// Merge configuration from OrdererGroup
	r.mergeConsenterConfig(&consenter.Spec, ordererGroup.Spec.Common, ordererGroup.Spec.Components.Consenter)

	// Create or update the consenter
	if err := r.Create(ctx, consenter); err != nil {
		if client.IgnoreAlreadyExists(err) != nil {
			return fmt.Errorf("failed to create consenter: %w", err)
		}
		// Update existing consenter
		if err := r.Update(ctx, consenter); err != nil {
			return fmt.Errorf("failed to update consenter: %w", err)
		}
	}

	log.Info("Consenter reconciled successfully", "name", consenterName)
	return nil
}

// reconcileAssembler creates or updates the OrdererAssembler CRD
func (r *OrdererGroupReconciler) reconcileAssembler(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	assemblerName := fmt.Sprintf("%s-assembler", ordererGroup.Name)
	assembler := &fabricxv1alpha1.OrdererAssembler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      assemblerName,
			Namespace: ordererGroup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: ordererGroup.APIVersion,
					Kind:       ordererGroup.Kind,
					Name:       ordererGroup.Name,
					UID:        ordererGroup.UID,
					Controller: &[]bool{true}[0],
				},
			},
		},
		Spec: fabricxv1alpha1.OrdererAssemblerSpec{
			MSPID:   ordererGroup.Spec.MSPID,
			PartyID: ordererGroup.Spec.PartyID,
		},
	}

	// Merge configuration from OrdererGroup
	r.mergeAssemblerConfig(&assembler.Spec, ordererGroup.Spec.Common, ordererGroup.Spec.Components.Assembler)

	// Create or update the assembler
	if err := r.Create(ctx, assembler); err != nil {
		if client.IgnoreAlreadyExists(err) != nil {
			return fmt.Errorf("failed to create assembler: %w", err)
		}
		// Update existing assembler
		if err := r.Update(ctx, assembler); err != nil {
			return fmt.Errorf("failed to update assembler: %w", err)
		}
	}

	log.Info("Assembler reconciled successfully", "name", assemblerName)
	return nil
}

// reconcileRouter creates or updates the OrdererRouter CRD
func (r *OrdererGroupReconciler) reconcileRouter(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	routerName := fmt.Sprintf("%s-router", ordererGroup.Name)
	router := &fabricxv1alpha1.OrdererRouter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routerName,
			Namespace: ordererGroup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: ordererGroup.APIVersion,
					Kind:       ordererGroup.Kind,
					Name:       ordererGroup.Name,
					UID:        ordererGroup.UID,
					Controller: &[]bool{true}[0],
				},
			},
		},
		Spec: fabricxv1alpha1.OrdererRouterSpec{
			MSPID:   ordererGroup.Spec.MSPID,
			PartyID: ordererGroup.Spec.PartyID,
		},
	}

	// Merge configuration from OrdererGroup
	r.mergeRouterConfig(&router.Spec, ordererGroup.Spec.Common, ordererGroup.Spec.Components.Router)

	// Create or update the router
	if err := r.Create(ctx, router); err != nil {
		if client.IgnoreAlreadyExists(err) != nil {
			return fmt.Errorf("failed to create router: %w", err)
		}
		// Update existing router
		if err := r.Update(ctx, router); err != nil {
			return fmt.Errorf("failed to update router: %w", err)
		}
	}

	log.Info("Router reconciled successfully", "name", routerName)
	return nil
}

// reconcileBatchers creates or updates the OrdererBatcher CRDs for each batcher instance
func (r *OrdererGroupReconciler) reconcileBatchers(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	for i, batcherConfig := range ordererGroup.Spec.Components.Batchers {
		batcherName := fmt.Sprintf("%s-batcher-%d", ordererGroup.Name, i)
		batcher := &fabricxv1alpha1.OrdererBatcher{
			ObjectMeta: metav1.ObjectMeta{
				Name:      batcherName,
				Namespace: ordererGroup.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: ordererGroup.APIVersion,
						Kind:       ordererGroup.Kind,
						Name:       ordererGroup.Name,
						UID:        ordererGroup.UID,
						Controller: &[]bool{true}[0],
					},
				},
			},
			Spec: fabricxv1alpha1.OrdererBatcherSpec{
				MSPID:   ordererGroup.Spec.MSPID,
				PartyID: ordererGroup.Spec.PartyID,
				ShardID: batcherConfig.ShardID,
			},
		}

		// Merge configuration from OrdererGroup
		r.mergeBatcherConfig(&batcher.Spec, ordererGroup.Spec.Common, &batcherConfig)

		// Create or update the batcher
		if err := r.Create(ctx, batcher); err != nil {
			if client.IgnoreAlreadyExists(err) != nil {
				return fmt.Errorf("failed to create batcher %d: %w", i, err)
			}
			// Update existing batcher
			if err := r.Update(ctx, batcher); err != nil {
				return fmt.Errorf("failed to update batcher %d: %w", i, err)
			}
		}

		log.Info("Batcher reconciled successfully", "name", batcherName, "shardID", batcherConfig.ShardID)
	}

	return nil
}

// mergeComponentConfig merges common configuration with component-specific configuration
func (r *OrdererGroupReconciler) mergeComponentConfig(spec interface{}, common *fabricxv1alpha1.CommonComponentConfig, component *fabricxv1alpha1.ComponentConfig) {
	// This is a generic merge function that can be implemented based on the specific needs
	// For now, we'll use reflection or type-specific merging
	// The actual implementation would depend on the specific struct types
}

// mergeConsenterConfig merges common configuration with consenter-specific configuration
func (r *OrdererGroupReconciler) mergeConsenterConfig(spec *fabricxv1alpha1.OrdererConsenterSpec, common *fabricxv1alpha1.CommonComponentConfig, component *fabricxv1alpha1.ComponentConfig) {
	// Apply common configuration if specified
	if common != nil {
		if spec.Replicas == 0 {
			spec.Replicas = common.Replicas
		}
		spec.Storage = common.Storage
		spec.Resources = common.Resources
		spec.SecurityContext = common.SecurityContext
		spec.PodAnnotations = common.PodAnnotations
		spec.PodLabels = common.PodLabels
		spec.Volumes = common.Volumes
		spec.Affinity = common.Affinity
		spec.VolumeMounts = common.VolumeMounts
		spec.ImagePullSecrets = common.ImagePullSecrets
		spec.Tolerations = common.Tolerations
	}

	// Apply component-specific configuration
	if component != nil {
		if component.Replicas > 0 {
			spec.Replicas = component.Replicas
		}
		if component.Storage != nil {
			spec.Storage = component.Storage
		}
		if component.Resources != nil {
			spec.Resources = component.Resources
		}
		if component.SecurityContext != nil {
			spec.SecurityContext = component.SecurityContext
		}
		if component.PodAnnotations != nil {
			spec.PodAnnotations = component.PodAnnotations
		}
		if component.PodLabels != nil {
			spec.PodLabels = component.PodLabels
		}
		if component.Volumes != nil {
			spec.Volumes = component.Volumes
		}
		if component.Affinity != nil {
			spec.Affinity = component.Affinity
		}
		if component.VolumeMounts != nil {
			spec.VolumeMounts = component.VolumeMounts
		}
		if component.ImagePullSecrets != nil {
			spec.ImagePullSecrets = component.ImagePullSecrets
		}
		if component.Tolerations != nil {
			spec.Tolerations = component.Tolerations
		}
		spec.Ingress = component.Ingress
		spec.Certificates = component.Certificates
		spec.Endpoints = component.Endpoints
		spec.Env = component.Env
		spec.Command = component.Command
		spec.Args = component.Args
	}
}

// mergeAssemblerConfig merges common configuration with assembler-specific configuration
func (r *OrdererGroupReconciler) mergeAssemblerConfig(spec *fabricxv1alpha1.OrdererAssemblerSpec, common *fabricxv1alpha1.CommonComponentConfig, component *fabricxv1alpha1.ComponentConfig) {
	// Apply common configuration if specified
	if common != nil {
		if spec.Replicas == 0 {
			spec.Replicas = common.Replicas
		}
		spec.Storage = common.Storage
		spec.Resources = common.Resources
		spec.SecurityContext = common.SecurityContext
		spec.PodAnnotations = common.PodAnnotations
		spec.PodLabels = common.PodLabels
		spec.Volumes = common.Volumes
		spec.Affinity = common.Affinity
		spec.VolumeMounts = common.VolumeMounts
		spec.ImagePullSecrets = common.ImagePullSecrets
		spec.Tolerations = common.Tolerations
	}

	// Apply component-specific configuration
	if component != nil {
		if component.Replicas > 0 {
			spec.Replicas = component.Replicas
		}
		if component.Storage != nil {
			spec.Storage = component.Storage
		}
		if component.Resources != nil {
			spec.Resources = component.Resources
		}
		if component.SecurityContext != nil {
			spec.SecurityContext = component.SecurityContext
		}
		if component.PodAnnotations != nil {
			spec.PodAnnotations = component.PodAnnotations
		}
		if component.PodLabels != nil {
			spec.PodLabels = component.PodLabels
		}
		if component.Volumes != nil {
			spec.Volumes = component.Volumes
		}
		if component.Affinity != nil {
			spec.Affinity = component.Affinity
		}
		if component.VolumeMounts != nil {
			spec.VolumeMounts = component.VolumeMounts
		}
		if component.ImagePullSecrets != nil {
			spec.ImagePullSecrets = component.ImagePullSecrets
		}
		if component.Tolerations != nil {
			spec.Tolerations = component.Tolerations
		}
		spec.Ingress = component.Ingress
		spec.Certificates = component.Certificates
		spec.Endpoints = component.Endpoints
		spec.Env = component.Env
		spec.Command = component.Command
		spec.Args = component.Args
	}
}

// mergeRouterConfig merges common configuration with router-specific configuration
func (r *OrdererGroupReconciler) mergeRouterConfig(spec *fabricxv1alpha1.OrdererRouterSpec, common *fabricxv1alpha1.CommonComponentConfig, component *fabricxv1alpha1.ComponentConfig) {
	// Apply common configuration if specified
	if common != nil {
		if spec.Replicas == 0 {
			spec.Replicas = common.Replicas
		}
		spec.Storage = common.Storage
		spec.Resources = common.Resources
		spec.SecurityContext = common.SecurityContext
		spec.PodAnnotations = common.PodAnnotations
		spec.PodLabels = common.PodLabels
		spec.Volumes = common.Volumes
		spec.Affinity = common.Affinity
		spec.VolumeMounts = common.VolumeMounts
		spec.ImagePullSecrets = common.ImagePullSecrets
		spec.Tolerations = common.Tolerations
	}

	// Apply component-specific configuration
	if component != nil {
		if component.Replicas > 0 {
			spec.Replicas = component.Replicas
		}
		if component.Storage != nil {
			spec.Storage = component.Storage
		}
		if component.Resources != nil {
			spec.Resources = component.Resources
		}
		if component.SecurityContext != nil {
			spec.SecurityContext = component.SecurityContext
		}
		if component.PodAnnotations != nil {
			spec.PodAnnotations = component.PodAnnotations
		}
		if component.PodLabels != nil {
			spec.PodLabels = component.PodLabels
		}
		if component.Volumes != nil {
			spec.Volumes = component.Volumes
		}
		if component.Affinity != nil {
			spec.Affinity = component.Affinity
		}
		if component.VolumeMounts != nil {
			spec.VolumeMounts = component.VolumeMounts
		}
		if component.ImagePullSecrets != nil {
			spec.ImagePullSecrets = component.ImagePullSecrets
		}
		if component.Tolerations != nil {
			spec.Tolerations = component.Tolerations
		}
		spec.Ingress = component.Ingress
		spec.Certificates = component.Certificates
		spec.Endpoints = component.Endpoints
		spec.Env = component.Env
		spec.Command = component.Command
		spec.Args = component.Args
	}
}

// mergeBatcherConfig merges common configuration with batcher-specific configuration
func (r *OrdererGroupReconciler) mergeBatcherConfig(spec *fabricxv1alpha1.OrdererBatcherSpec, common *fabricxv1alpha1.CommonComponentConfig, batcher *fabricxv1alpha1.BatcherInstance) {
	// Apply common configuration if specified
	if common != nil {
		if spec.Replicas == 0 {
			spec.Replicas = common.Replicas
		}
		spec.Storage = common.Storage
		spec.Resources = common.Resources
		spec.SecurityContext = common.SecurityContext
		spec.PodAnnotations = common.PodAnnotations
		spec.PodLabels = common.PodLabels
		spec.Volumes = common.Volumes
		spec.Affinity = common.Affinity
		spec.VolumeMounts = common.VolumeMounts
		spec.ImagePullSecrets = common.ImagePullSecrets
		spec.Tolerations = common.Tolerations
	}

	// Apply batcher-specific configuration
	if batcher != nil {
		if batcher.Replicas > 0 {
			spec.Replicas = batcher.Replicas
		}
		if batcher.Storage != nil {
			spec.Storage = batcher.Storage
		}
		if batcher.Resources != nil {
			spec.Resources = batcher.Resources
		}
		if batcher.SecurityContext != nil {
			spec.SecurityContext = batcher.SecurityContext
		}
		if batcher.PodAnnotations != nil {
			spec.PodAnnotations = batcher.PodAnnotations
		}
		if batcher.PodLabels != nil {
			spec.PodLabels = batcher.PodLabels
		}
		if batcher.Volumes != nil {
			spec.Volumes = batcher.Volumes
		}
		if batcher.Affinity != nil {
			spec.Affinity = batcher.Affinity
		}
		if batcher.VolumeMounts != nil {
			spec.VolumeMounts = batcher.VolumeMounts
		}
		if batcher.ImagePullSecrets != nil {
			spec.ImagePullSecrets = batcher.ImagePullSecrets
		}
		if batcher.Tolerations != nil {
			spec.Tolerations = batcher.Tolerations
		}
		spec.Ingress = batcher.Ingress
		spec.Certificates = batcher.Certificates
		spec.Endpoints = batcher.Endpoints
		spec.Env = batcher.Env
		spec.Command = batcher.Command
		spec.Args = batcher.Args
	}
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

	// Child CRDs will be automatically deleted due to owner references
	// No need to manually delete them

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

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererGroup{}).
		Named("orderergroup").
		Complete(r)
}
