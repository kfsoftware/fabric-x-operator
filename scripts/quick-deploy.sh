#!/bin/bash

# Quick deploy script for Fabric X operator using pre-built binary
# This is faster than docker-build as it doesn't rebuild in Docker

set -e

# Generate unique image tag with timestamp
export IMAGE=local/fabric-x-operator:$(date +%Y%m%d%H%M%S)

echo "Building operator with image: $IMAGE"

# Build binary and docker image with pre-built binary (faster)
make docker-build-binary IMG=$IMAGE

# Import image into K3D cluster
echo "Importing image to k3d cluster..."
k3d image import $IMAGE --cluster k8s-hlf

# Deploy the operator
echo "Deploying operator..."
make deploy IMG=$IMAGE

echo "Deployment complete with image: $IMAGE"
echo ""
echo "Wait for operator to be ready with:"
echo "  kubectl wait --for=condition=ready pod -l control-plane=controller-manager -n fabric-x-operator-system --timeout=120s"
