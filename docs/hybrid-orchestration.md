# Hybrid Orchestration

This document describes Propeller's Hybrid Orchestration feature, which enables managing both Kubernetes-managed proplets and external proplets (such as ESP32 devices) from a single control plane.

## Architecture Overview

Hybrid Orchestration extends Propeller's architecture to support:

1. **Kubernetes-managed proplets**: Proplets running as pods within the Kubernetes cluster
2. **External proplets**: Proplets running on external devices (ESP32, Raspberry Pi, edge devices)
3. **Unified management**: Single API and control plane for both types of proplets
4. **Intelligent scheduling**: Task placement based on proplet type, capabilities, and resource requirements

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                      │
│  ┌─────────────────┐    ┌─────────────────────────────────┐ │
│  │  Propeller      │    │         K8s Proplets           │ │
│  │  Operator       │────┤  ┌─────┐ ┌─────┐ ┌─────┐      │ │
│  │                 │    │  │Pod 1│ │Pod 2│ │Pod 3│      │ │
│  └─────────────────┘    │  └─────┘ └─────┘ └─────┘      │ │
│           │              └─────────────────────────────────┘ │
└───────────┼─────────────────────────────────────────────────┘
            │ MQTT
            │
    ┌───────▼──────────────────────────────────────────────┐
    │                MQTT Broker                         │
    └───────┬──────────────────────────────────────────────┘
            │
            │ MQTT
    ┌───────▼──────────────────────────────────────────────┐
    │              External Proplets                     │
    │  ┌─────┐    ┌─────┐    ┌─────┐    ┌─────┐         │
    │  │ESP32│    │ RPi │    │Edge │    │ ... │         │
    │  └─────┘    └─────┘    └─────┘    └─────┘         │
    └────────────────────────────────────────────────────┘
```

## Components

### 1. Propeller Operator

The Kubernetes operator manages both types of proplets:

- **Controllers**: `PropletController` and `TaskController`
- **Custom Resources**: `Proplet` and `Task` CRDs
- **Health Monitoring**: Automated health checks and failure recovery
- **MQTT Integration**: Communication with external proplets

### 2. Hybrid Manager Service

The `HybridOrchestrator` service provides:

- Unified API for both proplet types
- Task creation and management
- Proplet discovery and registration
- Legacy compatibility

### 3. Hybrid Scheduler

Intelligent scheduling that considers:

- Proplet type preferences
- Resource requirements
- Task characteristics (edge vs. compute-intensive)
- Load balancing between K8s and external proplets

## Installation

### Prerequisites

- Kubernetes cluster (v1.20+)
- MQTT broker (e.g., Mosquitto, EMQX)
- Propeller CLI

### 1. Deploy the Operator

```bash
# Apply CRDs
kubectl apply -f k8s/manifests/crds.yaml

# Apply RBAC resources
kubectl apply -f k8s/manifests/rbac.yaml

# Deploy the operator
kubectl apply -f k8s/manifests/deployment.yaml
```

### 2. Configure MQTT Broker

Update the operator deployment with your MQTT broker configuration:

```yaml
env:
- name: MQTT_BROKER
  value: "mqtt://your-mqtt-broker:1883"
- name: MQTT_USERNAME
  value: "your-username"
- name: MQTT_PASSWORD
  valueFrom:
    secretKeyRef:
      name: mqtt-credentials
      key: password
```

### 3. Verify Installation

```bash
# Check operator status
kubectl get pods -n propeller-system

# Check CRDs
kubectl get crds | grep propeller.absmach.fr

# Check operator logs
kubectl logs -n propeller-system deployment/propeller-operator
```

## Configuration

The hybrid orchestrator can be configured via environment variables or a configuration file.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `K8S_NAMESPACE` | `propeller-system` | Kubernetes namespace |
| `MQTT_BROKER` | `localhost:1883` | MQTT broker address |
| `PROPELLER_DOMAIN_ID` | `propeller` | Domain ID for MQTT topics |
| `PROPELLER_CHANNEL_ID` | `control` | Channel ID for MQTT topics |
| `SCHEDULER_TYPE` | `hybrid` | Scheduler type (roundrobin, resource-aware, hybrid) |
| `HEALTH_CHECK_INTERVAL` | `1m` | Health check interval |
| `OFFLINE_THRESHOLD` | `5m` | Time before proplet is considered offline |
| `FAILURE_THRESHOLD` | `15m` | Time before proplet is considered failed |

### Advanced Configuration

For advanced configuration, create a ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: propeller-config
  namespace: propeller-system
data:
  config.yaml: |
    scheduler:
      type: "hybrid"
      hybrid:
        preferK8sForCompute: true
        preferExternalForEdge: true
        computeKeywords: ["ml", "ai", "compute", "batch"]
        edgeKeywords: ["sensor", "iot", "edge", "device"]
        loadBalancingRatio: 0.5
    healthMonitor:
      checkInterval: "1m"
      offlineThreshold: "5m"
      failureThreshold: "15m"
      autoEvacuateTasks: true
```

