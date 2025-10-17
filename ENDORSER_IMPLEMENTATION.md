# Endorser Controller Implementation

## Overview

A complete implementation of the Endorser controller for fabric-x-operator with typed core.yaml configuration support. The controller manages Fabric Smart Client endorsers with full lifecycle management including certificates, configuration secrets, and Kubernetes resources.

## Implementation Summary

### 1. Typed API Structures ([api/v1alpha1/endorser_types.go](api/v1alpha1/endorser_types.go))

Created comprehensive typed structures that map to the core.yaml format:

#### Core Configuration Types
- **EndorserCoreConfig**: Root configuration structure
- **LoggingConfig**: Logging settings (level, format)
- **FSCConfig**: Fabric Smart Client configuration
  - **FSCIdentity**: Certificate and key configuration
  - **FSCP2PConfig**: P2P networking (listen address, type, routing)
  - **PersistenceConfig**: Storage configuration (SQLite, PostgreSQL)
  - **EndpointConfig**: Endpoint resolvers with identities and addresses

#### Fabric Integration
- **FabricConfig**: Hyperledger Fabric integration
  - **MSPConfig**: MSP configurations
  - **FabricTLSConfig**: TLS settings
  - **PeerConfig**: Peer configurations
  - **ChannelConfig**: Channel configurations

#### Token Service
- **TokenConfig**: Token service configuration
  - **TMSConfig**: Token Management Service
  - **FSCEndorsementConfig**: Endorsement policies and settings

### 2. Helper Functions ([api/v1alpha1/endorser_helpers.go](api/v1alpha1/endorser_helpers.go))

Utility functions for Endorser resources:
- `GenerateCoreYAML()`: Converts typed configuration to YAML format
- `GetCoreConfigSecretName()`: Returns secret name for core.yaml
- `GetCertSecretName()`: Returns secret name for certificates
- `GetServiceName()`: Returns Kubernetes service name
- `GetDeploymentName()`: Returns deployment name
- `GetFullImage()`: Returns full container image with tag

### 3. Comprehensive Controller ([internal/controller/endorser_controller.go](internal/controller/endorser_controller.go))

Full-featured controller with extensive functionality:

#### Bootstrap Modes
- **configure**: Only creates certificates (preparation phase)
- **deploy**: Full deployment with all resources

#### Resource Management

1. **Certificates**
   - Automatic enrollment via CA
   - Sign and TLS certificates
   - Stored in Kubernetes secrets
   - Integration with existing certs package

2. **Core Configuration Secret**
   - Generates core.yaml from typed config
   - Automatic YAML generation
   - Mounted into pod at `/var/hyperledger/fabric/config`

3. **Persistent Volume Claims**
   - Optional storage configuration
   - Configurable size, access mode, storage class
   - Mounted at `/var/hyperledger/fabric/data`

4. **Service**
   - ClusterIP service
   - P2P port: 9301
   - Metrics port: 9090

5. **Deployment**
   - Configurable replicas
   - Resource limits/requests
   - Pod annotations and labels
   - Volume mounts for config and certificates
   - Automatic owner references for garbage collection

#### Features
- Panic recovery for stability
- Finalizer management for cleanup
- Status updates with detailed messages
- Continuous reconciliation (1-minute requeue)
- Proper error handling and logging
- Type conversion between API and certs packages

### 4. Sample Configuration ([config/samples/fabricx_v1alpha1_endorser.yaml](config/samples/fabricx_v1alpha1_endorser.yaml))

Comprehensive example demonstrating:
- All configuration options
- Logging setup
- FSC identity and P2P configuration
- Multiple endpoint resolvers (issuer, auditor, endorsers, owners)
- Fabric integration with MSPs, peers, channels
- Token service with endorsement policy
- CA enrollment configuration
- Istio ingress configuration

### 5. Comprehensive Tests ([internal/controller/endorser_controller_test.go](internal/controller/endorser_controller_test.go))

#### Test Coverage

**Integration Tests:**
1. **Basic Endorser in Configure Mode**
   - Creates endorser resource
   - Verifies status updates
   - Confirms configure mode operation

2. **Full Deployment in Deploy Mode**
   - Creates endorser with all configurations
   - Verifies core config secret creation
   - Checks service creation with correct ports
   - Validates deployment with correct image and replicas
   - Confirms volume mounts are configured

3. **Invalid Bootstrap Mode Handling**
   - Tests error handling
   - Verifies failed status
   - Checks error messages

4. **PVC Creation with Storage**
   - Tests storage configuration
   - Verifies PVC creation
   - Validates size, access mode, and storage class

**Unit Tests:**
1. **Core YAML Generation**
   - Tests typed configuration to YAML conversion
   - Verifies all configuration sections
   - Tests complex token configuration

2. **Helper Functions**
   - Tests secret name generation
   - Verifies image name construction
   - Checks default values

## Key Features

### Type Safety
- All core.yaml configuration is strongly typed
- Compile-time validation of configuration structure
- IntelliSense support in IDEs
- Prevents configuration errors at deployment time

### Flexibility
- Supports both file paths and secret references for certificates
- Configurable persistence (SQLite, PostgreSQL, etc.)
- Flexible endpoint resolver configuration
- Customizable token service settings
- Optional inline routing configuration via RawExtension

### Kubernetes Native
- Owner references for automatic cleanup
- Proper RBAC permissions
- Status tracking with conditions
- Support for storage classes and resource limits
- Integration with Istio for ingress

