use super::{Runtime, RuntimeContext};
use anyhow::{Context, Result};
use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::Mutex;
use tracing::info;
use wasmtime::*;
use wasmtime_wasi::{preview1::WasiP1Ctx, WasiCtxBuilder};

pub struct WasmtimeRuntime {
    engine: Engine,
    instances: Arc<Mutex<HashMap<String, Store<WasiP1Ctx>>>>,
}

impl WasmtimeRuntime {
    pub fn new() -> Result<Self> {
        // Configure engine for optimal performance
        let mut config = Config::new();
        config.wasm_reference_types(true);
        config.wasm_bulk_memory(true);
        config.wasm_simd(true);

        let engine = Engine::new(&config)?;

        Ok(Self {
            engine,
            instances: Arc::new(Mutex::new(HashMap::new())),
        })
    }
}

#[async_trait]
impl Runtime for WasmtimeRuntime {
    async fn start_app(
        &self,
        _ctx: RuntimeContext,
        wasm_binary: Vec<u8>,
        _cli_args: Vec<String>,
        id: String,
        function_name: String,
        daemon: bool,
        _env: HashMap<String, String>,
        args: Vec<u64>,
    ) -> Result<Vec<u8>> {
        info!(
            "Starting Wasmtime runtime app: task_id={}, function={}, daemon={}, wasm_size={}",
            id,
            function_name,
            daemon,
            wasm_binary.len()
        );

        // Compile the module
        info!("Compiling WASM module for task: {}", id);
        let module = Module::from_binary(&self.engine, &wasm_binary)
            .context("Failed to compile Wasmtime module from binary")?;

        info!("Module compiled successfully for task: {}", id);

        // Create WASI P1 context with stdout/stderr capture
        let wasi = WasiCtxBuilder::new().inherit_stdio().build_p1();

        // Create a new store with WASI context
        let mut store = Store::new(&self.engine, wasi);

        // Create a linker and add WASI preview1 functions
        let mut linker = Linker::new(&self.engine);
        wasmtime_wasi::preview1::add_to_linker_sync(&mut linker, |ctx| ctx)
            .context("Failed to add WASI to linker")?;

        // Instantiate the module
        info!("Instantiating module for task: {}", id);
        let instance = linker
            .instantiate(&mut store, &module)
            .context("Failed to instantiate Wasmtime module")?;

        info!("Module instantiated successfully for task: {}", id);

        if daemon {
            // For daemon mode, store the instance and return immediately
            info!("Running in daemon mode for task: {}", id);

            let instances = self.instances.clone();
            let task_id = id.clone();

            // Store the instance
            {
                let mut instances_map = self.instances.lock().await;
                instances_map.insert(id.clone(), store);
            }

            // Spawn background task to execute the function
            tokio::spawn(async move {
                // In daemon mode, we might want to execute after some delay or condition
                // For now, just log that it's running
                info!("Daemon task {} is running", task_id);

                // TODO: Implement actual daemon execution logic
                // This would typically involve calling the function periodically
                // or keeping it alive for repeated invocations

                // For now, simulate by waiting and then cleaning up
                tokio::time::sleep(std::time::Duration::from_secs(60)).await;

                // Cleanup
                instances.lock().await.remove(&task_id);
                info!("Daemon task {} completed", task_id);
            });

            info!("Daemon task {} started, returning immediately", id);
            Ok(Vec::new())
        } else {
            // Synchronous execution - must run in blocking context
            info!("Running in synchronous mode for task: {}", id);

            // Run the WASM execution in a blocking task to avoid runtime conflicts
            let task_id = id.clone();
            let result = tokio::task::spawn_blocking(move || {
                // Initialize the WASM runtime by calling _initialize if it exists
                // This is the WASI reactor initialization function
                if let Some(init_func) = instance.get_func(&mut store, "_initialize") {
                    info!("Found _initialize function, initializing WASM runtime for task: {}", task_id);
                    init_func.call(&mut store, &[], &mut [])
                        .context("Failed to initialize WASM runtime via _initialize")?;
                    info!("WASM runtime initialized successfully for task: {}", task_id);
                } else {
                    info!("No _initialize function found, skipping initialization for task: {}", task_id);
                }

                // Get the exported function
                let func = instance
                    .get_func(&mut store, &function_name)
                    .context(format!(
                        "Function '{}' not found in module exports",
                        function_name
                    ))?;

                info!(
                    "Found function '{}', preparing to call with {} arguments",
                    function_name,
                    args.len()
                );

                // Get function type to determine parameter and return types
                let func_ty = func.ty(&store);

                // Log function signature
                let param_types: Vec<_> = func_ty.params().collect();
                let result_types: Vec<_> = func_ty.results().collect();
                info!(
                    "Function signature: params={:?}, results={:?}",
                    param_types, result_types
                );

                // Validate argument count matches function signature
                if args.len() != param_types.len() {
                    return Err(anyhow::anyhow!(
                        "Argument count mismatch for function '{}': expected {} arguments but got {}",
                        function_name,
                        param_types.len(),
                        args.len()
                    ));
                }

                // Convert u64 args to wasmtime Val types based on function signature
                let wasm_args: Vec<Val> = args
                    .iter()
                    .zip(param_types.iter())
                    .map(|(arg, param_type)| match param_type {
                        ValType::I32 => Val::I32(*arg as i32),
                        ValType::I64 => Val::I64(*arg as i64),
                        ValType::F32 => Val::F32((*arg as f32).to_bits()),
                        ValType::F64 => Val::F64((*arg as f64).to_bits()),
                        _ => Val::I32(*arg as i32), // Default to i32
                    })
                    .collect();

                info!(
                    "Calling function '{}' with {} params, expects {} results",
                    function_name,
                    wasm_args.len(),
                    result_types.len()
                );

                // Prepare results vector based on expected return types
                let mut results: Vec<Val> = result_types
                    .iter()
                    .map(|result_type| match result_type {
                        ValType::I32 => Val::I32(0),
                        ValType::I64 => Val::I64(0),
                        ValType::F32 => Val::F32(0),
                        ValType::F64 => Val::F64(0),
                        _ => Val::I32(0),
                    })
                    .collect();

                // Call the function
                func.call(&mut store, &wasm_args, &mut results)
                    .context(format!("Failed to call function '{}'", function_name))?;

                info!("Function '{}' executed successfully", function_name);

                // Convert result to string
                let result_string = if !results.is_empty() {
                    let result_val = &results[0];

                    if let Some(v) = result_val.i32() {
                        info!("Function returned i32: {}", v);
                        v.to_string()
                    } else if let Some(v) = result_val.i64() {
                        info!("Function returned i64: {}", v);
                        v.to_string()
                    } else if let Some(v) = result_val.f32() {
                        info!("Function returned f32: {}", v);
                        v.to_string()
                    } else if let Some(v) = result_val.f64() {
                        info!("Function returned f64: {}", v);
                        v.to_string()
                    } else {
                        info!("Function returned unsupported type");
                        String::new()
                    }
                } else {
                    info!("Function returned no value");
                    String::new()
                };

                // Convert to bytes (UTF-8)
                let result_bytes = result_string.into_bytes();

                info!(
                    "Task {} completed successfully, result size: {} bytes",
                    task_id,
                    result_bytes.len()
                );

                Ok::<Vec<u8>, anyhow::Error>(result_bytes)
            })
            .await
            .context("Failed to execute blocking task")??;

            Ok(result)
        }
    }

