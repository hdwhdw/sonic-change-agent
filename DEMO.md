# SONiC Change Agent Demo

This document provides step-by-step instructions to demonstrate the Kubernetes-native SONiC firmware management capabilities using the sonic-change-agent.

## Prerequisites

- Access to a SONiC device at `admin@vlab-01`
- Kubernetes cluster with minikube master and vlab-01 as worker node
- Docker installed on both master and worker nodes

## Setup Overview

We have a Kubernetes cluster where:
- **Minikube master** (`192.168.49.2`): Runs the Kubernetes control plane
- **vlab-01 SONiC device**: Joined as a worker node, runs the sonic-change-agent

The sonic-change-agent watches for NetworkDevice CRD changes and simulates firmware preload operations.

## Demo Steps

### Step 1: Verify Cluster Status

First, verify that the Kubernetes cluster is running and vlab-01 is connected:

```bash
# Check cluster nodes
NO_PROXY=192.168.49.2 minikube kubectl -- get nodes

# Expected output:
# NAME       STATUS   ROLES           AGE   VERSION
# minikube   Ready    control-plane   1h    v1.27.4
# vlab-01    Ready    <none>          1h    v1.27.4
```

### Step 2: Verify NetworkDevice CRD is Installed

Check that our custom NetworkDevice CRD is properly installed:

```bash
# List custom resource definitions
NO_PROXY=192.168.49.2 minikube kubectl -- get crd networkdevices.sonic.io

# Expected output:
# NAME                        CREATED AT
# networkdevices.sonic.io     2025-09-25T18:XX:XXZ
```

### Step 3: Check Current NetworkDevice Resources

View the existing NetworkDevice resources:

```bash
# List all NetworkDevice resources
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevices

# Expected output:
# NAME                   AGE
# vlab-01               10m
# vlab-01-preload-test  10m

# Get detailed view of vlab-01 device
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o yaml
```

### Step 4: Verify sonic-change-agent is Running

Check that the sonic-change-agent DaemonSet is deployed and running on vlab-01:

```bash
# Check DaemonSet status
NO_PROXY=192.168.49.2 minikube kubectl -- get daemonset sonic-change-agent

# Expected output:
# NAME                 DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
# sonic-change-agent   1         1         1       1            1           <none>          15m

# Check pod status
NO_PROXY=192.168.49.2 minikube kubectl -- get pods -l app=sonic-change-agent -o wide

# Expected output shows pod running on vlab-01:
# NAME                       READY   STATUS    RESTARTS   AGE   IP                 NODE
# sonic-change-agent-xxxxx   1/1     Running   0          10m   fec0::ffff:afa:1   vlab-01
```

### Step 5: View Agent Logs - Initial State

Check the sonic-change-agent logs to see it detected the existing NetworkDevice:

```bash
# Get current pod name
POD_NAME=$(NO_PROXY=192.168.49.2 minikube kubectl -- get pods -l app=sonic-change-agent -o jsonpath='{.items[0].metadata.name}')

# View logs directly from vlab-01 (due to network setup)
ssh admin@vlab-01 "sudo docker logs \$(sudo docker ps --filter 'name=k8s_sonic-change-agent' --format '{{.ID}}') --tail=10"
```

You should see logs showing:
- Agent startup
- CRD controller initialization
- NetworkDevice ADDED event detection
- Preload operation detection (if configured)

### Step 6: Demonstrate UPDATE Event Detection

Now we'll modify the NetworkDevice to trigger an UPDATE event that the agent will detect:

```bash
# First, check the current requestId
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o jsonpath='{.spec.preload.requestId}'

# Edit the NetworkDevice to change the requestId
NO_PROXY=192.168.49.2 minikube kubectl -- patch networkdevice vlab-01 --type='merge' -p='{"spec":{"preload":{"requestId":"demo-request-'.$(date +%s)'"}}}'
```

### Step 7: Observe Agent Response to Changes

Immediately check the logs to see the agent detect and respond to the change:

```bash
# View latest logs
ssh admin@vlab-01 "sudo docker logs \$(sudo docker ps --filter 'name=k8s_sonic-change-agent' --format '{{.ID}}') --tail=15"
```

You should see:
- `🟡 NetworkDevice UPDATED` - Shows the agent detected the change
- `🚀 New preload operation requested` - Shows it identified the new requestId
- `🔧 Starting firmware preload operation` - Shows it's starting real preload
- `📦 Sending SetPackage request` - Shows actual gNOI System.SetPackage call
- `✅ SetPackage completed successfully` - Shows successful firmware download (2GB)
- `✅ Firmware preload completed successfully` - Shows full operation success

