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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

const (
	// CommitterFinalizerName is the name of the finalizer used by Committer
	CommitterFinalizerName = "committer.fabricx.kfsoft.tech/finalizer"
)

// CommitterReconciler reconciles a Committer object
type CommitterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committers/finalizers,verbs=update
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committercoordinators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committersidecars,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committervalidators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committerverifiers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CommitterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in Committer reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the Committer status to failed
			committer := &fabricxv1alpha1.Committer{}
			if err := r.Get(ctx, req.NamespacedName, committer); err == nil {
				panicMsg := fmt.Sprintf("Panic in Committer reconciliation: %v", panicErr)
				r.updateCommitterStatus(ctx, committer, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var committer fabricxv1alpha1.Committer
	if err := r.Get(ctx, req.NamespacedName, &committer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the Committer is being deleted
	if !committer.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &committer)
	}

	// Set initial status if not set
	if committer.Status.Status == "" {
		r.updateCommitterStatus(ctx, &committer, fabricxv1alpha1.PendingStatus, "Initializing Committer")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &committer); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateCommitterStatus(ctx, &committer, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the Committer
	if err := r.reconcileCommitter(ctx, &committer); err != nil {
		// The reconcileCommitter method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if committer.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile Committer: %v", err)
			r.updateCommitterStatus(ctx, &committer, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile Committer")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute to ensure continuous monitoring
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileCommitter handles the reconciliation of a Committer
func (r *CommitterReconciler) reconcileCommitter(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	log := logf.FromContext(ctx)

	log.Info("Starting Committer reconciliation",
		"name", committer.Name,
		"namespace", committer.Namespace,
		"bootstrapMode", committer.Spec.BootstrapMode)

	// Reconcile child components
	if err := r.reconcileChildComponents(ctx, committer); err != nil {
		return fmt.Errorf("failed to reconcile child components: %w", err)
	}

	// Reconcile child CRDs
	if err := r.reconcileChildCRDs(ctx, committer); err != nil {
		return fmt.Errorf("failed to reconcile child CRDs: %w", err)
	}

	// Update status to success
	r.updateCommitterStatus(ctx, committer, fabricxv1alpha1.RunningStatus, "Committer reconciled successfully")

	log.Info("Committer reconciliation completed successfully")
	return nil
}

// reconcileChildComponents handles the reconciliation of child components
func (r *CommitterReconciler) reconcileChildComponents(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	log := logf.FromContext(ctx)
	log.Info("Reconciling child components", "committer", committer.Name)
	// TODO: Implement child component reconciliation if needed
	return nil
}

// reconcileChildCRDs handles the reconciliation of child CRDs
func (r *CommitterReconciler) reconcileChildCRDs(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling child CRDs", "committer", committer.Name)

	// Reconcile Coordinator CRD
	if err := r.reconcileCoordinatorCRD(ctx, committer); err != nil {
		return fmt.Errorf("failed to reconcile coordinator CRD: %w", err)
	}

	// Reconcile Sidecar CRD
	if err := r.reconcileSidecarCRD(ctx, committer); err != nil {
		return fmt.Errorf("failed to reconcile sidecar CRD: %w", err)
	}

	// Reconcile Validator CRD
	if err := r.reconcileValidatorCRD(ctx, committer); err != nil {
		return fmt.Errorf("failed to reconcile validator CRD: %w", err)
	}

	// Reconcile Verifier CRD
	if err := r.reconcileVerifierCRD(ctx, committer); err != nil {
		return fmt.Errorf("failed to reconcile verifier CRD: %w", err)
	}

	return nil
}

// reconcileCoordinatorCRD creates or updates the Coordinator CRD
func (r *CommitterReconciler) reconcileCoordinatorCRD(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	log := logf.FromContext(ctx)

	coordinatorName := fmt.Sprintf("%s-coordinator", committer.Name)
	coordinator := &fabricxv1alpha1.CommitterCoordinator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coordinatorName,
			Namespace: committer.Namespace,
		},
	}

	// Check if coordinator already exists
	err := r.Get(ctx, client.ObjectKey{Name: coordinatorName, Namespace: committer.Namespace}, coordinator)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Coordinator doesn't exist, create it
			coordinator.Spec = r.buildCoordinatorSpec(committer)
			if err := r.Create(ctx, coordinator); err != nil {
				return fmt.Errorf("failed to create coordinator CRD: %w", err)
			}
			log.Info("Created coordinator CRD", "name", coordinatorName)
		} else {
			return fmt.Errorf("failed to check coordinator CRD: %w", err)
		}
	} else {
		// Coordinator exists, update it
		coordinator.Spec = r.buildCoordinatorSpec(committer)
		if err := r.Update(ctx, coordinator); err != nil {
			return fmt.Errorf("failed to update coordinator CRD: %w", err)
		}
		log.Info("Updated coordinator CRD", "name", coordinatorName)
	}

	return nil
}

