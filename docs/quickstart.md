# Quickstart: Fabric-X Network on Kubernetes

This guide walks you through deploying a complete 4-party Fabric-X network on a fresh Kubernetes cluster using the fabric-x-operator, then running the block explorer to visualize transactions.

## Prerequisites

- A running Kubernetes cluster (K3D, Kind, or any other)
- `kubectl` configured to access the cluster
- `docker` for building operator images
- `bun` (for the explorer frontend)
- `go` 1.25+ (for the explorer backend)

This guide uses K3D. Create a cluster if you don't have one:

```bash
k3d cluster create fabric-x --agents 2
```

## 1. Install Dependencies

### Gateway API CRDs

```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/standard-install.yaml
```

### Cert-Manager

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.3/cert-manager.yaml
kubectl wait deployment cert-manager-webhook -n cert-manager \
  --for=condition=Available --timeout=5m
```

### CNPG Operator (PostgreSQL)

Required for the committer's validator and query-service components.

```bash
kubectl apply --server-side -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.25/releases/cnpg-1.25.1.yaml
kubectl wait pod -l app.kubernetes.io/name=cloudnative-pg -n cnpg-system \
  --for=condition=Ready --timeout=2m
```

## 2. Build and Deploy the Operator

```bash
export IMG=local/fabric-x-operator:$(date +%Y%m%d%H%M%S)
make docker-build IMG=$IMG
k3d image import $IMG --cluster fabric-x    # adjust cluster name
make install                                 # install CRDs
make deploy IMG=$IMG                         # deploy operator
kubectl wait pod -l control-plane=controller-manager \
  -n fabric-x-operator-system --for=condition=Ready --timeout=2m
```

## 3. Deploy the Certificate Authority

```bash
kubectl apply -f - <<'EOF'
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: net-ca
  namespace: default
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
      - net-ca
      - net-ca.default
      - net-ca.default.svc.cluster.local
      - localhost
  service:
    type: ClusterIP
EOF
```

Wait for the CA to be running:

```bash
kubectl wait --for=jsonpath='{.status.status}'=RUNNING ca/net-ca --timeout=2m
```

Verify the crypto secrets were created:

```bash
kubectl get secret net-ca-tls-crypto net-ca-msp-crypto net-ca-tlsca-crypto
```

## 4. Deploy 4 Orderer Groups (Configure Mode)

SmartBFT consensus requires exactly 4 parties. Deploy all four in `configure` mode first — this enrolls certificates without launching the actual pods.

```bash
for PARTY in 1 2 3 4; do
kubectl apply -f - <<EOF
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: OrdererGroup
metadata:
  name: orderergroup-party${PARTY}
  namespace: default
spec:
  partyID: ${PARTY}
  bootstrapMode: configure
  mspid: Org1MSP
  image: hyperledger/fabric-x-orderer
  imageTag: "0.0.24"
  common:
    replicas: 1
    podLabels:
      app.kubernetes.io/component: fabric-x
    storage:
      accessMode: ReadWriteOnce
      size: 1Gi
  components:
    assembler:
      sans:
        dnsNames:
          - orderergroup-party${PARTY}-assembler.default.svc.cluster.local
      replicas: 1
    batchers:
      - shardID: 1
        sans:
          dnsNames:
            - orderergroup-party${PARTY}-batcher-0.default.svc.cluster.local
        replicas: 1
    consenter:
      consenterID: ${PARTY}
      sans:
        dnsNames:
          - orderergroup-party${PARTY}-consenter.default.svc.cluster.local
      endpoints:
        - orderergroup-party${PARTY}-consenter.default.svc.cluster.local:7050
      replicas: 1
    router:
      sans:
        dnsNames:
          - orderergroup-party${PARTY}-router-service.default.svc.cluster.local
      replicas: 1
  enrollment:
    sign:
      ca:
        caname: ca
        cahost: net-ca.default
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: net-ca-tls-crypto
            namespace: default
        enrollid: admin
        enrollsecret: adminpw
    tls:
      ca:
        caname: tlsca
        cahost: net-ca.default
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: net-ca-tls-crypto
            namespace: default
        enrollid: admin
        enrollsecret: adminpw
  genesis:
    secretKey: genesis.block
    secretName: genesis-block
    secretNamespace: default
