#!/bin/bash

# Test script for batcher downsizing functionality

set -e

echo "=== Testing Batcher Downsizing Functionality ==="

# Apply the initial OrdererGroup with 3 batchers
echo "1. Applying OrdererGroup with 3 batchers..."
kubectl apply -f test-orderergroup-downsizing.yaml

# Wait for the resources to be created
echo "2. Waiting for resources to be created..."
sleep 30

# Check that all 3 batcher deployments exist
echo "3. Verifying 3 batcher deployments exist..."
kubectl get deployments | grep test-orderergroup-downsizing-batcher

# Apply the downsized OrdererGroup with 2 batchers
echo "4. Applying downsized OrdererGroup with 2 batchers..."
kubectl apply -f test-orderergroup-downsized.yaml

# Wait for the downsizing to take effect
echo "5. Waiting for downsizing to take effect..."
sleep 30

# Check that only 2 batcher deployments exist (batcher-2 should be removed)
echo "6. Verifying only 2 batcher deployments exist (batcher-2 should be removed)..."
kubectl get deployments | grep test-orderergroup-downsizing-batcher

# Check that batcher-2 deployment is gone
echo "7. Verifying batcher-2 deployment is removed..."
if kubectl get deployment test-orderergroup-downsizing-batcher-2 2>/dev/null; then
    echo "ERROR: batcher-2 deployment still exists"
    exit 1
else
    echo "SUCCESS: batcher-2 deployment has been removed"
fi

# Check that related resources for batcher-2 are also removed
echo "8. Verifying batcher-2 related resources are removed..."
if kubectl get service test-orderergroup-downsizing-batcher-2 2>/dev/null; then
    echo "ERROR: batcher-2 service still exists"
    exit 1
else
    echo "SUCCESS: batcher-2 service has been removed"
fi

if kubectl get configmap test-orderergroup-downsizing-batcher-2-config 2>/dev/null; then
    echo "ERROR: batcher-2 configmap still exists"
    exit 1
else
    echo "SUCCESS: batcher-2 configmap has been removed"
fi

if kubectl get pvc test-orderergroup-downsizing-batcher-2-store-pvc 2>/dev/null; then
    echo "ERROR: batcher-2 PVC still exists"
    exit 1
else
    echo "SUCCESS: batcher-2 PVC has been removed"
fi

echo "=== Downsizing Test Completed Successfully ==="

# Cleanup
echo "9. Cleaning up test resources..."
kubectl delete -f test-orderergroup-downsized.yaml

echo "Test completed successfully!" 