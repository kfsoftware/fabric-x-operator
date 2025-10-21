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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

var _ = Describe("Endorser Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a resource", func() {
		const resourceName = "test-endorser"
		const namespace = "default"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}

		AfterEach(func() {
			// Cleanup
			resource := &fabricxv1alpha1.Endorser{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Endorser")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

				// Trigger reconciliation to process finalizer
				By("Reconciling the deletion")
				reconciler := &EndorserReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				// Wait for deletion
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, resource)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("should successfully reconcile a basic endorser in configure mode", func() {
			By("Creating the Endorser resource")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "configure",
					MSPID:         "Org1MSP",
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/node.crt",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/node.key",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
								Type:          "websocket",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Endorser status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, endorser)
				if err != nil {
					return false
				}
				return endorser.Status.Status != ""
			}, timeout, interval).Should(BeTrue())

			By("Verifying the Endorser is in PENDING or RUNNING status")
			Expect(k8sClient.Get(ctx, typeNamespacedName, endorser)).To(Succeed())
			Expect(endorser.Status.Status).To(Or(
				Equal(fabricxv1alpha1.PendingStatus),
				Equal(fabricxv1alpha1.RunningStatus),
			))
		})

		It("should create deployment and service in deploy mode", func() {
			By("Creating the Endorser resource in deploy mode")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "deploy",
					MSPID:         "Org1MSP",
					Image:         "hyperledger/fabric-smart-client",
					Version:       "latest",
					Common: &fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
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
					Core: fabricxv1alpha1.EndorserCoreConfig{
						Logging: &fabricxv1alpha1.LoggingConfig{
							Spec:   "info",
							Format: "%{color}%{time:15:04:05.000} [%{module}] %{shortfunc} %{level:.4s}%{color:reset} %{message}",
						},
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/node.crt",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/node.key",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
								Type:          "websocket",
							},
							Persistences: map[string]fabricxv1alpha1.PersistenceConfig{
								"default": {
									Type: "sqlite",
									Opts: map[string]string{
										"dataSource": "file:/var/hyperledger/fabric/data/fts.sqlite",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the core config secret is created")
			secretName := endorser.GetCoreConfigSecretName()
			secret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      secretName,
					Namespace: namespace,
				}, secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying the core.yaml content exists in secret")
			Expect(secret.Data).To(HaveKey("core.yaml"))
			Expect(len(secret.Data["core.yaml"])).To(BeNumerically(">", 0))

			By("Verifying the Service is created")
			service := &corev1.Service{}
			serviceName := endorser.GetServiceName()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}, service)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying the Service has correct ports")
			Expect(service.Spec.Ports).To(HaveLen(2))
			portNames := []string{}
			for _, port := range service.Spec.Ports {
				portNames = append(portNames, port.Name)
			}
			Expect(portNames).To(ContainElements("p2p", "metrics"))

			By("Verifying the Deployment is created")
			deployment := &appsv1.Deployment{}
			deploymentName := endorser.GetDeploymentName()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying the Deployment has correct replicas")
			Expect(deployment.Spec.Replicas).NotTo(BeNil())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))

			By("Verifying the Deployment has correct image")
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("hyperledger/fabric-smart-client:latest"))

			By("Verifying the Deployment has volume mounts")
			volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
			volumeMountNames := []string{}
			for _, vm := range volumeMounts {
				volumeMountNames = append(volumeMountNames, vm.Name)
			}
			// Without enrollment configuration, we only expect core-config
			Expect(volumeMountNames).To(ContainElement("core-config"))
		})

		It("should handle invalid bootstrap mode gracefully", func() {
			By("Creating the Endorser resource with invalid bootstrap mode")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "invalid",
					MSPID:         "Org1MSP",
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/node.crt",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/node.key",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())

			By("Verifying the Endorser status is FAILED")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, endorser)
				if err != nil {
					return false
				}
				return endorser.Status.Status == fabricxv1alpha1.FailedStatus
			}, timeout, interval).Should(BeTrue())

			By("Verifying the error message contains bootstrap mode info")
			Expect(k8sClient.Get(ctx, typeNamespacedName, endorser)).To(Succeed())
			Expect(endorser.Status.Message).To(ContainSubstring("Invalid bootstrap mode"))
		})

		PIt("should create PVC when storage is configured", func() {
			By("Creating the Endorser resource with storage configuration")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "deploy",
					MSPID:         "Org1MSP",
					Image:         "hyperledger/fabric-smart-client",
					Version:       "latest",
					Common: &fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
						Storage: &fabricxv1alpha1.StorageConfig{
							Size:         "10Gi",
							AccessMode:   "ReadWriteOnce",
							StorageClass: "standard",
						},
					},
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/node.crt",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/node.key",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the PVC is created")
			pvc := &corev1.PersistentVolumeClaim{}
			pvcName := resourceName + "-data"
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      pvcName,
					Namespace: namespace,
				}, pvc)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying the PVC has correct size")
			storageQuantity := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storageQuantity.String()).To(Equal("10Gi"))

			By("Verifying the PVC has correct access mode")
			Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))

			By("Verifying the PVC has correct storage class")
			Expect(pvc.Spec.StorageClassName).NotTo(BeNil())
			Expect(*pvc.Spec.StorageClassName).To(Equal("standard"))
		})
	})

	Context("Core YAML Generation", func() {
		It("should generate valid core.yaml from typed configuration", func() {
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-endorser",
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					Core: fabricxv1alpha1.EndorserCoreConfig{
						Logging: &fabricxv1alpha1.LoggingConfig{
							Spec:   "info",
							Format: "%{message}",
						},
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/path/to/cert",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/path/to/key",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
								Type:          "websocket",
							},
							Persistences: map[string]fabricxv1alpha1.PersistenceConfig{
								"default": {
									Type: "sqlite",
									Opts: map[string]string{
										"dataSource": "file:/data/db.sqlite",
									},
								},
							},
						},
						Fabric: &fabricxv1alpha1.FabricConfig{
							Enabled: true,
							Default: &fabricxv1alpha1.FabricDefaultConfig{
								TLS: &fabricxv1alpha1.FabricTLSConfig{
									Enabled: true,
								},
								Channels: []fabricxv1alpha1.ChannelConfig{
									{
										Name:    "arma",
										Default: true,
									},
								},
							},
						},
					},
				},
			}

			By("Generating core.yaml")
			coreYAML, err := endorser.GenerateCoreYAML()
			Expect(err).NotTo(HaveOccurred())
			Expect(coreYAML).NotTo(BeEmpty())

			By("Verifying core.yaml contains expected sections")
			Expect(coreYAML).To(ContainSubstring("logging:"))
			Expect(coreYAML).To(ContainSubstring("fsc:"))
			Expect(coreYAML).To(ContainSubstring("id: endorser1"))
			Expect(coreYAML).To(ContainSubstring("listenAddress: /ip4/0.0.0.0/tcp/9301"))
			Expect(coreYAML).To(ContainSubstring("fabric:"))
			Expect(coreYAML).To(ContainSubstring("enabled: true"))
		})

		It("should handle token configuration in core.yaml", func() {
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-endorser",
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{File: "/cert"},
								Key:  fabricxv1alpha1.KeyFileConfig{File: "/key"},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
							},
						},
						Token: &fabricxv1alpha1.TokenConfig{
							Enabled: true,
							TMS: map[string]fabricxv1alpha1.TMSConfig{
								"mytms": {
									Network:   "default",
									Channel:   "arma",
									Namespace: "token_ns",
									Driver:    "zkatdlog",
									Services: &fabricxv1alpha1.TMSServices{
										Network: &fabricxv1alpha1.NetworkServiceConfig{
											Fabric: &fabricxv1alpha1.FabricNetworkConfig{
												FSCEndorsement: &fabricxv1alpha1.FSCEndorsementConfig{
													Endorser:  true,
													ID:        "endorser",
													Endorsers: []string{"endorser1", "endorser2"},
													Policy: &fabricxv1alpha1.EndorsementPolicy{
														Type: "all",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			By("Generating core.yaml with token configuration")
			coreYAML, err := endorser.GenerateCoreYAML()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying token configuration is present")
			Expect(coreYAML).To(ContainSubstring("token:"))
			Expect(coreYAML).To(ContainSubstring("tms:"))
			Expect(coreYAML).To(ContainSubstring("mytms:"))
			Expect(coreYAML).To(ContainSubstring("driver: zkatdlog"))
			Expect(coreYAML).To(ContainSubstring("fsc_endorsement:"))
			Expect(coreYAML).To(ContainSubstring("endorsers:"))
		})
	})

	Context("Helper Functions", func() {
		It("should return correct secret names", func() {
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-endorser",
				},
			}

			Expect(endorser.GetCoreConfigSecretName()).To(Equal("test-endorser-core-config"))
			Expect(endorser.GetCertSecretName()).To(Equal("test-endorser-certs"))
			Expect(endorser.GetServiceName()).To(Equal("test-endorser-service"))
			Expect(endorser.GetDeploymentName()).To(Equal("test-endorser"))
		})

		It("should return correct image with tag", func() {
			endorser := &fabricxv1alpha1.Endorser{
				Spec: fabricxv1alpha1.EndorserSpec{
					Image:   "myregistry/endorser",
					Version: "v1.0.0",
				},
			}

			Expect(endorser.GetFullImage()).To(Equal("myregistry/endorser:v1.0.0"))
		})

		It("should use default image and version", func() {
			endorser := &fabricxv1alpha1.Endorser{}

			Expect(endorser.GetDefaultImage()).To(Equal("hyperledger/fabric-smart-client"))
			Expect(endorser.GetDefaultVersion()).To(Equal("latest"))
			Expect(endorser.GetFullImage()).To(Equal("hyperledger/fabric-smart-client:latest"))
		})
	})

	Context("Container Args Configuration", func() {
		const (
			resourceName = "test-endorser-args"
			namespace    = "default"
			timeout      = time.Second * 10
			interval     = time.Millisecond * 250
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}

		AfterEach(func() {
			// Cleanup
			resource := &fabricxv1alpha1.Endorser{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Endorser")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

				// Trigger reconciliation to process finalizer
				By("Reconciling the deletion")
				reconciler := &EndorserReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				// Wait for deletion
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, resource)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("should configure custom command and args for the container", func() {
			By("Creating the Endorser resource with custom command and args")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "deploy",
					MSPID:         "Org1MSP",
					Image:         "myregistry/custom-endorser",
					Version:       "v1.0.0",
					Command: []string{
						"app",
					},
					Args: []string{
						"--conf",
						"/var/hyperledger/fabric/config",
						"--port",
						"9000",
					},
					Common: &fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
					},
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/node.crt",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/node.key",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
								Type:          "websocket",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment is created with custom args")
			deployment := &appsv1.Deployment{}
			deploymentName := endorser.GetDeploymentName()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying the container has the correct command and args")
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Command).To(Equal([]string{
				"app",
			}))
			Expect(container.Args).To(Equal([]string{
				"--conf",
				"/var/hyperledger/fabric/config",
				"--port",
				"9000",
			}))
		})

		It("should handle empty args field", func() {
			By("Creating the Endorser resource without args")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName + "-no-args",
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "deploy",
					MSPID:         "Org1MSP",
					Image:         "hyperledger/fabric-smart-client",
					Version:       "latest",
					Common: &fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
					},
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/node.crt",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/node.key",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
								Type:          "websocket",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			typeNamespacedNameNoArgs := types.NamespacedName{
				Name:      resourceName + "-no-args",
				Namespace: namespace,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameNoArgs,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment is created without args")
			deployment := &appsv1.Deployment{}
			deploymentName := endorser.GetDeploymentName()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying the container has no command or args (nil or empty)")
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Command).To(Or(BeNil(), BeEmpty()))
			Expect(container.Args).To(Or(BeNil(), BeEmpty()))

			// Cleanup
			Expect(k8sClient.Delete(ctx, endorser)).To(Succeed())
			controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameNoArgs,
			})
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedNameNoArgs, endorser)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should update deployment when command and args change", func() {
			By("Creating the Endorser resource with initial command and args")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName + "-update",
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "deploy",
					MSPID:         "Org1MSP",
					Image:         "myregistry/custom-endorser",
					Version:       "v1.0.0",
					Command: []string{
						"app",
					},
					Args: []string{
						"--conf",
						"/var/hyperledger/fabric/config",
					},
					Common: &fabricxv1alpha1.CommonComponentConfig{
						Replicas: 1,
					},
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/node.crt",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/node.key",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
								Type:          "websocket",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			typeNamespacedNameUpdate := types.NamespacedName{
				Name:      resourceName + "-update",
				Namespace: namespace,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameUpdate,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying initial deployment command and args")
			deployment := &appsv1.Deployment{}
			deploymentName := endorser.GetDeploymentName()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			initialCommand := deployment.Spec.Template.Spec.Containers[0].Command
			Expect(initialCommand).To(Equal([]string{
				"app",
			}))
			initialArgs := deployment.Spec.Template.Spec.Containers[0].Args
			Expect(initialArgs).To(Equal([]string{
				"--conf",
				"/var/hyperledger/fabric/config",
			}))

			By("Updating the Endorser command and args")
			Expect(k8sClient.Get(ctx, typeNamespacedNameUpdate, endorser)).To(Succeed())
			endorser.Spec.Command = []string{
				"app",
			}
			endorser.Spec.Args = []string{
				"--conf",
				"/var/hyperledger/fabric/config",
				"--port",
				"9000",
				"--verbose",
			}
			Expect(k8sClient.Update(ctx, endorser)).To(Succeed())

			By("Reconciling after update")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameUpdate,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying updated deployment command and args")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, deployment)
				if err != nil {
					return false
				}
				return len(deployment.Spec.Template.Spec.Containers[0].Args) == 5
			}, timeout, interval).Should(BeTrue())

			updatedCommand := deployment.Spec.Template.Spec.Containers[0].Command
			Expect(updatedCommand).To(Equal([]string{
				"app",
			}))
			updatedArgs := deployment.Spec.Template.Spec.Containers[0].Args
			Expect(updatedArgs).To(Equal([]string{
				"--conf",
				"/var/hyperledger/fabric/config",
				"--port",
				"9000",
				"--verbose",
			}))

			// Cleanup
			Expect(k8sClient.Delete(ctx, endorser)).To(Succeed())
			controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameUpdate,
			})
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedNameUpdate, endorser)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("CA Enrollment Integration Tests", func() {
		const (
			namespace        = "default"
			caNamespace      = "default"
			caName           = "test-ca"
			caAdminUser      = "admin"
			caAdminPassword  = "adminpw"
			endorserUser     = "endorser1"
			endorserPassword = "endorser1pw"
			endorserTLSUser  = "endorser1-tls"
			endorserTLSPw    = "endorser1-tls-pw"
		)

		// NOTE: These tests demonstrate the certificate enrollment configuration structure
		// In the envtest environment, pods don't actually run and HTTP calls to CA fail
		//
		// For full e2e tests with actual CA enrollment, see test/e2e/ directory which uses
		// Kind/K3d clusters where fabric-ca-server pods can actually run and serve HTTP

		It("should support enrollment with inline CA certificate configuration", func() {
			// This test verifies the API structure for CA enrollment with inline certificate
			// It also verifies that the controller handles CA connection failures gracefully

			endorserName := "endorser-inline-cert"

			By("Creating Endorser with inline CA certificate configuration")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      endorserName,
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "configure",
					MSPID:         "Org1MSP",
					Enrollment: &fabricxv1alpha1.EnrollmentConfig{
						Sign: &fabricxv1alpha1.CertificateConfig{
							CA: &fabricxv1alpha1.CACertificateConfig{
								CAName:       caName,
								CAHost:       caName + "." + caNamespace + ".svc.cluster.local",
								CAPort:       7054,
								EnrollID:     endorserUser,
								EnrollSecret: endorserPassword,
								CATLS: &fabricxv1alpha1.CATLSConfig{
									// In real test, this would be the actual CA cert
									CACert: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...",
								},
							},
						},
						TLS: &fabricxv1alpha1.CertificateConfig{
							CA: &fabricxv1alpha1.CACertificateConfig{
								CAName:       caName,
								CAHost:       caName + "." + caNamespace + ".svc.cluster.local",
								CAPort:       7054,
								EnrollID:     endorserTLSUser,
								EnrollSecret: endorserTLSPw,
								CATLS: &fabricxv1alpha1.CATLSConfig{
									CACert: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...",
								},
							},
							SANS: &fabricxv1alpha1.SANSConfig{
								DNSNames: []string{
									endorserName,
									endorserName + "." + namespace + ".svc.cluster.local",
								},
								IPAddresses: []string{"127.0.0.1"},
							},
						},
					},
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/sign-cert.pem",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/sign-key.pem",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Verifying Endorser was created with correct enrollment configuration")
			createdEndorser := &fabricxv1alpha1.Endorser{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      endorserName,
				Namespace: namespace,
			}, createdEndorser)).To(Succeed())

			By("Verifying sign certificate configuration")
			Expect(createdEndorser.Spec.Enrollment).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.Sign).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.Sign.CA).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CAName).To(Equal(caName))
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CAHost).To(ContainSubstring(caName))
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CAPort).To(Equal(int32(7054)))
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.EnrollID).To(Equal(endorserUser))
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CATLS.CACert).NotTo(BeEmpty())

			By("Verifying TLS certificate configuration")
			Expect(createdEndorser.Spec.Enrollment.TLS).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.TLS.CA).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.TLS.CA.CAName).To(Equal(caName))
			Expect(createdEndorser.Spec.Enrollment.TLS.SANS).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.TLS.SANS.DNSNames).To(ContainElement(endorserName))
			Expect(createdEndorser.Spec.Enrollment.TLS.SANS.IPAddresses).To(ContainElement("127.0.0.1"))

			By("Verifying inline CA certificate is configured (not SecretRef)")
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CATLS.SecretRef).To(BeNil())
			Expect(createdEndorser.Spec.Enrollment.TLS.CA.CATLS.SecretRef).To(BeNil())

			By("Testing Reconcile() - should fail gracefully due to CA unavailable")
			reconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      endorserName,
					Namespace: namespace,
				},
			})

			By("Verifying reconcile failed due to CA connection error")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to enroll"))

			By("Verifying status was updated to FAILED")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      endorserName,
					Namespace: namespace,
				}, createdEndorser)
				if err != nil {
					return false
				}
				return createdEndorser.Status.Status == fabricxv1alpha1.FailedStatus
			}, timeout, interval).Should(BeTrue())

			By("Verifying failure message mentions enrollment")
			Expect(createdEndorser.Status.Message).To(ContainSubstring("Failed to reconcile"))

			// Cleanup
			k8sClient.Delete(ctx, endorser)
			reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      endorserName,
					Namespace: namespace,
				},
			})
		})

		It("should support enrollment with CA certificate from SecretRef configuration", func() {
			// This test verifies the API structure for SecretRef-based CA certificate
			// Actual CA enrollment requires HTTP connectivity to fabric-ca-server (e2e tests)

			endorserName := "endorser-secret-ref"

			By("Creating secret with CA certificate")
			caCertSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ca-tls-cert",
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					// In real test, this would be the actual CA cert
					"ca.pem": []byte("-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"),
				},
			}
			Expect(k8sClient.Create(ctx, caCertSecret)).To(Succeed())

			By("Creating Endorser with SecretRef for CA certificate")
			endorser := &fabricxv1alpha1.Endorser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      endorserName,
					Namespace: namespace,
				},
				Spec: fabricxv1alpha1.EndorserSpec{
					BootstrapMode: "configure",
					MSPID:         "Org1MSP",
					Enrollment: &fabricxv1alpha1.EnrollmentConfig{
						Sign: &fabricxv1alpha1.CertificateConfig{
							CA: &fabricxv1alpha1.CACertificateConfig{
								CAName:       caName,
								CAHost:       caName + "." + caNamespace + ".svc.cluster.local",
								CAPort:       7054,
								EnrollID:     endorserUser,
								EnrollSecret: endorserPassword,
								CATLS: &fabricxv1alpha1.CATLSConfig{
									// Reference secret instead of inline cert
									SecretRef: &fabricxv1alpha1.SecretRef{
										Name:      "ca-tls-cert",
										Key:       "ca.pem",
										Namespace: namespace,
									},
								},
							},
						},
						TLS: &fabricxv1alpha1.CertificateConfig{
							CA: &fabricxv1alpha1.CACertificateConfig{
								CAName:       caName,
								CAHost:       caName + "." + caNamespace + ".svc.cluster.local",
								CAPort:       7054,
								EnrollID:     endorserTLSUser,
								EnrollSecret: endorserTLSPw,
								CATLS: &fabricxv1alpha1.CATLSConfig{
									SecretRef: &fabricxv1alpha1.SecretRef{
										Name:      "ca-tls-cert",
										Key:       "ca.pem",
										Namespace: namespace,
									},
								},
							},
							SANS: &fabricxv1alpha1.SANSConfig{
								DNSNames: []string{
									endorserName,
									endorserName + "." + namespace + ".svc.cluster.local",
								},
							},
						},
					},
					// Component-specific SANS override
					SANS: &fabricxv1alpha1.SANSConfig{
						DNSNames: []string{
							endorserName + "-custom",
							endorserName + "-custom." + namespace + ".svc.cluster.local",
						},
						IPAddresses: []string{"10.0.0.100"},
					},
					Core: fabricxv1alpha1.EndorserCoreConfig{
						FSC: fabricxv1alpha1.FSCConfig{
							ID: "endorser1",
							Identity: fabricxv1alpha1.FSCIdentity{
								Cert: fabricxv1alpha1.CertFileConfig{
									File: "/var/hyperledger/fabric/keys/sign-cert.pem",
								},
								Key: fabricxv1alpha1.KeyFileConfig{
									File: "/var/hyperledger/fabric/keys/sign-key.pem",
								},
							},
							P2P: fabricxv1alpha1.FSCP2PConfig{
								ListenAddress: "/ip4/0.0.0.0/tcp/9301",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, endorser)).To(Succeed())

			By("Verifying Endorser was created with SecretRef configuration")
			createdEndorser := &fabricxv1alpha1.Endorser{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      endorserName,
				Namespace: namespace,
			}, createdEndorser)).To(Succeed())

			By("Verifying SecretRef is configured for sign certificate")
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CATLS.SecretRef).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CATLS.SecretRef.Name).To(Equal("ca-tls-cert"))
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CATLS.SecretRef.Key).To(Equal("ca.pem"))
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CATLS.SecretRef.Namespace).To(Equal(namespace))

			By("Verifying SecretRef is configured for TLS certificate")
			Expect(createdEndorser.Spec.Enrollment.TLS.CA.CATLS.SecretRef).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.TLS.CA.CATLS.SecretRef.Name).To(Equal("ca-tls-cert"))

			By("Verifying inline CA cert is NOT set (SecretRef is used instead)")
			Expect(createdEndorser.Spec.Enrollment.Sign.CA.CATLS.CACert).To(BeEmpty())
			Expect(createdEndorser.Spec.Enrollment.TLS.CA.CATLS.CACert).To(BeEmpty())

			By("Verifying component-specific SANS override enrollment SANS")
			Expect(createdEndorser.Spec.SANS).NotTo(BeNil())
			Expect(createdEndorser.Spec.SANS.DNSNames).To(ContainElement(endorserName + "-custom"))
			Expect(createdEndorser.Spec.SANS.IPAddresses).To(ContainElement("10.0.0.100"))

			By("Verifying enrollment SANS are different from component SANS")
			Expect(createdEndorser.Spec.Enrollment.TLS.SANS).NotTo(BeNil())
			Expect(createdEndorser.Spec.Enrollment.TLS.SANS.DNSNames).To(ContainElement(endorserName))
			Expect(createdEndorser.Spec.Enrollment.TLS.SANS.DNSNames).NotTo(ContainElement(endorserName + "-custom"))

			By("Testing Reconcile() - should fail gracefully due to CA unavailable")
			reconciler := &EndorserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      endorserName,
					Namespace: namespace,
				},
			})

			By("Verifying reconcile failed due to CA connection error")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Or(
				ContainSubstring("failed to enroll"),
				ContainSubstring("Failed to process certificate"),
			))

			By("Verifying status was updated to FAILED")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      endorserName,
					Namespace: namespace,
				}, createdEndorser)
				if err != nil {
					return false
				}
				return createdEndorser.Status.Status == fabricxv1alpha1.FailedStatus
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			k8sClient.Delete(ctx, caCertSecret)
			k8sClient.Delete(ctx, endorser)
			reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      endorserName,
					Namespace: namespace,
				},
			})
			reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      endorserName,
					Namespace: namespace,
				},
			})
		})
	})
})
