# wasi-nn Example (OpenVINO)

This example runs machine-learning inference with [wasi-nn](https://github.com/WebAssembly/wasi-nn) using the proplet wasi-nn image.

For full end-to-end setup and operations guidance, use the Propeller docs:

- https://propeller.absmach.eu/docs
- https://propeller.absmach.eu/docs/proplet

## Prerequisites

- x86_64/amd64 host (OpenVINO requirement)
- Docker
- Rust toolchain with `wasm32-wasip1` target
- `proplet:wasi-nn` image (`make docker_proplet_wasinn`)

## Quick Start

Build the WASM example:

```bash
cargo build --manifest-path examples/wasi-nn/Cargo.toml --target=wasm32-wasip1
```

Download OpenVINO fixtures:

```bash
mkdir -p examples/wasi-nn/fixture
wget -q https://download.01.org/openvinotoolkit/fixtures/mobilenet/mobilenet.xml -O examples/wasi-nn/fixture/model.xml
wget -q https://download.01.org/openvinotoolkit/fixtures/mobilenet/mobilenet.bin -O examples/wasi-nn/fixture/model.bin
wget -q https://download.01.org/openvinotoolkit/fixtures/mobilenet/tensor-1x224x224x3-f32.bgr -O examples/wasi-nn/fixture/tensor.bgr
```

Build the proplet wasi-nn image:

```bash
make docker_proplet_wasinn
```

Run inference directly via wasmtime:

```bash
docker run --rm --entrypoint wasmtime \
  -v "$(pwd)/examples/wasi-nn/target/wasm32-wasip1/debug/wasi-nn-example.wasm:/test/wasi-nn-example.wasm" \
  -v "$(pwd)/examples/wasi-nn/fixture:/home/proplet/fixture" \
  ghcr.io/absmach/propeller/proplet:wasi-nn \
  run -S nn --dir=/home/proplet/fixture::fixture /test/wasi-nn-example.wasm
```

Expected output includes:

- `Loaded graph into wasi-nn with ID: ...`
- `Created wasi-nn execution context with ID: ...`
- `Found results, sorted top 5: ...`

## Notes

- `-S nn` enables the wasi-nn proposal in wasmtime.
- `--dir=/home/proplet/fixture::fixture` maps model files into the guest.
- `_start` is the default WASI entry point when running through task orchestration.

## References

- [wasmtime wasi-nn examples](https://github.com/bytecodealliance/wasmtime/tree/main/crates/wasi-nn/examples)
- [OpenVINO installation](https://docs.openvino.ai/2025/get-started/install-openvino.html)
