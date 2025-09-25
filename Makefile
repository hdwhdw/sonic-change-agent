# Makefile for sonic-change-agent

# Variables
BINARY_NAME=sonic-change-agent
IMAGE_NAME=sonic-change-agent
TAG=v0.1.0
DOCKERFILE=Dockerfile

# Build the Go binary
.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-a -installsuffix cgo \
		-ldflags '-w -s' \
		-o $(BINARY_NAME) \
		./cmd/sonic-change-agent

# Build Docker image
.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE_NAME):$(TAG) .

# Load image into minikube
.PHONY: minikube-load
minikube-load: docker-build
	minikube image load $(IMAGE_NAME):$(TAG)

# Deploy to Kubernetes
.PHONY: deploy
deploy:
	kubectl apply -f manifests/daemonset.yaml

# Remove from Kubernetes
.PHONY: undeploy
undeploy:
	kubectl delete -f manifests/daemonset.yaml

# Full deployment pipeline
.PHONY: deploy-test
deploy-test: minikube-load deploy

# View logs
.PHONY: logs
logs:
	kubectl logs -l app=sonic-change-agent -f

# Get pod status
.PHONY: status
status:
	kubectl get pods -l app=sonic-change-agent -o wide

# Clean up
.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	docker rmi $(IMAGE_NAME):$(TAG) 2>/dev/null || true

# Initialize go modules
.PHONY: deps
deps:
	go mod tidy
	go mod download

# Run locally for testing
.PHONY: run
run:
	go run ./cmd/sonic-change-agent --device-name=local-test --interval=5s --v=2

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build        - Build the Go binary"
	@echo "  docker-build - Build Docker image"
	@echo "  minikube-load - Load image into minikube"
	@echo "  deploy       - Deploy DaemonSet to Kubernetes"
	@echo "  undeploy     - Remove DaemonSet from Kubernetes"
	@echo "  deploy-test  - Full deployment pipeline"
	@echo "  logs         - View agent logs"
	@echo "  status       - Get pod status"
	@echo "  clean        - Clean up binaries and images"
	@echo "  deps         - Download Go dependencies"
	@echo "  run          - Run locally for testing"
	@echo "  help         - Show this help"