## Usage

### Managing K8s Proplets

Create a K8s-managed proplet:

```yaml
apiVersion: propeller.absmach.fr/v1alpha1
kind: Proplet
metadata:
  name: k8s-worker-pool
  namespace: default
spec:
  type: k8s
  k8s:
    image: propeller/proplet:latest
    replicas: 3
  connectionConfig:
    domainId: propeller
    channelId: control
    broker: mqtt://mqtt-broker:1883
  resources:
    cpu: "500m"
    memory: "512Mi"
```

### Managing External Proplets

External proplets register themselves automatically when they connect. You can also pre-register them:

```yaml
apiVersion: propeller.absmach.fr/v1alpha1
kind: Proplet
metadata:
  name: esp32-sensor-node
  namespace: default
spec:
  type: external
  external:
    endpoint: "192.168.1.100:8080"
    deviceType: esp32
    capabilities:
      - wasm
      - sensor
      - wifi
  connectionConfig:
    domainId: propeller
    channelId: control
    broker: mqtt://mqtt-broker:1883
```

### Creating Tasks

Create a task that can run on either type of proplet:

```yaml
apiVersion: propeller.absmach.fr/v1alpha1
kind: Task
metadata:
  name: sensor-data-processing
  namespace: default
spec:
  name: sensor-processing
  imageUrl: registry.example.com/sensor-processor:v1.0.0
  preferredPropletType: external  # Prefer external proplets
  propletSelector:
    matchDeviceTypes:
      - esp32
      - raspberry-pi
    matchCapabilities:
      - sensor
  resourceRequirements:
    cpu: "100m"
    memory: "64Mi"
```

For compute-intensive tasks:

```yaml
apiVersion: propeller.absmach.fr/v1alpha1
kind: Task
metadata:
  name: ml-inference
  namespace: default
spec:
  name: ml-model-inference
  imageUrl: registry.example.com/ml-inference:v2.0.0
  preferredPropletType: k8s  # Prefer K8s proplets
  resourceRequirements:
    cpu: "1000m"
    memory: "2Gi"
```

### Monitoring

Check proplet status:

```bash
# List all proplets
kubectl get proplets

# Get detailed proplet information
kubectl describe proplet esp32-sensor-node

# Check proplet conditions
kubectl get proplet esp32-sensor-node -o yaml
```

Check task status:

```bash
# List all tasks
kubectl get tasks

# Check task execution details
kubectl describe task sensor-data-processing

# View task logs (for K8s proplets)
kubectl logs -l propeller.absmach.fr/task=sensor-data-processing
```

## External Proplet Development

### ESP32 Integration

For ESP32 devices, use the embed-proplet implementation:

```c
#include "proplet_client.h"

void app_main() {
    // Initialize WiFi
    wifi_init();
    
    // Connect to MQTT broker
    proplet_config_t config = {
        .broker_uri = "mqtt://mqtt-broker:1883",
        .domain_id = "propeller",
        .channel_id = "control",
        .device_id = "esp32-001"
    };
    
    proplet_client_init(&config);
    proplet_client_start();
}
```

### Custom External Proplets

For other platforms, implement the MQTT protocol:

1. **Discovery**: Publish to `m/{domain}/c/{channel}/messages/control/proplet/create`
2. **Heartbeat**: Publish to `m/{domain}/c/{channel}/messages/control/proplet/alive`
3. **Task Execution**: Subscribe to `m/{domain}/c/{channel}/messages/control/manager/start`
4. **Results**: Publish to `m/{domain}/c/{channel}/messages/control/proplet/results`

Example in Python:

