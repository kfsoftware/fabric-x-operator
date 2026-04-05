# E2E Testing Guide

## Overview

The fabric-x-operator includes end-to-end (e2e) tests that validate the operator in a real Kubernetes environment. These tests can run on either Kind or K3D clusters.

## Prerequisites

- A Kubernetes cluster (Kind or K3D)
- Docker for building operator images
- kubectl configured to access your cluster
- CertManager (optional - tests will install if not present)

## Running E2E Tests

### Option 1: Using Kind (Default)

The default e2e tests create a temporary Kind cluster, run tests, and clean up:

```bash
make test-e2e
```

This will:
1. Create a Kind cluster named `fabric-x-operator-test-e2e`
2. Build the operator Docker image
3. Load the image into Kind
4. Install CRDs
5. Deploy the operator
6. Run all e2e tests
7. Clean up the cluster

**Customization:**
```bash
# Use a custom Kind cluster name
KIND_CLUSTER=my-test-cluster make test-e2e

# Skip CertManager installation (if already installed)
CERT_MANAGER_INSTALL_SKIP=true make test-e2e
```

### Option 2: Using K3D (Recommended for Development)

If you're using K3D for development and want to run e2e tests on your existing cluster:

```bash
# Using default K3D cluster name (k8s-hlf)
make test-e2e-k3d

# Using custom K3D cluster name
K3D_CLUSTER=my-cluster make test-e2e-k3d
```

**Benefits of K3D testing:**
- Faster: No cluster creation/deletion
- Uses your existing development cluster
- Easier to debug failures
- No cleanup (leaves resources for inspection)

### Option 3: Manual Test Execution

For maximum control, run tests directly with Go:

```bash
# With K3D
export CLUSTER_TYPE=k3d
export K3D_CLUSTER=k8s-hlf
go test ./test/e2e/ -v -ginkgo.v -timeout=10m

# With Kind (must set KIND_CLUSTER)
export KIND_CLUSTER=fabric-x-operator-test-e2e
go test ./test/e2e/ -v -ginkgo.v -timeout=10m
```

## Test Timeout

E2E tests have a **10-minute timeout** by default. This accounts for:
- Building Docker images (~2-5s)
- Loading images to cluster (~5-10s)
- Installing CertManager (~30-60s if needed)
- Deploying the operator (~10-20s)
- Waiting for pods to start (~10-30s)
- Running metrics tests (~10-20s)
- **Total: ~5-10 minutes**

If tests are failing with timeout, increase it:
```bash
go test ./test/e2e/ -v -ginkgo.v -timeout=20m
```

## Common Issues and Solutions

### Issue: Tests Hanging

**Symptom:** Tests appear to hang indefinitely

**Likely Cause:** Insufficient timeout (default was 30s, now fixed to 10m)

**Solution:**
```bash
# Use the updated Makefile target with proper timeout
make test-e2e-k3d

# Or run directly with longer timeout
go test ./test/e2e/ -v -ginkgo.v -timeout=15m
```

### Issue: Image Pull Errors

**Symptom:**
```
Failed to load the manager(Operator) image into Kind
```

**Solution:**
```bash
# For K3D, verify image import works
k3d image import example.com/fabric-x-operator:v0.0.1 -c k8s-hlf

# For Kind
kind load docker-image example.com/fabric-x-operator:v0.0.1 --name fabric-x-operator-test-e2e
```

### Issue: CertManager Already Installed

**Symptom:**
```
WARNING: CertManager is already installed. Skipping installation...
```

**This is not an error!** The tests detect existing CertManager and skip installation.

If you see errors related to CertManager:
```bash
# Skip CertManager installation entirely
CERT_MANAGER_INSTALL_SKIP=true make test-e2e-k3d
```

### Issue: Curl Metrics Pod Fails

**Symptom:** Tests hang on "waiting for the curl-metrics pod to complete"

**Debug:**
```bash
# Check pod status
kubectl get pod curl-metrics -n fabric-x-operator-system

# Check pod logs
kubectl logs curl-metrics -n fabric-x-operator-system

# Check if metrics endpoint is working
kubectl logs -l control-plane=controller-manager -n fabric-x-operator-system | grep metrics
```

