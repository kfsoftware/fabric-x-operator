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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// TLSRouteConfig contains the configuration for creating a TLSRoute
type TLSRouteConfig struct {
	// Name of the TLSRoute resource
	Name string
	// Namespace of the TLSRoute resource
	Namespace string
	// Hostnames for SNI matching
	Hostnames []string
	// Service name to route to
	ServiceName string
	// Service port to route to
	ServicePort int32
	// Labels to apply to the TLSRoute
	Labels map[string]string
	// Owner object for controller reference
	Owner client.Object
	// Scheme for setting controller reference
	Scheme *runtime.Scheme
	// Gateway name to attach to (defaults to "fabric-x-gateway")
	GatewayName string
}

// ReconcileTLSRoute creates or updates a Gateway API TLSRoute resource
func ReconcileTLSRoute(ctx context.Context, c client.Client, config TLSRouteConfig) error {
	log := logf.FromContext(ctx)

	// Default gateway name
	if config.GatewayName == "" {
		config.GatewayName = "fabric-x-gateway"
	}

	// Gateway reference
	gatewayNamespace := gatewayv1.Namespace(config.Namespace)
	parentRef := gatewayv1alpha2.ParentReference{
		Name:      gatewayv1.ObjectName(config.GatewayName),
		Namespace: &gatewayNamespace,
	}

	// Convert hosts to Hostname type
	var hostnames []gatewayv1alpha2.Hostname
	for _, host := range config.Hostnames {
		hostnames = append(hostnames, gatewayv1alpha2.Hostname(host))
	}

	// Service reference
	servicePort := gatewayv1alpha2.PortNumber(config.ServicePort)
	backendRef := gatewayv1alpha2.BackendRef{
		BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
			Name: gatewayv1.ObjectName(config.ServiceName),
			Port: &servicePort,
		},
	}

	// Create TLSRoute resource template
	tlsRouteTemplate := &gatewayv1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
			Labels:    config.Labels,
		},
		Spec: gatewayv1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{
				ParentRefs: []gatewayv1alpha2.ParentReference{parentRef},
			},
			Hostnames: hostnames,
			Rules: []gatewayv1alpha2.TLSRouteRule{
				{
					BackendRefs: []gatewayv1alpha2.BackendRef{backendRef},
				},
			},
		},
	}

	// Set controller reference if owner is provided
	if config.Owner != nil && config.Scheme != nil {
		if err := controllerutil.SetControllerReference(config.Owner, tlsRouteTemplate, config.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for TLSRoute: %w", err)
		}
	}

	// Check if TLSRoute already exists
	existingTLSRoute := &gatewayv1alpha2.TLSRoute{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      config.Name,
		Namespace: config.Namespace,
	}, existingTLSRoute)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new TLSRoute
			if err := c.Create(ctx, tlsRouteTemplate); err != nil {
				return fmt.Errorf("failed to create TLSRoute: %w", err)
			}
			log.Info("Created Gateway API TLSRoute", "tlsroute", config.Name)
		} else {
			return fmt.Errorf("failed to get existing TLSRoute: %w", err)
		}
	} else {
		// Update existing TLSRoute - always update to ensure it's current
		existingTLSRoute.Spec = tlsRouteTemplate.Spec
		existingTLSRoute.Labels = tlsRouteTemplate.Labels
		if err := c.Update(ctx, existingTLSRoute); err != nil {
			return fmt.Errorf("failed to update TLSRoute: %w", err)
		}
		log.Info("Updated Gateway API TLSRoute", "tlsroute", config.Name)
	}

	log.Info("Gateway API TLSRoute reconciled successfully", "tlsroute", config.Name)
	return nil
}

// DeleteTLSRoute deletes a Gateway API TLSRoute resource
func DeleteTLSRoute(ctx context.Context, c client.Client, name, namespace string) error {
	log := logf.FromContext(ctx)

	// Delete TLSRoute
	tlsRoute := &gatewayv1alpha2.TLSRoute{}
	tlsRoute.SetName(name)
	tlsRoute.SetNamespace(namespace)

	if err := c.Delete(ctx, tlsRoute); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete TLSRoute", "name", name)
		return err
	}

	log.Info("Deleted TLSRoute", "name", name)
	return nil
}
