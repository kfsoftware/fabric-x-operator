#!/usr/bin/env bash
# Run token-sdk-x locally against an existing Fabric-X network in k8s.
#
# Assumes the cluster already has:
#   - operator running
#   - CA, OrdererGroup, Committer RUNNING
#   - an admin Identity with its MSP Secret (default: tokensdk-admin-cert)
#   - a ChainNamespace already Deployed for the token namespace
#
# This script only wires the local Go services to the running cluster:
#   1. extract MSP from the existing k8s Secret into conf/<node>/keys/fabric/...
#   2. patch mspID in conf/*/core.yaml
#   3. start kubectl port-forwards (orderer / sidecar / query)
#   4. run issuer/endorser1/owner1/owner2 with 'go run'

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

KUBE_CONTEXT="${KUBE_CONTEXT:-k3d-helm-test}"
KUBE_NS="${KUBE_NS:-default}"
MSPID="${MSPID:-Org1MSP}"
IDENTITY_SECRET="${IDENTITY_SECRET:-tokensdk-admin-cert}"

ORDERER_SVC="${ORDERER_SVC:-orderergroup-party1-router-service}"
ORDERER_PORT="${ORDERER_PORT:-7150}"
SIDECAR_SVC="${SIDECAR_SVC:-e2e-committer-sidecar-service}"
SIDECAR_PORT="${SIDECAR_PORT:-5050}"
QUERY_SVC="${QUERY_SVC:-e2e-committer-query-service-service}"
QUERY_PORT="${QUERY_PORT:-9001}"

LOCAL_ORDERER_PORT="${LOCAL_ORDERER_PORT:-7050}"
LOCAL_SIDECAR_PORT="${LOCAL_SIDECAR_PORT:-5400}"
LOCAL_QUERY_PORT="${LOCAL_QUERY_PORT:-5500}"

PF_DIR="$REPO_ROOT/.k8s-pf"
APPS_DIR="$REPO_ROOT/.k8s-apps"
KUBECTL=(kubectl --context "$KUBE_CONTEXT" -n "$KUBE_NS")

log() { printf '\033[36m[k8s]\033[0m %s\n' "$*"; }
die() { printf '\033[31m[k8s] %s\033[0m\n' "$*" >&2; exit 1; }

# --- extract MSP from the existing Identity secret ----------------------------
extract_msp() {
  log "extracting MSP from secret/$IDENTITY_SECRET"
  local tmp; tmp=$(mktemp -d); trap 'rm -rf "$tmp"' RETURN

  "${KUBECTL[@]}" get secret "$IDENTITY_SECRET" >/dev/null \
    || die "secret/$IDENTITY_SECRET not found in $KUBE_NS — override with IDENTITY_SECRET=..."

  "${KUBECTL[@]}" get secret "$IDENTITY_SECRET" -o json \
    | jq -r '.data | to_entries[] | "\(.key)\t\(.value)"' \
    | while IFS=$'\t' read -r k v; do
        printf '%s' "$v" | base64 -d > "$tmp/$k"
      done

  for f in cert.pem key.pem cacert.pem; do
    [[ -s "$tmp/$f" ]] || die "secret missing $f"
  done

  local msp="$tmp/msp"
  mkdir -p "$msp/signcerts" "$msp/keystore" "$msp/cacerts" "$msp/admincerts"
  cp "$tmp/cert.pem"   "$msp/signcerts/cert.pem"
  cp "$tmp/key.pem"    "$msp/keystore/priv_sk"
  cp "$tmp/cacert.pem" "$msp/cacerts/ca.pem"
  cp "$tmp/cert.pem"   "$msp/admincerts/cert.pem"
  [[ -s "$tmp/tls-cacert.pem" ]] && { mkdir -p "$msp/tlscacerts"; cp "$tmp/tls-cacert.pem" "$msp/tlscacerts/tls-ca.pem"; }

  cat > "$msp/config.yaml" <<'YAML'
NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: orderer
YAML

  for node in issuer owner1 owner2 endorser1; do
    local dst="$REPO_ROOT/conf/$node/keys/fabric"
    mkdir -p "$REPO_ROOT/conf/$node/data" "$dst"
    rm -rf "$dst/user"; cp -r "$msp" "$dst/user"
  done
  rm -rf "$REPO_ROOT/conf/endorser1/keys/fabric/endorser" "$REPO_ROOT/conf/endorser1/keys/fabric/admin"
  cp -r "$msp" "$REPO_ROOT/conf/endorser1/keys/fabric/endorser"
  cp -r "$msp" "$REPO_ROOT/conf/endorser1/keys/fabric/admin"
  log "MSP laid out under conf/{issuer,owner1,owner2,endorser1}/keys/fabric"
}

patch_cores() {
  log "patching mspID -> $MSPID, endpoints -> localhost in conf/*/core.yaml"
  for f in conf/issuer/core.yaml conf/owner1/core.yaml conf/owner2/core.yaml conf/endorser1/core.yaml; do
    [[ -f "$f" ]] && sed -i.bak -E \
      -e "s/(mspID:[[:space:]]*).*/\1$MSPID/" \
      -e "s|committer-sidecar:[0-9]+|localhost:5400|g" \
      -e "s|committer-queryservice:[0-9]+|localhost:5500|g" \
      -e "s|[a-z0-9]+\.example\.com:|localhost:|g" \
      "$f" && rm -f "$f.bak"
  done

  # Patch P2P routing to use localhost addresses
  for node in issuer owner1 owner2 endorser1; do
    cat > "conf/$node/routing-config.yaml" <<'YAML'
routes:
  issuer:
    - localhost:9101
  endorser1:
    - localhost:9301
  owner1:
    - localhost:9501
  owner2:
    - localhost:9601
YAML
  done
}