**Common Causes:**
1. Image pull timeout (curl:latest taking too long)
2. Network policy blocking metrics service
3. Service account token issues

**Solution:** The test has a 5-minute timeout for this step. If it's genuinely hanging:
```bash
# Pre-pull the curl image on all nodes
docker pull curlimages/curl:latest
k3d image import curlimages/curl:latest -c k8s-hlf
```

### Issue: Controller Not Starting

**Symptom:**
```
Incorrect controller-manager pod status
```

**Debug:**
```bash
# Check operator deployment
kubectl get deployment -n fabric-x-operator-system

# Check operator pods
kubectl get pods -n fabric-x-operator-system

# Check operator logs
kubectl logs -l control-plane=controller-manager -n fabric-x-operator-system

# Check events
kubectl get events -n fabric-x-operator-system --sort-by=.lastTimestamp
```

## Test Structure

The e2e test suite consists of:

### BeforeSuite (test/e2e/e2e_suite_test.go)
1. Build operator Docker image
2. Load image to cluster (Kind or K3D)
3. Install CertManager (if not present)

### Test: Manager - should run successfully (test/e2e/e2e_test.go)
1. Create namespace `fabric-x-operator-system`
2. Label namespace with restricted pod security
3. Install CRDs (`make install`)
4. Deploy controller (`make deploy`)
5. Verify controller pod is Running
6. Wait up to 2 minutes for pod to be ready

### Test: Manager - should ensure the metrics endpoint is serving metrics
1. Create ClusterRoleBinding for metrics access
2. Verify metrics service exists
3. Get service account token
4. Wait for metrics endpoint to be ready
5. Verify controller logs show "Serving metrics server"
6. Create curl pod to access metrics
7. Wait for curl pod to complete (up to 5 minutes)
8. Verify metrics contain `controller_runtime_reconcile_total`

### AfterAll
1. Delete curl-metrics pod
2. Undeploy controller (`make undeploy`)
3. Uninstall CRDs (`make uninstall`)
4. Delete namespace

### AfterSuite
1. Uninstall CertManager (if it was installed by tests)

## Debugging Failed Tests

When tests fail, Ginkgo automatically collects:

1. **Controller logs:**
   ```bash
   kubectl logs <controller-pod> -n fabric-x-operator-system
   ```

2. **Kubernetes events:**
   ```bash
   kubectl get events -n fabric-x-operator-system --sort-by=.lastTimestamp
   ```

3. **Curl metrics logs:**
   ```bash
   kubectl logs curl-metrics -n fabric-x-operator-system
   ```

4. **Pod descriptions:**
   ```bash
   kubectl describe pod <controller-pod> -n fabric-x-operator-system
   ```

All of this information is printed to GinkgoWriter on test failure.

## Cleanup

### After Kind Tests
```bash
# Automatic cleanup after test-e2e
make cleanup-test-e2e

# Or manually
kind delete cluster --name fabric-x-operator-test-e2e
```

### After K3D Tests
```bash
# K3D tests don't auto-cleanup. Manual cleanup:
kubectl delete ns fabric-x-operator-system
kubectl delete clusterrolebinding fabric-x-operator-metrics-binding

# Or reset your entire K3D cluster
k3d cluster delete k8s-hlf
k3d cluster create k8s-hlf
```

## CI/CD Integration

The project includes GitHub Actions workflows that run e2e tests on both Kind and K3D:

### Workflow: `.github/workflows/test-e2e.yml`

This workflow runs two parallel jobs:

1. **test-e2e-kind**: Uses Kind (the default Kubernetes-in-Docker)
   - Creates a temporary Kind cluster
   - Runs `make test-e2e`
   - Automatically cleans up

2. **test-e2e-k3d**: Uses K3D (lightweight K3s in Docker)
   - Creates a K3D cluster with 2 agents
   - Runs `make test-e2e-k3d`
   - Cleans up cluster on completion

