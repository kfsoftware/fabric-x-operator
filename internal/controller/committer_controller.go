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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/committer"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

// CommitterReconciler reconciles a Committer object
type CommitterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committers/finalizers,verbs=update

const (
	committerFinalizerName = "committer.fabricx.kfsoft.tech/finalizer"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CommitterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Committer instance
	committerInstance := &fabricxv1alpha1.Committer{}
	err := r.Get(ctx, req.NamespacedName, committerInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			log.Info("Committer resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Committer")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling Committer",
		"name", committerInstance.Name,
		"namespace", committerInstance.Namespace,
		"bootstrapMode", committerInstance.Spec.BootstrapMode)

	// Check if the instance is marked for deletion
	if committerInstance.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, committerInstance)
	}

	// Add finalizer if not present
	if !utils.ContainsString(committerInstance.ObjectMeta.Finalizers, committerFinalizerName) {
		committerInstance.ObjectMeta.Finalizers = append(committerInstance.ObjectMeta.Finalizers, committerFinalizerName)
		if err := r.Update(ctx, committerInstance); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize status if not present
	if committerInstance.Status.ComponentStatuses == nil {
		committerInstance.Status.ComponentStatuses = make(map[string]fabricxv1alpha1.ComponentStatus)
	}

	// Create component controllers
	coordinatorController := committer.NewCoordinatorController(r.Client, r.Scheme)
	sidecarController := committer.NewSidecarController(r.Client, r.Scheme)
	validatorController := committer.NewValidatorController(r.Client, r.Scheme)
	verifierServiceController := committer.NewVerifierServiceController(r.Client, r.Scheme)

	// Track overall status
	var overallError error
	componentErrors := make(map[string]error)

	// Reconcile each component
	if committerInstance.Spec.Components.Coordinator != nil {
		if err := coordinatorController.Reconcile(ctx, committerInstance, committerInstance.Spec.Components.Coordinator); err != nil {
			log.Error(err, "Failed to reconcile Coordinator component")
			componentErrors["coordinator"] = err
			overallError = fmt.Errorf("coordinator reconciliation failed: %w", err)
		} else {
			// Update component status
			committerInstance.Status.ComponentStatuses["coordinator"] = fabricxv1alpha1.ComponentStatus{
				Ready:          true,
				ReplicasReady:  committerInstance.Spec.Components.Coordinator.Replicas,
				ReplicasTotal:  committerInstance.Spec.Components.Coordinator.Replicas,
				LastUpdateTime: metav1.Now(),
			}
		}
	}

	if committerInstance.Spec.Components.Sidecar != nil {
		if err := sidecarController.Reconcile(ctx, committerInstance, committerInstance.Spec.Components.Sidecar); err != nil {
			log.Error(err, "Failed to reconcile Sidecar component")
			componentErrors["sidecar"] = err
			overallError = fmt.Errorf("sidecar reconciliation failed: %w", err)
		} else {
			// Update component status
			committerInstance.Status.ComponentStatuses["sidecar"] = fabricxv1alpha1.ComponentStatus{
				Ready:          true,
				ReplicasReady:  committerInstance.Spec.Components.Sidecar.Replicas,
				ReplicasTotal:  committerInstance.Spec.Components.Sidecar.Replicas,
				LastUpdateTime: metav1.Now(),
			}
		}
	}

	if committerInstance.Spec.Components.Validator != nil {
		if err := validatorController.Reconcile(ctx, committerInstance, committerInstance.Spec.Components.Validator); err != nil {
			log.Error(err, "Failed to reconcile Validator component")
			componentErrors["validator"] = err
			overallError = fmt.Errorf("validator reconciliation failed: %w", err)
		} else {
			// Update component status
			committerInstance.Status.ComponentStatuses["validator"] = fabricxv1alpha1.ComponentStatus{
				Ready:          true,
				ReplicasReady:  committerInstance.Spec.Components.Validator.Replicas,
				ReplicasTotal:  committerInstance.Spec.Components.Validator.Replicas,
				LastUpdateTime: metav1.Now(),
			}
		}
	}

	if committerInstance.Spec.Components.VerifierService != nil {
		if err := verifierServiceController.Reconcile(ctx, committerInstance, committerInstance.Spec.Components.VerifierService); err != nil {
			log.Error(err, "Failed to reconcile VerifierService component")
			componentErrors["verifier-service"] = err
			overallError = fmt.Errorf("verifier-service reconciliation failed: %w", err)
		} else {
			// Update component status
			committerInstance.Status.ComponentStatuses["verifier-service"] = fabricxv1alpha1.ComponentStatus{
				Ready:          true,
				ReplicasReady:  committerInstance.Spec.Components.VerifierService.Replicas,
				ReplicasTotal:  committerInstance.Spec.Components.VerifierService.Replicas,
				LastUpdateTime: metav1.Now(),
			}
		}
	}

	// Update overall status
	if overallError != nil {
		committerInstance.Status.Status = fabricxv1alpha1.FailedStatus
		committerInstance.Status.Message = overallError.Error()
		committerInstance.Status.Phase = "Failed"
	} else {
		committerInstance.Status.Status = fabricxv1alpha1.RunningStatus
		committerInstance.Status.Message = "All components reconciled successfully"
		committerInstance.Status.Phase = "Running"
	}

	// Update the status
	if err := r.Status().Update(ctx, committerInstance); err != nil {
		log.Error(err, "Failed to update Committer status")
		return ctrl.Result{}, err
	}

	if overallError != nil {
		log.Error(overallError, "Committer reconciliation failed")
		return ctrl.Result{RequeueAfter: time.Minute}, overallError
	}

	log.Info("Committer reconciled successfully")
	return ctrl.Result{}, nil
}

