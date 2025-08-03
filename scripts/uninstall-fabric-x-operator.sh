#!/bin/bash

# Fabric-X Operator Uninstallation Script
# This script removes the fabric-x-operator and cleans up resources

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
OPERATOR_NAMESPACE="fabric-x-system"
K3D_CLUSTER_NAME="k3d-k8s-hlf"

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
        print_warning "K3D cluster '$K3D_CLUSTER_NAME' not found"
        print_status "Skipping k3d-specific cleanup..."
        return 1
    fi
    
    print_success "K3D cluster '$K3D_CLUSTER_NAME' found"
    return 0
}

# Function to delete custom resources
delete_custom_resources() {
    print_status "Deleting custom resources..."
    
    # Delete all CAs
    if kubectl get ca --all-namespaces --no-headers 2>/dev/null; then
        print_status "Deleting all CA resources..."
        kubectl delete ca --all --all-namespaces --ignore-not-found=true
        print_success "CA resources deleted"
    fi
    
    # Delete all Endorsers
    if kubectl get endorser --all-namespaces --no-headers 2>/dev/null; then
        print_status "Deleting all Endorser resources..."
        kubectl delete endorser --all --all-namespaces --ignore-not-found=true
        print_success "Endorser resources deleted"
    fi
    
    # Delete all Committers
    if kubectl get committer --all-namespaces --no-headers 2>/dev/null; then
        print_status "Deleting all Committer resources..."
        kubectl delete committer --all --all-namespaces --ignore-not-found=true
        print_success "Committer resources deleted"
    fi
    
    # Delete all OrdererGroups
    if kubectl get orderergroup --all-namespaces --no-headers 2>/dev/null; then
        print_status "Deleting all OrdererGroup resources..."
        kubectl delete orderergroup --all --all-namespaces --ignore-not-found=true
        print_success "OrdererGroup resources deleted"
    fi
    
    print_success "All custom resources deleted"
}

# Function to undeploy the operator
undeploy_operator() {
    print_status "Undeploying fabric-x-operator..."
    
    if make undeploy; then
        print_success "Operator undeployed successfully"
    else
        print_warning "Failed to undeploy operator (might already be removed)"
    fi
}

# Function to uninstall CRDs
uninstall_crds() {
    print_status "Uninstalling Custom Resource Definitions..."
    
    if make uninstall; then
        print_success "CRDs uninstalled successfully"
    else
        print_warning "Failed to uninstall CRDs (might already be removed)"
    fi
}

# Function to delete namespace
delete_namespace() {
    print_status "Deleting namespace: $OPERATOR_NAMESPACE"
    
    if kubectl delete namespace "$OPERATOR_NAMESPACE" --ignore-not-found=true; then
        print_success "Namespace deleted successfully"
    else
        print_warning "Namespace might already be deleted"
    fi
}

# Function to clean up k3d images
cleanup_k3d_images() {
    if check_k3d_cluster; then
        print_status "Cleaning up k3d images..."
        
        # List and remove fabric-x-operator images
        local images=$(k3d image list -c "$K3D_CLUSTER_NAME" | grep "fabric-x-operator" | awk '{print $1}' || true)
        
        if [ -n "$images" ]; then
            echo "$images" | while read -r image; do
                print_status "Removing image: $image"
                k3d image rm "$image" -c "$K3D_CLUSTER_NAME" 2>/dev/null || true
            done
            print_success "K3D images cleaned up"
        else
            print_status "No fabric-x-operator images found in k3d cluster"
        fi
    fi
}

# Function to verify uninstallation
verify_uninstallation() {
    print_status "Verifying uninstallation..."
    
    # Check if CRDs are removed
    if kubectl get crd | grep -q "fabricx.kfsoft.tech"; then
        print_warning "Some CRDs are still present"
        return 1
    else
        print_success "CRDs are removed"
    fi
    
    # Check if operator namespace is removed
    if kubectl get namespace "$OPERATOR_NAMESPACE" 2>/dev/null; then
        print_warning "Operator namespace is still present"
        return 1
    else
        print_success "Operator namespace is removed"
    fi
    
    # Check if operator pods are removed
    if kubectl get pods -n "$OPERATOR_NAMESPACE" -l control-plane=controller-manager --no-headers 2>/dev/null | grep -q "Running"; then
        print_warning "Operator pods are still running"
        return 1
    else
        print_success "Operator pods are removed"
    fi
    
    return 0
}

# Function to show completion message
show_completion() {
    print_success "Fabric-X Operator uninstallation completed!"
    echo
    echo "The following resources have been removed:"
    echo "  ✓ Custom resources (CA, Endorser, Committer, OrdererGroup)"
    echo "  ✓ Operator deployment"
    echo "  ✓ Custom Resource Definitions (CRDs)"
    echo "  ✓ Operator namespace"
    echo "  ✓ K3D images (if applicable)"
    echo
    echo "To reinstall the operator, run:"
    echo "  ./scripts/install-fabric-x-operator.sh"
}

# Function to cleanup on error
cleanup() {
    print_error "Uninstallation failed. Some resources might still be present."
    print_status "You may need to manually clean up remaining resources."
}

# Set up error handling
trap cleanup ERR

# Main uninstallation function
main() {
    echo "=========================================="
    echo "Fabric-X Operator Uninstallation Script"
    echo "=========================================="
    echo
    
    # Check prerequisites
    check_prerequisites
    
    # Delete custom resources first
    delete_custom_resources
    
    # Undeploy operator
    undeploy_operator
    
    # Uninstall CRDs
    uninstall_crds
    
    # Delete namespace
    delete_namespace
    
    # Clean up k3d images
    cleanup_k3d_images
    
    # Verify uninstallation
    if verify_uninstallation; then
        show_completion
    else
        print_warning "Some resources might still be present"
        print_status "You may need to manually clean up remaining resources"
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
            echo "  --cluster NAME Custom k3d cluster name (default: k3d-k8s-hlf)"
            echo "  --force        Force deletion without confirmation"
            echo
            echo "This script will:"
            echo "  1. Delete all custom resources (CA, Endorser, Committer, OrdererGroup)"
            echo "  2. Undeploy the operator"
            echo "  3. Uninstall CRDs"
            echo "  4. Delete the operator namespace"
            echo "  5. Clean up k3d images"
            echo "  6. Verify the uninstallation"
            exit 0
            ;;
        --cluster)
            K3D_CLUSTER_NAME="$2"
            shift 2
            ;;
        --force)
            FORCE=true
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Confirm uninstallation unless --force is used
if [ "$FORCE" != "true" ]; then
    echo "This will completely remove the Fabric-X Operator and all its resources."
    echo "Are you sure you want to continue? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        print_status "Uninstallation cancelled"
        exit 0
    fi
fi

# Run main uninstallation
main 