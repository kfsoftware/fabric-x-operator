# Certificate Provisioning with Fabric CA

This document explains how to use the Fabric CA certificate provisioning feature in the Fabric X Operator for OrdererGroup components.

## Overview

The Fabric X Operator now supports automatic certificate provisioning using Fabric CA. When you create an OrdererGroup, the operator will:

1. Connect to the specified Fabric CA server
2. Enroll each component (consenter, batcher, assembler, router) with the CA
3. Generate both signing and TLS certificates for each component using ECDSA P256
4. Store the certificates in Kubernetes secrets
5. Make the certificates available to the components via volume mounts

**Note**: The operator no longer generates self-signed certificates. All certificates are now provisioned through Fabric CA for better security and consistency.

## Configuration

### Global Enrollment Configuration

You can specify global enrollment settings that will be inherited by all components:

```yaml
spec:
  enrollment:
    sign:
      cahost: 'ca.org1.localho.st'
      caport: 7054
      catls:
        secretRef:
          name: 'org1-ca'
          key: 'ca-cert.pem'
          namespace: 'default'
      enrollid: 'admin'
      enrollsecret: 'adminpw'
    tls:
      cahost: 'ca.org1.localho.st'
      caport: 7054
      catls:
        secretRef:
          name: 'org1-ca'
          key: 'ca-cert.pem'
          namespace: 'default'
      enrollid: 'admin'
      enrollsecret: 'adminpw'
```

### Component-Specific Certificate Configuration

You can also specify component-specific certificate settings that override the global configuration:

```yaml
spec:
  components:
    consenter:
      certificates:
        cahost: 'ca.org1.localho.st'
        caport: 7054
        catls:
          secretRef:
            name: 'org1-ca'
            key: 'ca-cert.pem'
            namespace: 'default'
        enrollid: 'admin'
        enrollsecret: 'adminpw'
```

## Certificate Types

The operator generates two types of certificates for each component using Fabric CA:

1. **Signing Certificates**: Used for signing transactions and blocks
2. **TLS Certificates**: Used for secure communication between components

All certificates are generated using ECDSA P256 curve for optimal security and performance.

## Generated Secrets

For each component, the operator creates Kubernetes secrets with the following naming pattern:
- `{orderergroup-name}-{component-name}-sign-cert` for signing certificates
- `{orderergroup-name}-{component-name}-tls-cert` for TLS certificates

Each secret contains:
- `cert.pem`: The component's certificate (provisioned by Fabric CA)
- `key.pem`: The component's private key (provisioned by Fabric CA)
- `ca.pem`: The CA certificate

## Example Configuration

Here's a complete example showing how to configure certificate provisioning:

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: OrdererGroup
metadata:
  name: orderergroup-sample
spec:
  bootstrapMode: 'configure'
  
  # Global enrollment configuration
  enrollment:
    sign:
      cahost: 'ca.org1.localho.st'
      caport: 7054
      catls:
        secretRef:
          name: 'org1-ca'
          key: 'ca-cert.pem'
          namespace: 'default'
      enrollid: 'admin'
      enrollsecret: 'adminpw'
    tls:
      cahost: 'ca.org1.localho.st'
      caport: 7054
      catls:
        secretRef:
          name: 'org1-ca'
          key: 'ca-cert.pem'
          namespace: 'default'
      enrollid: 'admin'
      enrollsecret: 'adminpw'
  
  components:
    consenter:
      replicas: 1
      # Component-specific certificates (optional)
      certificates:
        cahost: 'ca.org1.localho.st'
        caport: 7054
        catls:
          secretRef:
            name: 'org1-ca'
            key: 'ca-cert.pem'
            namespace: 'default'
        enrollid: 'admin'
        enrollsecret: 'adminpw'
    
    batcher:
      replicas: 2
      # Will inherit global enrollment settings
    
    assembler:
      replicas: 1
      # Will inherit global enrollment settings
    
    router:
      replicas: 1
      # Will inherit global enrollment settings
```

## Prerequisites

Before using certificate provisioning, ensure you have:

1. **Fabric CA Server**: A running Fabric CA server that the operator can connect to
2. **CA Certificate Secret**: A Kubernetes secret containing the CA certificate
3. **Enrollment Credentials**: Valid enrollment ID and secret for the CA

### Setting up the CA Certificate Secret

Create a secret containing your Fabric CA certificate:

```bash
kubectl create secret generic org1-ca \
  --from-file=ca-cert.pem=/path/to/ca-cert.pem \
  --namespace=default
```

## How It Works

1. **Reconciliation**: When an OrdererGroup is created or updated, the operator triggers certificate provisioning
2. **Component Enrollment**: For each component, the operator:
   - Connects to the Fabric CA server using the provided configuration
   - Enrolls the component with a unique enrollment ID
   - Generates both signing and TLS certificates using ECDSA P256
3. **Secret Creation**: The operator creates Kubernetes secrets containing the certificates
4. **Component Deployment**: The components can then mount these secrets and use the certificates

## Architecture

The certificate provisioning is integrated into the component controller architecture:

- **BaseComponentController**: Contains the certificate service and handles certificate operations
- **Individual Component Controllers**: (Consenter, Batcher, Assembler, Router) inherit from BaseComponentController
- **Certificate Service**: Handles Fabric CA operations and secret management
- **OrdererGroup Controller**: Orchestrates the overall reconciliation process

## Troubleshooting

### Common Issues

1. **CA Connection Failed**: Check that the CA host and port are correct
2. **Authentication Failed**: Verify the enrollment ID and secret
3. **Certificate Generation Failed**: Ensure the CA certificate is valid and accessible

### Debugging

Check the operator logs for certificate provisioning messages:

```bash
kubectl logs -f deployment/fabric-x-operator -n fabric-x-system
```

Look for messages like:
- "Successfully provisioned certificates for component consenter"
- "Failed to provision certificates for component batcher"

### Certificate Secret Inspection

You can inspect the generated certificate secrets:

```bash
kubectl get secrets -l fabric-x/orderergroup=orderergroup-sample
kubectl describe secret orderergroup-sample-consenter-sign-cert
```

## Security Considerations

1. **Secret Management**: Certificate secrets are automatically cleaned up when the OrdererGroup is deleted
2. **Access Control**: Ensure only authorized components can access the certificate secrets
3. **Certificate Rotation**: The operator will regenerate certificates on reconciliation if needed
4. **Network Security**: Use TLS for all CA communications
5. **Key Security**: All private keys are generated using ECDSA P256 and stored securely in Kubernetes secrets

## Advanced Configuration

### Custom Enrollment Attributes

The operator supports custom enrollment attributes for fine-grained control over certificate generation. This can be extended in future versions.

### Multiple CA Support

You can configure different CAs for different components or certificate types by specifying component-specific certificate configurations.

### Certificate Renewal

The operator will automatically handle certificate renewal when certificates are close to expiration. This is handled during regular reconciliation cycles.

## Migration from Self-Signed Certificates

If you were previously using self-signed certificates, the operator now automatically uses Fabric CA for all certificate generation. No manual migration is required - simply configure the enrollment settings and the operator will handle the rest. 