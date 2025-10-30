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

// RouteConfig contains the common configuration for creating Gateway API routes (TLS or TCP)
type RouteConfig struct {
	// Name of the Route resource
	Name string
	// Namespace of the Route resource
	Namespace string
	// Hostnames for SNI matching (TLSRoute only)
	Hostnames []string
	// Service name to route to
	ServiceName string
	// Service port to route to
	ServicePort int32
	// Labels to apply to the Route
	Labels map[string]string
	// Owner object for controller reference
	Owner client.Object
	// Scheme for setting controller reference
	Scheme *runtime.Scheme
	// Gateway reference in format "namespace/name" or just "name"
	// If only name is provided, uses the same namespace as the Route
	// Defaults to "fabric-x-gateway" if empty
	IngressGateway string
}

// TLSRouteConfig is an alias for backward compatibility
type TLSRouteConfig = RouteConfig

// ReconcileTLSRoute creates or updates a Gateway API TLSRoute resource
func ReconcileTLSRoute(ctx context.Context, c client.Client, config TLSRouteConfig) error {
	log := logf.FromContext(ctx)

	// Parse IngressGateway (format: "namespace/name" or just "name")
	gatewayName := "fabric-x-gateway"
	gatewayNamespace := config.Namespace

	if config.IngressGateway != "" {
		// Split by "/" to get namespace and name
		parts := strings.Split(config.IngressGateway, "/")
		if len(parts) == 2 {
			gatewayNamespace = parts[0]
			gatewayName = parts[1]
		} else {
			// No slash found, use the whole string as gateway name
			gatewayName = config.IngressGateway
		}
	}

	// Gateway reference
	gwNamespace := gatewayv1.Namespace(gatewayNamespace)
	gwGroup := gatewayv1.Group("gateway.networking.k8s.io")
	gwKind := gatewayv1.Kind("Gateway")
	parentRef := gatewayv1alpha2.ParentReference{
		Name:      gatewayv1.ObjectName(gatewayName),
		Namespace: &gwNamespace,
		Group:     &gwGroup,
		Kind:      &gwKind,
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

// ReconcileTCPRoute creates or updates a Gateway API TCPRoute resource for non-TLS services
func ReconcileTCPRoute(ctx context.Context, c client.Client, config RouteConfig) error {
	log := logf.FromContext(ctx)

	// Parse IngressGateway (format: "namespace/name" or just "name")
	gatewayName := "fabric-x-gateway"
	gatewayNamespace := config.Namespace

	if config.IngressGateway != "" {
		// Split by "/" to get namespace and name
		parts := strings.Split(config.IngressGateway, "/")
		if len(parts) == 2 {
			gatewayNamespace = parts[0]
			gatewayName = parts[1]
		} else {
			// No slash found, use the whole string as gateway name
			gatewayName = config.IngressGateway
		}
	}

	// Gateway reference
	gwNamespace := gatewayv1.Namespace(gatewayNamespace)
	gwGroup := gatewayv1.Group("gateway.networking.k8s.io")
	gwKind := gatewayv1.Kind("Gateway")
	parentRef := gatewayv1alpha2.ParentReference{
		Name:      gatewayv1.ObjectName(gatewayName),
		Namespace: &gwNamespace,
		Group:     &gwGroup,
		Kind:      &gwKind,
	}

	// Service reference
	servicePort := gatewayv1alpha2.PortNumber(config.ServicePort)
	backendRef := gatewayv1alpha2.BackendRef{
		BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
			Name: gatewayv1.ObjectName(config.ServiceName),
			Port: &servicePort,
		},
	}

	// Create TCPRoute resource template
	tcpRouteTemplate := &gatewayv1alpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
			Labels:    config.Labels,
		},
		Spec: gatewayv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{
				ParentRefs: []gatewayv1alpha2.ParentReference{parentRef},
			},
			Rules: []gatewayv1alpha2.TCPRouteRule{
				{
					BackendRefs: []gatewayv1alpha2.BackendRef{backendRef},
				},
			},
		},
	}

	// Set controller reference if owner is provided
	if config.Owner != nil && config.Scheme != nil {
		if err := controllerutil.SetControllerReference(config.Owner, tcpRouteTemplate, config.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for TCPRoute: %w", err)
		}
	}

	// Check if TCPRoute already exists
	existingTCPRoute := &gatewayv1alpha2.TCPRoute{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      config.Name,
		Namespace: config.Namespace,
	}, existingTCPRoute)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new TCPRoute
			if err := c.Create(ctx, tcpRouteTemplate); err != nil {
				return fmt.Errorf("failed to create TCPRoute: %w", err)
			}
			log.Info("Created Gateway API TCPRoute", "tcproute", config.Name)
		} else {
			return fmt.Errorf("failed to get existing TCPRoute: %w", err)
		}
	} else {
		// Update existing TCPRoute - always update to ensure it's current
		existingTCPRoute.Spec = tcpRouteTemplate.Spec
		existingTCPRoute.Labels = tcpRouteTemplate.Labels
		if err := c.Update(ctx, existingTCPRoute); err != nil {
			return fmt.Errorf("failed to update TCPRoute: %w", err)
		}
		log.Info("Updated Gateway API TCPRoute", "tcproute", config.Name)
	}

	log.Info("Gateway API TCPRoute reconciled successfully", "tcproute", config.Name)
	return nil
}

