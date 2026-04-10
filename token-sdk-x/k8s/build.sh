#!/usr/bin/env bash
# Build token-sdk-x node images for in-cluster deployment against fabric-x-operator.
# Patches conf/<node>/{core.yaml,routing-config.yaml} to use in-cluster service DNS,
# builds 4 images (issuer/endorser1/owner1/owner2), imports into k3d, then restores.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

K3D_CLUSTER="${K3D_CLUSTER:-helm-test}"
TAG="${TAG:-dev}"
SIDECAR_DNS="${SIDECAR_DNS:-e2e-committer-sidecar-service:5050}"
QUERY_DNS="${QUERY_DNS:-e2e-committer-query-service-service:9001}"

NODES=(issuer endorser1 owner1 owner2)
bin_for() {
  case "$1" in
    issuer) echo issuer ;;
    endorser1) echo endorser ;;
    owner1|owner2) echo owner ;;
  esac
}

log(){ printf "\033[36m[k8s-build]\033[0m %s\n" "$*"; }

write_routing() {
  local node="$1"
  cat > "conf/$node/routing-config.yaml" <<YAML
routes:
  issuer:
    - tokensdk-issuer:9101
  auditor:
    - tokensdk-auditor:9201
  endorser1:
    - tokensdk-endorser1:9301
  endorser2:
    - tokensdk-endorser2:9401
  owner1:
    - tokensdk-owner1:9501
  owner2:
    - tokensdk-owner2:9601
YAML
}

patch_core() {
  local node="$1" f="conf/$1/core.yaml"
  cp "$f" "$f.k8sbak"
  sed -i.tmp -E \
    -e "s|^([[:space:]]*-[[:space:]]*address:[[:space:]]*)(localhost|committer-sidecar)(:[0-9]+)|\1$SIDECAR_DNS|g" \
    -e "s|^([[:space:]]*-[[:space:]]*address:[[:space:]]*)(localhost|committer-queryservice)(:[0-9]+)|\1$QUERY_DNS|g" \
    "$f"
  rm -f "$f.tmp"
}

restore_core() {
  local node="$1" f="conf/$1/core.yaml"
  [[ -f "$f.k8sbak" ]] && mv "$f.k8sbak" "$f"
}

# Distinguish sidecar vs queryservice replacement: both use the "- address:" pattern.
# Use a python helper for accuracy.
python_patch() {
  local node="$1" f="conf/$1/core.yaml"
  cp "$f" "$f.k8sbak"
  python3 - "$f" "$SIDECAR_DNS" "$QUERY_DNS" <<'PY'
import re, sys
path, sidecar, query = sys.argv[1], sys.argv[2], sys.argv[3]
src = open(path).read()
def repl_block(text, key, new_addr):
    pat = re.compile(rf"({key}:\s*\n\s*- address:\s*)([^\s\n]+)")
    return pat.sub(lambda m: m.group(1)+new_addr, text)
src = repl_block(src, "peers", sidecar)
src = repl_block(src, "queryService", query)
open(path, "w").write(src)
PY
}

cleanup() {
  for n in "${NODES[@]}"; do restore_core "$n" || true; done
}
trap cleanup EXIT

log "patching conf/{issuer,endorser1,owner1,owner2} for in-cluster DNS"
for n in "${NODES[@]}"; do
  python_patch "$n"
  write_routing "$n"
done

for n in "${NODES[@]}"; do
  bin="$(bin_for "$n")"
  img="tokensdk-$n:$TAG"
  log "building $img (NODE_BIN=$bin NODE_DIR=$n)"
  docker build -f scripts/k8s/Dockerfile.k8s \
    --build-arg NODE_BIN="$bin" \
    --build-arg NODE_DIR="$n" \
    -t "$img" .
  log "importing $img into k3d cluster $K3D_CLUSTER"
  k3d image import "$img" --cluster "$K3D_CLUSTER" --mode direct
done

log "done. images:"
for n in "${NODES[@]}"; do echo "  tokensdk-$n:$TAG"; done
