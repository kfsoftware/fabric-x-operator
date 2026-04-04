package ca

import (
	"context"
	"testing"
	"time"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCAReconciler_Reconcile(t *testing.T) {
	// Setup
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	tests := []struct {
		name           string
		ca             *fabricxv1alpha1.CA
		expectedStatus fabricxv1alpha1.DeploymentStatus
		expectError    bool
	}{
		{
			name: "successful reconciliation",
			ca: &fabricxv1alpha1.CA{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ca",
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.CASpec{
					Image:   "hyperledger/fabric-ca:1.5.15",
					Version: "1.5.15",
					Replicas: func() *int32 {
						replicas := int32(1)
						return &replicas
					}(),
					Database: fabricxv1alpha1.FabricCADatabase{
						Type:       "sqlite3",
						Datasource: "/var/hyperledger/fabric-ca/fabric-ca-server.db",
					},
					CA: fabricxv1alpha1.FabricCAItemConf{
						Name: "ca",
						Registry: fabricxv1alpha1.FabricCAItemRegistry{
							MaxEnrollments: -1,
							Identities: []fabricxv1alpha1.FabricCAIdentity{
								{
									Name:        "admin",
									Pass:        "adminpw",
									Type:        "client",
									Affiliation: "",
									Attrs: fabricxv1alpha1.FabricCAIdentityAttrs{
										RegistrarRoles: "*",
										DelegateRoles:  "*",
										Attributes:     "*",
										Revoker:        true,
										IntermediateCA: true,
										GenCRL:         true,
										AffiliationMgr: true,
									},
								},
							},
						},

						CSR: fabricxv1alpha1.FabricCACSR{
							CN: "fabric-ca-server",
							Hosts: []string{
								"localhost",
								"ca.example.com",
							},
							Names: []fabricxv1alpha1.FabricCANames{
								{
									C:  "US",
									ST: "North Carolina",
									L:  "Raleigh",
									O:  "Hyperledger",
									OU: "Fabric",
								},
							},
							CA: fabricxv1alpha1.FabricCACSRCA{
								Expiry:     "131400h",
								PathLength: 0,
							},
						},
						BCCSP: fabricxv1alpha1.FabricCAItemBCCSP{
							Default: "SW",
							SW: fabricxv1alpha1.FabricCAItemBCCSPSW{
								Hash:     "SHA2",
								Security: 256,
							},
						},
						Intermediate: fabricxv1alpha1.FabricCAItemIntermediate{
							ParentServer: fabricxv1alpha1.FabricCAItemIntermediateParentServer{
								URL:    "",
								CAName: "",
							},
						},
						CFG: fabricxv1alpha1.FabricCAItemCFG{
							Identities: fabricxv1alpha1.FabricCAItemCFGIdentities{
								AllowRemove: false,
							},
							Affiliations: fabricxv1alpha1.FabricCAItemCFGAffiliations{
								AllowRemove: false,
							},
						},
					},
					TLSCA: fabricxv1alpha1.FabricCAItemConf{
						Name: "tlsca",
						Registry: fabricxv1alpha1.FabricCAItemRegistry{
							MaxEnrollments: -1,
							Identities: []fabricxv1alpha1.FabricCAIdentity{
								{
									Name:        "admin",
									Pass:        "adminpw",
									Type:        "client",
									Affiliation: "",
									Attrs: fabricxv1alpha1.FabricCAIdentityAttrs{
										RegistrarRoles: "*",
										DelegateRoles:  "*",
										Attributes:     "*",
										Revoker:        true,
										IntermediateCA: true,
										GenCRL:         true,
										AffiliationMgr: true,
									},
								},
							},
						},

						CSR: fabricxv1alpha1.FabricCACSR{
							CN: "fabric-tlsca-server",
							Hosts: []string{
								"localhost",
								"tlsca.example.com",
							},
							Names: []fabricxv1alpha1.FabricCANames{
								{
									C:  "US",
									ST: "North Carolina",
									L:  "Raleigh",
									O:  "Hyperledger",
									OU: "Fabric",
								},
							},
							CA: fabricxv1alpha1.FabricCACSRCA{
								Expiry:     "131400h",
								PathLength: 0,
							},
						},
						BCCSP: fabricxv1alpha1.FabricCAItemBCCSP{
							Default: "SW",
							SW: fabricxv1alpha1.FabricCAItemBCCSPSW{
								Hash:     "SHA2",
								Security: 256,
							},
						},
						Intermediate: fabricxv1alpha1.FabricCAItemIntermediate{
							ParentServer: fabricxv1alpha1.FabricCAItemIntermediateParentServer{
								URL:    "",
								CAName: "",
							},
						},
						CFG: fabricxv1alpha1.FabricCAItemCFG{
							Identities: fabricxv1alpha1.FabricCAItemCFGIdentities{
								AllowRemove: false,
							},
							Affiliations: fabricxv1alpha1.FabricCAItemCFGAffiliations{
								AllowRemove: false,
							},
						},
					},
				},
			},
			expectedStatus: fabricxv1alpha1.PendingStatus, // CA starts with PENDING status
			expectError:    false,
		},
		{
			name:           "CA not found",
			ca:             nil,
			expectedStatus: fabricxv1alpha1.PendingStatus,
			expectError:    false, // Reconcile returns nil when CA not found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			var objs []client.Object
			if tt.ca != nil {
				objs = append(objs, tt.ca)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

			// Create reconciler
			r := &CAReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-ca",
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

				// Only check CA if it was provided in the test
				if tt.ca != nil {
					// Check if CA was created/updated
					var ca fabricxv1alpha1.CA
					err = fakeClient.Get(context.Background(), req.NamespacedName, &ca)
					if err != nil {
						t.Errorf("Failed to get CA: %v", err)
					}

					// Check status
					if ca.Status.Status != tt.expectedStatus {
						t.Errorf("Expected status %s, got %s", tt.expectedStatus, ca.Status.Status)
					}

					// Check if finalizer was added
					if !containsString(ca.Finalizers, caFinalizer) {
						t.Errorf("Expected finalizer to be added")
					}
				}
			}

			// Check reconciliation result
			if !result.Requeue && result.RequeueAfter != 0 {
				t.Errorf("Unexpected reconciliation result: %v", result)
			}
		})
	}
}