// buildCoordinatorSpec builds the Coordinator spec from Committer spec
func (r *CommitterReconciler) buildCoordinatorSpec(committer *fabricxv1alpha1.Committer) fabricxv1alpha1.CommitterCoordinatorSpec {
	spec := fabricxv1alpha1.CommitterCoordinatorSpec{
		BootstrapMode: committer.Spec.BootstrapMode,
		MSPID:         committer.Spec.MSPID,
		PartyID:       1, // Default PartyID for committer components
		Replicas:      1,
	}

	// Copy common configuration if available
	if committer.Spec.Common != nil {
		spec.Replicas = committer.Spec.Common.Replicas
		spec.Storage = committer.Spec.Common.Storage
		spec.Resources = committer.Spec.Common.Resources
		spec.SecurityContext = committer.Spec.Common.SecurityContext
		spec.PodAnnotations = committer.Spec.Common.PodAnnotations
		spec.PodLabels = committer.Spec.Common.PodLabels
		spec.Volumes = committer.Spec.Common.Volumes
		spec.Affinity = committer.Spec.Common.Affinity
		spec.VolumeMounts = committer.Spec.Common.VolumeMounts
		spec.ImagePullSecrets = committer.Spec.Common.ImagePullSecrets
		spec.Tolerations = committer.Spec.Common.Tolerations
	}

	// Copy component-specific configuration if available
	if committer.Spec.Components.Coordinator != nil {
		spec.Ingress = committer.Spec.Components.Coordinator.Ingress
		spec.Enrollment = committer.Spec.Components.Coordinator.Enrollment
		spec.SANS = committer.Spec.Components.Coordinator.SANS
		spec.Endpoints = committer.Spec.Components.Coordinator.Endpoints
		spec.Env = committer.Spec.Components.Coordinator.Env
		spec.Command = committer.Spec.Components.Coordinator.Command
		spec.Args = committer.Spec.Components.Coordinator.Args
		// Copy coordinator endpoints arrays if provided at parent level
		spec.VerifierEndpoints = committer.Spec.Components.CoordinatorVerifierEndpoints
		spec.ValidatorCommitterEndpoints = committer.Spec.Components.CoordinatorValidatorCommitterEndpoints
	}

	return spec
}

// reconcileSidecarCRD creates or updates the Sidecar CRD
func (r *CommitterReconciler) reconcileSidecarCRD(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	log := logf.FromContext(ctx)

	sidecarName := fmt.Sprintf("%s-sidecar", committer.Name)
	sidecar := &fabricxv1alpha1.CommitterSidecar{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sidecarName,
			Namespace: committer.Namespace,
		},
	}

	// Check if sidecar already exists
	err := r.Get(ctx, client.ObjectKey{Name: sidecarName, Namespace: committer.Namespace}, sidecar)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Sidecar doesn't exist, create it
			sidecar.Spec = r.buildSidecarSpec(committer)
			if err := r.Create(ctx, sidecar); err != nil {
				return fmt.Errorf("failed to create sidecar CRD: %w", err)
			}
			log.Info("Created sidecar CRD", "name", sidecarName)
		} else {
			return fmt.Errorf("failed to check sidecar CRD: %w", err)
		}
	} else {
		// Sidecar exists, update it
		sidecar.Spec = r.buildSidecarSpec(committer)
		if err := r.Update(ctx, sidecar); err != nil {
			return fmt.Errorf("failed to update sidecar CRD: %w", err)
		}
		log.Info("Updated sidecar CRD", "name", sidecarName)
	}

	return nil
}

