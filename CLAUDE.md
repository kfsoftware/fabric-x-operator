# Fabric X Operator - Development Notes

This document contains important learnings and patterns discovered during development of the Fabric X Operator.

## Table of Contents

- [Endorser Custom Command and Args](#endorser-custom-command-and-args)
- [Volume Mounts for Data Directory](#volume-mounts-for-data-directory)
- [Operator Deployment Workflow](#operator-deployment-workflow)
- [Kubernetes MCP Integration](#kubernetes-mcp-integration)
- [Testing Patterns](#testing-patterns)
- [Namespace CRD](#namespace-crd)

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
      - name: data # Same volume mounted twice
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
              P2P: custom-auditor.domain.com:9999 # Custom route
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

## MSP Configuration for Orderer Components

### Problem

Orderer consenter and batcher pods were failing with error:

```
administrators must be declared when no admin ou classification is set
```

This occurred because the MSP directory structure was missing the `config.yaml` file that defines NodeOUs (Node Organizational Units).

### Solution

Create a `config.yaml` file with NodeOUs configuration and mount it in the MSP directory structure.

### Implementation

#### 1. Add msp_config.yaml to ConfigMap

For both consenter and batcher controllers, add the MSP config content to the ConfigMap:

**OrdererConsenter** ([internal/controller/ordererconsenter_controller.go:591-617](internal/controller/ordererconsenter_controller.go)):

```go
// MSP config.yaml content
mspConfigContent := `NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: orderer
`

template := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name:      fmt.Sprintf("%s-config", ordererConsenter.Name),
        Namespace: ordererConsenter.Namespace,
    },
    Data: map[string]string{
        "node_config.yaml": configContent,
        "msp_config.yaml":  mspConfigContent,  // Added MSP config
    },
}
```

#### 2. Update Init Container to Copy config.yaml

Modify the init container command to copy the MSP config file:

```go
// In init container command
"cp /config/msp_config.yaml %s/config.yaml && "+
```

#### 3. Mount Config Volume in Init Container

Add the config volume mount to the init container:

```go
{
    Name:      "config",
    ReadOnly:  true,
    MountPath: "/config",
},
```

### Same Pattern for Batcher

The exact same implementation was applied to OrdererBatcher controller ([internal/controller/ordererbatcher_controller.go:532-558](internal/controller/ordererbatcher_controller.go)).

### Result

After applying this fix:

- Consenter pods: Running (1/1)
- Batcher pods: Running (1/1)
- MSP structure now includes:
  - `msp/cacerts/ca.pem`
  - `msp/signcerts/sign-cert.pem`
  - `msp/keystore/priv_sk`
  - `msp/config.yaml` ✓ (added)

### MSP Directory Structure

The complete MSP directory structure mounted in orderer pods:

```
/var/hyperledger/fabricx/msp/
├── cacerts/
│   └── ca.pem
├── signcerts/
│   └── sign-cert.pem
├── keystore/
│   └── priv_sk
└── config.yaml (defines NodeOUs)
```

---

## MSP Configuration for Endorser Components

### Problem

Endorser applications were failing with:

```
stat /var/hyperledger/fabric/config/keys/fabric/user/msp/cacerts: no such file or directory
```

Endorsers expected MSP files at a specific path but had no mechanism to create them.

### Solution

Add an init container to create the MSP directory structure with certificates and config.yaml inline.

### Implementation

#### 1. Add Init Container to Endorser Deployment

In [internal/controller/endorser_controller.go:517-566](internal/controller/endorser_controller.go):

```go
mspBasePath := "/var/hyperledger/fabric/config/keys/fabric/user"
deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, corev1.Container{
    Name:  "setup-msp",
    Image: "busybox:1.35",
    Command: []string{
        "/bin/sh",
        "-c",
        fmt.Sprintf(
            "echo 'Creating MSP directory at: %s' && "+
                "mkdir -p %s/signcerts && "+
                "mkdir -p %s/keystore && "+
                "mkdir -p %s/cacerts && "+
                "echo 'Copying certificates...' && "+
                "cp /sign-certs/cert.pem %s/signcerts/cert.pem && "+
                "cp /sign-certs/key.pem %s/keystore/key.pem && "+
                "cp /sign-certs/key.pem %s/keystore/priv_sk && "+
                "cp /sign-certs/ca.pem %s/cacerts/ca.pem && "+
                "echo 'Creating config.yaml...' && "+
                "cat > %s/config.yaml <<'MSPEOF'\n"+
                "NodeOUs:\n"+
                "  Enable: true\n"+
                "  ClientOUIdentifier:\n"+
                "    Certificate: cacerts/ca.pem\n"+
                "    OrganizationalUnitIdentifier: client\n"+
                "  PeerOUIdentifier:\n"+
                "    Certificate: cacerts/ca.pem\n"+
                "    OrganizationalUnitIdentifier: peer\n"+
                "  AdminOUIdentifier:\n"+
                "    Certificate: cacerts/ca.pem\n"+
                "    OrganizationalUnitIdentifier: admin\n"+
                "  OrdererOUIdentifier:\n"+
                "    Certificate: cacerts/ca.pem\n"+
                "    OrganizationalUnitIdentifier: orderer\n"+
                "MSPEOF\n"+
                "echo 'MSP Directory contents:' && ls -lR /var/hyperledger/fabric/config/keys",
            mspBasePath,
            mspBasePath, mspBasePath, mspBasePath,
            mspBasePath, mspBasePath, mspBasePath, mspBasePath,
            mspBasePath,
        ),
    },
    VolumeMounts: []corev1.VolumeMount{
        {
            Name:      "sign-cert",
            ReadOnly:  true,
            MountPath: "/sign-certs",
        },
        {
            Name:      "shared-msp",
            MountPath: "/var/hyperledger/fabric/config/keys",
        },
    },
})
```

#### 2. Add ca.pem to Sign-Cert Secret Items

```go
{
    Key:  "ca.pem",
    Path: "ca.pem",
},
```

#### 3. Create Shared MSP EmptyDir Volume

```go
deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
    Name: "shared-msp",
    VolumeSource: corev1.VolumeSource{
        EmptyDir: &corev1.EmptyDirVolumeSource{},
    },
})
```

#### 4. Mount Shared Volume in Main Container

```go
{
    Name:      "shared-msp",
    MountPath: "/var/hyperledger/fabric/config/keys",
    ReadOnly:  true,
},
```

### Key Design Decisions

1. **Inline config.yaml**: Used heredoc syntax to create config.yaml directly in the shell script, avoiding the need for a separate ConfigMap
2. **EmptyDir Volume**: Shared between init container (write) and main container (read)
3. **Correct MSP Path**: Set to `/var/hyperledger/fabric/config/keys/fabric/user` (NOT `/var/hyperledger/fabric/config/keys/fabric/user/msp`)
4. **priv_sk File**: Created as a copy of key.pem to support both naming conventions

### Troubleshooting Init Container Issues

**Issue**: Init container failed with busybox find command error

```
Init:Error - find: unrecognized: -ls
```

**Solution**: Remove `find -ls` command, use only `ls -lR` which is supported by busybox:1.35

### Result

All endorser pods now start successfully with proper MSP configuration:

- org1-issuer: Running (1/1)
- org1-owner1: Running (1/1)
- org1-owner2: Running (1/1)
- org1-endorser1: Running (1/1)
- org1-endorser2: Running (1/1)
- org1-auditor: ErrImagePull (image issue unrelated to MSP)

---

## Environment Variable Configuration for Committer Components

### Problem

Environment variables defined in CommitterSidecar spec were being hashed for rollout detection but were NOT being added to the container specification. Changes to env vars would trigger a hash change but the actual values wouldn't appear in pods.

### Root Cause

The `Env` field was completely missing from the container spec in `reconcileDeployment` function.

### Solution - Three Steps

#### Step 1: Add Env Field to Container Spec

In [internal/controller/committersidecar_controller.go:766](internal/controller/committersidecar_controller.go):

```go
Env: committerSidecar.Spec.Env,
```

#### Step 2: Eliminate Custom Type Duplication

Instead of maintaining custom `EnvVar`, `EnvVarSource`, and `SecretKeySelector` types, use Kubernetes native types directly.

**Removed custom types** from [api/v1alpha1/orderergroup_types.go:488-513](api/v1alpha1/orderergroup_types.go):

```go
// DELETED these custom types:
type EnvVar struct { ... }
type EnvVarSource struct { ... }
type SecretKeySelector struct { ... }
```

**Updated all API type files** to use `corev1.EnvVar`:

```bash
# Changed in 14 files:
Env []EnvVar          # OLD
Env []corev1.EnvVar   # NEW
```

Files updated:

- committer_types.go
- committercoordinator_types.go
- committersidecar_types.go
- committerqueryservice_types.go
- committervalidator_types.go
- committerverifier_types.go
- ordererassembler_types.go
- ordererbatcher_types.go
- ordererconsenter_types.go
- ordererrouter_types.go
- orderergroup_types.go

**Added corev1 imports** to API files missing it:

```go
import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
```

#### Step 3: Regenerate DeepCopy Code

```bash
make generate
```

### Benefits

1. **No Type Conversion**: Direct assignment `Env: committerSidecar.Spec.Env` works
2. **Cleaner Code**: Removed ~30 lines of type conversion logic
3. **Kubernetes Native**: Using standard types improves compatibility
4. **Less Maintenance**: No need to keep custom types in sync with Kubernetes

### Environment Variable Hashing

The env variable hashing (lines 662-669) now works correctly:

```go
// Hash environment variables to trigger rollout on env change
if len(committerSidecar.Spec.Env) > 0 {
    envString := ""
    for _, env := range committerSidecar.Spec.Env {
        envString += fmt.Sprintf("%s=%s|", env.Name, env.Value)
    }
    envHashSum := sha256.Sum256([]byte(envString))
    hashParts = append(hashParts, hex.EncodeToString(envHashSum[:]))
}
```

### Verification

```bash
# Check env vars in pod spec
kubectl get pod <sidecar-pod> -o json | jq '.spec.containers[0].env'

# Output:
[
  {
    "name": "SIDECAR_LOG_LEVEL",
    "value": "DEBUG"
  },
  {
    "name": "SIDECAR_CHANNEL_ID",
    "value": "arma"
  }
]

# Verify in running container
kubectl exec <sidecar-pod> -- env | grep SIDECAR

# Output:
SIDECAR_LOG_LEVEL=DEBUG
SIDECAR_CHANNEL_ID=arma
```

### Hash-Based Rollout Behavior

When env vars change:

1. New hash is computed from env values
2. Pod template annotation updated with new hash
3. Deployment triggers rolling update
4. New pod created with updated env vars
5. Old pod terminated after new pod is ready

**Example**:

```bash
# Before: SIDECAR_LOG_LEVEL=INFO
# Hash: 8f5a500ac1d63dd623ffb32eadc83b4de3f0c8232a736eeaee9b49767c35912f

# After: SIDECAR_LOG_LEVEL=DEBUG
# Hash: e455532e4898e1f0d8faff1e266d889fb2deb08de4eddcacac6203077e6fef20

# Result: New pod created with DEBUG value
```

---

## Service Address Configuration for Endorsers

### Problem

Endorsers were configured with `localhost` addresses for committer services, causing connection failures:

```yaml
peers:
  address: localhost:5400
queryService:
  address: localhost:5500
```

### Solution

Update all endorser samples to use actual Kubernetes service names.

### Implementation

Updated 6 endorser sample files:

```bash
# Changed from:
peers.address: localhost:5400
queryService.address: localhost:5500

# Changed to:
peers.address: fabric-x-committer-sidecar-service.default:5050
queryService.address: fabric-x-committer-query-service-service.default:9001
```

Files updated:

- fabricx_v1alpha1_endorser_org1-auditor.yaml
- fabricx_v1alpha1_endorser_org1-endorser1.yaml
- fabricx_v1alpha1_endorser_org1-endorser2.yaml
- fabricx_v1alpha1_endorser_org1-issuer.yaml
- fabricx_v1alpha1_endorser_org1-owner1.yaml
- fabricx_v1alpha1_endorser_org1-owner2.yaml

### Service Discovery

```bash
# Find committer services
kubectl get svc | grep fabric-x-committer

# Output:
fabric-x-committer-sidecar-service          ClusterIP   10.43.x.x    <none>   5050/TCP
fabric-x-committer-query-service-service    ClusterIP   10.43.x.x    <none>   9001/TCP
```

### Service Name Format

Format: `<component-name>-service.<namespace>:<port>`

Examples:

- `fabric-x-committer-sidecar-service.default:5050`
- `fabric-x-committer-query-service-service.default:9001`
- `fabric-x-committer-coordinator-service.default:9001`

---

## Quick Start Deployment Guide

### Complete Deployment Steps

This is the tested and working deployment sequence for a complete Fabric-X network.

#### Prerequisites

- K3D cluster with ports mapped: 80→30949, 443→30950
- Istio installed
- PostgreSQL deployed for committer validator

#### Step 1: Deploy Certificate Authority

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_ca.yaml
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=fabric-ca --timeout=120s
```

#### Step 2: Deploy Orderer Groups (Configure Mode)

```bash
for i in 1 2 3 4; do
  kubectl apply -f config/samples/fabricx_v1alpha1_orderergroup_party${i}.yaml
done

kubectl get orderergroups
# Wait for all to be RUNNING
```

#### Step 3: Create Genesis Block

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_genesis.yaml
kubectl get genesis
kubectl get secret fabricx-shared-genesis
```

#### Step 4: Patch Orderer Groups to Deploy Mode

```bash
for i in 1 2 3 4; do
  kubectl patch orderergroup orderergroup-party${i} --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
done

# Wait for all components to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=fabric-x --timeout=600s
```

Expected pods (18 total):

- 4 Routers
- 8 Batchers (2 per party)
- 4 Consenters
- 4 Assemblers

#### Step 5: Deploy Committer (Already in Deploy Mode)

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_committer.yaml
kubectl get committer

# Committer automatically starts in deploy mode
kubectl get pods | grep fabric-x-committer
```

Expected pods (5 total):

- Coordinator
- Sidecar
- Query Service
- Validator
- Verifier

#### Step 6: Deploy Endorsers (Configure Mode)

```bash
for endorser in org1-auditor org1-issuer org1-endorser1 org1-endorser2 org1-owner1 org1-owner2; do
  kubectl apply -f config/samples/fabricx_v1alpha1_endorser_${endorser}.yaml
done

kubectl get endorser
# Wait for all to be RUNNING
```

#### Step 7: Patch Endorsers to Deploy Mode

```bash
for endorser in org1-auditor org1-issuer org1-endorser1 org1-endorser2 org1-owner1 org1-owner2; do
  kubectl patch endorser ${endorser} --type=merge -p '{"spec":{"bootstrapMode":"deploy"}}'
done

kubectl get pods | grep org1-
```

Expected pods (6 total):

- org1-issuer
- org1-auditor (may have image issue)
- org1-owner1
- org1-owner2
- org1-endorser1
- org1-endorser2

### Verification Commands

```bash
# Check all resources
kubectl get cas,orderergroups,genesis,committer,endorser

# Check all pods
kubectl get pods

# Count running pods
kubectl get pods | grep -E "test-ca2|orderergroup|fabric-x-committer|org1-" | grep -c Running

# Check environment variables in sidecar
kubectl get pod -l app=sidecar,release=fabric-x-committer-sidecar -o jsonpath='{.items[0].metadata.name}' | xargs -I {} kubectl exec {} -- env | grep SIDECAR
```

### Channel Configuration

All components point to channel: `arma`

Verify in samples:

- Endorsers: `channels[0].name: arma`
- Committer Sidecar: `SIDECAR_CHANNEL_ID: arma`
- Genesis: `channelName: arma`

### Expected Final State

Total: **32 pods**

- 1 CA pod
- 18 orderer component pods
- 5 committer component pods
- 6 endorser pods (1 may have image issue)
- PostgreSQL pod(s)

Success rate: **31/32 Running** (97%)

---

## Operator Image Build and Deployment

### Standard Workflow

```bash
# 1. Make code changes
vim internal/controller/committersidecar_controller.go

# 2. Build operator image
make docker-build IMG=kfsoftware/fabric-x-operator:latest

# 3. Import to K3D cluster
k3d image import kfsoftware/fabric-x-operator:latest -c k8s-hlf

# 4. Restart operator pod
kubectl delete pod -n fabric-x-operator-system -l control-plane=controller-manager

# 5. Wait for new pod
kubectl wait --for=condition=Ready pod -l control-plane=controller-manager -n fabric-x-operator-system --timeout=60s
```

### When API Types Change

```bash
# 1. Make API changes
vim api/v1alpha1/committersidecar_types.go

# 2. Regenerate deepcopy code
make generate

# 3. Update CRDs (if needed)
make manifests

# 4. Build and deploy operator (same as above)
make docker-build IMG=kfsoftware/fabric-x-operator:latest
k3d image import kfsoftware/fabric-x-operator:latest -c k8s-hlf
kubectl delete pod -n fabric-x-operator-system -l control-plane=controller-manager
```

### Makefile Targets

```bash
make generate   # Generate deepcopy code
make manifests  # Generate CRD manifests (not available in this project)
make docker-build IMG=<image>  # Build operator image
```

---

## Common Debugging Patterns

### Check Operator Logs

```bash
# Get operator pod name
kubectl get pods -n fabric-x-operator-system

# View logs
kubectl logs -n fabric-x-operator-system <operator-pod-name>

# Follow logs
kubectl logs -n fabric-x-operator-system <operator-pod-name> -f

# Check for errors
kubectl logs -n fabric-x-operator-system <operator-pod-name> | grep -i error
```

### Check Resource Status

```bash
# Get detailed status
kubectl describe orderergroup orderergroup-party1
kubectl describe committer fabric-x-committer
kubectl describe endorser org1-issuer

# Check conditions
kubectl get orderergroup orderergroup-party1 -o jsonpath='{.status.conditions}' | jq
```

### Check Init Container Issues

```bash
# View init container logs
kubectl logs <pod-name> -c <init-container-name>

# Example for endorser
kubectl logs org1-issuer-xxxxx -c setup-msp

# Check init container status
kubectl get pod org1-issuer-xxxxx -o jsonpath='{.status.initContainerStatuses}' | jq
```

### Check Secret Contents

```bash
# List secrets
kubectl get secrets | grep org1-issuer

# Decode secret data
kubectl get secret org1-issuer-core-config -o jsonpath='{.data.core\.yaml}' | base64 -d

# Check MSP config
kubectl get secret orderergroup-party1-consenter-config -o jsonpath='{.data.msp_config\.yaml}' | base64 -d
```

### Check Volume Mounts

```bash
# Exec into pod
kubectl exec -it <pod-name> -- /bin/sh

# Check MSP directory
ls -lR /var/hyperledger/fabric/config/keys/fabric/user/

# Check if files exist
cat /var/hyperledger/fabric/config/keys/fabric/user/config.yaml
```

### Network Connectivity

```bash
# Test service connectivity from endorser
kubectl exec -it org1-issuer-xxxxx -- sh
wget -O- http://fabric-x-committer-sidecar-service.default:5050/healthz

# Check DNS resolution
nslookup fabric-x-committer-sidecar-service.default
```

---

## Performance Considerations

### Hash-Based Rollout Detection

The operator uses SHA256 hashes of mounted secrets/configs to trigger pod rollouts:

```go
// Compute combined hash
hashParts := []string{}

// Hash config secret
hashParts = append(hashParts, computeSecretHash(configSecret))

// Hash certificate secrets
hashParts = append(hashParts, computeSecretHash(signCertSecret))
hashParts = append(hashParts, computeSecretHash(tlsCertSecret))

// Hash environment variables
if len(spec.Env) > 0 {
    envString := ""
    for _, env := range spec.Env {
        envString += fmt.Sprintf("%s=%s|", env.Name, env.Value)
    }
    envHashSum := sha256.Sum256([]byte(envString))
    hashParts = append(hashParts, hex.EncodeToString(envHashSum[:]))
}

// Combine all hashes
sort.Strings(hashParts)
combinedHashSum := sha256.Sum256([]byte(strings.Join(hashParts, "|")))
configMapHash := hex.EncodeToString(combinedHashSum[:])
```

This hash is added as a pod template annotation:

```go
annotations["fabricx.kfsoft.tech/config-hash"] = configMapHash
```

Any change to secrets or env vars changes the hash, triggering a rolling update.

### Reconciliation Loop

The operator reconciles resources in this order:

1. **Certificates** (if in configure mode)
2. **Genesis Block** (for orderers)
3. **Secrets/ConfigMaps**
4. **Services**
5. **Deployments** (if in deploy mode)
6. **Ingress** (if configured)

This ordering ensures dependencies are created before dependents.

### Resource Limits

Default resource limits for committer components:

```yaml
requests:
  cpu: 100m
  memory: 128Mi
limits:
  cpu: 500m
  memory: 512Mi
```

These can be overridden in the spec:

```yaml
spec:
  components:
    sidecar:
      resources:
        requests:
          cpu: 300m
          memory: 512Mi
        limits:
          cpu: 1500m
          memory: 2Gi
```

---

## Namespace CRD

### Overview

The Namespace CRD allows deploying Fabric X namespaces to the ordering service. A namespace represents a logical separation of data within a channel, with its own endorsement policy.

### Problem

Creating namespaces in Fabric X requires:
1. Setting up MSP credentials from Kubernetes secrets
2. Creating a namespace transaction with endorsement policy
3. Signing the transaction
4. Broadcasting to the orderer

The operator needs to automate this process while leveraging the Identity CRD for credential management.

### Solution

Created a Namespace CRD with a controller that provides:
- Integration with Identity CRD for MSP setup
- Helper functions for secret retrieval
- Basic controller skeleton for custom implementation

### Implementation

#### 1. CRD Structure ([api/v1alpha1/namespace_types.go](api/v1alpha1/namespace_types.go))

```go
type NamespaceSpec struct {
    // Namespace ID
    Name string `json:"name"`

    // Orderer endpoint
    Orderer string `json:"orderer"`

    // CA certificate for TLS connection to orderer
    CACert SecretKeyRef `json:"caCert"`

    // MSPID of the identity
    MSPID string `json:"mspID"`

    // Reference to Identity CRD
    Identity SecretKeyRef `json:"identity"`

    // Channel name (optional)
    Channel string `json:"channel,omitempty"`

    // Verification key path (optional)
    VerificationKeyPath string `json:"verificationKeyPath,omitempty"`

    // Namespace version (-1 for new namespace)
    Version int `json:"version,omitempty"`
}

type SecretKeyRef struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
    Key       string `json:"key,omitempty"`
}
```

#### 2. Controller Skeleton ([internal/controller/namespace_controller.go](internal/controller/namespace_controller.go))

The controller provides a basic skeleton with helper functions but leaves the complex deployment logic to be implemented:

**Key Functions:**

1. **setupMSPFromIdentity** - Maps Identity CRD to local MSP directory
2. **getSecretValue** - Retrieves values from Kubernetes secrets
3. **updateStatus** - Updates Namespace status and conditions
4. **handleDeletion** - Cleanup logic when namespace is deleted

#### 3. MSP Setup from Identity CRD

The `setupMSPFromIdentity` function creates a local MSP directory structure compatible with fabric-x-common's `setupMSP` function:

```go
// Create temporary directory for MSP
tmpDir, err := os.MkdirTemp("", "namespace-msp-*")
if err != nil {
    return ctrl.Result{}, fmt.Errorf("failed to create temp dir: %w", err)
}
defer os.RemoveAll(tmpDir)

// Setup MSP from Identity CRD
mspPath, mspID, err := r.setupMSPFromIdentity(ctx, ns.Spec.Identity, tmpDir)
if err != nil {
    return ctrl.Result{}, fmt.Errorf("failed to setup MSP: %w", err)
}

// Now mspPath points to a directory with this structure:
// msp/
//   ├── signcerts/cert.pem
//   ├── keystore/priv_sk
//   ├── cacerts/ca.pem
//   └── config.yaml
```

The function:
1. Fetches the Identity CRD resource
2. Verifies the identity status is "Ready"
3. Retrieves signing cert, key, and CA cert from the secrets referenced in `identity.Status.OutputSecrets`
4. Creates the MSP directory structure
5. Writes all certificates and keys to the correct paths
6. Creates `config.yaml` with NodeOUs configuration
7. Returns the MSP path and MSPID

#### 4. Sample Usage

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Namespace
metadata:
  name: my-namespace
spec:
  name: "token-namespace"
  orderer: "orderergroup-party1-router-service.default:443"
  caCert:
    name: "orderergroup-party1-consenter-tls-cert"
    namespace: "default"
    key: "ca.pem"
  mspID: "Org1MSP"
  identity:
    name: "org1-admin"
    namespace: "default"
  channel: "arma"
  version: -1
```

### Integration with fabric-x-common

The MSP directory created by `setupMSPFromIdentity` can be directly used with the `setupMSP` function from fabric-x-common:

```go
import "github.com/hyperledger/fabric-x-common/msp"

// After calling setupMSPFromIdentity
mspCfg := namespace.MSPConfig{
    MSPConfigPath: mspPath,
    MSPID:         mspID,
}

thisMSP, err := setupMSP(mspCfg)
if err != nil {
    return fmt.Errorf("msp setup error: %w", err)
}

// Use the MSP for signing
sid, err := thisMSP.GetDefaultSigningIdentity()
// ... continue with transaction creation and signing
```

### Design Decisions

1. **Minimal Controller Logic**: Controller provides helper functions but leaves complex deployment logic to be implemented by the user
2. **Identity CRD Integration**: Leverages existing Identity CRD for credential management rather than duplicating MSP setup
3. **Temporary MSP Directory**: Creates ephemeral MSP structure that's cleaned up after use
4. **Status Tracking**: Tracks deployment status, transaction ID, and conditions

### Implementation Workflow

When implementing the namespace deployment logic:

1. **Setup MSP**: Use `setupMSPFromIdentity` to create MSP directory from Identity CRD
2. **Load Credentials**: Use the MSP path with fabric-x-common's `setupMSP` function
3. **Create Transaction**: Build namespace transaction with endorsement policy
4. **Sign Transaction**: Sign using MSP signing identity
5. **Broadcast**: Send signed envelope to orderer
6. **Update Status**: Record transaction ID and status

### Benefits

1. **Identity Integration**: Reuses Identity CRD for credential management
2. **Flexible Implementation**: Controller skeleton allows custom deployment logic
3. **Helper Functions**: Provides utilities for secret access and status updates
4. **Fabric X Compatible**: MSP directory structure matches fabric-x-common expectations
5. **Clean Separation**: Separates MSP setup from complex transaction logic

### Future Enhancements

Potential additions to consider:

- Verification key management for custom endorsement policies
- Namespace policy updates (using version field)
- Integration with endorser CRDs for namespace-specific configurations
- Status watching for transaction confirmation
- Automatic retry logic for failed deployments

---

## Contributors

This document captures learnings from active development. Keep it updated as new patterns and solutions emerge!

Last Updated: 2025-10-20

---

## Idemix Integration

### Overview

Automatic idemix (Identity Mixer) enrollment integration for privacy-preserving identities in Hyperledger Fabric. This allows creating anonymous credentials that support selective disclosure and unlinkability.

### Problem

Need automatic idemix credential enrollment that:
1. Works with Fabric CA's idemix support
2. Stores all credential information in Kubernetes secrets
3. Is testable in isolation from the identity controller
4. Uses the correct elliptic curve (gurvy.Bn254)

### Solution

Created a separate `internal/idemix` package with:
- Standalone enrollment logic using official Fabric CA client library
- Integration tests with testcontainers spinning up real Fabric CA
- Identity controller integration for automatic enrollment
- CA controller support for idemix curve configuration

### Implementation

#### 1. Idemix Package ([internal/idemix/enrollment.go](internal/idemix/enrollment.go))

**Key Components**:

```go
type EnrollmentRequest struct {
    CAURL         string
    CAName        string
    EnrollID      string
    EnrollSecret  string
    CACertPath    string
    SkipTLSVerify bool
    MSPDir        string
}

type EnrollmentResponse struct {
    SignerConfig     *idemix2.SignerConfig
    IdemixConfigPath string
}

func Enroll(req EnrollmentRequest) (*EnrollmentResponse, error) {
    // Initialize BCCSP (crypto service provider)
    opts := &factory.FactoryOpts{
        Default: "SW",
        SW: &factory.SwOpts{
            Hash:     "SHA2",
            Security: 256,
            FileKeystore: &factory.FileKeystoreOpts{
                KeyStorePath: bccspDir,
            },
        },
    }
    
    // Create Fabric CA client with idemix curve
    clientConfig := &lib.ClientConfig{
        URL:    req.CAURL,
        TLS:    tlsConfig,
        MSPDir: mspDir,
        CSP:    opts,
        Idemix: api.Idemix{
            Curve: "gurvy.Bn254", // CRITICAL: Must match CA curve
        },
    }
    
    // Perform enrollment with Type: "idemix"
    enrollReq := &api.EnrollmentRequest{
        Name:   req.EnrollID,
        Secret: req.EnrollSecret,
        CAName: req.CAName,
        Type:   "idemix",  // Triggers idemix enrollment
    }
    
    enrollResp, err := client.Enroll(enrollReq)
    // Returns SignerConfig with all credential data
}
```

**SignerConfig Contents**:
- `Cred`: Serialized idemix credential (344 bytes)
- `Sk`: Secret key (32 bytes)
- `OrganizationalUnitIdentifier`: OU string
- `Role`: Role (e.g., 1 for member)
- `EnrollmentID`: Enrollment ID (e.g., "admin")
- `CredentialRevocationInformation`: CRI data
- `CurveID`: Curve used ("gurvy.Bn254")
- `RevocationHandle`: Revocation handle

#### 2. Testcontainer Integration ([internal/idemix/enrollment_test.go](internal/idemix/enrollment_test.go))

**Test Setup**:

```go
func TestIdemixEnrollmentWithCA(t *testing.T) {
    // Start Fabric CA with idemix support
    req := testcontainers.ContainerRequest{
        Image: "hyperledger/fabric-ca:1.5.15",
        Cmd: []string{
            "sh", "-c",
            "fabric-ca-server start -b admin:adminpw --tls.enabled --idemix.curve gurvy.Bn254 -d",
        },
        WaitingFor: wait.ForLog("Listening on https://0.0.0.0:7054"),
    }
    
    // Perform idemix enrollment
    enrollReq := idemix.EnrollmentRequest{
        CAURL:        caURL,
        CAName:       "ca",
        EnrollID:     "admin",
        EnrollSecret: "adminpw",
        CACertPath:   tlsCertPath,
        MSPDir:       mspDir,
    }
    
    resp, err := idemix.Enroll(enrollReq)
    // Validate credential components
}
```

**Important Test Fixes**:
1. **TLS Cert Cleanup**: CA container output includes extra characters before PEM block, so we trim to `-----BEGIN CERTIFICATE-----`
2. **Curve Matching**: Client MUST specify `Idemix: api.Idemix{Curve: "gurvy.Bn254"}` to match CA curve
3. **CA Startup**: CA must be started with `--idemix.curve gurvy.Bn254` flag

#### 3. Identity Controller Integration ([internal/controller/identity_controller.go](internal/controller/identity_controller.go))

**Modified `handleEnrollment`**:

```go
func (r *IdentityReconciler) handleEnrollment(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity) (ctrl.Result, error) {
    enrollment := identity.Spec.Enrollment
    
    // Check if idemix enrollment is requested
    if enrollment.Idemix != nil {
        return r.handleIdemixEnrollment(ctx, logger, identity)
    }
    
    // Otherwise do X.509 enrollment...
}
```

**New `handleIdemixEnrollment` function**:

```go
func (r *IdentityReconciler) handleIdemixEnrollment(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity) (ctrl.Result, error) {
    // Get CA resource and TLS cert
    // Create temp MSP directory
    // Perform idemix enrollment
    enrollResp, err := idemix.Enroll(enrollReq)
    
    // Create comprehensive Kubernetes secret
    return r.createIdemixSecret(ctx, logger, identity, enrollResp)
}
```

**Idemix Secret Creation**:

```go
func (r *IdentityReconciler) createIdemixSecret(ctx context.Context, logger logr.Logger, identity *fabricxv1alpha1.Identity, enrollResp *idemix.EnrollmentResponse) error {
    secretData := make(map[string][]byte)
    
    // 1. Full SignerConfig as JSON
    secretData["SignerConfig"] = signerConfigJSON
    
    // 2. Individual components
    secretData["Cred"] = enrollResp.SignerConfig.GetCred()
    secretData["Sk"] = enrollResp.SignerConfig.GetSk()
    secretData["CRI"] = enrollResp.SignerConfig.GetCredentialRevocationInformation()
    
    // 3. Metadata JSON
    metadata := map[string]string{
        "enrollment_id":     enrollResp.SignerConfig.GetEnrollmentID(),
        "ou_identifier":     enrollResp.SignerConfig.GetOrganizationalUnitIdentifier(),
        "role":              fmt.Sprintf("%d", enrollResp.SignerConfig.GetRole()),
        "curve_id":          enrollResp.SignerConfig.CurveID,
        "revocation_handle": enrollResp.SignerConfig.RevocationHandle,
    }
    secretData["metadata.json"] = metadataJSON
    
    // 4. All files from idemix config directory
    for _, file := range files {
        content, _ := os.ReadFile(filepath.Join(enrollResp.IdemixConfigPath, file.Name()))
        secretData[fmt.Sprintf("user/%s", file.Name())] = content
    }
    
    // Create secret with all data
    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-idemix-cred", output.SecretPrefix),
            Namespace: namespace,
        },
        Data: secretData,
    }
    
    // Create and track in status
    identity.Status.OutputSecrets.IdemixCred = secretName
}
```

#### 4. CA Controller Idemix Support ([internal/controller/ca/ca_controller.go](internal/controller/ca/ca_controller.go))

**Updated Deployment Command**:

```go
Command: func() []string {
    baseCmd := `fabric-ca-server start`
    
    // Add idemix flag if configured
    if ca.Spec.Idemix != nil && ca.Spec.Idemix.Curve != "" {
        baseCmd += fmt.Sprintf(" --idemix.curve %s", ca.Spec.Idemix.Curve)
    }
    
    return []string{"sh", "-c", baseCmd}
}()
```

#### 5. API Types

**CA Spec** ([api/v1alpha1/ca_types.go](api/v1alpha1/ca_types.go)):

```go
type CASpec struct {
    // ... other fields ...
    
    // Idemix configuration
    Idemix *FabricCAIdemix `json:"idemix,omitempty"`
}

type FabricCAIdemix struct {
    // Curve for idemix (e.g., "gurvy.Bn254", "amcl.Fp256bn")
    // +kubebuilder:default="gurvy.Bn254"
    Curve string `json:"curve,omitempty"`
}
```

**Identity Spec** ([api/v1alpha1/identity_types.go](api/v1alpha1/identity_types.go)):

```go
type IdentityEnrollment struct {
    // ... other fields ...
    
    // Idemix enrollment configuration
    Idemix *IdentityIdemixEnrollment `json:"idemix,omitempty"`
}

type IdentityIdemixEnrollment struct {
    // CA reference for idemix enrollment (defaults to main CARef if not specified)
    CARef *IdentityCARef `json:"caRef,omitempty"`
}

type IdentityOutputSecrets struct {
    // ... X.509 cert fields ...
    
    // Idemix credential secret name
    IdemixCred string `json:"idemixCred,omitempty"`
}
```

### Directory Structure

After idemix enrollment, the MSP directory contains:

```
<MSPDir>/
  ├── keystore/              # BCCSP keystore
  └── user/
      └── SignerConfig       # JSON file with complete credential
```

The Kubernetes secret mirrors this structure with additional metadata:

```
Secret Data:
  SignerConfig            # Complete JSON SignerConfig
  Cred                    # Raw credential bytes
  Sk                      # Secret key bytes
  CRI                     # Credential Revocation Information
  metadata.json           # Human-readable metadata
  user/SignerConfig       # Copy of original SignerConfig file
```

### Sample Deployment

#### 1. Deploy Idemix CA ([config/samples/fabricx_v1alpha1_ca_idemix.yaml](config/samples/fabricx_v1alpha1_ca_idemix.yaml))

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: endorser-idemix-ca
spec:
  image: hyperledger/fabric-ca
  version: 1.5.15
  idemix:
    curve: gurvy.Bn254  # Enable idemix with gurvy.Bn254 curve
  ca:
    name: idemix-ca
    registry:
      identities:
        - name: admin
          pass: adminpw
          type: client
```

#### 2. Create Idemix Identity ([config/samples/fabricx_v1alpha1_identity_idemix.yaml](config/samples/fabricx_v1alpha1_identity_idemix.yaml))

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Identity
metadata:
  name: org1-idemix-user
spec:
  type: user
  mspID: Org1MSP
  enrollment:
    caRef:
      name: endorser-idemix-ca
    enrollID: admin
    enrollSecretRef:
      name: idemix-enrollment-secret
      key: password
    enrollTLS: false
    idemix: {}  # Enable idemix enrollment
  output:
    secretPrefix: org1-idemix-user
```

#### 3. Verify Idemix Secret

```bash
# List idemix secret
kubectl get secret org1-idemix-user-idemix-cred

# View metadata
kubectl get secret org1-idemix-user-idemix-cred -o jsonpath='{.data.metadata\.json}' | base64 -d | jq

# Output:
# {
#   "enrollment_id": "admin",
#   "ou_identifier": "ou1",
#   "role": "1",
#   "curve_id": "gurvy.Bn254",
#   "revocation_handle": "..."
# }

# View SignerConfig
kubectl get secret org1-idemix-user-idemix-cred -o jsonpath='{.data.SignerConfig}' | base64 -d | jq
```

### Key Learnings

1. **Curve Matching is Critical**: 
   - CA must be started with `--idemix.curve gurvy.Bn254`
   - Client MUST specify `Idemix: api.Idemix{Curve: "gurvy.Bn254"}`
   - Mismatch causes: "Invalid Idemix credential request: failure [set bytes failed [invalid point: subgroup check failed]]"

2. **Supported Curves**:
   - `gurvy.Bn254` (recommended, BN254 pairing-friendly curve)
   - `amcl.Fp256bn` (default if not specified)
   - `amcl.Fp256Miraclbn`

3. **TLS Certificate Handling**:
   - Container exec output may include control characters
   - Trim to `-----BEGIN CERTIFICATE-----` to clean cert data

4. **Directory Structure**:
   - Idemix creates `<MSPDir>/user/SignerConfig`
   - BCCSP keystore at `<MSPDir>/keystore/`
   - All credential data is in SignerConfig JSON

5. **Secret Organization**:
   - Store complete SignerConfig JSON
   - Also store individual components (Cred, Sk, CRI) for easy access
   - Include human-readable metadata.json
   - Mirror file structure with `user/` prefix

6. **Testing**:
   - Use testcontainers for integration testing
   - Spin up real Fabric CA with idemix support
   - Verify all credential components
   - Test both successful and failed enrollments

### Benefits

1. **Privacy-Preserving**: Idemix credentials support anonymous transactions
2. **Automatic**: No manual enrollment needed
3. **Testable**: Isolated package with comprehensive tests
4. **Complete**: All credential data stored in single secret
5. **Flexible**: Can use different CA for idemix vs X.509

### Future Enhancements

Potential improvements:
- Support for idemix attribute-based credentials
- Automatic curve detection from CA
- Re-enrollment support
- Revocation integration
- Multiple idemix credentials per identity

---

