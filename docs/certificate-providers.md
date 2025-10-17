# Certificate Provider Abstraction

The Fabric X Operator supports multiple certificate providers through a pluggable interface. This allows you to choose the best certificate management solution for your environment.

## Supported Providers

1. **Fabric CA** - Hyperledger Fabric Certificate Authority
2. **Manual** - User-provided certificates from Kubernetes secrets
3. **Hashicorp Vault** - Vault PKI (stub implementation)

## Architecture

The certificate provider abstraction consists of:

- **`CertificateProvider` interface** - Common interface for all providers
- **Provider implementations** - Fabric CA, Manual, and Vault
- **Provider factory** - Creates providers based on configuration
- **API types** - CRD fields for provider configuration

### Interface

```go
type CertificateProvider interface {
    ProvisionSignCertificate(ctx context.Context, req SignCertificateRequest) (*CertificateData, error)
    ProvisionTLSCertificate(ctx context.Context, req TLSCertificateRequest) (*CertificateData, error)
    Name() string
}
```

## Usage Examples

### Fabric CA Provider (Default)

This is the default provider when using the existing `ca` field (backward compatible):

```yaml
apiVersion: fabricx.kfs.dev/v1alpha1
kind: Endorser
metadata:
  name: endorser1
spec:
  mspid: Org1MSP
  bootstrapMode: configure

  # Backward compatible: uses Fabric CA by default
  enrollment:
    sign:
      ca:
        caHost: ca.org1.example.com
        caPort: 7054
        caName: ca-org1
        enrollID: endorser1
        enrollSecret: endorser1pw
        caTLS:
          cacert: "LS0tLS1CRUdJTi..." # Base64 CA cert
    tls:
      ca:
        caHost: ca.org1.example.com
        caPort: 7054
        caName: ca-org1
        enrollID: endorser1-tls
        enrollSecret: endorser1tlspw
        caTLS:
          secretRef:
            name: ca-tls-cert
            key: ca.pem
            namespace: default
      sans:
        dnsNames:
          - endorser1.org1.svc.cluster.local
          - endorser1.org1.example.com
```

### Fabric CA Provider (New Syntax)

Use the new `provider` field with explicit provider type:

```yaml
apiVersion: fabricx.kfs.dev/v1alpha1
kind: Endorser
metadata:
  name: endorser1
spec:
  mspid: Org1MSP
  bootstrapMode: configure

  enrollment:
    sign:
      providerType: fabric-ca
      provider:
        fabricCA:
          caHost: ca.org1.example.com
          caPort: 7054
          caName: ca-org1
          enrollID: endorser1
          enrollSecret: endorser1pw
          caTLS:
            cacert: "LS0tLS1CRUdJTi..."
    tls:
      providerType: fabric-ca
      provider:
        fabricCA:
          caHost: ca.org1.example.com
          caPort: 7054
          enrollID: endorser1-tls
          enrollSecret: endorser1tlspw
          caTLS:
            secretRef:
              name: ca-tls-cert
              key: ca.pem
      sans:
        dnsNames:
          - endorser1.org1.svc.cluster.local
```

### Manual Provider

Use pre-existing certificates from Kubernetes secrets:

```yaml
apiVersion: fabricx.kfs.dev/v1alpha1
kind: Endorser
metadata:
  name: endorser1
spec:
  mspid: Org1MSP
  bootstrapMode: configure

  enrollment:
    sign:
      providerType: manual
      provider:
        manual:
          secretRef:
            name: endorser1-sign-cert
            namespace: default
          # Optional: custom key names (defaults shown)
          certKey: cert.pem
          keyKey: key.pem
          caKey: ca.pem
    tls:
      providerType: manual
      provider:
        manual:
          secretRef:
            name: endorser1-tls-cert
            namespace: default
```

The referenced secret should contain:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: endorser1-sign-cert
type: Opaque
data:
  cert.pem: <base64-encoded-certificate>
  key.pem: <base64-encoded-private-key>
  ca.pem: <base64-encoded-ca-certificate>
```

### Hashicorp Vault Provider (Stub)

Use Vault PKI for dynamic certificate generation:

```yaml
apiVersion: fabricx.kfs.dev/v1alpha1
kind: Endorser
metadata:
  name: endorser1
spec:
  mspid: Org1MSP
  bootstrapMode: configure

  enrollment:
    sign:
      providerType: vault
      provider:
        vault:
          address: https://vault.example.com:8200
          pkiPath: fabric-pki
          role: fabric-endorser
          authMethod: kubernetes
          serviceAccount: fabric-operator
          ttl: 8760h  # 1 year
    tls:
      providerType: vault
      provider:
        vault:
          address: https://vault.example.com:8200
          pkiPath: fabric-pki
          role: fabric-endorser
          authMethod: token
          tokenSecretRef:
            name: vault-token
            key: token
          namespace: fabric  # Vault namespace (Enterprise)
          ttl: 8760h
      sans:
        dnsNames:
          - endorser1.org1.svc.cluster.local
