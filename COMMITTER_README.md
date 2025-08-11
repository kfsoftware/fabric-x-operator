# Fabric X Committer Controller

The Fabric X Committer Controller manages the deployment and configuration of Committer components in a Hyperledger Fabric X network. The Committer is responsible for transaction validation, commitment, and coordination across the network.

## Architecture

The Committer consists of four main subcomponents:

1. **Coordinator** - Manages transaction coordination and dependency resolution
2. **Sidecar** - Handles orderer communication and genesis block management
3. **Validator** - Validates transactions and manages database operations
4. **Verifier Service** - Provides parallel transaction verification services

## Components

### Coordinator

The Coordinator component manages transaction coordination and dependency resolution:

- **Port**: 9001
- **Configuration**: Manages dependency graphs and transaction coordination
- **Endpoints**: Communicates with verifier (localhost:5001) and validator (localhost:6001)
- **Monitoring**: Prometheus metrics on port 2120

### Sidecar

The Sidecar component handles orderer communication and genesis block management:

- **Port**: 5050
- **Configuration**: Manages orderer connections and genesis block
- **Features**: 
  - Orderer channel communication
  - Genesis block management
  - Ledger path management
  - Last committed block tracking
- **Monitoring**: Prometheus metrics on port 2111

### Validator

The Validator component validates transactions and manages database operations:

- **Port**: 6001
- **Configuration**: Database connectivity and transaction validation
- **Features**:
  - Database connection management
  - Transaction validation
  - Resource limits management
  - Worker pool management
- **Monitoring**: Prometheus metrics on port 2116

### Verifier Service

The Verifier Service provides parallel transaction verification:

- **Port**: 5001
- **Configuration**: Parallel execution and batch processing
- **Features**:
  - Parallel transaction verification
  - Batch size and time management
  - Channel buffer management
  - High parallelism support
- **Monitoring**: Prometheus metrics on port 2115

## Configuration

### Sample Configuration

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Committer
metadata:
  name: fabric-x-committer
  namespace: default

spec:
  bootstrapMode: deploy
  mspid: CommitterMSP
  common:
    replicas: 1
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
    podAnnotations:
      prometheus.io/port: '8080'
      prometheus.io/scrape: 'true'
    podLabels:
      app.kubernetes.io/component: fabric-x
      app.kubernetes.io/part-of: committer
  genesis:
    secretName: fabricx-shared-genesis
    secretKey: genesis.block
    secretNamespace: default
  components:
    coordinator:
      replicas: 1
      resources:
        requests:
          cpu: 200m
          memory: 256Mi
        limits:
          cpu: 1000m
          memory: 1Gi
      env:
        - name: COORDINATOR_LOG_LEVEL
          value: INFO
      ingress:
        istio:
          hosts:
            - coordinator-committer.localho.st
          ingressGateway: ingressgateway
          port: 443
    sidecar:
      replicas: 1
      resources:
        requests:
          cpu: 300m
          memory: 512Mi
        limits:
          cpu: 1500m
          memory: 2Gi
      env:
        - name: SIDECAR_LOG_LEVEL
          value: INFO
        - name: SIDECAR_CHANNEL_ID
          value: arma
      ingress:
        istio:
          hosts:
            - sidecar-committer.localho.st
          ingressGateway: ingressgateway
          port: 443
    validator:
      replicas: 1
      resources:
        requests:
          cpu: 400m
          memory: 1Gi
        limits:
          cpu: 2000m
          memory: 4Gi
      env:
        - name: VALIDATOR_LOG_LEVEL
          value: INFO
        - name: VALIDATOR_DB_HOST
          value: localhost
        - name: VALIDATOR_DB_PORT
          value: "5435"
      ingress:
        istio:
          hosts:
            - validator-committer.localho.st
          ingressGateway: ingressgateway
          port: 443
    verifierService:
      replicas: 1
      resources:
        requests:
          cpu: 200m
          memory: 256Mi
        limits:
          cpu: 1000m
          memory: 1Gi
      env:
        - name: VERIFIER_LOG_LEVEL
          value: INFO
        - name: VERIFIER_PARALLELISM
          value: "80"
      ingress:
        istio:
          hosts:
            - verifier-committer.localho.st
          ingressGateway: ingressgateway
          port: 443
  enrollment:
    sign:
      ca:
        caname: ca
        cahost: test-ca2.default
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: test-ca2-tls-crypto
            namespace: default
        enrollid: admin
        enrollsecret: adminpw
    tls:
      ca:
        caname: tlsca
        cahost: test-ca2.default
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: test-ca2-tls-crypto
            namespace: default
        enrollid: admin
        enrollsecret: adminpw
