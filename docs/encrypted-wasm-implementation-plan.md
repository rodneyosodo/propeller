# Implementation Plan: Encrypted WASM Workload Support for Proplet

## Overview

This plan outlines the implementation of encrypted WASM workload support in Propeller's Proplet component, enabling it to run encrypted WebAssembly workloads within Trusted Execution Environments (TEEs) while maintaining backward compatibility with existing unencrypted workloads.

## Design Decisions

Based on requirements analysis:

1. **TEE Support**: Optional TEE support - works both inside and outside TEEs with appropriate workload types
   - Encrypted workloads inside TEE ✅ (works)
   - Encrypted workloads outside TEE ❌ (errors)
   - Unencrypted workloads inside TEE ✅ (works)
   - Unencrypted workloads outside TEE ✅ (works - current behavior)

2. **WASM Distribution**: Direct OCI registry pull from Proplet
   - Proplet pulls encrypted WASM images directly from OCI-compliant registries
   - Uses same approach as `tee-wasm-runner`

3. **KBS Integration**: Proplet directly contacts KBS
   - Independent attestation and key retrieval
   - No dependency on Manager for key management

4. **Architecture Integration**: New encrypted runtime type
   - Create `TeeWasmRuntime` alongside existing `WasmtimeRuntime` and `HostRuntime`
   - Clean separation of concerns

## Architecture Components

### 1. New Runtime: `TeeWasmRuntime`

**Location**: `proplet/src/runtime/tee_runtime.rs`

**Responsibilities**:

- TEE attestation via attestation-agent
- OCI image pull with encryption support
- KBS client integration for key retrieval
- Decrypt WASM layers inside TEE
- Execute decrypted WASM using Wasmtime

**Key Dependencies** (from tee-wasm-runner):

- `attestation-agent` - TEE attestation
- `image-rs` - OCI image pulling with encryption support
- `kbs_protocol` - KBS client protocol
- `oci-client` - OCI registry client
- `ocicrypt-rs` (via image-rs) - Encryption/decryption

### 2. Configuration Extensions

**Location**: `proplet/src/config.rs`

**New Configuration Fields**:

```rust
pub struct PropletConfig {
    // ... existing fields ...

    // TEE & Encryption Support
    pub tee_enabled: bool,
    pub kbs_uri: Option<String>,
    pub kbs_resource_path: String,
    pub aa_config_path: Option<String>,
    pub layer_store_path: String,
}
```

**Environment Variables**:

- `PROPLET_TEE_ENABLED` - Enable TEE support (default: false) -> We can detect automatically if we are running inside a TEE
- `PROPLET_KBS_URI` - KBS endpoint URL
- `PROPLET_KBS_RESOURCE_PATH` - Default resource path for keys (e.g., "default/key/encryption-key")
- `PROPLET_AA_CONFIG_PATH` - Attestation agent config file path
- `PROPLET_LAYER_STORE_PATH` - Directory to store OCI layers (default: "/tmp/proplet/layers")

### 3. Protocol Extensions

**Location**: `proplet/src/types.rs`

**Modified `StartRequest`**:

```rust
pub struct StartRequest {
    // ... existing fields ...

    // New fields for encrypted workloads
    #[serde(default)]
    pub encrypted: bool,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub oci_reference: String,  // e.g., "docker.io/user/wasm:encrypted"
    #[serde(default)]
    pub kbs_resource_path: Option<String>,  // Override default KBS path
}
```

**Detection Logic**:

- If `encrypted: true` and `oci_reference` provided → Use `TeeWasmRuntime`
- If `image_url` provided (existing) → Use current chunked MQTT approach
- If `file` provided (existing) → Use current base64 approach

No need to use `oci_reference` since we can either get the image from OCI using the `image_url` or from proxy using chunks.

### 4. Service Integration

**Location**: `proplet/src/service.rs`

**Modified `handle_start_command`**:

```rust
async fn handle_start_command(&self, msg: MqttMessage) -> Result<()> {
    let req: StartRequest = msg.decode()?;
    req.validate()?;

    // Determine which runtime to use
    let runtime = if req.encrypted && !req.oci_reference.is_empty() {
        // Verify TEE is available
        if !self.config.tee_enabled {
            return Err(anyhow::anyhow!(
                // Rather detect if we are running inside a TEE rather than using and env variable
                "Encrypted workloads require TEE support. Enable with PROPLET_TEE_ENABLED=true"
            ));
        }
        self.tee_runtime.clone()
    } else {
        self.runtime.clone() // Existing runtime
    };

    // ... rest of execution logic
}
```

### 5. Main Application Setup

**Location**: `proplet/src/main.rs`

**Runtime Initialization**:

```rust
// Existing runtime selection
let runtime: Arc<dyn Runtime> = if let Some(external_runtime) = &config.external_wasm_runtime {
    Arc::new(HostRuntime::new(external_runtime.clone()))
} else {
    Arc::new(WasmtimeRuntime::new()?)
};

// New TEE runtime (optional)
let tee_runtime: Option<Arc<dyn Runtime>> = if config.tee_enabled {
    info!("Initializing TEE runtime support");
    Some(Arc::new(TeeWasmRuntime::new(&config).await?))
} else {
    None
};

let service = Arc::new(PropletService::new(
    config.clone(),
    pubsub,
    runtime,
    tee_runtime
));
```

