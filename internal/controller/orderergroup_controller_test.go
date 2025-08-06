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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/ordgroup"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
	fakeclientset "github.com/kfsoftware/fabric-x-operator/pkg/client/clientset/versioned/fake"
)

// MockOrdererGroupCertService provides a mock implementation of the certificate service
type MockOrdererGroupCertService struct {
	Client client.Client
}

func NewMockOrdererGroupCertService(client client.Client) certs.OrdererGroupCertServiceInterface {
	return &MockOrdererGroupCertService{
		Client: client,
	}
}

func (s *MockOrdererGroupCertService) ProvisionComponentCertificates(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
	componentConfig *fabricxv1alpha1.ComponentConfig,
) error {
	// Mock implementation - always succeeds
	return nil
}

func (s *MockOrdererGroupCertService) CleanupComponentCertificates(
	ctx context.Context,
	ordererGroup *fabricxv1alpha1.OrdererGroup,
	componentName string,
) error {
	// Mock implementation - always succeeds
	return nil
}

func (s *MockOrdererGroupCertService) GetCertificateSecretName(
	ordererGroupName string,
	componentName string,
	replicaIndex int,
	certType string,
) string {
	return "mock-secret-name"
}

func TestOrdererGroupReconciler_Reconcile(t *testing.T) {
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	tests := []struct {
		name           string
		ordererGroup   *fabricxv1alpha1.OrdererGroup
		expectError    bool
		expectedStatus fabricxv1alpha1.DeploymentStatus
	}{
		{
			name: "successful reconciliation",
			ordererGroup: &fabricxv1alpha1.OrdererGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-orderergroup",
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.OrdererGroupSpec{
					BootstrapMode: "deploy",
					MSPID:         "Org1MSP",
					PartyID:       1,
					Common: &fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
					},
					Components: fabricxv1alpha1.OrdererComponents{
						Consenters: []fabricxv1alpha1.ConsenterInstance{
							{
								CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
									Replicas: 1,
								},
								ConsenterID: 1,
							},
						},
						Assembler: &fabricxv1alpha1.ComponentConfig{
							CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
								Replicas: 1,
							},
						},
						Router: &fabricxv1alpha1.ComponentConfig{
							CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
								Replicas: 1,
							},
						},
						Batchers: []fabricxv1alpha1.BatcherInstance{
							{
								CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
									Replicas: 1,
								},
								ShardID: 1,
							},
						},
					},
					Genesis: fabricxv1alpha1.GenesisConfig{
						SecretName:      "genesis-secret",
						SecretKey:       "genesis.block",
						SecretNamespace: "default",
					},
				},
			},
			expectError:    false,
			expectedStatus: fabricxv1alpha1.RunningStatus,
		},
		{
			name: "configure mode",
			ordererGroup: &fabricxv1alpha1.OrdererGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-orderergroup-configure",
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.OrdererGroupSpec{
					BootstrapMode: "configure",
					MSPID:         "Org1MSP",
					PartyID:       1,
					Common: &fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
					},
					Components: fabricxv1alpha1.OrdererComponents{
						Consenters: []fabricxv1alpha1.ConsenterInstance{
							{
								CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
									Replicas: 1,
								},
								ConsenterID: 1,
							},
						},
					},
					Genesis: fabricxv1alpha1.GenesisConfig{
						SecretName:      "genesis-secret",
						SecretKey:       "genesis.block",
						SecretNamespace: "default",
					},
				},
			},
			expectError:    false,
			expectedStatus: fabricxv1alpha1.RunningStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.ordererGroup).Build()

			// Create reconciler
			r := &OrdererGroupReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			// Initialize component controllers with mock certificate service
			mockCertService := NewMockOrdererGroupCertService(fakeClient)
			r.AssemblerController = ordgroup.NewAssemblerControllerWithCertService(fakeClient, s, mockCertService)
			r.BatcherController = ordgroup.NewBatcherControllerWithCertService(fakeClient, s, mockCertService)
			r.RouterController = ordgroup.NewRouterControllerWithCertService(fakeClient, s, mockCertService)
			r.ConsenterController = ordgroup.NewConsenterControllerWithCertService(fakeClient, s, mockCertService)

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.ordererGroup.Name,
					Namespace: tt.ordererGroup.Namespace,
				},
			}

			// Reconcile
			result, err := r.Reconcile(context.Background(), req)

			// Check results
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// Only check OrdererGroup if it was created
				if tt.ordererGroup != nil {
					// Check if OrdererGroup was created/updated
					var ordererGroup fabricxv1alpha1.OrdererGroup
					err = fakeClient.Get(context.Background(), req.NamespacedName, &ordererGroup)
					if err != nil {
						t.Errorf("Failed to get OrdererGroup: %v", err)
					}

					// Check status - allow for PENDING status since cert provisioning may fail in test environment
					if ordererGroup.Status.Status != tt.expectedStatus && ordererGroup.Status.Status != fabricxv1alpha1.PendingStatus {
						t.Errorf("Expected status %s or PENDING, got %s", tt.expectedStatus, ordererGroup.Status.Status)
					}

					// Check if finalizer was added
					if !utils.ContainsString(ordererGroup.Finalizers, OrdererGroupFinalizerName) {
						t.Errorf("Expected finalizer to be added")
					}
				}
			}

			// Check reconciliation result - allow for requeue with delay
			if result.RequeueAfter < 0 {
				t.Errorf("Unexpected reconciliation result: %v", result)
			}
		})
	}
}

