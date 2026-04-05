# Hash-Based Rollout Detection

## Overview

The Fabric X Operator uses SHA256 hashing to automatically trigger pod rollouts when configuration changes occur. This ensures deployments stay synchronized with their ConfigMaps and Secrets without manual intervention.

## How It Works

### 1. Hash Computation

When a controller reconciles a deployment, it:
1. Computes SHA256 hashes of all mounted Secrets and ConfigMaps
2. Computes hash of environment variables (if any)
3. Combines all hashes into a single deterministic hash
4. Adds this hash as a pod template annotation

### 2. Automatic Rollout

When configuration changes:
1. The hash value changes
2. Kubernetes detects the pod template annotation has changed
3. Kubernetes triggers a rolling update automatically
4. Old pods are terminated and new pods are created with the updated config

### 3. Hash Annotation

The combined hash is stored as a pod template annotation:
```yaml
metadata:
  annotations:
    fabricx.kfsoft.tech/config-hash: "abc123def456..."
```

## Implementation

### Using the Hash Utility Functions

The operator provides reusable hash utilities in `internal/controller/utils/hash.go`:

#### Option 1: HashBuilder (Recommended)

```go
import "github.com/kfsoftware/fabric-x-operator/internal/controller/utils"

// Build combined hash using fluent API
configHash := utils.NewHashBuilder().
    AddConfigMap(ctx, r.Client, "my-config", namespace).
    AddSecret(ctx, r.Client, "my-secret", namespace).
    AddSecret(ctx, r.Client, "tls-cert", namespace).
    AddEnvVars(spec.Env).
    Build()

// Add to pod template annotations
deployment.Spec.Template.ObjectMeta.Annotations["fabricx.kfsoft.tech/config-hash"] = configHash
```

#### Option 2: Individual Hash Functions

```go
import "github.com/kfsoftware/fabric-x-operator/internal/controller/utils"

var hashParts []string

// Hash ConfigMap
if hash, err := utils.ComputeConfigMapHash(ctx, r.Client, "my-config", namespace); err == nil {
    hashParts = append(hashParts, hash)
}

// Hash Secret
if hash, err := utils.ComputeSecretHash(ctx, r.Client, "my-secret", namespace); err == nil {
    hashParts = append(hashParts, hash)
}

// Hash environment variables
if envHash := utils.ComputeEnvVarsHash(spec.Env); envHash != "" {
    hashParts = append(hashParts, envHash)
}

// Combine all hashes
configHash := utils.ComputeCombinedHash(hashParts)
```

## Controllers with Hash-Based Rollout

### Orderer Components

All orderer components implement hash-based rollout:

- ✅ **OrdererRouter**: Hashes router config and certificates
- ✅ **OrdererBatcher**: Hashes batcher config and certificates
- ✅ **OrdererConsenter**: Hashes consenter config and certificates
- ✅ **OrdererAssembler**: Hashes assembler config and certificates

**Example from OrdererConsenter:**
```go
// Compute hash of genesis block secret
hash := sha256.Sum256([]byte(dataString))
configMapHash := hex.EncodeToString(hash[:])

// Add to deployment annotations
annotations["fabricx.kfsoft.tech/config-hash"] = configMapHash
```

### Committer Components

All committer components implement hash-based rollout:

- ✅ **CommitterCoordinator**: Hashes config and certificates
- ✅ **CommitterSidecar**: Hashes config, certificates, and env vars
- ✅ **CommitterValidator**: Hashes config, certificates, and PostgreSQL credentials
- ✅ **CommitterVerifier**: Hashes config and certificates
- ✅ **CommitterQueryService**: Hashes config and PostgreSQL credentials

**Example from CommitterSidecar:**
```go
var hashParts []string

// Hash config secret
if hash, err := r.computeSecretHash(ctx, configSecretName, namespace); err == nil {
    hashParts = append(hashParts, hash)
}

// Hash sign cert secret
if hash, err := r.computeSecretHash(ctx, signCertSecretName, namespace); err == nil {
    hashParts = append(hashParts, hash)
}

// Hash environment variables
if len(spec.Env) > 0 {
    envHash := utils.ComputeEnvVarsHash(spec.Env)
    hashParts = append(hashParts, envHash)
}

// Combine and apply
configHash := utils.ComputeCombinedHash(hashParts)
deployment.Spec.Template.ObjectMeta.Annotations["fabricx.kfsoft.tech/config-hash"] = configHash
```

### Endorser Components

- ✅ **Endorser**: Hashes core config and certificates

**Example from Endorser:**
```go
hash := sha256.New()
hash.Write([]byte(coreYAML))
// ... hash certificates ...
configHash := hex.EncodeToString(hash.Sum(nil))

annotations["fabric-x.kfsoft.tech/core-config-hash"] = configHash
```

### CA Components

- ✅ **CA**: Hashes CA configuration

## What Gets Hashed

### ConfigMaps and Secrets

All data keys are included in the hash:
- ConfigMap: `data` and `binaryData` fields
- Secret: `data` field

### Environment Variables

Only name and value are hashed:
```go
for _, env := range envVars {
    envString += fmt.Sprintf("%s=%s|", env.Name, env.Value)
}
```

**Note:** Environment variables with `valueFrom` (SecretKeyRef, ConfigMapKeyRef) are NOT hashed directly. The underlying Secret/ConfigMap should be hashed separately.

### Genesis Blocks

For orderer components, the genesis block data is hashed.