## Implementation Phases

### Phase 1: Foundation

**Goal**: Set up basic infrastructure

1. **Add Dependencies to Cargo.toml**
   - Add `attestation-agent`, `image-rs`, `kbs_protocol`, `oci-client`
   - Configure features for TEE support (similar to tee-wasm-runner)

2. **Extend Configuration**
   - Add TEE-related config fields
   - Add environment variable parsing
   - Add config validation

3. **Create TeeWasmRuntime Skeleton**
   - Create `proplet/src/runtime/tee_runtime.rs`
   - Implement `Runtime` trait with stub methods
   - Set up attestation agent initialization

**Deliverable**: Compilable code with TEE runtime stub

### Phase 2: OCI Image Pulling

**Goal**: Implement encrypted image pulling

1. **OCI Client Integration**
   - Implement image reference parsing
   - Set up `PullClient` with encryption support
   - Configure layer store for downloaded layers

2. **Encryption Detection**
   - Parse manifest to detect encrypted layers
   - Determine media types (encrypted vs unencrypted)
   - Handle both WASM and standard OCI images

3. **Layer Download**
   - Pull manifest and config
   - Download encrypted/unencrypted layers
   - Store in layer store directory

**Deliverable**: Can pull encrypted WASM images from OCI registries

### Phase 3: TEE Attestation & Decryption

**Goal**: Implement secure decryption

1. **Attestation Agent Setup**
   - Initialize attestation agent with config
   - Generate TEE evidence
   - Support multiple TEE platforms (TDX, SEV-SNP, Sample)

2. **KBS Client Integration**
   - Initialize KBS client with attestation
   - Fetch decryption keys from KBS
   - Handle key format (base64, hex, raw)

3. **Layer Decryption**
   - Integrate with `image-rs` decryption
   - Support JWE encryption format
   - Extract decrypted WASM module

**Deliverable**: Can decrypt encrypted WASM layers using KBS keys

### Phase 4: WASM Execution

**Goal**: Execute decrypted WASM

1. **Wasmtime Integration**
   - Set up Wasmtime engine and linker
   - Configure WASI support
   - Handle function invocation

2. **Result Handling**
   - Capture stdout/stderr
   - Return execution results
   - Handle errors properly

3. **Cleanup**
   - Clean up temporary files
   - Release resources
   - Handle task termination

**Deliverable**: Can execute decrypted WASM workloads

### Phase 5: Service Integration

**Goal**: Integrate with Proplet service

1. **Protocol Extensions**
   - Extend `StartRequest` with encryption fields
   - Add validation for encrypted requests
   - Update request handling logic

2. **Runtime Selection**
   - Implement runtime selection logic
   - Validate TEE availability for encrypted workloads
   - Fall back to standard runtimes for unencrypted

3. **Error Handling**
   - Add TEE-specific error messages
   - Handle KBS connection failures
   - Detect running outside TEE with encrypted workloads

**Deliverable**: Full integration with Proplet service

### Phase 6: Testing & Documentation (Week 5)

**Goal**: Ensure reliability and usability

1. **Unit Tests**
   - Test configuration loading
   - Test protocol parsing
   - Test runtime selection logic

2. **Integration Tests**
   - Test with sample attestation
   - Test encrypted image pulling
   - Test with KBS setup

3. **Documentation**
   - Configuration guide
   - Encryption setup guide
   - Example workflows
   - Troubleshooting guide

**Deliverable**: Production-ready encrypted workload support

## Dependencies & Cargo.toml Changes

Add to `proplet/Cargo.toml`:

```toml
[dependencies]
# Existing dependencies...

# TEE & Encryption Support (optional feature)
attestation-agent = { git = "https://github.com/confidential-containers/guest-components", optional = true, default-features = false, features = ["rust-crypto", "kbs"] }
image-rs = { git = "https://github.com/confidential-containers/guest-components", optional = true, default-features = false, features = ["encryption-ring", "keywrap-grpc", "oci-client-rustls", "signature", "futures"] }
kbs_protocol = { git = "https://github.com/confidential-containers/guest-components", optional = true, features = ["background_check", "rust-crypto"], default-features = false }
oci-client = { version = "0.15", optional = true, default-features = false, features = ["rustls-tls"] }
oci-spec = { version = "0.8", optional = true }
resource_uri = { git = "https://github.com/confidential-containers/guest-components", optional = true }
hex = { version = "0.4", optional = true }
futures-util = { version = "0.3", optional = true }

[features]
default = []
tee = ["attestation-agent", "image-rs", "kbs_protocol", "oci-client", "oci-spec", "resource_uri", "hex", "futures-util"]
tdx-attester = ["tee", "attestation-agent/tdx-attester"]
snp-attester = ["tee", "attestation-agent/snp-attester"]
sgx-attester = ["tee", "attestation-agent/sgx-attester"]
sample-attester = ["tee", "attestation-agent/sample"]
```

## File Structure

