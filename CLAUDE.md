# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

sonic-change-agent is a Kubernetes controller for managing SONiC network devices through gRPC Network Operations Interface (gNOI). The controller watches NetworkDevice CRD resources and executes automated workflows like OS upgrades.

**Technology Stack:**
- Go 1.21 (primary language)
- Kubernetes controller-runtime pattern
- gRPC/gNOI for device communication  
- Redis integration for SONiC CONFIG_DB
- Python pytest for integration testing

## Essential Development Commands

### Quick Development Workflow
- `make dev` - Format, vet, unit test, and build (fastest development cycle)
- `make check` - Fast validation without building
- `make ci` - Full CI workflow (format, vet, test, docker-build) - **ALWAYS run before committing**

### Build Commands  
- `make build` - Build the binary to `build/bin/sonic-change-agent`
- `make docker-build` - Build Docker images

### Testing Commands
- `make test` - Run all tests (unit + integration) 
- `make unit` - Go unit tests only with coverage
- `make test-integration` - Integration tests with Docker rebuild (full)
- `make test-quick` - Integration tests without Docker rebuild (faster)
- `make coverage` - Generate HTML coverage report in `build/coverage/`

### Code Quality
- `make fmt` - Format Go code
- `make vet` - Run Go vet  
- `make lint` - Run golangci-lint (requires separate installation)
- `make tidy` - Run go mod tidy

### Manual Testing Environment
- `./test/scripts/dev-env.py setup` - Create complete test environment
- `./test/scripts/dev-env.py device sonic-test --operation OSUpgrade --action PreloadImage` - Trigger workflow
- `./test/scripts/dev-env.py logs "session-name"` - Collect comprehensive logs
- `./test/scripts/dev-env.py cleanup` - Remove test environment

## Architecture

### Core Package Structure
- `pkg/controller/` - Kubernetes controller logic for NetworkDevice CRD reconciliation
- `pkg/gnoi/` - gNOI client/server implementation with file transfer services
- `pkg/workflow/` - Workflow implementations (currently `PreloadImage` for `OSUpgrade`)
- `pkg/config/` - Redis CONFIG_DB integration for SONiC device configuration
- `pkg/security/` - Path validation and security features

### Key Design Patterns
- **Controller Pattern**: Watches NetworkDevice CRD and reconciles desired state
- **Workflow Factory**: Extensible workflow system supporting multiple operations
- **gNOI Integration**: Uses standardized network operations interface for device communication
- **DaemonSet Deployment**: Runs on specific Kubernetes nodes for device proximity

### NetworkDevice CRD Schema
The controller responds to NetworkDevice resources with these key fields:
- `spec.operation` - Type of operation (OSUpgrade, ConfigUpdate, etc.)
- `spec.operationAction` - Specific action (PreloadImage, ApplyConfig, etc.)  
- `spec.osVersion` - Target OS version for upgrades
- `spec.firmwareProfile` - Device-specific firmware profile

### Workflow Extension
To add new workflows:
1. Implement `workflow.Interface` in `pkg/workflow/`
2. Register in `workflow.NewWorkflowFactory()` 
3. Add corresponding CRD operation/action types

## Development Environment

### Prerequisites  
- Docker, minikube, Python 3.7+, Go 1.21+
- Run `make setup` to verify all prerequisites

### Testing Architecture
- **Integration Tests**: Full Kubernetes cluster with minikube profile `sonic-test`
- **Test Environment**: Unified `TestEnvironment` class in `test/lib/environment.py`
- **Manual Testing**: CLI in `test/scripts/dev-env.py` for interactive debugging
- **Log Collection**: Automatic timestamped logs in `test_logs/` after each test

### Development Iteration
1. Code changes → `make dev` (fast validation + build)
2. Integration testing → `make test-integration` or `./test/scripts/dev-env.py` commands
3. Before committing → `make ci` (required - includes all validation and Docker build)

## Important Implementation Details

### gNOI File Transfer
- Implements `file.TransferToRemote` for OS image preloading
- Currently in DRY_RUN mode - actual gRPC calls are simulated
- Path translation handles SONiC-specific file system conventions

### Security Considerations  
- Path validation prevents directory traversal attacks
- Redis connections use proper authentication when configured
- Container security contexts enforce non-root execution

### Error Handling
- Graceful fallback for unsupported workflow types
- Comprehensive logging using structured logging (klog/v2)
- Status propagation back to NetworkDevice CRD for observability

### Dependencies
Key Go modules:
- `k8s.io/client-go` - Kubernetes API client
- `github.com/openconfig/gnoi` - gNOI protocol implementation  
- `github.com/go-redis/redis/v8` - Redis client for CONFIG_DB
- `google.golang.org/grpc` - gRPC framework

## Testing Strategy

Run integration tests to verify end-to-end functionality. The test suite creates a complete environment including:
- minikube cluster with NetworkDevice CRD
- Redis deployment simulating SONiC CONFIG_DB
- sonic-change-agent controller deployment
- Automated workflow triggering and validation

Use `make test-integration PYTEST_ARGS='-v -k test_preload'` for targeted testing.