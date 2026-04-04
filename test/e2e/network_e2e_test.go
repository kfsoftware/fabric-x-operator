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

var _ = Describe("Full Network E2E", Ordered, func() {
	const (
		caName    = "net-ca"
		channelID = "e2echannel"
		ns        = "default"
	)

	BeforeAll(func() {
		By("deploying a CA for the network")
		applyYAML(caYAML(caName, ns))

		By("waiting for CA to be ready")
		waitForPod("app=ca", ns, 2*time.Minute)

		By("verifying CA crypto secrets exist")
		expectSecretExists(caName+"-tls-crypto", ns)
		expectSecretExists(caName+"-msp-crypto", ns)
		expectSecretExists(caName+"-tlsca-crypto", ns)
	})

	AfterAll(func() {
		By("cleaning up network resources")
		kubectl("delete", "chainnamespace", "--all", "--ignore-not-found=true")
		kubectl("delete", "committer", "--all", "--ignore-not-found=true", "-n", ns)
		kubectl("delete", "genesis", "--all", "--ignore-not-found=true", "-n", ns)
		kubectl("delete", "orderergroup", "--all", "--ignore-not-found=true", "-n", ns)
		kubectl("delete", "identity", "--all", "--ignore-not-found=true", "-n", ns)
		kubectl("delete", "ca", caName, "--ignore-not-found=true", "-n", ns)
		// Clean CA PVC to avoid stale data on next run (but not postgres)
		kubectl("delete", "pvc", caName, "--force", "--ignore-not-found=true", "-n", ns)
	})

	Context("OrdererGroup Lifecycle", Ordered, func() {
		It("should deploy orderer group in configure mode and enroll certificates", func() {
			applyYAML(ordererGroupYAML(1, caName, ns))

			By("waiting for orderer group to be RUNNING")
			waitForCRStatus("orderergroup", "orderergroup-party1", ns, "RUNNING", 2*time.Minute)

			By("waiting for certificate secrets to be created")
			Eventually(func(g Gomega) {
				for _, comp := range []string{"router", "consenter", "assembler", "batcher-0"} {
					cmd := exec.Command("kubectl", "get", "secret",
						fmt.Sprintf("orderergroup-party1-%s-sign-cert", comp), "-n", ns)
					_, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred(),
						fmt.Sprintf("Secret orderergroup-party1-%s-sign-cert should exist", comp))
				}
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("should create a genesis block", func() {
			applyYAML(genesisYAML(caName, channelID, ns))

			waitForCRStatus("genesis", "e2e-genesis", ns, "RUNNING", 2*time.Minute)

			By("verifying genesis secret was created")
			expectSecretExists("e2e-genesis-block", ns)
		})

		It("should deploy orderer components when switched to deploy mode", func() {
			kubectl("patch", "orderergroup", "orderergroup-party1", "-n", ns,
				"--type=merge", "-p", `{"spec":{"bootstrapMode":"deploy"}}`)

			By("waiting for router pod")
			waitForPod("ordererrouter=orderergroup-party1-router", ns, 5*time.Minute)

			By("waiting for consenter pod")
			waitForPod("ordererconsenter=orderergroup-party1-consenter", ns, 5*time.Minute)

			By("waiting for batcher pod")
			waitForPod("ordererbatcher=orderergroup-party1-batcher-0", ns, 5*time.Minute)

			By("waiting for assembler pod")
			waitForPod("ordererassembler=orderergroup-party1-assembler", ns, 5*time.Minute)
		})
	})

	Context("Committer Lifecycle", Ordered, func() {
		It("should deploy committer components", func() {
			By("installing CNPG operator for PostgreSQL")
			cmd := exec.Command("kubectl", "apply", "--server-side", "-f",
				"https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.25/releases/cnpg-1.25.1.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for CNPG controller to be ready")
			waitForPod("app.kubernetes.io/name=cloudnative-pg", "cnpg-system", 2*time.Minute)

			By("deploying PostgreSQL cluster")
			applyYAML(postgresYAML(ns))

			By("waiting for PostgreSQL to be ready")
			waitForPod("cnpg.io/cluster=e2e-postgres", ns, 2*time.Minute)

			By("deploying committer")
			applyYAML(committerYAML(caName, channelID, ns))

			By("waiting for committer to be RUNNING")
			waitForCRStatus("committer", "e2e-committer", ns, "RUNNING", 2*time.Minute)

			By("verifying coordinator pod is running")
			waitForPod("app=coordinator", ns, 2*time.Minute)

			By("verifying sidecar pod is running")
			waitForPod("app=sidecar", ns, 2*time.Minute)

			By("verifying verifier pod is running")
			waitForPod("app=verifier", ns, 2*time.Minute)
		})
	})

	Context("ChainNamespace Lifecycle", Ordered, func() {
		It("should create an admin identity", func() {
			By("creating enrollment secret")
			applyYAML(fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: e2e-admin-enroll
  namespace: %s
type: Opaque
stringData:
  password: adminpw
`, ns))

			By("creating admin identity")
			applyYAML(fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Identity
metadata:
  name: e2e-admin
  namespace: %s
spec:
  type: admin
  mspID: Org1MSP
  enrollment:
    caRef:
      name: %s
      namespace: %s
    enrollID: admin
    enrollSecretRef:
      name: e2e-admin-enroll
      key: password
      namespace: %s
  output:
    secretName: e2e-admin-cert
`, ns, caName, ns, ns))

			waitForCRStatus("identity", "e2e-admin", ns, "READY", 2*time.Minute)
			expectSecretExists("e2e-admin-cert", ns)
		})

		It("should deploy a chain namespace and get a transaction ID", func() {
			applyYAML(fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: ChainNamespace
metadata:
  name: e2e-namespace
spec:
  name: "e2enamespace"
  orderer: "orderergroup-party1-router-service.%s.svc.cluster.local:7150"
  tls:
    enabled: false
  mspID: "Org1MSP"
  identity:
    name: "e2e-admin"
    namespace: "%s"
  channel: "%s"
  version: -1
`, ns, ns, channelID))

			By("waiting for namespace to be Deployed")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "chainnamespace", "e2e-namespace",
					"-o", "jsonpath={.status.status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Deployed"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying transaction ID is set")
			cmd := exec.Command("kubectl", "get", "chainnamespace", "e2e-namespace",
				"-o", "jsonpath={.status.txID}")
			txID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(txID).NotTo(BeEmpty(), "ChainNamespace should have a transaction ID")
			_, _ = fmt.Fprintf(GinkgoWriter, "Namespace deployed with TxID: %s\n", txID)
		})
	})
})

// --- Helper functions ---

func applyYAML(yaml string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader([]byte(yaml))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to apply YAML")
}

func kubectl(args ...string) string {
	cmd := exec.Command("kubectl", args...)
	output, _ := utils.Run(cmd)
	return output
}

func expectSecretExists(name, ns string) {
	cmd := exec.Command("kubectl", "get", "secret", name, "-n", ns)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Secret %s should exist", name))
}

func waitForPod(label, ns string, timeout time.Duration) {
	EventuallyWithOffset(1, func(g Gomega) {
		cmd := exec.Command("kubectl", "wait", "--for=condition=ready",
			"pod", "-l", label, "-n", ns, "--timeout=10s")
		_, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
	}, timeout, 5*time.Second).Should(Succeed())
}

func waitForCRStatus(kind, name, ns, expectedStatus string, timeout time.Duration) {
	EventuallyWithOffset(1, func(g Gomega) {
		cmd := exec.Command("kubectl", "get", kind, name, "-n", ns,
			"-o", "jsonpath={.status.status}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal(expectedStatus),
			fmt.Sprintf("%s/%s status is %q, expected %q", kind, name, output, expectedStatus))
	}, timeout, 5*time.Second).Should(Succeed())
}

// --- YAML generators ---

func caYAML(name, ns string) string {
	return fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  image: hyperledger/fabric-ca
  version: "1.5.15"
  ca:
    name: ca
    registry:
      maxenrollments: -1
      identities:
        - name: admin
          pass: adminpw
          type: client
          affiliation: ""
          attrs:
            hf.Registrar.Roles: "*"
            hf.Registrar.DelegateRoles: "*"
            hf.Registrar.Attributes: "*"
            hf.Revoker: true
            hf.AffiliationMgr: true
            hf.GenCRL: true
            hf.IntermediateCA: true
  tlsca:
    name: tlsca
    registry:
      maxenrollments: -1
      identities:
        - name: admin
          pass: adminpw
          type: client
          affiliation: ""
          attrs:
            hf.Registrar.Roles: "*"
            hf.Registrar.DelegateRoles: "*"
            hf.Registrar.Attributes: "*"
            hf.Revoker: true
            hf.AffiliationMgr: true
            hf.GenCRL: true
            hf.IntermediateCA: true
  tls:
    domains:
      - %[1]s
      - %[1]s.%[2]s
      - %[1]s.%[2]s.svc.cluster.local
      - localhost
  service:
    type: ClusterIP
`, name, ns)
}

func ordererGroupYAML(partyID int, caName, ns string) string {
	name := fmt.Sprintf("orderergroup-party%d", partyID)
	return fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: OrdererGroup
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  partyID: %[3]d
  bootstrapMode: configure
  mspid: Org1MSP
  image: hyperledger/fabric-x-orderer
  imageTag: "0.0.24"
  common:
    replicas: 1
    podLabels:
      app.kubernetes.io/component: fabric-x
  components:
    assembler:
      replicas: 1
    batchers:
      - shardID: 1
        replicas: 1
    consenter:
      consenterID: %[3]d
      endpoints:
        - %[1]s-consenter.%[2]s.svc.cluster.local:7050
      replicas: 1
    router:
      replicas: 1
  enrollment:
    sign:
      ca:
        caname: ca
        cahost: %[4]s.%[2]s
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: %[4]s-tls-crypto
            namespace: %[2]s
        enrollid: admin
        enrollsecret: adminpw
    tls:
      ca:
        caname: tlsca
        cahost: %[4]s.%[2]s
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: %[4]s-tls-crypto
            namespace: %[2]s
        enrollid: admin
        enrollsecret: adminpw
  genesis:
    secretKey: genesis.block
    secretName: e2e-genesis-block
    secretNamespace: %[2]s
`, name, ns, partyID, caName)
}

func genesisYAML(caName, channelID, ns string) string {
	// Build consenter and party entries for 1 party (sufficient for E2E)
	consenters := ""
	parties := ""
	endpoints := ""
	for i := 1; i <= 1; i++ {
		prefix := fmt.Sprintf("orderergroup-party%d", i)
		consenters += fmt.Sprintf(`
    - id: %d
      mspId: Org1MSP
      host: %s-consenter.%s.svc.cluster.local
      port: 7052
      identityRef:
        name: %s-consenter-sign-cert
        namespace: %s
        key: cert.pem
      clientTlsCertRef:
        name: %s-consenter-tls-cert
        namespace: %s
        key: cert.pem
      serverTlsCertRef:
        name: %s-consenter-tls-cert
        namespace: %s
        key: cert.pem
`, i, prefix, ns, prefix, ns, prefix, ns, prefix, ns)

		endpoints += fmt.Sprintf(`
        - "id=%d,broadcast,%s-router-service.%s.svc.cluster.local:7150"
        - "id=%d,deliver,%s-assembler.%s.svc.cluster.local:7050"
`, i, prefix, ns, i, prefix, ns)

		parties += fmt.Sprintf(`
    - partyID: %[1]d
      caCerts:
        - name: %[2]s-msp-crypto
          namespace: %[4]s
          key: certfile
      tlsCaCerts:
        - name: %[2]s-tlsca-crypto
          namespace: %[4]s
          key: certfile
      routerConfig:
        host: %[3]s-router-service.%[4]s.svc.cluster.local
        port: 7150
        tlsCert:
          name: %[3]s-router-tls-cert
          namespace: %[4]s
          key: cert.pem
      batchersConfig:
        - shardID: 1
          host: %[3]s-batcher-0.%[4]s.svc.cluster.local
          port: 7151
          signCert:
            name: %[3]s-batcher-0-sign-cert
            namespace: %[4]s
            key: cert.pem
          tlsCert:
            name: %[3]s-batcher-0-tls-cert
            namespace: %[4]s
            key: cert.pem
      consenterConfig:
        host: %[3]s-consenter.%[4]s.svc.cluster.local
        port: 7052
        signCert:
          name: %[3]s-consenter-sign-cert
          namespace: %[4]s
          key: cert.pem
        tlsCert:
          name: %[3]s-consenter-tls-cert
          namespace: %[4]s
          key: cert.pem
      assemblerConfig:
        host: %[3]s-assembler.%[4]s.svc.cluster.local
        port: 7050
        tlsCert:
          name: %[3]s-assembler-tls-cert
          namespace: %[4]s
          key: cert.pem
`, i, caName, prefix, ns)
	}

	return fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Genesis
metadata:
  name: e2e-genesis
  namespace: %[1]s
spec:
  channelID: %[2]s
  configTemplate:
    configMapName: e2e-configtx
    key: configtx.yaml
  ordererOrganizations:
    - name: Org1MSP
      mspId: Org1MSP
      signCaCertRef:
        name: %[3]s-msp-crypto
        namespace: %[1]s
        key: certfile
      tlsCaCertRef:
        name: %[3]s-tlsca-crypto
        namespace: %[1]s
        key: certfile
      endpoints: %[4]s
      router:
        host: orderergroup-party1-router-service.%[1]s.svc.cluster.local
        port: 7150
        partyID: 1
        signCertRef:
          name: orderergroup-party1-router-sign-cert
          namespace: %[1]s
          key: cert.pem
        tlsCertRef:
          name: orderergroup-party1-router-tls-cert
          namespace: %[1]s
          key: cert.pem
      assembler:
        host: orderergroup-party1-assembler.%[1]s.svc.cluster.local
        port: 7050
        tlsCertRef:
          name: orderergroup-party1-assembler-tls-cert
          namespace: %[1]s
          key: cert.pem
  applicationOrgs:
    - name: Org1MSP
      mspId: Org1MSP
      signCaCertRef:
        name: %[3]s-msp-crypto
        namespace: %[1]s
        key: certfile
      tlsCaCertRef:
        name: %[3]s-tlsca-crypto
        namespace: %[1]s
        key: certfile
  consenters: %[5]s
  parties: %[6]s
  output:
    secretName: e2e-genesis-block
    blockKey: genesis.block
`, ns, channelID, caName, endpoints, consenters, parties)
}

func postgresYAML(ns string) string {
	return fmt.Sprintf(`
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: e2e-postgres
  namespace: %s
spec:
  instances: 1
  bootstrap:
    initdb:
      database: fabricx
      owner: fabricx
  storage:
    size: 1Gi
  resources:
    requests:
      memory: "256Mi"
      cpu: "250m"
    limits:
      memory: "512Mi"
      cpu: "500m"
`, ns)
}

func committerYAML(caName, channelID, ns string) string {
	return fmt.Sprintf(`
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Committer
metadata:
  name: e2e-committer
  namespace: %[1]s
spec:
  bootstrapMode: deploy
  mspid: Org1MSP
  image: hyperledger/fabric-x-committer
  imageTag: "0.1.9"
  common:
    replicas: 1
  genesis:
    secretName: e2e-genesis-block
    secretKey: genesis.block
    secretNamespace: %[1]s
  components:
    ordererEndpoints:
      - orderergroup-party1-assembler.%[1]s.svc.cluster.local:7050
    committerHost: e2e-committer-coordinator-service.%[1]s.svc.cluster.local
    committerPort: 9001
    coordinatorVerifierEndpoints:
      - e2e-committer-verifier-service.%[1]s.svc.cluster.local:5001
    coordinatorValidatorCommitterEndpoints:
      - e2e-committer-validator-service.%[1]s.svc.cluster.local:6001
    coordinator:
      replicas: 1
    sidecar:
      replicas: 1
      env:
        - name: SC_SIDECAR_ORDERER_CHANNEL_ID
          value: %[2]s
    validator:
      replicas: 1
      postgresql:
        host: e2e-postgres-rw.%[1]s.svc.cluster.local
        port: 5432
        database: fabricx
        username: fabricx
        passwordSecret:
          name: e2e-postgres-app
          namespace: %[1]s
          key: password
    verifierService:
      replicas: 1
    queryService:
      replicas: 1
      command:
        - committer
      args:
        - start-query
        - --config=/config/config.yaml
      postgresql:
        host: e2e-postgres-rw.%[1]s.svc.cluster.local
        port: 5432
        database: fabricx
        username: fabricx
        passwordSecret:
          name: e2e-postgres-app
          namespace: %[1]s
          key: password
  enrollment:
    sign:
      ca:
        caname: ca
        cahost: %[3]s.%[1]s
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: %[3]s-tls-crypto
            namespace: %[1]s
        enrollid: admin
        enrollsecret: adminpw
    tls:
      ca:
        caname: tlsca
        cahost: %[3]s.%[1]s
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: %[3]s-tls-crypto
            namespace: %[1]s
        enrollid: admin
        enrollsecret: adminpw
`, ns, channelID, caName)
}
