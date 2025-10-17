# Quickstart Guide: CommitterQueryService with PostgreSQL

This guide shows how to deploy the CommitterQueryService with PostgreSQL database using CloudNativePG (CNPG).

## Prerequisites

- Kubernetes cluster (k3d, minikube, or any other)
- kubectl configured
- fabric-x-operator deployed

## Step 1: Install CloudNativePG Operator

Deploy the CNPG operator to manage PostgreSQL clusters:

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.24/releases/cnpg-1.24.0.yaml
```

Wait for the operator to be ready:

```bash
kubectl wait --for=condition=Available deployment/cnpg-controller-manager -n cnpg-system --timeout=60s
```

## Step 2: Create PostgreSQL Cluster

Apply the PostgreSQL cluster configuration:

```bash
kubectl apply -f testing/postgresql-cluster.yaml
```

This creates:
- **PostgreSQL Cluster**: 1 replica with 5Gi storage
- **Database**: `queryservice`
- **User**: `queryservice` / `queryservice123`
- **Services**:
  - `postgresql-rw.default.svc.cluster.local:5432` (read-write)
  - `postgresql-r.default.svc.cluster.local:5432` (read-only)
  - `postgresql-ro.default.svc.cluster.local:5432` (read-only)

Wait for PostgreSQL to be ready:

```bash
kubectl wait --for=condition=Ready cluster/postgresql -n default --timeout=120s
```

Verify PostgreSQL is running:

```bash
kubectl get cluster -n default
kubectl get pods -l cnpg.io/cluster=postgresql
```

## Step 3: Deploy CommitterQueryService

### Option A: Via Committer Resource (Recommended)

Deploy the full Committer stack which includes QueryService:

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_committer.yaml
```

The QueryService will automatically be configured with PostgreSQL connection details:

```yaml
components:
  queryService:
    postgresql:
      host: postgresql-rw.default.svc.cluster.local
      port: 5432
      database: queryservice
      username: queryservice
      passwordSecret:
        name: postgresql-credentials
        namespace: default
        key: password
      maxConnections: 50
      minConnections: 10
      loadBalance: true
      retry:
        maxElapsedTime: 5m
```

### Option B: Standalone QueryService

Deploy QueryService independently:

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_committerqueryservice.yaml
```

## Step 4: Verify Deployment

Check QueryService status:

```bash
kubectl get committerqueryservice -n default
kubectl get pods -l app=query-service
```

Expected output:
```
NAME                               STATE     MESSAGE
fabric-x-committer-query-service   RUNNING   CommitterQueryService reconciled successfully
```

Check the logs:

```bash
kubectl logs -l app=query-service -n default
```

You should see database connection information:
```
Starting Query-Service
DB source: postgres://queryservice:***@postgresql-rw.default.svc.cluster.local:5432/queryservice?sslmode=disable
```

## Step 5: Verify Database Configuration

View the generated configuration:

```bash
kubectl get secret fabric-x-committer-query-service-config -n default -o jsonpath='{.data.config\.yaml}' | base64 -d
```

You should see:

```yaml
server:
  endpoint: 0.0.0.0:9001

# Resource limits
min-batch-keys: 1024
max-batch-wait: 100ms
view-aggregation-window: 100ms
max-aggregated-views: 1024
max-view-timeout: 10s

# Database configuration
database:
  endpoints:
    - postgresql-rw.default.svc.cluster.local:5432
  username: queryservice
  password: queryservice123
  database: queryservice
  max-connections: 50
  min-connections: 10
  load-balance: true
  retry:
    max-elapsed-time: 5m

party-id: 1
msp-id: CommitterMSP
```

## Step 6: Access QueryService

Forward the port to access QueryService locally:

```bash
kubectl port-forward svc/fabric-x-committer-query-service 9001:9001 -n default
```

Or access via Istio ingress (if enabled):
```
https://query-service-committer.localho.st
```

## Troubleshooting

### PostgreSQL Not Ready

Check PostgreSQL cluster status:

```bash
kubectl describe cluster postgresql -n default
kubectl logs -l cnpg.io/cluster=postgresql -n default
```

### QueryService Connection Issues

Check if password secret exists:

```bash
kubectl get secret postgresql-credentials -n default
```

View QueryService pod events:

```bash
kubectl describe pod -l app=query-service -n default
```

### Database Connection Errors

Connect to PostgreSQL directly to verify:

```bash
kubectl exec -it postgresql-1 -n default -- psql -U queryservice -d queryservice
```

## Configuration Options

### PostgreSQL Settings

You can customize PostgreSQL configuration in the Cluster spec:

```yaml
postgresql:
  parameters:
    max_connections: "200"
    shared_buffers: "512MB"
    work_mem: "16MB"
```

### QueryService Database Settings

Adjust connection pool settings:

```yaml
postgresql:
  maxConnections: 100    # Maximum connections
  minConnections: 20     # Minimum connections
  loadBalance: true      # Enable load balancing
  retry:
    maxElapsedTime: 10m  # Retry timeout
```

## Cleanup

Remove QueryService:

```bash
kubectl delete committerqueryservice fabric-x-committer-query-service -n default
# or
kubectl delete committer fabric-x-committer -n default
```

Remove PostgreSQL cluster:

```bash
kubectl delete -f testing/postgresql-cluster.yaml
```

Remove CNPG operator:

```bash
kubectl delete -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.24/releases/cnpg-1.24.0.yaml
```

## Next Steps

- Configure monitoring and metrics
- Set up backup and recovery
- Configure high availability (multiple replicas)
- Integrate with Istio service mesh
- Set up SSL/TLS for PostgreSQL connections

## Additional Resources

- [CloudNativePG Documentation](https://cloudnative-pg.io/documentation/)
- [Fabric-X Operator Documentation](./README.md)
- [CommitterQueryService API Reference](./api/v1alpha1/committerqueryservice_types.go)
