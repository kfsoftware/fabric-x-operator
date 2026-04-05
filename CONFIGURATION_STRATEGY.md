# Fabric X Operator Configuration Strategy

## Problem Statement

Managing configuration for multiple components (consenter, batcher, assembler, router) with their own certificates, hosts, ingress, PVC, and other properties can become a maintenance nightmare when:

1. **Repetition**: Same configuration repeated across components
2. **Inconsistency**: Different components configured differently for no good reason
3. **Complexity**: Deep nesting makes configuration hard to read and maintain
4. **Scalability**: Adding new components requires duplicating configuration

## Solution: Inheritance and Composition Pattern

The Fabric X Operator uses a **composition and inheritance pattern** to solve these problems:

### 1. Common Configuration Inheritance

```yaml
spec:
  # Global common configuration - applies to all components
  common:
    replicas: 1
    storage:
      size: "20Gi"
      storageClass: "fast-ssd"
    resources:
      cpuRequests: "1000m"
      memoryRequests: "2Gi"
    securityContext:
      runAsUser: 1000
      readOnlyRootFilesystem: true
    # Pod configuration
    podAnnotations:
      prometheus.io/scrape: "true"
    podLabels:
      app.kubernetes.io/component: "fabric-x"
    volumes:
      - name: "shared-config"
        volumeSource:
          configMap:
            name: "fabric-x-config"
    volumeMounts:
      - name: "shared-config"
        mountPath: "/etc/fabric-x/config"
    imagePullSecrets:
      - name: "fabric-x-registry"
    tolerations:
      - key: "node-role.kubernetes.io/master"
        operator: "Exists"
        effect: "NoSchedule"
  
  # Component-specific configurations inherit from common
  components:
    consenter:
      replicas: 3  # Override common replicas
      storage:
        size: "50Gi"  # Override common storage size
      # Inherits common resources, security context, and pod configuration
```

### 2. Component-Specific Overrides

Components can override any common setting while inheriting the rest:

```yaml
components:
  batcher:
    replicas: 5  # Override common replicas
    resources:
      cpuRequests: "500m"  # Override common CPU
    # Component-specific pod configuration
    podLabels:
      fabric-x/batcher-type: "high-throughput"
    volumes:
      - name: "batcher-cache"
        volumeSource:
          emptyDir:
            medium: "Memory"
    # Inherits common storage, security context, and other pod settings
```

### 3. Global Enrollment Configuration

Certificate enrollment can be configured globally and inherited:

```yaml
spec:
  # Global enrollment - inherited by all components
  enrollment:
    sign:
      cahost: 'ca.org1.example.com'
      enrollid: 'admin'
      enrollsecret: 'adminpw'
  
  components:
    assembler:
      # Override enrollment for this component
      certificates:
        enrollid: 'assembler-admin'
        enrollsecret: 'assembler-pw'
```

## Benefits of This Approach

### 1. **DRY (Don't Repeat Yourself)**
- Common settings defined once in `spec.common`
- Components inherit automatically
- Override only what's different

### 2. **Consistency**
- All components start with the same baseline
- Reduces configuration drift
- Easier to audit and maintain

### 3. **Flexibility**
- Components can override any setting
- Component-specific configurations are clearly visible
- Easy to add new components

### 4. **Readability**
- Clear separation between common and component-specific settings
- Hierarchical structure is easy to understand
- Comments explain inheritance behavior

### 5. **Maintainability**
- Change common settings in one place
- Component-specific changes are isolated
- Easy to add new configuration properties

## Configuration Structure

```
OrdererGroupSpec
├── common (CommonComponentConfig)
│   ├── replicas
│   ├── storage
│   ├── resources
│   ├── securityContext
│   ├── podAnnotations
│   ├── podLabels
│   ├── volumes
│   ├── affinity
│   ├── volumeMounts
│   ├── imagePullSecrets
│   └── tolerations
├── genesis (GenesisConfig)
├── enrollment (EnrollmentConfig)
└── components (OrdererComponents)
    ├── consenter (ComponentConfig)
    ├── batcher (ComponentConfig)
    ├── assembler (ComponentConfig)
    └── router (ComponentConfig)
```

## Inheritance Rules

1. **Common → Component**: All common settings are inherited
2. **Component Override**: Component-specific settings override common
3. **Global Enrollment**: Global enrollment settings are inherited unless overridden
4. **Component Enrollment**: Component-specific enrollment overrides global

## Pod Configuration Fields

The following Kubernetes pod configuration fields are supported with inheritance:

### Pod Annotations
```yaml
common:
  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"

components:
  consenter:
    podAnnotations:
      fabric-x/consenter-type: "raft"  # Component-specific
```

### Pod Labels
```yaml
common:
  podLabels:
    app.kubernetes.io/component: "fabric-x"

components:
  batcher:
    podLabels:
      fabric-x/batcher-type: "high-throughput"  # Component-specific
```

