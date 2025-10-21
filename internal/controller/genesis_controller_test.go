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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	genesisutils "github.com/kfsoftware/fabric-x-operator/internal/controller/genesis"
)

// generateTestCertificate creates a valid test certificate using Go's crypto library
func generateTestCertificate() []byte {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	return certPEM
}

var _ = Describe("Genesis Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		genesis := &fabricxv1alpha1.Genesis{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Genesis")
			err := k8sClient.Get(ctx, typeNamespacedName, genesis)
			if err != nil && errors.IsNotFound(err) {
				// Generate test certificates
				caCert := generateTestCertificate()
				adminCert := generateTestCertificate()
				tlsCert := generateTestCertificate()
				identityCert := generateTestCertificate()

				// Create mock secrets for OrdererOrg
				ordererOrgSignCASecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ordererorg-sign-ca-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"ca.crt": caCert,
					},
				}
				Expect(k8sClient.Create(ctx, ordererOrgSignCASecret)).To(Succeed())

				ordererOrgTLSCASecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ordererorg-tls-ca-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"ca.crt": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, ordererOrgTLSCASecret)).To(Succeed())

				ordererOrgAdminSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ordererorg-admin-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"admin.crt": adminCert,
					},
				}
				Expect(k8sClient.Create(ctx, ordererOrgAdminSecret)).To(Succeed())

				// Create mock secrets for TestOrg
				testOrgSignCASecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testorg-sign-ca-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"ca.crt": caCert,
					},
				}
				Expect(k8sClient.Create(ctx, testOrgSignCASecret)).To(Succeed())

				testOrgTLSCASecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testorg-tls-ca-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"ca.crt": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, testOrgTLSCASecret)).To(Succeed())

				testOrgAdminSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testorg-admin-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"admin.crt": adminCert,
					},
				}
				Expect(k8sClient.Create(ctx, testOrgAdminSecret)).To(Succeed())

				// Create mock secrets for orderer nodes
				orderer1TLSSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orderer1-tls-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"tls.crt": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, orderer1TLSSecret)).To(Succeed())

				orderer1IdentitySecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orderer1-identity-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"identity.crt": identityCert,
					},
				}
				Expect(k8sClient.Create(ctx, orderer1IdentitySecret)).To(Succeed())

				orderer2TLSSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orderer2-tls-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"tls.crt": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, orderer2TLSSecret)).To(Succeed())

				orderer2IdentitySecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orderer2-identity-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"identity.crt": identityCert,
					},
				}
				Expect(k8sClient.Create(ctx, orderer2IdentitySecret)).To(Succeed())

				// Create meta namespace CA secret
				metaNamespaceCASecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-meta-ca-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"ca.pem": caCert,
					},
				}
				Expect(k8sClient.Create(ctx, metaNamespaceCASecret)).To(Succeed())

				// Create party certificate secrets for router, batcher, consenter, assembler
				partyRouterTLSSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "party1-router-tls-cert",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, partyRouterTLSSecret)).To(Succeed())

				partyBatcherSignSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "party1-batcher-sign-cert",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": identityCert,
					},
				}
				Expect(k8sClient.Create(ctx, partyBatcherSignSecret)).To(Succeed())

				partyBatcherTLSSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "party1-batcher-tls-cert",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, partyBatcherTLSSecret)).To(Succeed())

				partyConsenterSignSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "party1-consenter-sign-cert",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": identityCert,
					},
				}
				Expect(k8sClient.Create(ctx, partyConsenterSignSecret)).To(Succeed())

				partyConsenterTLSSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "party1-consenter-tls-cert",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, partyConsenterTLSSecret)).To(Succeed())

				partyAssemblerTLSSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "party1-assembler-tls-cert",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, partyAssemblerTLSSecret)).To(Succeed())

				// Create party CA secrets (required by party configuration)
				partyCACertSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ordererorg-ca-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"ca.crt": caCert,
					},
				}
				Expect(k8sClient.Create(ctx, partyCACertSecret)).To(Succeed())

				partyTLSCACertSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ordererorg-tlsca-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"ca.crt": tlsCert,
					},
				}
				Expect(k8sClient.Create(ctx, partyTLSCACertSecret)).To(Succeed())

				// Generate certificates for orderer organization
				ordererOrg, _, err := genesisutils.GenerateOrdererOrganization("ordererorg", "OrdererOrgMSP")
				Expect(err).NotTo(HaveOccurred())

				// Generate certificates for application organization
				appOrg, _, err := genesisutils.GenerateApplicationOrganization("testorg", "TestOrgMSP", "external")
				Expect(err).NotTo(HaveOccurred())

				// Generate orderer nodes for orderer organization
				consenters, _, err := genesisutils.GenerateConsenters("OrdererOrgMSP", 2, "orderer", 7050)
				Expect(err).NotTo(HaveOccurred())

				resource := &fabricxv1alpha1.Genesis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: fabricxv1alpha1.GenesisSpec{
						ChannelID: "testchannel",
						ConfigTemplate: fabricxv1alpha1.ConfigTemplateReference{
							ConfigMapName: "test-config-template",
							Key:           "configtx.yaml",
						},
						// Add orderer organization with generated certificates
						OrdererOrganizations: []fabricxv1alpha1.OrdererOrganization{*ordererOrg},
						// Add application organization
						ApplicationOrgs: []fabricxv1alpha1.ApplicationOrganization{*appOrg},
						// Add orderer nodes for orderer organization
						Consenters: consenters,
						// Add required meta namespace CA
						MetaNamespaceCA: fabricxv1alpha1.SecretKeyNSSelector{
							Name:      "test-meta-ca-secret",
							Namespace: "default",
							Key:       "ca.pem",
						},
						// Add at least 1 party (required for consensus)
						Parties: []fabricxv1alpha1.PartyConfig{
							{
								PartyID: 1,
								CACerts: []fabricxv1alpha1.SecretKeyNSSelector{
									{
										Name:      "ordererorg-ca-secret",
										Namespace: "default",
										Key:       "ca.crt",
									},
								},
								TLSCACerts: []fabricxv1alpha1.SecretKeyNSSelector{
									{
										Name:      "ordererorg-tlsca-secret",
										Namespace: "default",
										Key:       "ca.crt",
									},
								},
								RouterConfig: &fabricxv1alpha1.PartyRouterConfig{
									Host: "party1-router.default.svc.cluster.local",
									Port: 7150,
									TLSCert: fabricxv1alpha1.SecretKeyNSSelector{
										Name:      "party1-router-tls-cert",
										Namespace: "default",
										Key:       "cert.pem",
									},
								},
								BatchersConfig: []fabricxv1alpha1.PartyBatcherConfig{
									{
										ShardID: 1,
										Host:    "party1-batcher.default.svc.cluster.local",
										Port:    7151,
										SignCert: fabricxv1alpha1.SecretKeyNSSelector{
											Name:      "party1-batcher-sign-cert",
											Namespace: "default",
											Key:       "cert.pem",
										},
										TLSCert: fabricxv1alpha1.SecretKeyNSSelector{
											Name:      "party1-batcher-tls-cert",
											Namespace: "default",
											Key:       "cert.pem",
										},
									},
								},
								ConsenterConfig: &fabricxv1alpha1.PartyConsenterConfig{
									Host: "party1-consenter.default.svc.cluster.local",
									Port: 7052,
									SignCert: fabricxv1alpha1.SecretKeyNSSelector{
										Name:      "party1-consenter-sign-cert",
										Namespace: "default",
										Key:       "cert.pem",
									},
									TLSCert: fabricxv1alpha1.SecretKeyNSSelector{
										Name:      "party1-consenter-tls-cert",
										Namespace: "default",
										Key:       "cert.pem",
									},
								},
								AssemblerConfig: &fabricxv1alpha1.PartyAssemblerConfig{
									Host: "party1-assembler.default.svc.cluster.local",
									Port: 7153,
									TLSCert: fabricxv1alpha1.SecretKeyNSSelector{
										Name:      "party1-assembler-tls-cert",
										Namespace: "default",
										Key:       "cert.pem",
									},
								},
							},
						},
						Output: fabricxv1alpha1.GenesisOutput{
							SecretName: "test-genesis-secret",
							BlockKey:   "genesis.block",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &fabricxv1alpha1.Genesis{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Genesis")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &GenesisReconciler{
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
