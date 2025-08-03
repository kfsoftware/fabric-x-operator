#!/bin/bash

# Test script for OrdererGroup status updates
set -e

echo "=== Testing OrdererGroup Status Updates ==="

# Apply the test OrdererGroup
echo "Applying test OrdererGroup..."
kubectl apply -f test-orderergroup-status.yaml

# Wait a moment for the controller to process
echo "Waiting for controller to process..."
sleep 5

# Check the status
echo "Checking OrdererGroup status..."
kubectl get orderergroups test-orderergroup-status -o wide

echo ""
echo "Detailed status:"
kubectl describe orderergroup test-orderergroup-status

echo ""
echo "Checking logs for status update messages..."
kubectl logs -n default -l app.kubernetes.io/name=fabric-x-operator --tail=20 | grep -E "(Updating OrdererGroup status|OrdererGroup status updated|Failed to reconcile)"

echo ""
echo "=== Test completed ==="
echo "If you see 'FAILED' status with an error message, the status updates are working correctly."
echo "If you see empty status or no error message, there may be an issue with status updates."

# Clean up
echo ""
echo "Cleaning up test OrdererGroup..."
kubectl delete -f test-orderergroup-status.yaml 