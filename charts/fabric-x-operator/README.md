# fabric-x-operator

A Helm chart for the Hyperledger Fabric X Kubernetes Operator.

## Install

```bash
helm install fabric-x-operator ./charts/fabric-x-operator \
  --namespace fabric-x-operator-system \
  --create-namespace
```

## Install with custom image

```bash
helm install fabric-x-operator ./charts/fabric-x-operator \
  --namespace fabric-x-operator-system \
  --create-namespace \
  --set image.repository=kfsoftware/fabric-x-operator \
  --set image.tag=0.0.1
```

## Uninstall

```bash
helm uninstall fabric-x-operator -n fabric-x-operator-system
```

CRDs in `crds/` are NOT deleted on uninstall (Helm convention). To remove them:

```bash
kubectl delete crd -l app.kubernetes.io/part-of=fabric-x-operator
```

## Values

See [values.yaml](./values.yaml) for the full list of configurable values.

Key values:

| Key | Default | Description |
|-----|---------|-------------|
| `image.repository` | `kfsoftware/fabric-x-operator` | Container image |
| `image.tag` | Chart `appVersion` | Image tag |
| `replicaCount` | `1` | Number of replicas |
| `installCRDs` | `true` | Install CRDs with the chart |
| `rbac.create` | `true` | Create ClusterRole/ClusterRoleBinding |
| `leaderElection.enabled` | `true` | Enable leader election |
| `metrics.enabled` | `true` | Expose metrics endpoint |
| `metrics.serviceMonitor.enabled` | `false` | Create Prometheus ServiceMonitor |

## Managing CRDs

By convention, Helm does not upgrade or delete CRDs in `crds/`. If you need to
update CRDs after a chart upgrade:

```bash
kubectl apply -f charts/fabric-x-operator/crds/
```
