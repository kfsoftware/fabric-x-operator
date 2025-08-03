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

package committer

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

// ValidatorController handles reconciliation for the Validator component
type ValidatorController struct {
	BaseComponentController
}

// NewValidatorController creates a new Validator controller
func NewValidatorController(client client.Client, scheme *runtime.Scheme) *ValidatorController {
	return &ValidatorController{
		BaseComponentController: NewBaseComponentController(client, scheme),
	}
}

// Reconcile reconciles the Validator component
func (r *ValidatorController) Reconcile(ctx context.Context, committer *fabricxv1alpha1.Committer, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Validator component",
		"name", committer.Name,
		"namespace", committer.Namespace,
		"replicas", config.Replicas,
		"mode", committer.Spec.BootstrapMode)

	// Handle different modes
	switch committer.Spec.BootstrapMode {
	case "configure":
		// In configure mode, only create certificates
		if err := r.reconcileCertificates(ctx, committer, "validator", config); err != nil {
			return fmt.Errorf("failed to reconcile validator certificates: %w", err)
		}
		log.Info("Validator certificates created in configure mode")
		return nil

	case "deploy":
		// In deploy mode, create all resources
		// 1. Create/Update certificates first
		if err := r.reconcileCertificates(ctx, committer, "validator", config); err != nil {
			return fmt.Errorf("failed to reconcile validator certificates: %w", err)
		}

		// 2. Create/Update ConfigMap for Validator configuration
		if err := r.reconcileConfigMap(ctx, committer, config); err != nil {
			return fmt.Errorf("failed to reconcile validator configmap: %w", err)
		}

		// 3. Create/Update Service for Validator
		if err := r.reconcileService(ctx, committer, config); err != nil {
			return fmt.Errorf("failed to reconcile validator service: %w", err)
		}

		// 4. Create/Update Deployment for Validator
		if err := r.reconcileDeployment(ctx, committer, config); err != nil {
			return fmt.Errorf("failed to reconcile validator deployment: %w", err)
		}

		// 5. Create/Update Ingress for Validator (if configured)
		if config.Ingress != nil {
			if err := r.reconcileIngress(ctx, committer, config); err != nil {
				return fmt.Errorf("failed to reconcile validator ingress: %w", err)
			}
		}

	default:
		return fmt.Errorf("unknown bootstrap mode: %s", committer.Spec.BootstrapMode)
	}

	log.Info("Validator component reconciled successfully")
	return nil
}

// Cleanup cleans up the Validator component resources
func (r *ValidatorController) Cleanup(ctx context.Context, committer *fabricxv1alpha1.Committer, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Cleaning up Validator component",
		"name", committer.Name,
		"namespace", committer.Namespace)

	// Always cleanup certificates regardless of mode
	if err := r.cleanupCertificates(ctx, committer, "validator"); err != nil {
		log.Error(err, "Failed to cleanup validator certificates")
	}

	// Only cleanup other resources in deploy mode
	if committer.Spec.BootstrapMode == "deploy" {
		// 1. Delete Deployment
		if err := r.cleanupDeployment(ctx, committer); err != nil {
			log.Error(err, "Failed to cleanup validator deployment")
		}

		// 2. Delete Service
		if err := r.cleanupService(ctx, committer); err != nil {
			log.Error(err, "Failed to cleanup validator service")
		}

		// 3. Delete Ingress
		if err := r.cleanupIngress(ctx, committer); err != nil {
			log.Error(err, "Failed to cleanup validator ingress")
		}

		// 4. Delete ConfigMap
		if err := r.cleanupConfigMap(ctx, committer); err != nil {
			log.Error(err, "Failed to cleanup validator configmap")
		}
	}

	log.Info("Validator component cleanup completed")
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap for Validator configuration
func (r *ValidatorController) reconcileConfigMap(ctx context.Context, committer *fabricxv1alpha1.Committer, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement ConfigMap reconciliation
	// This would create/update a ConfigMap containing the Validator configuration
	// based on the provided config and committer spec
	return nil
}

// reconcileService creates or updates the Service for Validator
func (r *ValidatorController) reconcileService(ctx context.Context, committer *fabricxv1alpha1.Committer, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Service reconciliation
	// This would create/update a Service to expose the Validator pods
	return nil
}

// reconcileDeployment creates or updates the Deployment for Validator
func (r *ValidatorController) reconcileDeployment(ctx context.Context, committer *fabricxv1alpha1.Committer, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Deployment reconciliation
	// This would create/update a Deployment for the Validator component
	// using the provided configuration (replicas, resources, volumes, etc.)
	return nil
}

// reconcileIngress creates or updates the Ingress for Validator
func (r *ValidatorController) reconcileIngress(ctx context.Context, committer *fabricxv1alpha1.Committer, config *fabricxv1alpha1.ComponentConfig) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// cleanupDeployment deletes the Validator Deployment
func (r *ValidatorController) cleanupDeployment(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	// TODO: Implement Deployment cleanup
	return nil
}

// cleanupService deletes the Validator Service
func (r *ValidatorController) cleanupService(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	// TODO: Implement Service cleanup
	return nil
}

// cleanupIngress deletes the Validator Ingress
func (r *ValidatorController) cleanupIngress(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	// TODO: Implement Ingress cleanup
	return nil
}

// cleanupConfigMap deletes the Validator ConfigMap
func (r *ValidatorController) cleanupConfigMap(ctx context.Context, committer *fabricxv1alpha1.Committer) error {
	// TODO: Implement ConfigMap cleanup
	return nil
}
