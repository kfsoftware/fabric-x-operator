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
	clientset "github.com/kfsoftware/fabric-x-operator/pkg/client/clientset/versioned"
)

const (
	// OrdererGroupFinalizerName is the name of the finalizer used by OrdererGroup
	OrdererGroupFinalizerName = "orderergroup.fabricx.kfsoft.tech/finalizer"
)

// OrdererGroupReconciler reconciles a OrdererGroup object
type OrdererGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Clientset clientset.Interface

	// Component controllers (kept for backward compatibility)
	ConsenterController *ordgroup.ConsenterController
	AssemblerController *ordgroup.AssemblerController
	RouterController    *ordgroup.RouterController
	BatcherController   *ordgroup.BatcherController
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=orderergroups/status,verbs=get;update;patch
// +kubebuilder:groups=fabricx.kfsoft.tech,resources=orderergroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererassemblers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererbatchers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererconsenters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererrouters,verbs=get;list;watch;create;update;patch;delete
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

	// Check if we should manage child CRDs
	manageChildCRDs := true
	if ordererGroup.Spec.ManageChildCRDs != nil {
		manageChildCRDs = *ordererGroup.Spec.ManageChildCRDs
	}

	if manageChildCRDs {
		// Reconcile child CRDs using the clientset
		if err := r.reconcileChildCRDs(ctx, ordererGroup); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile child CRDs: %v", err)
			log.Error(err, "Failed to reconcile child CRDs")
			r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile child CRDs: %w", err)
		}
	} else {
		// Use existing component controllers for backward compatibility
		if err := r.reconcileChildComponents(ctx, ordererGroup); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile child components: %v", err)
			log.Error(err, "Failed to reconcile child components")
			r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile child components: %w", err)
		}
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

// reconcileChildCRDs reconciles all child CRDs using the clientset
func (r *OrdererGroupReconciler) reconcileChildCRDs(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	// Reconcile Consenter
	if ordererGroup.Spec.Components.Consenter != nil {
		if err := r.reconcileConsenterCRD(ctx, ordererGroup); err != nil {
			return fmt.Errorf("failed to reconcile consenter CRD: %w", err)
		}
		log.Info("Consenter CRD reconciled successfully")
	}

	// Reconcile Assembler
	if ordererGroup.Spec.Components.Assembler != nil {
		if err := r.reconcileAssemblerCRD(ctx, ordererGroup); err != nil {
			return fmt.Errorf("failed to reconcile assembler CRD: %w", err)
		}
		log.Info("Assembler CRD reconciled successfully")
	}

	// Reconcile Router
	if ordererGroup.Spec.Components.Router != nil {
		if err := r.reconcileRouterCRD(ctx, ordererGroup); err != nil {
			return fmt.Errorf("failed to reconcile router CRD: %w", err)
		}
		log.Info("Router CRD reconciled successfully")
	}

	// Reconcile Batchers (multiple batcher instances)
	if len(ordererGroup.Spec.Components.Batchers) > 0 {
		if err := r.reconcileBatcherCRDs(ctx, ordererGroup); err != nil {
			return fmt.Errorf("failed to reconcile batcher CRDs: %w", err)
		}
		log.Info("Batcher CRDs reconciled successfully")
	}

	log.Info("All child CRDs reconciled successfully")
	return nil
}

// reconcileConsenterCRD creates or updates the Consenter CRD
func (r *OrdererGroupReconciler) reconcileConsenterCRD(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	consenterName := fmt.Sprintf("%s-consenter", ordererGroup.Name)

	// Check if the CRD already exists
	existingConsenter, err := r.Clientset.ApiV1alpha1().OrdererConsenters(ordererGroup.Namespace).Get(ctx, consenterName, metav1.GetOptions{})
	if err == nil {
		// Update existing CRD
		existingConsenter.Spec = r.buildConsenterSpec(ordererGroup, ordererGroup.Spec.Components.Consenter)
		_, err = r.Clientset.ApiV1alpha1().OrdererConsenters(ordererGroup.Namespace).Update(ctx, existingConsenter, metav1.UpdateOptions{})
		return err
	}

	// Create new CRD
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
		Spec: r.buildConsenterSpec(ordererGroup, ordererGroup.Spec.Components.Consenter),
	}

	_, err = r.Clientset.ApiV1alpha1().OrdererConsenters(ordererGroup.Namespace).Create(ctx, consenter, metav1.CreateOptions{})
	return err
}

