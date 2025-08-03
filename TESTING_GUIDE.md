# Testing OrdererGroup with Fabric CA

## Quick Test Setup

### 1. Prerequisites
- Fabric CA server running at `test-ca2:7054`
- CA certificate secret `test-ca2` in namespace `default`
- Fabric X Operator deployed and running

### 2. Create CA Certificate Secret
```bash
# If you have the CA certificate file:
kubectl create secret generic test-ca2 \
  --from-file=ca-cert.pem=/path/to/ca-cert.pem \
  --namespace=default

# Or if you have the certificate content:
kubectl create secret generic test-ca2 \
  --from-literal=ca-cert.pem='-----BEGIN CERTIFICATE-----...' \
  --namespace=default
```

### 3. Apply OrdererGroup
```bash
kubectl apply -f test-orderergroup-ca.yaml
```

### 4. Monitor Progress
```bash
# Check OrdererGroup status
kubectl get orderergroup test-orderergroup -n default -o yaml

# Check operator logs
kubectl logs -f deployment/fabric-x-operator -n fabric-x-system

# Check generated certificates
kubectl get secrets -l fabric-x/orderergroup=test-orderergroup -n default

# Check specific certificates
kubectl describe secret test-orderergroup-consenter-sign-cert -n default
kubectl describe secret test-orderergroup-consenter-tls-cert -n default
```

### 5. Cleanup
```bash
kubectl delete orderergroup test-orderergroup -n default
```

## Expected Behavior

1. **Certificate Generation**: The operator will connect to `test-ca2:7054` and enroll each component
2. **Secret Creation**: 8 certificate secrets will be created (sign + tls for each of 4 components)
3. **Component Deployment**: Components will be deployed with proper certificate mounts

## Troubleshooting

- **CA Connection Failed**: Verify `test-ca2` is accessible and the CA certificate is correct
- **Authentication Failed**: Check that `admin/adminpw` credentials are valid
- **Secret Not Found**: Ensure the CA certificate secret exists in the default namespace 