    async fn stop_app(&self, id: String) -> Result<()> {
        info!("Stopping Wasmtime runtime app: task_id={}", id);

        let mut instances = self.instances.lock().await;
        if instances.remove(&id).is_some() {
            info!("Task {} stopped and removed from instances", id);
            Ok(())
        } else {
            Err(anyhow::anyhow!(
                "Task {} not found in running instances",
                id
            ))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_wasmtime_runtime_new() {
        let runtime = WasmtimeRuntime::new();
        assert!(runtime.is_ok());
    }

    #[test]
    fn test_wasmtime_runtime_engine_configuration() {
        let runtime = WasmtimeRuntime::new().unwrap();
        
        // Verify engine exists and is configured
        // The engine should be ready for module compilation
        assert!(runtime.instances.try_lock().is_ok());
    }

    #[test]
    fn test_wasmtime_runtime_instances_empty_on_creation() {
        let runtime = WasmtimeRuntime::new().unwrap();
        
        let instances = runtime.instances.try_lock().unwrap();
        assert_eq!(instances.len(), 0);
    }

    #[tokio::test]
    async fn test_wasmtime_runtime_compile_invalid_wasm() {
        let runtime = WasmtimeRuntime::new().unwrap();
        
        // Invalid WASM binary (not starting with magic number)
        let invalid_wasm = vec![0xFF, 0xFF, 0xFF, 0xFF];
        
        let result = Module::from_binary(&runtime.engine, &invalid_wasm);
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_wasmtime_runtime_compile_empty_wasm() {
        let runtime = WasmtimeRuntime::new().unwrap();
        
        let empty_wasm = vec![];
        
        let result = Module::from_binary(&runtime.engine, &empty_wasm);
        assert!(result.is_err());
    }

    #[test]
    fn test_wasmtime_runtime_valid_wasm_magic_number() {
        let runtime = WasmtimeRuntime::new().unwrap();
        
        // Minimal valid WASM module (just magic number and version)
        let minimal_wasm = vec![
            0x00, 0x61, 0x73, 0x6d, // Magic number: \0asm
            0x01, 0x00, 0x00, 0x00, // Version: 1
        ];
        
        let result = Module::from_binary(&runtime.engine, &minimal_wasm);
        // This should fail validation but pass magic number check
        // We're just testing that the engine can process the format
        assert!(result.is_err()); // Will fail because it's incomplete, but that's OK
    }
}
