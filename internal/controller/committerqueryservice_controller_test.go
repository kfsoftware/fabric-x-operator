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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

var _ = Describe("CommitterQueryService Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-query-service"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		queryService := &fabricxv1alpha1.CommitterQueryService{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind CommitterQueryService")
			err := k8sClient.Get(ctx, typeNamespacedName, queryService)
			if err != nil && errors.IsNotFound(err) {
				resource := &fabricxv1alpha1.CommitterQueryService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: fabricxv1alpha1.CommitterQueryServiceSpec{
						BootstrapMode: "configure",
						MSPID:         "Org1MSP",
						PartyID:       1,
						Replicas:      1,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &fabricxv1alpha1.CommitterQueryService{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance CommitterQueryService")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CommitterQueryServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create config secret in deploy mode", func() {
			By("Setting bootstrap mode to deploy")
			Expect(k8sClient.Get(ctx, typeNamespacedName, queryService)).To(Succeed())
			queryService.Spec.BootstrapMode = "deploy"
			Expect(k8sClient.Update(ctx, queryService)).To(Succeed())

			By("Reconciling the resource")
			controllerReconciler := &CommitterQueryServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if config secret was created")
			secret := &corev1.Secret{}
			secretName := resourceName + "-config"
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      secretName,
					Namespace: "default",
				}, secret)
			}).Should(Succeed())

			By("Verifying secret contains config.yaml")
			Expect(secret.Data).To(HaveKey("config.yaml"))
		})
	})

	Context("When testing BuildQueryServiceConfig", func() {
		It("should generate valid query service configuration", func() {
			data := utils.CommitterQueryServiceTemplateData{
				Name:    "test-qs",
				PartyID: 1,
				MSPID:   "Org1MSP",
				Port:    9001,
			}

			config := utils.BuildQueryServiceConfig(data)

			By("Verifying configuration contains required fields")
			Expect(config).To(ContainSubstring("server:"))
			Expect(config).To(ContainSubstring("endpoint: 0.0.0.0:9001"))
			Expect(config).To(ContainSubstring("party-id: 1"))
			Expect(config).To(ContainSubstring("msp-id: Org1MSP"))
			Expect(config).To(ContainSubstring("min-batch-keys: 1024"))
			Expect(config).To(ContainSubstring("max-batch-wait: 100ms"))
			Expect(config).To(ContainSubstring("view-aggregation-window: 100ms"))
			Expect(config).To(ContainSubstring("max-aggregated-views: 1024"))
			Expect(config).To(ContainSubstring("max-view-timeout: 10s"))
		})

		It("should generate configuration with different port", func() {
			data := utils.CommitterQueryServiceTemplateData{
				Name:    "test-qs-2",
				PartyID: 2,
				MSPID:   "Org2MSP",
				Port:    9002,
			}

			config := utils.BuildQueryServiceConfig(data)

			Expect(config).To(ContainSubstring("endpoint: 0.0.0.0:9002"))
			Expect(config).To(ContainSubstring("party-id: 2"))
			Expect(config).To(ContainSubstring("msp-id: Org2MSP"))
		})
	})
})