// buildSidecarSpec builds the Sidecar spec from Committer spec
func (r *CommitterReconciler) buildSidecarSpec(committer *fabricxv1alpha1.Committer) fabricxv1alpha1.CommitterSidecarSpec {
	spec := fabricxv1alpha1.CommitterSidecarSpec{
		BootstrapMode: committer.Spec.BootstrapMode,
		MSPID:         committer.Spec.MSPID,
		PartyID:       1, // Default PartyID for committer components
		Replicas:      1,
		Genesis:       committer.Spec.Genesis,
	}

	// Copy common configuration if available
	if committer.Spec.Common != nil {
		spec.Replicas = committer.Spec.Common.Replicas
		spec.Storage = committer.Spec.Common.Storage
		spec.Resources = committer.Spec.Common.Resources
		spec.SecurityContext = committer.Spec.Common.SecurityContext
		spec.PodAnnotations = committer.Spec.Common.PodAnnotations
		spec.PodLabels = committer.Spec.Common.PodLabels
		spec.Volumes = committer.Spec.Common.Volumes
		spec.Affinity = committer.Spec.Common.Affinity
		spec.VolumeMounts = committer.Spec.Common.VolumeMounts
		spec.ImagePullSecrets = committer.Spec.Common.ImagePullSecrets
		spec.Tolerations = committer.Spec.Common.Tolerations
	}

	// Copy component-specific configuration if available
	if committer.Spec.Components.Sidecar != nil {
		spec.Ingress = committer.Spec.Components.Sidecar.Ingress
		spec.Enrollment = committer.Spec.Components.Sidecar.Enrollment
		spec.SANS = committer.Spec.Components.Sidecar.SANS
		spec.Endpoints = committer.Spec.Components.Sidecar.Endpoints
		spec.Env = committer.Spec.Components.Sidecar.Env
		spec.Command = committer.Spec.Components.Sidecar.Command
		spec.Args = committer.Spec.Components.Sidecar.Args
	}

	// Resolve orderer endpoints precedence: component-level overrides parent-level shared config
	if committer.Spec.Components.Sidecar != nil && len(committer.Spec.Components.Sidecar.Endpoints) > 0 {
		spec.OrdererEndpoints = append([]string{}, committer.Spec.Components.Sidecar.Endpoints...)
	} else if len(committer.Spec.Components.OrdererEndpoints) > 0 {
		spec.OrdererEndpoints = append([]string{}, committer.Spec.Components.OrdererEndpoints...)
	}

	// Pass down committer (coordinator) endpoint for sidecar if provided at parent-level
	if committer.Spec.Components.CommitterHost != "" {
		spec.CommitterHost = committer.Spec.Components.CommitterHost
	}
	if committer.Spec.Components.CommitterPort != 0 {
		spec.CommitterPort = committer.Spec.Components.CommitterPort
	}

	return spec
}

