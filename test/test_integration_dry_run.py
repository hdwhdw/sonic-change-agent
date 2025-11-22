"""
Dry run integration tests for sonic-change-agent.

These tests run with DRY_RUN=true and validate workflow logic without actual 
file transfers. They focus on essential end-to-end scenarios that require 
a Kubernetes cluster. Most test coverage is provided by unit tests in pkg/ packages.

For real end-to-end tests with actual HTTP transfers, see test_integration_real_e2e.py.
"""

import pytest
import time
import subprocess


def kubectl(*args):
    """Helper to run kubectl commands in test cluster."""
    cmd = ["minikube", "kubectl", "--profile", "sonic-test", "--"] + list(args)
    return subprocess.run(cmd, capture_output=True, text=True)


@pytest.mark.workflow
def test_end_to_end_preload_workflow(sonic_deployment, network_device):
    """
    Essential E2E test: Complete preload workflow from CRD creation to completion.
    
    This is the core integration test that verifies:
    - Kubernetes controller watches NetworkDevice CRD
    - Workflow factory creates correct workflow
    - gNOI client executes file transfer (DRY_RUN mode)
    - Status is updated back to CRD
    """
    # Create NetworkDevice for PreloadImage - use sonic-test to match the agent's deviceName
    device_name = network_device("sonic-test", 
                                operation="OSUpgrade", 
                                operationAction="PreloadImage",
                                osVersion="202505.01",
                                firmwareProfile="SONiC-Test-Profile")
    
    # Wait for workflow execution
    print(f"Waiting for E2E workflow execution on {device_name}...")
    
    # Verify the NetworkDevice was created successfully
    result = kubectl("get", "networkdevice", device_name, "-o", "yaml")
    assert result.returncode == 0, f"Failed to create NetworkDevice: {result.stderr}"
    print("✅ NetworkDevice created successfully")
    
    # Wait for controller processing
    time.sleep(10)
    
    # Check controller logs for complete workflow execution
    result = kubectl("logs", "daemonset/sonic-change-agent", "--tail=50")
    assert result.returncode == 0, "Failed to get controller logs"
    
    logs = result.stdout
    print("Controller logs:", logs[-500:])  # Show recent logs for debugging
    
    # Verify complete workflow execution path
    assert "NetworkDevice ADDED" in logs, "Controller didn't detect NetworkDevice"
    assert "Starting workflow execution" in logs, "Workflow execution not started"
    assert "Executing preload workflow" in logs, "Preload workflow not executed"
    assert "202505.01" in logs, "OS version not processed"
    assert "SONiC-Test-Profile" in logs, "Firmware profile not processed"
    assert "DRY_RUN: Would transfer file" in logs, "gNOI file transfer not attempted"
    assert "Preload workflow completed successfully" in logs, "Workflow not completed successfully"
    
    # Verify NetworkDevice status was updated by controller
    result = kubectl("get", "networkdevice", device_name, "-o", "yaml")
    assert result.returncode == 0, "Failed to get updated NetworkDevice"
    
    device_yaml = result.stdout
    assert "status:" in device_yaml, "NetworkDevice status not updated"
    assert "operationState:" in device_yaml, "OperationState not set"
    assert "lastTransitionTime:" in device_yaml, "LastTransitionTime not set"
    
    print("✅ End-to-end PreloadImage workflow test passed")


def test_essential_system_health(sonic_deployment):
    """
    Essential system health check: Verify core components are running.
    
    Tests that all required components can start and initialize properly
    in a real Kubernetes environment.
    """
    # Verify all required pods are running
    result = kubectl("get", "pods", "-o", "wide")
    assert result.returncode == 0, "Failed to get pods status"
    
    pods_output = result.stdout
    print("Pods status:", pods_output)
    
    assert "redis" in pods_output, "Redis pod not found"
    assert "sonic-change-agent" in pods_output, "sonic-change-agent pod not found"
    assert "Running" in pods_output, "Not all required pods are running"
    
    # Verify sonic-change-agent is healthy
    result = kubectl("get", "pods", "-l", "app=sonic-change-agent")
    assert result.returncode == 0, "Failed to get sonic-change-agent pod status"
    assert "CrashLoopBackOff" not in result.stdout, "sonic-change-agent is crash looping"
    assert "Error" not in result.stdout, "sonic-change-agent pod in error state"
    
    # Verify controller startup and initialization
    result = kubectl("logs", "daemonset/sonic-change-agent")
    assert result.returncode == 0, "Failed to get controller logs"
    
    full_logs = result.stdout
    assert "Starting sonic-change-agent" in full_logs, "Controller never started"
    assert "Starting controller" in full_logs, "Controller initialization failed"
    assert "Cache synced successfully" in full_logs, "Kubernetes informer cache never synced"
    
    # Verify no recent crashes or panics
    result = kubectl("logs", "daemonset/sonic-change-agent", "--tail=50")
    assert result.returncode == 0, "Failed to get recent logs"
    
    recent_logs = result.stdout.lower()
    assert "panic" not in recent_logs, "Panic found in recent logs"
    assert "fatal" not in recent_logs, "Fatal error found in recent logs"
    
    print("✅ Essential system health test passed")


def test_crd_deployment_integration(sonic_deployment):
    """
    Test CRD deployment and basic functionality in Kubernetes.
    
    Verifies that the NetworkDevice CRD is properly deployed and can be
    accessed through the Kubernetes API.
    """
    # Verify CRD is deployed and accessible
    result = kubectl("get", "crd", "networkdevices.sonic.k8s.io")
    assert result.returncode == 0, "NetworkDevice CRD not found in cluster"
    
    # Verify CRD schema and metadata
    result = kubectl("get", "crd", "networkdevices.sonic.k8s.io", "-o", "yaml")
    assert result.returncode == 0, "Failed to get CRD details"
    
    crd_yaml = result.stdout
    assert "sonic.k8s.io" in crd_yaml, "Wrong API group in deployed CRD"
    assert "NetworkDevice" in crd_yaml, "Wrong kind in deployed CRD"
    assert "status:" in crd_yaml, "Status subresource not enabled in CRD"
    assert "v1" in crd_yaml, "API version not properly set"
    
    # Test that we can create and list NetworkDevice resources
    result = kubectl("get", "networkdevices")
    assert result.returncode == 0, "Cannot list NetworkDevice resources"
    
    print("✅ CRD deployment integration test passed")