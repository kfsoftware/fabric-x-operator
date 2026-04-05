# TLS Usage Scenarios

## Overview

The CA supports multiple TLS configuration scenarios. You can use TLS with or without ingress, depending on your deployment requirements. The ingress configuration includes an `enabled` flag to control whether ingress resources are created.

## Scenario 1: TLS Only (No Ingress)

### Use Case
- Internal cluster communication only
- Services within the same namespace or cluster
- Development environments
- When you don't need external access

### Configuration
```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: test-ca-tls-only
spec:
  # Ingress configuration (disabled for TLS-only setup)
  ingress:
    enabled: false  # Disable ingress for internal-only access
    istio:
      hosts:
        - 'ca.org1.localho.st'
        - 'ca.org1.example.com'
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
  
  # TLS configuration with domain names (no ingress)
  tls:
    subject:
      C: US
      L: Raleigh
      O: Hyperledger
      OU: Fabric
      ST: North Carolina
    domains:
      - 'ca.org1.localho.st'
      - 'ca.org1.example.com'
      - 'localhost'
      - 'test-ca-tls-only.default.svc.cluster.local'  # Kubernetes service DNS
      - 'test-ca-tls-only.default.svc'                # Short service DNS
      - 'test-ca-tls-only'                            # Service name
  
  # Service configuration (ClusterIP for internal access)
  service:
    type: ClusterIP
  
  # CA configuration with proper hosts
  ca:
    csr:
      hosts:
      - localhost
      - ca.org1.localho.st
      - ca.org1.example.com
      - test-ca-tls-only.default.svc.cluster.local
      - test-ca-tls-only.default.svc
      - test-ca-tls-only
      # ... rest of CA configuration
```

### Access Methods
1. **Internal Cluster Access**:
   ```bash
   # From within the cluster
   curl -k https://test-ca-tls-only.default.svc:7054
   ```

2. **Port Forward**:
   ```bash
   # From outside the cluster
   kubectl port-forward svc/test-ca-tls-only 7054:7054
   curl -k https://localhost:7054
   ```

3. **NodePort Service** (if needed):
   ```yaml
   service:
     type: NodePort
   ```

## Scenario 2: TLS with Ingress

### Use Case
- External access required
- Load balancing needed
- Production environments
- Multi-domain support

### Configuration
```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: test-ca-with-ingress
spec:
  # Ingress configuration (enabled for external access)
  ingress:
    enabled: true  # Enable ingress for external access
    istio:
      hosts:
        - 'ca.org1.example.com'
        - 'ca.org2.example.com'
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
  
  # TLS configuration
  tls:
    subject:
      C: US
      L: Raleigh
      O: Hyperledger
      OU: Fabric
      ST: North Carolina
    domains:
      - 'ca.org1.example.com'
      - 'ca.org2.example.com'
      - 'localhost'
  
  # Service configuration
  service:
    type: ClusterIP
  
  # CA configuration
  ca:
    csr:
      hosts:
      - localhost
      - ca.org1.example.com
      - ca.org2.example.com
      # ... rest of CA configuration
```

### Access Methods
1. **External Access**:
   ```bash
   # From external clients
   curl -k https://ca.org1.example.com
   ```

2. **Internal Access**:
   ```bash
   # From within the cluster
   curl -k https://test-ca-with-ingress.default.svc:7054
   ```

## Scenario 3: TLS with LoadBalancer

### Use Case
- Direct external access without ingress
- Cloud environments with load balancers
- When you want to bypass ingress

### Configuration
```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: test-ca-loadbalancer
spec:
  # Ingress configuration (disabled for LoadBalancer)
  ingress:
    enabled: false  # Disable ingress when using LoadBalancer
    istio:
      hosts:
        - 'ca.org1.example.com'
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
  
  # TLS configuration
  tls:
    subject:
      C: US
      L: Raleigh
      O: Hyperledger
      OU: Fabric
      ST: North Carolina
    domains:
      - 'ca.org1.example.com'
      - 'localhost'
  
  # LoadBalancer service
  service:
    type: LoadBalancer
  
  # CA configuration
  ca:
    csr:
      hosts:
      - localhost
      - ca.org1.example.com
      # ... rest of CA configuration
```

### Access Methods
1. **External Access via LoadBalancer**:
   ```bash
   # Get the external IP
   kubectl get svc test-ca-loadbalancer
   
   # Access via external IP
   curl -k https://<EXTERNAL-IP>:7054
   ```

## Scenario 4: Development Environment

### Use Case
- Local development
- Testing environments
- Minimal configuration