```python
import paho.mqtt.client as mqtt
import json
import time

class PropletClient:
    def __init__(self, broker, domain_id, channel_id, device_id):
        self.broker = broker
        self.domain_id = domain_id
        self.channel_id = channel_id
        self.device_id = device_id
        self.client = mqtt.Client()
        
    def connect(self):
        self.client.connect(self.broker, 1883, 60)
        self.client.on_message = self.on_message
        
        # Subscribe to task start commands
        topic = f"m/{self.domain_id}/c/{self.channel_id}/messages/control/manager/start"
        self.client.subscribe(topic)
        
        # Send discovery message
        self.register()
        
    def register(self):
        topic = f"m/{self.domain_id}/c/{self.channel_id}/messages/control/proplet/create"
        payload = {
            "proplet_id": self.device_id,
            "device_type": "python-worker",
            "capabilities": ["wasm", "python"]
        }
        self.client.publish(topic, json.dumps(payload))
        
    def heartbeat(self):
        topic = f"m/{self.domain_id}/c/{self.channel_id}/messages/control/proplet/alive"
        payload = {
            "proplet_id": self.device_id,
            "status": "alive"
        }
        self.client.publish(topic, json.dumps(payload))

    def on_message(self, client, userdata, msg):
        # Handle task execution
        payload = json.loads(msg.payload.decode())
        # Execute WebAssembly task
        results = self.execute_task(payload)
        self.send_results(payload["id"], results)
```

## Troubleshooting

### Common Issues

1. **External proplets not appearing**
   - Check MQTT broker connectivity
   - Verify domain and channel IDs match
   - Check operator logs for MQTT subscription errors

2. **Tasks not being scheduled**
   - Verify proplet selector criteria
   - Check proplet health status
   - Review scheduler logs

3. **K8s proplets failing to start**
   - Check RBAC permissions
   - Verify container image availability
   - Review pod logs

### Debugging Commands

```bash
# Check operator logs
kubectl logs -n propeller-system deployment/propeller-operator

# Check MQTT connectivity
kubectl run -it --rm debug --image=eclipse-mosquitto:2.0 --restart=Never -- mosquitto_sub -h mqtt-broker -t "m/+/c/+/messages/#"

# Describe resources for detailed status
kubectl describe proplet <proplet-name>
kubectl describe task <task-name>

# Check resource usage
kubectl top pods -n propeller-system
```

### Metrics and Monitoring

The operator exposes Prometheus metrics on port 8080:

- `propeller_proplets_total`: Total number of proplets
- `propeller_tasks_total`: Total number of tasks
- `propeller_task_duration_seconds`: Task execution duration
- `propeller_proplet_health_status`: Proplet health status

## Migration from Legacy Propeller

To migrate from the legacy Propeller architecture:

1. **Install the operator** alongside existing components
2. **Create Proplet resources** for existing external devices
3. **Gradually migrate tasks** to use the new Task CRD
4. **Update client applications** to use the hybrid API
5. **Decommission legacy components** once migration is complete

The hybrid orchestrator maintains backward compatibility with the existing MQTT protocol, enabling gradual migration.

## Security Considerations

1. **MQTT Security**
   - Use TLS/SSL for MQTT connections
   - Implement authentication and authorization
   - Configure topic-level access controls

2. **Kubernetes Security**
   - Apply Pod Security Standards
   - Use Network Policies to restrict communication
   - Enable RBAC with minimal permissions

3. **Container Security**
   - Use distroless or minimal base images
   - Scan images for vulnerabilities
   - Implement resource limits

4. **WebAssembly Security**
   - Validate WebAssembly modules before execution
   - Implement sandboxing for untrusted code
   - Monitor resource usage during execution

## Performance Tuning

### Scaling

- **K8s Proplets**: Scale via replica count in PropletSpec
- **External Proplets**: Add more devices and register them
- **Operator**: Enable leader election for HA deployment

### Resource Management

```yaml
spec:
  k8s:
    resources:
      limits:
        cpu: "1000m"
        memory: "1Gi"
      requests:
        cpu: "500m"
        memory: "512Mi"
```

### Health Check Tuning

Adjust thresholds based on your network conditions:

```yaml
env:
- name: HEALTH_CHECK_INTERVAL
  value: "30s"  # More frequent checks
- name: OFFLINE_THRESHOLD
  value: "2m"   # Faster offline detection
```

## Contributing

To contribute to the hybrid orchestration feature:

1. Fork the repository
2. Create feature branch
3. Add tests for new functionality
4. Update documentation
5. Submit pull request

See [CONTRIBUTING.md](../CONTRIBUTING.md) for detailed guidelines.

## License

This project is licensed under the Apache-2.0 License - see the [LICENSE](../LICENSE) file for details.