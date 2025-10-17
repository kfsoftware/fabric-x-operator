# PostgreSQL Integration for Committer Components

This document describes how the Committer components (Validator and QueryService) integrate with PostgreSQL using CloudNativePG.

## Overview

The Fabric-X Committer stack includes two components that require PostgreSQL database:

1. **Validator** - Validates and prepares transactions before committing
2. **QueryService** - Provides query capabilities for committed transactions

Both components share the same PostgreSQL cluster and database (`queryservice`) but use different tables/schemas for their data.

## Architecture

```
┌─────────────────────────────────────────────┐
│         Committer (Parent Resource)         │
└─────────────────────────────────────────────┘
                    │
        ┌───────────┴───────────┐
        │                       │
        ▼                       ▼
┌──────────────┐        ┌─────────────────┐
│  Validator   │        │  QueryService   │
└──────────────┘        └─────────────────┘
        │                       │
        └───────────┬───────────┘
                    ▼
        ┌───────────────────────┐
        │  PostgreSQL Cluster   │
        │   (CloudNativePG)     │
        │                       │
        │ DB: queryservice      │
        │ User: queryservice    │
        └───────────────────────┘
```

## PostgreSQL Cluster Configuration

### Cluster Specification

File: `testing/postgresql-cluster.yaml`

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: postgresql
  namespace: default
spec:
  instances: 1                    # Single replica (increase for HA)

  postgresql:
    parameters:
      max_connections: "100"      # Maximum connections
      shared_buffers: "256MB"     # Memory for caching

  storage:
    size: 5Gi                     # Storage size
    storageClass: local-path      # Storage class

  bootstrap:
    initdb:
      database: queryservice       # Database name
      owner: queryservice          # Database owner
      secret:
        name: postgresql-credentials
```

### Services Created

CloudNativePG automatically creates three services:

1. **postgresql-rw** (Read-Write)
   - `postgresql-rw.default.svc.cluster.local:5432`
   - Used for writes and consistent reads
   - Routes to primary instance

2. **postgresql-r** (Read-Only)
   - `postgresql-r.default.svc.cluster.local:5432`
   - Used for read-only queries
   - Routes to any available replica

3. **postgresql-ro** (Read-Only)
   - `postgresql-ro.default.svc.cluster.local:5432`
   - Alternative read-only endpoint

## Committer Configuration

### Validator Component

```yaml
components:
  validator:
    replicas: 1
    postgresql:
      host: postgresql-rw.default.svc.cluster.local
      port: 5432
      database: queryservice
      username: queryservice
      passwordSecret:
        name: postgresql-credentials
        namespace: default
        key: password
      maxConnections: 100
      minConnections: 10
      loadBalance: true
      retry:
        maxElapsedTime: 5m
```

### QueryService Component

```yaml
components:
  queryService:
    replicas: 1
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

## Configuration Parameters

### Connection Settings

| Parameter | Description | Validator Default | QueryService Default |
|-----------|-------------|-------------------|----------------------|
| `host` | PostgreSQL service hostname | postgresql-rw.default.svc.cluster.local | postgresql-rw.default.svc.cluster.local |
| `port` | PostgreSQL port | 5432 | 5432 |
| `database` | Database name | queryservice | queryservice |
| `username` | Database user | queryservice | queryservice |

### Connection Pool Settings

| Parameter | Description | Validator Default | QueryService Default |
|-----------|-------------|-------------------|----------------------|
| `maxConnections` | Maximum connections in pool | 100 | 50 |
| `minConnections` | Minimum connections in pool | 10 | 10 |
| `loadBalance` | Enable connection load balancing | true | true |

### Retry Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `maxElapsedTime` | Maximum time for connection retry | 5m |

## Security

### Credentials Management

Passwords are stored in Kubernetes Secrets and referenced by the components:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-credentials
  namespace: default
type: kubernetes.io/basic-auth
stringData:
  username: queryservice
  password: queryservice123  # Change in production!
```

### Best Practices

1. **Use Strong Passwords**: Change default passwords in production
2. **Namespace Isolation**: Use separate namespaces for different environments
3. **Secret Rotation**: Rotate database credentials regularly
4. **TLS Encryption**: Enable SSL/TLS for database connections (see Advanced Configuration)
5. **RBAC**: Restrict access to PostgreSQL secrets

## Deployment Steps

### 1. Install CloudNativePG Operator

```bash
kubectl apply --server-side -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.24/releases/cnpg-1.24.0.yaml

kubectl wait --for=condition=Available \
  deployment/cnpg-controller-manager \
  -n cnpg-system --timeout=60s
```

### 2. Deploy PostgreSQL Cluster

```bash
kubectl apply -f testing/postgresql-cluster.yaml

