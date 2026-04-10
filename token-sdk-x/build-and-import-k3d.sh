#!/usr/bin/env bash
#
# Build Docker images for all node types and import them into k3d cluster
#
# Usage:
#   ./build-and-import-k3d.sh [PLATFORM] [K3D_CLUSTER]
#
# Arguments:
#   PLATFORM      - Platform to build for: fabricx (default) or fabric3
#   K3D_CLUSTER   - k3d cluster name (default: k8s-hlf)
#

set -e

# Configuration
PLATFORM="${1:-fabricx}"
K3D_CLUSTER="${2:-k8s-hlf}"
IMAGE_PREFIX="token-sdk"
IMAGE_TAG="${PLATFORM}-latest"

# Node types to build
NODE_TYPES=("issuer" "endorser" "owner")

echo "=========================================="
echo "Building Token SDK Docker Images"
echo "=========================================="
echo "Platform: ${PLATFORM}"
echo "k3d Cluster: ${K3D_CLUSTER}"
echo "Node Types: ${NODE_TYPES[*]}"
echo "=========================================="
echo ""

# Function to build a single node type
build_node_type() {
    local node_type=$1
    local image_name="${IMAGE_PREFIX}-${node_type}:${IMAGE_TAG}"

    echo "-------------------------------------------"
    echo "[${node_type}] Building image: ${image_name}"
    echo "-------------------------------------------"

    docker build \
        --tag "${image_name}" \
        --build-arg NODE_TYPE="${node_type}" \
        --build-arg PLATFORM="${PLATFORM}" \
        --file Dockerfile \
        .

    if [ $? -eq 0 ]; then
        echo "✓ [${node_type}] Build successful"
        return 0
    else
        echo "✗ [${node_type}] Build failed"
        return 1
    fi
}

# Function to import image into k3d
import_to_k3d() {
    local node_type=$1
    local image_name="${IMAGE_PREFIX}-${node_type}:${IMAGE_TAG}"

    echo "-------------------------------------------"
    echo "[${node_type}] Importing to k3d cluster: ${K3D_CLUSTER}"
    echo "-------------------------------------------"

    k3d image import "${image_name}" --cluster "${K3D_CLUSTER}"

    if [ $? -eq 0 ]; then
        echo "✓ [${node_type}] Import successful"
        return 0
    else
        echo "✗ [${node_type}] Import failed"
        return 1
    fi
}

# Check if k3d cluster exists
echo "Checking if k3d cluster '${K3D_CLUSTER}' exists..."
if ! k3d cluster list | grep -q "${K3D_CLUSTER}"; then
    echo "✗ k3d cluster '${K3D_CLUSTER}' not found"
    echo ""
    echo "Available clusters:"
    k3d cluster list
    echo ""
    echo "To create a cluster, run:"
    echo "  k3d cluster create ${K3D_CLUSTER}"
    exit 1
fi
echo "✓ k3d cluster '${K3D_CLUSTER}' found"
echo ""

# Build all node types
echo "=========================================="
echo "Step 1: Building Docker Images"
echo "=========================================="
echo ""

FAILED_BUILDS=()

for node_type in "${NODE_TYPES[@]}"; do
    if ! build_node_type "${node_type}"; then
        FAILED_BUILDS+=("${node_type}")
    fi
    echo ""
done

# Check if any builds failed
if [ ${#FAILED_BUILDS[@]} -gt 0 ]; then
    echo "✗ Build failed for: ${FAILED_BUILDS[*]}"
    exit 1
fi

echo "✓ All images built successfully"
echo ""

# Import all images to k3d
echo "=========================================="
echo "Step 2: Importing Images to k3d"
echo "=========================================="
echo ""

FAILED_IMPORTS=()

for node_type in "${NODE_TYPES[@]}"; do
    if ! import_to_k3d "${node_type}"; then
        FAILED_IMPORTS+=("${node_type}")
    fi
    echo ""
done

# Check if any imports failed
if [ ${#FAILED_IMPORTS[@]} -gt 0 ]; then
    echo "✗ Import failed for: ${FAILED_IMPORTS[*]}"
    exit 1
fi

echo "=========================================="
echo "✓ All operations completed successfully!"
echo "=========================================="
echo ""
echo "Built and imported images:"
for node_type in "${NODE_TYPES[@]}"; do
    echo "  - ${IMAGE_PREFIX}-${node_type}:${IMAGE_TAG}"
done
echo ""
echo "You can now deploy these images to your k3d cluster."