### Volumes
```yaml
common:
  volumes:
    - name: "shared-config"
      volumeSource:
        configMap:
          name: "fabric-x-config"

components:
  consenter:
    volumes:
      - name: "consenter-data"
        volumeSource:
          persistentVolumeClaim:
            claimName: "consenter-pvc"
```

### Volume Mounts
```yaml
common:
  volumeMounts:
    - name: "shared-config"
      mountPath: "/etc/fabric-x/config"
      readOnly: true

components:
  consenter:
    volumeMounts:
      - name: "consenter-data"
        mountPath: "/var/fabric-x/consenter"
```

### Affinity
```yaml
common:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: "node-role.kubernetes.io/worker"
                operator: "Exists"

components:
  consenter:
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app.kubernetes.io/component: "consenter"
            topologyKey: "kubernetes.io/hostname"
```

### Image Pull Secrets
```yaml
common:
  imagePullSecrets:
    - name: "fabric-x-registry"

components:
  assembler:
    imagePullSecrets:
      - name: "assembler-registry"
      - name: "fabric-x-registry"  # Include common + component-specific
```

### Tolerations
```yaml
common:
  tolerations:
    - key: "node-role.kubernetes.io/master"
      operator: "Exists"
      effect: "NoSchedule"

components:
  consenter:
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "consenter"
        effect: "NoSchedule"
```

## Examples

### Simple Configuration
```yaml
spec:
  common:
    replicas: 1
    storage:
      size: "10Gi"
    podAnnotations:
      prometheus.io/scrape: "true"
  components:
    consenter: {}  # Uses all common settings
    batcher: {}    # Uses all common settings
```

### Complex Configuration
```yaml
spec:
  common:
    replicas: 1
    storage:
      size: "20Gi"
    resources:
      cpuRequests: "1000m"
    podAnnotations:
      prometheus.io/scrape: "true"
    volumes:
      - name: "shared-config"
        volumeSource:
          configMap:
            name: "fabric-x-config"
  components:
    consenter:
      replicas: 3  # Override
      storage:
        size: "50Gi"  # Override
      resources:
        cpuRequests: "2000m"  # Override
      podAnnotations:
        fabric-x/consenter-type: "raft"  # Component-specific
      volumes:
        - name: "consenter-data"
          volumeSource:
            persistentVolumeClaim:
              claimName: "consenter-pvc"
    batcher:
      replicas: 5  # Override
      # Inherit storage, resources, and pod configuration
```

## Migration from Old Structure

The old structure had separate sections for each component:

```yaml
# OLD - Repetitive and hard to maintain
spec:
  replicas:
    consenter: 1
    batcher: 2
    assembler: 1
    router: 1
  ingress:
    consenter:
      istio:
        hosts: ['consenter.example.com']
    batcher:
      istio:
        hosts: ['batcher.example.com']
    # ... repeated for each component
  volumes:
    consenter:
      - name: "consenter-data"
    batcher:
      - name: "batcher-cache"
    # ... repeated for each component
```

The new structure eliminates repetition:

```yaml
# NEW - Clean and maintainable
spec:
  common:
    replicas: 1
    volumes:
      - name: "shared-config"
  components:
    consenter:
      replicas: 1  # Only override if different
      volumes:
        - name: "consenter-data"  # Component-specific
    batcher:
      replicas: 2  # Only override if different
      volumes:
        - name: "batcher-cache"  # Component-specific
    assembler: {}  # Use common
    router: {}     # Use common
```

## Best Practices

1. **Start with Common**: Define shared settings in `spec.common`
2. **Override Sparingly**: Only override what's truly different
3. **Use Comments**: Document why overrides are needed
4. **Group Related Settings**: Keep related configurations together
5. **Validate Inheritance**: Ensure the controller properly merges configurations
6. **Pod Configuration**: Use common pod settings for consistency, override for component-specific needs

## Controller Implementation

The controller uses helper functions to merge configurations:

```go
func (r *OrdererGroupReconciler) getMergedComponentConfig(
    ordererGroup *fabricxv1alpha1.OrdererGroup,
    componentName string,
    componentConfig *fabricxv1alpha1.ComponentConfig,
) *fabricxv1alpha1.ComponentConfig {
    // Start with common configuration
    merged := &fabricxv1alpha1.ComponentConfig{
        CommonComponentConfig: *ordererGroup.Spec.Common,
    }
    
    // Apply component-specific overrides
    if componentConfig != nil {
        if componentConfig.Replicas > 0 {
            merged.Replicas = componentConfig.Replicas
        }
        if componentConfig.PodAnnotations != nil {
            merged.PodAnnotations = componentConfig.PodAnnotations
        }
        if componentConfig.Volumes != nil {
            merged.Volumes = componentConfig.Volumes
        }
        // ... merge other fields
    }
    
    return merged
}
```

This approach makes configuration management much more maintainable and reduces the risk of configuration nightmares. 