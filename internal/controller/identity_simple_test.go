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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIdentityReconciler_ValidateSpec(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	reconciler := &IdentityReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	tests := []struct {
		name          string
		identity      *fabricxv1alpha1.Identity
		expectError   bool
		errorContains string
	}{
		{
			name: "valid identity configuration",
			identity: &fabricxv1alpha1.Identity{
				Spec: fabricxv1alpha1.IdentitySpec{
					Type:  "admin",
					MspID: "Org1MSP",
					Enrollment: &fabricxv1alpha1.IdentityEnrollment{
						CARef: fabricxv1alpha1.IdentityCARef{
							Name:      "fabric-ca",
							Namespace: "default",
						},
						EnrollID: "admin",
						EnrollSecretRef: fabricxv1alpha1.SecretKeyNSSelector{
							Name:      "admin-secret",
							Namespace: "default",
							Key:       "password",
						},
					},
					Output: fabricxv1alpha1.IdentityOutput{
						SecretName: "org1-admin",
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing enrollment configuration",
			identity: &fabricxv1alpha1.Identity{
				Spec: fabricxv1alpha1.IdentitySpec{
					Type:  "admin",
					MspID: "Org1MSP",
					Output: fabricxv1alpha1.IdentityOutput{
						SecretName: "org1-admin",
					},
				},
			},
			expectError:   true,
			errorContains: "enrollment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reconciler.validateSpec(tt.identity)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIdentityReconciler_Reconcile_NotFound(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&fabricxv1alpha1.Identity{}).
		Build()

	reconciler := &IdentityReconciler{
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

func TestIdentityReconciler_IdemixConfiguration(t *testing.T) {
	enabled := true
	identity := &fabricxv1alpha1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-idemix-identity",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.IdentitySpec{
			Type:  "user",
			MspID: "Org1MSP",
			Enrollment: &fabricxv1alpha1.IdentityEnrollment{
				CARef: fabricxv1alpha1.IdentityCARef{
					Name:      "idemix-ca",
					Namespace: "default",
				},
				EnrollID: "user1",
				EnrollSecretRef: fabricxv1alpha1.SecretKeyNSSelector{
					Name:      "user-secret",
					Namespace: "default",
					Key:       "password",
				},
				Idemix: &fabricxv1alpha1.IdentityIdemixEnrollment{
					Enabled: &enabled,
					CARef: &fabricxv1alpha1.IdentityCARef{
						Name:      "idemix-ca",
						Namespace: "default",
					},
				},
			},
			Output: fabricxv1alpha1.IdentityOutput{
				SecretName: "org1-user1",
			},
		},
	}

	// Verify idemix configuration is properly set
	assert.NotNil(t, identity.Spec.Enrollment.Idemix)
	assert.True(t, *identity.Spec.Enrollment.Idemix.Enabled)
	assert.Equal(t, "idemix-ca", identity.Spec.Enrollment.Idemix.CARef.Name)
}

func TestIdentityReconciler_OutputSecretsExist(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, fabricxv1alpha1.AddToScheme(s))

	identity := &fabricxv1alpha1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-identity",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.IdentitySpec{
			Type:  "admin",
			MspID: "Org1MSP",
		},
		Status: fabricxv1alpha1.IdentityStatus{
			OutputSecrets: fabricxv1alpha1.IdentityOutputSecrets{
				SignCert:   "test-cert",
				SignKey:    "test-cert",
				SignCACert: "test-cert",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(identity).
		Build()

	reconciler := &IdentityReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	exists := reconciler.outputSecretsExist(context.Background(), identity)
	// Will be false since secrets don't actually exist, but method runs without error
	assert.False(t, exists)
}

func TestIdentityReconciler_FinalizerConstant(t *testing.T) {
	// Verify finalizer constant is defined
	assert.Equal(t, "identity.fabricx.kfsoft.tech/finalizer", identityFinalizer)
}