EOF
done
```

Wait for all orderer groups to be ready:

```bash
for PARTY in 1 2 3 4; do
  kubectl wait --for=jsonpath='{.status.status}'=RUNNING \
    orderergroup/orderergroup-party${PARTY} --timeout=2m
done
```

## 5. Create the Genesis Block

The Genesis resource generates the genesis block from the enrolled certificates. It references the secrets created by the orderer groups.

```bash
kubectl apply -f - <<'EOF'
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Genesis
metadata:
  name: network-genesis
  namespace: default
spec:
  channelID: arma
  configTemplate:
    configMapName: e2e-configtx
    key: configtx.yaml
  ordererOrganizations:
    - name: Org1MSP
      mspId: Org1MSP
      signCaCertRef:
        name: net-ca-msp-crypto
        namespace: default
        key: certfile
      tlsCaCertRef:
        name: net-ca-tlsca-crypto
        namespace: default
        key: certfile
      endpoints:
        - "id=1,broadcast,orderergroup-party1-router-service.default.svc.cluster.local:7150"
        - "id=1,deliver,orderergroup-party1-assembler.default.svc.cluster.local:7050"
        - "id=2,broadcast,orderergroup-party2-router-service.default.svc.cluster.local:7150"
        - "id=2,deliver,orderergroup-party2-assembler.default.svc.cluster.local:7050"
        - "id=3,broadcast,orderergroup-party3-router-service.default.svc.cluster.local:7150"
        - "id=3,deliver,orderergroup-party3-assembler.default.svc.cluster.local:7050"
        - "id=4,broadcast,orderergroup-party4-router-service.default.svc.cluster.local:7150"
        - "id=4,deliver,orderergroup-party4-assembler.default.svc.cluster.local:7050"
      router:
        host: orderergroup-party1-router-service.default.svc.cluster.local
        port: 7150
        partyID: 1
        signCertRef:
          name: orderergroup-party1-router-sign-cert
          namespace: default
          key: cert.pem
        tlsCertRef:
          name: orderergroup-party1-router-tls-cert
          namespace: default
          key: cert.pem
      assembler:
        host: orderergroup-party1-assembler.default.svc.cluster.local
        port: 7050
        tlsCertRef:
          name: orderergroup-party1-assembler-tls-cert
          namespace: default
          key: cert.pem
  applicationOrgs:
    - name: Org1MSP
      mspId: Org1MSP
      signCaCertRef:
        name: net-ca-msp-crypto
        namespace: default
        key: certfile
      tlsCaCertRef:
        name: net-ca-tlsca-crypto
        namespace: default
        key: certfile
  consenters:
    - id: 1
      mspId: Org1MSP
      host: orderergroup-party1-consenter.default.svc.cluster.local
      port: 7052
      identityRef:
        name: orderergroup-party1-consenter-sign-cert
        namespace: default
        key: cert.pem
      clientTlsCertRef:
        name: orderergroup-party1-consenter-tls-cert
        namespace: default
        key: cert.pem
      serverTlsCertRef:
        name: orderergroup-party1-consenter-tls-cert
        namespace: default
        key: cert.pem
    - id: 2
      mspId: Org1MSP
      host: orderergroup-party2-consenter.default.svc.cluster.local
      port: 7052
      identityRef:
        name: orderergroup-party2-consenter-sign-cert
        namespace: default
        key: cert.pem
      clientTlsCertRef:
        name: orderergroup-party2-consenter-tls-cert
        namespace: default
        key: cert.pem
      serverTlsCertRef:
        name: orderergroup-party2-consenter-tls-cert
        namespace: default
        key: cert.pem
    - id: 3
      mspId: Org1MSP
      host: orderergroup-party3-consenter.default.svc.cluster.local
      port: 7052
      identityRef:
        name: orderergroup-party3-consenter-sign-cert
        namespace: default
        key: cert.pem
      clientTlsCertRef:
        name: orderergroup-party3-consenter-tls-cert
        namespace: default
        key: cert.pem
      serverTlsCertRef:
        name: orderergroup-party3-consenter-tls-cert
        namespace: default
        key: cert.pem
    - id: 4
      mspId: Org1MSP
      host: orderergroup-party4-consenter.default.svc.cluster.local
      port: 7052
      identityRef:
        name: orderergroup-party4-consenter-sign-cert
        namespace: default
        key: cert.pem
      clientTlsCertRef:
        name: orderergroup-party4-consenter-tls-cert
        namespace: default
        key: cert.pem
      serverTlsCertRef:
        name: orderergroup-party4-consenter-tls-cert
        namespace: default
        key: cert.pem
  parties:
    - partyID: 1
      caCerts:
        - name: net-ca-msp-crypto
          namespace: default
          key: certfile
      tlsCaCerts:
        - name: net-ca-tlsca-crypto
          namespace: default
          key: certfile
      routerConfig:
        host: orderergroup-party1-router-service.default.svc.cluster.local
        port: 7150
        tlsCert:
          name: orderergroup-party1-router-tls-cert
          namespace: default
          key: cert.pem
      batchersConfig:
        - shardID: 1
          host: orderergroup-party1-batcher-0.default.svc.cluster.local
          port: 7151
          signCert:
            name: orderergroup-party1-batcher-0-sign-cert
            namespace: default
            key: cert.pem
          tlsCert:
            name: orderergroup-party1-batcher-0-tls-cert
            namespace: default
            key: cert.pem
      consenterConfig:
        host: orderergroup-party1-consenter.default.svc.cluster.local
        port: 7052
        signCert:
          name: orderergroup-party1-consenter-sign-cert
          namespace: default
          key: cert.pem
        tlsCert:
          name: orderergroup-party1-consenter-tls-cert
          namespace: default
          key: cert.pem
      assemblerConfig:
        host: orderergroup-party1-assembler.default.svc.cluster.local
        port: 7050
        tlsCert:
          name: orderergroup-party1-assembler-tls-cert
          namespace: default
          key: cert.pem
    - partyID: 2
      caCerts:
        - name: net-ca-msp-crypto
          namespace: default
          key: certfile
      tlsCaCerts:
        - name: net-ca-tlsca-crypto
          namespace: default
          key: certfile
      routerConfig:
        host: orderergroup-party2-router-service.default.svc.cluster.local
        port: 7150
        tlsCert:
          name: orderergroup-party2-router-tls-cert
          namespace: default
          key: cert.pem
      batchersConfig:
        - shardID: 1
          host: orderergroup-party2-batcher-0.default.svc.cluster.local
          port: 7151
          signCert:
            name: orderergroup-party2-batcher-0-sign-cert
            namespace: default
            key: cert.pem
          tlsCert:
            name: orderergroup-party2-batcher-0-tls-cert
            namespace: default
            key: cert.pem
      consenterConfig:
        host: orderergroup-party2-consenter.default.svc.cluster.local
        port: 7052
        signCert:
          name: orderergroup-party2-consenter-sign-cert
          namespace: default
          key: cert.pem
        tlsCert:
          name: orderergroup-party2-consenter-tls-cert
          namespace: default
          key: cert.pem
      assemblerConfig:
        host: orderergroup-party2-assembler.default.svc.cluster.local
        port: 7050
        tlsCert:
          name: orderergroup-party2-assembler-tls-cert
          namespace: default
          key: cert.pem
    - partyID: 3
      caCerts:
        - name: net-ca-msp-crypto
          namespace: default
          key: certfile
      tlsCaCerts:
        - name: net-ca-tlsca-crypto
          namespace: default
          key: certfile
      routerConfig:
        host: orderergroup-party3-router-service.default.svc.cluster.local
        port: 7150
        tlsCert:
          name: orderergroup-party3-router-tls-cert
          namespace: default
          key: cert.pem
      batchersConfig:
        - shardID: 1
          host: orderergroup-party3-batcher-0.default.svc.cluster.local
          port: 7151
          signCert:
            name: orderergroup-party3-batcher-0-sign-cert
            namespace: default
            key: cert.pem
          tlsCert:
            name: orderergroup-party3-batcher-0-tls-cert
            namespace: default
            key: cert.pem
      consenterConfig:
        host: orderergroup-party3-consenter.default.svc.cluster.local
        port: 7052
        signCert:
          name: orderergroup-party3-consenter-sign-cert
          namespace: default
          key: cert.pem
        tlsCert:
          name: orderergroup-party3-consenter-tls-cert
          namespace: default
          key: cert.pem
      assemblerConfig:
        host: orderergroup-party3-assembler.default.svc.cluster.local
        port: 7050
        tlsCert:
          name: orderergroup-party3-assembler-tls-cert
          namespace: default
          key: cert.pem
    - partyID: 4
      caCerts:
        - name: net-ca-msp-crypto
          namespace: default
          key: certfile
      tlsCaCerts:
        - name: net-ca-tlsca-crypto
          namespace: default
          key: certfile
      routerConfig:
        host: orderergroup-party4-router-service.default.svc.cluster.local
        port: 7150
        tlsCert:
          name: orderergroup-party4-router-tls-cert
          namespace: default
          key: cert.pem
      batchersConfig:
        - shardID: 1
          host: orderergroup-party4-batcher-0.default.svc.cluster.local
          port: 7151
          signCert:
            name: orderergroup-party4-batcher-0-sign-cert
            namespace: default
            key: cert.pem
          tlsCert:
            name: orderergroup-party4-batcher-0-tls-cert
            namespace: default
            key: cert.pem
      consenterConfig:
        host: orderergroup-party4-consenter.default.svc.cluster.local
        port: 7052
        signCert:
          name: orderergroup-party4-consenter-sign-cert
          namespace: default
          key: cert.pem
        tlsCert:
          name: orderergroup-party4-consenter-tls-cert
          namespace: default
          key: cert.pem
      assemblerConfig:
        host: orderergroup-party4-assembler.default.svc.cluster.local
        port: 7050
        tlsCert:
          name: orderergroup-party4-assembler-tls-cert
          namespace: default
          key: cert.pem
  output:
    secretName: genesis-block
    blockKey: genesis.block