## Best Practices

### 1. Hash All Mounted Resources

Always hash:
- Configuration ConfigMaps/Secrets
- Certificate Secrets
- Any Secret/ConfigMap mounted as volume or env var

### 2. Use Consistent Annotation Key

Use the standard annotation key:
```go
"fabricx.kfsoft.tech/config-hash"
```

### 3. Handle Missing Resources Gracefully

Use the HashBuilder which silently ignores missing resources:
```go
configHash := utils.NewHashBuilder().
    AddSecret(ctx, r.Client, "optional-secret", namespace).  // Won't fail if missing
    Build()
```

Or handle errors explicitly:
```go
if hash, err := utils.ComputeSecretHash(ctx, r.Client, secretName, namespace); err != nil {
    log.V(1).Info("Secret not found, skipping hash", "secret", secretName)
} else {
    hashParts = append(hashParts, hash)
}
```

### 4. Sort Before Combining

The utility functions automatically sort hashes before combining to ensure deterministic results:
```go
sort.Strings(hashParts)
combinedHash := sha256.Sum256([]byte(strings.Join(hashParts, "|")))
```

### 5. Test Hash Changes

When updating a Secret or ConfigMap, verify the hash changes:
```bash
# Get current hash
kubectl get deployment my-deployment -o jsonpath='{.spec.template.metadata.annotations.fabricx\.kfsoft\.tech/config-hash}'

# Update the secret
kubectl patch secret my-secret -p '{"data":{"key":"new-value-base64"}}'

# Wait for reconciliation
sleep 5

# Verify hash changed
kubectl get deployment my-deployment -o jsonpath='{.spec.template.metadata.annotations.fabricx\.kfsoft\.tech/config-hash}'
```

## Troubleshooting

### Hash Not Updating

**Problem:** Changed a Secret/ConfigMap but pods didn't rollout.

**Solutions:**
1. Check if the Secret/ConfigMap is actually being hashed:
   ```bash
   kubectl logs -n fabric-x-operator-system deployment/fabric-x-operator-controller-manager | grep "hash"
   ```

2. Verify the controller is reconciling:
   ```bash
   kubectl get <resource-type> <resource-name> -o yaml | grep -A 5 status
   ```

3. Check if the resource is being watched:
   ```bash
   kubectl logs -n fabric-x-operator-system deployment/fabric-x-operator-controller-manager | grep "Reconciling"
   ```

### Rollout Taking Too Long

**Problem:** Hash changed but rollout is slow.

**Solutions:**
1. Check deployment strategy (RollingUpdate vs Recreate)
2. Check pod readiness probes
3. Check resource availability in the cluster

### Hash Collisions

**Problem:** Different configs produce the same hash (extremely rare).

**Solution:** SHA256 has negligible collision probability. If you suspect a collision:
1. Add a version or timestamp to your ConfigMap
2. Force reconciliation: `kubectl annotate <resource> reconcile=true --overwrite`

## Migration Guide

### Migrating Existing Controllers

If a controller doesn't have hash-based rollout:

1. **Import the hash utilities:**
   ```go
   import "github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
   ```

2. **Add SHA256 import:**
   ```go
   import "crypto/sha256"
   ```

3. **Compute hash before creating deployment:**
   ```go
   configHash := utils.NewHashBuilder().
       AddConfigMap(ctx, r.Client, configMapName, namespace).
       AddSecret(ctx, r.Client, secretName, namespace).
       Build()
   ```

4. **Add annotation to pod template:**
   ```go
   if deployment.Spec.Template.ObjectMeta.Annotations == nil {
       deployment.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
   }
   deployment.Spec.Template.ObjectMeta.Annotations["fabricx.kfsoft.tech/config-hash"] = configHash
   ```

5. **Test the changes:**
   - Deploy the updated controller
   - Update a ConfigMap/Secret
   - Verify pods rollout automatically

## Performance Considerations

### Hash Computation Cost

- SHA256 is fast (~500 MB/s on modern CPUs)
- Hashing typical ConfigMaps/Secrets takes < 1ms
- Negligible impact on reconciliation time

### Cache Considerations

- Kubernetes already caches Secrets/ConfigMaps
- No additional caching needed for hash computation
- Hash is recomputed on every reconciliation (by design)

## Security Considerations

### Hash Visibility

- Hashes are stored in pod annotations (visible to anyone who can view pods)
- Hashes are one-way (cannot reverse to get original config)
- Hashes don't expose secret values

### Hash Manipulation

- Users with pod edit permissions could modify the hash annotation
- This would NOT affect the actual config (mounted from Secrets/ConfigMaps)
- Would only cause unnecessary pod rollouts

## References

- [Kubernetes Deployments](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
- [ConfigMap Best Practices](https://kubernetes.io/docs/concepts/configuration/configmap/#configmaps-and-pods)
- [Secret Management](https://kubernetes.io/docs/concepts/configuration/secret/)
- [SHA256 Specification](https://nvlpubs.nist.gov/nistpubs/FIPS/NIST.FIPS.180-4.pdf)

## Summary

Hash-based rollout ensures:
- ✅ **Automatic Updates**: No manual pod restarts needed
- ✅ **Consistency**: Pods always run with current config
- ✅ **Deterministic**: Same config = same hash
- ✅ **Efficient**: Fast hash computation
- ✅ **Reliable**: Kubernetes-native rollout mechanism

All Fabric X Operator controllers implement this pattern for seamless configuration updates.
