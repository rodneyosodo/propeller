# 🚀 Propeller

**Propeller** is a cutting-edge orchestrator for **WebAssembly (Wasm)** workloads across the **Cloud-Edge continuum**. It enables seamless deployment of Wasm applications from powerful cloud servers to constrained microcontrollers, combining flexibility, security, and performance.

## 🌟 Features

- 🌐 **Cloud-Edge Orchestration**: Deploy Wasm workloads effortlessly across diverse environments, from robust cloud servers to lightweight microcontrollers.
- ⚡ **Fast Boot Times**: Take advantage of Wasm's near-instant startup for efficient workload execution.
- 📦 **FaaS Deployment**: Enable Function-as-a-Service (FaaS) capabilities for scalable and event-driven applications.
- 🖥️ **OCI Registry Support**: Push and pull Wasm workloads from OCI-compliant registries for streamlined workflow integration.
- 🔧 **WAMR on Zephyr RTOS**: Deploy lightweight Wasm workloads on constrained devices running Zephyr RTOS via the WebAssembly Micro Runtime (WAMR).
- 🛠️ **Powerful Service Mesh**: Integrates with **[SuperMQ](https://github.com/absmach)** for secure, efficient IoT device communication.
- 🔒 **Security at the Core**: Propeller ensures secure workload execution and communication for IoT environments.
- ☸️ **Hybrid Orchestration**: NEW! Manage both Kubernetes-managed and external proplets from a single control plane.

## 🆕 Hybrid Orchestration

Propeller now supports **Hybrid Orchestration**, enabling you to manage workloads across both Kubernetes clusters and external edge devices from a unified control plane.

### Key Benefits:
- **Unified Management**: Single API for both K8s pods and external devices (ESP32, Raspberry Pi, etc.)
- **Intelligent Scheduling**: Smart task placement based on proplet capabilities and resource requirements
- **Automatic Discovery**: External devices auto-register when they come online
- **Health Monitoring**: Automated failure detection and task evacuation
- **Backward Compatibility**: Works with existing MQTT-based proplets

### Architecture:
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
            │ MQTT
    ┌───────▼──────────────────────────────────────────────┐
    │              External Proplets                     │
    │  ┌─────┐    ┌─────┐    ┌─────┐    ┌─────┐         │
    │  │ESP32│    │ RPi │    │Edge │    │ ... │         │
    │  └─────┘    └─────┘    └─────┘    └─────┘         │
    └────────────────────────────────────────────────────┘
```

## 🛠️ How It Works

1. **Develop in WebAssembly**: Write portable, lightweight Wasm workloads for your application.
2. **Register Workloads**: Push your workloads to an OCI-compliant registry for easy deployment.
3. **Deploy Anywhere**: Use Propeller to orchestrate and manage workload deployment across the cloud, edge, and IoT devices.
4. **Monitor & Scale**: Leverage real-time monitoring and dynamic scaling to optimize your system's performance.

![Propeller Orchestration Diagram](docs/architecture.svg)

## 🚀 Quick Start

### Prerequisites
- Kubernetes cluster (v1.20+)
- MQTT broker (Mosquitto, EMQX, etc.)
- kubectl configured

### 1. Install Propeller Operator

```bash
# Install Custom Resource Definitions
kubectl apply -f https://raw.githubusercontent.com/absmach/propeller/main/k8s/manifests/crds.yaml

# Install RBAC resources
kubectl apply -f https://raw.githubusercontent.com/absmach/propeller/main/k8s/manifests/rbac.yaml

# Deploy the operator
kubectl apply -f https://raw.githubusercontent.com/absmach/propeller/main/k8s/manifests/deployment.yaml
```

### 2. Create Your First K8s Proplet

```bash
kubectl apply -f - <<EOF
apiVersion: propeller.absmach.fr/v1alpha1
kind: Proplet
metadata:
  name: my-k8s-workers
spec:
  type: k8s
  k8s:
    image: propeller/proplet:latest
    replicas: 3
  connectionConfig:
    domainId: propeller
    channelId: control
    broker: mqtt://mqtt-broker:1883
EOF
```

### 3. Deploy a Task

```bash
kubectl apply -f - <<EOF
apiVersion: propeller.absmach.fr/v1alpha1
kind: Task
metadata:
  name: hello-wasm
spec:
  name: hello-world
  imageUrl: registry.example.com/hello-wasm:latest
  preferredPropletType: any
EOF
```

### 4. Monitor Execution

```bash
# Check proplet status
kubectl get proplets

# Check task status
kubectl get tasks

# View task details
kubectl describe task hello-wasm
```

## 📋 Examples

### External ESP32 Proplet
```yaml
apiVersion: propeller.absmach.fr/v1alpha1
kind: Proplet
metadata:
  name: esp32-sensor
spec:
  type: external
  external:
    deviceType: esp32
    capabilities: [wasm, sensor, wifi]
  connectionConfig:
    domainId: propeller
    channelId: control
```

### Edge Processing Task
```yaml
apiVersion: propeller.absmach.fr/v1alpha1
kind: Task
metadata:
  name: sensor-processing
spec:
  name: process-sensor-data
  imageUrl: registry.example.com/sensor-processor:v1.0.0
  preferredPropletType: external
  propletSelector:
    matchDeviceTypes: [esp32, raspberry-pi]
    matchCapabilities: [sensor]
```

### ML Inference Task
```yaml
apiVersion: propeller.absmach.fr/v1alpha1
kind: Task
metadata:
  name: ml-inference
spec:
  name: image-classification
  imageUrl: registry.example.com/ml-model:v2.0.0
  preferredPropletType: k8s
  resourceRequirements:
    cpu: "2000m"
    memory: "4Gi"
```

## 📖 Documentation

- **[Hybrid Orchestration Guide](docs/hybrid-orchestration.md)**: Complete setup and usage guide
- **[Architecture Overview](docs/architecture.md)**: System design and components
- **[API Reference](docs/api-reference.md)**: Custom Resource definitions
- **[Examples](k8s/examples/)**: Ready-to-use YAML configurations

## 💡 Use Cases

- 🏭 **Industrial IoT**: Deploy analytics or control applications to edge devices in factories.
- 🛡️ **Secure Workloads**: Run isolated, portable workloads securely on cloud or edge devices.
- 🌎 **Smart Cities**: Power scalable IoT networks with efficient communication and dynamic workloads.
- ☁️ **Serverless Applications**: Deploy FaaS applications leveraging Propeller's Wasm orchestration capabilities.
- 🤖 **Edge AI**: Run ML inference on edge devices with automatic failover to cloud resources.

## 🔧 Configuration

Configure the operator via environment variables:

```yaml
env:
- name: MQTT_BROKER
  value: "mqtt://your-broker:1883"
- name: SCHEDULER_TYPE
  value: "hybrid"
- name: HEALTH_CHECK_INTERVAL
  value: "1m"
- name: OFFLINE_THRESHOLD
  value: "5m"
```

## 🧪 Development

### Building from Source

```bash
# Clone the repository
git clone https://github.com/absmach/propeller.git
cd propeller

# Build the operator
make build

# Build Docker image
make docker-build

# Run tests
make test
```

### Running Locally

```bash
# Install dependencies
go mod tidy

# Run the operator locally
make run
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Areas for Contribution
- 🐛 Bug fixes and improvements
- 📝 Documentation enhancements
- 🔧 New device integrations
- ⚡ Performance optimizations
- 🧪 Test coverage improvements

## 📊 Metrics and Monitoring

Propeller exposes Prometheus metrics:

- `propeller_proplets_total`: Number of registered proplets
- `propeller_tasks_total`: Number of tasks
- `propeller_task_duration_seconds`: Task execution time
- `propeller_proplet_health_status`: Proplet health status

## 🔒 Security

- **Sandboxed Execution**: WebAssembly provides inherent sandboxing
- **mTLS Support**: Secure MQTT communication
- **RBAC Integration**: Kubernetes Role-Based Access Control
- **Network Policies**: Restrict pod-to-pod communication
- **Resource Limits**: Prevent resource exhaustion

## 🔍 Troubleshooting

### Common Issues

1. **Proplets not appearing**: Check MQTT connectivity and credentials
2. **Tasks stuck in Pending**: Verify proplet selector criteria
3. **External proplets offline**: Check device connectivity and MQTT topics

### Debug Commands

```bash
# Check operator logs
kubectl logs -n propeller-system deployment/propeller-operator

# Describe resources
kubectl describe proplet <name>
kubectl describe task <name>

# Monitor MQTT traffic
kubectl run mqtt-debug --image=eclipse-mosquitto:2.0 -it --rm -- mosquitto_sub -h mqtt-broker -t "#"
```

## 📈 Roadmap

- [ ] WebUI Dashboard for monitoring and management
- [ ] Advanced scheduling algorithms (ML-based)
- [ ] Multi-cluster federation
- [ ] Integration with service meshes (Istio, Linkerd)
- [ ] Enhanced security policies
- [ ] Performance optimizations for large-scale deployments

## 📜 License

Propeller is licensed under the **Apache-2.0 License**. See the [LICENSE](LICENSE) file for more details.

## 🙏 Acknowledgments

>🇪🇺 This work has been partially supported by the [ELASTIC project](https://elasticproject.eu/), which received funding from the [Smart Networks and Services Joint Undertaking](https://smart-networks.europa.eu/) (SNS JU) under the European Union's [Horizon Europe](https://research-and-innovation.ec.europa.eu/funding/funding-opportunities/funding-programmes-and-open-calls/horizon-europe_en) research and innovation programme under [Grant Agreement No. 101139067](https://cordis.europa.eu/project/id/101139067). Views and opinions expressed are however those of the author(s) only and do not necessarily reflect those of the European Union. Neither the European Union nor the granting authority can be held responsible for them.

---

**Ready to orchestrate WebAssembly workloads across the cloud-edge continuum?** Get started with Propeller today! 🚀