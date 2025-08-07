#!/bin/bash

# Test script to verify bootstrap mode functionality
# This script tests that secrets are created when bootstrapMode is configured
# and that full deployment happens when bootstrapMode is "deploy"

set -e

echo "=== Testing Bootstrap Mode Functionality ==="

# Function to check if secrets exist
check_secrets() {
    local component=$1
    local name=$2
    local namespace=${3:-default}
    
    echo "Checking secrets for $component $name..."
    
    # Check for sign certificate secret
    if kubectl get secret "${name}-sign-cert" -n "$namespace" >/dev/null 2>&1; then
        echo "✓ Sign certificate secret exists for $component $name"
    else
        echo "✗ Sign certificate secret missing for $component $name"
        return 1
    fi
    
    # Check for TLS certificate secret
    if kubectl get secret "${name}-tls-cert" -n "$namespace" >/dev/null 2>&1; then
        echo "✓ TLS certificate secret exists for $component $name"
    else
        echo "✗ TLS certificate secret missing for $component $name"
        return 1
    fi
}

# Function to check if deployment exists
check_deployment() {
    local component=$1
    local name=$2
    local namespace=${3:-default}
    
    echo "Checking deployment for $component $name..."
    
    if kubectl get deployment "$name" -n "$namespace" >/dev/null 2>&1; then
        echo "✓ Deployment exists for $component $name"
    else
        echo "✗ Deployment missing for $component $name"
        return 1
    fi
}

# Function to check if service exists
check_service() {
    local component=$1
    local name=$2
    local namespace=${3:-default}
    
    echo "Checking service for $component $name..."
    
    if kubectl get service "$name" -n "$namespace" >/dev/null 2>&1; then
        echo "✓ Service exists for $component $name"
    else
        echo "✗ Service missing for $component $name"
        return 1
    fi
}

# Function to test a component in configure mode
test_configure_mode() {
    local component=$1
    local name=$2
    
    echo "=== Testing $component in configure mode ==="
    
    # Apply the configure mode resource
    kubectl apply -f test-bootstrap-mode.yaml
    
    # Wait a bit for reconciliation
    sleep 10
    
    # Check that secrets are created
    if check_secrets "$component" "$name"; then
        echo "✓ Configure mode secrets created successfully for $component"
    else
        echo "✗ Configure mode secrets failed for $component"
        return 1
    fi
    
    # Check that deployment is NOT created (configure mode should not create deployments)
    if kubectl get deployment "$name" -n default >/dev/null 2>&1; then
        echo "✗ Deployment should not exist in configure mode for $component"
        return 1
    else
        echo "✓ No deployment created in configure mode for $component (expected)"
    fi
    
    # Clean up
    kubectl delete "$component" "$name" -n default --ignore-not-found=true
    sleep 5
}

# Function to test a component in deploy mode
test_deploy_mode() {
    local component=$1
    local name=$2
    
    echo "=== Testing $component in deploy mode ==="
    
    # Apply the deploy mode resource
    kubectl apply -f test-bootstrap-mode.yaml
    
    # Wait a bit for reconciliation
    sleep 15
    
    # Check that secrets are created
    if check_secrets "$component" "$name"; then
        echo "✓ Deploy mode secrets created successfully for $component"
    else
        echo "✗ Deploy mode secrets failed for $component"
        return 1
    fi
    
    # Check that deployment is created
    if check_deployment "$component" "$name"; then
        echo "✓ Deploy mode deployment created successfully for $component"
    else
        echo "✗ Deploy mode deployment failed for $component"
        return 1
    fi
    
    # Check that service is created
    if check_service "$component" "$name"; then
        echo "✓ Deploy mode service created successfully for $component"
    else
        echo "✗ Deploy mode service failed for $component"
        return 1
    fi
    
    # Clean up
    kubectl delete "$component" "$name" -n default --ignore-not-found=true
    sleep 5
}

# Test all components in configure mode
echo "Testing configure mode for all components..."
test_configure_mode "OrdererConsenter" "test-consenter-configure"
test_configure_mode "OrdererAssembler" "test-assembler-configure"
test_configure_mode "OrdererBatcher" "test-batcher-configure"
test_configure_mode "OrdererRouter" "test-router-configure"

# Test all components in deploy mode
echo "Testing deploy mode for all components..."
test_deploy_mode "OrdererConsenter" "test-consenter-deploy"
test_deploy_mode "OrdererAssembler" "test-assembler-deploy"
test_deploy_mode "OrdererBatcher" "test-batcher-deploy"
test_deploy_mode "OrdererRouter" "test-router-deploy"

echo "=== Bootstrap Mode Testing Completed ==="
echo "All tests passed! Bootstrap mode functionality is working correctly."