// ReconcileHTTPRoute creates or updates a Gateway API HTTPRoute resource for HTTP/gRPC services
func ReconcileHTTPRoute(ctx context.Context, c client.Client, config RouteConfig) error {
	log := logf.FromContext(ctx)

	// Parse IngressGateway (format: "namespace/name" or just "name")
	gatewayName := "fabric-x-gateway"
	gatewayNamespace := config.Namespace

	if config.IngressGateway != "" {
		// Split by "/" to get namespace and name
		parts := strings.Split(config.IngressGateway, "/")
		if len(parts) == 2 {
			gatewayNamespace = parts[0]
			gatewayName = parts[1]
		} else {
			// No slash found, use the whole string as gateway name
			gatewayName = config.IngressGateway
		}
	}

	// Gateway reference
	gwNamespace := gatewayv1.Namespace(gatewayNamespace)
	parentRef := gatewayv1.ParentReference{
		Name:      gatewayv1.ObjectName(gatewayName),
		Namespace: &gwNamespace,
	}

	// Convert hosts to Hostname type
	var hostnames []gatewayv1.Hostname
	for _, host := range config.Hostnames {
		hostnames = append(hostnames, gatewayv1.Hostname(host))
	}

	// Service reference
	servicePort := gatewayv1.PortNumber(config.ServicePort)
	backendRef := gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName(config.ServiceName),
				Port: &servicePort,
			},
		},
	}

	// Create HTTPRoute resource template
	httpRouteTemplate := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
			Labels:    config.Labels,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{parentRef},
			},
			Hostnames: hostnames,
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{backendRef},
				},
			},
		},
	}

	// Set controller reference if owner is provided
	if config.Owner != nil && config.Scheme != nil {
		if err := controllerutil.SetControllerReference(config.Owner, httpRouteTemplate, config.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for HTTPRoute: %w", err)
		}
	}

	// Check if HTTPRoute already exists
	existingHTTPRoute := &gatewayv1.HTTPRoute{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      config.Name,
		Namespace: config.Namespace,
	}, existingHTTPRoute)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new HTTPRoute
			if err := c.Create(ctx, httpRouteTemplate); err != nil {
				return fmt.Errorf("failed to create HTTPRoute: %w", err)
			}
			log.Info("Created Gateway API HTTPRoute", "httproute", config.Name)
		} else {
			return fmt.Errorf("failed to get existing HTTPRoute: %w", err)
		}
	} else {
		// Update existing HTTPRoute - always update to ensure it's current
		existingHTTPRoute.Spec = httpRouteTemplate.Spec
		existingHTTPRoute.Labels = httpRouteTemplate.Labels
		if err := c.Update(ctx, existingHTTPRoute); err != nil {
			return fmt.Errorf("failed to update HTTPRoute: %w", err)
		}
		log.Info("Updated Gateway API HTTPRoute", "httproute", config.Name)
	}

	log.Info("Gateway API HTTPRoute reconciled successfully", "httproute", config.Name)
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
