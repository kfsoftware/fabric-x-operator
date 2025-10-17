# Fabric X Operator - Development Notes

This document contains important learnings and patterns discovered during development of the Fabric X Operator.

## Table of Contents

- [Endorser Custom Command and Args](#endorser-custom-command-and-args)
- [Volume Mounts for Data Directory](#volume-mounts-for-data-directory)
- [Operator Deployment Workflow](#operator-deployment-workflow)
- [Kubernetes MCP Integration](#kubernetes-mcp-integration)
- [Testing Patterns](#testing-patterns)

---

## Endorser Custom Command and Args

### Problem
Custom Docker images for endorsers (token-sdk-issuer, token-sdk-owner, token-sdk-endorser) require specific command and arguments that differ from the default Fabric Smart Client entrypoint.

### Solution
Added `command` and `args` fields to the `EndorserSpec` to allow full customization of container execution.

### Implementation

#### 1. API Changes ([api/v1alpha1/endorser_types.go](api/v1alpha1/endorser_types.go))

```go
type EndorserSpec struct {
    // ... other fields ...

    // Command to run in the container (overrides image's ENTRYPOINT)
    Command []string `json:"command,omitempty"`

    // Args to pass to the container (overrides image's CMD)
    Args []string `json:"args,omitempty"`
}
```

#### 2. Controller Changes ([internal/controller/endorser_controller.go](internal/controller/endorser_controller.go))

```go
Containers: []corev1.Container{
    {
        Name:    "endorser",
        Image:   endorser.GetFullImage(),
        Command: endorser.Spec.Command,  // Pass command from spec
        Args:    endorser.Spec.Args,     // Pass args from spec
        // ... rest of container spec
    },
}
```

#### 3. Sample Usage

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Endorser
metadata:
  name: org1-issuer
spec:
  image: token-sdk-issuer
  version: fabricx-latest
  command:
    - "app"
  args:
    - "--conf=/var/hyperledger/fabric/config"
    - "--port=9000"
```

### Dockerfile Context

This matches the Dockerfile pattern:

```dockerfile
FROM golang:1.25.2 AS builder
WORKDIR /go/src/app
ARG NODE_TYPE="owner"
ARG PLATFORM="fabricx"
# ... build steps ...
RUN cd "${NODE_TYPE}" && go build -tags "${PLATFORM}" -o /app

FROM busybox
EXPOSE 9000
WORKDIR /conf
COPY --from=builder /app /usr/bin/app
CMD ["app", "--conf", "/conf"]
```

### Testing

Added comprehensive unit tests in [internal/controller/endorser_controller_test.go](internal/controller/endorser_controller_test.go):

1. **Test: should configure custom command and args for the container**
   - Verifies command and args are properly set in deployment
   - Validates container spec includes both fields

2. **Test: should handle empty args field**
   - Ensures nil/empty command and args don't cause issues
   - Tests backward compatibility

3. **Test: should update deployment when command and args change**
   - Validates reconciliation updates deployment
   - Tests dynamic configuration changes

### CRD Update Required

After adding these fields, you must regenerate and apply CRDs:

```bash
make manifests
kubectl apply -f config/crd/bases/fabricx.kfsoft.tech_endorsers.yaml
```

---

## Volume Mounts for Data Directory

### Problem
The endorser application tries to create `/var/hyperledger/fabric/config/data` directory, but `/var/hyperledger/fabric/config` is mounted read-only from the core-config Secret.

**Error:**
```
error creating data directory /var/hyperledger/fabric/config/data:
mkdir /var/hyperledger/fabric/config/data: read-only file system
```

### Solution
Mount the same PVC to both `/var/hyperledger/fabric/data` (for backward compatibility) and `/var/hyperledger/fabric/config/data` (for the application's needs).

### Implementation

Using the existing `Common.volumeMounts` field without modifying the controller:

```yaml
spec:
  common:
    volumes:
      - name: data
        volumeSource:
          persistentVolumeClaim:
            claimName: org1-issuer-data
            readOnly: false

    volumeMounts:
      - name: data
        mountPath: /var/hyperledger/fabric/data
        readOnly: false
      - name: data  # Same volume mounted twice
        mountPath: /var/hyperledger/fabric/config/data
        readOnly: false
```

### Why This Works

1. **Same PVC, Multiple Mount Points**: Kubernetes allows the same volume to be mounted at multiple paths
2. **No Controller Changes**: Uses existing `Common` spec fields
3. **Both Paths Available**: Application can write to either location
4. **Backward Compatible**: Existing `/var/hyperledger/fabric/data` path still works

### All Samples Updated

All 6 endorser samples now include this dual mount pattern:
- `fabricx_v1alpha1_endorser_org1-issuer.yaml`
- `fabricx_v1alpha1_endorser_org1-auditor.yaml`
- `fabricx_v1alpha1_endorser_org1-owner1.yaml`
- `fabricx_v1alpha1_endorser_org1-owner2.yaml`
- `fabricx_v1alpha1_endorser_org1-endorser1.yaml`
- `fabricx_v1alpha1_endorser_org1-endorser2.yaml`

---

## Operator Deployment Workflow

### Building and Deploying New Operator Version

When you make changes to the operator code, use this workflow to deploy to your K3D cluster:

```bash
# Generate a unique image tag with timestamp
export IMAGE=local/fabric-x-operator:$(date +%Y%m%d%H%M%S)

# Build the Docker image
make docker-build IMG=$IMAGE

# Import image into K3D cluster
k3d image import $IMAGE --cluster k8s-hlf

# Deploy the operator
make deploy IMG=$IMAGE
```

### Why This Works

1. **Timestamp Tags**: Ensures each build is unique and avoids cache issues
2. **Local Registry**: Uses `local/` prefix for images that stay in the cluster
3. **K3D Import**: Loads image directly into cluster nodes (no registry needed)
4. **Automatic Update**: `make deploy` updates the operator deployment

### Workflow for Code Changes

```bash
# 1. Make code changes
vim internal/controller/endorser_controller.go

# 2. Update CRDs if needed
make manifests

# 3. Build and deploy
export IMAGE=local/fabric-x-operator:$(date +%Y%m%d%H%M%S)
make docker-build IMG=$IMAGE
k3d image import $IMAGE --cluster k8s-hlf
make deploy IMG=$IMAGE

# 4. Apply updated CRDs if schema changed
kubectl apply -f config/crd/bases/fabricx.kfsoft.tech_endorsers.yaml

# 5. Apply updated samples
kubectl apply -f config/samples/fabricx_v1alpha1_endorser_org1-issuer.yaml
```

---

## Kubernetes MCP Integration

The operator can be managed using Kubernetes MCP (Model Context Protocol) tools for declarative resource management.

### Available MCP Tools

#### Resource Management
- `mcp__kubernetes__resources_list` - List resources by apiVersion and kind
- `mcp__kubernetes__resources_get` - Get a specific resource
- `mcp__kubernetes__resources_create_or_update` - Apply YAML/JSON
- `mcp__kubernetes__resources_delete` - Delete a resource

#### Pod Management
- `mcp__kubernetes__pods_list` - List pods
- `mcp__kubernetes__pods_list_in_namespace` - List pods in namespace
- `mcp__kubernetes__pods_get` - Get pod details
- `mcp__kubernetes__pods_log` - Get pod logs
- `mcp__kubernetes__pods_exec` - Execute commands in pods

### Example: Deploying Endorsers with MCP

```bash
# List all endorsers
mcp__kubernetes__resources_list \
  --apiVersion fabricx.kfsoft.tech/v1alpha1 \
  --kind Endorser

# Get specific endorser
mcp__kubernetes__resources_get \
  --apiVersion fabricx.kfsoft.tech/v1alpha1 \
  --kind Endorser \
  --name org1-issuer \
  --namespace default

# Check endorser pods
mcp__kubernetes__pods_list \
  --labelSelector "app.kubernetes.io/part-of=fabric-x"
```

### Deploying Endorsers: Two-Phase Pattern

#### Phase 1: Configure Mode (Certificate Enrollment)

```bash
# Apply all endorsers in configure mode
kubectl apply -f config/samples/fabricx_v1alpha1_endorser_org1-issuer.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-auditor.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-owner1.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-owner2.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-endorser1.yaml \
               -f config/samples/fabricx_v1alpha1_endorser_org1-endorser2.yaml

# Verify certificates are created
kubectl get secrets | grep -E "org1-(issuer|auditor|owner1|owner2|endorser1|endorser2)-(sign|tls)-cert"

# Wait for all to be RUNNING
kubectl get endorsers
```

#### Phase 2: Deploy Mode (Application Deployment)

```bash
# Patch all endorsers to deploy mode
for node in issuer auditor owner1 owner2 endorser1 endorser2; do
  kubectl patch endorser org1-${node} --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
done

# Verify pods are created
kubectl get pods -l app.kubernetes.io/part-of=fabric-x

# Check logs
kubectl logs -l app=org1-issuer
```

---

## Testing Patterns

### Unit Testing

The operator uses Ginkgo and Gomega for testing. Tests are located in `internal/controller/*_test.go`.

### Running Tests

```bash
# Run all controller tests
go test ./internal/controller -v

# Run specific test focus
go test ./internal/controller -ginkgo.focus="Container Args Configuration" -v

# Run with timeout
go test ./internal/controller -timeout 120s -v
```

### Test Structure for New Features

When adding a new field like `command` and `args`, create tests that cover:

1. **Happy Path**: Feature works as expected
2. **Empty/Nil Values**: Handles missing configuration gracefully
3. **Updates**: Reconciliation updates deployment when values change

Example test pattern:

```go
Context("Container Args Configuration", func() {
    It("should configure custom command and args for the container", func() {
        // Create endorser with command and args
        // Verify deployment has correct values
    })

    It("should handle empty args field", func() {
        // Create endorser without command/args
        // Verify deployment handles nil/empty gracefully
    })

    It("should update deployment when command and args change", func() {
        // Create endorser with initial values
        // Update endorser with new values
        // Verify deployment is updated
    })
})
```

### Integration Testing

For testing with actual Kubernetes resources:

```bash
# Start kind/k3d cluster
k3d cluster create -p "80:30949@agent:0" -p "443:30950@agent:0" --agents 2 k8s-hlf

# Deploy operator
make deploy IMG=local/fabric-x-operator:latest

# Apply test samples
kubectl apply -f config/samples/

# Verify resources
kubectl get endorsers,pods,services
```

---

## Common Issues and Solutions

### Issue: CRD Schema Error

**Error:**
```
unknown field "spec.command"
```

**Solution:**
```bash
# Regenerate and apply CRDs
make manifests
kubectl apply -f config/crd/bases/fabricx.kfsoft.tech_endorsers.yaml
```

### Issue: ImagePullBackOff

**Error:**
```
Back-off pulling image "hyperledger/fabric-smart-client:latest"
```

**Solution:**
- Use custom images that exist: `token-sdk-issuer:fabricx-latest`
- Or build and import custom images into K3D cluster

### Issue: CrashLoopBackOff

**Common Causes:**
1. Missing required configuration
2. Certificate enrollment failed
3. Volume mount issues
4. Application configuration errors

**Debug:**
```bash
# Check pod logs
kubectl logs <pod-name>

# Check pod events
kubectl describe pod <pod-name>

# Check endorser status
kubectl describe endorser <endorser-name>
```

### Issue: Read-Only Filesystem

**Error:**
```
mkdir /var/hyperledger/fabric/config/data: read-only file system
```

**Solution:**
Add writable volume mount at the required path (see "Volume Mounts for Data Directory" section above).

---

## Development Checklist

When adding new features to the Endorser controller:

- [ ] Update `EndorserSpec` in `api/v1alpha1/endorser_types.go`
- [ ] Add field documentation with `//` comments
- [ ] Update controller logic in `internal/controller/endorser_controller.go`
- [ ] Run `make manifests` to regenerate CRDs
- [ ] Add unit tests in `internal/controller/endorser_controller_test.go`
- [ ] Update sample YAMLs in `config/samples/`
- [ ] Apply updated CRDs to cluster
- [ ] Test with actual deployment
- [ ] Update documentation (this file!)

---

## Useful Commands Reference

### Operator Development
```bash
# Build and deploy operator
export IMAGE=local/fabric-x-operator:$(date +%Y%m%d%H%M%S)
make docker-build IMG=$IMAGE
k3d image import $IMAGE --cluster k8s-hlf
make deploy IMG=$IMAGE

# Regenerate CRDs
make manifests

# Apply CRDs
kubectl apply -f config/crd/bases/fabricx.kfsoft.tech_endorsers.yaml
```

### Resource Management
```bash
# List all endorsers
kubectl get endorsers

# Get endorser YAML
kubectl get endorser org1-issuer -o yaml

# Patch endorser
kubectl patch endorser org1-issuer --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'

# Delete endorser
kubectl delete endorser org1-issuer
```

### Debugging
```bash
# Pod logs
kubectl logs -l app=org1-issuer
kubectl logs -l app=org1-issuer --previous  # Previous crash

# Pod status
kubectl get pods -l app.kubernetes.io/part-of=fabric-x
kubectl describe pod <pod-name>

# Exec into pod
kubectl exec -it <pod-name> -- /bin/sh

# Check secrets
kubectl get secrets | grep org1-issuer
kubectl get secret org1-issuer-core-config -o yaml
```

### Testing
```bash
# Run tests
go test ./internal/controller -v

# Run specific test
go test ./internal/controller -ginkgo.focus="Container Args" -v

# Run with coverage
go test ./internal/controller -cover -v
```

---

## Routing Configuration File

### Problem
The routing configuration needs to be available as a file named `routing-conf` in the `/var/hyperledger/fabric/config` directory with a specific YAML format mapping node names to their P2P addresses.

### Solution
Generate a `routing-conf` YAML file from the endpoint resolvers configuration in the Endorser spec.

### Required Format

```yaml
routes:
  issuer:
    - issuer.example.com:9101
  auditor:
    - auditor.example.com:9201
  endorser1:
    - endorser1.example.com:9301
  endorser2:
    - endorser2.example.com:9401
  owner1:
    - owner1.example.com:9501
  owner2:
    - owner2.example.com:9601
```

### Implementation

#### 1. Helper Method to Generate routing-conf ([api/v1alpha1/endorser_helpers.go](api/v1alpha1/endorser_helpers.go))

```go
// GenerateRoutingConfig generates the routing-conf YAML content from endpoint resolvers
func (e *Endorser) GenerateRoutingConfig() (string, error) {
    coreConfig := e.Spec.Core

    // Build routes map from endpoint resolvers
    routes := make(map[string][]string)

    // Check if endpoint resolvers exist
    if coreConfig.FSC.Endpoint != nil && len(coreConfig.FSC.Endpoint.Resolvers) > 0 {
        for _, resolver := range coreConfig.FSC.Endpoint.Resolvers {
            // Get P2P address from the resolver
            if p2pAddr, ok := resolver.Addresses["P2P"]; ok {
                // Create route entry: name -> [address]
                routes[resolver.Name] = []string{p2pAddr}
            }
        }
    }

    // Create routing config structure
    routingConfig := map[string]interface{}{
        "routes": routes,
    }

    // Marshal to YAML
    yamlBytes, err := yaml.Marshal(routingConfig)
    if err != nil {
        return "", fmt.Errorf("failed to marshal routing config to YAML: %w", err)
    }

    return string(yamlBytes), nil
}
```

#### 2. Source: Endpoint Resolvers in Endorser Spec

The routing configuration is built from the `endpoint.resolvers` section:

```yaml
spec:
  core:
    fsc:
      endpoint:
        resolvers:
          - name: auditor
            addresses:
              P2P: auditor.example.com:9201
          - name: endorser1
            addresses:
              P2P: endorser1.example.com:9301
```

Each resolver's `P2P` address becomes a route entry.

#### 3. Update Core.yaml to Reference routing-conf

Changed from embedding inline routing to referencing the file:

```go
// Reference the routing-conf file
routing["path"] = "/var/hyperledger/fabric/config/routing-conf"
```

#### 4. Update Secret to Include routing-conf ([internal/controller/endorser_controller.go](internal/controller/endorser_controller.go))

```go
func (r *EndorserReconciler) reconcileCoreConfigSecret(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
    // Generate core.yaml content
    coreYAML, err := endorser.GenerateCoreYAML()
    if err != nil {
        return fmt.Errorf("failed to generate core.yaml: %w", err)
    }

    // Generate routing config YAML from endpoint resolvers
    routingConfig, err := endorser.GenerateRoutingConfig()
    if err != nil {
        return fmt.Errorf("failed to generate routing config: %w", err)
    }

    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      secretName,
            Namespace: endorser.Namespace,
        },
        Type: corev1.SecretTypeOpaque,
        Data: map[string][]byte{
            "core.yaml":    []byte(coreYAML),
            "routing-conf": []byte(routingConfig),  // Added routing-conf
        },
    }
    // ... rest of secret creation
}
```

### Result

The core-config Secret now contains two files:

1. **core.yaml**: Main configuration with routing reference
   ```yaml
   fsc:
     p2p:
       opts:
         routing:
           path: /var/hyperledger/fabric/config/routing-conf
   ```

2. **routing-conf**: Routing configuration (YAML)
   ```yaml
   routes:
     auditor:
     - auditor.example.com:9201
     endorser1:
     - endorser1.example.com:9301
     endorser2:
     - endorser2.example.com:9401
     issuer:
     - issuer.example.com:9101
     owner1:
     - owner1.example.com:9501
     owner2:
     - owner2.example.com:9601
   ```

### Benefits

1. **File-based Configuration**: Application loads routing from file
2. **User-Defined Routes**: Routes come from endpoint resolvers (user-configurable)
3. **Cleaner Separation**: Routing config separated from core config
4. **Easier Debugging**: Can inspect routing-conf separately
5. **No Inline Parsing**: Avoids JSON parsing issues

### Customizing Routes

Users can customize routes by modifying the `addresses.P2P` field in their endorser YAML:

```yaml
spec:
  core:
    fsc:
      endpoint:
        resolvers:
          - name: auditor
            addresses:
              P2P: custom-auditor.domain.com:9999  # Custom route
```

### Verification

After deploying, you can verify the files in the secret:

```bash
# View secret contents
kubectl get secret org1-issuer-core-config -o yaml

# Decode routing-conf
kubectl get secret org1-issuer-core-config -o jsonpath='{.data.routing-conf}' | base64 -d

# Decode core.yaml
kubectl get secret org1-issuer-core-config -o jsonpath='{.data.core\.yaml}' | base64 -d
```

Both files are mounted at `/var/hyperledger/fabric/config/` in the endorser pod.

---

## Architecture Notes

### Endorser Component Structure

```
Endorser Custom Resource
    ↓
EndorserReconciler (Controller)
    ↓
Creates/Updates:
    - Core Config Secret (core.yaml)
    - Certificate Secrets (sign + TLS)
    - Deployment (endorser pod)
    - Service (P2P + metrics)
    - Istio VirtualService (ingress)
```

### Two-Phase Bootstrap Pattern

**Configure Mode:**
- Enrolls certificates with CA
- Creates secrets
- Does NOT create deployment

**Deploy Mode:**
- Creates deployment
- Mounts all secrets
- Starts application pod

This pattern ensures all certificates exist before cross-referencing between endorsers.

### Volume Mount Strategy

The endorser pod has multiple volume mounts:

1. **Core Config**: `/var/hyperledger/fabric/config` (read-only Secret)
2. **Sign Cert**: `/var/hyperledger/fabric/msp/signcerts/cert.pem` (read-only Secret)
3. **Sign Key**: `/var/hyperledger/fabric/msp/keystore/key.pem` (read-only Secret)
4. **TLS Cert**: `/var/hyperledger/fabric/tls` (read-only Secret)
5. **Resolver Certs**: `/var/hyperledger/fabric/resolvers/{node-name}` (read-only Secrets)
6. **Data**: `/var/hyperledger/fabric/data` (read-write PVC)
7. **Config Data**: `/var/hyperledger/fabric/config/data` (read-write PVC, same volume as #6)

---

## Resources

- [Kubernetes Operator Development](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
- [K3D Documentation](https://k3d.io/)
- [Fabric Smart Client](https://github.com/hyperledger-labs/fabric-smart-client)

---

## Contributors

This document captures learnings from active development. Keep it updated as new patterns and solutions emerge!

Last Updated: 2025-10-17