func TestCAReconciler_setDefaults(t *testing.T) {
	r := &CAReconciler{}

	tests := []struct {
		name     string
		ca       *fabricxv1alpha1.CA
		expected *fabricxv1alpha1.CA
	}{
		{
			name: "set all defaults",
			ca: &fabricxv1alpha1.CA{
				Spec: fabricxv1alpha1.CASpec{},
			},
			expected: &fabricxv1alpha1.CA{
				Spec: fabricxv1alpha1.CASpec{
					Image:   "hyperledger/fabric-ca:1.5.15",
					Version: "1.5.15",
					Replicas: func() *int32 {
						replicas := int32(1)
						return &replicas
					}(),
					CredentialStore: fabricxv1alpha1.CredentialStoreKubernetes,
					Service: fabricxv1alpha1.FabricCASpecService{
						ServiceType: corev1.ServiceTypeClusterIP,
					},
					Storage: fabricxv1alpha1.FabricCAStorage{
						AccessMode: "ReadWriteOnce",
						Size:       "1Gi",
					},
					CLRSizeLimit: 512000,
					Database: fabricxv1alpha1.FabricCADatabase{
						Type:       "sqlite3",
						Datasource: "/var/hyperledger/fabric-ca/fabric-ca-server.db",
					},
					Metrics: fabricxv1alpha1.FabricCAMetrics{
						Provider: "prometheus",
						Statsd: fabricxv1alpha1.FabricCAMetricsStatsd{
							Network:       "udp",
							Address:       "127.0.0.1:8125",
							WriteInterval: "10s",
							Prefix:        "fabric-ca",
						},
					},
					CA: fabricxv1alpha1.FabricCAItemConf{
						Name: "ca",
						CRL: fabricxv1alpha1.FabricCACRL{
							Expiry: "24h",
						},
						Registry: fabricxv1alpha1.FabricCAItemRegistry{
							MaxEnrollments: -1,
						},
						CSR: fabricxv1alpha1.FabricCACSR{
							CN: "fabric-ca-server",
							Names: []fabricxv1alpha1.FabricCANames{
								{
									C:  "US",
									ST: "North Carolina",
									L:  "",
									O:  "Hyperledger",
									OU: "Fabric",
								},
							},
							CA: fabricxv1alpha1.FabricCACSRCA{
								Expiry: "131400h",
							},
						},
						BCCSP: fabricxv1alpha1.FabricCAItemBCCSP{
							Default: "SW",
							SW: fabricxv1alpha1.FabricCAItemBCCSPSW{
								Hash:     "SHA2",
								Security: 256,
							},
						},
					},
					TLSCA: fabricxv1alpha1.FabricCAItemConf{
						Name: "tlsca",
						CRL: fabricxv1alpha1.FabricCACRL{
							Expiry: "24h",
						},
						Registry: fabricxv1alpha1.FabricCAItemRegistry{
							MaxEnrollments: -1,
						},
						CSR: fabricxv1alpha1.FabricCACSR{
							CN: "fabric-tlsca-server",
							Names: []fabricxv1alpha1.FabricCANames{
								{
									C:  "US",
									ST: "North Carolina",
									L:  "",
									O:  "Hyperledger",
									OU: "Fabric",
								},
							},
							CA: fabricxv1alpha1.FabricCACSRCA{
								Expiry: "131400h",
							},
						},
						BCCSP: fabricxv1alpha1.FabricCAItemBCCSP{
							Default: "SW",
							SW: fabricxv1alpha1.FabricCAItemBCCSPSW{
								Hash:     "SHA2",
								Security: 256,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.setDefaults(tt.ca)

			// Check key defaults
			if tt.ca.Spec.Image != tt.expected.Spec.Image {
				t.Errorf("Expected Image %s, got %s", tt.expected.Spec.Image, tt.ca.Spec.Image)
			}
			if tt.ca.Spec.Version != tt.expected.Spec.Version {
				t.Errorf("Expected Version %s, got %s", tt.expected.Spec.Version, tt.ca.Spec.Version)
			}
			if *tt.ca.Spec.Replicas != *tt.expected.Spec.Replicas {
				t.Errorf("Expected Replicas %d, got %d", *tt.expected.Spec.Replicas, *tt.ca.Spec.Replicas)
			}
			if tt.ca.Spec.CA.Name != tt.expected.Spec.CA.Name {
				t.Errorf("Expected CA Name %s, got %s", tt.expected.Spec.CA.Name, tt.ca.Spec.CA.Name)
			}
			if tt.ca.Spec.TLSCA.Name != tt.expected.Spec.TLSCA.Name {
				t.Errorf("Expected TLSCA Name %s, got %s", tt.expected.Spec.TLSCA.Name, tt.ca.Spec.TLSCA.Name)
			}
		})
	}
}

func TestCAReconciler_ComputeConfigMapHash(t *testing.T) {
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	// Create test CA
	ca := &fabricxv1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.CASpec{
			CA: fabricxv1alpha1.FabricCAItemConf{
				Name: "ca",
			},
			TLSCA: fabricxv1alpha1.FabricCAItemConf{
				Name: "tlsca",
			},
		},
	}

	// Create test ConfigMaps
	caConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca-config",
			Namespace: "default",
		},
		Data: map[string]string{
			"ca.yaml": "version: 1.4.9\nport: 7054\ndebug: true",
		},
	}

	tlsConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca-config-tls",
			Namespace: "default",
		},
		Data: map[string]string{
			"fabric-ca-server-config.yaml": "tls:\n  enabled: true",
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ca, caConfigMap, tlsConfigMap).Build()

	// Create reconciler
	r := &CAReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test hash computation
	ctx := context.Background()
	hash1, err := r.ComputeConfigMapHash(ctx, ca)
	if err != nil {
		t.Fatalf("Failed to compute hash: %v", err)
	}

	if hash1 == "" {
		t.Error("Expected non-empty hash")
	}

	// Test hash changes when ConfigMap changes
	caConfigMap.Data["ca.yaml"] = "version: 1.4.9\nport: 7054\ndebug: false"
	if err := fakeClient.Update(ctx, caConfigMap); err != nil {
		t.Fatalf("Failed to update ConfigMap: %v", err)
	}

	hash2, err := r.ComputeConfigMapHash(ctx, ca)
	if err != nil {
		t.Fatalf("Failed to compute new hash: %v", err)
	}

	if hash1 == hash2 {
		t.Error("Expected hash to change when ConfigMap content changes")
	}
}

