# sonic-change-agent Development Workflow

This document describes the manual development workflow for testing and debugging the sonic-change-agent using the new test architecture.

## Overview

The test environment provides a complete Kubernetes cluster with:
- **minikube cluster** (`sonic-test` profile)
- **Redis** for CONFIG_DB simulation
- **sonic-change-agent** controller
- **NetworkDevice CRD** for triggering workflows

## Quick Start

```bash
# Complete workflow in one go
./test/scripts/dev-env.py setup
./test/scripts/dev-env.py device sonic-test --operation OSUpgrade --action PreloadImage
./test/scripts/dev-env.py logs "my-test-session"
./test/scripts/dev-env.py cleanup
```

## Detailed Workflow

### 1. Environment Setup

#### Clean Start
```bash
# Remove any existing test environment
./test/scripts/dev-env.py cleanup
```

#### Full Setup
```bash
# Create cluster, build image, deploy all components
./test/scripts/dev-env.py setup

# Or with existing Docker image
./test/scripts/dev-env.py setup --skip-build
```

What this does:
- Creates minikube cluster with profile `sonic-test`
- Builds Docker image `sonic-change-agent:test`
- Deploys Redis with CONFIG_DB configuration
- Deploys sonic-change-agent controller
- Sets up NetworkDevice CRD and RBAC

### 2. Environment Verification

```bash
# Check overall status
./test/scripts/dev-env.py status
```

Expected output:
```
📊 Environment Status (cluster: sonic-test)
✅ Cluster: Running

🐳 Pods:
NAME                       READY   STATUS    RESTARTS   AGE
redis-7b7bb4fcb6-xxxxx     1/1     Running   0          30s
sonic-change-agent-xxxxx   1/1     Running   0          15s

📡 NetworkDevices: None
```

### 3. Triggering Workflows

#### Create NetworkDevice for Preload
```bash
# IMPORTANT: Device name must match node name (sonic-test)
./test/scripts/dev-env.py device sonic-test --operation OSUpgrade --action PreloadImage

# Optional: Custom firmware profile and OS version
./test/scripts/dev-env.py device sonic-test \
  --operation OSUpgrade \
  --action PreloadImage \
  --os-version "202505.01" \
  --firmware-profile "SONiC-Custom-Profile"
```

#### Other Workflow Types
```bash
# Different operations (future)
./test/scripts/dev-env.py device sonic-test --operation ConfigUpdate --action ApplyConfig
./test/scripts/dev-env.py device sonic-test --operation Reboot --action SoftReboot
```

### 4. Verification and Monitoring

#### Real-time Log Monitoring
```bash
# Follow controller logs
minikube kubectl --profile sonic-test -- logs -l app=sonic-change-agent -f

# Get recent logs
minikube kubectl --profile sonic-test -- logs -l app=sonic-change-agent --tail=20

# Logs since specific time
minikube kubectl --profile sonic-test -- logs -l app=sonic-change-agent --since=2m
```

#### NetworkDevice Status
```bash
# Check NetworkDevice resource
minikube kubectl --profile sonic-test -- get networkdevice sonic-test -o yaml

# Check just the status
minikube kubectl --profile sonic-test -- get networkdevice sonic-test -o jsonpath='{.status}'

# Describe for events
minikube kubectl --profile sonic-test -- describe networkdevice sonic-test
```

#### Cluster-wide Status
```bash
# All pods
minikube kubectl --profile sonic-test -- get pods -o wide

# All NetworkDevices
minikube kubectl --profile sonic-test -- get networkdevices

# Recent events
minikube kubectl --profile sonic-test -- get events --sort-by='.lastTimestamp'
```

### 5. Log Collection

```bash
# Collect comprehensive logs for debugging
./test/scripts/dev-env.py logs "preload-debug-session"
```

This creates a timestamped directory under `test_logs/` with:
- Pod logs from all containers
- Pod descriptions
- Cluster-wide information (nodes, services, events)
- NetworkDevice resources
- Summary README

### 6. Development Iteration

#### Quick Redeploy (code changes)
```bash
# Rebuild and redeploy after code changes
./test/scripts/dev-env.py deploy --rebuild
```

