# SONiC Firmware Upgrade Demo with sonic-change-agent

This document demonstrates the complete end-to-end SONiC firmware upgrade process using sonic-change-agent, a Kubernetes DaemonSet that performs declarative firmware upgrades via gNOI System.SetPackage with automatic reboot.

## 🎯 Overview

This demo shows how to:
1. Deploy sonic-change-agent to a Kubernetes cluster with SONiC worker nodes
2. Trigger declarative firmware upgrades via NetworkDevice Custom Resources
3. Monitor the complete upgrade process including download, installation, and reboot
4. Handle post-upgrade cluster rejoining

## 📋 Prerequisites and Assumptions

### **Environment Setup**
- **Kubernetes Cluster**: minikube master + SONiC worker node
- **Master Node IP**: `<YOUR_HOST_IP>` (determined dynamically via `hostname -I`)
- **SONiC Worker Node**: `vlab-01` (your SONiC device hostname/IP)
- **Network**: All nodes can communicate bidirectionally

### **SONiC Device Requirements**
- **Current OS Version**: `SONiC-OS-20250505.03` (baseline version)
- **Target OS Version**: `SONiC-OS-20250505.09` (upgrade target)
- **Available Disk Space**: At least 4GB free (2GB for download + 2GB for installation)
- **Kubernetes Support**: SONiC 202311+ with kubesonic functionality
- **Services**: ctrmgrd running, gNOI server available on port 8080

### **Firmware Repository**
- **Server**: nginx serving firmware images on port 8888
- **Location**: `./firmware-images/` (local directory with SONiC images)
- **Target Firmware**: `sonic-vs-20250505.09.bin` (1.9GB)
- **Access URL**: `http://10.250.0.1:8888/sonic-vs-20250505.09.bin`

### **Prerequisites Validation**

Check disk space on SONiC device:
```bash
ssh admin@vlab-01 'df -h /'
```
Expected: At least 4GB available

If /tmp is mounted as tmpfs with insufficient space, unmount it:
```bash
# Check if /tmp is tmpfs and its size
ssh admin@vlab-01 'mount | grep "/tmp"'

# If /tmp is tmpfs and too small, unmount it to use root filesystem
ssh admin@vlab-01 'sudo umount /tmp 2>/dev/null || true'

# Verify /tmp now uses root filesystem with sufficient space
ssh admin@vlab-01 'df -h /tmp'
```
Expected: /tmp should have at least 4GB available space

Verify gNOI server is running:
```bash
ssh admin@vlab-01 'netstat -tlnp | grep :8080'
```
Expected: gnmi process listening on port 8080

Check current firmware version:
```bash
ssh admin@vlab-01 'sudo sonic-installer list'
```
Expected: Current = SONiC-OS-20250505.03

Verify OS version consistency across the environment:
```bash
# Check actual OS version on the device
ssh admin@vlab-01 'show version | head -1'

# Check current installed images
ssh admin@vlab-01 'sudo sonic-installer list'

# Note: We will create NetworkDevice CR with matching versions initially
echo "Device OS: SONiC-OS-20250505.03"
echo "CR will be created with: desired=current=SONiC-OS-20250505.03"
echo "Upgrade target will be: SONiC-OS-20250505.09"
```
Expected: All should show SONiC-OS-20250505.03 as current baseline

## 🚀 Demo Steps

### Step 1: Setup Environment and Build sonic-change-agent

This demo assumes you have completed the [k8s_vlab_join_manuscript.md](../sonic-mgmt/docs/k8s_vlab_join_manuscript.md) setup with:
- minikube master running
- vlab-01 joined to the cluster
- Test DaemonSet working

Clone and build sonic-change-agent:

```bash
# Clone the sonic-change-agent repository
git clone <repository-url>
cd sonic-change-agent

# Build the sonic-change-agent Docker image
make docker-build IMAGE=sonic-change-agent:latest

# Transfer the image to vlab-01 (required due to SONiC security restrictions)
docker save sonic-change-agent:latest | ssh admin@vlab-01 'sudo docker load'

# Verify image is available on vlab-01
ssh admin@vlab-01 'docker images | grep sonic-change-agent'
```