```

**Note:** Vault provider is currently a stub and will return "not yet implemented" errors.

## Provider Configuration Reference

### Common Fields

All certificate configurations support:

- `providerType` - Provider type: `fabric-ca`, `manual`, or `vault`
- `provider` - Provider-specific configuration
- `sans` - Subject Alternative Names (for TLS certificates only)

### Fabric CA Configuration

```yaml
provider:
  fabricCA:
    caHost: string        # Required: CA server hostname
    caPort: int32         # Required: CA server port
    caName: string        # Optional: CA name for multi-CA servers
    enrollID: string      # Required: Enrollment ID
    enrollSecret: string  # Required: Enrollment secret
    caTLS:                # Optional: CA TLS certificate
      cacert: string      # Option 1: Inline base64-encoded cert
      secretRef:          # Option 2: Reference to K8s secret
        name: string
        key: string
        namespace: string
```

### Manual Configuration

```yaml
provider:
  manual:
    secretRef:            # Required: Reference to certificate secret
      name: string        # Required: Secret name
      namespace: string   # Optional: defaults to "default"
    certKey: string       # Optional: Key for certificate (default: "cert.pem")
    keyKey: string        # Optional: Key for private key (default: "key.pem")
    caKey: string         # Optional: Key for CA cert (default: "ca.pem")
```

### Vault Configuration

```yaml
provider:
  vault:
    address: string       # Required: Vault server address (e.g., "https://vault:8200")
    pkiPath: string       # Required: PKI mount path (e.g., "pki" or "fabric-pki")
    role: string          # Required: Role name for certificate issuance
    authMethod: string    # Required: "kubernetes" or "token"
    serviceAccount: string # Required if authMethod=kubernetes
    tokenSecretRef:       # Required if authMethod=token
      name: string
      key: string
      namespace: string
    namespace: string     # Optional: Vault namespace (Enterprise only)
    ttl: string           # Optional: Certificate TTL (e.g., "8760h"), uses role default if not specified
```

## Migration Guide

### From Old CA Field to New Provider Field

Old syntax (still supported):

```yaml
enrollment:
  sign:
    ca:
      caHost: ca.example.com
      caPort: 7054
```

New syntax:

```yaml
enrollment:
  sign:
    providerType: fabric-ca
    provider:
      fabricCA:
        caHost: ca.example.com
        caPort: 7054
```

Both syntaxes work and are fully backward compatible. The operator will automatically detect the old `ca` field and treat it as Fabric CA.

## Adding Custom Providers

To add a new certificate provider:

1. **Implement the `CertificateProvider` interface:**

```go
type MyProvider struct{}

func (p *MyProvider) Name() string {
    return "my-provider"
}

func (p *MyProvider) ProvisionSignCertificate(ctx context.Context, req SignCertificateRequest) (*CertificateData, error) {
    // Implementation
}

func (p *MyProvider) ProvisionTLSCertificate(ctx context.Context, req TLSCertificateRequest) (*CertificateData, error) {
    // Implementation
}
```

2. **Register the provider in the factory:**

```go
certs.DefaultProviderFactory.Register("my-provider", func() certs.CertificateProvider {
    return &MyProvider{}
})
```

3. **Add API types** in `api/v1alpha1/orderergroup_types.go`:

```go
type CertificateProviderConfig struct {
    // ... existing providers ...
    MyProvider *MyProviderConfig `json:"myProvider,omitempty"`
}

type MyProviderConfig struct {
    // Your provider-specific configuration
}
```

4. **Add conversion logic** in `internal/controller/certs/api_converter.go`

5. **Use in CRDs:**

```yaml
enrollment:
  sign:
    providerType: my-provider
    provider:
      myProvider:
        # Your configuration
```

## Implementation Details

### File Structure

```
internal/controller/certs/
├── provider.go                 # Interface and types
├── provider_factory.go         # Provider registry
├── provider_fabricca.go        # Fabric CA implementation
├── provider_manual.go          # Manual implementation
├── provider_vault.go           # Vault stub
├── api_converter.go            # API type conversion
└── provision.go                # High-level helper functions
```

### Cross-Controller Usage

The provider abstraction works across all controllers:

- **Endorser** - Uses providers for component certificates
- **Committer** - Uses providers for component certificates
- **OrdererGroup** - Uses providers for consenter/batcher/assembler/router certificates

All controllers use the same `certs.ProvisionCertificates()` function with different configurations.

## Future Enhancements

Potential future providers:

- **AWS Certificate Manager (ACM)** - AWS-managed certificates
- **Google Certificate Authority Service** - GCP-managed certificates
- **cert-manager** - Kubernetes-native certificate management
- **Let's Encrypt** - Free automated certificates (via cert-manager)
- **Azure Key Vault** - Azure-managed certificates

## Troubleshooting

### "Unknown certificate provider type" error

Ensure `providerType` is one of: `fabric-ca`, `manual`, or `vault`.

### "Provider configuration is required" error

The `provider` field must contain the provider-specific configuration matching the `providerType`.

### "Fabric CA provider requires CA configuration" error

When using `providerType: fabric-ca`, you must provide either:
- The `provider.fabricCA` field, OR
- The legacy `ca` field (for backward compatibility)

### "Manual provider requires manual configuration" error

When using `providerType: manual`, you must provide `provider.manual.secretRef`.

### "Vault provider not yet implemented" error

The Vault provider is currently a stub. To implement it, see the TODOs in `internal/controller/certs/provider_vault.go`.

## Testing

All providers are tested through the existing Endorser controller tests. To run the tests:

```bash
make test
```

Or run specific Endorser tests:

```bash
go test ./internal/controller -v -ginkgo.focus="Endorser Controller"
```
