use anyhow::{Context, Result};
use async_trait::async_trait;
use image_rs::layer_store::LayerStore;
use image_rs::meta_store::MetaStore;
use image_rs::pull::PullClient;
use kbs_protocol::evidence_provider::NativeEvidenceProvider;
use kbs_protocol::KbsClientBuilder;
use kbs_protocol::KbsClientCapabilities;
use oci_client::client::ClientConfig;
use oci_client::secrets::RegistryAuth;
use oci_client::Reference;
use oci_spec::image::ImageConfiguration;
use resource_uri::ResourceUri;
use std::collections::HashMap;
use std::path::PathBuf;
use std::process::Command;
use std::sync::Arc;
use tokio::io::AsyncWriteExt;
use tokio::sync::RwLock;
use tracing::{debug, error, info, warn};
use wasmtime::HeapType;

use crate::config::PropletConfig;
use crate::runtime::{Runtime, RuntimeContext, StartConfig};

#[cfg(feature = "tee")]
use attestation_agent::{AttestationAPIs, AttestationAgent};

type KbsClientType = Box<dyn KbsClientCapabilities>;

pub struct TeeWasmRuntime {
    config: PropletConfig,
    #[cfg(feature = "tee")]
    attestation_agent: Option<AttestationAgent>,
}

impl TeeWasmRuntime {
    pub async fn new(config: &PropletConfig) -> Result<Self> {
        #[cfg(feature = "tee")]
        {
            info!("Initializing TEE support");
            let mut agent = AttestationAgent::new(config.aa_config_path.as_deref())
                .context("Failed to create attestation agent")?;
            agent.init().await?;
            info!("TEE attestation agent initialized");

            if config.tee_enabled {
                if let Err(e) = Self::setup_keyprovider_config() {
                    warn!("Failed to setup keyprovider config: {}, continuing without it", e);
                    warn!("Decryption may fail - ensure OCICRYPT_KEYPROVIDER_CONFIG is set");
                }
            }

            return Ok(Self {
                config: config.clone(),
                attestation_agent: Some(agent),
            });
        }

        #[cfg(not(feature = "tee"))]
        Ok(Self {
            config: config.clone(),
            attestation_agent: None,
        })
    }