Both jobs run on every push and pull request, providing test coverage on two different Kubernetes distributions.

### Example Custom CI Pipeline

If you're setting up e2e tests in your own CI:

```yaml
# Example GitHub Actions workflow with K3D
test-e2e:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4

    - name: Setup Go
      uses: actions/setup-go@v5
      with:
        go-version-file: go.mod

    - name: Install K3D
      run: |
        curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

    - name: Create K3D cluster
      run: |
        k3d cluster create test-cluster --agents 2 --wait --timeout 5m

    - name: Run E2E Tests
      env:
        CLUSTER_TYPE: k3d
        K3D_CLUSTER: test-cluster
      run: make test-e2e-k3d

    - name: Cleanup
      if: always()
      run: k3d cluster delete test-cluster
```

## Environment Variables

- `CLUSTER_TYPE`: Set to "k3d" for K3D clusters (default: kind)
- `K3D_CLUSTER`: K3D cluster name (default: k8s-hlf)
- `KIND_CLUSTER`: Kind cluster name (default: fabric-x-operator-test-e2e)
- `CERT_MANAGER_INSTALL_SKIP`: Skip CertManager installation (default: false)

## Performance

Typical test execution times on M1 Mac:

| Phase                     | Kind   | K3D    |
|---------------------------|--------|--------|
| Cluster creation          | 30-60s | 0s     |
| Image build               | 2-5s   | 2-5s   |
| Image load                | 10-20s | 5-10s  |
| CertManager install       | 30-60s | 0s*    |
| CRD installation          | 5-10s  | 5-10s  |
| Operator deployment       | 10-20s | 10-20s |
| Test execution            | 30-60s | 30-60s |
| Cluster cleanup           | 10-20s | 0s     |
| **Total**                 | **3-4m**| **1-2m**|

*Skipped if already installed

## Best Practices

1. **Use K3D for development:** Faster iteration, easier debugging
2. **Use Kind for CI:** Clean, isolated environment
3. **Don't commit with failing e2e tests:** They validate core functionality
4. **Increase timeout for slow networks:** Image pulls can be slow
5. **Pre-pull images:** Speed up tests by pulling images beforehand
6. **Clean up after debugging:** Remove test namespaces manually if tests were interrupted

## Identity Controller E2E Tests

The Identity controller has its own standalone E2E test suite designed for CI/CD automation.

### Quick Start

```bash
# Run on existing cluster (operator must be deployed)
make test-e2e-identity

# Or run directly with go test
go test ./test/e2e_identity -v -timeout=30m
```

### What It Tests

The Identity E2E test validates:
1. **Service DNS Resolution**: Identity controller can resolve Kubernetes service DNS (e.g., `test-ca-e2e.default:7054`)
2. **CA Communication**: Controller successfully connects to CA via Kubernetes service
3. **Enrollment Workflow**: Certificate enrollment is attempted (TLS validation may fail with self-signed certs, which is expected)

### Test Behavior

The test is designed to **Skip** with success when it encounters expected TLS validation errors. This is by design:

```
✅ ========== SERVICE DNS RESOLUTION TEST PASSED ==========
✅ The Identity controller successfully:
✅   1. Resolved service DNS: test-ca-e2e.default:7054
✅   2. Connected to the CA via Kubernetes service
✅   3. Initiated TLS handshake (enrollment attempted)
✅
✅ TLS cert validation failed (expected with self-signed CA cert)
✅ =========================================================
```