```
proplet/
├── src/
│   ├── main.rs                     # [MODIFY] Add TEE runtime initialization
│   ├── config.rs                   # [MODIFY] Add TEE config fields
│   ├── service.rs                  # [MODIFY] Add runtime selection logic
│   ├── types.rs                    # [MODIFY] Extend StartRequest
│   └── runtime/
│       ├── mod.rs                  # [MODIFY] Export TeeWasmRuntime
│       ├── wasmtime_runtime.rs     # [EXISTING]
│       ├── host.rs                 # [EXISTING]
│       └── tee_runtime.rs          # [NEW] TEE WASM runtime implementation
├── Cargo.toml                      # [MODIFY] Add dependencies and features
└── README.md                       # [MODIFY] Add encrypted workload docs
```

## Testing Strategy

### 1. Unit Tests

- Configuration parsing
- Protocol message validation
- Runtime selection logic

### 2. Integration Tests

**Test Case 1: Unencrypted WASM (Baseline)**

- Verify existing functionality still works
- No TEE required

**Test Case 2: Encrypted WASM with Sample Attestation**

- Use sample attester for testing
- Set up local KBS with sample policy
- Pull encrypted image from registry
- Verify decryption and execution

**Test Case 3: Encrypted WASM outside TEE (Error Case)**

- Attempt to run encrypted workload without TEE
- Verify proper error message

**Test Case 4: TEE Detection**

- Run inside actual TEE (if available)
- Verify real attestation works

### 3. End-to-End Workflow Test

1. Build encrypted WASM image
2. Push to registry
3. Set up KBS with encryption key
4. Configure Proplet with TEE support
5. Send start request via MQTT
6. Verify successful execution
7. Check results

## Security Considerations

1. **Key Security**
   - Decryption keys never leave TEE memory
   - Keys fetched only after successful attestation
   - Temporary files cleaned up after execution

2. **Attestation**
   - Real TEE attestation for production
   - Sample attestation only for testing
   - Configurable attestation policies via KBS

3. **Access Control**
   - KBS enforces access policies
   - Resource paths prevent unauthorized access
   - Manager cannot intercept decryption keys

4. **Error Handling**
   - Don't leak sensitive information in errors
   - Fail securely (don't fall back to unencrypted)
   - Log security events properly

## Migration & Backward Compatibility

**Existing Functionality Preserved**:

- All existing WASM execution methods continue to work
- No breaking changes to protocol
- TEE support is opt-in via configuration
- New fields in `StartRequest` are optional with defaults

**Migration Path**:

1. Update Proplet binary with TEE support
2. Keep `PROPLET_TEE_ENABLED=false` for existing deployments
3. Enable TEE support only for workers running in TEEs
4. Gradually migrate workloads to encrypted format

## Open Questions & Future Enhancements

### Open Questions

1. Should we support mixed mode (some workers with TEE, some without)?
   - **Answer**: Yes, via runtime selection based on request type

2. How to handle workload that requests encryption but Proplet doesn't support it?
   - **Answer**: Return clear error message, fail the task

3. Should Manager be aware of which workers support TEE?
   - **Answer**: Yes - Manager should track worker capabilities and schedule accordingly

### Future Enhancements

1. **Worker Capability Reporting**
   - Report TEE availability in discovery/liveliness messages
   - Manager schedules encrypted workloads only to TEE-capable workers

2. **Multiple KBS Support**
   - Support per-workload KBS configuration
   - Different KBS for different security domains

3. **Performance Optimizations**
   - Cache decrypted modules (if security allows)
   - Reuse attestation sessions
   - Parallel layer downloads

4. **Enhanced Monitoring**
   - Track attestation metrics
   - Monitor decryption performance
   - Alert on security events

5. **Additional TEE Platforms**
   - Add ARM TrustZone support
   - Add RISC-V Keystone support
   - Support custom TEE implementations

## Success Criteria

1. ✅ Can run unencrypted WASM (existing functionality)
2. ✅ Can pull encrypted WASM from OCI registry
3. ✅ Can perform TEE attestation (at least with sample attester)
4. ✅ Can fetch decryption keys from KBS
5. ✅ Can decrypt and execute encrypted WASM inside TEE
6. ✅ Errors correctly when attempting encrypted workload outside TEE
7. ✅ Backward compatible with existing Proplet deployments
8. ✅ Well documented with examples
9. ✅ Tested with integration tests
10. ✅ Performance impact < 10% for unencrypted workloads

## References

- **tee-wasm-runner**: `/home/rodneyosodo/code/opensource/guest-components/tee-wasm-runner`
  - Reference implementation for TEE WASM execution
  - Source of attestation-agent and image-rs integration patterns

- **Proplet**: `/home/rodneyosodo/code/absmach/propeller/proplet`
  - Current architecture and runtime abstraction
  - MQTT protocol and service patterns

- **Confidential Containers**: https://github.com/confidential-containers/guest-components
  - Source of attestation-agent, image-rs, kbs_protocol

- **KBS Setup Guide**: `/home/rodneyosodo/code/opensource/guest-components/tee-wasm-runner/KBS_SETUP.md`
  - Reference for KBS configuration and testing