// handleDeletion handles the cleanup when a Committer is being deleted
func (r *CommitterReconciler) handleDeletion(ctx context.Context, committerInstance *fabricxv1alpha1.Committer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Handling deletion of Committer",
		"name", committerInstance.Name,
		"namespace", committerInstance.Namespace)

	// Create component controllers for cleanup
	coordinatorController := committer.NewCoordinatorController(r.Client, r.Scheme)
	sidecarController := committer.NewSidecarController(r.Client, r.Scheme)
	validatorController := committer.NewValidatorController(r.Client, r.Scheme)
	verifierServiceController := committer.NewVerifierServiceController(r.Client, r.Scheme)

	// Cleanup each component
	if committerInstance.Spec.Components.Coordinator != nil {
		if err := coordinatorController.Cleanup(ctx, committerInstance, committerInstance.Spec.Components.Coordinator); err != nil {
			log.Error(err, "Failed to cleanup Coordinator component")
		}
	}

	if committerInstance.Spec.Components.Sidecar != nil {
		if err := sidecarController.Cleanup(ctx, committerInstance, committerInstance.Spec.Components.Sidecar); err != nil {
			log.Error(err, "Failed to cleanup Sidecar component")
		}
	}

	if committerInstance.Spec.Components.Validator != nil {
		if err := validatorController.Cleanup(ctx, committerInstance, committerInstance.Spec.Components.Validator); err != nil {
			log.Error(err, "Failed to cleanup Validator component")
		}
	}

	if committerInstance.Spec.Components.VerifierService != nil {
		if err := verifierServiceController.Cleanup(ctx, committerInstance, committerInstance.Spec.Components.VerifierService); err != nil {
			log.Error(err, "Failed to cleanup VerifierService component")
		}
	}

	// Remove finalizer
	committerInstance.ObjectMeta.Finalizers = utils.RemoveString(committerInstance.ObjectMeta.Finalizers, committerFinalizerName)
	if err := r.Update(ctx, committerInstance); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Committer deletion completed")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CommitterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.Committer{}).
		Named("committer").
		Complete(r)
}