// reconcileAssemblerCRD creates or updates the Assembler CRD
func (r *OrdererGroupReconciler) reconcileAssemblerCRD(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	assemblerName := fmt.Sprintf("%s-assembler", ordererGroup.Name)

	// Check if the CRD already exists
	existingAssembler, err := r.Clientset.ApiV1alpha1().OrdererAssemblers(ordererGroup.Namespace).Get(ctx, assemblerName, metav1.GetOptions{})
	if err == nil {
		// Update existing CRD
		existingAssembler.Spec = r.buildAssemblerSpec(ordererGroup, ordererGroup.Spec.Components.Assembler)
		_, err = r.Clientset.ApiV1alpha1().OrdererAssemblers(ordererGroup.Namespace).Update(ctx, existingAssembler, metav1.UpdateOptions{})
		return err
	}

	// Create new CRD
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
		Spec: r.buildAssemblerSpec(ordererGroup, ordererGroup.Spec.Components.Assembler),
	}

	_, err = r.Clientset.ApiV1alpha1().OrdererAssemblers(ordererGroup.Namespace).Create(ctx, assembler, metav1.CreateOptions{})
	return err
}

// reconcileRouterCRD creates or updates the Router CRD
func (r *OrdererGroupReconciler) reconcileRouterCRD(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	routerName := fmt.Sprintf("%s-router", ordererGroup.Name)

	// Check if the CRD already exists
	existingRouter, err := r.Clientset.ApiV1alpha1().OrdererRouters(ordererGroup.Namespace).Get(ctx, routerName, metav1.GetOptions{})
	if err == nil {
		// Update existing CRD
		existingRouter.Spec = r.buildRouterSpec(ordererGroup, ordererGroup.Spec.Components.Router)
		_, err = r.Clientset.ApiV1alpha1().OrdererRouters(ordererGroup.Namespace).Update(ctx, existingRouter, metav1.UpdateOptions{})
		return err
	}

	// Create new CRD
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
		Spec: r.buildRouterSpec(ordererGroup, ordererGroup.Spec.Components.Router),
	}

	_, err = r.Clientset.ApiV1alpha1().OrdererRouters(ordererGroup.Namespace).Create(ctx, router, metav1.CreateOptions{})
	return err
}

// reconcileBatcherCRDs creates or updates the Batcher CRDs
func (r *OrdererGroupReconciler) reconcileBatcherCRDs(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	for i, batcherInstance := range ordererGroup.Spec.Components.Batchers {
		batcherName := fmt.Sprintf("%s-batcher-%d", ordererGroup.Name, i)

		// Check if the CRD already exists
		existingBatcher, err := r.Clientset.ApiV1alpha1().OrdererBatchers(ordererGroup.Namespace).Get(ctx, batcherName, metav1.GetOptions{})
		if err == nil {
			// Update existing CRD
			existingBatcher.Spec = r.buildBatcherSpec(ordererGroup, &batcherInstance)
			_, err = r.Clientset.ApiV1alpha1().OrdererBatchers(ordererGroup.Namespace).Update(ctx, existingBatcher, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("failed to update batcher CRD %s: %w", batcherName, err)
			}
			continue
		}

		// Create new CRD
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
			Spec: r.buildBatcherSpec(ordererGroup, &batcherInstance),
		}

		_, err = r.Clientset.ApiV1alpha1().OrdererBatchers(ordererGroup.Namespace).Create(ctx, batcher, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create batcher CRD %s: %w", batcherName, err)
		}
	}

	return nil
}