EOF
```

Wait for the genesis block to be created:

```bash
kubectl wait --for=jsonpath='{.status.status}'=RUNNING genesis/network-genesis --timeout=2m
kubectl get secret genesis-block
```

## 6. Switch Orderer Groups to Deploy Mode

Now that the genesis block exists, switch all orderer groups from `configure` to `deploy` mode. This launches the actual orderer pods.

```bash
for PARTY in 1 2 3 4; do
  kubectl patch orderergroup orderergroup-party${PARTY} \
    --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
done
```

Wait for the orderer pods to be ready (this may take a few minutes):

```bash
for PARTY in 1 2 3 4; do
  echo "Waiting for party ${PARTY} pods..."
  kubectl wait pod -l ordererrouter=orderergroup-party${PARTY}-router \
    --for=condition=Ready --timeout=5m
  kubectl wait pod -l release=orderergroup-party${PARTY}-consenter \
    --for=condition=Ready --timeout=5m
  kubectl wait pod -l release=orderergroup-party${PARTY}-batcher-0 \
    --for=condition=Ready --timeout=5m
  kubectl wait pod -l ordererassembler=orderergroup-party${PARTY}-assembler \
    --for=condition=Ready --timeout=5m
done
```

## 7. Deploy PostgreSQL

The committer's validator and query-service need PostgreSQL for state storage.

```bash
kubectl apply -f - <<'EOF'
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: network-postgres
  namespace: default
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
EOF

