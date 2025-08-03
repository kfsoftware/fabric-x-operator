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

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
)

// ComponentController defines the interface for component-specific controllers
type ComponentController interface {
	// Reconcile reconciles the component
	Reconcile(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error

	// Cleanup cleans up the component resources
	Cleanup(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error
}

// BaseComponentController provides common functionality for component controllers
type BaseComponentController struct {
	Client      client.Client
	Scheme      *runtime.Scheme
	CertService certs.OrdererGroupCertServiceInterface
}

// NewBaseComponentController creates a new base component controller
func NewBaseComponentController(client client.Client, scheme *runtime.Scheme) BaseComponentController {
	return BaseComponentController{
		Client:      client,
		Scheme:      scheme,
		CertService: certs.NewOrdererGroupCertService(client),
	}
}

// CertificateData represents certificate and key data
type CertificateData struct {
	Cert []byte
	Key  []byte
}

// reconcileCertificates handles certificate creation for a component using Fabric CA
func (r *BaseComponentController) reconcileCertificates(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
	config *fabricxv1alpha1.ComponentConfig,
) error {
	// Use the certificate service to provision certificates from Fabric CA
	if err := r.CertService.ProvisionComponentCertificates(ctx, ordererGroup, componentName, config); err != nil {
		return fmt.Errorf("failed to provision certificates for component %s: %w", componentName, err)
	}

	return nil
}

// cleanupCertificates removes certificate secrets for a component
func (r *BaseComponentController) cleanupCertificates(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
) error {
	// Use the certificate service to clean up certificates
	if err := r.CertService.CleanupComponentCertificates(ctx, ordererGroup, componentName); err != nil {
		return fmt.Errorf("failed to cleanup certificates for component %s: %w", componentName, err)
	}

	return nil
}