// buildConsenterSpec builds the Consenter spec from OrdererGroup configuration
func (r *OrdererGroupReconciler) buildConsenterSpec(ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) fabricxv1alpha1.OrdererConsenterSpec {
	// Determine deployment mode
	deploymentMode := ordererGroup.Spec.DeploymentMode
	if deploymentMode == "" {
		deploymentMode = "deploy" // Default to deploy mode
	}

	spec := fabricxv1alpha1.OrdererConsenterSpec{
		DeploymentMode: deploymentMode,
		MSPID:          ordererGroup.Spec.MSPID,
		PartyID:        ordererGroup.Spec.PartyID,
	}

	// Merge common configuration
	if ordererGroup.Spec.Common != nil {
		spec.Replicas = ordererGroup.Spec.Common.Replicas
		spec.Storage = ordererGroup.Spec.Common.Storage
		spec.Resources = ordererGroup.Spec.Common.Resources
		spec.SecurityContext = ordererGroup.Spec.Common.SecurityContext
		spec.PodAnnotations = ordererGroup.Spec.Common.PodAnnotations
		spec.PodLabels = ordererGroup.Spec.Common.PodLabels
		spec.Volumes = ordererGroup.Spec.Common.Volumes
		spec.Affinity = ordererGroup.Spec.Common.Affinity
		spec.VolumeMounts = ordererGroup.Spec.Common.VolumeMounts
		spec.ImagePullSecrets = ordererGroup.Spec.Common.ImagePullSecrets
		spec.Tolerations = ordererGroup.Spec.Common.Tolerations
	}

	// Override with component-specific configuration
	if config != nil {
		if config.Replicas != 0 {
			spec.Replicas = config.Replicas
		}
		if config.Storage != nil {
			spec.Storage = config.Storage
		}
		if config.Resources != nil {
			spec.Resources = config.Resources
		}
		if config.SecurityContext != nil {
			spec.SecurityContext = config.SecurityContext
		}
		if config.PodAnnotations != nil {
			spec.PodAnnotations = config.PodAnnotations
		}
		if config.PodLabels != nil {
			spec.PodLabels = config.PodLabels
		}
		if config.Volumes != nil {
			spec.Volumes = config.Volumes
		}
		if config.Affinity != nil {
			spec.Affinity = config.Affinity
		}
		if config.VolumeMounts != nil {
			spec.VolumeMounts = config.VolumeMounts
		}
		if config.ImagePullSecrets != nil {
			spec.ImagePullSecrets = config.ImagePullSecrets
		}
		if config.Tolerations != nil {
			spec.Tolerations = config.Tolerations
		}
		spec.Ingress = config.Ingress
		spec.Certificates = config.Certificates
		spec.Endpoints = config.Endpoints
		spec.Env = config.Env
		spec.Command = config.Command
		spec.Args = config.Args
	}

	return spec
}

