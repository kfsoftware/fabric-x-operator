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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	ca "github.com/kfsoftware/fabric-x-operator/internal/controller/ca"
)

var _ = Describe("CA Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		caInstance := &fabricxv1alpha1.CA{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind CA")
			err := k8sClient.Get(ctx, typeNamespacedName, caInstance)
			if err != nil && errors.IsNotFound(err) {
				resource := &fabricxv1alpha1.CA{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: fabricxv1alpha1.CASpec{
						Image:   "hyperledger/fabric-ca:1.5.15",
						Version: "1.5.15",
						Hosts:   []string{"localhost"},
						Database: fabricxv1alpha1.FabricCADatabase{
							Type:       "sqlite3",
							Datasource: "/var/hyperledger/fabric-ca/fabric-ca-server.db",
						},
						CA: fabricxv1alpha1.FabricCAItemConf{
							Name: "ca",
							CSR: fabricxv1alpha1.FabricCACSR{
								CN:    "ca",
								Hosts: []string{"localhost"},
								Names: []fabricxv1alpha1.FabricCANames{
									{
										C:  "US",
										ST: "North Carolina",
										O:  "Hyperledger",
										L:  "Raleigh",
										OU: "Fabric",
									},
								},
								CA: fabricxv1alpha1.FabricCACSRCA{
									Expiry:     "131400h",
									PathLength: 0,
								},
							},
							CRL: fabricxv1alpha1.FabricCACRL{
								Expiry: "24h",
							},
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
							CSR: fabricxv1alpha1.FabricCACSR{
								CN:    "tlsca",
								Hosts: []string{"localhost"},
								Names: []fabricxv1alpha1.FabricCANames{
									{
										C:  "US",
										ST: "North Carolina",
										O:  "Hyperledger",
										L:  "Raleigh",
										OU: "Fabric",
									},
								},
								CA: fabricxv1alpha1.FabricCACSRCA{
									Expiry:     "131400h",
									PathLength: 0,
								},
							},
							CRL: fabricxv1alpha1.FabricCACRL{
								Expiry: "24h",
							},
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
							BCCSP: fabricxv1alpha1.FabricCAItemBCCSP{
								Default: "SW",
								SW: fabricxv1alpha1.FabricCAItemBCCSPSW{
									Hash:     "SHA2",
									Security: 256,
								},
							},
						},
						TLS: fabricxv1alpha1.FabricCATLS{
							Subject: fabricxv1alpha1.FabricCANames{
								C:  "US",
								ST: "North Carolina",
								O:  "Hyperledger",
								L:  "Raleigh",
								OU: "Fabric",
							},
						},
						Service: fabricxv1alpha1.FabricCASpecService{
							ServiceType: "ClusterIP",
						},
						Storage: fabricxv1alpha1.FabricCAStorage{
							StorageClass: "",
							AccessMode:   "ReadWriteOnce",
							Size:         "1Gi",
						},
						CredentialStore: fabricxv1alpha1.CredentialStoreKubernetes,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &fabricxv1alpha1.CA{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance CA")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ca.CAReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
