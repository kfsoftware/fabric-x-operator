# Fabric-X Operator Installation Scripts

This directory contains scripts for installing and managing the Fabric-X Operator in a k3d environment.

## Scripts Overview

### 1. `install-fabric-x-operator.sh`
**Purpose**: Installs the Fabric-X Operator into a k3d cluster

**What it does**:
- Checks prerequisites (docker, k3d, kubectl, make)
- Builds the operator Docker image
- Loads the image into k3d cluster
- Installs Custom Resource Definitions (CRDs)
- Deploys the operator
- Waits for operator to be ready
- Verifies the installation

**Usage**:
```bash
# Basic installation
./scripts/install-fabric-x-operator.sh

# Custom image name and tag
./scripts/install-fabric-x-operator.sh --image my-registry/fabric-x-operator --tag v1.0.0

# Custom k3d cluster name
./scripts/install-fabric-x-operator.sh --cluster my-k3d-cluster

# Show help
./scripts/install-fabric-x-operator.sh --help
```

### 2. `uninstall-fabric-x-operator.sh`
**Purpose**: Removes the Fabric-X Operator and cleans up resources

**What it does**:
- Deletes all custom resources (CA, Endorser, Committer, OrdererGroup)
- Undeploys the operator
- Uninstalls CRDs
- Deletes the operator namespace
- Cleans up k3d images
- Verifies the uninstallation

**Usage**:
```bash
# Interactive uninstallation (with confirmation)
./scripts/uninstall-fabric-x-operator.sh

# Force uninstallation (no confirmation)
./scripts/uninstall-fabric-x-operator.sh --force

# Custom k3d cluster name
./scripts/uninstall-fabric-x-operator.sh --cluster my-k3d-cluster

# Show help
./scripts/uninstall-fabric-x-operator.sh --help
```

### 3. `setup-fabric-x.sh`
**Purpose**: Complete setup script that installs both the operator and kubectl-fabricx plugin

**What it does**:
- Checks all prerequisites (docker, k3d, kubectl, make, go)
- Builds and installs kubectl-fabricx plugin
- Installs the fabric-x-operator
- Verifies the complete setup

**Usage**:
```bash
# Complete setup (operator + plugin)
./scripts/setup-fabric-x.sh

# Plugin only
./scripts/setup-fabric-x.sh --plugin-only

# Operator only
./scripts/setup-fabric-x.sh --operator-only

# Show help
./scripts/setup-fabric-x.sh --help
```

## Prerequisites

Before running these scripts, ensure you have the following tools installed:

- **Docker**: For building and running containers
- **k3d**: For local Kubernetes cluster management
- **kubectl**: For Kubernetes cluster interaction
- **make**: For building the operator
- **go**: For building the kubectl plugin (only for setup-fabric-x.sh)

## Quick Start

### Option 1: Complete Setup (Recommended)
```bash
# Install everything (operator + plugin)
./scripts/setup-fabric-x.sh
```

### Option 2: Step-by-Step Setup
```bash
# 1. Install the operator
./scripts/install-fabric-x-operator.sh

# 2. Build and install the plugin (optional)
cd kubectl-fabricx
go build -o kubectl-fabricx .
cp kubectl-fabricx ~/.local/bin/
```

## Verification

After installation, verify that everything is working:

```bash
# Check if operator is running
kubectl get pods -n fabric-x-system

# Check if CRDs are installed
kubectl get crd | grep fabricx

# Test the plugin (if installed)
kubectl fabricx --help
```

## Usage Examples

### Using kubectl-fabricx plugin:
```bash
# Create a CA
kubectl fabricx ca create --name my-ca --namespace my-ns

# List CAs
kubectl get ca -A

# Get CA details
kubectl describe ca my-ca -n my-ns
```

### Using direct kubectl commands:
```bash
# List all custom resources
kubectl get ca,endorser,committer,orderergroup -A

# Get operator logs
kubectl logs -n fabric-x-system deployment/controller-manager
```

## Troubleshooting

### Common Issues

1. **k3d cluster not found**
   ```bash
   # Create the cluster first
   k3d cluster create k3d-k8s-hlf
   ```

2. **Image build fails**
   ```bash
   # Check Docker is running
   docker ps
   
   # Try building manually
   make docker-build IMG=kfsoft.tech/fabric-x-operator:latest
   ```

3. **Plugin not found**
   ```bash
   # Add to PATH
   export PATH="$HOME/.local/bin:$PATH"
   
   # Or install manually
   cd kubectl-fabricx
   go build -o kubectl-fabricx .
   sudo cp kubectl-fabricx /usr/local/bin/
   ```

4. **Operator not ready**
   ```bash
   # Check operator logs
   kubectl logs -n fabric-x-system deployment/controller-manager
   
   # Check events
   kubectl get events -n fabric-x-system
   ```

### Cleanup

To completely remove everything:
```bash
# Remove operator and resources
./scripts/uninstall-fabric-x-operator.sh --force

# Remove plugin
rm ~/.local/bin/kubectl-fabricx
```

## Configuration

You can customize the installation by modifying the variables at the top of each script:

- `OPERATOR_NAMESPACE`: Namespace for the operator (default: fabric-x-system)
- `IMAGE_NAME`: Docker image name (default: kfsoft.tech/fabric-x-operator)
- `IMAGE_TAG`: Docker image tag (default: latest)
- `K3D_CLUSTER_NAME`: k3d cluster name (default: k3d-k8s-hlf)

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review the operator logs: `kubectl logs -n fabric-x-system deployment/controller-manager`
3. Check the project documentation in the main README.md 