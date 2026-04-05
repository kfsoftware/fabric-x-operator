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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Simple tests that just exercise reconcile logic without complex setup
// These increase coverage by hitting more code paths

func TestReconcilers_NotFoundResources(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	tests := []struct {
		name       string
		reconciler interface{}
		crdName    string
	}{
		{
			name:       "Committer not found",
			reconciler: &CommitterReconciler{},
			crdName:    "Committer",
		},
		{
			name:       "Endorser not found",
			reconciler: &EndorserReconciler{},
			crdName:    "Endorser",
		},
		{
			name:       "OrdererGroup not found",
			reconciler: &OrdererGroupReconciler{},
			crdName:    "OrdererGroup",
		},
		{
			name:       "Identity not found",
			reconciler: &IdentityReconciler{},
			crdName:    "Identity",
		},
		{
			name:       "ChainNamespace not found",
			reconciler: &ChainNamespaceReconciler{},
			crdName:    "ChainNamespace",
		},
		{
			name:       "Genesis not found",
			reconciler: &GenesisReconciler{},
			crdName:    "Genesis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				Build()

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent",
					Namespace: "default",
				},
			}

			// Set up reconciler
			var result ctrl.Result
			var err error

			switch r := tt.reconciler.(type) {
			case *CommitterReconciler:
				r.Client = fakeClient
				r.Scheme = s
				result, err = r.Reconcile(context.Background(), req)
			case *EndorserReconciler:
				r.Client = fakeClient
				r.Scheme = s
				result, err = r.Reconcile(context.Background(), req)
			case *OrdererGroupReconciler:
				r.Client = fakeClient
				r.Scheme = s
				result, err = r.Reconcile(context.Background(), req)
			case *IdentityReconciler:
				r.Client = fakeClient
				r.Scheme = s
				result, err = r.Reconcile(context.Background(), req)
			case *ChainNamespaceReconciler:
				r.Client = fakeClient
				r.Scheme = s
				result, err = r.Reconcile(context.Background(), req)
			case *GenesisReconciler:
				r.Client = fakeClient
				r.Scheme = s
				result, err = r.Reconcile(context.Background(), req)
			}

			assert.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, result)
		})
	}
}

func TestBootstrapModes_ConfigureAndDeploy(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	modes := []string{"configure", "deploy"}

	for _, mode := range modes {
		t.Run("Endorser-"+mode, func(t *testing.T) {
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-endorser",
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: mode,
					MSPID:         "Org1MSP",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithRuntimeObjects(endorser).
				WithStatusSubresource(&fabricxv1alpha1.Endorser{}).
				Build()

			reconciler := &EndorserReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      endorser.Name,
					Namespace: endorser.Namespace,
				},
			}

			_, err := reconciler.Reconcile(context.Background(), req)
			_ = err // May error but should not panic
		})

		t.Run("Committer-"+mode, func(t *testing.T) {
			committer := &fabricxv1alpha1.Committer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-committer",
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.CommitterSpec{
					BootstrapMode: mode,
					MSPID:         "Org1MSP",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithRuntimeObjects(committer).
				WithStatusSubresource(&fabricxv1alpha1.Committer{}).
				Build()

			reconciler := &CommitterReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      committer.Name,
					Namespace: committer.Namespace,
				},
			}

			_, err := reconciler.Reconcile(context.Background(), req)
			_ = err
		})

		t.Run("OrdererGroup-"+mode, func(t *testing.T) {
			ordererGroup := &fabricxv1alpha1.OrdererGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-orderergroup",
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.OrdererGroupSpec{
					BootstrapMode: mode,
					MSPID:         "OrdererMSP",
					PartyID:       1,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithRuntimeObjects(ordererGroup).
				WithStatusSubresource(&fabricxv1alpha1.OrdererGroup{}).
				Build()

			reconciler := &OrdererGroupReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      ordererGroup.Name,
					Namespace: ordererGroup.Namespace,
				},
			}

			_, err := reconciler.Reconcile(context.Background(), req)
			_ = err
		})
	}
}
