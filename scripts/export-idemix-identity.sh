#!/bin/bash

# Script to export idemix identity from Kubernetes secret to local folder structure
# Usage: ./export-idemix-identity.sh <identity-name> [output-dir]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check arguments
if [ -z "$1" ]; then
    echo -e "${RED}Error: Identity name is required${NC}"
    echo "Usage: $0 <identity-name> [output-dir]"
    echo "Example: $0 org1-idemix-user ./testdata/idemix"
    exit 1
fi

IDENTITY_NAME="$1"
OUTPUT_DIR="${2:-./testdata/idemix}"

# Get namespace from identity resource or use default
NAMESPACE=$(kubectl get identity "$IDENTITY_NAME" -o jsonpath='{.metadata.namespace}' 2>/dev/null || echo "default")

echo -e "${GREEN}Exporting idemix identity: $IDENTITY_NAME${NC}"
echo -e "${GREEN}Namespace: $NAMESPACE${NC}"
echo -e "${GREEN}Output directory: $OUTPUT_DIR${NC}"
echo ""

# Get the secret name from identity status
SECRET_NAME=$(kubectl get identity "$IDENTITY_NAME" -n "$NAMESPACE" -o jsonpath='{.status.outputSecrets.idemixCred}' 2>/dev/null)

if [ -z "$SECRET_NAME" ]; then
    # Try alternative naming pattern
    SECRET_NAME="${IDENTITY_NAME}-idemix-cred"
    echo -e "${YELLOW}No idemixCred in status, trying: $SECRET_NAME${NC}"
fi

# Check if secret exists
if ! kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" &>/dev/null; then
    echo -e "${RED}Error: Secret $SECRET_NAME not found in namespace $NAMESPACE${NC}"
    exit 1
fi

echo -e "${GREEN}Found secret: $SECRET_NAME${NC}"
echo ""

# Create output directory structure
mkdir -p "$OUTPUT_DIR/msp"
mkdir -p "$OUTPUT_DIR/ca"

# Export SignerConfig fields to msp directory
echo -e "${GREEN}Exporting SignerConfig fields to $OUTPUT_DIR/msp/${NC}"

# Get all secret keys
SECRET_KEYS=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath='{.data}' | jq -r 'keys[]')

# Export each field
for key in $SECRET_KEYS; do
    case "$key" in
        Cred|Sk|credential_revocation_information|curveID|enrollment_id|revocation_handle|role|organizational_unit_identifier)
            # These are SignerConfig fields - save to msp directory
            echo "  - $key"
            kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath="{.data.$key}" | base64 -d > "$OUTPUT_DIR/msp/$key"
            ;;
        user-SignerConfig)
            # Original SignerConfig file
            echo "  - SignerConfig (original)"
            kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath="{.data.$key}" | base64 -d > "$OUTPUT_DIR/msp/SignerConfig"
            ;;
        user-*)
            # CA files (IssuerPublicKey, RevocationPublicKey, etc.)
            FILENAME="${key#user-}"
            echo "  - $FILENAME (to ca/)"
            kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath="{.data.$key}" | base64 -d > "$OUTPUT_DIR/ca/$FILENAME"
            ;;
        *)
            echo "  - $key (unknown, skipping)"
            ;;
    esac
done

echo ""

# Get CA name from identity spec
CA_NAME=$(kubectl get identity "$IDENTITY_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.enrollment.caRef.name}' 2>/dev/null)

if [ -n "$CA_NAME" ]; then
    CA_NAMESPACE=$(kubectl get identity "$IDENTITY_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.enrollment.caRef.namespace}' 2>/dev/null || echo "$NAMESPACE")

    echo -e "${GREEN}Fetching idemix CA public keys from: $CA_NAME${NC}"

    # Get CA service name
    CA_SERVICE="${CA_NAME}.${CA_NAMESPACE}"
    CA_PORT=$(kubectl get service "$CA_NAME" -n "$CA_NAMESPACE" -o jsonpath='{.spec.ports[?(@.name=="ca-port")].port}' 2>/dev/null || echo "7054")

    # Get CA pod for port-forward
    CA_POD=$(kubectl get pods -n "$CA_NAMESPACE" -l "app=ca,release=$CA_NAME" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

    if [ -n "$CA_POD" ]; then
        echo "  Using pod: $CA_POD"

        # Start port-forward in background
        kubectl port-forward -n "$CA_NAMESPACE" "$CA_POD" 7054:7054 >/dev/null 2>&1 &
        PF_PID=$!
        sleep 2

        # Fetch idemix public keys
        echo "  Fetching IssuerPublicKey..."
        curl -sk "https://localhost:7054/cainfo" | jq -r '.result.CAChain' | base64 -d > "$OUTPUT_DIR/ca/IssuerPublicKey" 2>/dev/null || \
            curl -sk "http://localhost:7054/api/v1/cainfo" > /dev/null 2>&1

        echo "  Fetching IssuerRevocationPublicKey..."
        # Try to get from CA filesystem via exec
        kubectl exec -n "$CA_NAMESPACE" "$CA_POD" -- cat /var/hyperledger/fabric-ca-server/IssuerRevocationPublicKey > "$OUTPUT_DIR/ca/IssuerRevocationPublicKey" 2>/dev/null || \
            echo "    (RevocationPublicKey not available)"

        # Kill port-forward
        kill $PF_PID 2>/dev/null || true

        echo -e "${GREEN}  ✓ CA public keys fetched${NC}"
    else
        echo -e "${YELLOW}  Warning: CA pod not found, skipping CA public keys${NC}"
    fi
    echo ""
fi

echo -e "${GREEN}✓ Export complete!${NC}"
echo ""
echo "Directory structure:"
tree "$OUTPUT_DIR" 2>/dev/null || ls -lR "$OUTPUT_DIR"

echo ""
echo -e "${GREEN}Idemix identity exported to: $OUTPUT_DIR${NC}"
echo ""
echo "Usage in Go code:"
echo "  mspPath := \"$OUTPUT_DIR/msp\""
echo "  caPath := \"$OUTPUT_DIR/ca\""
