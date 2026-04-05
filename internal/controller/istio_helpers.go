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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// IstioConfig contains the configuration for creating Istio resources
type IstioResourceConfig struct {
	// Name of the Istio resource (VirtualService or DestinationRule)
	Name string
	// Namespace of the Istio resource
	Namespace string
	// Hosts for the VirtualService
	Hosts []string
	// Service name to route to
	ServiceName string
	// Service port to route to
	ServicePort int32
	// Gateway reference (e.g., "istio-system/istio-ingressgateway")
	Gateway string
	// Enable HTTP/2 (h2c) for backend connections
	EnableHTTP2 bool
	// Labels to apply to the resource
	Labels map[string]string
	// Owner object for controller reference
	Owner client.Object
	// Scheme for setting controller reference
	Scheme *runtime.Scheme
}

var (
	istioNetworkingGVK = schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
	}
)

// ReconcileIstioVirtualService creates or updates an Istio VirtualService
func ReconcileIstioVirtualService(ctx context.Context, c client.Client, config IstioResourceConfig) error {
	log := logf.FromContext(ctx)

	// Create VirtualService spec
	// For gRPC with TLS: use TLS routing with SNI matching for passthrough
	vsSpec := map[string]interface{}{
		"hosts":    config.Hosts,
		"gateways": []string{config.Gateway},
		"tls": []map[string]interface{}{
			{
				"match": []map[string]interface{}{
					{
						"sniHosts": config.Hosts,
					},
				},
				"route": []map[string]interface{}{
					{
						"destination": map[string]interface{}{
							"host": fmt.Sprintf("%s.%s.svc.cluster.local", config.ServiceName, config.Namespace),
							"port": map[string]interface{}{
								"number": config.ServicePort,
							},
						},
					},
				},
			},
		},
	}

	// Create unstructured VirtualService
	vs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      config.Name,
				"namespace": config.Namespace,
				"labels":    config.Labels,
			},
			"spec": vsSpec,
		},
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(config.Owner, vs, config.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for VirtualService: %w", err)
	}

	// Try to get existing VirtualService
	existingVS := &unstructured.Unstructured{}
	existingVS.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
		Kind:    "VirtualService",
	})

	err := c.Get(ctx, types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, existingVS)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new VirtualService
			log.Info("Creating VirtualService", "name", config.Name, "namespace", config.Namespace)
			if err := c.Create(ctx, vs); err != nil {
				return fmt.Errorf("failed to create VirtualService: %w", err)
			}
			log.Info("VirtualService created successfully", "name", config.Name)
			return nil
		}
		return fmt.Errorf("failed to get VirtualService: %w", err)
	}

	// Update existing VirtualService
	existingVS.Object["spec"] = vsSpec
	if err := controllerutil.SetControllerReference(config.Owner, existingVS, config.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for VirtualService: %w", err)
	}

	log.Info("Updating VirtualService", "name", config.Name, "namespace", config.Namespace)
	if err := c.Update(ctx, existingVS); err != nil {
		return fmt.Errorf("failed to update VirtualService: %w", err)
	}

	log.Info("VirtualService updated successfully", "name", config.Name)
	return nil
}

// ReconcileIstioDestinationRule creates or updates an Istio DestinationRule
func ReconcileIstioDestinationRule(ctx context.Context, c client.Client, config IstioResourceConfig) error {
	log := logf.FromContext(ctx)

	// Create DestinationRule spec with HTTP/2 support
	drSpec := map[string]interface{}{
		"host": fmt.Sprintf("%s.%s.svc.cluster.local", config.ServiceName, config.Namespace),
	}

	// Add trafficPolicy with HTTP/2 if enabled
	if config.EnableHTTP2 {
		drSpec["trafficPolicy"] = map[string]interface{}{
			"connectionPool": map[string]interface{}{
				"http": map[string]interface{}{
					"h2UpgradePolicy":   "UPGRADE",
					"useClientProtocol": false,
				},
			},
		}
	}

	// Create unstructured DestinationRule
	dr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "DestinationRule",
			"metadata": map[string]interface{}{
				"name":      config.Name,
				"namespace": config.Namespace,
				"labels":    config.Labels,
			},
			"spec": drSpec,
		},
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(config.Owner, dr, config.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for DestinationRule: %w", err)
	}

	// Try to get existing DestinationRule
	existingDR := &unstructured.Unstructured{}
	existingDR.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
		Kind:    "DestinationRule",
	})

	err := c.Get(ctx, types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, existingDR)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new DestinationRule
			log.Info("Creating DestinationRule", "name", config.Name, "namespace", config.Namespace, "enableHTTP2", config.EnableHTTP2)
			if err := c.Create(ctx, dr); err != nil {
				return fmt.Errorf("failed to create DestinationRule: %w", err)
			}
			log.Info("DestinationRule created successfully", "name", config.Name)
			return nil
		}
		return fmt.Errorf("failed to get DestinationRule: %w", err)
	}

	// Update existing DestinationRule
	existingDR.Object["spec"] = drSpec
	if err := controllerutil.SetControllerReference(config.Owner, existingDR, config.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for DestinationRule: %w", err)
	}

	log.Info("Updating DestinationRule", "name", config.Name, "namespace", config.Namespace, "enableHTTP2", config.EnableHTTP2)
	if err := c.Update(ctx, existingDR); err != nil {
		return fmt.Errorf("failed to update DestinationRule: %w", err)
	}

	log.Info("DestinationRule updated successfully", "name", config.Name)
	return nil
}
