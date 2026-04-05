#!/bin/bash

# Fabric-X Operator Installation Script
# This script builds the fabric-x-operator image, loads it into k3d, and deploys it

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
OPERATOR_NAME="fabric-x-operator"
OPERATOR_NAMESPACE="fabric-x-system"
IMAGE_NAME="kfsoft.tech/fabric-x-operator"
IMAGE_TAG="latest"
K3D_CLUSTER_NAME="k8s-hlf"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    local missing_tools=()
    
    if ! command_exists docker; then
        missing_tools+=("docker")
    fi
    
    if ! command_exists k3d; then
        missing_tools+=("k3d")
    fi
    
    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi
    
    if ! command_exists make; then
        missing_tools+=("make")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        print_error "Missing required tools: ${missing_tools[*]}"
        print_status "Please install the missing tools and try again."
        exit 1
    fi
    
    print_success "All prerequisites are installed"
}

# Function to check if k3d cluster exists
check_k3d_cluster() {
    print_status "Checking k3d cluster..."
    
    if ! k3d cluster list | grep -q "$K3D_CLUSTER_NAME"; then
        print_error "K3D cluster '$K3D_CLUSTER_NAME' not found"
        print_status "Please create the cluster first with: k3d cluster create $K3D_CLUSTER_NAME"
        exit 1
    fi
    
    print_success "K3D cluster '$K3D_CLUSTER_NAME' found"
}

# Function to build the operator image
build_operator_image() {
    print_status "Building fabric-x-operator image..."
    
    # Build the image
    if make docker-build IMG="$IMAGE_NAME:$IMAGE_TAG"; then
        print_success "Operator image built successfully: $IMAGE_NAME:$IMAGE_TAG"
    else
        print_error "Failed to build operator image"
        exit 1
    fi
}

# Function to load image into k3d
load_image_to_k3d() {
    print_status "Loading image into k3d cluster..."
    
    if k3d image import "$IMAGE_NAME:$IMAGE_TAG" -c "$K3D_CLUSTER_NAME"; then
        print_success "Image loaded into k3d cluster successfully"
    else
        print_error "Failed to load image into k3d cluster"
        exit 1
    fi
}

# Function to install CRDs
install_crds() {
    print_status "Installing Custom Resource Definitions..."
    
    if make install; then
        print_success "CRDs installed successfully"
    else
        print_error "Failed to install CRDs"
        exit 1
    fi
}

# Function to create namespace
create_namespace() {
    print_status "Creating namespace: $OPERATOR_NAMESPACE"
    
    if kubectl create namespace "$OPERATOR_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -; then
        print_success "Namespace created successfully"
    else
        print_warning "Namespace might already exist, continuing..."
    fi
}

# Function to deploy the operator
deploy_operator() {
    print_status "Deploying fabric-x-operator..."
    
    # Update the image in the kustomization
    cd config/manager && kustomize edit set image controller="$IMAGE_NAME:$IMAGE_TAG"
    cd ../..
    
    # Deploy the operator
    if make deploy; then
        print_success "Operator deployed successfully"
    else
        print_error "Failed to deploy operator"
        exit 1
    fi
}

# Function to wait for operator to be ready
wait_for_operator() {
    print_status "Waiting for operator to be ready..."
    
    local timeout=300  # 5 minutes
    local interval=10  # 10 seconds
    local elapsed=0
    
    while [ $elapsed -lt $timeout ]; do
        if kubectl get deployment -n "$OPERATOR_NAMESPACE" controller-manager -o jsonpath='{.status.readyReplicas}' 2>/dev/null | grep -q "1"; then
            print_success "Operator is ready!"
            return 0
        fi
        
        print_status "Waiting for operator to be ready... ($elapsed/$timeout seconds)"
        sleep $interval
        elapsed=$((elapsed + interval))
    done
    
    print_error "Operator failed to become ready within $timeout seconds"
    return 1
}

# Function to verify installation
verify_installation() {
    print_status "Verifying installation..."
    
    # Check if CRDs are installed
    if kubectl get crd | grep -q "fabricx.kfsoft.tech"; then
        print_success "CRDs are installed"
    else
        print_error "CRDs are not installed"
        return 1
    fi
    
    # Check if operator pods are running
    if kubectl get pods -n "$OPERATOR_NAMESPACE" -l control-plane=controller-manager --no-headers | grep -q "Running"; then
        print_success "Operator pods are running"
    else
        print_error "Operator pods are not running"
        return 1
    fi
    
    # Check if operator service is available
    if kubectl get service -n "$OPERATOR_NAMESPACE" controller-manager-metrics-service; then
        print_success "Operator service is available"
    else
        print_warning "Operator service not found"
    fi
    
    return 0
}

# Function to show usage information
show_usage() {
    print_status "Fabric-X Operator Installation Complete!"
    echo
    echo "You can now use the operator with:"
    echo "  kubectl get ca -A                    # List all CAs"
    echo "  kubectl get endorser -A              # List all endorsers"
    echo "  kubectl get committer -A             # List all committers"
    echo "  kubectl get orderergroup -A          # List all orderer groups"
    echo
    echo "Or use the kubectl-fabricx plugin:"
    echo "  kubectl fabricx ca create --name my-ca --namespace my-ns"
    echo
    echo "To uninstall the operator:"
    echo "  make undeploy"
    echo "  make uninstall"
}

# Function to cleanup on error
cleanup() {
    print_error "Installation failed. Cleaning up..."
    make undeploy 2>/dev/null || true
    make uninstall 2>/dev/null || true
}

# Set up error handling
trap cleanup ERR

# Main installation function
main() {
    echo "=========================================="
    echo "Fabric-X Operator Installation Script"
    echo "=========================================="
    echo
    
    # Check prerequisites
    check_prerequisites
    
    # Check k3d cluster
    check_k3d_cluster
    
    # Build operator image
    build_operator_image
    
    # Load image into k3d
    load_image_to_k3d
    
    # Install CRDs
    install_crds
    
    # Create namespace
    create_namespace
    
    # Deploy operator
    deploy_operator
    
    # Wait for operator to be ready
    if wait_for_operator; then
        # Verify installation
        if verify_installation; then
            print_success "Fabric-X Operator installation completed successfully!"
            show_usage
        else
            print_error "Installation verification failed"
            exit 1
        fi
    else
        print_error "Operator failed to become ready"
        exit 1
    fi
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo
            echo "Options:"
            echo "  --help, -h     Show this help message"
            echo "  --image IMG    Custom image name (default: kfsoft.tech/fabric-x-operator)"
            echo "  --tag TAG      Custom image tag (default: latest)"
            echo "  --cluster NAME Custom k3d cluster name (default: k3d-k8s-hlf)"
            echo
            echo "This script will:"
            echo "  1. Check prerequisites (docker, k3d, kubectl, make)"
            echo "  2. Build the fabric-x-operator image"
            echo "  3. Load the image into k3d cluster"
            echo "  4. Install CRDs"
            echo "  5. Deploy the operator"
            echo "  6. Wait for operator to be ready"
            echo "  7. Verify the installation"
            exit 0
            ;;
        --image)
            IMAGE_NAME="$2"
            shift 2
            ;;
        --tag)
            IMAGE_TAG="$2"
            shift 2
            ;;
        --cluster)
            K3D_CLUSTER_NAME="$2"
            shift 2
            ;;
        *)
            print_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Run main installation
main 