// buildAssemblerSpec builds the Assembler spec from OrdererGroup configuration
func (r *OrdererGroupReconciler) buildAssemblerSpec(ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) fabricxv1alpha1.OrdererAssemblerSpec {
	// Determine deployment mode
	deploymentMode := ordererGroup.Spec.DeploymentMode
	if deploymentMode == "" {
		deploymentMode = "deploy" // Default to deploy mode
	}

	spec := fabricxv1alpha1.OrdererAssemblerSpec{
		DeploymentMode: deploymentMode,
		MSPID:          ordererGroup.Spec.MSPID,
		PartyID:        ordererGroup.Spec.PartyID,
	}

	// Merge common configuration
	if ordererGroup.Spec.Common != nil {
		spec.Replicas = ordererGroup.Spec.Common.Replicas
		spec.Storage = ordererGroup.Spec.Common.Storage
		spec.Resources = ordererGroup.Spec.Common.Resources
		spec.SecurityContext = ordererGroup.Spec.Common.SecurityContext
		spec.PodAnnotations = ordererGroup.Spec.Common.PodAnnotations
		spec.PodLabels = ordererGroup.Spec.Common.PodLabels
		spec.Volumes = ordererGroup.Spec.Common.Volumes
		spec.Affinity = ordererGroup.Spec.Common.Affinity
		spec.VolumeMounts = ordererGroup.Spec.Common.VolumeMounts
		spec.ImagePullSecrets = ordererGroup.Spec.Common.ImagePullSecrets
		spec.Tolerations = ordererGroup.Spec.Common.Tolerations
	}

	// Override with component-specific configuration
	if config != nil {
		if config.Replicas != 0 {
			spec.Replicas = config.Replicas
		}
		if config.Storage != nil {
			spec.Storage = config.Storage
		}
		if config.Resources != nil {
			spec.Resources = config.Resources
		}
		if config.SecurityContext != nil {
			spec.SecurityContext = config.SecurityContext
		}
		if config.PodAnnotations != nil {
			spec.PodAnnotations = config.PodAnnotations
		}
		if config.PodLabels != nil {
			spec.PodLabels = config.PodLabels
		}
		if config.Volumes != nil {
			spec.Volumes = config.Volumes
		}
		if config.Affinity != nil {
			spec.Affinity = config.Affinity
		}
		if config.VolumeMounts != nil {
			spec.VolumeMounts = config.VolumeMounts
		}
		if config.ImagePullSecrets != nil {
			spec.ImagePullSecrets = config.ImagePullSecrets
		}
		if config.Tolerations != nil {
			spec.Tolerations = config.Tolerations
		}
		spec.Ingress = config.Ingress
		spec.Certificates = config.Certificates
		spec.Endpoints = config.Endpoints
		spec.Env = config.Env
		spec.Command = config.Command
		spec.Args = config.Args
	}

	return spec
}

// buildRouterSpec builds the Router spec from OrdererGroup configuration
func (r *OrdererGroupReconciler) buildRouterSpec(ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) fabricxv1alpha1.OrdererRouterSpec {
	// Determine deployment mode
	deploymentMode := ordererGroup.Spec.DeploymentMode
	if deploymentMode == "" {
		deploymentMode = "deploy" // Default to deploy mode
	}

	spec := fabricxv1alpha1.OrdererRouterSpec{
		DeploymentMode: deploymentMode,
		MSPID:          ordererGroup.Spec.MSPID,
		PartyID:        ordererGroup.Spec.PartyID,
	}

	// Merge common configuration
	if ordererGroup.Spec.Common != nil {
		spec.Replicas = ordererGroup.Spec.Common.Replicas
		spec.Storage = ordererGroup.Spec.Common.Storage
		spec.Resources = ordererGroup.Spec.Common.Resources
		spec.SecurityContext = ordererGroup.Spec.Common.SecurityContext
		spec.PodAnnotations = ordererGroup.Spec.Common.PodAnnotations
		spec.PodLabels = ordererGroup.Spec.Common.PodLabels
		spec.Volumes = ordererGroup.Spec.Common.Volumes
		spec.Affinity = ordererGroup.Spec.Common.Affinity
		spec.VolumeMounts = ordererGroup.Spec.Common.VolumeMounts
		spec.ImagePullSecrets = ordererGroup.Spec.Common.ImagePullSecrets
		spec.Tolerations = ordererGroup.Spec.Common.Tolerations
	}

	// Override with component-specific configuration
	if config != nil {
		if config.Replicas != 0 {
			spec.Replicas = config.Replicas
		}
		if config.Storage != nil {
			spec.Storage = config.Storage
		}
		if config.Resources != nil {
			spec.Resources = config.Resources
		}
		if config.SecurityContext != nil {
			spec.SecurityContext = config.SecurityContext
		}
		if config.PodAnnotations != nil {
			spec.PodAnnotations = config.PodAnnotations
		}
		if config.PodLabels != nil {
			spec.PodLabels = config.PodLabels
		}
		if config.Volumes != nil {
			spec.Volumes = config.Volumes
		}
		if config.Affinity != nil {
			spec.Affinity = config.Affinity
		}
		if config.VolumeMounts != nil {
			spec.VolumeMounts = config.VolumeMounts
		}
		if config.ImagePullSecrets != nil {
			spec.ImagePullSecrets = config.ImagePullSecrets
		}
		if config.Tolerations != nil {
			spec.Tolerations = config.Tolerations
		}
		spec.Ingress = config.Ingress
		spec.Certificates = config.Certificates
		spec.Endpoints = config.Endpoints
		spec.Env = config.Env
		spec.Command = config.Command
		spec.Args = config.Args
	}

	return spec
}

