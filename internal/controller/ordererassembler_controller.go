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
	// OrdererAssemblerFinalizerName is the name of the finalizer used by OrdererAssembler
	OrdererAssemblerFinalizerName = "ordererassembler.fabricx.kfsoft.tech/finalizer"
)

// OrdererAssemblerReconciler reconciles a OrdererAssembler object
type OrdererAssemblerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererassemblers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererassemblers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererassemblers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OrdererAssemblerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererAssembler reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the OrdererAssembler status to failed
			ordererAssembler := &fabricxv1alpha1.OrdererAssembler{}
			if err := r.Get(ctx, req.NamespacedName, ordererAssembler); err == nil {
				panicMsg := fmt.Sprintf("Panic in OrdererAssembler reconciliation: %v", panicErr)
				r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var ordererAssembler fabricxv1alpha1.OrdererAssembler
	if err := r.Get(ctx, req.NamespacedName, &ordererAssembler); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the OrdererAssembler is being deleted
	if !ordererAssembler.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &ordererAssembler)
	}

	// Set initial status if not set
	if ordererAssembler.Status.Status == "" {
		r.updateOrdererAssemblerStatus(ctx, &ordererAssembler, fabricxv1alpha1.PendingStatus, "Initializing OrdererAssembler")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &ordererAssembler); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateOrdererAssemblerStatus(ctx, &ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the OrdererAssembler
	if err := r.reconcileOrdererAssembler(ctx, &ordererAssembler); err != nil {
		// The reconcileOrdererAssembler method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if ordererAssembler.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile OrdererAssembler: %v", err)
			r.updateOrdererAssemblerStatus(ctx, &ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile OrdererAssembler")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileOrdererAssembler handles the reconciliation of an OrdererAssembler
func (r *OrdererAssemblerReconciler) reconcileOrdererAssembler(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererAssembler reconciliation",
				"ordererAssembler", ordererAssembler.Name, "namespace", ordererAssembler.Namespace)

			// Update the OrdererAssembler status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererAssembler reconciliation: %v", panicErr)
			r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Starting OrdererAssembler reconciliation",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace,
		"deploymentMode", ordererAssembler.Spec.DeploymentMode)

	// Determine deployment mode
	deploymentMode := ordererAssembler.Spec.DeploymentMode
	if deploymentMode == "" {
		deploymentMode = "deploy" // Default to deploy mode
	}

	// Reconcile based on deployment mode
	switch deploymentMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, ordererAssembler); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, ordererAssembler); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid deployment mode: %s", deploymentMode)
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid deployment mode")
		r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	// Update status to success
	r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.RunningStatus, "OrdererAssembler reconciled successfully")

	log.Info("OrdererAssembler reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *OrdererAssemblerReconciler) reconcileConfigureMode(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererAssembler in configure mode",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace)

	// In configure mode, only create certificates
	if err := r.reconcileCertificates(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("OrdererAssembler certificates created in configure mode")

	log.Info("OrdererAssembler configure mode reconciliation completed")
	return nil
}

// reconcileCertificates creates or updates certificates for the OrdererAssembler
func (r *OrdererAssemblerReconciler) reconcileCertificates(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	// For individual OrdererAssembler instances, we'll create a simple certificate approach
	// that doesn't rely on the OrdererGroup certificate service

	// Check if certificates are configured
	if ordererAssembler.Spec.Certificates == nil {
		log.Info("No certificate configuration found, skipping certificate creation")
		return nil
	}

	// Create sign certificate secret
	signSecretName := fmt.Sprintf("%s-sign-cert", ordererAssembler.Name)
	if err := r.createCertificateSecret(ctx, ordererAssembler, signSecretName, "sign"); err != nil {
		return fmt.Errorf("failed to create sign certificate secret: %w", err)
	}

	// Create TLS certificate secret
	tlsSecretName := fmt.Sprintf("%s-tls-cert", ordererAssembler.Name)
	if err := r.createCertificateSecret(ctx, ordererAssembler, tlsSecretName, "tls"); err != nil {
		return fmt.Errorf("failed to create TLS certificate secret: %w", err)
	}

	log.Info("Certificates reconciled successfully", "assembler", ordererAssembler.Name)
	return nil
}

// createCertificateSecret creates a certificate secret for the OrdererAssembler
func (r *OrdererAssemblerReconciler) createCertificateSecret(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler, secretName, certType string) error {
	log := logf.FromContext(ctx)

	// Check if secret already exists
	existingSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: ordererAssembler.Namespace,
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
			Namespace: ordererAssembler.Namespace,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererassembler":         ordererAssembler.Name,
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
	if err := controllerutil.SetControllerReference(ordererAssembler, secret, r.Scheme); err != nil {
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
func (r *OrdererAssemblerReconciler) reconcileDeployMode(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererAssembler in deploy mode",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace)

	// 1. Create/Update certificates first
	if err := r.reconcileCertificates(ctx, ordererAssembler); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// TODO: Implement full deployment resource creation
	// - Create ConfigMap for Assembler configuration
	// - Create Service for Assembler
	// - Create Deployment for Assembler
	// - Create PVC if needed
	// - Create Ingress if configured

	log.Info("OrdererAssembler deploy mode reconciliation completed")
	return nil
}

// handleDeletion handles the deletion of an OrdererAssembler
func (r *OrdererAssemblerReconciler) handleDeletion(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererAssembler deletion",
				"ordererAssembler", ordererAssembler.Name, "namespace", ordererAssembler.Namespace)

			// Update the OrdererAssembler status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererAssembler deletion: %v", panicErr)
			r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Handling OrdererAssembler deletion",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace)

	// Set status to indicate deletion
	r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.PendingStatus, "Deleting OrdererAssembler resources")

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, ordererAssembler); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateOrdererAssemblerStatus(ctx, ordererAssembler, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("OrdererAssembler deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the OrdererAssembler
func (r *OrdererAssemblerReconciler) ensureFinalizer(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	if !utils.ContainsString(ordererAssembler.Finalizers, OrdererAssemblerFinalizerName) {
		ordererAssembler.Finalizers = append(ordererAssembler.Finalizers, OrdererAssemblerFinalizerName)
		return r.Update(ctx, ordererAssembler)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererAssembler
func (r *OrdererAssemblerReconciler) removeFinalizer(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler) error {
	ordererAssembler.Finalizers = utils.RemoveString(ordererAssembler.Finalizers, OrdererAssemblerFinalizerName)
	return r.Update(ctx, ordererAssembler)
}

// updateOrdererAssemblerStatus updates the OrdererAssembler status with the given status and message
func (r *OrdererAssemblerReconciler) updateOrdererAssemblerStatus(ctx context.Context, ordererAssembler *fabricxv1alpha1.OrdererAssembler, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating OrdererAssembler status",
		"name", ordererAssembler.Name,
		"namespace", ordererAssembler.Namespace,
		"status", status,
		"message", message)

	// Update the status
	ordererAssembler.Status.Status = status
	ordererAssembler.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	ordererAssembler.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, ordererAssembler); err != nil {
		log.Error(err, "Failed to update OrdererAssembler status")
	} else {
		log.Info("OrdererAssembler status updated successfully",
			"name", ordererAssembler.Name,
			"namespace", ordererAssembler.Namespace,
			"status", status,
			"message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererAssemblerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererAssembler{}).
		Named("ordererassembler").
		Complete(r)
}
