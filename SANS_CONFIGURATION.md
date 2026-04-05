# SANS (Subject Alternative Names) Configuration

This document explains how to configure Subject Alternative Names (SANS) for TLS certificates in the Fabric X Operator.

## Overview

Subject Alternative Names (SANS) allow you to specify additional hostnames and IP addresses that should be included in TLS certificates. This is essential for ensuring that certificates are valid for all the ways your services can be accessed.

## SANS Configuration Structure

The SANS configuration is available under the `ca` section in certificate configurations. This structure allows for future expansion to support other certificate generation methods (like Vault) while keeping the SANS configuration organized.

### 1. OrdererGroup Certificate Configuration

In `OrdererGroup` resources, you can configure SANS for both global enrollment and component-specific certificates:

```yaml
spec:
  enrollment:
    sign:
      # ... other certificate config ...
      ca:
        sans:
          dnsNames:
            - "orderer1.example.com"
            - "orderer1.org1.example.com"
            - "*.orderer1.org1.example.com"
          ipAddresses:
            - "192.168.1.10"
            - "10.0.0.10"
    tls:
      # ... other certificate config ...
      ca:
        sans:
          dnsNames:
            - "orderer1-tls.example.com"
            - "orderer1-tls.org1.example.com"
          ipAddresses:
            - "192.168.1.11"
            - "10.0.0.11"
```

### 2. Component-Specific SANS

Each component can have its own SANS configuration:

```yaml
spec:
  components:
    consenter:
      certificates:
        # ... other certificate config ...
        ca:
          sans:
            dnsNames:
              - "consenter1.example.com"
              - "consenter1.org1.example.com"
            ipAddresses:
              - "192.168.1.20"
              - "10.0.0.20"
```

## SANS Configuration Fields

### DNSNames
A list of DNS names (hostnames) to include in the certificate. These are typically the domain names that clients will use to connect to your services.

```yaml
dnsNames:
  - "service.example.com"
  - "service.org1.example.com"
  - "*.service.org1.example.com"  # Wildcard for subdomains
```

### IPAddresses
A list of IP addresses to include in the certificate. These can be IPv4 or IPv6 addresses.

```yaml
ipAddresses:
  - "192.168.1.100"
  - "10.0.0.100"
  - "2001:db8::1"  # IPv6 address
```

## Configuration Structure

The SANS configuration is nested under the `ca` section to allow for future expansion:

```yaml
certificates:
  cahost: "ca.example.com"
  caport: 7054
  caname: "ca-org1"
  enrollid: "admin"
  enrollsecret: "adminpw"
  catls:
    cacert: "LS0tLS1CRUdJTi..."
    secretRef:
      name: "ca-tls-secret"
      key: "ca.crt"
      namespace: "default"
  # CA configuration for certificate generation
  ca:
    sans:
      dnsNames:
        - "service.example.com"
        - "service.org1.example.com"
      ipAddresses:
        - "192.168.1.100"
        - "10.0.0.100"
```

## Best Practices

### 1. Include All Access Methods
Make sure to include all the ways your service can be accessed:
- External domain names
- Internal domain names
- Load balancer IPs
- Service IPs
- Localhost (for development)

### 2. Use Wildcards Appropriately
Wildcards can be useful but should be used carefully:
```yaml
dnsNames:
  - "*.org1.example.com"  # Covers all subdomains
  - "service.org1.example.com"  # Specific service
```

### 3. Include Internal and External Addresses
For services that need to be accessible both internally and externally:

```yaml
ca:
  sans:
    dnsNames:
      - "service.example.com"  # External
      - "service.internal"     # Internal
      - "service.org1.svc.cluster.local"  # Kubernetes service
    ipAddresses:
      - "192.168.1.100"  # External IP
      - "10.0.0.100"     # Internal IP
```

### 4. Security Considerations
- Only include the minimum necessary SANS
- Avoid overly broad wildcards
- Regularly review and update SANS as your infrastructure changes

## Examples

### Complete OrdererGroup Example
See `config/samples/fabricx_v1alpha1_orderergroup_sans_example.yaml` for a complete example of SANS configuration in an OrdererGroup.

## Implementation Notes

The SANS configuration is used when generating TLS certificates through the Fabric CA. The operator will:

1. Read the SANS configuration from the `ca.sans` section
2. Pass the SANS information to the certificate generation process
3. Include the specified DNS names and IP addresses in the generated certificates

This ensures that the generated certificates are valid for all the specified access methods.

## Future Considerations

The `ca` section structure allows for future expansion to support:
- Vault-based certificate generation
- Other certificate authorities
- Additional certificate configuration options