kubectl wait pod -l cnpg.io/cluster=network-postgres \
  --for=condition=Ready --timeout=2m
```

## 8. Deploy the Committer

The committer validates and commits transactions to the ledger.

```bash
kubectl apply -f - <<'EOF'
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Committer
metadata:
  name: network-committer
  namespace: default
spec:
  bootstrapMode: deploy
  mspid: Org1MSP
  image: hyperledger/fabric-x-committer
  imageTag: "0.1.9"
  common:
    replicas: 1
  genesis:
    secretName: genesis-block
    secretKey: genesis.block
    secretNamespace: default
  components:
    ordererEndpoints:
      - orderergroup-party1-assembler.default.svc.cluster.local:7050
    committerHost: network-committer-coordinator-service.default.svc.cluster.local
    committerPort: 9001
    coordinatorVerifierEndpoints:
      - network-committer-verifier-service.default.svc.cluster.local:5001
    coordinatorValidatorCommitterEndpoints:
      - network-committer-validator-service.default.svc.cluster.local:6001
    coordinator:
      replicas: 1
    sidecar:
      replicas: 1
      env:
        - name: SC_SIDECAR_ORDERER_CHANNEL_ID
          value: arma
    validator:
      replicas: 1
      postgresql:
        host: network-postgres-rw.default.svc.cluster.local
        port: 5432
        database: fabricx
        username: fabricx
        passwordSecret:
          name: network-postgres-app
          namespace: default
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
        host: network-postgres-rw.default.svc.cluster.local
        port: 5432
        database: fabricx
        username: fabricx
        passwordSecret:
          name: network-postgres-app
          namespace: default
          key: password
  enrollment:
    sign:
      ca:
        caname: ca
        cahost: net-ca.default
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: net-ca-tls-crypto
            namespace: default
        enrollid: admin
        enrollsecret: adminpw
    tls:
      ca:
        caname: tlsca
        cahost: net-ca.default
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: net-ca-tls-crypto
            namespace: default
        enrollid: admin
        enrollsecret: adminpw
