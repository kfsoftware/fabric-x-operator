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

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	ordgroup "github.com/kfsoftware/fabric-x-operator/internal/controller/ordgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestOrdererGroupReconciler_Reconcile(t *testing.T) {
	// Setup
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	tests := []struct {
		name           string
		ordererGroup   *fabricxv1alpha1.OrdererGroup
		expectedStatus fabricxv1alpha1.DeploymentStatus
		expectError    bool
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
					Common: &fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
						Storage: &fabricxv1alpha1.StorageConfig{
							Size:       "1Gi",
							AccessMode: "ReadWriteOnce",
						},
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					},
					Components: fabricxv1alpha1.OrdererComponents{
						Consenter: &fabricxv1alpha1.ComponentConfig{
							CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
								Replicas: 1,
							},
						},
						Batcher: &fabricxv1alpha1.ComponentConfig{
							CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
								Replicas: 1,
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
					},
					Enrollment: &fabricxv1alpha1.EnrollmentConfig{
						Sign: &fabricxv1alpha1.CertificateConfig{
							CAHost: "ca.example.com",
							CAPort: 7054,
							CATLS: &fabricxv1alpha1.CATLSConfig{
								CACert: "",
							},
							EnrollID:     "admin",
							EnrollSecret: "adminpw",
						},
					},
				},
			},
			expectedStatus: fabricxv1alpha1.RunningStatus,
			expectError:    false,
		},
		{
			name:           "OrdererGroup not found",
			ordererGroup:   nil,
			expectedStatus: fabricxv1alpha1.FailedStatus,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			var objs []client.Object
			if tt.ordererGroup != nil {
				objs = append(objs, tt.ordererGroup)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

			// Create reconciler
			r := &OrdererGroupReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			// Initialize component controllers
			r.AssemblerController = ordgroup.NewAssemblerController(fakeClient, s)
			r.BatcherController = ordgroup.NewBatcherController(fakeClient, s)
			r.RouterController = ordgroup.NewRouterController(fakeClient, s)
			r.ConsenterController = ordgroup.NewConsenterController(fakeClient, s)
			r.CertService = certs.NewOrdererGroupCertService(fakeClient)

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-orderergroup",
					Namespace: "default",
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

				// Check if OrdererGroup was created/updated
				var ordererGroup fabricxv1alpha1.OrdererGroup
				err = fakeClient.Get(context.Background(), req.NamespacedName, &ordererGroup)
				if err != nil {
					t.Errorf("Failed to get OrdererGroup: %v", err)
				}

				// Check status
				if ordererGroup.Status.Status != tt.expectedStatus {
					t.Errorf("Expected status %s, got %s", tt.expectedStatus, ordererGroup.Status.Status)
				}

				// Check if finalizer was added
				if !containsString(ordererGroup.Finalizers, FinalizerName) {
					t.Errorf("Expected finalizer to be added")
				}
			}

			// Check reconciliation result
			if !result.Requeue && result.RequeueAfter != 0 {
				t.Errorf("Unexpected reconciliation result: %v", result)
			}
		})
	}
}

