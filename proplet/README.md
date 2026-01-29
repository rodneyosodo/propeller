# Proplet

A Rust implementation of a Proplet worker component for Propeller orchestration system.

## Overview

Proplet is a lightweight worker service that executes WebAssembly workloads on edge devices, IoT nodes, or cloud instances. It communicates with a central Manager via MQTT and provides task lifecycle management.

## Features

- **Multiple Runtime Support**:
  - **Wasmtime Runtime**: In-process WebAssembly execution using Wasmtime
  - **Host Runtime**: External WebAssembly runtime integration via subprocess execution
  - **TeeWasmRuntime (TEE)**: Encrypted WASM execution with TEE attestation and KBS integration

- **MQTT Communication**: Pub/sub messaging with central Manager for task orchestration

- **Liveliness Reporting**: Periodic heartbeats (every 10 seconds) to indicate worker status

- **Task Management**: Full lifecycle support (start, stop, remove tasks)

- **Chunk Assembly**: Handles large Wasm binary downloads split into chunks

- **Daemon Mode**: Supports both daemon and non-daemon task execution

- **Environment Variables & CLI Args**: Passes configuration to Wasm functions (via Host runtime)

## Building

```bash
# Build with all features
cargo build --release --features "tee,all-attesters"

# Build with specific TEE support
cargo build --release --features "tee,tdx-attester"

# Build without TEE support
cargo build --release
```

## Configuration

Configure via environment variables:

```bash
# Logging
export PROPLET_LOG_LEVEL=info

# Instance identification
export PROPLET_INSTANCE_ID=$(uuidgen)

# MQTT configuration
export PROPLET_MQTT_ADDRESS=tcp://localhost:1883
export PROPLET_MQTT_TIMEOUT=30
export PROPLET_MQTT_QOS=2

# SuperMQ credentials
export PROPLET_DOMAIN_ID=your-domain-id
export PROPLET_CHANNEL_ID=your-channel-id
export PROPLET_CLIENT_ID=your-client-id
export PROPLET_CLIENT_KEY=your-client-key

# Optional: External WebAssembly runtime
export PROPLET_EXTERNAL_WASM_RUNTIME=/usr/local/bin/wasmtime

# Optional: Kubernetes namespace
export PROPLET_MANAGER_K8S_NAMESPACE=default

# Liveliness interval (seconds)
export PROPLET_LIVELINESS_INTERVAL=10

# Optional: Enable monitoring
export PROPLET_ENABLE_MONITORING=true
```

## Running Encrypted WASM Workloads

To enable encrypted workloads, you must have a KBS instance running and configured with encryption keys. Proplet will automatically detect if it's running inside a Trusted Execution Environment (TEE). You can use this qemu [config](hal/ubuntu/qemu.sh) to run a TEE VM.

The attestation-agent service uses port **50010** and coco_keyprovider uses port **50011**.

### Prerequisites

1. If the Attestation Agent is not running you can manually run it by executing the following:

```bash
sudo attestation-agent --attestation_sock 127.0.0.1:50010
```

- 50010 is the port that attestation-agent exposes to talk to it using gRPC

The attestation-agent will perform TEE attestation.

2. **KBS** must be running and configured with encryption keys. KBS should be running at the host machine.
3. If the keyprovider is not running you can manually run it by executing the following:

```bash
sudo coco_keyprovider --socket 127.0.0.1:50011 --kbs http://10.0.2.2:8082
```

- 50011 is the port that coco_keyprovider exposes to talk to it using gRPC
- http://10.0.2.2:8082 is the KBS endpoint

The coco-keyprovider is a standalone gRPC service that handles both attestation and KBS communication. It:

- Runs as a gRPC service listening on a configurable socket address (e.g., 127.0.0.1:50011)
- Performs attestation using NativeEvidenceProvider to securely retrieve keys from KBS
- Communicates with KBS to fetch and register Key Encryption Keys (KEKs)

### TEE Configuration

Proplet automatically detects if it's running inside a Trusted Execution Environment (TEE) by checking for:

- Intel TDX: `/dev/tdx_guest`, `/sys/firmware/tdx_guest`, cpuinfo flags, kernel messages
- AMD SEV/SNP: `/dev/sev`, EFI variables, cpuinfo flags, kernel messages
- Intel SGX: `/dev/sgx_enclave`, `/dev/sgx/enclave`, cpuinfo flags

**Configuration:**