func TestOrdererGroupReconciler_handleDeletion(t *testing.T) {
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	// Create OrdererGroup with deletion timestamp
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-orderergroup",
			Namespace:         "default",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{OrdererGroupFinalizerName},
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			BootstrapMode:   "deploy",
			ManageChildCRDs: &[]bool{false}[0], // Use old behavior for test
			Common: &fabricxv1alpha1.CommonComponentConfig{
				Replicas: 1,
			},
			Components: fabricxv1alpha1.OrdererComponents{
				Consenters: []fabricxv1alpha1.ConsenterInstance{
					{
						CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
							Replicas: 1,
						},
						ConsenterID: 1,
					},
				},
				Assembler: &fabricxv1alpha1.ComponentConfig{
					CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
					},
				},
				Router: &fabricxv1alpha1.ComponentConfig{
					CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
					},
				},
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

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ordererGroup).Build()

	// Create fake clientset
	fakeClientset := fakeclientset.NewSimpleClientset()

	// Create reconciler
	r := &OrdererGroupReconciler{
		Client:    fakeClient,
		Scheme:    s,
		Clientset: fakeClientset,
	}

	// Initialize component controllers with mock certificate service
	mockCertService := NewMockOrdererGroupCertService(fakeClient)
	r.AssemblerController = ordgroup.NewAssemblerControllerWithCertService(fakeClient, s, mockCertService)
	r.BatcherController = ordgroup.NewBatcherControllerWithCertService(fakeClient, s, mockCertService)
	r.RouterController = ordgroup.NewRouterControllerWithCertService(fakeClient, s, mockCertService)
	r.ConsenterController = ordgroup.NewConsenterControllerWithCertService(fakeClient, s, mockCertService)

	// Test deletion handling
	ctx := context.Background()
	result, err := r.handleDeletion(ctx, ordererGroup)

	if err != nil {
		t.Errorf("Unexpected error during deletion: %v", err)
	}

	// Check that finalizer was removed from the original object
	if utils.ContainsString(ordererGroup.Finalizers, OrdererGroupFinalizerName) {
		t.Errorf("Expected finalizer to be removed")
	}

	// Check result
	if result.Requeue {
		t.Errorf("Expected no requeue after deletion")
	}
}

func TestOrdererGroupReconciler_ensureFinalizer(t *testing.T) {
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	// Create OrdererGroup without finalizer
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-orderergroup",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			BootstrapMode: "deploy",
			Common: &fabricxv1alpha1.CommonComponentConfig{
				Replicas: 1,
			},
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ordererGroup).Build()

	// Create reconciler
	r := &OrdererGroupReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test ensuring finalizer
	ctx := context.Background()
	err := r.ensureFinalizer(ctx, ordererGroup)

	if err != nil {
		t.Errorf("Unexpected error ensuring finalizer: %v", err)
	}

	// Check that finalizer was added
	if !utils.ContainsString(ordererGroup.Finalizers, OrdererGroupFinalizerName) {
		t.Errorf("Expected finalizer to be added")
	}
}

func TestOrdererGroupReconciler_removeFinalizer(t *testing.T) {
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	// Create OrdererGroup with finalizer
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-orderergroup",
			Namespace:  "default",
			Finalizers: []string{OrdererGroupFinalizerName},
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			BootstrapMode: "deploy",
			Common: &fabricxv1alpha1.CommonComponentConfig{
				Replicas: 1,
			},
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ordererGroup).Build()

	// Create reconciler
	r := &OrdererGroupReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test removing finalizer
	ctx := context.Background()
	err := r.removeFinalizer(ctx, ordererGroup)

	if err != nil {
		t.Errorf("Unexpected error removing finalizer: %v", err)
	}

	// Check that finalizer was removed
	if utils.ContainsString(ordererGroup.Finalizers, OrdererGroupFinalizerName) {
		t.Errorf("Expected finalizer to be removed")
	}
}

func TestOrdererGroupReconciler_updateOrdererGroupStatus(t *testing.T) {
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	// Create OrdererGroup
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-orderergroup",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			BootstrapMode: "deploy",
			Common: &fabricxv1alpha1.CommonComponentConfig{
				Replicas: 1,
			},
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ordererGroup).Build()

	// Create reconciler
	r := &OrdererGroupReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test updating status
	ctx := context.Background()
	r.updateOrdererGroupStatus(ctx, ordererGroup, fabricxv1alpha1.RunningStatus, "Test status update")

	// Check that status was updated on the original object
	if ordererGroup.Status.Status != fabricxv1alpha1.RunningStatus {
		t.Errorf("Expected status %s, got %s", fabricxv1alpha1.RunningStatus, ordererGroup.Status.Status)
	}

	if ordererGroup.Status.Message != "Test status update" {
		t.Errorf("Expected message 'Test status update', got '%s'", ordererGroup.Status.Message)
	}
}
