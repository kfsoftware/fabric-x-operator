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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

var _ = Describe("ChainNamespace Controller", func() {
	Context("When reconciling a ChainNamespace resource", func() {
		ctx := context.Background()

		var (
			testCounter = 0
		)

		// Helper function to create test resources
		createTestPrerequisites := func(suffix string) {
			identityName := fmt.Sprintf("test-identity-%s", suffix)
			enrollSecret := fmt.Sprintf("test-enroll-secret-%s", suffix)
			caCertSecret := fmt.Sprintf("test-ca-cert-%s", suffix)

			By("creating prerequisite secrets and identity")

			// Create enrollment secret first
			enrollSecretObj := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      enrollSecret,
					Namespace: "default",
				},
				StringData: map[string]string{
					"password": "adminpw",
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: enrollSecretObj.Name, Namespace: enrollSecretObj.Namespace}, &corev1.Secret{})
			if err != nil && apierrors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, enrollSecretObj)).To(Succeed())
			}

			// Create a test identity with minimal enrollment config
			identity := &fabricxv1alpha1.Identity{
				ObjectMeta: metav1.ObjectMeta{
					Name:      identityName,
					Namespace: "default",
				},
				Spec: fabricxv1alpha1.IdentitySpec{
					Type:  "user",
					MspID: "Org1MSP",
					Enrollment: &fabricxv1alpha1.IdentityEnrollment{
						CARef: fabricxv1alpha1.IdentityCARef{
							Name:      "test-ca",
							Namespace: "default",
						},
						EnrollID: "admin",
						EnrollSecretRef: fabricxv1alpha1.SecretKeyNSSelector{
							Name:      enrollSecret,
							Key:       "password",
							Namespace: "default",
						},
						EnrollTLS: false,
					},
					Output: fabricxv1alpha1.IdentityOutput{
						SecretPrefix: identityName,
						Namespace:    "default",
					},
				},
				Status: fabricxv1alpha1.IdentityStatus{
					Status: "Ready",
				},
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: identityName, Namespace: "default"}, identity)
			if err != nil && apierrors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, identity)).To(Succeed())
			}

			// Create secrets that would be created by identity enrollment
			secrets := []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      identityName + "-sign-cert",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": []byte("-----BEGIN CERTIFICATE-----\nMIICert\n-----END CERTIFICATE-----"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      identityName + "-sign-key",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": []byte("-----BEGIN EC PRIVATE KEY-----\nMIIKey\n-----END EC PRIVATE KEY-----"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      identityName + "-sign-cacert",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"cert.pem": []byte("-----BEGIN CERTIFICATE-----\nMIICACert\n-----END CERTIFICATE-----"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      caCertSecret,
						Namespace: "default",
					},
					Data: map[string][]byte{
						"ca.pem": []byte("-----BEGIN CERTIFICATE-----\nMIICACert\n-----END CERTIFICATE-----"),
					},
				},
			}

			for _, secret := range secrets {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &corev1.Secret{})
				if err != nil && apierrors.IsNotFound(err) {
					Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				}
			}
		}

		// Helper function to cleanup test resources
		cleanupTestPrerequisites := func(suffix string) {
			identityName := fmt.Sprintf("test-identity-%s", suffix)
			enrollSecret := fmt.Sprintf("test-enroll-secret-%s", suffix)
			caCertSecret := fmt.Sprintf("test-ca-cert-%s", suffix)

			By("cleaning up prerequisite resources")
			// Clean up identity
			identity := &fabricxv1alpha1.Identity{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: identityName, Namespace: "default"}, identity)
			if err == nil {
				Expect(k8sClient.Delete(ctx, identity)).To(Succeed())
			}

			// Clean up secrets
			secretNames := []string{
				identityName + "-sign-cert",
				identityName + "-sign-key",
				identityName + "-sign-cacert",
				caCertSecret,
				enrollSecret,
			}
			for _, name := range secretNames {
				secret := &corev1.Secret{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, secret)
				if err == nil {
					Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
				}
			}
		}

		It("should add finalizer on first reconcile", func() {
			testCounter++
			suffix := fmt.Sprintf("finalizer-%d", testCounter)
			resourceName := fmt.Sprintf("test-namespace-%s", suffix)
			identityName := fmt.Sprintf("test-identity-%s", suffix)
			caCertSecret := fmt.Sprintf("test-ca-cert-%s", suffix)

			createTestPrerequisites(suffix)
			defer cleanupTestPrerequisites(suffix)

			By("creating the custom resource for ChainNamespace")
			resource := &fabricxv1alpha1.ChainNamespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: fabricxv1alpha1.NamespaceSpec{
					Name:    "test-ns",
					Orderer: "orderer.example.com:7050",
					CACert: fabricxv1alpha1.SecretKeyRef{
						Name:      caCertSecret,
						Namespace: "default",
						Key:       "ca.pem",
					},
					MSPID: "Org1MSP",
					Identity: fabricxv1alpha1.SecretKeyRef{
						Name:      identityName,
						Namespace: "default",
						Key:       "identity",
					},
					Channel: "mychannel",
					Version: -1,
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() {
				By("cleaning up the ChainNamespace resource")
				res := &fabricxv1alpha1.ChainNamespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, res)
				if err == nil {
					Expect(k8sClient.Delete(ctx, res)).To(Succeed())
				}
			}()

			By("reconciling the created resource")
			controllerReconciler := &ChainNamespaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: resourceName},
			})
			// Expect error because orderer is not reachable in test environment
			// But finalizer should be added

			By("verifying finalizer was added")
			updatedResource := &fabricxv1alpha1.ChainNamespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, updatedResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedResource.Finalizers).To(ContainElement(ChainNamespaceFinalizerName))
		})

		It("should validate required fields", func() {
			testCounter++
			suffix := fmt.Sprintf("validation-%d", testCounter)
			resourceName := fmt.Sprintf("test-namespace-%s", suffix)

			createTestPrerequisites(suffix)
			defer cleanupTestPrerequisites(suffix)

			By("creating a ChainNamespace with missing fields")
			resource := &fabricxv1alpha1.ChainNamespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: fabricxv1alpha1.NamespaceSpec{
					Name: "", // Invalid: empty name
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() {
				By("cleaning up the ChainNamespace resource")
				res := &fabricxv1alpha1.ChainNamespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, res)
				if err == nil {
					Expect(k8sClient.Delete(ctx, res)).To(Succeed())
				}
			}()

			By("reconciling the resource")
			controllerReconciler := &ChainNamespaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: resourceName},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation error"))

			By("verifying status reflects failure")
			updatedResource := &fabricxv1alpha1.ChainNamespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, updatedResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedResource.Status.Status).To(Equal("Failed"))
		})

		It("should skip reconciliation if already deployed", func() {
			testCounter++
			suffix := fmt.Sprintf("deployed-%d", testCounter)
			resourceName := fmt.Sprintf("test-namespace-%s", suffix)
			identityName := fmt.Sprintf("test-identity-%s", suffix)
			caCertSecret := fmt.Sprintf("test-ca-cert-%s", suffix)

			createTestPrerequisites(suffix)
			defer cleanupTestPrerequisites(suffix)

			By("creating a ChainNamespace")
			resource := &fabricxv1alpha1.ChainNamespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Finalizers: []string{ChainNamespaceFinalizerName},
				},
				Spec: fabricxv1alpha1.NamespaceSpec{
					Name:    "test-ns",
					Orderer: "orderer.example.com:7050",
					CACert: fabricxv1alpha1.SecretKeyRef{
						Name:      caCertSecret,
						Namespace: "default",
						Key:       "ca.pem",
					},
					MSPID: "Org1MSP",
					Identity: fabricxv1alpha1.SecretKeyRef{
						Name:      identityName,
						Namespace: "default",
						Key:       "identity",
					},
					Channel: "mychannel",
					Version: -1,
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			defer func() {
				By("cleaning up the ChainNamespace resource")
				res := &fabricxv1alpha1.ChainNamespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, res)
				if err == nil {
					Expect(k8sClient.Delete(ctx, res)).To(Succeed())
				}
			}()

			By("updating the status to mark it as deployed")
			// In Kubernetes, status must be updated separately from spec
			updatedResource := &fabricxv1alpha1.ChainNamespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, updatedResource)).To(Succeed())
			updatedResource.Status.Status = "Deployed"
			updatedResource.Status.TxID = "test-tx-id-123"
			updatedResource.Status.Message = "Already deployed"
			Expect(k8sClient.Status().Update(ctx, updatedResource)).To(Succeed())

			By("reconciling the already-deployed resource")
			controllerReconciler := &ChainNamespaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: resourceName},
			})
			Expect(err).NotTo(HaveOccurred())

			By("verifying status remains unchanged")
			finalResource := &fabricxv1alpha1.ChainNamespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, finalResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalResource.Status.Status).To(Equal("Deployed"))
			Expect(finalResource.Status.TxID).To(Equal("test-tx-id-123"))
		})
	})
})