EOF
```

Wait for all committer components:

```bash
kubectl wait --for=jsonpath='{.status.status}'=RUNNING committer/network-committer --timeout=2m
kubectl wait pod -l app=coordinator --for=condition=Ready --timeout=2m
kubectl wait pod -l app=sidecar --for=condition=Ready --timeout=2m
kubectl wait pod -l app=verifier --for=condition=Ready --timeout=2m
```

## 9. Create an Admin Identity

```bash
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: admin-enroll
  namespace: default
type: Opaque
stringData:
  password: adminpw
---
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Identity
metadata:
  name: admin-identity
  namespace: default
spec:
  type: admin
  mspID: Org1MSP
  enrollment:
    caRef:
      name: net-ca
      namespace: default
    enrollID: admin
    enrollSecretRef:
      name: admin-enroll
      key: password
      namespace: default
  output:
    secretName: admin-cert
EOF

kubectl wait --for=jsonpath='{.status.status}'=READY identity/admin-identity --timeout=2m
```

## 10. Deploy a Chain Namespace

This creates a token namespace on the ledger — the foundation for token operations.

```bash
kubectl apply -f - <<'EOF'
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: ChainNamespace
metadata:
  name: token-ns
spec:
  name: "token_namespace"
  orderer: "orderergroup-party1-router-service.default.svc.cluster.local:7150"
  sidecar: "network-committer-sidecar-service.default.svc.cluster.local:5050"
  finalityTimeoutSeconds: 120
  tls:
    enabled: false
  mspID: "Org1MSP"
  identity:
    name: "admin-identity"
    namespace: "default"
    key: "key.pem"
  channel: "arma"
  version: -1
EOF
```

Wait for the namespace to be deployed:

```bash
kubectl wait --for=jsonpath='{.status.status}'=Deployed chainnamespace/token-ns --timeout=2m
kubectl get chainnamespace token-ns -o jsonpath='{.status.txID}'
```

You should see a transaction ID confirming the namespace was committed to the ledger.

## 11. Deploy the Token SDK Application

The token-sdk-x application runs 4 services on the network: an **issuer** (creates tokens), an **endorser** (validates transactions), and two **owners** (hold and transfer tokens). Each service gets its MSP identity from the `admin-cert` secret via init containers.

### Build the Token Images

This step requires the [token-sdk-x source](https://github.com/hyperledger/fabric-samples) cloned locally. The build script patches the node configs for in-cluster DNS, builds 4 Docker images, and imports them into k3d.

```bash
cd /path/to/fabric-samples/token-sdk-x