func TestCAReconciler_GetDeploymentTemplate(t *testing.T) {
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	ca := &fabricxv1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca",
			Namespace: "default",
		},
		Spec: fabricxv1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:1.5.15",
			Version: "1.5.15",
			Replicas: func() *int32 {
				replicas := int32(1)
				return &replicas
			}(),
			CA: fabricxv1alpha1.FabricCAItemConf{
				Name: "ca",
			},
			TLSCA: fabricxv1alpha1.FabricCAItemConf{
				Name: "tlsca",
			},
		},
	}

	// Create test ConfigMaps
	caConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca-config",
			Namespace: "default",
		},
		Data: map[string]string{
			"ca.yaml": "version: 1.4.9",
		},
	}

	tlsConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca-config-tls",
			Namespace: "default",
		},
		Data: map[string]string{
			"fabric-ca-server-config.yaml": "tls:\n  enabled: true",
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ca, caConfigMap, tlsConfigMap).Build()

	// Create reconciler
	r := &CAReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test deployment template generation
	ctx := context.Background()
	deployment := r.GetDeploymentTemplate(ctx, ca)

	// Check deployment properties
	if deployment.Name != ca.Name {
		t.Errorf("Expected deployment name %s, got %s", ca.Name, deployment.Name)
	}

	if deployment.Namespace != ca.Namespace {
		t.Errorf("Expected deployment namespace %s, got %s", ca.Namespace, deployment.Namespace)
	}

	if *deployment.Spec.Replicas != *ca.Spec.Replicas {
		t.Errorf("Expected replicas %d, got %d", *ca.Spec.Replicas, *deployment.Spec.Replicas)
	}

	// Check if ConfigMap hash annotation is present
	if hash, exists := deployment.Spec.Template.Annotations["checksum/config"]; !exists {
		t.Error("Expected ConfigMap hash annotation to be present")
	} else if hash == "" {
		t.Error("Expected non-empty ConfigMap hash")
	}

	// Check container properties
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		t.Error("Expected at least one container")
	}

	container := deployment.Spec.Template.Spec.Containers[0]
	expectedImage := "hyperledger/fabric-ca:1.5.15:1.5.15" // Image:Version format
	if container.Image != expectedImage {
		t.Errorf("Expected container image %s, got %s", expectedImage, container.Image)
	}

	// Check volume mounts
	expectedVolumeMounts := []string{"data", "ca-config", "ca-config-tls", "tls-secret", "msp-cryptomaterial", "msp-tls-cryptomaterial"}
	for _, expectedMount := range expectedVolumeMounts {
		found := false
		for _, mount := range container.VolumeMounts {
			if mount.Name == expectedMount {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected volume mount %s not found", expectedMount)
		}
	}
}