// reconcileValidatorCRD creates or updates the Validator CRD
func (r *CommitterReconciler) reconcileValidatorCRD(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	log := logf.FromContext(ctx)

	validatorName := fmt.Sprintf("%s-validator", committer.Name)
	validator := &fabricxv1alpha1.CommitterValidator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      validatorName,
			Namespace: committer.Namespace,
		},
	}

	// Check if validator already exists
	err := r.Get(ctx, client.ObjectKey{Name: validatorName, Namespace: committer.Namespace}, validator)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Validator doesn't exist, create it
			validator.Spec = r.buildValidatorSpec(committer)
			if err := r.Create(ctx, validator); err != nil {
				return fmt.Errorf("failed to create validator CRD: %w", err)
			}
			log.Info("Created validator CRD", "name", validatorName)
		} else {
			return fmt.Errorf("failed to check validator CRD: %w", err)
		}
	} else {
		// Validator exists, update it
		validator.Spec = r.buildValidatorSpec(committer)
		if err := r.Update(ctx, validator); err != nil {
			return fmt.Errorf("failed to update validator CRD: %w", err)
		}
		log.Info("Updated validator CRD", "name", validatorName)
	}

	return nil
}

// buildValidatorSpec builds the Validator spec from Committer spec
func (r *CommitterReconciler) buildValidatorSpec(committer *fabricxv1alpha1.Committer) fabricxv1alpha1.CommitterValidatorSpec {
	spec := fabricxv1alpha1.CommitterValidatorSpec{
		BootstrapMode: committer.Spec.BootstrapMode,
		MSPID:         committer.Spec.MSPID,
		PartyID:       1, // Default PartyID for committer components
		Replicas:      1,
	}

	// Copy common configuration if available
	if committer.Spec.Common != nil {
		spec.Replicas = committer.Spec.Common.Replicas
		spec.Storage = committer.Spec.Common.Storage
		spec.Resources = committer.Spec.Common.Resources
		spec.SecurityContext = committer.Spec.Common.SecurityContext
		spec.PodAnnotations = committer.Spec.Common.PodAnnotations
		spec.PodLabels = committer.Spec.Common.PodLabels
		spec.Volumes = committer.Spec.Common.Volumes
		spec.Affinity = committer.Spec.Common.Affinity
		spec.VolumeMounts = committer.Spec.Common.VolumeMounts
		spec.ImagePullSecrets = committer.Spec.Common.ImagePullSecrets
		spec.Tolerations = committer.Spec.Common.Tolerations
	}

	// Copy component-specific configuration if available
	if committer.Spec.Components.Validator != nil {
		spec.Ingress = committer.Spec.Components.Validator.Ingress
		spec.Enrollment = committer.Spec.Components.Validator.Enrollment
		spec.SANS = committer.Spec.Components.Validator.SANS
		spec.Endpoints = committer.Spec.Components.Validator.Endpoints
		spec.Env = committer.Spec.Components.Validator.Env
		spec.Command = committer.Spec.Components.Validator.Command
		spec.Args = committer.Spec.Components.Validator.Args
		spec.PostgreSQL = committer.Spec.Components.Validator.PostgreSQL
	}

	return spec
}

// reconcileVerifierCRD creates or updates the Verifier CRD
func (r *CommitterReconciler) reconcileVerifierCRD(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	log := logf.FromContext(ctx)

	verifierName := fmt.Sprintf("%s-verifier", committer.Name)
	verifier := &fabricxv1alpha1.CommitterVerifier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      verifierName,
			Namespace: committer.Namespace,
		},
	}

	// Check if verifier already exists
	err := r.Get(ctx, client.ObjectKey{Name: verifierName, Namespace: committer.Namespace}, verifier)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Verifier doesn't exist, create it
			verifier.Spec = r.buildVerifierSpec(committer)
			if err := r.Create(ctx, verifier); err != nil {
				return fmt.Errorf("failed to create verifier CRD: %w", err)
			}
			log.Info("Created verifier CRD", "name", verifierName)
		} else {
			return fmt.Errorf("failed to check verifier CRD: %w", err)
		}
	} else {
		// Verifier exists, update it
		verifier.Spec = r.buildVerifierSpec(committer)
		if err := r.Update(ctx, verifier); err != nil {
			return fmt.Errorf("failed to update verifier CRD: %w", err)
		}
		log.Info("Updated verifier CRD", "name", verifierName)
	}

	return nil
}

