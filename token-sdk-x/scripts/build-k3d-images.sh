#!/usr/bin/env bash
#
# Build Docker images for all node types and import them into k3d cluster
#
# Usage:
#   ./scripts/build-k3d-images.sh [OPTIONS]
#
# Options:
#   --platform PLATFORM     Platform to build for: fabricx (default) or fabric3
#   --cluster CLUSTER       k3d cluster name (default: k8s-hlf)
#   --registry REGISTRY     Image registry prefix (optional)
#   --tag TAG              Image tag (default: latest)
#   --skip-build           Skip building, only import existing images
#   --skip-import          Skip importing to k3d
#   -h, --help             Show this help message
#

set -e

# Default configuration
PLATFORM="fabricx"
K3D_CLUSTER="k8s-hlf"
IMAGE_PREFIX="token-sdk"
IMAGE_TAG="latest"
REGISTRY=""
SKIP_BUILD=false
SKIP_IMPORT=false

# Node types to build
NODE_TYPES=("issuer" "endorser" "owner")

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ${NC} $*"
}

log_success() {
    echo -e "${GREEN}✓${NC} $*"
}

log_error() {
    echo -e "${RED}✗${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}⚠${NC} $*"
}

print_header() {
    echo ""
    echo "=========================================="
    echo "$1"
    echo "=========================================="
}

show_help() {
    cat << EOF
Build Docker images for Token SDK nodes and import to k3d

Usage:
  ./scripts/build-k3d-images.sh [OPTIONS]

Options:
  --platform PLATFORM     Platform: fabricx (default) or fabric3
  --cluster CLUSTER       k3d cluster name (default: k8s-hlf)
  --registry REGISTRY     Image registry prefix (e.g., myregistry.io/)
  --tag TAG              Image tag (default: latest)
  --skip-build           Skip building, only import existing images
  --skip-import          Skip importing to k3d
  -h, --help             Show this help message

Examples:
  # Build and import with defaults
  ./scripts/build-k3d-images.sh

  # Build for fabric3 platform
  ./scripts/build-k3d-images.sh --platform fabric3

  # Build with custom registry and tag
  ./scripts/build-k3d-images.sh --registry myregistry.io/ --tag v1.0.0

  # Only build, don't import to k3d
  ./scripts/build-k3d-images.sh --skip-import

  # Only import existing images
  ./scripts/build-k3d-images.sh --skip-build

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --platform)
            PLATFORM="$2"
            shift 2
            ;;
        --cluster)
            K3D_CLUSTER="$2"
            shift 2
            ;;
        --registry)
            REGISTRY="$2"
            shift 2
            ;;
        --tag)
            IMAGE_TAG="$2"
            shift 2
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --skip-import)
            SKIP_IMPORT=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Validate platform
if [[ "$PLATFORM" != "fabricx" && "$PLATFORM" != "fabric3" ]]; then
    log_error "Invalid platform: $PLATFORM. Must be 'fabricx' or 'fabric3'"
    exit 1
fi

# Function to get full image name
get_image_name() {
    local node_type=$1
    echo "${REGISTRY}${IMAGE_PREFIX}-${node_type}:${IMAGE_TAG}"
}

# Function to build a single node type
build_node_type() {
    local node_type=$1
    local image_name=$(get_image_name "$node_type")

    log_info "Building ${node_type}: ${image_name}"

    if docker build \
        --tag "${image_name}" \
        --build-arg NODE_TYPE="${node_type}" \
        --build-arg PLATFORM="${PLATFORM}" \
        --file Dockerfile \
        . ; then
        log_success "Built ${node_type} successfully"
        return 0
    else
        log_error "Failed to build ${node_type}"
        return 1
    fi
}

# Function to import image into k3d
import_to_k3d() {
    local node_type=$1
    local image_name=$(get_image_name "$node_type")

    log_info "Importing ${node_type} to k3d cluster: ${K3D_CLUSTER}"

    if k3d image import "${image_name}" --cluster "${K3D_CLUSTER}"; then
        log_success "Imported ${node_type} successfully"
        return 0
    else
        log_error "Failed to import ${node_type}"
        return 1
    fi
}

# Main script
print_header "Token SDK k3d Image Builder"
echo "Platform:      ${PLATFORM}"
echo "k3d Cluster:   ${K3D_CLUSTER}"
echo "Image Prefix:  ${IMAGE_PREFIX}"
echo "Image Tag:     ${IMAGE_TAG}"
[[ -n "$REGISTRY" ]] && echo "Registry:      ${REGISTRY}"
echo "Node Types:    ${NODE_TYPES[*]}"
echo "Skip Build:    ${SKIP_BUILD}"
echo "Skip Import:   ${SKIP_IMPORT}"
echo "=========================================="

# Verify prerequisites
if [[ "$SKIP_BUILD" == false ]]; then
    log_info "Checking Docker..."
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi

    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi
    log_success "Docker is available"
fi

if [[ "$SKIP_IMPORT" == false ]]; then
    log_info "Checking k3d..."
    if ! command -v k3d &> /dev/null; then
        log_error "k3d is not installed or not in PATH"
        exit 1
    fi

    log_info "Checking if k3d cluster '${K3D_CLUSTER}' exists..."
    if ! k3d cluster list | grep -q "${K3D_CLUSTER}"; then
        log_error "k3d cluster '${K3D_CLUSTER}' not found"
        echo ""
        echo "Available clusters:"
        k3d cluster list
        echo ""
        echo "To create a cluster, run:"
        echo "  k3d cluster create ${K3D_CLUSTER}"
        exit 1
    fi
    log_success "k3d cluster '${K3D_CLUSTER}' found"
fi

# Build images
if [[ "$SKIP_BUILD" == false ]]; then
    print_header "Building Docker Images"

    FAILED_BUILDS=()

    for node_type in "${NODE_TYPES[@]}"; do
        if ! build_node_type "${node_type}"; then
            FAILED_BUILDS+=("${node_type}")
        fi
        echo ""
    done

    if [ ${#FAILED_BUILDS[@]} -gt 0 ]; then
        log_error "Build failed for: ${FAILED_BUILDS[*]}"
        exit 1
    fi

    log_success "All images built successfully"
else
    log_warning "Skipping build step"
fi

# Import images to k3d
if [[ "$SKIP_IMPORT" == false ]]; then
    print_header "Importing Images to k3d"

    FAILED_IMPORTS=()

    for node_type in "${NODE_TYPES[@]}"; do
        if ! import_to_k3d "${node_type}"; then
            FAILED_IMPORTS+=("${node_type}")
        fi
        echo ""
    done

    if [ ${#FAILED_IMPORTS[@]} -gt 0 ]; then
        log_error "Import failed for: ${FAILED_IMPORTS[*]}"
        exit 1
    fi

    log_success "All images imported successfully"
else
    log_warning "Skipping import step"
fi

# Summary
print_header "✓ All operations completed successfully!"
echo "Built and imported images:"
for node_type in "${NODE_TYPES[@]}"; do
    echo "  - $(get_image_name "$node_type")"
done
echo ""
log_success "Images are ready to use in k3d cluster '${K3D_CLUSTER}'"
echo ""
echo "Next steps:"
echo "  1. Deploy the images to Kubernetes using Helm or kubectl"
echo "  2. Ensure proper configuration (core.yaml, certificates) is mounted"
echo "  3. Verify pods are running: kubectl get pods"
echo ""
