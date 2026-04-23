# Propeller

[**Propeller**](https://propeller.absmach.eu) is a WebAssembly (Wasm) workload orchestrator for the Cloud-Edge continuum. It enables seamless deployment of Wasm applications from powerful cloud servers to constrained microcontrollers, combining flexibility, security, and performance.

Propeller builds on top of [Magistrala](https://github.com/absmach/magistrala), an open-source IoT platform that provides identity, access control, device provisioning, data processing, and observability. Together, they form a complete solution for deploying and orchestrating Wasm workloads across distributed edge environments.

## Features

- **Cloud-Edge Orchestration**: Deploy Wasm workloads effortlessly across diverse environments, from robust cloud servers to lightweight microcontrollers.
- **Fast Boot Times**: Take advantage of Wasm's near-instant startup for efficient workload execution.
- **FaaS Deployment**: Enable Function-as-a-Service (FaaS) capabilities for scalable and event-driven applications.
- **OCI Registry Support**: Push and pull Wasm workloads from OCI-compliant registries for streamlined workflow integration.
- **WAMR on Zephyr RTOS**: Deploy lightweight Wasm workloads on constrained devices running Zephyr RTOS via the WebAssembly Micro Runtime (WAMR).
- **Powerful Service Mesh**: Integrates with [Magistrala](https://github.com/absmach/magistrala) for secure, efficient IoT device communication.
- **Security at the Core**: Propeller ensures secure workload execution and communication for IoT environments.
- **Federated Learning**: Built-in support for federated machine learning workflows with FedAvg aggregation, enabling privacy-preserving distributed training across edge devices.
- **Job Orchestration**: Group multiple tasks into jobs with configurable execution modes (parallel, sequential, or dependency-based), enabling complex multi-step workflows with fail-fast semantics.

## How It Works

1. **Develop in WebAssembly**: Write portable, lightweight Wasm workloads for your application.
2. **Register Workloads**: Push your workloads to an OCI-compliant registry for easy deployment.
3. **Deploy Anywhere**: Use Propeller to orchestrate and manage workload deployment across the cloud, edge, and IoT devices.
4. **Monitor & Scale**: Leverage real-time monitoring and dynamic scaling to optimize your system's performance.

## Architecture

Propeller consists of several key components:

- **CLI**: Command-line interface for interacting with the Propeller system
- **Manager**: Central service for task management and proplet coordination
- **Proplet**: Worker nodes that execute WebAssembly workloads (implemented in Rust)
- **Proxy**: Service for downloading and distributing Wasm modules from OCI registries

![Propeller Orchestration Diagram](https://propeller.absmach.eu/architecture.svg)

## Documentation

For setup instructions, API references, and usage examples, see the [documentation](https://propeller.absmach.eu/docs).

## Use Cases

- **Industrial IoT**: Deploy analytics or control applications to edge devices in factories.
- **Secure Workloads**: Run isolated, portable workloads securely on cloud or edge devices.
- **Smart Cities**: Power scalable IoT networks with efficient communication and dynamic workloads.
- **Serverless Applications**: Deploy FaaS applications leveraging Propeller's Wasm orchestration capabilities.
- **Federated Machine Learning**: Train machine learning models across distributed edge devices without exposing raw data, perfect for privacy-sensitive applications.
- **ML Inference**: Run machine learning models at the edge using wasi-nn with OpenVINO backend.

## Contributing

Contributions are welcome! Please check the [CONTRIBUTING.md](https://github.com/absmach/.github/blob/main/CONTRIBUTING.md) for details on how to get started.

## License

Propeller is licensed under the **Apache-2.0 License**. See the [LICENSE](LICENSE) file for more details.

> This work has been partially supported by the [ELASTIC project](https://elasticproject.eu/), which received funding from the [Smart Networks and Services Joint Undertaking](https://smart-networks.europa.eu/) (SNS JU) under the European Union’s [Horizon Europe](https://research-and-innovation.ec.europa.eu/funding/funding-opportunities/funding-programmes-and-open-calls/horizon-europe_en) research and innovation programme under [Grant Agreement No. 101139067](https://cordis.europa.eu/project/id/101139067). Views and opinions expressed are however those of the author(s) only and do not necessarily reflect those of the European Union. Neither the European Union nor the granting authority can be held responsible for them.