# Adjust to match your committer service names
export SIDECAR_DNS="network-committer-sidecar-service:5050"
export QUERY_DNS="network-committer-query-service-service:9001"
export K3D_CLUSTER="fabric-x"    # your k3d cluster name

bash scripts/k8s/build.sh
```

This creates four images: `tokensdk-issuer:dev`, `tokensdk-endorser1:dev`, `tokensdk-owner1:dev`, `tokensdk-owner2:dev`.

### Deploy to Kubernetes

```bash
kubectl apply -f token-sdk-x/k8s/manifests.yaml
```

Wait for all pods to be ready:

```bash
kubectl wait pod -l app=tokensdk-issuer --for=condition=Ready --timeout=2m
kubectl wait pod -l app=tokensdk-endorser1 --for=condition=Ready --timeout=2m
kubectl wait pod -l app=tokensdk-owner1 --for=condition=Ready --timeout=2m
kubectl wait pod -l app=tokensdk-owner2 --for=condition=Ready --timeout=2m
```

### Initialize the Token Namespace

The endorser must publish the zero-knowledge public parameters to the ledger before any tokens can be issued:

```bash
kubectl port-forward svc/tokensdk-endorser1 9300:9300 &

curl -X POST http://localhost:9300/endorser/init
```

### Issue Tokens

Port-forward the issuer and issue 10000 EURX cents (100.00 EUR) to alice on owner1:

```bash
kubectl port-forward svc/tokensdk-issuer 9100:9100 &

curl -s -X POST http://localhost:9100/issuer/issue \
  -H 'Content-Type: application/json' \
  -d '{
    "amount": {"code": "EURX", "value": 10000},
    "counterparty": {"node": "owner1", "account": "alice"}
  }'
```

### Check Balances

```bash
kubectl port-forward svc/tokensdk-owner1 9500:9500 &

curl -s http://localhost:9500/owner/accounts | jq .
```

### Transfer Tokens

Transfer 3000 cents (30.00 EUR) from alice to carlos on owner2:

```bash
curl -s -X POST http://localhost:9500/owner/accounts/alice/transfer \
  -H 'Content-Type: application/json' \
  -d '{
    "amount": {"code": "EURX", "value": 3000},
    "counterparty": {"node": "owner2", "account": "carlos"}
  }'
```

### View Transaction History

```bash
curl -s http://localhost:9500/owner/accounts/alice/transactions | jq .
```

### Token API Reference

| Endpoint | Service | Method | Description |
|----------|---------|--------|-------------|
| `/endorser/init` | Endorser (9300) | POST | Publish ZK public parameters |
| `/issuer/issue` | Issuer (9100) | POST | Issue tokens to an account |
| `/owner/accounts` | Owner (9500/9600) | GET | List all accounts and balances |
| `/owner/accounts/{id}` | Owner | GET | Get account balance |
| `/owner/accounts/{id}/transfer` | Owner | POST | Transfer tokens |
| `/owner/accounts/{id}/redeem` | Owner | POST | Redeem (burn) tokens |
| `/owner/accounts/{id}/transactions` | Owner | GET | Transaction history |
| `/healthz` | All | GET | Health check |

## 12. Run the Block Explorer (Optional)

The explorer connects to the committer's sidecar (for block data) and query-service (for state queries) via gRPC, plus PostgreSQL for transaction listings. After issuing and transferring tokens above, the explorer lets you see the decoded transactions on-chain.

### Port-Forward the Services

```bash
# Query service (state queries, notifications)
kubectl port-forward svc/network-committer-query-service-service 5500:9001 &

# Sidecar (block and transaction data)
kubectl port-forward svc/network-committer-sidecar-service 5400:5050 &

