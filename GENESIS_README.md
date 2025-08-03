# Fabric X Genesis Configuration

This document explains how to configure the Fabric X Genesis resource to create genesis blocks with both internal and external organizations, along with specific orderer nodes for consensus.

## Overview

The Genesis resource supports two types of organizations:

1. **Internal Organizations**: Use Fabric CA for certificate management
2. **External Organizations**: Use externally provided certificates

Additionally, you can specify individual orderer nodes for consensus with their specific TLS certificates and identities.

## Configuration Structure

### Internal Organizations

Internal organizations use Fabric CA for certificate management. The operator will automatically fetch certificates from the specified CA.

```yaml
internalOrgs:
  - name: "Org1"
    mspId: "Org1MSP"
    caReference:
      name: "org1-ca"
      namespace: "fabric-network"
    adminIdentity: "admin"
    ordererIdentity: "orderer"  # Optional, only for orderer orgs
```

**Fields:**
- `name`: Organization name
- `mspId`: MSP ID for the organization
- `caReference`: Reference to the Fabric CA resource
  - `name`: Name of the CA resource
  - `namespace`: Namespace where the CA is deployed
- `adminIdentity`: Name of the admin identity in the CA
- `ordererIdentity`: Name of the orderer identity in the CA (optional, only for orderer organizations)

### External Organizations

External organizations use certificates provided directly in the configuration.

```yaml
externalOrgs:
  - name: "ExternalOrg1"
    mspId: "ExternalOrg1MSP"
    signCert: "<BASE64_ENCODED_SIGNING_CERT>"
    tlsCert: "<BASE64_ENCODED_TLS_CERT>"
    adminCert: "<BASE64_ENCODED_ADMIN_CERT>"
    ordererCert: "<BASE64_ENCODED_ORDERER_CERT>"  # Optional
```

**Fields:**
- `name`: Organization name
- `mspId`: MSP ID for the organization
- `signCert`: Base64-encoded signing certificate
- `tlsCert`: Base64-encoded TLS certificate
- `adminCert`: Base64-encoded admin certificate
- `ordererCert`: Base64-encoded orderer certificate (optional, only for orderer organizations)

### Orderer Nodes for Consensus

Specify individual orderer nodes that will participate in consensus. Each node has its own TLS certificates and identity.

```yaml
ordererNodes:
  - id: 1
    host: "orderer1.org1.example.com"
    port: 7050
    mspId: "Org1MSP"
    clientTlsCert: "<BASE64_ENCODED_CLIENT_TLS_CERT>"
    serverTlsCert: "<BASE64_ENCODED_SERVER_TLS_CERT>"
    identity: "<BASE64_ENCODED_IDENTITY>"
```

**Fields:**
- `id`: Unique identifier for the orderer node
- `host`: Host address of the orderer node
- `port`: Port number for the orderer node
- `mspId`: MSP ID of the organization this orderer belongs to
- `clientTlsCert`: Base64-encoded client TLS certificate
- `serverTlsCert`: Base64-encoded server TLS certificate
- `identity`: Base64-encoded identity certificate

### Configuration Template

Reference to a ConfigMap containing the configtx.yaml template:

```yaml
configTemplate:
  configMapName: "fabricx-config-template"
  key: "configtx.yaml"
```

### Output Configuration

Specify where the generated genesis block should be stored:

```yaml
output:
  secretName: "fabricx-shared-genesis"
  blockKey: "genesis.block"
```

## Complete Example

Here's a complete example that demonstrates both internal and external organizations with specific orderer nodes:

```yaml
apiVersion: fabricx.kfsoft.tech/v1alpha1
kind: Genesis
metadata:
  name: shared-genesis
  namespace: fabric-network
spec:
  # Config template reference
  configTemplate:
    configMapName: "fabricx-config-template"
    key: "configtx.yaml"
  
  # Internal organizations (using Fabric CA)
  internalOrgs:
    - name: "Org1"
      mspId: "Org1MSP"
      caReference:
        name: "org1-ca"
        namespace: "fabric-network"
      adminIdentity: "admin"
      ordererIdentity: "orderer"
    
    - name: "Org2"
      mspId: "Org2MSP"
      caReference:
        name: "org2-ca"
        namespace: "fabric-network"
      adminIdentity: "admin"
  
  # External organizations (with provided certificates)
  externalOrgs:
    - name: "ExternalOrg1"
      mspId: "ExternalOrg1MSP"
      signCert: "<BASE64_ENCODED_SIGNING_CERT>"
      tlsCert: "<BASE64_ENCODED_TLS_CERT>"
      adminCert: "<BASE64_ENCODED_ADMIN_CERT>"
      ordererCert: "<BASE64_ENCODED_ORDERER_CERT>"
  
  # Specific orderer nodes for consensus
  ordererNodes:
    - id: 1
      host: "orderer1.org1.example.com"
      port: 7050
      mspId: "Org1MSP"
      clientTlsCert: "<BASE64_ENCODED_CLIENT_TLS_CERT>"
      serverTlsCert: "<BASE64_ENCODED_SERVER_TLS_CERT>"
      identity: "<BASE64_ENCODED_IDENTITY>"
    
    - id: 2
      host: "orderer2.org1.example.com"
      port: 7050
      mspId: "Org1MSP"
      clientTlsCert: "<BASE64_ENCODED_CLIENT_TLS_CERT>"
      serverTlsCert: "<BASE64_ENCODED_SERVER_TLS_CERT>"
      identity: "<BASE64_ENCODED_IDENTITY>"
    
    - id: 3
      host: "orderer1.externalorg1.example.com"
      port: 7050
      mspId: "ExternalOrg1MSP"
      clientTlsCert: "<BASE64_ENCODED_CLIENT_TLS_CERT>"
      serverTlsCert: "<BASE64_ENCODED_SERVER_TLS_CERT>"
      identity: "<BASE64_ENCODED_IDENTITY>"
  
  # Output configuration
  output:
    secretName: "fabricx-shared-genesis"
    blockKey: "genesis.block"
```

## Certificate Requirements

### For External Organizations

1. **Signing Certificate**: Used for transaction signing
2. **TLS Certificate**: Used for TLS connections
3. **Admin Certificate**: Used for administrative operations
4. **Orderer Certificate**: Used for orderer operations (only for orderer organizations)

### For Orderer Nodes

1. **Client TLS Certificate**: Used for client-side TLS connections
2. **Server TLS Certificate**: Used for server-side TLS connections
3. **Identity Certificate**: Used for node identity

All certificates must be provided in base64-encoded format.

## Usage

1. Create the ConfigMap with your configtx.yaml template
2. Deploy the Genesis resource
3. The operator will:
   - Fetch certificates from internal CAs
   - Use provided certificates for external organizations
   - Generate the genesis block
   - Store it in the specified Secret

## Status

The Genesis resource provides status information:

- `status.status`: Current status (Pending, Running, Failed, etc.)
- `status.message`: Description of the current state
- `status.conditions`: Detailed conditions about the resource

## Best Practices

1. **Certificate Management**: Ensure all certificates are valid and properly encoded
2. **MSP IDs**: Use consistent MSP ID naming conventions
3. **Orderer Nodes**: Ensure all orderer nodes are reachable and properly configured
4. **Security**: Store sensitive certificates securely and consider using Kubernetes Secrets
5. **Validation**: Validate the generated genesis block before using it in production 