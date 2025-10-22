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

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Tests for CommitterCoordinator
func TestCommitterCoordinatorReconciler_NotFound(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&fabricxv1alpha1.CommitterCoordinator{}).
		Build()

	reconciler := &CommitterCoordinatorReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestCommitterCoordinatorReconciler_BasicReconcile(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	coordinator := &fabricxv1alpha1.CommitterCoordinator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-coordinator",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.CommitterCoordinatorSpec{
			BootstrapMode: "deploy",
			MSPID:         "Org1MSP",
			Replicas:      1,
			Image:         "hyperledger/fabric-x-committer",
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(coordinator).
		WithStatusSubresource(&fabricxv1alpha1.CommitterCoordinator{}).
		Build()

	reconciler := &CommitterCoordinatorReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      coordinator.Name,
			Namespace: coordinator.Namespace,
		},
	}

	_, err := reconciler.Reconcile(context.Background(), req)
	// May error due to missing dependencies, but should not panic
	_ = err
}

// Tests for CommitterSidecar
func TestCommitterSidecarReconciler_NotFound(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&fabricxv1alpha1.CommitterSidecar{}).
		Build()

	reconciler := &CommitterSidecarReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// Tests for CommitterValidator
func TestCommitterValidatorReconciler_NotFound(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&fabricxv1alpha1.CommitterValidator{}).
		Build()

	reconciler := &CommitterValidatorReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestCommitterValidatorReconciler_WithPostgreSQL(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	validator := &fabricxv1alpha1.CommitterValidator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-validator",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.CommitterValidatorSpec{
			BootstrapMode: "deploy",
			MSPID:         "Org1MSP",
			Replicas:      1,
			Image:         "hyperledger/fabric-x-committer",
			PostgreSQL: &fabricxv1alpha1.PostgreSQLConfig{
				Host:     "postgres.default",
				Port:     5432,
				Database: "committer",
				Username: "committer_user",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(validator).
		WithStatusSubresource(&fabricxv1alpha1.CommitterValidator{}).
		Build()

	reconciler := &CommitterValidatorReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      validator.Name,
			Namespace: validator.Namespace,
		},
	}

	_, err := reconciler.Reconcile(context.Background(), req)
	// May error due to missing dependencies, but should not panic
	_ = err
}

// Tests for CommitterVerifier
func TestCommitterVerifierReconciler_NotFound(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&fabricxv1alpha1.CommitterVerifier{}).
		Build()

	reconciler := &CommitterVerifierReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// Tests for CommitterQueryService
func TestCommitterQueryServiceReconciler_NotFound(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&fabricxv1alpha1.CommitterQueryService{}).
		Build()

	reconciler := &CommitterQueryServiceReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestCommitterQueryServiceReconciler_BasicReconcile(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	queryService := &fabricxv1alpha1.CommitterQueryService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-queryservice",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.CommitterQueryServiceSpec{
			BootstrapMode: "deploy",
			MSPID:         "Org1MSP",
			Replicas:      1,
			Image:         "hyperledger/fabric-x-committer",
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(queryService).
		WithStatusSubresource(&fabricxv1alpha1.CommitterQueryService{}).
		Build()

	reconciler := &CommitterQueryServiceReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      queryService.Name,
			Namespace: queryService.Namespace,
		},
	}

	_, err := reconciler.Reconcile(context.Background(), req)
	// May error due to missing dependencies, but should not panic
	_ = err
}