// buildVerifierSpec builds the Verifier spec from Committer spec
func (r *CommitterReconciler) buildVerifierSpec(committer *fabricxv1alpha1.Committer) fabricxv1alpha1.CommitterVerifierSpec {
	spec := fabricxv1alpha1.CommitterVerifierSpec{
		BootstrapMode: committer.Spec.BootstrapMode,
		MSPID:         committer.Spec.MSPID,
		PartyID:       1, // Default PartyID for committer components
		Replicas:      1,
	}

	// Copy common configuration if available
	if committer.Spec.Common != nil {
		spec.Replicas = committer.Spec.Common.Replicas
		spec.Storage = committer.Spec.Common.Storage
		spec.Resources = committer.Spec.Common.Resources
		spec.SecurityContext = committer.Spec.Common.SecurityContext
		spec.PodAnnotations = committer.Spec.Common.PodAnnotations
		spec.PodLabels = committer.Spec.Common.PodLabels
		spec.Volumes = committer.Spec.Common.Volumes
		spec.Affinity = committer.Spec.Common.Affinity
		spec.VolumeMounts = committer.Spec.Common.VolumeMounts
		spec.ImagePullSecrets = committer.Spec.Common.ImagePullSecrets
		spec.Tolerations = committer.Spec.Common.Tolerations
	}

	// Copy component-specific configuration if available
	if committer.Spec.Components.VerifierService != nil {
		spec.Ingress = committer.Spec.Components.VerifierService.Ingress
		spec.Enrollment = committer.Spec.Components.VerifierService.Enrollment
		spec.SANS = committer.Spec.Components.VerifierService.SANS
		spec.Endpoints = committer.Spec.Components.VerifierService.Endpoints
		spec.Env = committer.Spec.Components.VerifierService.Env
		spec.Command = committer.Spec.Components.VerifierService.Command
		spec.Args = committer.Spec.Components.VerifierService.Args
	}

	return spec
}