// buildBatcherSpec builds the Batcher spec from OrdererGroup configuration
func (r *OrdererGroupReconciler) buildBatcherSpec(ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.BatcherInstance) fabricxv1alpha1.OrdererBatcherSpec {
	// Determine deployment mode
	deploymentMode := ordererGroup.Spec.DeploymentMode
	if deploymentMode == "" {
		deploymentMode = "deploy" // Default to deploy mode
	}

	spec := fabricxv1alpha1.OrdererBatcherSpec{
		DeploymentMode: deploymentMode,
		MSPID:          ordererGroup.Spec.MSPID,
		PartyID:        ordererGroup.Spec.PartyID,
		ShardID:        config.ShardID,
	}

	// Merge common configuration
	if ordererGroup.Spec.Common != nil {
		spec.Replicas = ordererGroup.Spec.Common.Replicas
		spec.Storage = ordererGroup.Spec.Common.Storage
		spec.Resources = ordererGroup.Spec.Common.Resources
		spec.SecurityContext = ordererGroup.Spec.Common.SecurityContext
		spec.PodAnnotations = ordererGroup.Spec.Common.PodAnnotations
		spec.PodLabels = ordererGroup.Spec.Common.PodLabels
		spec.Volumes = ordererGroup.Spec.Common.Volumes
		spec.Affinity = ordererGroup.Spec.Common.Affinity
		spec.VolumeMounts = ordererGroup.Spec.Common.VolumeMounts
		spec.ImagePullSecrets = ordererGroup.Spec.Common.ImagePullSecrets
		spec.Tolerations = ordererGroup.Spec.Common.Tolerations
	}

	// Override with component-specific configuration
	if config != nil {
		if config.Replicas != 0 {
			spec.Replicas = config.Replicas
		}
		if config.Storage != nil {
			spec.Storage = config.Storage
		}
		if config.Resources != nil {
			spec.Resources = config.Resources
		}
		if config.SecurityContext != nil {
			spec.SecurityContext = config.SecurityContext
		}
		if config.PodAnnotations != nil {
			spec.PodAnnotations = config.PodAnnotations
		}
		if config.PodLabels != nil {
			spec.PodLabels = config.PodLabels
		}
		if config.Volumes != nil {
			spec.Volumes = config.Volumes
		}
		if config.Affinity != nil {
			spec.Affinity = config.Affinity
		}
		if config.VolumeMounts != nil {
			spec.VolumeMounts = config.VolumeMounts
		}
		if config.ImagePullSecrets != nil {
			spec.ImagePullSecrets = config.ImagePullSecrets
		}
		if config.Tolerations != nil {
			spec.Tolerations = config.Tolerations
		}
		spec.Ingress = config.Ingress
		spec.Certificates = config.Certificates
		spec.Endpoints = config.Endpoints
		spec.Env = config.Env
		spec.Command = config.Command
		spec.Args = config.Args
	}

	return spec
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

	// Check if we should manage child CRDs
	manageChildCRDs := true
	if ordererGroup.Spec.ManageChildCRDs != nil {
		manageChildCRDs = *ordererGroup.Spec.ManageChildCRDs
	}

	if manageChildCRDs {
		// Cleanup child CRDs using the clientset
		if err := r.cleanupChildCRDs(ctx, ordererGroup); err != nil {
			errorMsg := fmt.Sprintf("Failed to cleanup child CRDs: %v", err)
			log.Error(err, "Failed to cleanup child CRDs")
			r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
			return ctrl.Result{}, err
		}
	} else {
		// Cleanup child components using existing controllers
		if err := r.cleanupChildComponents(ctx, ordererGroup); err != nil {
			errorMsg := fmt.Sprintf("Failed to cleanup child components: %v", err)
			log.Error(err, "Failed to cleanup child components")
			r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.FailedStatus, errorMsg)
			return ctrl.Result{}, err
		}
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

// cleanupChildCRDs cleans up all child CRDs of the OrdererGroup
func (r *OrdererGroupReconciler) cleanupChildCRDs(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	log := logf.FromContext(ctx)

	// Cleanup Consenter CRD
	if ordererGroup.Spec.Components.Consenter != nil {
		if err := r.cleanupConsenterCRD(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup consenter CRD")
		}
	}

	// Cleanup Assembler CRD
	if ordererGroup.Spec.Components.Assembler != nil {
		if err := r.cleanupAssemblerCRD(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup assembler CRD")
		}
	}

	// Cleanup Router CRD
	if ordererGroup.Spec.Components.Router != nil {
		if err := r.cleanupRouterCRD(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup router CRD")
		}
	}

	// Cleanup Batcher CRDs
	if len(ordererGroup.Spec.Components.Batchers) > 0 {
		if err := r.cleanupBatcherCRDs(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher CRDs")
		}
	}

	log.Info("All child CRDs cleaned up successfully")
	return nil
}

// cleanupConsenterCRD deletes the Consenter CRD
func (r *OrdererGroupReconciler) cleanupConsenterCRD(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	consenterName := fmt.Sprintf("%s-consenter", ordererGroup.Name)
	return r.Clientset.ApiV1alpha1().OrdererConsenters(ordererGroup.Namespace).Delete(ctx, consenterName, metav1.DeleteOptions{})
}

// cleanupAssemblerCRD deletes the Assembler CRD
func (r *OrdererGroupReconciler) cleanupAssemblerCRD(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	assemblerName := fmt.Sprintf("%s-assembler", ordererGroup.Name)
	return r.Clientset.ApiV1alpha1().OrdererAssemblers(ordererGroup.Namespace).Delete(ctx, assemblerName, metav1.DeleteOptions{})
}

// cleanupRouterCRD deletes the Router CRD
func (r *OrdererGroupReconciler) cleanupRouterCRD(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	routerName := fmt.Sprintf("%s-router", ordererGroup.Name)
	return r.Clientset.ApiV1alpha1().OrdererRouters(ordererGroup.Namespace).Delete(ctx, routerName, metav1.DeleteOptions{})
}

// cleanupBatcherCRDs deletes all Batcher CRDs
func (r *OrdererGroupReconciler) cleanupBatcherCRDs(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	for i := range ordererGroup.Spec.Components.Batchers {
		batcherName := fmt.Sprintf("%s-batcher-%d", ordererGroup.Name, i)
		if err := r.Clientset.ApiV1alpha1().OrdererBatchers(ordererGroup.Namespace).Delete(ctx, batcherName, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete batcher CRD %s: %w", batcherName, err)
		}
	}
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
	// Initialize the clientset using the manager's config
	config := mgr.GetConfig()
	clientset, err := clientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}
	r.Clientset = clientset

	// Initialize component controllers (kept for backward compatibility)
	r.ConsenterController = ordgroup.NewConsenterController(r.Client, r.Scheme)
	r.AssemblerController = ordgroup.NewAssemblerController(r.Client, r.Scheme)
	r.RouterController = ordgroup.NewRouterController(r.Client, r.Scheme)
	r.BatcherController = ordgroup.NewBatcherController(r.Client, r.Scheme)

	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererGroup{}).
		Named("orderergroup").
		Complete(r)
}
