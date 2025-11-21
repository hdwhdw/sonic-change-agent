"""
Test environment management for sonic-change-agent.

Provides a reusable TestEnvironment class that can be used both by pytest fixtures
and standalone scripts for manual testing/debugging.
"""

import subprocess
import time
import os
import tempfile
import yaml
import json
from datetime import datetime


def run_cmd(cmd, capture=True, cwd=None):
    """Run command and return result."""
    if isinstance(cmd, str):
        cmd = cmd.split()
    
    if capture:
        return subprocess.run(cmd, capture_output=True, text=True, cwd=cwd)
    else:
        return subprocess.run(cmd, cwd=cwd)


def kubectl(*args):
    """Helper to run kubectl commands in test cluster."""
    cmd = ["minikube", "kubectl", "--profile", "sonic-test", "--"] + list(args)
    return run_cmd(cmd)


class TestEnvironment:
    """Manages the complete test environment for sonic-change-agent."""
    
    def __init__(self, cluster_name="sonic-test", image_name="sonic-change-agent:test"):
        self.cluster_name = cluster_name
        self.image_name = image_name
        self.gnoi_image_name = "gnoi-light:test"
        self.created_devices = []
    
    def setup_cluster(self):
        """Create and configure test cluster."""
        print(f"\n🏗️  Setting up test cluster: {self.cluster_name}")
        
        # Clean up existing cluster
        print("Cleaning up any existing cluster...")
        run_cmd(["minikube", "delete", "--profile", self.cluster_name])
        
        # Create cluster
        print("Creating minikube cluster...")
        result = run_cmd([
            "minikube", "start", 
            "--profile", self.cluster_name,
            "--driver=docker",
            "--kubernetes-version=v1.29.0"
        ])
        
        if result.returncode != 0:
            raise Exception(f"Failed to create cluster: {result.stderr}")
        
        # Wait for cluster ready
        print("Waiting for cluster to be ready...")
        for i in range(30):
            result = kubectl("get", "nodes")
            if result.returncode == 0 and "Ready" in result.stdout:
                print("✅ Cluster is ready")
                break
            time.sleep(5)
        else:
            raise Exception("Cluster not ready after 2.5 minutes")
    
    def build_image(self, skip_if_exists=False):
        """Build Docker images for testing."""
        print(f"\n🐳 Building Docker images: {self.image_name}, {self.gnoi_image_name}")
        
        # Get project root: test/lib/environment.py -> project root (2 levels up)
        project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
        
        # Build sonic-change-agent image
        if skip_if_exists:
            result = run_cmd(["docker", "inspect", self.image_name])
            if result.returncode == 0:
                print(f"✅ Using existing Docker image: {self.image_name}")
            else:
                self._build_sonic_agent_image(project_root)
        else:
            self._build_sonic_agent_image(project_root)
        
        # Build gnoi-light image
        if skip_if_exists:
            result = run_cmd(["docker", "inspect", self.gnoi_image_name])
            if result.returncode == 0:
                print(f"✅ Using existing Docker image: {self.gnoi_image_name}")
            else:
                self._build_gnoi_light_image(project_root)
        else:
            self._build_gnoi_light_image(project_root)
        
        print("✅ Docker images built")
    
    def _build_sonic_agent_image(self, project_root):
        """Build sonic-change-agent Docker image."""
        dockerfile_path = os.path.join(project_root, "Dockerfile.sonic-change-agent")
        
        if not os.path.exists(dockerfile_path):
            raise Exception(f"Dockerfile not found at {dockerfile_path}")
        
        result = run_cmd(["docker", "build", "-f", dockerfile_path, "-t", self.image_name, "."], cwd=project_root)
        if result.returncode != 0:
            raise Exception(f"Failed to build sonic-change-agent image: {result.stderr}")
    
    def _build_gnoi_light_image(self, project_root):
        """Build gnoi-light Docker image."""
        dockerfile_path = os.path.join(project_root, "Dockerfile.gnoi-light")
        
        if not os.path.exists(dockerfile_path):
            raise Exception(f"Dockerfile not found at {dockerfile_path}")
        
        result = run_cmd(["docker", "build", "-f", dockerfile_path, "-t", self.gnoi_image_name, "."], cwd=project_root)
        if result.returncode != 0:
            raise Exception(f"Failed to build gnoi-light image: {result.stderr}")
    
    def deploy_redis(self):
        """Deploy Redis with CONFIG_DB configuration."""
        print("\n📦 Deploying Redis...")
        
        # Redis manifest
        redis_manifest = {
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {
                "name": "redis",
                "labels": {"app": "redis"}
            },
            "spec": {
                "replicas": 1,
                "selector": {"matchLabels": {"app": "redis"}},
                "template": {
                    "metadata": {"labels": {"app": "redis"}},
                    "spec": {
                        "hostNetwork": True,
                        "containers": [{
                            "name": "redis",
                            "image": "redis:7-alpine",
                            "ports": [{"containerPort": 6379}],
                            "command": ["redis-server", "--save", "", "--appendonly", "no"]
                        }]
                    }
                }
            }
        }
        
        # Deploy Redis
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            yaml.dump(redis_manifest, f)
            manifest_path = f.name
        
        try:
            result = kubectl("apply", "-f", manifest_path)
            if result.returncode != 0:
                raise Exception(f"Failed to deploy Redis: {result.stderr}")
            
            # Wait for Redis
            print("Waiting for Redis to be ready...")
            for i in range(12):
                result = kubectl("get", "pods", "-l", "app=redis")
                if result.returncode == 0 and "Running" in result.stdout:
                    break
                time.sleep(5)
            else:
                raise Exception("Redis not ready")
            
            # Configure Redis - get node IP
            result = kubectl("get", "nodes", "-o", "jsonpath={.items[0].status.addresses[0].address}")
            if result.returncode != 0:
                raise Exception("Failed to get node IP")
            
            node_ip = result.stdout.strip()
            print(f"Using node IP: {node_ip}")
            
            # Set CONFIG_DB data in database 4
            config_commands = [
                f"redis-cli -n 4 HSET 'KUBERNETES_MASTER|SERVER' ip '{node_ip}' port '8443' insecure 'False' disable 'False'",
                f"redis-cli -n 4 HSET 'GNMI|gnmi' port '8080' client_auth 'false'"
            ]
            
            for cmd in config_commands:
                result = kubectl("exec", "deployment/redis", "--", "sh", "-c", cmd)
                if result.returncode != 0:
                    raise Exception(f"Failed to set Redis config: {cmd}")
            
            print("✅ Redis deployed and configured")
            
        finally:
            os.unlink(manifest_path)
    
    def deploy_agent(self):
        """Deploy sonic-change-agent to cluster."""
        print(f"\n🚀 Deploying sonic-change-agent with images: {self.image_name}, {self.gnoi_image_name}")
        
        # Load images into cluster
        print("Loading Docker images into cluster...")
        result = run_cmd(["minikube", "image", "load", self.image_name, "--profile", self.cluster_name])
        if result.returncode != 0:
            raise Exception(f"Failed to load sonic-change-agent image: {result.stderr}")
        
        result = run_cmd(["minikube", "image", "load", self.gnoi_image_name, "--profile", self.cluster_name])
        if result.returncode != 0:
            raise Exception(f"Failed to load gnoi-light image: {result.stderr}")
        
        # Get project root: test/lib/environment.py -> project root (2 levels up)
        project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
        
        # Deploy CRD
        print("Deploying CRD...")
        crd_path = os.path.join(project_root, "manifests", "crd.yaml")
        result = kubectl("apply", "-f", crd_path)
        if result.returncode != 0:
            raise Exception(f"Failed to deploy CRD: {result.stderr}")
        
        # Wait for CRD
        for i in range(12):
            result = kubectl("get", "crd", "networkdevices.sonic.k8s.io")
            if result.returncode == 0:
                break
            time.sleep(5)
        else:
            raise Exception("CRD not established")
        
        # Deploy RBAC
        print("Deploying RBAC...")
        rbac_path = os.path.join(project_root, "manifests", "rbac.yaml")
        result = kubectl("apply", "-f", rbac_path)
        if result.returncode != 0:
            raise Exception(f"Failed to deploy RBAC: {result.stderr}")
        
        # Deploy DaemonSet with correct image
        print("Deploying DaemonSet...")
        daemonset_path = os.path.join(project_root, "manifests", "daemonset.yaml")
        with open(daemonset_path, "r") as f:
            daemonset_content = f.read()
        
        # Update image names
        updated_content = daemonset_content.replace("sonic-change-agent:latest", self.image_name)
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(updated_content)
            daemonset_temp_path = f.name
        
        try:
            result = kubectl("apply", "-f", daemonset_temp_path)
            if result.returncode != 0:
                raise Exception(f"Failed to deploy DaemonSet: {result.stderr}")
            
            # Wait for pod
            print("Waiting for sonic-change-agent to be ready...")
            for i in range(24):
                result = kubectl("get", "pods", "-l", "app=sonic-change-agent")
                if result.returncode == 0 and "Running" in result.stdout:
                    # Additional check: ensure controller is synced
                    result = kubectl("logs", "daemonset/sonic-change-agent", "--tail=20")
                    if result.returncode == 0 and "Cache synced successfully" in result.stdout:
                        print("✅ sonic-change-agent deployed and ready")
                        break
                time.sleep(5)
            else:
                raise Exception("sonic-change-agent not ready")
            
        finally:
            os.unlink(daemonset_temp_path)
    
    def create_device(self, name, **spec_kwargs):
        """Create a NetworkDevice resource."""
        device_spec = {
            "apiVersion": "sonic.k8s.io/v1",
            "kind": "NetworkDevice", 
            "metadata": {"name": name},
            "spec": {
                "type": "leafRouter",
                "osVersion": "202505.01", 
                "firmwareProfile": "SONiC-Test-Profile",
                "operation": "OSUpgrade",
                "operationAction": "PreloadImage",
                **spec_kwargs
            }
        }
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            yaml.dump(device_spec, f)
            device_path = f.name
        
        try:
            result = kubectl("apply", "-f", device_path)
            if result.returncode != 0:
                raise Exception(f"Failed to create NetworkDevice {name}: {result.stderr}")
            
            self.created_devices.append(name)
            print(f"✅ Created NetworkDevice: {name}")
            return name
            
        finally:
            os.unlink(device_path)
    
    def collect_logs(self, test_name):
        """Collect container logs for debugging."""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        log_dir = os.path.join("test_logs", f"{test_name}_{timestamp}")
        os.makedirs(log_dir, exist_ok=True)
        
        print(f"\n📋 Collecting logs to: {log_dir}")
        
        # Get all pods
        result = kubectl("get", "pods", "-o", "json")
        if result.returncode != 0:
            print(f"Failed to get pods: {result.stderr}")
            return log_dir
        
        try:
            pods_data = json.loads(result.stdout)
            pods = pods_data.get("items", [])
        except json.JSONDecodeError:
            print("Failed to parse pods JSON")
            return log_dir
        
        # Collect logs from each pod
        for pod in pods:
            pod_name = pod["metadata"]["name"]
            pod_namespace = pod["metadata"].get("namespace", "default")
            
            print(f"  📜 Collecting logs from {pod_name}...")
            
            # Get pod logs
            result = kubectl("logs", pod_name, "-n", pod_namespace, "--all-containers=true")
            if result.returncode == 0:
                log_file = os.path.join(log_dir, f"{pod_name}.log")
                with open(log_file, "w") as f:
                    f.write(f"Pod: {pod_name}\n")
                    f.write(f"Namespace: {pod_namespace}\n") 
                    f.write(f"Collected: {datetime.now().isoformat()}\n")
                    f.write("=" * 60 + "\n")
                    f.write(result.stdout)
            else:
                print(f"    ⚠️  Failed to get logs from {pod_name}: {result.stderr}")
        
        print(f"✅ Logs collected in: {log_dir}")
        return log_dir
    
    def cleanup(self):
        """Clean up test environment."""
        print(f"\n🧹 Cleaning up test environment...")
        
        # Delete created devices
        for device_name in self.created_devices:
            kubectl("delete", "networkdevice", device_name, "--ignore-not-found=true")
            print(f"🧹 Deleted NetworkDevice: {device_name}")
        
        # Clean up deployments
        kubectl("delete", "daemonset", "sonic-change-agent", "--ignore-not-found=true")
        kubectl("delete", "deployment", "redis", "--ignore-not-found=true")
        
        # Clean up cluster
        run_cmd(["minikube", "delete", "--profile", self.cluster_name])
        
        # Clean up Docker images
        run_cmd(["docker", "rmi", self.image_name], capture=False)
        
        print("✅ Cleanup completed")
    
    def status(self):
        """Show current environment status."""
        print(f"\n📊 Environment Status (cluster: {self.cluster_name})")
        
        # Cluster status
        result = run_cmd(["minikube", "status", "--profile", self.cluster_name])
        if result.returncode == 0:
            print("✅ Cluster: Running")
        else:
            print("❌ Cluster: Not running")
            return
        
        # Pod status
        result = kubectl("get", "pods", "-o", "wide")
        if result.returncode == 0:
            print("\n🐳 Pods:")
            print(result.stdout)
        
        # NetworkDevice status
        result = kubectl("get", "networkdevices", "-o", "wide")
        if result.returncode == 0 and result.stdout.strip():
            print("\n📡 NetworkDevices:")
            print(result.stdout)
        else:
            print("\n📡 NetworkDevices: None")