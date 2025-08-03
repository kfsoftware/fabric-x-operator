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

// AssemblerController handles reconciliation for the Assembler component
type AssemblerController struct {
	BaseComponentController
}

// NewAssemblerController creates a new Assembler controller
func NewAssemblerController(client client.Client, scheme *runtime.Scheme) *AssemblerController {
	return &AssemblerController{
		BaseComponentController: NewBaseComponentController(client, scheme),
	}
}

// Reconcile reconciles the Assembler component
func (r *AssemblerController) Reconcile(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Assembler component",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace,
		"replicas", config.Replicas,
		"mode", ordererGroup.Spec.BootstrapMode)

	// Handle different modes
	switch ordererGroup.Spec.BootstrapMode {
	case "configure":
		// In configure mode, only create certificates
		if err := r.reconcileCertificates(ctx, ordererGroup, "assembler", config); err != nil {
			return fmt.Errorf("failed to reconcile assembler certificates: %w", err)
		}
		log.Info("Assembler certificates created in configure mode")
		return nil

	case "deploy":
		// In deploy mode, create all resources
		// 1. Create/Update certificates first
		if err := r.reconcileCertificates(ctx, ordererGroup, "assembler", config); err != nil {
			return fmt.Errorf("failed to reconcile assembler certificates: %w", err)
		}

		// 2. Create/Update ConfigMap for Assembler configuration
		if err := r.reconcileConfigMap(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile assembler configmap: %w", err)
		}

		// 3. Create/Update Service for Assembler
		if err := r.reconcileService(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile assembler service: %w", err)
		}

		// 4. Create/Update Deployment for Assembler
		if err := r.reconcileDeployment(ctx, ordererGroup, config); err != nil {
			return fmt.Errorf("failed to reconcile assembler deployment: %w", err)
		}

		// 5. Create/Update Ingress for Assembler (if configured)
		if config.Ingress != nil {
			if err := r.reconcileIngress(ctx, ordererGroup, config); err != nil {
				return fmt.Errorf("failed to reconcile assembler ingress: %w", err)
			}
		}

	default:
		return fmt.Errorf("unknown bootstrap mode: %s", ordererGroup.Spec.BootstrapMode)
	}

	log.Info("Assembler component reconciled successfully")
	return nil
}

// Cleanup cleans up the Assembler component resources
func (r *AssemblerController) Cleanup(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Cleaning up Assembler component",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace)

	// Always cleanup certificates regardless of mode
	if err := r.cleanupCertificates(ctx, ordererGroup, "assembler"); err != nil {
		log.Error(err, "Failed to cleanup assembler certificates")
	}

	// Only cleanup other resources in deploy mode
	if ordererGroup.Spec.BootstrapMode == "deploy" {
		// 1. Delete Deployment
		if err := r.cleanupDeployment(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup assembler deployment")
		}

		// 2. Delete Service
		if err := r.cleanupService(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup assembler service")
		}

		// 3. Delete Ingress
		if err := r.cleanupIngress(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup assembler ingress")
		}

		// 4. Delete ConfigMap
		if err := r.cleanupConfigMap(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup assembler configmap")
		}
	}

	log.Info("Assembler component cleanup completed")
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap for Assembler configuration
func (r *AssemblerController) reconcileConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement ConfigMap reconciliation
	// This would create/update a ConfigMap containing the Assembler configuration
	// based on the provided config and ordererGroup spec
	return nil
}

// reconcileSecret creates or updates the Secret for Assembler certificates
func (r *AssemblerController) reconcileSecret(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Secret reconciliation
	// This would create/update a Secret containing the Assembler certificates
	// based on the enrollment configuration
	return nil
}

// reconcileService creates or updates the Service for Assembler
func (r *AssemblerController) reconcileService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Service reconciliation
	// This would create/update a Service to expose the Assembler pods
	return nil
}

// reconcileDeployment creates or updates the Deployment for Assembler
func (r *AssemblerController) reconcileDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Deployment reconciliation
	// This would create/update a Deployment for the Assembler component
	// using the provided configuration (replicas, resources, volumes, etc.)
	return nil
}

// reconcileIngress creates or updates the Ingress for Assembler
func (r *AssemblerController) reconcileIngress(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// cleanupDeployment deletes the Assembler Deployment
func (r *AssemblerController) cleanupDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Deployment cleanup
	return nil
}

// cleanupService deletes the Assembler Service
func (r *AssemblerController) cleanupService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Service cleanup
	return nil
}

// cleanupIngress deletes the Assembler Ingress
func (r *AssemblerController) cleanupIngress(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Ingress cleanup
	return nil
}

// cleanupConfigMap deletes the Assembler ConfigMap
func (r *AssemblerController) cleanupConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement ConfigMap cleanup
	return nil
}

// cleanupSecret deletes the Assembler Secret
func (r *AssemblerController) cleanupSecret(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement Secret cleanup
	return nil
}