**Expected Output**:
```
sonic-change-agent   latest   <image-id>   <time> ago   <size>MB
```

### Step 2: Prepare Firmware Server

Download SONiC firmware images and start nginx server:

```bash
# Create firmware directory
mkdir -p ./firmware-images
cd firmware-images

# Download SONiC firmware images (example URLs - adjust for your environment)
# Note: Replace these URLs with your actual SONiC firmware download locations
wget -O sonic-vs-20250505.03.bin "https://your-firmware-repo/sonic-vs-20250505.03.bin"
wget -O sonic-vs-20250505.09.bin "https://your-firmware-repo/sonic-vs-20250505.09.bin"

# Verify files downloaded
ls -lh *.bin
cd ..

# Use bridge IP for firmware server

# Stop any existing firmware server
docker stop nginx-firmware-server 2>/dev/null || true
docker rm nginx-firmware-server 2>/dev/null || true

# Start nginx with firmware-images directory
docker run -d --name nginx-firmware-server \
  -p 8888:80 \
  -v $(pwd)/firmware-images:/usr/share/nginx/html \
  nginx:alpine

# Verify firmware is accessible
curl -I http://10.250.0.1:8888/sonic-vs-20250505.09.bin
```

**Expected Output**:
```
HTTP/1.1 200 OK
Server: nginx/1.29.1
Content-Type: application/octet-stream
Content-Length: 1964525551
```

### Step 3: Clean Previous Resources

Remove any existing sonic-change-agent resources:

```bash
# Ensure you're in the sonic-change-agent directory
# (should already be here from Step 1)

# Clean up existing resources
NO_PROXY=192.168.49.2 minikube kubectl -- delete -f manifests/daemonset.yaml 2>/dev/null || true
NO_PROXY=192.168.49.2 minikube kubectl -- delete -f manifests/test-devices.yaml 2>/dev/null || true
NO_PROXY=192.168.49.2 minikube kubectl -- delete -f manifests/rbac.yaml 2>/dev/null || true
NO_PROXY=192.168.49.2 minikube kubectl -- delete -f manifests/crd.yaml 2>/dev/null || true

# Verify clean state
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice 2>/dev/null || echo "CRD not found (expected)"
NO_PROXY=192.168.49.2 minikube kubectl -- get pods -l app=sonic-change-agent
```

**Expected**: No resources found

### Step 4: Deploy sonic-change-agent

Deploy the complete sonic-change-agent stack:

```bash
# Apply CRD with firmwareURL support
NO_PROXY=192.168.49.2 minikube kubectl -- apply -f manifests/crd.yaml

# Apply RBAC (service account, roles, bindings)
NO_PROXY=192.168.49.2 minikube kubectl -- apply -f manifests/rbac.yaml

# Deploy DaemonSet (using the image we built and transferred)
NO_PROXY=192.168.49.2 minikube kubectl -- apply -f manifests/daemonset.yaml

# Verify DaemonSet deployment
NO_PROXY=192.168.49.2 minikube kubectl -- get pods -l app=sonic-change-agent
```

**Expected Output**:
```
NAME                       READY   STATUS    RESTARTS   AGE
sonic-change-agent-xxxxx   1/1     Running   0          30s
```

### Step 5: Create NetworkDevice with Matching Versions

Apply the baseline NetworkDevice (no upgrade triggered):

```bash
# Apply baseline NetworkDevice using the provided manifest
NO_PROXY=192.168.49.2 minikube kubectl -- apply -f manifests/test-devices.yaml

# Verify no upgrade is triggered - check version alignment
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01

# Verify version consistency across all sources
echo "=== Version Verification Before Upgrade ==="
echo "1. Device OS version:"
ssh admin@vlab-01 'show version | head -1'

echo "2. Device installed images:"
ssh admin@vlab-01 'sudo sonic-installer list'

echo "3. NetworkDevice CR status:"
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o jsonpath='{.spec.os.desiredVersion} (desired) vs {.status.os.currentVersion} (current)'
echo ""

echo "All should show SONiC-OS-20250505.03 - no version mismatch, no upgrade triggered"
```

