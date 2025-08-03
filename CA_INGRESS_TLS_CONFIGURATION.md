# CA Ingress and TLS Domain Configuration

## Overview

The CA controller now supports ingress configuration for external access and TLS domain configuration for proper certificate generation. This allows the CA to be accessible from external domains and ensures TLS certificates are generated with the correct domain names.

## New Configuration Fields

### 1. Ingress Configuration

The CA now supports ingress configuration similar to OrdererGroup components:

```yaml
spec:
  ingress:
    istio:
      hosts:
        - 'ca.org1.localho.st'
        - 'ca.org1.example.com'
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
```

#### Ingress Fields
- **hosts**: List of domain names for the CA ingress
- **port**: Port number for the ingress (typically 443 for HTTPS)
- **ingressGateway**: Istio ingress gateway name
- **tls.enabled**: Enable TLS for the ingress
- **tls.secretName**: Secret containing TLS certificate for the ingress

### 2. TLS Domain Configuration

The CA now supports domain names for TLS certificate generation:

```yaml
spec:
  tls:
    subject:
      C: US
      L: Raleigh
      O: Hyperledger
      OU: Fabric
      ST: North Carolina
    # Domain names for TLS certificate generation
    domains:
      - 'ca.org1.localho.st'
      - 'ca.org1.example.com'
      - 'localhost'
```

#### TLS Domain Fields
- **subject**: Certificate subject information (Country, State, Organization, etc.)
- **domains**: List of domain names to include in the TLS certificate

## Example Configuration

Here's a complete example showing how to configure a CA with ingress and TLS domains:

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: CA
metadata:
  name: test-ca2
spec:
  # Ingress configuration for external access
  ingress:
    istio:
      hosts:
        - 'ca.org1.localho.st'
        - 'ca.org1.example.com'
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-tls-secret'
  
  # TLS configuration with domain names
  tls:
    subject:
      C: US
      L: Raleigh
      O: Hyperledger
      OU: Fabric
      ST: North Carolina
    # Domain names for TLS certificate generation
    domains:
      - 'ca.org1.localho.st'
      - 'ca.org1.example.com'
      - 'localhost'
  
  # CA configuration (existing fields)
  ca:
    # ... existing CA configuration
    csr:
      hosts:
      - localhost
      - ca.org1.localho.st
      - ca.org1.example.com
      # ... rest of CSR configuration
  
  # TLS CA configuration (existing fields)
  tlsca:
    # ... existing TLS CA configuration
    csr:
      hosts:
      - localhost
      - ca.org1.localho.st
      - ca.org1.example.com
      # ... rest of CSR configuration
```

## Benefits

### 1. External Access
- **Ingress Support**: CA can be accessed from external domains
- **Load Balancing**: Istio ingress provides load balancing capabilities
- **TLS Termination**: Ingress can handle TLS termination

### 2. Proper Certificate Generation
- **Domain Validation**: TLS certificates include the correct domain names
- **Trust Establishment**: Other services can trust the CA certificates
- **Multi-Domain Support**: Single CA can serve multiple domains

### 3. Security
- **TLS Encryption**: All external communication is encrypted
- **Certificate Validation**: Proper domain validation in certificates
- **Access Control**: Ingress can provide additional access controls

## Usage Scenarios

### Scenario 1: Multi-Organization CA
```yaml
spec:
  ingress:
    istio:
      hosts:
        - 'ca.org1.example.com'
        - 'ca.org2.example.com'
      port: 443
      ingressGateway: 'istio-ingressgateway'
  tls:
    domains:
      - 'ca.org1.example.com'
      - 'ca.org2.example.com'
```

### Scenario 2: Development Environment
```yaml
spec:
  ingress:
    istio:
      hosts:
        - 'ca.localho.st'
      port: 443
      ingressGateway: 'istio-ingressgateway'
  tls:
    domains:
      - 'ca.localho.st'
      - 'localhost'
```

### Scenario 3: Production Environment
```yaml
spec:
  ingress:
    istio:
      hosts:
        - 'ca.production.example.com'
      port: 443
      ingressGateway: 'istio-ingressgateway'
      tls:
        enabled: true
        secretName: 'ca-production-tls'
  tls:
    domains:
      - 'ca.production.example.com'
```

## Implementation Details

### CA Controller Changes
The CA controller has been updated to:
1. **Process Ingress Configuration**: Create and manage Istio VirtualService and Gateway
2. **Generate TLS Certificates**: Use domain names in certificate generation
3. **Update CSR Configuration**: Include domain names in certificate signing requests

### Certificate Generation
When TLS domains are configured:
1. **CSR Generation**: Domain names are included in the certificate signing request
2. **Certificate Validation**: Generated certificates are valid for all specified domains
3. **Trust Chain**: Other services can validate certificates against the CA

### Ingress Management
The ingress configuration:
1. **Creates VirtualService**: Routes traffic to the CA service
2. **Configures Gateway**: Sets up the ingress gateway
3. **Manages TLS**: Handles TLS termination and certificate management

## Migration from Existing CAs

### Adding Ingress to Existing CA
```yaml
# Add to existing CA spec
spec:
  ingress:
    istio:
      hosts:
        - 'ca.org1.example.com'
      port: 443
      ingressGateway: 'istio-ingressgateway'
```

### Adding TLS Domains to Existing CA
```yaml
# Add to existing CA spec
spec:
  tls:
    domains:
      - 'ca.org1.example.com'
      - 'localhost'
```

## Troubleshooting

### Common Issues

1. **Ingress Not Working**
   - Check if Istio is installed and configured
   - Verify ingress gateway is running
   - Check VirtualService and Gateway resources

2. **Certificate Domain Mismatch**
   - Ensure domains in `tls.domains` match ingress hosts
   - Verify CSR includes all required domains
   - Check certificate validation

3. **TLS Certificate Issues**
   - Verify TLS secret exists and is valid
   - Check certificate domain names
   - Ensure proper certificate chain

### Debugging Commands
```bash
# Check CA status
kubectl get ca test-ca2

# Check ingress resources
kubectl get virtualservice
kubectl get gateway

# Check TLS certificates
kubectl get secret ca-tls-secret -o yaml

# Check CA logs
kubectl logs -l app.kubernetes.io/name=fabric-x-operator
```

## Best Practices

1. **Domain Planning**: Plan domain names before deployment
2. **Certificate Management**: Use proper certificate management for production
3. **Security**: Enable TLS for all external access
4. **Monitoring**: Monitor ingress and certificate health
5. **Backup**: Backup CA certificates and configuration

## Future Enhancements

1. **Multiple Ingress Types**: Support for other ingress controllers
2. **Certificate Auto-Renewal**: Automatic certificate renewal
3. **Domain Validation**: Automated domain validation
4. **Advanced TLS**: Support for advanced TLS features 