# Proplet-rs

A Rust implementation of the Proplet worker component for the Propeller orchestration system.

## Overview

Proplet-rs is a lightweight worker service that executes WebAssembly workloads on edge devices, IoT nodes, or cloud instances. It communicates with a central Manager via MQTT and provides task lifecycle management.

## Features

- **Dual Runtime Support**:
  - **Wasmtime Runtime**: In-process WebAssembly execution using Wasmtime
  - **Host Runtime**: External WebAssembly runtime integration via subprocess execution

- **MQTT Communication**: Pub/sub messaging with the central Manager for task orchestration
- **Liveliness Reporting**: Periodic heartbeats (every 10 seconds) to indicate worker status
- **Task Management**: Full lifecycle support (start, stop, remove tasks)
- **Chunk Assembly**: Handles large Wasm binary downloads split into chunks
- **Daemon Mode**: Supports both daemon and non-daemon task execution
- **Environment Variables & CLI Args**: Passes configuration to Wasm functions (via Host runtime)

## Architecture

The Rust implementation maintains feature parity with the Go version while leveraging Rust's performance and safety benefits:

- **Async/Await**: Built on Tokio for high-performance async I/O
- **Type Safety**: Strong typing throughout with comprehensive error handling
- **Memory Safety**: No garbage collection overhead, zero-cost abstractions
- **Modular Design**: Clean separation of concerns with runtime trait abstraction

## Building

```bash
cargo build --release
```

## Configuration

Configure via environment variables:

```bash
export PROPLET_LOG_LEVEL=info
export PROPLET_INSTANCE_ID="supermq387f8e1ff929"
export PROPLET_MQTT_ADDRESS="messaging.magistrala.absmach.eu"
export PROPLET_MQTT_TIMEOUT=30
export PROPLET_MQTT_QOS=2
export PROPLET_DOMAIN_ID="464e3727-0b19-4cde-bbac-4b66bb4d88ac"
export PROPLET_CHANNEL_ID="ac72e0fc-ab70-4aee-865c-2ae14e867d1a"
export PROPLET_CLIENT_ID="7c2a65a7-8231-4e29-885e-d99abbcb2454"
export PROPLET_CLIENT_KEY="b4dc0c06-f0ad-4d67-9355-5d025cef1750"
export PROPLET_EXTERNAL_WASM_RUNTIME="/usr/bin/wasmtime"
export PROPLET_MANAGER_K8S_NAMESPACE=default
export PROPLET_LIVELINESS_INTERVAL=10


domain_id = "464e3727-0b19-4cde-bbac-4b66bb4d88ac"
client_id = "7c2a65a7-8231-4e29-885e-d99abbcb2454"
client_key = "b4dc0c06-f0ad-4d67-9355-5d025cef1750"
channel_id = "ac72e0fc-ab70-4aee-865c-2ae14e867d1a"

mosquitto_pub -I "Hello" -u 461bf169-5b11-4f0c-9b5a-387f8e1ff929 -P dc61afb0-96a8-4062-aef1-ee3facaafd19 -t m/a1907d9b-dee8-487d-9df5-3e348eae0954/c/e08f4ddd-95e0-485b-b72b-4404d804257f -h messaging.magistrala.absmach.eu -m '[{"n":"hello","bu":"m","u":"m","bt":1765290060849000000,"v":100}]' -p 1883 -d
```

## Running

```bash
# Using Wasmtime runtime (default)
./target/release/proplet-rs

# Using external runtime
export PROPLET_EXTERNAL_WASM_RUNTIME=/path/to/wasmtime
./target/release/proplet-rs
```

## MQTT Topics

Proplet-rs uses the following MQTT topic structure:

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

## Differences from Go Implementation

1. **WASI Support**: The Wasmtime runtime currently uses a simplified setup without full WASI support. CLI args and environment variables are supported via the Host runtime.

2. **Error Handling**: Uses Rust's `Result` type and `anyhow` for error propagation instead of Go's error handling.

3. **Concurrency**: Uses Tokio's async/await instead of Go's goroutines and channels.

4. **Memory Management**: No garbage collection; uses Rust's ownership system for memory safety.

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

## License

Same as the main Propeller project.


mosquitto_pub -I "Hello" -u 461bf169-5b11-4f0c-9b5a-387f8e1ff929 -P dc61afb0-96a8-4062-aef1-ee3facaafd19 -t m/a1907d9b-dee8-487d-9df5-3e348eae0954/c/e08f4ddd-95e0-485b-b72b-4404d804257f -h messaging.magistrala.absmach.eu -m '[{"n":"hello","bu":"m","u":"m","bt":1765290060849000000,"v":100}]' -p 8883 -d --cafile /etc/ssl/certs/ca-certificates.crt

curl -s -S -i --cacert /etc/ssl/certs/ca-certificates.crt -X POST -H "Content-Type: application/senml+json" -H "Authorization: Client dc61afb0-96a8-4062-aef1-ee3facaafd19" https://messaging.magistrala.absmach.eu/api/http/m/a1907d9b-dee8-487d-9df5-3e348eae0954/c/e08f4ddd-95e0-485b-b72b-4404d804257f/ -d '[{"n":"hello","bu":"m","u":"m","bt":1765290060849000000,"v":100}]'

mosquitto_sub -u "461bf169-5b11-4f0c-9b5a-387f8e1ff929" -P "dc61afb0-96a8-4062-aef1-ee3facaafd19" -t m/a1907d9b-dee8-487d-9df5-3e348eae0954/c/e08f4ddd-95e0-485b-b72b-4404d804257f -I supermq -h messaging.magistrala.absmach.eu -p 1883 -d

mosquitto_pub -I "Hello" -u 461bf169-5b11-4f0c-9b5a-387f8e1ff929 -P dc61afb0-96a8-4062-aef1-ee3facaafd19 -t m/a1907d9b-dee8-487d-9df5-3e348eae0954/c/e08f4ddd-95e0-485b-b72b-4404d804257f -h messaging.magistrala.absmach.eu -m '[{"n":"hello","bu":"m","u":"m","bt":1765290060849000000,"v":100}]' -p 1883 -d