    #[cfg(feature = "tee")]
    fn setup_keyprovider_config() -> Result<()> {
        info!("Setting up keyprovider configuration");
        
        // Check if OCICRYPT_KEYPROVIDER_CONFIG is already set
        if let Ok(existing_path) = std::env::var("OCICRYPT_KEYPROVIDER_CONFIG") {
            info!("OCICRYPT_KEYPROVIDER_CONFIG already set to: {}", existing_path);
            if std::path::Path::new(&existing_path).exists() {
                info!("Config file exists, using existing configuration");
                return Ok(());
            } else {
                warn!("Config file at {} does not exist, will create new one", existing_path);
            }
        }
        
        let config_path = PathBuf::from("/tmp/proplet/ocicrypt_keyprovider.conf");
        let config_dir = config_path.parent().unwrap();
        
        std::fs::create_dir_all(&config_dir)
            .context("Failed to create keyprovider config directory")?;

        // Use TCP address to match coco-keyprovider (listening on 50011)
        let keyprovider_addr = "127.0.0.1:50011";
        
        let config_content = format!(
            r#"{{
  "key-providers": {{
    "attestation-agent": {{
      "grpc": "{}"
    }}
  }}
}}"#,
            keyprovider_addr
        );

        std::fs::write(&config_path, config_content)
            .context("Failed to write keyprovider config")?;

        info!("Created OCICRYPT_KEYPROVIDER_CONFIG at: {}", config_path.display());
        info!("Keyprovider address (TCP): {}", keyprovider_addr);
        info!("This should match coco-keyprovider listening address");
        warn!("Verify coco-keyprovider is listening on: {}", keyprovider_addr);

        std::env::set_var("OCICRYPT_KEYPROVIDER_CONFIG", config_path);

        info!("Set OCICRYPT_KEYPROVIDER_CONFIG environment variable");

        Ok(())
    }

    #[cfg(feature = "tee")]
    async fn setup_kbs_client(&self) -> Result<KbsClientType> {
        let kbs_uri = self
            .config
            .kbs_uri
            .as_ref()
            .ok_or_else(|| anyhow::anyhow!("KBS URI is required for encrypted images"))?;

        info!("Setting up KBS client with URI: {}", kbs_uri);

        let evidence_provider = Box::new(NativeEvidenceProvider::new()?);

        let client = KbsClientBuilder::with_evidence_provider(evidence_provider, kbs_uri).build()?;

        Ok(Box::new(client))
    }

    #[cfg(feature = "tee")]
    async fn get_decryption_key(&self, mut client: Box<dyn KbsClientCapabilities>, kbs_resource_path: &str) -> Result<Vec<u8>> {
        info!("Using KBS resource path: {}", kbs_resource_path);

        let kbs_uri = self
            .config
            .kbs_uri
            .as_ref()
            .ok_or_else(|| anyhow::anyhow!("KBS URI is required for encrypted images"))?;

        let kbs_host = kbs_uri
            .trim_start_matches("http://")
            .trim_start_matches("https://");

        let full_resource_uri = format!("kbs://{}/{}", kbs_host, kbs_resource_path);
        info!("Constructed full KBS resource URI: {}", full_resource_uri);

        let resource_uri = ResourceUri::try_from(full_resource_uri.as_str())
            .map_err(|e| anyhow::anyhow!("Failed to create resource URI: {}", e))?;

        info!("Fetching resource from KBS: {:?}", resource_uri);

        let key = client
            .get_resource(resource_uri)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to get decryption key from KBS: {}", e))?;

        info!("Received key from KBS: {} bytes", key.len());

        if key.is_empty() {
            return Err(anyhow::anyhow!(
                "Invalid decryption key from KBS: Empty key"
            ));
        }

        info!("Decryption key from KBS: {} bytes", key.len());

        Ok(key)
    }

    #[cfg(feature = "tee")]
    async fn pull_and_decrypt_wasm(&self, oci_reference: &str, kbs_resource_path: &str) -> Result<PathBuf> {
        // Ensure OCICRYPT_KEYPROVIDER_CONFIG is set before any image-rs operations
        // This is critical because ocicrypt-rs reads it at lazy_static initialization
        if let Err(e) = Self::setup_keyprovider_config() {
            warn!("Failed to setup keyprovider config: {}", e);
        }
        
        let image_ref = Reference::try_from(oci_reference.to_string())
            .context("Failed to parse image reference")?;

        info!("Pulling image: {}", image_ref);

        let layer_store = LayerStore::new(PathBuf::from(&self.config.layer_store_path))
            .context("Failed to create layer store")?;

        let client_config = ClientConfig::default();

        let mut pull_client = PullClient::new(
            image_ref.clone(),
            layer_store,
            &RegistryAuth::Anonymous,
            4,
            client_config,
        )?;

        let (manifest, _digest, config) = pull_client
            .pull_manifest()
            .await
            .context("Failed to pull manifest")?;

        info!("Successfully pulled manifest for image: {}", image_ref);

        let is_wasm_image = manifest.config.media_type.contains("wasm")
            || manifest
                .layers
                .iter()
                .any(|l| l.media_type.contains("wasm"));

        let is_encrypted = manifest
            .layers
            .iter()
            .any(|l| l.media_type.contains("encrypted"));

        info!("Image type: {}", if is_wasm_image { "WASM" } else { "OCI" });
        info!("Image encrypted: {}", is_encrypted);

        if is_wasm_image && !is_encrypted {
            info!("Pulling WASM blob directly");

            let wasm_layer = manifest
                .layers
                .first()
                .ok_or_else(|| anyhow::anyhow!("No layers found in WASM image"))?;

            info!(
                "WASM layer: {} ({})",
                wasm_layer.digest,
                wasm_layer.media_type
            );

            let blob_stream = pull_client
                .client
                .pull_blob_stream(&image_ref, wasm_layer)
                .await
                .context("Failed to pull WASM blob")?;

            let wasm_filename = format!("{}.wasm", wasm_layer.digest.replace("sha256:", ""));
            let wasm_path = PathBuf::from(&self.config.layer_store_path).join(wasm_filename);

            info!("Writing WASM to: {:?}", wasm_path);

            let mut file = tokio::fs::File::create(&wasm_path)
                .await
                .context("Failed to create WASM file")?;

            use futures_util::StreamExt;
            let mut stream = blob_stream.stream;
            while let Some(chunk) = stream.next().await {
                let chunk = chunk.context("Failed to read blob chunk")?;
                file.write_all(&chunk)
                    .await
                    .context("Failed to write WASM chunk")?;
            }

            file.sync_all().await.context("Failed to sync WASM file")?;

            info!("Successfully pulled WASM to: {:?}", wasm_path);

            Ok(wasm_path)
        } else {
            info!("Processing with OCI layer handler (for decryption if encrypted)");

            let image_config =
                ImageConfiguration::from_reader(config.as_bytes()).unwrap_or_else(|e| {
                    warn!(
                        "Failed to parse image config (may be minimal WASM config): {}",
                        e
                    );
                    info!("Using default OCI configuration for WASM image");
                    ImageConfiguration::default()
                });

            let diff_ids = image_config.rootfs().diff_ids();

            let diff_ids_vec: Vec<String> = if diff_ids.is_empty() && !manifest.layers.is_empty() {
                info!(
                    "Image config has no diff_ids, generating placeholders for {} layers",
                    manifest.layers.len()
                );
                manifest
                    .layers
                    .iter()
                    .map(|layer| {
                        if is_encrypted {
                            info!("Using empty diff_id for encrypted layer (digest validation will be skipped)");
                            String::new()
                        } else {
                            layer.digest.clone()
                        }
                    })
                    .collect()
            } else {
                diff_ids.to_vec()
            };

            let decrypt_config: Option<String> = if is_encrypted {
                info!(
                    "Encrypted image detected - keyprovider will handle decryption via gRPC"
                );
                info!("Ensure OCICRYPT_KEYPROVIDER_CONFIG is set");
                Some("provider:attestation-agent".to_string())
            } else {
                info!("No encrypted layers detected");
                None
            };

            let layer_metas = pull_client
                .async_pull_layers(
                    manifest.layers.clone(),
                    &diff_ids_vec,
                    &decrypt_config.as_deref(),
                    Arc::new(RwLock::new(MetaStore::default())),
                )
                .await
                .context("Failed to pull and decrypt layers")?;

            let layer_store_path = layer_metas
                .first()
                .map(|m| PathBuf::from(&m.store_path))
                .ok_or_else(|| anyhow::anyhow!("No layers found in image"))?;

            info!("Layer store path: {:?}", layer_store_path);

            let wasm_path = layer_store_path.join("module.wasm");
            info!("Checking for WASM at: {:?}", wasm_path);

            if wasm_path.exists() && wasm_path.is_file() {
                info!("Found WASM module at: {:?}", wasm_path);
                return Ok(wasm_path);
            }

            info!("module.wasm not found, searching directory...");

            if let Ok(entries) = std::fs::read_dir(&layer_store_path) {
                let files: Vec<_> = entries.filter_map(|e| e.ok()).collect();
                info!("Directory contents ({} entries):", files.len());
                for entry in &files {
                    info!(
                        "  - {:?} (is_file: {})",
                        entry.path(),
                        entry.path().is_file()
                    );
                }

                for entry in files {
                    let path = entry.path();
                    if path.is_file() {
                        if let Some(ext) = path.extension() {
                            if ext == "wasm" {
                                info!("Found WASM file: {:?}", path);
                                return Ok(path);
                            }
                        }
                    }
                }
            }

            Err(anyhow::anyhow!(
                "No WASM file found in layer store: {:?}",
                layer_store_path
            ))
        }
    }

    #[cfg(not(feature = "tee"))]
    async fn pull_and_decrypt_wasm(&self, _oci_reference: &str) -> Result<PathBuf> {
        Err(anyhow::anyhow!(
            "TEE support is not enabled. Please compile with the 'tee' feature."
        ))
    }

    fn run_wasm(&self, wasm_path: &PathBuf, config: &StartConfig) -> Result<Vec<u8>> {
        info!("Running WASM from: {:?}", wasm_path);
        info!("Function: {}", config.function_name);

        let runtime_path = self.config.external_wasm_runtime.as_ref()
            .map(|s| s.as_str())
            .unwrap_or("wasmtime");
        
        info!("Using WASM runtime: {}", runtime_path);
        
        let mut cmd = Command::new(runtime_path);

        // Add --dir flag for WASI directory access
        cmd.arg("--dir").arg("/tmp");

        // Add cli_args (like --invoke add) before the wasm file
        for arg in &config.cli_args {
            cmd.arg(arg);
        }

        // Add the wasm file path
        cmd.arg(wasm_path);

        // Add function arguments after the wasm file
        for arg in &config.args {
            cmd.arg(arg.to_string());
        }

        info!("Executing command: {:?}", cmd);

        // Explicitly set stdout and stderr to be piped
        cmd.stdout(std::process::Stdio::piped());
        cmd.stderr(std::process::Stdio::piped());

        let output = cmd.output().context("Failed to execute WASM runtime")?;

        info!("WASM exit status: {:?}", output.status);
        info!("WASM stdout length: {} bytes", output.stdout.len());
        info!("WASM stderr length: {} bytes", output.stderr.len());

        if !output.stdout.is_empty() {
            info!("WASM stdout:\n{}", String::from_utf8_lossy(&output.stdout));
        } else {
            info!("WASM stdout is empty");
        }

        if !output.stderr.is_empty() {
            warn!("WASM stderr:\n{}", String::from_utf8_lossy(&output.stderr));
        } else {
            info!("WASM stderr is empty");
        }

        if !output.status.success() {
            return Err(anyhow::anyhow!(
                "WASM execution failed with exit code: {:?}",
                output.status.code()
            ));
        }

        info!("WASM execution completed successfully");

        Ok(output.stdout)
    }

    fn run_wasm_embedded(&self, wasm_binary: &[u8], start_config: &StartConfig) -> Result<Vec<u8>> {
        use wasmtime::*;

        info!("Running embedded WASM with wasmtime (without WASI for now)");

        let mut config = Config::new();
        config.wasm_component_model(false);
        config.async_support(false);

        let engine = Engine::new(&config)?;
        let mut linker = Linker::new(&engine);

        let module = Module::from_binary(&engine, wasm_binary)?;

        let mut store = Store::new(&engine, ());
        let instance = linker.instantiate(&mut store, &module)?;

        let func_name = if start_config.function_name.is_empty() {
            "_start"
        } else {
            &start_config.function_name
        };

        let func = instance.get_func(&mut store, func_name)
            .ok_or_else(|| anyhow::anyhow!("Function '{}' not found in WASM module", func_name))?;

        let result_count = func.ty(&store).results().len();
        let mut results = if result_count > 0 {
            vec![Val::null_any_ref()]
        } else {
            vec![]
        };

        func.call(&mut store, &[], &mut results)?;

        let output = results
            .into_iter()
            .filter_map(|v| match v {
                Val::I32(i) => Some(i.to_le_bytes().to_vec()),
                Val::I64(i) => Some(i.to_le_bytes().to_vec()),
                _ => None,
            })
            .flatten()
            .collect::<Vec<_>>();

        Ok(output)
    }
}