### Step 8: Add a Complete Preload Operation

Create a new NetworkDevice with full preload specification:

```bash
# Create a new NetworkDevice with preload operation
cat <<EOF | NO_PROXY=192.168.49.2 minikube kubectl -- apply -f -
apiVersion: sonic.io/v1
kind: NetworkDevice
metadata:
  name: demo-device
  namespace: default
spec:
  os:
    osType: SONiC
    desiredVersion: "202511.01"
  preload:
    targetVersion: "202511.01"
    imageURL: "https://firmware.azure.com/sonic/202511.01/sonic.bin"
    checksum:
      md5: "demo123456789abcdef"
    requestId: "demo-$(date +%s)"
    mode: Manual
status:
  state: Healthy
  os:
    currentVersion: "202505.39"
EOF
```

### Step 9: Verify Agent Detects New Device

Check that the agent detected the new NetworkDevice (note: it will only process devices matching its own name, so this demonstrates the field selector filtering):

```bash
# List all NetworkDevices
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevices

# The agent will only process the "vlab-01" device, not "demo-device"
# This demonstrates the field selector filtering functionality
```

### Step 10: Demonstrate Agent Filtering

Show that the agent only watches its own device by checking it doesn't react to the demo-device:

```bash
# View logs - should show no reaction to demo-device
ssh admin@vlab-01 "sudo docker logs \$(sudo docker ps --filter 'name=k8s_sonic-change-agent' --format '{{.ID}}') --tail=5"

# Update demo-device and confirm no reaction
NO_PROXY=192.168.49.2 minikube kubectl -- patch networkdevice demo-device --type='merge' -p='{"spec":{"preload":{"requestId":"ignored-'.$(date +%s)'"}}}'

# Verify no new logs (agent ignores devices other than vlab-01)
ssh admin@vlab-01 "sudo docker logs \$(sudo docker ps --filter 'name=k8s_sonic-change-agent' --format '{{.ID}}') --tail=3"
```

### Step 8: Verify Real Firmware Download

Check that the firmware was actually downloaded to the SONiC device:

```bash
# Check disk usage and firmware file on device
ssh admin@vlab-01 "df -h /tmp && ls -lah /tmp/sonic-firmware.bin"

# Expected output:
# Filesystem      Size  Used Avail Use% Mounted on
# tmpfs           4.0G  1.9G  2.2G  47% /tmp
# -rw------- 1 root root 1.9G Sep 25 19:05 /tmp/sonic-firmware.bin
```

This shows the 2GB SONiC firmware image was successfully downloaded via gNOI System.SetPackage.

### Step 9: Check CRD Status After Preload

Verify the NetworkDevice CRD status was updated to reflect the successful preload:

```bash
# Check preload status in CRD
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o jsonpath='{.status.preload}' && echo

# Expected output:
# {"message":"Firmware successfully preloaded","phase":"Succeeded","progress":100}
```

### Step 10: Demonstrate OS Version Sync from Device

Now let's demonstrate the bidirectional communication - the agent querying the actual SONiC device and updating the CRD status:

```bash
# Check current CRD status (may show old static value)
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o jsonpath='{.status.os.currentVersion}' && echo

# The agent queries the device every minute, but let's see the real-time logs
ssh admin@vlab-01 "sudo docker logs \$(sudo docker ps --filter 'name=k8s_sonic-change-agent' --format '{{.ID}}') --tail=10"
```

You should see logs showing:
- `🔍 Syncing OS version from device` - Agent starting sync
- `📱 Retrieved OS version from device` - Successful gNOI OS.Verify call (shows "SONiC-OS-20250510.21")
- `✅ Updated NetworkDevice CRD status` - CRD status updated with live data

```bash
# Verify the CRD now shows the REAL OS version from the device
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o jsonpath='{.status.os.currentVersion}' && echo

# Compare with what the device actually reports via gRPC
grpcurl -plaintext -d '{}' vlab-01:8080 gnoi.os.OS/Verify
```

### Step 11: Demonstrate Continuous Sync

The agent syncs every minute. To see this in action:

```bash
# Watch the agent logs for periodic sync (runs every minute)
ssh admin@vlab-01 "sudo docker logs \$(sudo docker ps --filter 'name=k8s_sonic-change-agent' --format '{{.ID}}') -f"

# In another terminal, watch the CRD status for any changes
NO_PROXY=192.168.49.2 minikube kubectl -- get networkdevice vlab-01 -o jsonpath='{.status.os.currentVersion}' -w
```

### Step 12: Clean Up Demo Resources

Remove the demo NetworkDevice:

```bash
NO_PROXY=192.168.49.2 minikube kubectl -- delete networkdevice demo-device
```

## Key Concepts Demonstrated

### 1. **Kubernetes Custom Resource Definitions (CRDs)**
- NetworkDevice CRD defines the schema for SONiC device configuration
- Validates fields like OS versions, preload operations, checksums
- Stored in etcd like any Kubernetes resource

### 2. **Controller Pattern**
- sonic-change-agent implements the Kubernetes controller pattern
- Uses informers to efficiently watch for resource changes
- Responds to Add/Update/Delete events

### 3. **Field Selectors**
- Agent only watches NetworkDevice resources matching its device name
- Prevents agents from processing other devices' configurations
- Efficient filtering at the API server level

### 4. **Declarative Configuration**
- Desired state specified in NetworkDevice spec
- Agent detects differences and takes action
- Status field tracks current state and operation progress

### 5. **Bidirectional Communication**
- **Kubernetes → Device**: CRD changes trigger agent actions (preload operations)
- **Device → Kubernetes**: Agent queries device state and updates CRD status
- Real device data synchronized to Kubernetes via gNOI OS.Verify calls
- Maintains current OS version in CRD status automatically

### 6. **gNOI Integration**
- Uses OpenConfig gNOI (gRPC Network Operations Interface)
- **OS Version Sync**: Calls `gnoi.os.OS/Verify` to get current OS version from device
- **Firmware Preload**: Calls `gnoi.system.System/SetPackage` with `activate=false` for staging firmware
- **HTTP Download**: Uses RemoteDownload protocol to stream 2GB firmware images directly to device
- **Real Operations**: Downloads actual SONiC firmware to `/tmp/sonic-firmware.bin` (not simulation)
- Plaintext gRPC connection to `:8080` (secure options available)
- Demonstrates modern network device APIs vs legacy SSH

### 7. **Event-Driven Architecture**
- Changes to NetworkDevice resources trigger immediate agent response
- No polling required - real-time responsiveness
- Scalable to many devices

## Expected Demo Flow

1. **Setup Verification** (2 min): Confirm cluster and agent are running
2. **Initial State** (1 min): Show agent detected existing configuration
3. **Change Detection** (3 min): Modify NetworkDevice, observe agent response
4. **Real Firmware Preload** (5 min): Watch 2GB firmware download via gNOI System.SetPackage
5. **CRD Status Verification** (2 min): Show successful preload status in Kubernetes
6. **OS Version Sync Demo** (3 min): Show bidirectional communication with device
7. **Continuous Sync** (2 min): Demonstrate periodic device state synchronization
8. **Filtering Demo** (2 min): Show agent only processes its own device
9. **Architecture Benefits** (3 min): Explain the Kubernetes-native approach

## Troubleshooting

### Agent Not Running
```bash
# Check DaemonSet events
NO_PROXY=192.168.49.2 minikube kubectl -- describe daemonset sonic-change-agent

# Check pod events
NO_PROXY=192.168.49.2 minikube kubectl -- describe pod -l app=sonic-change-agent
```

### Permission Issues
```bash
# Verify cluster role binding exists
NO_PROXY=192.168.49.2 minikube kubectl -- get clusterrolebinding default-admin
```

### Connectivity Issues
```bash
# Test from vlab-01 to API server
ssh admin@vlab-01 "curl -k https://10.52.0.72:6443 --connect-timeout 5"
```

## Architecture Benefits Highlighted

- **Kubernetes-Native**: Uses standard Kubernetes APIs and patterns
- **Bidirectional Communication**: Both declarative management AND device state synchronization
- **Modern APIs**: gRPC/gNOI instead of legacy SSH and screen scraping
- **Scalable**: Controller pattern scales to hundreds of devices
- **Reliable**: Built-in retry logic and eventual consistency
- **Observable**: Standard Kubernetes logging and monitoring with structured data
- **Declarative**: Infrastructure-as-code approach to device management
- **Event-Driven**: Real-time response to configuration changes
- **Live Status**: CRD status reflects actual device state, not just desired state

This demo shows the foundation for replacing legacy SSH-based firmware management with a modern, Kubernetes-native approach using CRDs, controllers, and gNOI APIs. The **real firmware preload operations** prove that:

1. **Kubernetes → Device**: CRD changes trigger actual gNOI System.SetPackage calls that download 2GB firmware images
2. **Device → Kubernetes**: Agent queries device state via gNOI OS.Verify and updates CRD status with live data
3. **Production-Ready**: Handles real SONiC firmware files, proper error handling, and status tracking

The bidirectional communication demonstrates that Kubernetes can serve as both a configuration source AND a device state repository for real network device management.