kubectl wait --for=condition=Ready \
  cluster/postgresql \
  -n default --timeout=120s
```

### 3. Verify PostgreSQL

```bash
# Check cluster status
kubectl get cluster -n default

# Check pods
kubectl get pods -l cnpg.io/cluster=postgresql

# Check services
kubectl get svc -l cnpg.io/cluster=postgresql
```

### 4. Deploy Committer

```bash
kubectl apply -f config/samples/fabricx_v1alpha1_committer.yaml
```

### 5. Verify Components

```bash
# Check Validator
kubectl get committervalidator
kubectl logs -l app=validator

# Check QueryService
kubectl get committerqueryservice
kubectl logs -l app=query-service
```

## Troubleshooting

### Connection Issues

**Symptom**: Components can't connect to PostgreSQL

**Check**:
1. PostgreSQL cluster is ready: `kubectl get cluster`
2. Services exist: `kubectl get svc -l cnpg.io/cluster=postgresql`
3. Secrets exist: `kubectl get secret postgresql-credentials`
4. Network policies allow traffic

**Debug**:
```bash
# Test connection from a pod
kubectl run -it --rm debug --image=postgres:16 --restart=Never -- \
  psql -h postgresql-rw.default.svc.cluster.local \
       -U queryservice -d queryservice
```

### Authentication Failures

**Symptom**: Password authentication failed

**Check**:
1. Secret has correct password: `kubectl get secret postgresql-credentials -o yaml`
2. Secret referenced correctly in component spec
3. User exists in database

**Fix**:
```bash
# Connect as superuser
kubectl exec -it postgresql-1 -- psql -U postgres

# Check users
\du

# Reset password if needed
ALTER USER queryservice WITH PASSWORD 'new-password';
```

### Connection Pool Exhausted

**Symptom**: Too many connections errors

**Check**:
```bash
# Check current connections
kubectl exec -it postgresql-1 -- psql -U postgres -c \
  "SELECT count(*) FROM pg_stat_activity;"
```

**Fix**:
1. Increase `max_connections` in PostgreSQL cluster
2. Decrease `maxConnections` in component specs
3. Add more replicas to components

## Advanced Configuration

### High Availability

Increase instances for HA:

```yaml
spec:
  instances: 3  # 3 replicas for HA
```

### Read Replicas

Use read-only endpoints for queries:

```yaml
postgresql:
  host: postgresql-r.default.svc.cluster.local  # Read-only
```

### SSL/TLS Encryption

Enable TLS:

```yaml
postgresql:
  parameters:
    ssl: "on"
    ssl_cert_file: "/etc/certs/tls.crt"
    ssl_key_file: "/etc/certs/tls.key"
```

### Backup Configuration

Configure automated backups:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: postgresql-backup
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  cluster:
    name: postgresql
```

### Monitoring

Enable Prometheus monitoring:

```yaml
spec:
  monitoring:
    enabled: true
    podMonitorEnabled: true
```

## Performance Tuning

### PostgreSQL Parameters

```yaml
postgresql:
  parameters:
    max_connections: "200"           # Increase for more concurrent connections
    shared_buffers: "512MB"          # 25% of RAM
    effective_cache_size: "1536MB"   # 75% of RAM
    work_mem: "16MB"                 # Per-operation memory
    maintenance_work_mem: "128MB"    # For maintenance operations
    random_page_cost: "1.1"          # For SSDs
    effective_io_concurrency: "200"  # For SSDs
```

### Connection Pool Sizing

**Rule of thumb**:
- Total connections = (Validator replicas × maxConnections) + (QueryService replicas × maxConnections)
- PostgreSQL max_connections should be 2-3× total connections

**Example**:
- Validator: 2 replicas × 100 = 200
- QueryService: 2 replicas × 50 = 100
- Total: 300
- PostgreSQL max_connections: 600-900

## Migration Guide

### From External PostgreSQL

If migrating from external PostgreSQL to CloudNativePG:

1. **Backup existing data**:
```bash
pg_dump -h old-host -U user -d database > backup.sql
```

2. **Deploy CloudNativePG cluster**

3. **Restore data**:
```bash
kubectl exec -i postgresql-1 -- psql -U queryservice -d queryservice < backup.sql
```

4. **Update Committer configuration** to point to new cluster

5. **Test and verify** components work correctly

6. **Decommission** old PostgreSQL instance

## References

- [CloudNativePG Documentation](https://cloudnative-pg.io/documentation/)
- [PostgreSQL Best Practices](https://wiki.postgresql.org/wiki/Performance_Optimization)
- [Fabric-X Operator Quickstart](../QUICKSTART-POSTGRESQL.md)
