#!/bin/bash
set -e

echo "=== Preparing Token Namespace Generation ==="
echo "This script extracts issuer certificates and idemix CA public key,"
echo "then generates the token namespace using tokengen."
echo ""

# Create working directories
WORK_DIR=$(mktemp -d)
ISSUERS_DIR="$WORK_DIR/issuers"
IDEMIX_CA_DIR="$WORK_DIR/idemix/ca"
OUTPUT_DIR="$WORK_DIR/namespace"

mkdir -p "$ISSUERS_DIR/signcerts"
mkdir -p "$ISSUERS_DIR/cacerts"
mkdir -p "$IDEMIX_CA_DIR/msp"
mkdir -p "$OUTPUT_DIR"

echo "Working directory: $WORK_DIR"

# Extract issuer certificate
echo ""
echo "=== Extracting Issuer Certificate ==="
kubectl get secret org1-issuer-sign-cert -o jsonpath='{.data.cert\.pem}' | base64 -d > "$ISSUERS_DIR/signcerts/issuer-cert.pem"
kubectl get secret org1-issuer-sign-cert -o jsonpath='{.data.ca\.pem}' | base64 -d > "$ISSUERS_DIR/cacerts/ca.pem"
echo "  ✓ Extracted issuer certificate"

# Extract idemix CA IssuerPublicKey
echo ""
echo "=== Extracting Idemix CA Public Key ==="
kubectl get secret endorser-idemix-ca-idemix-issuer-keys -o jsonpath='{.data.IssuerPublicKey}' | base64 -d > "$IDEMIX_CA_DIR/msp/IssuerPublicKey"
echo "  ✓ Extracted IssuerPublicKey ($(wc -c < "$IDEMIX_CA_DIR/msp/IssuerPublicKey") bytes)"

# Display directory structure
echo ""
echo "=== Directory Structure ==="
echo "Issuers:"
ls -lR "$ISSUERS_DIR"
echo ""
echo "Idemix CA:"
ls -lR "$IDEMIX_CA_DIR"

# Generate namespace using tokengen
echo ""
echo "=== Generating Token Namespace ==="
CMD="tokengen gen zkatdlognogh.v1 --base 300 --exponent 5 --issuers \"$ISSUERS_DIR\" --idemix \"$IDEMIX_CA_DIR\" --output \"$OUTPUT_DIR\""
echo "Running: $CMD"
echo ""

if command -v tokengen &> /dev/null; then
    tokengen gen zkatdlognogh.v1 --base 300 --exponent 5 --issuers "$ISSUERS_DIR" --idemix "$IDEMIX_CA_DIR" --output "$OUTPUT_DIR"
    echo "  ✓ Namespace generated successfully!"

    echo ""
    echo "=== Generated Files ==="
    ls -lh "$OUTPUT_DIR"

    # Create or update Kubernetes secret
    echo ""
    echo "=== Creating/Updating Kubernetes Secret 'token-namespace' ==="
    kubectl create secret generic token-namespace \
        --from-file="$OUTPUT_DIR" \
        --dry-run=client -o yaml | kubectl apply -f -

    echo "  ✓ Secret 'token-namespace' created/updated"

    echo ""
    echo "=== Verifying Secret ==="
    echo "Files in secret:"
    kubectl get secret token-namespace -o jsonpath='{.data}' | jq -r 'keys[]'

    echo ""
    echo "=== Cleanup ==="
    rm -rf "$WORK_DIR"
    echo "  ✓ Temporary files cleaned up"

    echo ""
    echo "=== SUCCESS ==="
    echo "Token namespace secret is ready! You can now:"
    echo "  1. Mount this secret in endorsers at /var/hyperledger/fabric/namespace"
    echo "  2. Patch endorsers to deploy mode: kubectl patch endorser <name> --type=merge -p '{\"spec\":{\"bootstrapMode\":\"deploy\"}}'"

else
    echo "ERROR: tokengen not found in PATH"
    echo ""
    echo "Please install tokengen first, then run this command manually:"
    echo "  $CMD"
    echo ""
    echo "After tokengen succeeds, create the secret:"
    echo "  kubectl create secret generic token-namespace --from-file=\"$OUTPUT_DIR\""
    echo ""
    echo "Keeping temporary directory for manual execution: $WORK_DIR"
    exit 1
fi