```

## Bootstrap Modes

### Configure Mode

In configure mode, only certificates are created:

```yaml
spec:
  bootstrapMode: configure
```

### Deploy Mode

In deploy mode, all resources are created:

```yaml
spec:
  bootstrapMode: deploy
```

## Component Configuration

### Common Configuration

Common configuration applies to all components:

```yaml
spec:
  common:
    replicas: 1
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
    podAnnotations:
      prometheus.io/port: '8080'
      prometheus.io/scrape: 'true'
    podLabels:
      app.kubernetes.io/component: fabric-x
      app.kubernetes.io/part-of: committer
```

### Component-Specific Configuration

Each component can override common settings:

```yaml
spec:
  components:
    coordinator:
      replicas: 2  # Override common replicas
      resources:
        requests:
          cpu: 200m  # Override common resources
          memory: 256Mi
        limits:
          cpu: 1000m
          memory: 1Gi
      env:
        - name: COORDINATOR_LOG_LEVEL
          value: INFO
      ingress:
        istio:
          hosts:
            - coordinator-committer.localho.st
          ingressGateway: ingressgateway
          port: 443
```

## Genesis Block Configuration

The Sidecar component requires a genesis block:

```yaml
spec:
  genesis:
    secretName: fabricx-shared-genesis
    secretKey: genesis.block
    secretNamespace: default
```

## Certificate Management

Certificates are managed through enrollment configuration:

```yaml
spec:
  enrollment:
    sign:
      ca:
        caname: ca
        cahost: test-ca2.default
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: test-ca2-tls-crypto
            namespace: default
        enrollid: admin
        enrollsecret: adminpw
    tls:
      ca:
        caname: tlsca
        cahost: test-ca2.default
        caport: 7054
        catls:
          secretRef:
            key: tls.crt
            name: test-ca2-tls-crypto
            namespace: default
        enrollid: admin
        enrollsecret: adminpw
```

## Deployment

### Prerequisites

1. Kubernetes cluster with Istio installed
2. Fabric CA server running
3. Genesis block secret available

### Installation

1. Apply the CRD:
```bash
kubectl apply -f config/crd/bases/fabricx.kfsoft.tech_committers.yaml
```

2. Deploy the operator:
```bash
kubectl apply -f config/samples/fabricx_v1alpha1_committer.yaml
```

### Verification

Check the status of the Committer:

```bash
kubectl get committer fabric-x-committer -o yaml
```

Check component status:

```bash
kubectl describe committer fabric-x-committer
```

## Monitoring

Each component exposes Prometheus metrics:

- **Coordinator**: Port 2120
- **Sidecar**: Port 2111
- **Validator**: Port 2116
- **Verifier Service**: Port 2115

## Troubleshooting

### Common Issues

1. **Certificate Issues**: Ensure Fabric CA is running and accessible
2. **Genesis Block**: Verify the genesis block secret exists and is accessible
3. **Resource Limits**: Check if pods have sufficient resources
4. **Network Connectivity**: Ensure components can communicate with each other

### Logs

Check component logs:

```bash
# Coordinator logs
kubectl logs -l app=coordinator,release=fabric-x-committer

# Sidecar logs
kubectl logs -l app=sidecar,release=fabric-x-committer

# Validator logs
kubectl logs -l app=validator,release=fabric-x-committer

# Verifier Service logs
kubectl logs -l app=verifier-service,release=fabric-x-committer
```

### Status

Check component status:

```bash
kubectl get committer fabric-x-committer -o jsonpath='{.status.componentStatuses}'
```

## Development

### Adding New Components

To add a new component:

1. Create the component controller in `internal/controller/committer/`
2. Add the component to the CommitterSpec in `api/v1alpha1/committer_types.go`
3. Add configuration templates in `internal/controller/utils/templates.go`
4. Update the main controller in `internal/controller/committer_controller.go`

### Testing

Run the tests:

```bash
make test
```

Run e2e tests:

```bash
make test-e2e
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the Apache License 2.0.