#### Patch Testing (no rebuild)
```bash
# Use existing image for faster testing
./test/scripts/dev-env.py deploy
```

### 7. Cleanup

```bash
# Remove entire test environment
./test/scripts/dev-env.py cleanup
```

## Expected Log Output

### Successful Preload Workflow
```
I1119 19:47:27.505107 controller.go:115] "NetworkDevice ADDED" device="sonic-test"
I1119 19:47:27.505127 controller.go:152] "Reconciliation state" deviceType="leafRouter" osVersion="202505.01" firmwareProfile="SONiC-Test-Profile" operation="OSUpgrade" operationAction="PreloadImage"
I1119 19:47:27.505135 controller.go:191] "Starting workflow execution" operation="OSUpgrade" operationAction="PreloadImage" osVersion="202505.01"
I1119 19:47:27.519895 controller.go:272] "Operation status updated successfully" operationState="in_progress"
I1119 19:47:27.519918 preload.go:48] "Executing preload workflow" osVersion="202505.01" firmwareProfile="SONiC-Test-Profile" imageURL="http://image-repo.example.com/sonic-202505.01-SONiC-Test-Profile.bin"
I1119 19:47:27.519937 service.go:27] "Starting file transfer via gNOI file service" sourceURL="http://image-repo.example.com/sonic-202505.01-SONiC-Test-Profile.bin"
I1119 19:47:27.519943 service.go:33] "DRY_RUN: Would transfer file via gNOI file.TransferToRemote"
I1119 19:47:27.519948 preload.go:59] "Preload workflow completed successfully"
I1119 19:47:27.519965 controller.go:208] "Workflow execution completed successfully"
```

### NetworkDevice Status (Completed)
```yaml
status:
  lastTransitionTime: "2025-11-19T19:47:27Z"
  operationActionState: completed
  operationState: completed
  state: Ready
```

## Troubleshooting

### Common Issues

#### 1. Device Name Mismatch
**Problem**: No workflow execution logs
**Solution**: Ensure device name matches node name (`sonic-test`)
```bash
# Check node name
minikube kubectl --profile sonic-test -- get nodes

# Create device with matching name
./test/scripts/dev-env.py device sonic-test
```

#### 2. Controller Not Running
**Problem**: No sonic-change-agent pod
**Solution**: Check deployment status
```bash
minikube kubectl --profile sonic-test -- get pods
minikube kubectl --profile sonic-test -- describe daemonset sonic-change-agent
```

#### 3. Image Build Issues
**Problem**: Docker build failures
**Solution**: Check Dockerfile path and dependencies
```bash
# Manual build test
docker build -f Dockerfile.sonic-change-agent -t sonic-change-agent:test .
```

### Debug Commands

```bash
# Controller startup logs
minikube kubectl --profile sonic-test -- logs -l app=sonic-change-agent --since=5m

# Redis connectivity
minikube kubectl --profile sonic-test -- exec deployment/redis -- redis-cli ping

# CRD status
minikube kubectl --profile sonic-test -- get crd networkdevices.sonic.k8s.io

# RBAC issues
minikube kubectl --profile sonic-test -- auth can-i '*' '*' --as=system:serviceaccount:default:sonic-change-agent
```

## Integration with pytest

The same TestEnvironment class powers both manual workflows and pytest:

```bash
# Use existing environment for faster testing
cd test && python3 -m pytest --reuse-env -k "test_preload_workflow" -v

# Normal full test suite
make test-integration
```

## Architecture

```
test/
├── lib/
│   └── environment.py     # TestEnvironment class
├── scripts/
│   └── dev-env.py         # CLI for manual operations
├── conftest.py            # pytest fixtures using TestEnvironment
└── test_integration.py    # Integration tests
```

The TestEnvironment class provides:
- `setup_cluster()` - minikube cluster creation
- `build_image()` - Docker image building
- `deploy_redis()` - Redis deployment
- `deploy_agent()` - sonic-change-agent deployment
- `create_device()` - NetworkDevice creation
- `collect_logs()` - Comprehensive log collection
- `cleanup()` - Environment teardown
- `status()` - Environment status checking

This unified architecture allows seamless switching between manual debugging and automated testing.