# --- port-forwards ------------------------------------------------------------
forward_stop_quiet() {
  [[ -d "$PF_DIR" ]] || return 0
  for f in "$PF_DIR"/*.pid; do
    [[ -f "$f" ]] || continue
    kill "$(cat "$f" 2>/dev/null)" 2>/dev/null || true; rm -f "$f"
  done
}

forward_start() {
  mkdir -p "$PF_DIR"; forward_stop_quiet
  log "port-forward orderer=$LOCAL_ORDERER_PORT sidecar=$LOCAL_SIDECAR_PORT query=$LOCAL_QUERY_PORT"
  "${KUBECTL[@]}" port-forward "svc/$ORDERER_SVC" "${LOCAL_ORDERER_PORT}:${ORDERER_PORT}" >"$PF_DIR/orderer.log" 2>&1 & echo $! >"$PF_DIR/orderer.pid"
  "${KUBECTL[@]}" port-forward "svc/$SIDECAR_SVC" "${LOCAL_SIDECAR_PORT}:${SIDECAR_PORT}" >"$PF_DIR/sidecar.log" 2>&1 & echo $! >"$PF_DIR/sidecar.pid"
  "${KUBECTL[@]}" port-forward "svc/$QUERY_SVC"   "${LOCAL_QUERY_PORT}:${QUERY_PORT}"     >"$PF_DIR/query.log"   2>&1 & echo $! >"$PF_DIR/query.pid"
  sleep 2
  for n in orderer sidecar query; do
    kill -0 "$(cat "$PF_DIR/$n.pid")" 2>/dev/null || { cat "$PF_DIR/$n.log" >&2; die "port-forward $n died"; }
  done
}

# --- local apps ---------------------------------------------------------------
apps_stop_quiet() {
  [[ -d "$APPS_DIR" ]] || return 0
  for f in "$APPS_DIR"/*.pid; do
    [[ -f "$f" ]] || continue
    local pid; pid=$(cat "$f" 2>/dev/null || true)
    [[ -n "$pid" ]] && { pkill -P "$pid" 2>/dev/null || true; kill "$pid" 2>/dev/null || true; }
    rm -f "$f"
  done
}

apps_start() {
  mkdir -p "$APPS_DIR"; apps_stop_quiet
  export ARMA_ROUTER_ADDRESS="${ARMA_ROUTER_ADDRESS:-localhost:$LOCAL_ORDERER_PORT}"
  log "starting local services (logs in $APPS_DIR/) ARMA_ROUTER_ADDRESS=$ARMA_ROUTER_ADDRESS"
  ( cd conf/issuer    && nohup go run -tags fabricx ../../issuer   --port 9100 >"$APPS_DIR/issuer.log"    2>&1 & echo $! >"$APPS_DIR/issuer.pid" )
  ( cd conf/endorser1 && nohup go run -tags fabricx ../../endorser --port 9300 >"$APPS_DIR/endorser1.log" 2>&1 & echo $! >"$APPS_DIR/endorser1.pid" )
  ( cd conf/owner1    && nohup go run -tags fabricx ../../owner    --port 9500 >"$APPS_DIR/owner1.log"    2>&1 & echo $! >"$APPS_DIR/owner1.pid" )
  ( cd conf/owner2    && nohup go run -tags fabricx ../../owner    --port 9600 >"$APPS_DIR/owner2.log"    2>&1 & echo $! >"$APPS_DIR/owner2.pid" )
  log "issuer:9100  endorser1:9300  owner1:9500  owner2:9600"
  log "logs: tail -F $APPS_DIR/*.log"
}

# --- commands -----------------------------------------------------------------
case "${1:-}" in
  up)
    command -v jq >/dev/null || die "jq required"
    extract_msp
    patch_cores
    forward_start
    apps_start
    ;;
  down)
    apps_stop_quiet
    forward_stop_quiet
    git -C "$REPO_ROOT" checkout -- conf/issuer/core.yaml conf/owner1/core.yaml conf/owner2/core.yaml conf/endorser1/core.yaml 2>/dev/null || true
    for n in issuer owner1 owner2 endorser1; do rm -rf "conf/$n/keys/fabric" "conf/$n/data"; done
    rm -rf "$PF_DIR" "$APPS_DIR"
    log "stopped and cleaned"
    ;;
  forward) forward_start ;;
  apps)    apps_start ;;
  stop)    apps_stop_quiet; forward_stop_quiet; log "stopped" ;;
  *) cat <<USAGE
Usage: $0 <up|down|forward|apps|stop>

  up       extract MSP, patch core.yaml, start port-forwards, start apps
  down     stop everything + clean up conf/
  forward  (re)start port-forwards only
  apps     (re)start local Go services only
  stop     stop apps + port-forwards (keep conf/ material)

Env: KUBE_CONTEXT [k3d-helm-test]  KUBE_NS [default]  MSPID [Org1MSP]
     IDENTITY_SECRET [tokensdk-admin-cert]
     ORDERER_SVC/PORT  SIDECAR_SVC/PORT  QUERY_SVC/PORT
USAGE
  ;;
esac
