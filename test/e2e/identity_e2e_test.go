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

package e2e

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kfsoftware/fabric-x-operator/test/utils"
)

const testNamespace = "default"

var _ = Describe("Identity Controller E2E", Ordered, func() {
	var caName string
	var identityName string

	BeforeAll(func() {
		caName = "test-ca-e2e"
		identityName = "test-identity-e2e"

		By("deploying the test Fabric CA using CA controller")
		caYAML := fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: %s
  namespace: %s
spec:
  image: hyperledger/fabric-ca
  version: "1.5.15"
  ca:
    name: ca
    registry:
      identities:
        - name: admin
          pass: adminpw
          type: client
          affiliation: ""
          attrs:
            hf.Registrar.Roles: "*"
            hf.Revoker: true
  tlsca:
    name: tlsca
    registry:
      identities:
        - name: admin
          pass: adminpw
          type: client
          affiliation: ""
          attrs:
            hf.Registrar.Roles: "*"
            hf.Revoker: true
  tls:
    domains:
      - %s
      - %s.%s
      - %s.%s.svc.cluster.local
      - localhost
  service:
    type: ClusterIP
`, caName, testNamespace, caName, caName, testNamespace, caName, testNamespace)

		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = bytes.NewReader([]byte(caYAML))
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create CA resource")

		By("waiting for CA pod to be ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "wait", "--for=condition=ready",
				"pod", "-l", "app=ca",
				"-n", testNamespace,
				"--timeout=120s")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying CA service exists")
		cmd = exec.Command("kubectl", "get", "service",
			caName,
			"-n", testNamespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "CA service should exist")
	})

	AfterAll(func() {
		By("cleaning up test identity")
		cmd := exec.Command("kubectl", "delete", "identity", identityName,
			"-n", testNamespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("cleaning up test CA")
		cmd = exec.Command("kubectl", "delete", "ca", caName,
			"-n", testNamespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("waiting for CA pod to be deleted")
		cmd = exec.Command("kubectl", "wait", "--for=delete",
			"pod", "-l", "app=ca",
			"-n", testNamespace,
			"--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	Context("X.509 Enrollment", func() {
		It("should successfully enroll an identity with sign certificate", func() {
			By("creating enrollment secret")
			enrollSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s-enroll-secret
  namespace: %s
type: Opaque
stringData:
  password: adminpw
`, identityName, testNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(enrollSecretYAML))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create enrollment secret")

			By("creating Identity resource with service DNS name")
			identityYAML := fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Identity
metadata:
  name: %s
  namespace: %s
spec:
  type: user
  mspID: Org1MSP
  enrollment:
    caRef:
      name: %s
      namespace: %s
    enrollID: admin
    enrollSecretRef:
      name: %s-enroll-secret
      key: password
      namespace: %s
  output:
    secretName: %s-cert
`, identityName, testNamespace, caName, testNamespace, identityName, testNamespace, identityName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(identityYAML))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Identity resource")

			By("waiting for Identity to be READY")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "identity", identityName,
					"-n", testNamespace,
					"-o", "jsonpath={.status.status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("READY"), "Identity should reach READY status")
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying cert secret was created")
			cmd = exec.Command("kubectl", "get", "secret",
				fmt.Sprintf("%s-cert", identityName),
				"-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Cert secret should exist")

			By("verifying certificate content is valid PEM")
			cmd = exec.Command("kubectl", "get", "secret",
				fmt.Sprintf("%s-cert", identityName),
				"-n", testNamespace,
				"-o", "jsonpath={.data.cert\\.pem}")
			certBase64, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(certBase64).NotTo(BeEmpty(), "Certificate should not be empty")

			// Decode and verify PEM format
			cmd = exec.Command("bash", "-c",
				fmt.Sprintf("echo '%s' | base64 -d", certBase64))
			certPEM, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(certPEM).To(ContainSubstring("-----BEGIN CERTIFICATE-----"))
			Expect(certPEM).To(ContainSubstring("-----END CERTIFICATE-----"))
		})

		It("should successfully enroll with dual certificates (sign + TLS)", func() {
			dualIdentityName := "test-identity-dual-e2e"

			By("creating enrollment secret for dual enrollment")
			enrollSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s-enroll-secret
  namespace: %s
type: Opaque
stringData:
  password: adminpw
`, dualIdentityName, testNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(enrollSecretYAML))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating Identity with dual enrollment")
			identityYAML := fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Identity
metadata:
  name: %s
  namespace: %s
spec:
  type: user
  mspID: Org1MSP
  enrollment:
    caRef:
      name: %s
      namespace: %s
    enrollID: admin
    enrollSecretRef:
      name: %s-enroll-secret
      key: password
      namespace: %s
  output:
    secretName: %s-cert
`, dualIdentityName, testNamespace, caName, testNamespace, dualIdentityName, testNamespace, dualIdentityName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(identityYAML))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for dual Identity to be READY")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "identity", dualIdentityName,
					"-n", testNamespace,
					"-o", "jsonpath={.status.status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("READY"))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying cert secret exists")
			certSecretName := fmt.Sprintf("%s-cert", dualIdentityName)
			cmd = exec.Command("kubectl", "get", "secret", certSecretName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Secret %s should exist", certSecretName))

			By("cleaning up dual identity")
			cmd = exec.Command("kubectl", "delete", "identity", dualIdentityName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})

	Context("Idemix Enrollment", func() {
		It("should successfully enroll with idemix credential", func() {
			idemixIdentityName := "test-identity-idemix-e2e"
			idemixCAName := "test-ca-idemix-e2e"

			By("deploying Fabric CA with idemix support")
			caYAML := fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: %s
  namespace: %s
spec:
  image: hyperledger/fabric-ca
  version: "1.5.15"
  ca:
    name: ca
    registry:
      identities:
        - name: admin
          pass: adminpw
          type: client
          affiliation: ""
          attrs:
            hf.Registrar.Roles: "*"
            hf.Revoker: true
  tlsca:
    name: tlsca
    registry:
      identities:
        - name: admin
          pass: adminpw
          type: client
          affiliation: ""
          attrs:
            hf.Registrar.Roles: "*"
            hf.Revoker: true
  idemix:
    curve: gurvy.Bn254
  tls:
    domains:
      - %s
      - %s.%s
      - %s.%s.svc.cluster.local
      - localhost
  service:
    type: ClusterIP
`, idemixCAName, testNamespace, idemixCAName, idemixCAName, testNamespace, idemixCAName, testNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(caYAML))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for idemix CA pod to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "wait", "--for=condition=ready",
					"pod", "-l", "app=ca",
					"-n", testNamespace,
					"--timeout=120s")
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("creating enrollment secret for idemix")
			enrollSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s-enroll-secret
  namespace: %s