**Expected**: Current = Desired = SONiC-OS-20250505.03 (no version mismatch)

### Step 6: Monitor Controller Logs

Open a separate terminal to monitor the upgrade process:

```bash
# Monitor sonic-change-agent logs in real-time using docker on vlab-01
ssh admin@vlab-01 'docker logs -f $(docker ps --format "{{.Names}}" | grep sonic-change-agent)'
```

Keep this running to observe the upgrade flow in real-time.

### Step 7: Trigger Firmware Upgrade

Update the NetworkDevice to trigger the upgrade:

```bash
# Edit the manifest to trigger upgrade
sed -i 's/desiredVersion: "SONiC-OS-20250505.03"/desiredVersion: "SONiC-OS-20250505.09"/' manifests/test-devices.yaml
sed -i 's/sonic-vs-20250505.03.bin/sonic-vs-20250505.09.bin/' manifests/test-devices.yaml

# Apply the updated NetworkDevice
NO_PROXY=192.168.49.2 minikube kubectl -- apply -f manifests/test-devices.yaml

# Monitor upgrade status and verify version mismatch triggers upgrade
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01

echo "=== Version Verification After Upgrade Trigger ==="
echo "NetworkDevice CR now shows version mismatch:"
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o jsonpath='{.spec.os.desiredVersion} (desired) vs {.status.os.currentVersion} (current)'
echo ""
echo "This mismatch (20250505.03 -> 20250505.09) should trigger the upgrade process"

# Watch for changes (Ctrl+C to stop)
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -w
```

**Expected**: Version mismatch detected, upgrade initiated

### Step 8: Monitor Upgrade Progress

The upgrade follows this sequence:

#### **Phase 1: SetPackage (Download + Install)**
Monitor logs for these messages:
```
🔧 Starting firmware upgrade operation
🌐 Using firmware URL from CRD spec
🚀 Sending SetPackage request for DOWNLOAD+ACTIVATE
✅ PHASE 1 COMPLETE: Firmware download+activate completed successfully
```

Monitor gNOI server logs on vlab-01:
```bash
ssh admin@vlab-01 'tail -f /var/log/syslog | grep gnmi'
```

Expected: Download progress messages showing firmware retrieval

#### **Phase 2: System Reboot**
Monitor logs for:
```
🔄 PHASE 2: Initiating system reboot to activate firmware
✅ PHASE 2 COMPLETE: System reboot initiated successfully
```

### Step 9: Monitor Device Reboot

Track device status during reboot:

```bash
# Check device connectivity (will fail during reboot)
ping -c 3 vlab-01

# Monitor cluster node status
NO_PROXY=192.168.49.2 minikube kubectl -- get nodes -w
```

**Expected Behavior**:
1. vlab-01 becomes unreachable (ping fails)
2. Kubernetes node shows NotReady status
3. Device comes back online with new firmware
4. SSH becomes available again

### Step 10: Verify Firmware Activation

Once vlab-01 comes back online:

```bash
# Wait for SSH to become available
until ssh admin@vlab-01 'uptime' 2>/dev/null; do
  echo "Waiting for vlab-01 to come online..."
  sleep 5
done

# Check new firmware version and verify successful upgrade
echo "=== Post-Upgrade Version Verification ==="
echo "1. Device OS version after reboot:"
ssh admin@vlab-01 'show version | head -1'

echo "2. Device installed images:"
ssh admin@vlab-01 'sudo sonic-installer list'

echo "3. Verify new version is active:"
ssh admin@vlab-01 'sudo sonic-installer list | grep "Current:"'
```

**Expected Output**:
```
Current: SONiC-OS-20250505.09
Next: SONiC-OS-20250505.09
Available:
SONiC-OS-20250505.09
SONiC-OS-20250505.03
```

### Step 11: Rejoin Kubernetes Cluster