#[async_trait]
impl Runtime for TeeWasmRuntime {
    async fn start_app(&self, _ctx: RuntimeContext, config: StartConfig) -> Result<Vec<u8>> {
        info!("Starting TEE WASM app: {}", config.id);

        #[cfg(feature = "tee")]
        {
            if let Some(ref agent) = self.attestation_agent {
                let evidence = agent
                    .get_evidence(b"wasm-runner")
                    .await
                    .context("Failed to get TEE evidence")?;
                info!("TEE evidence obtained: {} bytes", evidence.len());
            }
        }

        if !config.wasm_binary.is_empty() {
            return Ok(self.run_wasm_embedded(&config.wasm_binary, &config)?);
        }

        if !config.cli_args.is_empty() {
            if let Some(oci_ref) = config.cli_args.first() {
                if oci_ref.contains("/") || oci_ref.contains(":") {
                    info!("Pulling WASM from OCI: {}", oci_ref);
                    let kbs_path: &str = if config.kbs_resource_path.as_ref().map_or(false, |s| !s.is_empty()) {
                        info!("Using config-level KBS resource path: {}", config.kbs_resource_path.as_ref().unwrap_or(&String::new()));
                        config.kbs_resource_path.as_ref().map(|s| s.as_str()).unwrap_or("")
                    } else {
                        info!("No config-level KBS resource path, will use task-level if provided");
                        ""
                    };
                    let wasm_path = self.pull_and_decrypt_wasm(oci_ref, kbs_path).await?;
                    
                    // Remove the OCI reference from cli_args before executing
                    // since it's not needed by wasmtime (we already pulled the image)
                    let mut exec_config = config.clone();
                    exec_config.cli_args.remove(0);
                    
                    return self.run_wasm(&wasm_path, &exec_config);
                }
            }
        }

        Err(anyhow::anyhow!("No WASM binary or OCI reference provided"))
    }

    async fn stop_app(&self, _id: String) -> Result<()> {
        info!("Stopping TEE WASM app: {}", _id);
        Ok(())
    }

    async fn get_pid(&self, _id: &str) -> Result<Option<u32>> {
        Ok(None)
    }
}