```bash
# Optional: Attestation agent config path
export PROPLET_AA_CONFIG_PATH=/path/to/aa-config.toml

# Coco keyprovider address
export PROPLET_COCO_KEYPROVIDER_ADDRESS=127.0.0.1:50021

# Optional: Layer store for OCI images (default: /tmp/proplet/layers)
export PROPLET_LAYER_STORE_PATH=/tmp/proplet/layers
```

The AA config file is used to configure the Attestation Agent. It should contain the following:

```toml
[token_configs.kbs]
url = "http://10.0.2.2:8082"
[eventlog_config]
init_pcr = 17
enable_eventlog = false
```

### Task Manifest for Encrypted Workloads

```json
{
  "name": "add",
  "image_url": "docker.io/rodneydav/wasm-addition:encrypted",
  "kbs_resource_path": "default/wasm-keys/my-app",
  "encrypted": true,
  "cli_args": ["--invoke", "add"],
  "inputs": [10, 20]
}
```

**Note**: For encrypted workloads, always use `image_url` and do not provide a `file` field. The `kbs_resource_path` field is required for encrypted workloads and should match the path where the encryption key is stored in KBS. If not provided, an empty string will be used which may cause runtime errors when KBS requires a non-empty path.

### Complete Setup

1. **Start KBS** (Key Broker Service):

```bash
cd /path/to/trustee
docker-compose up -d
```

2. **Upload encryption key to KBS**:

```bash
./target/release/kbs-client \
  --url http://localhost:8080 \
  config \
  --auth-private-key kbs/config/private.key \
  set-resource \
  --resource-file /path/to/encryption-key.pem \
  --path default/key/my-app
```

3. **Encrypt and push WASM image**:

```bash
# Generate encryption keys
openssl rand -base64 32 > encryption.key

# Encrypt WASM image
skopeo copy \
  --encryption-key type:jwe:method:pkcs1:pubkey:./encryption.key \
  docker://docker.io/rodneydav/wasm-addition:latest \
  docker://docker.io/rodneydav/wasm-addition:encrypted

# Push to registry
skopeo copy dir:./output docker://docker.io/rodneydav/wasm-addition:encrypted
```

4. **Build and run Proplet**:

```bash
cd /home/rodneyosodo/code/absmach/propeller/proplet
cargo build --release --features tee

PROPLET_AA_CONFIG_PATH=/tmp/aa-config.toml PROPLET_COCO_KEYPROVIDER_ADDRESS=127.0.0.1:50011 ./target/release/proplet
```

## Running

```bash
# Using Wasmtime runtime (default)
./target/release/proplet

# Using external runtime
export PROPLET_EXTERNAL_WASM_RUNTIME=/path/to/wasmtime
./target/release/proplet
```

## MQTT Topics

Proplet uses the following MQTT topic structure:

```
m/{domainID}/c/{channelID}/control/proplet/alive
    └─ Liveliness heartbeat (every 10 seconds)

m/{domainID}/c/{channelID}/control/proplet/create
    └─ Discovery announcement on startup

m/{domainID}/c/{channelID}/control/manager/start
    └─ Task start commands from Manager

m/{domainID}/c/{channelID}/control/manager/stop
    └─ Task stop commands from Manager

m/{domainID}/c/{channelID}/registry/server
    └─ Incoming Wasm binary chunks from Registry

m/{domainID}/c/{channelID}/registry/proplet
    └─ Requests for Wasm binary chunks to Registry

m/{domainID}/c/{channelID}/control/proplet/results
    └─ Task execution results back to Manager
```

## Dependencies

Key Rust crates used:

- **tokio**: Async runtime
- **rumqttc**: MQTT client library
- **wasmtime**: WebAssembly runtime
- **serde**: Serialization/deserialization
- **anyhow**: Error handling
- **tracing**: Structured logging
- **uuid**: Unique identifiers

**TEE dependencies** (with `--features tee`):

- **attestation-agent**: TEE attestation and KBS integration
- **image-rs**: OCI image pulling with encryption support
- **kbs_protocol**: KBS client protocol
- **oci-client**: OCI registry client

## Performance

Rust provides several performance benefits:

- Zero-cost abstractions
- No garbage collection pauses
- Efficient async I/O with Tokio
- Native performance for compute-intensive tasks

## Future Enhancements

- Full WASI support in Wasmtime runtime
- Metrics and observability improvements
- Additional runtime backends (e.g., Wasmer)
- Enhanced security features
- gRPC support as alternative to MQTT
- Improved resource management and scheduling

## License

Same as the main Propeller project.