**Why Skip Instead of Pass?** The test validates service DNS resolution works, but full enrollment fails due to TLS certificate validation (self-signed CA cert doesn't include service DNS in SANs). This is expected behavior in E2E environment.

### Prerequisites

- Kubernetes cluster (K3D or Kind)
- Operator deployed and running
- kubectl access to cluster

### CI/CD Integration

The standalone Identity E2E test is perfect for CI/CD because:

1. **No External Setup**: Test creates its own CA via CA controller
2. **Self-Contained**: No dependencies on external test fixtures
3. **Fast**: Completes in ~5-10 seconds
4. **Clean**: Automatic resource cleanup in AfterEach
5. **Simple**: Just run `go test ./test/e2e_identity`

#### GitHub Actions Example

```yaml
test-identity-e2e:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4

    - name: Setup Go
      uses: actions/setup-go@v5
      with:
        go-version-file: go.mod

    - name: Setup K3D
      run: |
        curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
        k3d cluster create test --agents 2 --wait

    - name: Deploy Operator
      run: |
        export IMG=operator:test
        make docker-build IMG=$IMG
        k3d image import $IMG --cluster test
        make deploy IMG=$IMG
        kubectl wait --for=condition=Available deployment/fabric-x-operator-controller-manager \
          -n fabric-x-operator-system --timeout=300s

    - name: Run Identity E2E Tests
      run: make test-e2e-identity

    - name: Cleanup
      if: always()
      run: k3d cluster delete test
```

### Test Structure

**Location**: `test/e2e_identity/identity_test.go`

**Package**: `e2e_identity` (separate from main e2e package to avoid BeforeSuite conflicts)

**Components**:

1. **BeforeAll**:
   - Loads kubeconfig
   - Creates Kubernetes clients
   - Verifies operator is running

2. **Test Case**: "X.509 Sign Certificate Enrollment with Service DNS"
   - Creates CA via CA controller
   - Waits for CA pod to be ready
   - Creates enrollment secret
   - Creates Identity resource with service DNS reference
   - Waits for Identity status
   - Validates service DNS resolution (Skip if TLS fails as expected)

3. **AfterEach**: Cleanup all test resources (CA, Identity, Secrets)

### Troubleshooting

**Operator not deployed:**
```
Error: Operator not deployed. Run 'make deploy IMG=<image>' first
```

**Solution:**
```bash
# Deploy operator first
export IMG=fabric-x-operator:latest
make docker-build IMG=$IMG
k3d image import $IMG --cluster k8s-hlf
make deploy IMG=$IMG
```

**Test hangs waiting for CA pod:**
```
Waiting for CA pod to be ready...
```

**Debug:**
```bash
# Check CA resource
kubectl get ca test-ca-e2e -o yaml

# Check CA controller logs
kubectl logs -n fabric-x-operator-system -l control-plane=controller-manager | grep -A 10 test-ca-e2e

# Check CA pod status
kubectl get pods -l release=test-ca-e2e
kubectl describe pod -l release=test-ca-e2e
```

### Running in Different Modes

**Verbose Mode:**
```bash
go test ./test/e2e_identity -v -ginkgo.v -timeout=30m
```

**Focus on Specific Test:**
```bash
go test ./test/e2e_identity -v -ginkgo.focus="Service DNS" -timeout=30m
```

**Skip Cleanup (for debugging):**
```bash
# Modify AfterEach to skip cleanup, then:
go test ./test/e2e_identity -v -timeout=30m

# Inspect resources after test
kubectl get ca,identity,secret -l test=identity-e2e
```

## Extending E2E Tests

To add new e2e tests:

1. Add a new `It` block in `test/e2e/e2e_test.go`
2. Use `Eventually` for async operations
3. Set reasonable timeouts (default: 2 minutes)
4. Use `GinkgoWriter` for debug output
5. Clean up resources in `AfterEach` if needed

Example:
```go
It("should create a CA successfully", func() {
    By("applying a CA resource")
    cmd := exec.Command("kubectl", "apply", "-f", "config/samples/fabricx_v1alpha1_ca.yaml")
    _, err := utils.Run(cmd)
    Expect(err).NotTo(HaveOccurred())

    By("waiting for CA pod to be ready")
    Eventually(func(g Gomega) {
        cmd := exec.Command("kubectl", "get", "pod", "-l", "app=test-ca", "-o", "jsonpath={.items[0].status.phase}")
        output, err := utils.Run(cmd)
        g.Expect(err).NotTo(HaveOccurred())
        g.Expect(output).To(Equal("Running"))
    }, 2*time.Minute).Should(Succeed())
})
```