### Secret Management
- Certificates stored in Kubernetes secrets
- core.yaml configuration as a secret
- Automatic volume mounting into pods
- Support for both auto-enrollment and manual certificate management

## Usage

### 1. Generate CRD Manifests

```bash
make manifests
```

### 2. Generate Deep Copy Methods

```bash
make generate
```

### 3. Install CRDs

```bash
make install
```

### 4. Deploy the Operator

```bash
make deploy IMG=<your-registry>/fabric-x-operator:tag
```

### 5. Apply the Sample

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_endorser.yaml
```

## Controller Operation Flow

### Configure Mode
1. Parse EndorserSpec configuration
2. Convert API types to certs package types
3. Enroll with CA to obtain certificates
4. Store certificates in Kubernetes secret
5. Update status to RUNNING

### Deploy Mode
1. Execute Configure mode steps
2. Generate core.yaml from typed configuration
3. Create core config secret
4. Create PVC (if storage configured)
5. Create Kubernetes Service
6. Create Deployment with:
   - core.yaml mounted from secret
   - Certificates mounted from secret
   - Storage mounted from PVC (if configured)
   - Resource limits applied
   - Pod labels and annotations
7. Update status with endpoints

## Configuration Examples

### Minimal Configuration

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Endorser
metadata:
  name: endorser1
spec:
  bootstrapMode: deploy
  mspid: Org1MSP
  core:
    fsc:
      id: endorser1
      identity:
        cert:
          file: /var/hyperledger/fabric/keys/node.crt
        key:
          file: /var/hyperledger/fabric/keys/node.key
      p2p:
        listenAddress: /ip4/0.0.0.0/tcp/9301
```

### Full Configuration

See [config/samples/fabricx_v1alpha1_endorser.yaml](config/samples/fabricx_v1alpha1_endorser.yaml) for a complete example with:
- Logging configuration
- Multiple endpoint resolvers
- Fabric integration
- Token service
- CA enrollment
- Storage configuration
- Resource limits

## Testing

### Run All Tests

```bash
make test
```

### Run Specific Tests

```bash
go test ./internal/controller -v -ginkgo.focus="Endorser Controller"
```

### Run with Coverage

```bash
go test ./internal/controller -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Architecture Decisions

### 1. Type Conversion Layer
- API types in `v1alpha1` package for user-facing configuration
- Internal types in `certs` package for certificate provisioning
- Conversion functions in controller to bridge the two
- Allows independent evolution of API and implementation

### 2. Bootstrap Modes
- **configure**: Prepares resources without full deployment
- **deploy**: Complete operational deployment
- Allows staged rollout and dependency management

### 3. Secret Management
- Single secret for all certificates (sign + TLS)
- Separate secret for core.yaml configuration
- Automatic generation and updates
- Owner references ensure cleanup

### 4. RawExtension for Inline Config
- Used for flexible routing configuration
- Avoids strongly typing dynamic JSON
- Allows arbitrary configuration while maintaining validation

## Status Tracking

The controller maintains comprehensive status information:

```go
type EndorserStatus struct {
    Status               DeploymentStatus      // Overall status
    Message              string                // Human-readable message
    Conditions           []metav1.Condition    // Detailed conditions
    CoreConfigSecretName string                // Generated secret name
    CertificateSecrets   map[string]string     // Certificate secret mappings
    ServiceEndpoint      string                // Service FQDN
    P2PEndpoint          string                // P2P endpoint
}
```

## Troubleshooting

### Check Endorser Status

```bash
kubectl get endorser
kubectl describe endorser <name>
```

### View Logs

```bash
kubectl logs deployment/<endorser-name>
```

### Check Generated Secrets

```bash
kubectl get secret <endorser-name>-core-config -o yaml
kubectl get secret <endorser-name>-certs -o yaml
```

### Verify core.yaml Content

```bash
kubectl get secret <endorser-name>-core-config -o jsonpath='{.data.core\.yaml}' | base64 -d
```

## Future Enhancements

1. **Certificate Rotation**: Automatic renewal of expiring certificates
2. **Metrics Integration**: Prometheus metrics for monitoring
3. **Health Checks**: Liveness and readiness probes
4. **Multi-Replica Support**: Enhanced support for horizontal scaling
5. **Custom Storage Backends**: Support for additional persistence types
6. **Backup/Restore**: Automated backup and restore procedures

## Files Modified/Created

### Created Files
- `api/v1alpha1/endorser_types.go` - API type definitions
- `api/v1alpha1/endorser_helpers.go` - Helper functions
- `internal/controller/endorser_controller.go` - Controller implementation
- `internal/controller/endorser_controller_test.go` - Comprehensive tests
- `config/samples/fabricx_v1alpha1_endorser.yaml` - Sample configuration

### Modified Files
- `api/v1alpha1/zz_generated.deepcopy.go` - Auto-generated deep copy methods
- `config/crd/bases/fabricx.kfsoft.tech_endorsers.yaml` - Generated CRD

## Contributing

When modifying the Endorser controller:

1. Update API types in `endorser_types.go`
2. Run `make generate` to update deep copy methods
3. Run `make manifests` to update CRDs
4. Add/update tests in `endorser_controller_test.go`
5. Update sample YAML if needed
6. Run `make test` to verify all tests pass
7. Update this documentation

## References

- [Kubernetes Operator Pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
- [Kubebuilder Documentation](https://book.kubebuilder.io/)
- [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)
- [Fabric Smart Client](https://github.com/hyperledger-labs/fabric-smart-client)
- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
