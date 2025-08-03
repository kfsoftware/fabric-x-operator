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

package ordgroup

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

// BatcherController handles reconciliation for the Batcher component
type BatcherController struct {
	BaseComponentController
}

// NewBatcherController creates a new Batcher controller
func NewBatcherController(client client.Client, scheme *runtime.Scheme) *BatcherController {
	return &BatcherController{
		BaseComponentController: NewBaseComponentController(client, scheme),
	}
}

// Reconcile reconciles the Batcher component
func (r *BatcherController) Reconcile(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Batcher component",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace,
		"replicas", config.Replicas,
		"mode", ordererGroup.Spec.BootstrapMode)

	// Handle different modes
	switch ordererGroup.Spec.BootstrapMode {
	case "configure":
		// In configure mode, only create certificates
		if err := r.reconcileCertificates(ctx, ordererGroup, "batcher", config); err != nil {
			return fmt.Errorf("failed to reconcile batcher certificates: %w", err)
		}
		log.Info("Batcher certificates created in configure mode")
		return nil

	case "deploy":
		// In deploy mode, create all resources
		// 1. Create/Update certificates first
		if err := r.reconcileCertificates(ctx, ordererGroup, "batcher", config); err != nil {
			return fmt.Errorf("failed to reconcile batcher certificates: %w", err)
		}

		// 2. Create/Update ConfigMap for Batcher configuration
		if err := r.reconcileConfigMap(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile batcher configmap: %w", err)
		}

		// 3. Create/Update Service for Batcher
		if err := r.reconcileService(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile batcher service: %w", err)
		}

		// 4. Create/Update Deployment for Batcher
		if err := r.reconcileDeployment(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile batcher deployment: %w", err)
		}

		// 5. Create/Update Ingress for Batcher (if configured)
		if config.Ingress != nil {
			if err := r.reconcileIngress(ctx, ordererGroup, config); err != nil {
				return fmt.Errorf("failed to reconcile batcher ingress: %w", err)
			}
		}

	default:
		return fmt.Errorf("unknown bootstrap mode: %s", ordererGroup.Spec.BootstrapMode)
	}

	log.Info("Batcher component reconciled successfully")
	return nil
}

// Cleanup cleans up the Batcher component resources
func (r *BatcherController) Cleanup(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Cleaning up Batcher component",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace)

	// Always cleanup certificates regardless of mode
	if err := r.cleanupCertificates(ctx, ordererGroup, "batcher"); err != nil {
		log.Error(err, "Failed to cleanup batcher certificates")
	}

	// Only cleanup other resources in deploy mode
	if ordererGroup.Spec.BootstrapMode == "deploy" {
		// 1. Delete Deployment
		if err := r.cleanupDeployment(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher deployment")
		}

		// 2. Delete Service
		if err := r.cleanupService(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher service")
		}

		// 3. Delete Ingress
		if err := r.cleanupIngress(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher ingress")
		}

		// 4. Delete ConfigMap
		if err := r.cleanupConfigMap(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher configmap")
		}
	}

	log.Info("Batcher component cleanup completed")
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap for Batcher configuration
func (r *BatcherController) reconcileConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement ConfigMap reconciliation
	// This would create/update a ConfigMap containing the Batcher configuration
	// based on the provided config and ordererGroup spec
	return nil
}

// reconcileService creates or updates the Service for Batcher
func (r *BatcherController) reconcileService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Service reconciliation
	// This would create/update a Service to expose the Batcher pods
	return nil
}

// reconcileDeployment creates or updates the Deployment for Batcher
func (r *BatcherController) reconcileDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Deployment reconciliation
	// This would create/update a Deployment for the Batcher component
	// using the provided configuration (replicas, resources, volumes, etc.)
	return nil
}

// reconcileIngress creates or updates the Ingress for Batcher
func (r *BatcherController) reconcileIngress(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// cleanupDeployment deletes the Batcher Deployment
func (r *BatcherController) cleanupDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Deployment cleanup
	return nil
}

// cleanupService deletes the Batcher Service
func (r *BatcherController) cleanupService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Service cleanup
	return nil
}

// cleanupIngress deletes the Batcher Ingress
func (r *BatcherController) cleanupIngress(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Ingress cleanup
	return nil
}

// cleanupConfigMap deletes the Batcher ConfigMap
func (r *BatcherController) cleanupConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement ConfigMap cleanup
	return nil
}
