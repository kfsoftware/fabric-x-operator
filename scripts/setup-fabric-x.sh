#!/bin/bash

# Fabric-X Complete Setup Script
# This script installs the fabric-x-operator and kubectl-fabricx plugin

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
KUBECTL_FABRICX_DIR="$PROJECT_ROOT/kubectl-fabricx"

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
    
    if ! command_exists go; then
        missing_tools+=("go")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        print_error "Missing required tools: ${missing_tools[*]}"
        print_status "Please install the missing tools and try again."
        exit 1
    fi
    
    print_success "All prerequisites are installed"
}

# Function to build kubectl-fabricx plugin
build_kubectl_plugin() {
    print_status "Building kubectl-fabricx plugin..."
    
    cd "$KUBECTL_FABRICX_DIR"
    
    # Build the plugin
    if go build -o kubectl-fabricx .; then
        print_success "kubectl-fabricx plugin built successfully"
    else
        print_error "Failed to build kubectl-fabricx plugin"
        exit 1
    fi
    
    # Install the plugin
    local install_dir="$HOME/.local/bin"
    mkdir -p "$install_dir"
    
    if cp kubectl-fabricx "$install_dir/"; then
        print_success "kubectl-fabricx plugin installed to $install_dir"
    else
        print_error "Failed to install kubectl-fabricx plugin"
        exit 1
    fi
    
    cd "$PROJECT_ROOT"
}

# Function to install the operator
install_operator() {
    print_status "Installing fabric-x-operator..."
    
    if ./scripts/install-fabric-x-operator.sh; then
        print_success "fabric-x-operator installed successfully"
    else
        print_error "Failed to install fabric-x-operator"
        exit 1
    fi
}

# Function to verify setup
verify_setup() {
    print_status "Verifying setup..."
    
    # Check if kubectl-fabricx plugin is available
    if command_exists kubectl-fabricx; then
        print_success "kubectl-fabricx plugin is available"
    else
        print_warning "kubectl-fabricx plugin might not be in PATH"
        print_status "You may need to add $HOME/.local/bin to your PATH"
    fi
    
    # Check if operator is running
    if kubectl get pods -n fabric-x-system -l control-plane=controller-manager --no-headers 2>/dev/null | grep -q "Running"; then
        print_success "fabric-x-operator is running"
    else
        print_warning "fabric-x-operator might not be running"
    fi
    
    # Check if CRDs are installed
    if kubectl get crd | grep -q "fabricx.kfsoft.tech"; then
        print_success "CRDs are installed"
    else
        print_warning "CRDs might not be installed"
    fi
}

# Function to show usage information
show_usage() {
    print_success "Fabric-X setup completed successfully!"
    echo
    echo "You can now use:"
    echo
    echo "1. kubectl-fabricx plugin:"
    echo "   kubectl fabricx ca create --name my-ca --namespace my-ns"
    echo "   kubectl fabricx ca --help"
    echo
    echo "2. Direct kubectl commands:"
    echo "   kubectl get ca -A"
    echo "   kubectl get endorser -A"
    echo "   kubectl get committer -A"
    echo "   kubectl get orderergroup -A"
    echo
    echo "3. To uninstall everything:"
    echo "   ./scripts/uninstall-fabric-x-operator.sh"
    echo "   rm $HOME/.local/bin/kubectl-fabricx"
    echo
    echo "4. To rebuild the plugin:"
    echo "   cd kubectl-fabricx && go build -o kubectl-fabricx ."
    echo "   cp kubectl-fabricx $HOME/.local/bin/"
}

# Function to cleanup on error
cleanup() {
    print_error "Setup failed. Cleaning up..."
    # Add cleanup logic here if needed
}

# Set up error handling
trap cleanup ERR

# Main setup function
main() {
    echo "=========================================="
    echo "Fabric-X Complete Setup Script"
    echo "=========================================="
    echo
    
    # Check prerequisites
    check_prerequisites
    
    # Build kubectl-fabricx plugin
    build_kubectl_plugin
    
    # Install operator
    install_operator
    
    # Verify setup
    verify_setup
    
    # Show usage information
    show_usage
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo
            echo "Options:"
            echo "  --help, -h     Show this help message"
            echo "  --plugin-only  Only build and install the kubectl-fabricx plugin"
            echo "  --operator-only Only install the fabric-x-operator"
            echo
            echo "This script will:"
            echo "  1. Check prerequisites (docker, k3d, kubectl, make, go)"
            echo "  2. Build and install kubectl-fabricx plugin"
            echo "  3. Install fabric-x-operator"
            echo "  4. Verify the setup"
            exit 0
            ;;
        --plugin-only)
            PLUGIN_ONLY=true
            shift
            ;;
        --operator-only)
            OPERATOR_ONLY=true
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Run main setup
if [ "$PLUGIN_ONLY" = "true" ]; then
    echo "=========================================="
    echo "Fabric-X Plugin Setup Only"
    echo "=========================================="
    echo
    
    check_prerequisites
    build_kubectl_plugin
    verify_setup
    show_usage
elif [ "$OPERATOR_ONLY" = "true" ]; then
    echo "=========================================="
    echo "Fabric-X Operator Setup Only"
    echo "=========================================="
    echo
    
    check_prerequisites
    install_operator
    verify_setup
    show_usage
else
    main
fi 