### Configuration
```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: test-ca-dev
spec:
  # Ingress configuration (disabled for development)
  ingress:
    enabled: false  # Disable ingress for development
    istio:
      hosts:
        - 'ca.localho.st'
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
  
  # Minimal TLS configuration
  tls:
    subject:
      C: US
      L: Raleigh
      O: Hyperledger
      OU: Fabric
      ST: North Carolina
    domains:
      - 'localhost'
      - 'test-ca-dev.default.svc'
  
  # Simple service
  service:
    type: ClusterIP
  
  # CA configuration
  ca:
    csr:
      hosts:
      - localhost
      - test-ca-dev.default.svc
      # ... rest of CA configuration
```

## Ingress Enabled Flag

The `enabled` flag in the ingress configuration controls whether ingress resources are created:

### Enabled: true
- Creates Ingress resources
- Enables external access via ingress
- Requires ingress controller (Istio, nginx, etc.)

### Enabled: false
- No Ingress resources created
- Internal access only
- No external dependencies

### Configuration Examples

**Enable Ingress:**
```yaml
spec:
  ingress:
    enabled: true
    istio:
      hosts: ['ca.org1.example.com']
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
```

**Disable Ingress:**
```yaml
spec:
  ingress:
    enabled: false
    istio:
      hosts: ['ca.org1.example.com']
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
```

**No Ingress Configuration:**
```yaml
spec:
  # No ingress field - defaults to no ingress
  tls:
    domains: ['localhost', 'ca-service.default.svc']
```

## Domain Name Considerations

### Kubernetes Service DNS
When using TLS without ingress, include Kubernetes service DNS names:

```yaml
tls:
  domains:
    - 'ca-service.default.svc.cluster.local'  # Full DNS
    - 'ca-service.default.svc'                # Short DNS
    - 'ca-service'                            # Service name
    - 'localhost'                             # Local access
    - 'ca.org1.example.com'                  # External domain (if applicable)
```

### Certificate Validation
The CA certificate will be valid for all specified domains:

```bash
# Check certificate domains
openssl x509 -in ca-cert.pem -text -noout | grep DNS
```

## Security Considerations

### TLS-Only (No Ingress)
**Pros:**
- Simpler configuration
- No external exposure
- Internal cluster security
- No ingress dependencies

**Cons:**
- Limited external access
- Manual port forwarding needed
- No load balancing

### TLS with Ingress
**Pros:**
- External access
- Load balancing
- TLS termination at ingress
- Multi-domain support

**Cons:**
- More complex configuration
- External exposure
- Ingress dependency
- Additional security considerations

## Access Patterns

### Internal Access
```bash
# Direct service access
curl -k https://ca-service.default.svc:7054

# Port forward
kubectl port-forward svc/ca-service 7054:7054
curl -k https://localhost:7054
```

### External Access
```bash
# Via ingress
curl -k https://ca.org1.example.com

# Via load balancer
curl -k https://<EXTERNAL-IP>:7054

# Via node port
curl -k https://<NODE-IP>:<NODE-PORT>
```

## Troubleshooting

### Certificate Issues
```bash
# Check certificate domains
kubectl get secret ca-tls-crypto -o yaml
openssl x509 -in ca-cert.pem -text -noout

# Verify certificate validation
openssl s_client -connect ca-service.default.svc:7054 -servername ca.org1.example.com
```

### Access Issues
```bash
# Check service
kubectl get svc ca-service

# Check endpoints
kubectl get endpoints ca-service

# Check CA logs
kubectl logs -l app=ca-service

# Test connectivity
kubectl run test-pod --image=busybox --rm -it -- wget -O- https://ca-service.default.svc:7054
```

### Ingress Issues
```bash
# Check ingress resources
kubectl get ingress

# Check ingress controller
kubectl get pods -n ingress-nginx  # or istio-system

# Check ingress logs
kubectl logs -n ingress-nginx -l app.kubernetes.io/name=ingress-nginx
```

## Best Practices

1. **Domain Planning**: Plan all required domains before deployment
2. **Certificate Management**: Use proper certificate management for production
3. **Security**: Enable TLS for all CA communication
4. **Monitoring**: Monitor certificate expiration and renewal
5. **Backup**: Backup CA certificates and configuration
6. **Testing**: Test certificate validation in your environment
7. **Ingress Control**: Use the `enabled` flag to control ingress creation

## Migration Scenarios

### From TLS-Only to TLS with Ingress
```yaml
# Add ingress configuration
spec:
  ingress:
    enabled: true
    istio:
      hosts: ['ca.org1.example.com']
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
```

### From Ingress to LoadBalancer
```yaml
# Change service type and disable ingress
spec:
  ingress:
    enabled: false
  service:
    type: LoadBalancer
```

### From LoadBalancer to TLS-Only
```yaml
# Change service type and disable ingress
spec:
  ingress:
    enabled: false
  service:
    type: ClusterIP
```

### Disable Ingress for Existing CA
```yaml
# Set enabled to false
spec:
  ingress:
    enabled: false
    # Keep existing istio configuration for reference
    istio:
      hosts: ['ca.org1.example.com']
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
``` 