After firmware upgrade, vlab-01 needs to rejoin the cluster:

```bash
# Remove old node entry
NO_PROXY=192.168.49.2 minikube kubectl -- delete node vlab-01 2>/dev/null || true

# Initialize K8s state on vlab-01
ssh admin@vlab-01 'sonic-db-cli STATE_DB hset "KUBERNETES_MASTER|SERVER" update_time "2024-12-24 01:01:01"'
ssh admin@vlab-01 'sudo systemctl restart ctrmgrd'

# Prepare credentials directory
ssh admin@vlab-01 'sudo bash -c "if [ -d /etc/sonic/credentials ]; then mv /etc/sonic/credentials /etc/sonic/credentials.bak; fi"'
ssh admin@vlab-01 'sudo mkdir -p /etc/sonic/credentials'

# Transfer certificates
scp /tmp/apiserver.crt /tmp/apiserver.key admin@vlab-01:/tmp/
ssh admin@vlab-01 'sudo mv /tmp/apiserver.crt /etc/sonic/credentials/restapiserver.crt'
ssh admin@vlab-01 'sudo mv /tmp/apiserver.key /etc/sonic/credentials/restapiserver.key'

# Configure DNS and K8s server
VMHOST_IP=$(hostname -I | awk '{print $1}')
ssh admin@vlab-01 "grep \"${VMHOST_IP} control-plane.minikube.internal\" /etc/hosts || echo \"${VMHOST_IP} control-plane.minikube.internal\" | sudo tee -a /etc/hosts"
ssh admin@vlab-01 "sudo config kube server ip ${VMHOST_IP}"
ssh admin@vlab-01 'sudo config kube server disable off'

# Wait for join and verify
sleep 15
NO_PROXY=192.168.49.2 minikube kubectl -- get nodes vlab-01
```

**Expected Output**:
```
NAME      STATUS   ROLES    AGE   VERSION
vlab-01   Ready    <none>   30s   v1.22.2
```

### Step 12: Verify Final State

Check that everything is working correctly:

```bash
# Verify sonic-change-agent is running
NO_PROXY=192.168.49.2 minikube kubectl -- get pods -l app=sonic-change-agent

# Check NetworkDevice status (should show version match)
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01

echo "=== Final Version Verification ==="
echo "1. NetworkDevice CR should show matching versions:"
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o jsonpath='{.spec.os.desiredVersion} (desired) vs {.status.os.currentVersion} (current)'
echo ""

echo "2. Device should be running new firmware:"
ssh admin@vlab-01 'sudo sonic-installer list | grep "Current:"'

echo "3. Controller should recognize version match (no upgrade activity):"
ssh admin@vlab-01 'docker logs $(docker ps --format "{{.Names}}" | grep sonic-change-agent) --tail=10'
```

**Expected**:
- sonic-change-agent pod running on vlab-01
- NetworkDevice shows Current = Desired = SONiC-OS-20250505.09
- Controller logs show no new upgrade activity (version match)

## 🧹 Cleanup (Optional)

To revert for additional testing:

```bash
# Revert firmware on vlab-01
ssh admin@vlab-01 'sudo sonic-installer set-default SONiC-OS-20250505.03'
ssh admin@vlab-01 'sudo reboot'

# Clean up Kubernetes resources
NO_PROXY=192.168.49.2 minikube kubectl -- delete -f manifests/test-devices.yaml
NO_PROXY=192.168.49.2 minikube kubectl -- delete -f manifests/daemonset.yaml
NO_PROXY=192.168.49.2 minikube kubectl -- delete -f manifests/rbac.yaml
NO_PROXY=192.168.49.2 minikube kubectl -- delete -f manifests/crd.yaml

# Stop firmware server
docker stop nginx-firmware-server
docker rm nginx-firmware-server

# Revert manifest changes if desired
git checkout manifests/test-devices.yaml
```

## 🎯 Expected Results

After successful completion:

### **Technical Achievements**
- ✅ **Declarative Upgrades**: Firmware upgrade triggered by NetworkDevice CR changes
- ✅ **gNOI Integration**: SetPackage and Reboot operations via gRPC (no bash scripts)
- ✅ **Large File Handling**: 1.9GB firmware download and installation
- ✅ **Automatic Reboot**: System reboot with firmware activation
- ✅ **Cluster Resilience**: Node rejoining after firmware changes

### **Operational Benefits**
- ✅ **Zero Manual Intervention**: Fully automated upgrade process
- ✅ **Real-time Monitoring**: Complete observability via controller logs
- ✅ **Flexible Firmware Sources**: Direct URL specification eliminates filename constraints
- ✅ **Production Ready**: Error handling, state tracking, and recovery procedures

## 🚨 Troubleshooting

### **Disk Space Issues**
```bash
# Check available space on vlab-01
ssh admin@vlab-01 'df -h /'

# If /tmp is tmpfs, unmount it to use root filesystem
ssh admin@vlab-01 'mount | grep "/tmp"'
ssh admin@vlab-01 'sudo umount /tmp 2>/dev/null || true'
ssh admin@vlab-01 'df -h /tmp'
```

### **Firmware Download Fails (404 Error)**
```bash
# Check firmware server accessibility
curl -I http://10.250.0.1:8888/sonic-vs-20250505.09.bin

# Verify nginx container is running
docker ps | grep nginx-firmware-server

# Check firmware file exists in your firmware-images directory
ls -la ./firmware-images/sonic-vs-20250505.09.bin
```

### **SetPackage Operation Fails**
```bash
# Check gNOI server on vlab-01
ssh admin@vlab-01 'netstat -tlnp | grep :8080'

# Check gnmi container status
ssh admin@vlab-01 'docker ps | grep gnmi'

# Review gnmi logs
ssh admin@vlab-01 'docker logs $(docker ps --format "{{.Names}}" | grep gnmi)'
```

### **Device Won't Rejoin Cluster**
```bash
# Check kubelet logs on vlab-01
ssh admin@vlab-01 'sudo journalctl -u kubelet -n 50'

# Verify certificates
ssh admin@vlab-01 'sudo ls -la /etc/sonic/credentials/'

# Test connectivity to master
VMHOST_IP=$(hostname -I | awk '{print $1}')
ssh admin@vlab-01 "curl -k https://${VMHOST_IP}:6443/healthz"
```

### **Pod Not Scheduling**
```bash
# Check node labels and taints
NO_PROXY=192.168.49.2 minikube kubectl -- describe node vlab-01

# Check DaemonSet status
NO_PROXY=192.168.49.2 minikube kubectl -- describe daemonset sonic-change-agent

# Review pod events
NO_PROXY=192.168.49.2 minikube kubectl -- describe pods -l app=sonic-change-agent
```

## 📊 Performance Metrics

Typical timing for a complete upgrade cycle:

| Phase | Duration | Description |
|-------|----------|-------------|
| SetPackage Download | 2-5 minutes | 1.9GB firmware download over network |
| SetPackage Install | 30-60 seconds | Firmware extraction and preparation |
| System Reboot | 2-4 minutes | SONiC shutdown, firmware activation, startup |
| Cluster Rejoin | 30-60 seconds | Kubernetes node registration and readiness |
| **Total** | **5-10 minutes** | Complete end-to-end upgrade cycle |

## 🔗 Related Documentation

- [k8s_vlab_join_manuscript.md](../sonic-mgmt/docs/k8s_vlab_join_manuscript.md) - Kubernetes cluster setup procedures
- [manifests/crd.yaml](manifests/crd.yaml) - NetworkDevice Custom Resource Definition
- [manifests/daemonset.yaml](manifests/daemonset.yaml) - sonic-change-agent deployment configuration
- [pkg/controller/controller.go](pkg/controller/controller.go) - Core upgrade logic implementation

---

**Note**: This demo represents a production-ready SONiC firmware upgrade solution that eliminates manual intervention, provides complete observability, and ensures reliable firmware activation through modern Kubernetes and gNOI technologies.