// handleDeletion handles the deletion of a Committer
func (r *CommitterReconciler) handleDeletion(ctx context.Context, committer *fabricxv1alpha1.Committer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Handling Committer deletion",
		"name", committer.Name,
		"namespace", committer.Namespace)

	// Set status to indicate deletion
	r.updateCommitterStatus(ctx, committer, fabricxv1alpha1.PendingStatus, "Deleting Committer resources")

	// Clean up child CRDs
	if err := r.cleanupChildCRDs(ctx, committer); err != nil {
		errorMsg := fmt.Sprintf("Failed to cleanup child CRDs: %v", err)
		log.Error(err, "Failed to cleanup child CRDs")
		r.updateCommitterStatus(ctx, committer, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Remove finalizer
	if err := r.removeFinalizer(ctx, committer); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateCommitterStatus(ctx, committer, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("Committer deletion completed successfully")
	return ctrl.Result{}, nil
}

// cleanupChildCRDs cleans up child CRDs
func (r *CommitterReconciler) cleanupChildCRDs(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	log := logf.FromContext(ctx)

	// Clean up Coordinator CRD
	if err := r.cleanupCoordinatorCRD(ctx, committer); err != nil {
		return fmt.Errorf("failed to cleanup coordinator CRD: %w", err)
	}

	// Clean up Sidecar CRD
	if err := r.cleanupSidecarCRD(ctx, committer); err != nil {
		return fmt.Errorf("failed to cleanup sidecar CRD: %w", err)
	}

	// Clean up Validator CRD
	if err := r.cleanupValidatorCRD(ctx, committer); err != nil {
		return fmt.Errorf("failed to cleanup validator CRD: %w", err)
	}

	// Clean up Verifier CRD
	if err := r.cleanupVerifierCRD(ctx, committer); err != nil {
		return fmt.Errorf("failed to cleanup verifier CRD: %w", err)
	}

	log.Info("Child CRDs cleaned up successfully", "committer", committer.Name)
	return nil
}

// cleanupCoordinatorCRD cleans up the Coordinator CRD
func (r *CommitterReconciler) cleanupCoordinatorCRD(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	coordinatorName := fmt.Sprintf("%s-coordinator", committer.Name)
	coordinator := &fabricxv1alpha1.CommitterCoordinator{}

	if err := r.Get(ctx, client.ObjectKey{Name: coordinatorName, Namespace: committer.Namespace}, coordinator); err == nil {
		if err := r.Delete(ctx, coordinator); err != nil {
			return fmt.Errorf("failed to delete coordinator CRD: %w", err)
		}
	}

	return nil
}

// cleanupSidecarCRD cleans up the Sidecar CRD
func (r *CommitterReconciler) cleanupSidecarCRD(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	sidecarName := fmt.Sprintf("%s-sidecar", committer.Name)
	sidecar := &fabricxv1alpha1.CommitterSidecar{}

	if err := r.Get(ctx, client.ObjectKey{Name: sidecarName, Namespace: committer.Namespace}, sidecar); err == nil {
		if err := r.Delete(ctx, sidecar); err != nil {
			return fmt.Errorf("failed to delete sidecar CRD: %w", err)
		}
	}

	return nil
}

// cleanupValidatorCRD cleans up the Validator CRD
func (r *CommitterReconciler) cleanupValidatorCRD(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	validatorName := fmt.Sprintf("%s-validator", committer.Name)
	validator := &fabricxv1alpha1.CommitterValidator{}

	if err := r.Get(ctx, client.ObjectKey{Name: validatorName, Namespace: committer.Namespace}, validator); err == nil {
		if err := r.Delete(ctx, validator); err != nil {
			return fmt.Errorf("failed to delete validator CRD: %w", err)
		}
	}

	return nil
}

// cleanupVerifierCRD cleans up the Verifier CRD
func (r *CommitterReconciler) cleanupVerifierCRD(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	verifierName := fmt.Sprintf("%s-verifier", committer.Name)
	verifier := &fabricxv1alpha1.CommitterVerifier{}

	if err := r.Get(ctx, client.ObjectKey{Name: verifierName, Namespace: committer.Namespace}, verifier); err == nil {
		if err := r.Delete(ctx, verifier); err != nil {
			return fmt.Errorf("failed to delete verifier CRD: %w", err)
		}
	}

	return nil
}

// ensureFinalizer ensures the finalizer is present on the Committer
func (r *CommitterReconciler) ensureFinalizer(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	if !utils.ContainsString(committer.Finalizers, CommitterFinalizerName) {
		committer.Finalizers = append(committer.Finalizers, CommitterFinalizerName)
		return r.Update(ctx, committer)
	}
	return nil
}

// removeFinalizer removes the finalizer from the Committer
func (r *CommitterReconciler) removeFinalizer(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	committer.Finalizers = utils.RemoveString(committer.Finalizers, CommitterFinalizerName)
	return r.Update(ctx, committer)
}

// updateCommitterStatus updates the Committer status with the given status and message
func (r *CommitterReconciler) updateCommitterStatus(ctx context.Context, committer *fabricxv1alpha1.Committer, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating Committer status",
		"name", committer.Name,
		"namespace", committer.Namespace,
		"status", status,
		"message", message)

	// Update the status
	committer.Status.Status = status
	committer.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	committer.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, committer); err != nil {
		log.Error(err, "Failed to update Committer status")
	} else {
		log.Info("Committer status updated successfully",
			"name", committer.Name,
			"namespace", committer.Namespace,
			"status", status,
			"message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CommitterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.Committer{}).
		Owns(&fabricxv1alpha1.CommitterCoordinator{}).
		Owns(&fabricxv1alpha1.CommitterSidecar{}).
		Owns(&fabricxv1alpha1.CommitterValidator{}).
		Owns(&fabricxv1alpha1.CommitterVerifier{}).
		Named("committer").
		Complete(r)
}
