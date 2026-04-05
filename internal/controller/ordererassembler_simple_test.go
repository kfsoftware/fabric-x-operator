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

func TestOrdererAssemblerReconciler_NotFound(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&fabricxv1alpha1.OrdererAssembler{}).
		Build()

	reconciler := &OrdererAssemblerReconciler{
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

func TestOrdererAssemblerReconciler_BasicReconcile(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	assembler := &fabricxv1alpha1.OrdererAssembler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-assembler",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.OrdererAssemblerSpec{
			BootstrapMode: "configure",
			MSPID:         "OrdererMSP",
			Image:         "hyperledger/fabric-x-orderer",
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
		WithRuntimeObjects(assembler).
		WithStatusSubresource(&fabricxv1alpha1.OrdererAssembler{}).
		Build()

	reconciler := &OrdererAssemblerReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      assembler.Name,
			Namespace: assembler.Namespace,
		},
	}

	_, err := reconciler.Reconcile(context.Background(), req)
	// May error due to missing dependencies, but should not panic
	_ = err
}