type: Opaque
stringData:
  password: adminpw
`, idemixIdentityName, testNamespace)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(enrollSecretYAML))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating Identity with idemix enrollment")
			identityYAML := fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Identity
metadata:
  name: %s
  namespace: %s
spec:
  type: user
  mspID: Org1MSP
  enrollment:
    caRef:
      name: %s
      namespace: %s
    enrollID: admin
    enrollSecretRef:
      name: %s-enroll-secret
      key: password
      namespace: %s
    idemix: {}
  output:
    secretName: %s-cert
`, idemixIdentityName, testNamespace, idemixCAName, testNamespace, idemixIdentityName, testNamespace, idemixIdentityName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(identityYAML))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for idemix Identity to be READY")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "identity", idemixIdentityName,
					"-n", testNamespace,
					"-o", "jsonpath={.status.status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("READY"))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying idemix credential secret was created")
			cmd = exec.Command("kubectl", "get", "secret",
				fmt.Sprintf("%s-idemix-cred", idemixIdentityName),
				"-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying idemix secret contains required fields")
			requiredKeys := []string{"SignerConfig", "Cred", "Sk", "metadata.json"}
			for _, key := range requiredKeys {
				cmd = exec.Command("kubectl", "get", "secret",
					fmt.Sprintf("%s-idemix-cred", idemixIdentityName),
					"-n", testNamespace,
					"-o", fmt.Sprintf("jsonpath={.data.%s}", key))
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(output).NotTo(BeEmpty(), fmt.Sprintf("Idemix secret should contain %s", key))
			}

			By("cleaning up idemix resources")
			cmd = exec.Command("kubectl", "delete", "identity", idemixIdentityName, "-n", testNamespace)
			_, _ = utils.Run(cmd)

			cmd = exec.Command("kubectl", "delete", "ca", idemixCAName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})

	Context("Error Handling", func() {
		It("should fail gracefully with invalid credentials", func() {
			failIdentityName := "test-identity-fail-e2e"

			By("creating enrollment secret with wrong password")
			enrollSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s-enroll-secret
  namespace: %s
type: Opaque
stringData:
  password: wrongpassword
`, failIdentityName, testNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(enrollSecretYAML))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating Identity with invalid credentials")
			identityYAML := fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Identity
metadata:
  name: %s
  namespace: %s
spec:
  type: user
  mspID: Org1MSP
  enrollment:
    caRef:
      name: %s
      namespace: %s
    enrollID: admin
    enrollSecretRef:
      name: %s-enroll-secret
      key: password
      namespace: %s
  output:
    secretName: %s-cert
`, failIdentityName, testNamespace, caName, testNamespace, failIdentityName, testNamespace, failIdentityName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewReader([]byte(identityYAML))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Identity enters FAILED status")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "identity", failIdentityName,
					"-n", testNamespace,
					"-o", "jsonpath={.status.status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("FAILED"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying error message is present in status")
			cmd = exec.Command("kubectl", "get", "identity", failIdentityName,
				"-n", testNamespace,
				"-o", "jsonpath={.status.message}")
			message, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(message).NotTo(BeEmpty(), "Status message should contain error details")

			By("cleaning up failed identity")
			cmd = exec.Command("kubectl", "delete", "identity", failIdentityName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})
})
