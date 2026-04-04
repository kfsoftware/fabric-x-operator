# Fabric X Operator

Kubernetes operator for Hyperledger Fabric X using kubebuilder. Manages CAs, orderer groups (router, batcher, consenter, assembler), committers (coordinator, sidecar, validator, verifier, query service), endorsers, identities, genesis blocks, and chain namespaces.

## Architecture

- Two-phase bootstrap: `configure` mode enrolls certificates, `deploy` mode creates workloads
- Hash-based rollout detection via `fabricx.kfsoft.tech/config-hash` pod annotation triggers rolling updates when Secrets/ConfigMaps change
- Committer sidecar discovers orderer endpoints from genesis block using `bootstrap.genesis-block-file-path`, not from inline config
- Top-level `enrollment` on the Committer CR propagates to all sub-components as fallback
- All orderer components need MSP `config.yaml` with NodeOUs in their init containers

## Development

- Run `make manifests generate` after changing API types; scope paths to `./api/...;./internal/...;./cmd/...` (excludes kubectl-fabricx)
- Run `make test-unit` for CI-safe tests (`-short` flag skips identity E2E tests)
- Run `make test-e2e-k3d` for full E2E against a K3D cluster
- Build and deploy: `export IMG=local/fabric-x-operator:$(date +%Y%m%d%H%M%S) && make docker-build IMG=$IMG && k3d image import $IMG --cluster k8s-hlf && make deploy IMG=$IMG`
- Default image versions: orderer `0.0.24`, committer `0.1.9`, CA `1.5.15`
- Committer v0.1.9 CLI uses `start-<service>` format (not `start <service>`)
- Dockerfile base image must match go.mod Go version (currently `golang:1.25`)

## Testing a local network

1. Deploy CA, then orderer groups in `configure` mode
2. Create genesis block (use internal cluster endpoints, not external hostnames)
3. Patch orderer groups to `deploy` mode
4. Deploy committer (needs PostgreSQL via CNPG operator for validator/query-service)
5. Install Gateway API CRDs if using ingress features
6. Create Identity for admin, then create ChainNamespace to test the full pipeline

## Key patterns

- PVC `StorageClassName` must be `nil` (not empty string) to use the cluster default
- Assembler and router need writable store volumes (PVC for assembler, emptyDir for router)
- Genesis `MetaNamespaceCA` field is deprecated and optional
- Sidecar config uses `orderer.identity.msp-id` and `orderer.identity.msp-dir` for authentication with assemblers
- E2E tests install CRDs and operator in `BeforeSuite`, not per-test; CA service name equals the CA CR name (no `-service` suffix)