# PostgreSQL
kubectl port-forward svc/network-postgres-rw 5433:5432 &
```

Get the PostgreSQL password:

```bash
export PG_PASS=$(kubectl get secret network-postgres-app -o jsonpath='{.data.password}' | base64 -d)
```

### Build and Run

```bash
cd explorer
make build
./bin/explorer \
  --query-addr localhost:5500 \
  --sidecar-addr localhost:5400 \
  --http-addr :9090 \
  --pg-dsn "postgres://fabricx:${PG_PASS}@localhost:5433/fabricx?sslmode=disable"
```

Open http://localhost:9090 in your browser.

### What You'll See

- **Dashboard**: Blockchain height, total blocks, total transactions, recent activity
- **Transactions**: Paginated list of all transactions with status (COMMITTED / ABORTED)
- **Block Detail**: Click a block to see its header (hashes) and contained transactions
- **TX Detail**: Click a transaction to see the decoded envelope:
  - **Endorsers**: MSP ID and certificate subject of each endorsing peer
  - **Namespaces**: Each namespace touched by the transaction
  - **Reads**: Keys read with their versions
  - **Read-Writes**: Keys read and written (e.g., token request hashes, ownership records)
  - **Blind Writes**: Keys written without prior read (e.g., new token outputs with Pedersen commitments)
- **State Explorer**: Browse the current state of any namespace (e.g., `token_namespace`)
- **Live Feed**: Real-time SSE stream of new blocks as they are committed

## Architecture Reference

### Bootstrap Sequence

```
CA (RUNNING)
  └─> OrdererGroup x4 (configure → RUNNING)
        └─> Genesis (RUNNING) → genesis-block secret
              └─> OrdererGroup x4 (deploy → pods running)
                    └─> Committer (RUNNING)
                          └─> Identity (READY)
                                └─> ChainNamespace (Deployed)
                                      └─> Token SDK App (issuer, endorser, owner1, owner2)
                                            └─> Block Explorer (optional)
```

### Component Ports

| Component | Port | Purpose |
|-----------|------|---------|
| Router | 7150 | Transaction broadcast |
| Assembler | 7050 | Block delivery |
| Consenter | 7052 | BFT consensus |
| Batcher | 7151 | Shard collection |
| Coordinator | 9001 | Transaction coordination |
| Sidecar | 5050 | Block query + delivery listener |
| Verifier | 5001 | Transaction verification |
| Validator | 6001 | Transaction validation |
| Query Service | 9001 | State queries + notifications |
| Fabric CA | 7054 | Certificate enrollment |
| PostgreSQL | 5432 | Committer state storage |
| Token Issuer | 9100 | Issue tokens (REST API) |
| Token Endorser | 9300 | Endorse transactions + init params |
| Token Owner 1 | 9500 | Alice/Bob wallets (REST API) |
| Token Owner 2 | 9600 | Carlos/Dan wallets (REST API) |
| Explorer | 9090 | Block explorer web UI |

### Key Design Concepts

- **Two-phase bootstrap**: OrdererGroups start in `configure` mode (enroll certs, create secrets) then switch to `deploy` mode (launch pods) after the genesis block exists
- **SmartBFT consensus**: Requires exactly 4 parties for Byzantine fault tolerance
- **UTXO token model**: Token transfers create new outputs (blind writes with Pedersen commitments hiding amounts) and spend existing outputs (read-writes)
- **Zero-knowledge privacy**: All token amounts are hidden inside Pedersen commitments; ZK proofs guarantee correctness without revealing values

## Cleanup

```bash
# Token SDK app
kubectl delete deploy tokensdk-issuer tokensdk-endorser1 tokensdk-owner1 tokensdk-owner2
kubectl delete svc tokensdk-issuer tokensdk-endorser1 tokensdk-owner1 tokensdk-owner2
kubectl delete configmap tokensdk-msp-init

# Fabric-X network
kubectl delete chainnamespace --all
kubectl delete committer --all
kubectl delete genesis --all
kubectl delete orderergroup --all
kubectl delete identity --all
kubectl delete ca --all
kubectl delete cluster.postgresql.cnpg.io network-postgres
```
