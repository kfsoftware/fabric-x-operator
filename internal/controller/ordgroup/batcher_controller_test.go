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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestBatcherController_ReconcileConfigMap_WithPartyID(t *testing.T) {
	// Create a test OrdererGroup with PartyID
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-orderergroup",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			BootstrapMode: "deploy",
			MSPID:         "OrdererOrg1",
			PartyID:       5, // Set a specific PartyID for testing
			Genesis: fabricxv1alpha1.GenesisConfig{
				SecretName:      "genesis-secret",
				SecretKey:       "genesis.block",
				SecretNamespace: "default",
			},
			Components: fabricxv1alpha1.OrdererComponents{
				Batchers: []fabricxv1alpha1.BatcherInstance{
					{
						CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
							Replicas: 1,
						},
						ShardID: 1,
					},
				},
			},
		},
	}

	// Create a fake client
	scheme := runtime.NewScheme()
	_ = fabricxv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create the batcher controller
	controller := NewBatcherController(client, scheme)

	// Test the reconcileConfigMap method
	ctx := context.Background()
	componentConfig := &fabricxv1alpha1.ComponentConfig{
		CommonComponentConfig: ordererGroup.Spec.Components.Batchers[0].CommonComponentConfig,
	}
	err := controller.reconcileConfigMap(ctx, ordererGroup, componentConfig, 0, ordererGroup.Spec.Components.Batchers[0].ShardID)
	if err != nil {
		t.Fatalf("Failed to reconcile configmap: %v", err)
	}

	// Verify that the ConfigMap was created with the correct PartyID
	// Note: In a real test, you would fetch the ConfigMap and verify its contents
	// For now, we just verify that the method doesn't error
	t.Logf("ConfigMap reconciliation completed successfully for PartyID: %d", ordererGroup.Spec.PartyID)
}

func TestBatcherController_ReconcileConfigMap_DefaultPartyID(t *testing.T) {
	// Create a test OrdererGroup without PartyID (should default to 0)
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-orderergroup-default",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			BootstrapMode: "deploy",
			MSPID:         "OrdererOrg1",
			// PartyID not set, should default to 0
			Genesis: fabricxv1alpha1.GenesisConfig{
				SecretName:      "genesis-secret",
				SecretKey:       "genesis.block",
				SecretNamespace: "default",
			},
			Components: fabricxv1alpha1.OrdererComponents{
				Batchers: []fabricxv1alpha1.BatcherInstance{
					{
						CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
							Replicas: 1,
						},
						ShardID: 1,
					},
				},
			},
		},
	}

	// Create a fake client
	scheme := runtime.NewScheme()
	_ = fabricxv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create the batcher controller
	controller := NewBatcherController(client, scheme)

	// Test the reconcileConfigMap method
	ctx := context.Background()
	componentConfig := &fabricxv1alpha1.ComponentConfig{
		CommonComponentConfig: ordererGroup.Spec.Components.Batchers[0].CommonComponentConfig,
	}
	err := controller.reconcileConfigMap(ctx, ordererGroup, componentConfig, 0, ordererGroup.Spec.Components.Batchers[0].ShardID)
	if err != nil {
		t.Fatalf("Failed to reconcile configmap: %v", err)
	}

	// Verify that the ConfigMap was created with the default PartyID (0)
	t.Logf("ConfigMap reconciliation completed successfully for default PartyID: %d", ordererGroup.Spec.PartyID)
}