func TestOrdererGroupReconciler_getMergedComponentConfig(t *testing.T) {
	r := &OrdererGroupReconciler{}

	// Create test OrdererGroup
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			Common: &fabricxv1alpha1.CommonComponentConfig{
				Replicas: 2,
				Storage: &fabricxv1alpha1.StorageConfig{
					Size:       "2Gi",
					AccessMode: "ReadWriteMany",
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			Components: fabricxv1alpha1.OrdererComponents{
				Consenter: &fabricxv1alpha1.ComponentConfig{
					CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
						Replicas: 3,
					},
				},
			},
			Enrollment: &fabricxv1alpha1.EnrollmentConfig{
				Sign: &fabricxv1alpha1.CertificateConfig{
					CAHost: "ca.example.com",
					CAPort: 7054,
					CATLS: &fabricxv1alpha1.CATLSConfig{
						CACert: "",
					},
					EnrollID:     "admin",
					EnrollSecret: "adminpw",
				},
			},
		},
	}

	tests := []struct {
		name            string
		componentName   string
		componentConfig *fabricxv1alpha1.ComponentConfig
		expected        *fabricxv1alpha1.ComponentConfig
	}{
		{
			name:          "consenter with component-specific config",
			componentName: "consenter",
			componentConfig: &fabricxv1alpha1.ComponentConfig{
				CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
					Replicas: 3,
				},
			},
			expected: &fabricxv1alpha1.ComponentConfig{
				CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
					Replicas: 3, // Component-specific overrides common
				},
				Certificates: &fabricxv1alpha1.CertificateConfig{
					CAHost: "ca.example.com",
					CAPort: 7054,
					CATLS: &fabricxv1alpha1.CATLSConfig{
						CACert: "",
					},
					EnrollID:     "admin",
					EnrollSecret: "adminpw",
				},
			},
		},
		{
			name:            "batcher with no component-specific config",
			componentName:   "batcher",
			componentConfig: nil,
			expected: &fabricxv1alpha1.ComponentConfig{
				CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
					Replicas: 2, // Uses common config
				},
				Certificates: &fabricxv1alpha1.CertificateConfig{
					CAHost: "ca.example.com",
					CAPort: 7054,
					CATLS: &fabricxv1alpha1.CATLSConfig{
						CACert: "",
					},
					EnrollID:     "admin",
					EnrollSecret: "adminpw",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.getMergedComponentConfig(ordererGroup, tt.componentName, tt.componentConfig)

			// Check replicas
			if result.Replicas != tt.expected.Replicas {
				t.Errorf("Expected replicas %d, got %d", tt.expected.Replicas, result.Replicas)
			}

			// Check certificates if enrollment is configured
			if ordererGroup.Spec.Enrollment != nil {
				if result.Certificates == nil {
					t.Error("Expected certificates to be configured")
				} else {
					if result.Certificates.CAHost != tt.expected.Certificates.CAHost {
						t.Errorf("Expected CAHost %s, got %s", tt.expected.Certificates.CAHost, result.Certificates.CAHost)
					}
					if result.Certificates.CAPort != tt.expected.Certificates.CAPort {
						t.Errorf("Expected CAPort %d, got %d", tt.expected.Certificates.CAPort, result.Certificates.CAPort)
					}
					if result.Certificates.EnrollID != tt.expected.Certificates.EnrollID {
						t.Errorf("Expected EnrollID %s, got %s", tt.expected.Certificates.EnrollID, result.Certificates.EnrollID)
					}
				}
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
			Finalizers:        []string{FinalizerName},
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			BootstrapMode: "deploy",
			Common: &fabricxv1alpha1.CommonComponentConfig{
				Replicas: 1,
			},
			Components: fabricxv1alpha1.OrdererComponents{
				Consenter: &fabricxv1alpha1.ComponentConfig{
					CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
					},
				},
				Batcher: &fabricxv1alpha1.ComponentConfig{
					CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
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

	// Initialize component controllers
	r.AssemblerController = ordgroup.NewAssemblerController(fakeClient, s)
	r.BatcherController = ordgroup.NewBatcherController(fakeClient, s)
	r.RouterController = ordgroup.NewRouterController(fakeClient, s)
	r.ConsenterController = ordgroup.NewConsenterController(fakeClient, s)
	r.CertService = certs.NewOrdererGroupCertService(fakeClient)

	// Test deletion handling
	ctx := context.Background()
	result, err := r.handleDeletion(ctx, ordererGroup)

	if err != nil {
		t.Errorf("Unexpected error during deletion: %v", err)
	}

	// Check that finalizer was removed
	var updatedOrdererGroup fabricxv1alpha1.OrdererGroup
	err = fakeClient.Get(ctx, types.NamespacedName{Name: ordererGroup.Name, Namespace: ordererGroup.Namespace}, &updatedOrdererGroup)
	if err != nil {
		t.Errorf("Failed to get updated OrdererGroup: %v", err)
	}

	if containsString(updatedOrdererGroup.Finalizers, FinalizerName) {
		t.Error("Expected finalizer to be removed")
	}

	// Check reconciliation result
	if result.Requeue {
		t.Error("Expected no requeue after successful deletion")
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
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ordererGroup).Build()

	// Create reconciler
	r := &OrdererGroupReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test finalizer addition
	ctx := context.Background()
	err := r.ensureFinalizer(ctx, ordererGroup)
	if err != nil {
		t.Errorf("Unexpected error ensuring finalizer: %v", err)
	}

	// Check that finalizer was added
	if !containsString(ordererGroup.Finalizers, FinalizerName) {
		t.Error("Expected finalizer to be added")
	}

	// Test finalizer already exists
	err = r.ensureFinalizer(ctx, ordererGroup)
	if err != nil {
		t.Errorf("Unexpected error when finalizer already exists: %v", err)
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
			Finalizers: []string{FinalizerName, "another-finalizer"},
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
			BootstrapMode: "deploy",
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ordererGroup).Build()

	// Create reconciler
	r := &OrdererGroupReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test finalizer removal
	ctx := context.Background()
	err := r.removeFinalizer(ctx, ordererGroup)
	if err != nil {
		t.Errorf("Unexpected error removing finalizer: %v", err)
	}

	// Check that finalizer was removed
	if containsString(ordererGroup.Finalizers, FinalizerName) {
		t.Error("Expected finalizer to be removed")
	}

	// Check that other finalizers remain
	if !containsString(ordererGroup.Finalizers, "another-finalizer") {
		t.Error("Expected other finalizers to remain")
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
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ordererGroup).Build()

	// Create reconciler
	r := &OrdererGroupReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test status update
	ctx := context.Background()
	status := fabricxv1alpha1.RunningStatus
	message := "OrdererGroup is running successfully"
	r.updateOrdererGroupStatus(ctx, ordererGroup, status, message)

	// Check status was updated
	if ordererGroup.Status.Status != status {
		t.Errorf("Expected status %s, got %s", status, ordererGroup.Status.Status)
	}

	if ordererGroup.Status.Message != message {
		t.Errorf("Expected message %s, got %s", message, ordererGroup.Status.Message)
	}

	// Check conditions
	if len(ordererGroup.Status.Conditions) == 0 {
		t.Error("Expected conditions to be set")
	}

	condition := ordererGroup.Status.Conditions[0]
	if condition.Type != "Ready" {
		t.Errorf("Expected condition type Ready, got %s", condition.Type)
	}

	if condition.Status != metav1.ConditionTrue {
		t.Errorf("Expected condition status True, got %s", condition.Status)
	}
}