func TestCAReconciler_handleDeletion(t *testing.T) {
	s := scheme.Scheme
	fabricxv1alpha1.AddToScheme(s)

	// Create CA with deletion timestamp
	ca := &fabricxv1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ca",
			Namespace:         "default",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{caFinalizer},
		},
		Spec: fabricxv1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:1.5.15",
			Version: "1.5.15",
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ca).Build()

	// Create reconciler
	r := &CAReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	// Test deletion handling
	ctx := context.Background()
	result, err := r.handleDeletion(ctx, ca)

	if err != nil {
		t.Errorf("Unexpected error during deletion: %v", err)
	}

	// Check that finalizer was removed (CA might be deleted during cleanup)
	var updatedCA fabricxv1alpha1.CA
	err = fakeClient.Get(ctx, types.NamespacedName{Name: ca.Name, Namespace: ca.Namespace}, &updatedCA)
	if err == nil {
		// If CA still exists, check that finalizer was removed
		if containsString(updatedCA.Finalizers, caFinalizer) {
			t.Error("Expected finalizer to be removed")
		}
	}
	// If CA was deleted, that's also acceptable behavior

	// Check reconciliation result
	if result.Requeue {
		t.Error("Expected no requeue after successful deletion")
	}
}

// Helper function to check if slice contains string
func containsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}
