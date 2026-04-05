#!/bin/bash

# Test OrdererGroup Setup Script
# This script helps set up and test the OrdererGroup with Fabric CA

set -e

echo "=== Fabric X OrdererGroup Test Setup ==="

# Configuration
CA_HOST="test-ca2"
CA_PORT="7054"
CA_SECRET_NAME="test-ca2"
CA_CERT_KEY="ca-cert.pem"
NAMESPACE="default"
ENROLL_ID="admin"
ENROLL_SECRET="adminpw"
ORDERERGROUP_NAME="test-orderergroup"

echo "Configuration:"
echo "  CA Host: $CA_HOST"
echo "  CA Port: $CA_PORT"
echo "  CA Secret: $CA_SECRET_NAME"
echo "  Namespace: $NAMESPACE"
echo "  Enroll ID: $ENROLL_ID"
echo "  Enroll Secret: $ENROLL_SECRET"
echo "  OrdererGroup: $ORDERERGROUP_NAME"
echo ""

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "Error: kubectl is not installed or not in PATH"
    exit 1
fi

# Check if the namespace exists
echo "Checking namespace..."
if ! kubectl get namespace $NAMESPACE &> /dev/null; then
    echo "Creating namespace $NAMESPACE..."
    kubectl create namespace $NAMESPACE
fi

# Check if the CA certificate secret exists
echo "Checking CA certificate secret..."
if ! kubectl get secret $CA_SECRET_NAME -n $NAMESPACE &> /dev/null; then
    echo "Warning: CA certificate secret '$CA_SECRET_NAME' not found in namespace '$NAMESPACE'"
    echo "You need to create this secret with your CA certificate:"
    echo ""
    echo "kubectl create secret generic $CA_SECRET_NAME \\"
    echo "  --from-file=$CA_CERT_KEY=/path/to/your/ca-cert.pem \\"
    echo "  --namespace=$NAMESPACE"
    echo ""
    echo "Or if you have the CA certificate content:"
    echo "kubectl create secret generic $CA_SECRET_NAME \\"
    echo "  --from-literal=$CA_CERT_KEY='-----BEGIN CERTIFICATE-----...' \\"
    echo "  --namespace=$NAMESPACE"
    echo ""
    read -p "Do you want to continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Setup cancelled."
        exit 1
    fi
else
    echo "✓ CA certificate secret found"
fi

# Check if the operator is running
echo "Checking if Fabric X Operator is running..."
if ! kubectl get pods -n fabric-x-system --field-selector=status.phase=Running | grep -q fabric-x-operator; then
    echo "Warning: Fabric X Operator doesn't seem to be running in fabric-x-system namespace"
    echo "Make sure the operator is deployed and running before proceeding."
    echo ""
    read -p "Do you want to continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Setup cancelled."
        exit 1
    fi
else
    echo "✓ Fabric X Operator is running"
fi

# Apply the OrdererGroup
echo ""
echo "Applying OrdererGroup configuration..."
kubectl apply -f test-orderergroup-ca.yaml

echo ""
echo "=== OrdererGroup Applied ==="
echo "You can monitor the progress with these commands:"
echo ""
echo "# Check OrdererGroup status:"
echo "kubectl get orderergroup $ORDERERGROUP_NAME -n $NAMESPACE -o yaml"
echo ""
echo "# Check operator logs:"
echo "kubectl logs -f deployment/fabric-x-operator -n fabric-x-system"
echo ""
echo "# Check generated certificate secrets:"
echo "kubectl get secrets -l fabric-x/orderergroup=$ORDERERGROUP_NAME -n $NAMESPACE"
echo ""
echo "# Check specific component certificates:"
echo "kubectl describe secret $ORDERERGROUP_NAME-consenter-sign-cert -n $NAMESPACE"
echo "kubectl describe secret $ORDERERGROUP_NAME-consenter-tls-cert -n $NAMESPACE"
echo ""
echo "# Check component status:"
echo "kubectl get pods -l fabric-x/orderergroup=$ORDERERGROUP_NAME -n $NAMESPACE"
echo ""
echo "# Delete the OrdererGroup when done testing:"
echo "kubectl delete orderergroup $ORDERERGROUP_NAME -n $NAMESPACE"
echo ""

# Wait a moment and show initial status
echo "Waiting 10 seconds for initial reconciliation..."
sleep 10

echo "=== Initial Status ==="
echo "OrdererGroup:"
kubectl get orderergroup $ORDERERGROUP_NAME -n $NAMESPACE -o wide

echo ""
echo "Certificate Secrets:"
kubectl get secrets -l fabric-x/orderergroup=$ORDERERGROUP_NAME -n $NAMESPACE

echo ""
echo "=== Setup Complete ==="
echo "The OrdererGroup is now being reconciled by the operator."
echo "Check the logs above for any errors or success messages." 