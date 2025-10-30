# Quick Start Guide

This guide will walk you through setting up a complete Hyperledger Fabric X network using the Fabric X Operator on Kubernetes.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Step 1: Set Up Kubernetes Cluster](#step-1-set-up-kubernetes-cluster)
- [Step 2: Install the Kubernetes Operator](#step-2-install-the-kubernetes-operator)
- [Step 3: Install Istio Gateway and Configure DNS](#step-3-install-istio-gateway-and-configure-dns)
- [Step 4: Configure Environment Variables](#step-4-configure-environment-variables)
- [Step 5: Deploy Certificate Authority](#step-5-deploy-certificate-authority)
- [Step 6: Create Admin Identity](#step-6-create-admin-identity)
- [Step 7: Deploy Orderer Groups](#step-7-deploy-orderer-groups)
- [Step 8: Create Genesis Block](#step-8-create-genesis-block)
- [Step 9: Patch Orderer Groups to Deploy Mode](#step-9-patch-orderer-groups-to-deploy-mode)
- [Step 10: Deploy Committer](#step-10-deploy-committer)
- [Step 11: Deploy Endorsers](#step-11-deploy-endorsers)
- [Verification](#verification)
- [Next Steps](#next-steps)
- [Troubleshooting](#troubleshooting)

## Prerequisites

Before starting, ensure you have the following tools installed:

- **Docker** version 17.03+
- **kubectl** version 1.11.3+
- **Helm** 3.x - [Installation Guide](https://helm.sh/docs/intro/install/)
- **Go** version 1.24.0+ (for development)
- Access to a Kubernetes cluster (v1.11.3+)

## Step 1: Set Up Kubernetes Cluster

You can use either K3D or KinD to create a local Kubernetes cluster for testing.

### Option A: Using K3D

K3D is a lightweight wrapper to run K3s (Rancher Lab's minimal Kubernetes distribution) in Docker.

```bash
k3d cluster create -p "80:30949@agent:0" -p "443:30950@agent:0" --agents 2 k8s-hlf
```

This command creates a K3D cluster named `k8s-hlf` with:

- Port forwarding from host port 80 to node port 30949
- Port forwarding from host port 443 to node port 30950
- 2 agent nodes

### Option B: Using KinD

KinD (Kubernetes in Docker) runs Kubernetes clusters using Docker container nodes.

First, create a configuration file:

```bash
cat << EOF > kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.33.4
  extraPortMappings:
  - containerPort: 30949
    hostPort: 80
  - containerPort: 30950
    hostPort: 443
EOF
```

Then create the cluster:

```bash
kind create cluster --config=./kind-config.yaml
```

## Step 2: Install the Fabric X Operator

The Fabric X Operator manages the deployment and lifecycle of Hyperledger Fabric components in Kubernetes.

### Build and Deploy the Operator

```bash
# Generate CRDs and other manifests
make generate
make manifests

# Build operator image with timestamp tag
export IMAGE=local/fabric-x-operator:$(date +%Y%m%d%H%M%S)
make docker-build IMG=$IMAGE

# Import image into K3D cluster
k3d image import $IMAGE --cluster k8s-hlf

# Deploy the operator
make deploy IMG=$IMAGE
```

This installs:

- Custom Resource Definitions (CRDs) for Fabric Peers, Orderers, and Certificate Authorities
- The operator controller to manage these resources

### Verify Installation

```bash
kubectl get pods
```

You should see the operator pod running.

## Step 3: Install Istio Gateway and Configure DNS

The Fabric X Operator uses Istio for external ingress to orderer components. This enables gRPC routing with HTTP/2 support (h2c) for non-TLS communication on port 80, or TLS passthrough routing with SNI-based hostname matching on port 443.

### Install Istio with Gateway API Support

```bash
# Download Istio 1.26.3
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.26.3 sh -
export PATH="$PATH:$(pwd)/istio-1.26.3/bin"

# Install Istio with minimal profile
istioctl install --set profile=minimal -y
```

### Configure Istio Gateway Service with NodePorts

Update the Istio gateway service to use specific NodePorts that match your K3D cluster port mappings:

```bash
kubectl patch svc istio-ingressgateway -n istio-system --type=json -p='[
  {"op": "replace", "path": "/spec/type", "value": "NodePort"},
  {"op": "add", "path": "/spec/ports/-", "value": {"name": "http2", "port": 80, "protocol": "TCP", "targetPort": 80, "nodePort": 30949}},
  {"op": "add", "path": "/spec/ports/-", "value": {"name": "https", "port": 443, "protocol": "TCP", "targetPort": 443, "nodePort": 30950}}
]'
```

### Install Gateway API CRDs

```bash
# Install standard Gateway API CRDs
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/standard-install.yaml

# Install experimental Gateway API CRDs (for TLSRoute)
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/experimental-install.yaml
```

### Create Gateway Resource

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: istio-ingressgateway
  namespace: istio-system
  annotations:
    networking.istio.io/service-type: NodePort
spec:
  gatewayClassName: istio
  listeners:
  - name: http
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
  - name: https-passthrough
    port: 443
    protocol: TLS
    tls:
      mode: Passthrough
    allowedRoutes:
      namespaces:
        from: All
EOF
```

**Note**: The Gateway has two listeners:
- **Port 80 (HTTP)**: For gRPC with HTTP/2 (h2c) without TLS
- **Port 443 (TLS)**: For TLS passthrough routing with SNI

### Configure CoreDNS for In-Cluster DNS Resolution

This configuration allows pods inside the Kubernetes cluster to resolve `*.localho.st` domains to the Istio gateway, enabling the same domain names to work both externally and internally.

```bash
kubectl apply -f - <<EOF
kind: ConfigMap
apiVersion: v1
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health {
           lameduck 5s
        }
        rewrite name regex (.*)\.localho\.st istio-ingressgateway.istio-system.svc.cluster.local
        hosts {
          fallthrough
        }
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
EOF

# Restart CoreDNS to apply the configuration
kubectl rollout restart deployment coredns -n kube-system
kubectl rollout status deployment coredns -n kube-system --timeout=60s
```

**How it works:**
- External clients: `orderergroup-party1-router.localho.st:80` resolves to `127.0.0.1` (via public DNS), routes through k3d port mapping (80→30949) to Istio gateway
- Internal pods: `orderergroup-party1-router.localho.st` is rewritten by CoreDNS to `istio-ingressgateway.istio-system.svc.cluster.local`, routes directly to Istio gateway service

### Verify Installation

```bash
# Check Istio installation
kubectl get pods -n istio-system

# Check Gateway resource
kubectl get gateway -n istio-system

# Verify Gateway is programmed
kubectl get gateway istio-ingressgateway -n istio-system -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}'
# Should output: True
```

### Understanding Istio Configuration for Orderer Groups

The Fabric X Operator supports native Istio configuration for orderer components (routers and assemblers). This provides direct control over Istio VirtualService and DestinationRule resources, enabling proper gRPC/HTTP2 support.

#### Istio Native vs Gateway API

You can configure ingress in two ways:

**Option 1: Istio Native Configuration (Recommended for gRPC)**
```yaml
spec:
  ingress:
    istio:
      hosts:
        - "orderergroup-party1-router.localho.st"
      gateway: "istio-system/istio-ingressgateway"
      enableHTTP2: true  # Enable HTTP/2 (h2c) for gRPC
```

Creates native Istio resources:
- **VirtualService**: Routes traffic based on hostname
- **DestinationRule**: Configures HTTP/2 support for backend connections

**Option 2: Gateway API Configuration**
```yaml
spec:
  ingress:
    gateway:
      hosts:
        - "orderergroup-party1-router.localho.st"
      ingressGateway: "istio-system/istio-ingressgateway"
      port: 7150
      tls:
        enabled: false  # Use HTTPRoute for port 80
```

Creates Gateway API resources:
- **HTTPRoute** (when TLS disabled): HTTP routing on port 80
- **TLSRoute** (when TLS enabled): TLS passthrough on port 443

#### When to Use Istio Native Configuration

Use Istio native configuration when:
- You need gRPC/HTTP2 support without TLS (h2c)
- You want direct control over DestinationRule traffic policies
- You're using Istio-specific features

#### Orderer Group Sample Updates

All orderer group samples have been updated to use Istio native configuration with HTTP/2 enabled:

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: OrdererGroup
spec:
  router:
    ingress:
      istio:
        hosts:
          - "orderergroup-party1-router.localho.st"
        gateway: "istio-system/istio-ingressgateway"
        enableHTTP2: true
  assembler:
    ingress:
      istio:
        hosts:
          - "orderergroup-party1-assembler.localho.st"
        gateway: "istio-system/istio-ingressgateway"
        enableHTTP2: true
```

**Note**: All orderer endpoints use **port 80** (not 443) for gRPC communication with HTTP/2 support.

## Step 4: Configure Environment Variables

Set the container image versions for Fabric components:

```bash
export PEER_IMAGE=hyperledger/fabric-peer
export PEER_VERSION=3.1.0

export ORDERER_IMAGE=hyperledger/fabric-orderer
export ORDERER_VERSION=3.1.0

export CA_IMAGE=hyperledger/fabric-ca
export CA_VERSION=1.5.15
```

These environment variables can be referenced in your deployment scripts and configurations.

## Step 5: Deploy Certificate Authority

The Certificate Authority (CA) is responsible for issuing certificates to all network participants.

### Deploy the CA

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_ca.yaml
```

This deploys a Fabric CA with:

- Two CA instances: one for signing certificates (`ca`) and one for TLS certificates (`tlsca`)
- Ingress configuration for external access via Istio
- Default admin user with credentials: `admin`/`adminpw`

### Verify CA Deployment

```bash
kubectl get fabricca
kubectl get pods | grep ca
```

Wait for the CA pod to be in `Running` status:

```bash
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=fabric-ca --timeout=300s
```

### Check CA Endpoints

The CA will be accessible at:

- Internal endpoint: `test-ca2.default:7054`

## Step 6: Create Admin Identity

The Identity CRD allows you to enroll identities with the Fabric CA. **This step is required** as the admin identity's public key will be used for meta namespace verification in the genesis block.

### Create Admin Password Secret

First, create a secret containing the admin enrollment password:

```bash
kubectl create secret generic admin-enroll-secret --from-literal=password=adminpw
```

This creates a secret with the admin password (`adminpw`) that will be used for enrollment.

### Deploy Admin Identity

Create an admin identity by enrolling with the CA:

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_identity_admin.yaml
```

This Identity resource will:

- Enroll with the Fabric CA using the admin credentials
- Generate both signing and TLS certificates
- Create 6 output secrets containing the identity materials:
  - `org1-admin-sign-cert`: Signing certificate (used for meta namespace verification)
  - `org1-admin-sign-key`: Signing private key
  - `org1-admin-sign-cacert`: Signing CA certificate
  - `org1-admin-tls-cert`: TLS certificate
  - `org1-admin-tls-key`: TLS private key
  - `org1-admin-tls-cacert`: TLS CA certificate

### Verify Identity Creation

```bash
# Check identity status
kubectl get identity org1-admin

# Expected output:
# NAME         TYPE    MSPID     STATUS   EXPIRY         AGE
# org1-admin   admin   Org1MSP   READY    2026-10-19...  1m
```

### Verify Generated Secrets

```bash
# List all secrets created for the identity
kubectl get secrets | grep org1-admin

# Expected output shows 6 secrets:
# org1-admin-sign-cacert
# org1-admin-sign-cert
# org1-admin-sign-key
# org1-admin-tls-cacert
# org1-admin-tls-cert
# org1-admin-tls-key
```

**Important**: The `org1-admin-sign-cert` secret will be referenced in the genesis block for meta namespace verification.

### Validate Certificate

You can validate the enrolled certificate:

```bash
# View certificate details
kubectl get secret admin-org1-sign-cert -o jsonpath='{.data.cert\.pem}' | base64 -d | openssl x509 -text -noout | grep -E "(Subject:|Issuer:|Not After)"
```

Expected output:

```
        Issuer: C=US, L=Raleigh, O=Hyperledger, OU=Fabric, CN=ca
            Not After : Oct 19 20:07:00 2026 GMT
        Subject: OU=client, CN=admin
```

### Using Identity Secrets in Applications

The generated secrets can be mounted in application pods to authenticate with the Fabric network:

```yaml
volumes:
  - name: admin-msp
    projected:
      sources:
        - secret:
            name: admin-org1-sign-cert
            items:
              - key: cert.pem
                path: signcerts/cert.pem
        - secret:
            name: admin-org1-sign-key
            items:
              - key: cert.pem
                path: keystore/key.pem
        - secret:
            name: admin-org1-sign-cacert
            items:
              - key: cert.pem
                path: cacerts/ca.pem
```

### Understanding the Identity Spec

The Identity CRD supports the following configuration:

- **type**: Type of identity (`client`, `peer`, `orderer`, `admin`, `user`)
- **mspID**: MSP ID for the organization (e.g., `Org1MSP`)
- **enrollment**: Configuration for enrolling with Fabric CA
  - **caRef**: Reference to the CA resource
  - **enrollID**: Enrollment ID (username)
  - **enrollSecretRef**: Reference to secret containing enrollment password
  - **attrs**: Optional attributes to include in the certificate
  - **enrollTLS**: Whether to also enroll for TLS certificates (default: true)
- **output**: Configuration for output secrets
  - **secretPrefix**: Prefix for generated secret names
  - **namespace**: Optional namespace for secrets (defaults to identity namespace)
  - **labels**: Optional labels to apply to generated secrets

## Step 7: Deploy Orderer Groups

Orderer groups manage the ordering service for the Fabric network. We'll deploy 4 orderer parties to create a Byzantine Fault Tolerant (BFT) consensus network.

### Deploy Orderer Group - Party 1

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_orderergroup_party1.yaml
```

This deploys:

- **Router**: Routes transactions to appropriate batchers
- **Batchers**: Two shards for parallel transaction batching
- **Consenter**: Participates in consensus protocol
- **Assembler**: Assembles transactions into blocks

### Deploy Orderer Group - Party 2

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_orderergroup_party2.yaml
```

### Deploy Orderer Group - Party 3

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_orderergroup_party3.yaml
```

### Deploy Orderer Group - Party 4

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_orderergroup_party4.yaml
```

### Verify Orderer Group Deployments

```bash
kubectl get orderergroups
```

You should see 4 orderer groups in `Running` state:

```
NAME                    STATUS    AGE
orderergroup-party1     Running   2m
orderergroup-party2     Running   2m
orderergroup-party3     Running   2m
orderergroup-party4     Running   2m
```

Check the pods:

```bash
kubectl get pods | grep orderergroup
```

Wait for all orderer pods to be ready:

```bash
kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=fabric-x --timeout=600s
```

### Understanding Orderer Group Components

Each orderer group deploys several components:

1. **Router** (`router-org[1-4].localho.st`): Entry point for client transactions
2. **Batchers** (`batcher-[1-2]-org[1-4].localho.st`): Batch transactions for efficient processing
3. **Consenter**: Consensus participant (SmartBFT)
4. **Assembler**: Assembles ordered transactions into blocks

## Step 8: Create Genesis Block

The genesis block contains the initial configuration for the channel and must be created before the orderers can fully function.

### Deploy Genesis Configuration

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_genesis.yaml
```

This creates:

- The genesis block configuration for channel `arma`
- Configuration for 4 consenters (SmartBFT consensus)
- Configuration for 4 parties with their batchers, routers, and assemblers
- Application organization configuration (Org1MSP)

### Verify Genesis Block Creation

```bash
kubectl get genesis
```

Check that the genesis block secret was created:

```bash
kubectl get secret fabricx-shared-genesis
```

### View Genesis Configuration Details

```bash
kubectl describe genesis shared-genesis
```

## Step 9: Patch Orderer Groups to Deploy Mode

After the genesis block is created, you need to patch each orderer group to change from `configure` mode to `deploy` mode. This triggers the actual deployment of the orderer components.

### Understanding Bootstrap Modes

The orderer groups support two bootstrap modes:

- **configure**: Initial mode where orderers are configured but not fully deployed. This allows the genesis block to be created with all party configurations.
- **deploy**: Active mode where orderer components are fully deployed and operational.

### Patch Orderer Group - Party 1

```bash
kubectl patch orderergroup orderergroup-party1 --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
```

### Patch Orderer Group - Party 2

```bash
kubectl patch orderergroup orderergroup-party2 --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
```

### Patch Orderer Group - Party 3

```bash
kubectl patch orderergroup orderergroup-party3 --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
```

### Patch Orderer Group - Party 4

```bash
kubectl patch orderergroup orderergroup-party4 --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
```

### Patch All Orderer Groups at Once

Alternatively, you can patch all orderer groups in a single command:

```bash
for i in 1 2 3 4; do
  kubectl patch orderergroup orderergroup-party${i} --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
done
```

### Verify Orderer Groups are in Deploy Mode

```bash
kubectl get orderergroups -o custom-columns=NAME:.metadata.name,MODE:.spec.bootstrapMode,STATUS:.status.phase
```

Expected output:

```
NAME                    MODE      STATUS
orderergroup-party1     deploy    Running
orderergroup-party2     deploy    Running
orderergroup-party3     deploy    Running
orderergroup-party4     deploy    Running
```

### Wait for Components to be Ready

After patching, wait for all orderer components to be fully deployed and ready:

```bash
kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=fabric-x --timeout=600s
```

You can also watch the pods as they start:

```bash
kubectl get pods -w
```

Press `Ctrl+C` to stop watching once all pods are running.

### Verify Component Deployment

Check that all components are deployed for each party:

```bash
# Check routers
kubectl get pods -l app.kubernetes.io/component=router

# Check batchers (should see 8 total - 2 per party)
kubectl get pods -l app.kubernetes.io/component=batcher

# Check consenters (should see 4 total - 1 per party)
kubectl get pods -l app.kubernetes.io/component=consenter

# Check assemblers (should see 4 total - 1 per party)
kubectl get pods -l app.kubernetes.io/component=assembler
```

## Step 10: Deploy Committer

After the orderer groups are fully deployed and operational, you can deploy the Committer component. The Committer is responsible for validating, verifying, and committing transactions to the ledger.

### Understanding the Committer Architecture

The Committer consists of several components working together:

- **Coordinator**: Orchestrates the commit process and manages the workflow
- **Sidecar**: Interfaces with the ordering service to receive blocks
- **Validator**: Validates transactions against business rules and policies
- **Verifier**: Verifies transaction signatures and endorsements

### Prerequisites for Committer Deployment

Before deploying the Committer, you need to set up a PostgreSQL database for the Validator component. The Validator uses PostgreSQL to store transaction validation state.

#### Deploy PostgreSQL Using CloudNativePG Operator

CloudNativePG (CNPG) is a Kubernetes operator that manages the full lifecycle of PostgreSQL clusters with high availability, automated backups, and rolling updates.

**Step 1: Install CloudNativePG Operator**

```bash
# Install CloudNativePG operator (latest version)
kubectl apply --server-side -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.27/releases/cnpg-1.27.0.yaml

# Wait for the operator to be ready
kubectl wait --for=condition=Available deployment/cnpg-controller-manager \
  -n cnpg-system --timeout=120s

# Verify operator installation
kubectl get deployment -n cnpg-system
```

**Step 2: Deploy PostgreSQL Cluster**

```bash
# Deploy PostgreSQL cluster using CNPG
kubectl apply -f config/samples/cnpg_postgres_cluster.yaml

# Wait for PostgreSQL cluster to be ready
kubectl wait --for=condition=Ready cluster/fabric-x-postgres --timeout=300s

# Verify cluster is running
kubectl get cluster fabric-x-postgres
kubectl get pods -l cnpg.io/cluster=fabric-x-postgres
```

**What CNPG Creates Automatically:**
- PostgreSQL cluster with 1 instance (configurable)
- Database: `fabricx`
- Owner/Username: `fabricx`
- Secret: `fabric-x-postgres-app` (contains auto-generated credentials)
- Service: `fabric-x-postgres-rw` (read-write connections on port 5432)
- Service: `fabric-x-postgres-ro` (read-only connections on port 5432)
- Service: `fabric-x-postgres-r` (read connections, includes primary)

**Step 3: Get Database Credentials**

```bash
# The password is automatically generated and stored in a secret
kubectl get secret fabric-x-postgres-app -o jsonpath='{.data.password}' | base64 -d
echo

# View all connection details
kubectl get secret fabric-x-postgres-app -o yaml

# Connection details:
# Host: fabric-x-postgres-rw.default.svc.cluster.local
# Port: 5432
# Database: fabricx
# Username: fabricx
# Password: (from secret above)
```

### Update Committer Configuration

Before deploying, update the committer configuration to use the CNPG PostgreSQL credentials.

The committer YAML should reference the CNPG-managed PostgreSQL cluster:

```yaml
spec:
  components:
    validator:
      postgresql:
        host: fabric-x-postgres-rw.default.svc.cluster.local
        port: 5432
        database: fabricx
        username: fabricx
        passwordSecret:
          name: fabric-x-postgres-app
          key: password
          namespace: default
    queryService:
      postgresql:
        host: fabric-x-postgres-rw.default.svc.cluster.local
        port: 5432
        database: fabricx
        username: fabricx
        passwordSecret:
          name: fabric-x-postgres-app
          key: password
          namespace: default
```

**Note:** The sample `config/samples/fabricx_v1alpha1_committer.yaml` is already configured to use these CNPG defaults. If your CNPG cluster has a different name or namespace, update accordingly.

### Deploy the Committer

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_committer.yaml
```

This creates a Committer resource that deploys:

- **Coordinator** (1 replica): Manages the commit workflow
- **Sidecar** (1 replica): Receives blocks from orderers
- **Validator** (1 replica): Validates transactions
- **Verifier** (1 replica): Verifies signatures and endorsements

### Verify Committer Deployment

Check that the Committer resource was created:

```bash
kubectl get committer
```

Expected output:

```
NAME                   STATUS    AGE
fabric-x-committer     Running   30s
```

### Wait for Committer Components to be Ready

```bash
kubectl wait --for=condition=ready pod -l app.kubernetes.io/part-of=committer --timeout=600s
```

### Verify All Committer Components

```bash
# Check coordinator
kubectl get pods -l app.kubernetes.io/component=coordinator

# Check sidecar
kubectl get pods -l app.kubernetes.io/component=sidecar

# Check validator
kubectl get pods -l app.kubernetes.io/component=validator

# Check verifier
kubectl get pods -l app.kubernetes.io/component=verifier
```

You should see one pod for each component in `Running` status.

### View Committer Component Logs

```bash
# Coordinator logs
kubectl logs -l app.kubernetes.io/component=coordinator

# Sidecar logs
kubectl logs -l app.kubernetes.io/component=sidecar

# Validator logs
kubectl logs -l app.kubernetes.io/component=validator

# Verifier logs
kubectl logs -l app.kubernetes.io/component=verifier
```

### Verify Committer Connectivity to Orderers

Check the sidecar logs to ensure it's connecting to the orderers:

```bash
kubectl logs -l app.kubernetes.io/component=sidecar | grep -i "orderer\|connected"
```

You should see log entries indicating successful connections to the orderer endpoints.

### Test Committer Services

The Committer components are exposed via Kubernetes services. Test connectivity from within the cluster:

```bash
kubectl run test-committer --image=curlimages/curl:latest --rm -it -- sh
# Inside the pod:
curl http://fabric-x-committer-coordinator-service:9001/health
curl http://fabric-x-committer-sidecar-service:5050/health
```

### Understanding Committer Configuration

Key configuration parameters in the Committer spec:

#### MSP Configuration

```yaml
mspid: CommitterMSP
```

The MSP ID for the Committer organization.

#### Genesis Block Reference

```yaml
genesis:
  secretName: fabricx-shared-genesis
  secretKey: genesis.block
  secretNamespace: default
```

References the genesis block created in Step 9.

#### Orderer Endpoints

```yaml
ordererEndpoints:
  - orderergroup-party1-consenter:7052
  - orderergroup-party2-consenter:7052
  - orderergroup-party3-consenter:7052
  - orderergroup-party4-consenter:7052
```

The Committer connects to all 4 consenters for block delivery.

#### Component Resources

Each component has resource limits:

- **Coordinator**: 200m-1000m CPU, 256Mi-1Gi memory
- **Sidecar**: 300m-1500m CPU, 512Mi-2Gi memory
- **Validator**: 200m-1000m CPU, 256Mi-1Gi memory
- **Verifier**: 200m-1000m CPU, 256Mi-1Gi memory

Adjust these based on your workload requirements.

### Committer Configuration for Different Channels

The sidecar can be configured to work with specific channels:

```yaml
env:
  - name: SIDECAR_CHANNEL_ID
    value: arma
```

Update this to match your channel name (default is `arma` from the genesis block).

## Verification

### Check All Resources

```bash
# Check all Fabric X resources
kubectl get fabricca,orderergroups,genesis,committer

# Check all pods
kubectl get pods -o wide

# Check services
kubectl get svc

```

### Check Orderer Logs

View logs for each orderer component:

```bash
# Router logs
kubectl logs -l app.kubernetes.io/component=router

# Batcher logs
kubectl logs -l app.kubernetes.io/component=batcher

# Consenter logs
kubectl logs -l app.kubernetes.io/component=consenter

# Assembler logs
kubectl logs -l app.kubernetes.io/component=assembler
```

### Test CA Connectivity

From within the cluster:

```bash
kubectl run test-pod --image=curlimages/curl:latest --rm -it -- sh
# Inside the pod:
curl -k https://test-ca2.default:7054/cainfo?ca=ca
```

### Check Consensus Status

Verify that the consenters are communicating:

```bash
kubectl logs -l app.kubernetes.io/component=consenter | grep -i "consensus"
```

## Step 11: Deploy Endorsers

Endorsers are Fabric Smart Client (FSC) nodes that participate in token transactions and endorsements. In this step, we'll deploy 5 endorser nodes with different roles: issuer, two owners, and two endorsers.

### Understanding Endorser Roles

The token network consists of different participant types:

- **Issuer** (`org1-issuer`): Issues tokens to participants
- **Owners** (`org1-owner1`, `org1-owner2`): Token holders who can transfer tokens
- **Endorsers** (`org1-endorser1`, `org1-endorser2`): Transaction endorsers who validate and sign transactions

### Understanding Bootstrap Modes for Endorsers

Endorsers support two bootstrap modes, similar to orderer groups:

- **configure**: Only enrolls certificates and creates secrets (no deployment)
- **deploy**: Full deployment with application pods

This two-phase approach allows you to:

1. First, create all certificates (configure mode)
2. Then, cross-reference certificates between nodes
3. Finally, deploy all applications (deploy mode)

### Phase 1: Deploy Endorsers in Configure Mode

Deploy all 5 endorsers in configure mode to create their certificates:

```bash
# Deploy issuer
kubectl apply -f config/samples/fabricx_v1alpha1_endorser_org1-issuer.yaml

# Deploy owner1
kubectl apply -f config/samples/fabricx_v1alpha1_endorser_org1-owner1.yaml

# Deploy owner2
kubectl apply -f config/samples/fabricx_v1alpha1_endorser_org1-owner2.yaml

# Deploy endorser1
kubectl apply -f config/samples/fabricx_v1alpha1_endorser_org1-endorser1.yaml

# Deploy endorser2
kubectl apply -f config/samples/fabricx_v1alpha1_endorser_org1-endorser2.yaml
```

Or deploy all at once:

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_endorser_org1-issuer.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-owner1.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-owner2.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-endorser1.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-endorser2.yaml
```

### Verify Certificate Creation

Check that the endorser resources were created:

```bash
kubectl get endorsers
```

Expected output:

```
NAME              MODE        STATUS      AGE
org1-issuer       configure   Running     30s
org1-owner1       configure   Running     30s
org1-owner2       configure   Running     30s
org1-endorser1    configure   Running     30s
org1-endorser2    configure   Running     30s
```

Verify that certificate secrets were created:

```bash
kubectl get secrets | grep -E "org1-(issuer|owner1|owner2|endorser1|endorser2)-(sign|tls)-cert"
```

Expected output:

```
org1-issuer-sign-cert      Opaque   3      1m
org1-issuer-tls-cert       Opaque   3      1m
org1-owner1-sign-cert      Opaque   3      1m
org1-owner1-tls-cert       Opaque   3      1m
org1-owner2-sign-cert      Opaque   3      1m
org1-owner2-tls-cert       Opaque   3      1m
org1-endorser1-sign-cert   Opaque   3      1m
org1-endorser1-tls-cert    Opaque   3      1m
org1-endorser2-sign-cert   Opaque   3      1m
org1-endorser2-tls-cert    Opaque   3      1m
```

Each secret contains:

- `cert.pem`: The certificate
- `key.pem`: The private key
- `ca.pem`: The CA certificate

### Phase 2: Wait for Configure Mode to Complete

Each endorser needs to communicate with other endorsers using TLS certificates. The controller automatically creates resolver secrets by copying certificates from the referenced TLS cert secrets.

**How it works:**

- Each endorser's `secretRef` points to a remote endorser's TLS certificate (e.g., `org1-auditor-tls-cert`)
- The controller automatically creates a resolver secret (e.g., `org1-issuer-resolver-auditor`) containing a copy of that certificate
- The resolver secrets are named: `{endorser-name}-resolver-{remote-node-name}`

Wait for all endorsers to complete certificate enrollment in configure mode:

```bash
# Wait for all endorsers to be in RUNNING state
for node in issuer owner1 owner2 endorser1 endorser2; do
  kubectl wait --for=jsonpath='{.status.status}'=Running endorser/org1-${node} --timeout=300s
done
```

### Verify Resolver Secrets

The controller automatically creates resolver secrets once the referenced TLS certificate secrets exist. Check that all resolver secrets were created:

```bash
kubectl get secrets | grep resolver | wc -l
```

Expected output: `20` (5 endorsers × 4 resolvers each)

Check secrets for a specific endorser:

```bash
kubectl get secrets | grep "org1-issuer-resolver"
```

Expected output:

```
org1-issuer-resolver-endorser1     Opaque   1      1m
org1-issuer-resolver-endorser2     Opaque   1      1m
org1-issuer-resolver-owner1        Opaque   1      1m
org1-issuer-resolver-owner2        Opaque   1      1m
```

### Phase 3: Patch Endorsers to Deploy Mode

Now that all certificates and resolver secrets have been created, patch the endorsers to deploy mode:

```bash
kubectl patch endorser org1-issuer --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
kubectl patch endorser org1-owner1 --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
kubectl patch endorser org1-owner2 --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
kubectl patch endorser org1-endorser1 --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
kubectl patch endorser org1-endorser2 --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
```

Or patch all at once:

```bash
for node in issuer owner1 owner2 endorser1 endorser2; do
  kubectl patch endorser org1-${node} --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
done
```

### Verify Endorser Deployment

Check that all endorsers are in deploy mode:

```bash
kubectl get endorsers -o custom-columns=NAME:.metadata.name,MODE:.spec.bootstrapMode,STATUS:.status.status
```

Expected output:

```
NAME              MODE      STATUS
org1-issuer       deploy    Running
org1-owner1       deploy    Running
org1-owner2       deploy    Running
org1-endorser1    deploy    Running
org1-endorser2    deploy    Running
```

### Wait for Endorser Pods to be Ready

```bash
kubectl wait --for=condition=ready pod -l app.kubernetes.io/part-of=fabric-x --timeout=600s
```

Check all endorser pods:

```bash
kubectl get pods -l app.kubernetes.io/component=endorser
```

Expected output (5 pods):

```
NAME                             READY   STATUS    RESTARTS   AGE
org1-issuer-xxxxx               1/1     Running   0          2m
org1-owner1-xxxxx               1/1     Running   0          2m
org1-owner2-xxxxx               1/1     Running   0          2m
org1-endorser1-xxxxx            1/1     Running   0          2m
org1-endorser2-xxxxx            1/1     Running   0          2m
```

### Verify Endorser Services

Check that services were created:

```bash
kubectl get svc | grep org1
```

Expected output:

```
org1-issuer-service      ClusterIP   10.x.x.x   <none>   9101/TCP   3m
org1-owner1-service      ClusterIP   10.x.x.x   <none>   9501/TCP   3m
org1-owner2-service      ClusterIP   10.x.x.x   <none>   9601/TCP   3m
org1-endorser1-service   ClusterIP   10.x.x.x   <none>   9301/TCP   3m
org1-endorser2-service   ClusterIP   10.x.x.x   <none>   9401/TCP   3m
```

### Check Endorser Logs

View logs for a specific endorser:

```bash
kubectl logs -l app=org1-issuer
```

Check for successful startup:

```bash
kubectl logs -l app=org1-issuer | grep -i "started\|listening"
```

### Test Endorser Connectivity

Test P2P connectivity between endorsers from within the cluster:

```bash
kubectl run test-endorser --image=nicolaka/netshoot --rm -it -- bash

# Inside the pod, test connectivity:
curl -k http://org1-issuer-service:9101
curl -k http://org1-endorser1-service:9301
```

### Endorser Architecture

Each endorser deployment includes:

1. **Core Configuration Secret**: Contains the `core.yaml` with:

   - FSC identity (certificate/key paths from enrollment)
   - P2P configuration (listen address, websocket)
   - Inline routing configuration (all 6 parties)
   - Persistence configuration (SQLite)
   - Endpoint resolvers (SecretRef to other nodes)
   - Token TMS configuration

2. **Certificate Secrets**: Mounted into the pod at:

   - Sign cert: `/var/hyperledger/fabric/msp/signcerts/cert.pem`
   - Sign key: `/var/hyperledger/fabric/msp/keystore/key.pem`
   - TLS cert: `/var/hyperledger/fabric/tls/`

3. **Resolver Certificates**: Mounted at:

   - `/var/hyperledger/fabric/resolvers/{node-name}/cert.pem`

4. **Persistent Volume**: For SQLite database:
   - Mount path: `/var/hyperledger/fabric/data`
   - File: `fts.sqlite`

### Endorser Port Assignments

Each endorser listens on a unique P2P port:

| Endorser       | Port | Role                 |
| -------------- | ---- | -------------------- |
| org1-issuer    | 9101 | Token Issuer         |
| org1-endorser1 | 9301 | Transaction Endorser |
| org1-endorser2 | 9401 | Transaction Endorser |
| org1-owner1    | 9501 | Token Owner          |
| org1-owner2    | 9601 | Token Owner          |

### Troubleshooting Endorsers

#### Endorser Pod Not Starting

Check endorser status:

```bash
kubectl describe endorser org1-issuer
```

Check pod events:

```bash
kubectl describe pod -l app=org1-issuer
```

#### Resolver Secret Missing

If you see an error about missing resolver secrets:

```bash
# Check which resolver secrets are missing
kubectl get endorser org1-issuer -o yaml | grep -A 10 status

# Recreate the missing resolver secret (example for owner1)
kubectl get secret org1-owner1-sign-cert -o jsonpath='{.data.cert\.pem}' | \
  base64 -d | \
  kubectl create secret generic org1-issuer-resolver-owner1 --from-file=cert.pem=/dev/stdin
```

#### Certificate Enrollment Failed

Check CA is accessible:

```bash
kubectl logs -l app=org1-issuer | grep -i "enroll\|ca"
```

Verify CA certificate secret exists:

```bash
kubectl get secret test-ca2-tls-crypto
```

#### P2P Connection Issues

Check routing configuration in logs:

```bash
kubectl logs -l app=org1-issuer | grep -i "routing\|p2p\|peer"
```

Verify resolver certificates are mounted:

```bash
kubectl exec -it $(kubectl get pod -l app=org1-issuer -o name) -- ls -la /var/hyperledger/fabric/resolvers/
```

## Step 12: Managing Identities with Identity CRD

The Identity CRD provides a declarative way to manage Fabric identities and their cryptographic materials.

### Understanding Identity CRD

The Identity CRD supports two modes:

1. **Import Existing Certificates**: Organize existing identity materials into structured secrets
2. **Enroll with CA**: Automatically enroll identities with Fabric CA (future feature)

### Create Admin Identity Password Secret

First, create a secret with the admin password for future CA enrollment:

```bash
kubectl apply -f config/samples/admin-password-secret.yaml
```

### Deploy Admin Identity

Create an admin identity using existing CA certificates:

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_identity_admin.yaml
```

This will create an Identity resource that:

- Imports existing admin certificates from the CA
- Creates 6 organized output secrets (sign cert/key/CA + TLS cert/key/CA)
- Tracks certificate expiry dates
- Applies custom labels for easy identification

### Verify Identity Creation

Check the identity status:

```bash
kubectl get identity admin-org1
```

Expected output:

```
NAME         TYPE    MSPID      STATUS   EXPIRY      AGE
admin-org1   admin   Org1MSP    READY    <date>      10s
```

### Verify Generated Secrets

The Identity controller creates structured secrets:

```bash
kubectl get secrets | grep admin-org1
```

Expected output:

```
admin-org1-sign-cert      Opaque   1   10s
admin-org1-sign-key       Opaque   1   10s
admin-org1-sign-cacert    Opaque   1   10s
admin-org1-tls-cert       Opaque   1   10s
admin-org1-tls-key        Opaque   1   10s
admin-org1-tls-cacert     Opaque   1   10s
```

### Validate Certificate Chain

Verify the signing certificate:

```bash
kubectl get secret admin-org1-sign-cert -o jsonpath='{.data.cert\.pem}' | base64 -d | openssl x509 -noout -subject -issuer -dates
```

Verify the certificate and key match:

```bash
# Extract public key from certificate
kubectl get secret admin-org1-sign-cert -o jsonpath='{.data.cert\.pem}' | base64 -d | openssl x509 -noout -pubkey > /tmp/cert-pubkey.pem

# Extract public key from private key
kubectl get secret admin-org1-sign-key -o jsonpath='{.data.cert\.pem}' | base64 -d | openssl ec -pubout > /tmp/key-pubkey.pem 2>/dev/null

# Compare (should be identical)
diff /tmp/cert-pubkey.pem /tmp/key-pubkey.pem
```

### Using Identity Secrets in Applications

The generated secrets can be referenced in other Fabric components:

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Endorser
metadata:
  name: my-endorser
spec:
  # Reference the identity secrets
  signCert:
    secretKeyRef:
      name: admin-org1-sign-cert
      key: cert.pem
  signKey:
    secretKeyRef:
      name: admin-org1-sign-key
      key: cert.pem
```

### Identity Lifecycle Management

**Update Identity**: Modify the Identity resource to update configurations

```bash
kubectl edit identity admin-org1
```

**Delete Identity**: Automatically cleans up all generated secrets

```bash
kubectl delete identity admin-org1
```

All output secrets (`admin-org1-*`) will be automatically deleted due to owner references.

### Creating User Identities

Create additional user identities for different roles:

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Identity
metadata:
  name: user1-org1
spec:
  type: client
  mspID: Org1MSP
  existingCertificates:
    signCertRef:
      name: user1-cert
      key: cert.pem
    signKeyRef:
      name: user1-key
      key: key.pem
    signCACertRef:
      name: test-ca2-msp-crypto
      key: certfile
  output:
    secretPrefix: user1-org1
    labels:
      app: fabric-client
      user: user1
```

### Future: CA Enrollment

In a future release, the Identity CRD will support automatic enrollment:

```yaml
spec:
  type: admin
  mspID: Org1MSP

  # Enroll with CA (future feature)
  enrollment:
    caRef:
      name: test-ca2
    enrollID: admin
    enrollSecretRef:
      name: admin-password
      key: password
    enrollTLS: true
    attrs:
      - name: "hf.Registrar.Roles"
        value: "client,orderer,peer,user"
        ecert: true

  # Register new user before enrollment (future feature)
  register:
    registrarID: admin
    registrarSecretRef:
      name: admin-password
      key: password
    maxEnrollments: -1
```

### Identity Status Fields

The Identity controller tracks:

- **Status**: READY, FAILED, INVALID
- **Message**: Detailed status message
- **EnrollmentTime**: When the identity was created/enrolled
- **CertificateExpiry**: Signing certificate expiration date
- **TLSCertificateExpiry**: TLS certificate expiration date
- **OutputSecrets**: Names of all generated secrets

Check detailed status:

```bash
kubectl describe identity admin-org1
```

## Next Steps

Now that your complete Fabric X network is running with endorsers, you can:

1. **Submit Token Transactions**: Use the token SDK to issue, transfer, and redeem tokens
2. **Monitor Transaction Flow**: Observe transactions flowing through endorsers to the ordering service
3. **Test Endorsement Policies**: Verify that transactions require signatures from both endorser1 and endorser2
4. **Deploy Additional Applications**: Build and deploy token-based applications

## Troubleshooting

### Pods Not Starting

Check pod events:

```bash
kubectl describe pod <pod-name>
```

Check pod logs:

```bash
kubectl logs <pod-name>
```

### DNS Resolution Issues

Test DNS resolution from within a pod:

```bash
kubectl run test-dns --image=busybox --rm -it -- nslookup test-ca2.default.svc.cluster.local
```

### Certificate Issues

Check CA logs:

```bash
kubectl logs -l app.kubernetes.io/name=fabric-ca
```

Verify certificate secrets were created:

```bash
kubectl get secrets | grep cert
```

### Orderer Not Starting

1. Check that the CA is running and accessible
2. Verify the genesis block was created
3. Check orderer logs for enrollment errors:

```bash
kubectl logs <orderer-pod-name>
```

### Consensus Issues

Check consenter logs for error messages:

```bash
kubectl logs -l app.kubernetes.io/component=consenter --tail=100
```

Verify all 4 consenters are running:

```bash
kubectl get pods -l app.kubernetes.io/component=consenter
```

### Network Connectivity

Test connectivity between components:

```bash
# Create a test pod
kubectl run test-net --image=nicolaka/netshoot --rm -it -- bash

# Inside the pod, test connectivity to services:
curl http://orderergroup-party1-router-service:443
curl http://orderergroup-party1-consenter:7052
```

### Clean Up

To remove all deployed resources:

```bash
# Delete Fabric components
kubectl delete -f config/samples/fabricx_v1alpha1_genesis.yaml
kubectl delete -f config/samples/fabricx_v1alpha1_orderergroup_party4.yaml
kubectl delete -f config/samples/fabricx_v1alpha1_orderergroup_party3.yaml
kubectl delete -f config/samples/fabricx_v1alpha1_orderergroup_party2.yaml
kubectl delete -f config/samples/fabricx_v1alpha1_orderergroup_party1.yaml
kubectl delete -f config/samples/fabricx_v1alpha1_ca.yaml

# Uninstall operator
make undeploy

# Delete cluster (K3D)
k3d cluster delete k8s-hlf

# Or delete cluster (KinD)
kind delete cluster
```

## Additional Resources

- [Hyperledger Fabric Documentation](https://hyperledger-fabric.readthedocs.io/)
- [Fabric X Operator GitHub](https://github.com/kfsoftware/hlf-operator)
- [Kubectl HLF Plugin](https://github.com/kfsoftware/kubectl-hlf)

## Architecture Overview

The deployed network consists of:

- **1 Certificate Authority (CA)**: Issues certificates for all network participants
- **4 Orderer Parties**: Each running a full set of ordering components
  - 4 Routers (one per party)
  - 8 Batchers (2 shards per party)
  - 4 Consenters (SmartBFT consensus)
  - 4 Assemblers (one per party)
- **Genesis Block**: Contains the initial channel configuration

This configuration provides a Byzantine Fault Tolerant network that can tolerate up to 1 faulty node (with 4 consenters, BFT requires 3f+1 nodes, where f=1).

## Component Configuration Highlights

### Storage

All components use persistent storage with:

- Access Mode: `ReadWriteOnce`
- Size: `10Gi` (orderers), `1Gi` (CA)
- Storage Class: `fast-ssd` (configurable)

### Resource Limits

Recommended for production:

- **CA**: 512Mi memory, 500m CPU
- **Router**: 1Gi memory, 1 CPU
- **Batcher**: 2Gi memory, 2 CPU (per instance)
- **Consenter**: 4Gi memory, 2 CPU
- **Assembler**: 2Gi memory, 1 CPU

### Network Configuration

- Internal communication uses Kubernetes service discovery
- TLS is enabled for all component communication
- Components are accessible via ClusterIP services within the cluster

## Security Considerations

1. **TLS Everywhere**: All communication is encrypted using TLS
2. **Certificate Enrollment**: All components automatically enroll with the CA
3. **Mutual TLS**: Components authenticate each other using certificates
4. **RBAC**: Kubernetes RBAC controls access to resources
5. **Network Policies**: Consider implementing Kubernetes network policies for additional isolation

## Performance Tuning

Key configuration parameters for performance:

### Batcher Configuration

```yaml
BATCHER_TIMEOUT: 2s
BATCHER_PREFERRED_MAX_BYTES: 2097152
```

### Consensus Configuration (SmartBFT)

```yaml
requestBatchMaxCount: 100
requestBatchMaxBytes: 10485760
requestBatchMaxInterval: "500ms"
leaderHeartbeatTimeout: "1m0s"
```

Adjust these values based on your transaction volume and latency requirements.
