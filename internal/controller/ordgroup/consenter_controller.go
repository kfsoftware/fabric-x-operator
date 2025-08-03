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

// ConsenterController handles reconciliation for the Consenter component
type ConsenterController struct {
	BaseComponentController
}

// NewConsenterController creates a new Consenter controller
func NewConsenterController(client client.Client, scheme *runtime.Scheme) *ConsenterController {
	return &ConsenterController{
		BaseComponentController: NewBaseComponentController(client, scheme),
	}
}

// Reconcile reconciles the Consenter component
func (r *ConsenterController) Reconcile(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Consenter component",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace,
		"replicas", config.Replicas,
		"mode", ordererGroup.Spec.BootstrapMode)

	// Handle different modes
	switch ordererGroup.Spec.BootstrapMode {
	case "configure":
		// In configure mode, only create certificates
		if err := r.reconcileCertificates(ctx, ordererGroup, "consenter", config); err != nil {
			return fmt.Errorf("failed to reconcile consenter certificates: %w", err)
		}
		log.Info("Consenter certificates created in configure mode")
		return nil

	case "deploy":
		// In deploy mode, create all resources
		// 1. Create/Update certificates first
		if err := r.reconcileCertificates(ctx, ordererGroup, "consenter", config); err != nil {
			return fmt.Errorf("failed to reconcile consenter certificates: %w", err)
		}

		// 2. Create/Update ConfigMap for Consenter configuration
		if err := r.reconcileConfigMap(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile consenter configmap: %w", err)
		}

		// 3. Create/Update Service for Consenter
		if err := r.reconcileService(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile consenter service: %w", err)
		}

		// 4. Create/Update Deployment for Consenter
		if err := r.reconcileDeployment(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile consenter deployment: %w", err)
		}

		// 5. Create/Update Ingress for Consenter (if configured)
		if config.Ingress != nil {
			if err := r.reconcileIngress(ctx, ordererGroup, config); err != nil {
				return fmt.Errorf("failed to reconcile consenter ingress: %w", err)
			}
		}

	default:
		return fmt.Errorf("unknown bootstrap mode: %s", ordererGroup.Spec.BootstrapMode)
	}

	log.Info("Consenter component reconciled successfully")
	return nil
}

// Cleanup cleans up the Consenter component resources
func (r *ConsenterController) Cleanup(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Cleaning up Consenter component",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace)

	// Always cleanup certificates regardless of mode
	if err := r.cleanupCertificates(ctx, ordererGroup, "consenter"); err != nil {
		log.Error(err, "Failed to cleanup consenter certificates")
	}

	// Only cleanup other resources in deploy mode
	if ordererGroup.Spec.BootstrapMode == "deploy" {
		// 1. Delete Deployment
		if err := r.cleanupDeployment(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup consenter deployment")
		}

		// 2. Delete Service
		if err := r.cleanupService(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup consenter service")
		}

		// 3. Delete Ingress
		if err := r.cleanupIngress(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup consenter ingress")
		}

		// 4. Delete ConfigMap
		if err := r.cleanupConfigMap(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup consenter configmap")
		}
	}

	log.Info("Consenter component cleanup completed")
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap for Consenter configuration
func (r *ConsenterController) reconcileConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement ConfigMap reconciliation
	// This would create/update a ConfigMap containing the Consenter configuration
	// based on the provided config and ordererGroup spec
	return nil
}

// reconcileService creates or updates the Service for Consenter
func (r *ConsenterController) reconcileService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Service reconciliation
	// This would create/update a Service to expose the Consenter pods
	return nil
}

// reconcileDeployment creates or updates the Deployment for Consenter
func (r *ConsenterController) reconcileDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Deployment reconciliation
	// This would create/update a Deployment for the Consenter component
	// using the provided configuration (replicas, resources, volumes, etc.)
	return nil
}

// reconcileIngress creates or updates the Ingress for Consenter
func (r *ConsenterController) reconcileIngress(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// cleanupDeployment deletes the Consenter Deployment
func (r *ConsenterController) cleanupDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Deployment cleanup
	return nil
}

// cleanupService deletes the Consenter Service
func (r *ConsenterController) cleanupService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Service cleanup
	return nil
}

// cleanupIngress deletes the Consenter Ingress
func (r *ConsenterController) cleanupIngress(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Ingress cleanup
	return nil
}

// cleanupConfigMap deletes the Consenter ConfigMap
func (r *ConsenterController) cleanupConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement ConfigMap cleanup
	return nil
}
