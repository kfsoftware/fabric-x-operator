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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

const (
	// OrdererConsenterFinalizerName is the name of the finalizer used by OrdererConsenter
	OrdererConsenterFinalizerName = "ordererconsenter.fabricx.kfsoft.tech/finalizer"
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
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
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
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid deployment mode")
		r.updateOrdererConsenterStatus(ctx, ordererConsenter, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
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

	// In configure mode, only create certificates
	if err := r.reconcileCertificates(ctx, ordererConsenter); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("OrdererConsenter certificates created in configure mode")

	log.Info("OrdererConsenter configure mode reconciliation completed")
	return nil
}

// reconcileCertificates creates or updates certificates for the OrdererConsenter
func (r *OrdererConsenterReconciler) reconcileCertificates(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) error {
	log := logf.FromContext(ctx)

	// For individual OrdererConsenter instances, we'll create a simple certificate approach
	// that doesn't rely on the OrdererGroup certificate service

	// Check if certificates are configured
	if ordererConsenter.Spec.Certificates == nil {
		log.Info("No certificate configuration found, skipping certificate creation")
		return nil
	}

	// Create sign certificate secret
	signSecretName := fmt.Sprintf("%s-sign-cert", ordererConsenter.Name)
	if err := r.createCertificateSecret(ctx, ordererConsenter, signSecretName, "sign"); err != nil {
		return fmt.Errorf("failed to create sign certificate secret: %w", err)
	}

	// Create TLS certificate secret
	tlsSecretName := fmt.Sprintf("%s-tls-cert", ordererConsenter.Name)
	if err := r.createCertificateSecret(ctx, ordererConsenter, tlsSecretName, "tls"); err != nil {
		return fmt.Errorf("failed to create TLS certificate secret: %w", err)
	}

	log.Info("Certificates reconciled successfully", "consenter", ordererConsenter.Name)
	return nil
}

// createCertificateSecret creates a certificate secret for the OrdererConsenter
func (r *OrdererConsenterReconciler) createCertificateSecret(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter, secretName, certType string) error {
	log := logf.FromContext(ctx)

	// Check if secret already exists
	existingSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: ordererConsenter.Namespace,
		Name:      secretName,
	}, existingSecret)

	if err == nil {
		// Secret exists, check if it has the required data
		if existingSecret.Data != nil {
			if _, hasCert := existingSecret.Data["cert.pem"]; hasCert {
				if _, hasKey := existingSecret.Data["key.pem"]; hasKey {
					if _, hasCA := existingSecret.Data["ca.pem"]; hasCA {
						log.Info("Certificate secret already exists, skipping creation",
							"secret", secretName, "certType", certType)
						return nil
					}
				}
			}
		}
	}

	// Create a simple certificate secret with placeholder data
	// In a real implementation, this would call the actual certificate service
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ordererConsenter.Namespace,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererconsenter":         ordererConsenter.Name,
				"certificate-type":         certType,
				"fabricx.kfsoft.tech/type": "certificate",
			},
		},
		Data: map[string][]byte{
			"cert.pem": []byte("placeholder-certificate-data"),
			"key.pem":  []byte("placeholder-key-data"),
			"ca.pem":   []byte("placeholder-ca-data"),
		},
	}

	// Set the controller reference
	if err := controllerutil.SetControllerReference(ordererConsenter, secret, r.Scheme); err != nil {
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

	log.Info("Created certificate secret", "secret", secretName, "certType", certType)
	return nil
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *OrdererConsenterReconciler) reconcileDeployMode(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererConsenter in deploy mode",
		"name", ordererConsenter.Name,
		"namespace", ordererConsenter.Namespace)

	// 1. Create/Update certificates first
	if err := r.reconcileCertificates(ctx, ordererConsenter); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// TODO: Implement full deployment resource creation
	// - Create ConfigMap for Consenter configuration
	// - Create Service for Consenter
	// - Create Deployment for Consenter
	// - Create PVC if needed
	// - Create Ingress if configured

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
	if !utils.ContainsString(ordererConsenter.Finalizers, OrdererConsenterFinalizerName) {
		ordererConsenter.Finalizers = append(ordererConsenter.Finalizers, OrdererConsenterFinalizerName)
		return r.Update(ctx, ordererConsenter)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererConsenter
func (r *OrdererConsenterReconciler) removeFinalizer(ctx context.Context, ordererConsenter *fabricxv1alpha1.OrdererConsenter) error {
	ordererConsenter.Finalizers = utils.RemoveString(ordererConsenter.Finalizers, OrdererConsenterFinalizerName)
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
