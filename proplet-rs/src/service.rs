use crate::config::PropletConfig;
use crate::mqtt::{build_topic, MqttMessage, PubSub};
use crate::runtime::{Runtime, RuntimeContext};
use crate::types::*;
use anyhow::{Context, Result};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{mpsc, Mutex};
use tracing::{debug, error, info};

pub struct PropletService {
    config: PropletConfig,
    proplet: Arc<Mutex<Proplet>>,
    pubsub: PubSub,
    runtime: Arc<dyn Runtime>,
    chunks: Arc<Mutex<HashMap<String, Vec<Vec<u8>>>>>,
    chunk_metadata: Arc<Mutex<HashMap<String, ChunkMetadata>>>,
    running_tasks: Arc<Mutex<HashMap<String, TaskState>>>,
}

impl PropletService {
    pub fn new(config: PropletConfig, pubsub: PubSub, runtime: Arc<dyn Runtime>) -> Self {
        let proplet = Proplet::new(config.instance_id, "proplet-rs".to_string());

        Self {
            config,
            proplet: Arc::new(Mutex::new(proplet)),
            pubsub,
            runtime,
            chunks: Arc::new(Mutex::new(HashMap::new())),
            chunk_metadata: Arc::new(Mutex::new(HashMap::new())),
            running_tasks: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    pub async fn run(
        self: Arc<Self>,
        mut mqtt_rx: mpsc::UnboundedReceiver<MqttMessage>,
    ) -> Result<()> {
        info!("Starting PropletService");

        // Publish discovery message
        self.publish_discovery().await?;

        // Subscribe to topics
        self.subscribe_topics().await?;

        // Start liveliness updates
        let service = self.clone();
        tokio::spawn(async move {
            service.start_liveliness_updates().await;
        });

        // Process MQTT messages
        while let Some(msg) = mqtt_rx.recv().await {
            let service = self.clone();
            tokio::spawn(async move {
                if let Err(e) = service.handle_message(msg).await {
                    error!("Error handling message: {}", e);
                }
            });
        }

        Ok(())
    }

    async fn subscribe_topics(&self) -> Result<()> {
        let start_topic = build_topic(
            &self.config.domain_id,
            &self.config.channel_id,
            "control/manager/start",
        );
        self.pubsub.subscribe(&start_topic).await?;

        let stop_topic = build_topic(
            &self.config.domain_id,
            &self.config.channel_id,
            "control/manager/stop",
        );
        self.pubsub.subscribe(&stop_topic).await?;

        let chunk_topic = build_topic(
            &self.config.domain_id,
            &self.config.channel_id,
            "registry/server",
        );
        self.pubsub.subscribe(&chunk_topic).await?;

        Ok(())
    }

    async fn publish_discovery(&self) -> Result<()> {
        let discovery = DiscoveryMessage {
            proplet_id: self.config.client_id.clone(),
            namespace: self
                .config
                .k8s_namespace
                .clone()
                .unwrap_or_else(|| "default".to_string()),
        };

        let topic = build_topic(
            &self.config.domain_id,
            &self.config.channel_id,
            "control/proplet/create",
        );

        self.pubsub.publish(&topic, &discovery).await?;
        info!("Published discovery message");

        Ok(())
    }

    async fn start_liveliness_updates(&self) {
        let mut interval = tokio::time::interval(self.config.liveliness_interval());

        loop {
            interval.tick().await;

            if let Err(e) = self.publish_liveliness().await {
                error!("Failed to publish liveliness: {}", e);
            }
        }
    }

    async fn publish_liveliness(&self) -> Result<()> {
        let mut proplet = self.proplet.lock().await;
        proplet.set_alive(true);

        let running_tasks = self.running_tasks.lock().await;
        proplet.task_count = running_tasks.len();

        let liveliness = LivelinessMessage {
            proplet_id: self.config.client_id.clone(),
            status: "alive".to_string(),
            namespace: self
                .config
                .k8s_namespace
                .clone()
                .unwrap_or_else(|| "default".to_string()),
        };

        let topic = build_topic(
            &self.config.domain_id,
            &self.config.channel_id,
            "control/proplet/alive",
        );

        self.pubsub.publish(&topic, &liveliness).await?;
        debug!("Published liveliness update");

        Ok(())
    }

    async fn handle_message(&self, msg: MqttMessage) -> Result<()> {
        debug!("Handling message from topic: {}", msg.topic);

        if let Ok(payload_str) = String::from_utf8(msg.payload.clone()) {
            debug!("Raw message payload: {}", payload_str);
        }

        if msg.topic.contains("control/manager/start") {
            self.handle_start_command(msg).await
        } else if msg.topic.contains("control/manager/stop") {
            self.handle_stop_command(msg).await
        } else if msg.topic.contains("registry/server") {
            self.handle_chunk(msg).await
        } else {
            debug!("Ignoring message from unknown topic: {}", msg.topic);
            Ok(())
        }
    }

    async fn handle_start_command(&self, msg: MqttMessage) -> Result<()> {
        let req: StartRequest = msg.decode().map_err(|e| {
            error!("Failed to decode start request: {}", e);
            if let Ok(payload_str) = String::from_utf8(msg.payload.clone()) {
                error!("Payload was: {}", payload_str);
            }
            e
        })?;
        req.validate()?;

        info!("Received start command for task: {}", req.id);

        // Update running tasks
        {
            let mut tasks = self.running_tasks.lock().await;
            tasks.insert(req.id.clone(), TaskState::Running);
        }

        let wasm_binary = if !req.file.is_empty() {
            // Decode base64-encoded wasm file
            debug!("Decoding base64 wasm file, encoded size: {}", req.file.len());
            use base64::{engine::general_purpose::STANDARD, Engine};
            let decoded = STANDARD
                .decode(&req.file)
                .context("Failed to decode base64 file")?;
            info!("Decoded wasm binary, size: {} bytes", decoded.len());
            decoded
        } else if !req.image_url.is_empty() {
            // Request binary from registry
            info!("Requesting binary from registry: {}", req.image_url);
            self.request_binary_from_registry(&req.image_url).await?;

            // Wait for chunks to be assembled
            match self.wait_for_binary(&req.image_url).await {
                Ok(binary) => binary,
                Err(e) => {
                    error!("Failed to get binary for task {}: {}", req.id, e);
                    self.publish_result(&req.id, Vec::new(), Some(e.to_string()))
                        .await?;
                    return Err(e);
                }
            }
        } else {
            return Err(anyhow::anyhow!("No wasm binary or image URL provided"));
        };

        // Execute the task
        let runtime = self.runtime.clone();
        let pubsub = self.pubsub.clone();
        let running_tasks = self.running_tasks.clone();
        let domain_id = self.config.domain_id.clone();
        let channel_id = self.config.channel_id.clone();
        let proplet_id = self.proplet.lock().await.id;
        let task_id = req.id.clone();
        let task_name = req.name.clone();
        let env = req.env.unwrap_or_default();

        tokio::spawn(async move {
            let ctx = RuntimeContext { proplet_id };

            info!("Executing task {} in spawned task", task_id);

            let result = runtime
                .start_app(
                    ctx,
                    wasm_binary,
                    req.cli_args,
                    task_id.clone(),
                    task_name.clone(),
                    req.daemon,
                    env,
                    req.inputs,
                )
                .await;

            // Publish result
            let topic = build_topic(&domain_id, &channel_id, "control/proplet/results");

            // Build payload based on result
            let payload = match result {
                Ok(data) => {
                    let result_str = String::from_utf8_lossy(&data).to_string();
                    info!("Task {} completed successfully. Result: {}", task_id, result_str);

                    serde_json::json!({
                        "task_id": task_id.clone(),
                        "results": result_str,
                    })
                }
                Err(e) => {
                    error!("Task {} failed: {}", task_id, e);

                    serde_json::json!({
                        "task_id": task_id.clone(),
                        "error": e.to_string(),
                        "results": "",
                    })
                }
            };

            info!("Publishing result for task {}: {:?}", task_id, payload);

            if let Err(e) = pubsub.publish(&topic, &payload).await {
                error!("Failed to publish result for task {}: {}", task_id, e);
            } else {
                info!("Successfully published result for task {}", task_id);
            }

            // Remove from running tasks
            running_tasks.lock().await.remove(&task_id);
        });

        Ok(())
    }

    async fn handle_stop_command(&self, msg: MqttMessage) -> Result<()> {
        let req: StopRequest = msg.decode()?;
        req.validate()?;

        info!("Received stop command for task: {}", req.id);

        // Stop the task
        self.runtime.stop_app(req.id.clone()).await?;

        // Remove from running tasks
        self.running_tasks.lock().await.remove(&req.id);

        Ok(())
    }

    async fn handle_chunk(&self, msg: MqttMessage) -> Result<()> {
        let chunk: Chunk = msg.decode()?;

        debug!(
            "Received chunk {}/{} for app '{}'",
            chunk.chunk_idx + 1,
            chunk.total_chunks,
            chunk.app_name
        );

        let mut chunks = self.chunks.lock().await;
        let mut metadata = self.chunk_metadata.lock().await;

        // Store metadata if not already present
        if !metadata.contains_key(&chunk.app_name) {
            metadata.insert(
                chunk.app_name.clone(),
                ChunkMetadata {
                    app_name: chunk.app_name.clone(),
                    total_chunks: chunk.total_chunks,
                },
            );
        }

        // Append chunk data
        chunks
            .entry(chunk.app_name.clone())
            .or_insert_with(Vec::new)
            .push(chunk.data);

        Ok(())
    }

    async fn request_binary_from_registry(&self, app_name: &str) -> Result<()> {
        let topic = build_topic(
            &self.config.domain_id,
            &self.config.channel_id,
            "registry/proplet",
        );

        #[derive(serde::Serialize)]
        struct RegistryRequest {
            app_name: String,
        }

        let req = RegistryRequest {
            app_name: app_name.to_string(),
        };
        self.pubsub.publish(&topic, &req).await?;

        debug!("Requested binary from registry for app: {}", app_name);
        Ok(())
    }

    async fn wait_for_binary(&self, app_name: &str) -> Result<Vec<u8>> {
        // Wait up to 60 seconds for all chunks
        let timeout = tokio::time::Duration::from_secs(60);
        let start = tokio::time::Instant::now();
        let polling_interval = tokio::time::Duration::from_secs(5);

        loop {
            if start.elapsed() > timeout {
                return Err(anyhow::anyhow!("Timeout waiting for binary chunks"));
            }

            let assembled = self.try_assemble_chunks(app_name).await?;
            if let Some(binary) = assembled {
                return Ok(binary);
            }

            tokio::time::sleep(polling_interval).await;
        }
    }

    async fn try_assemble_chunks(&self, app_name: &str) -> Result<Option<Vec<u8>>> {
        let chunks = self.chunks.lock().await;
        let metadata = self.chunk_metadata.lock().await;

        if let Some(meta) = metadata.get(app_name) {
            if let Some(app_chunks) = chunks.get(app_name) {
                // Check if all chunks are present
                if app_chunks.len() == meta.total_chunks {
                    // Assemble binary
                    let mut binary = Vec::new();
                    for chunk_data in app_chunks {
                        binary.extend_from_slice(chunk_data);
                    }

                    info!(
                        "Assembled binary for app '{}', size: {} bytes",
                        app_name,
                        binary.len()
                    );
                    return Ok(Some(binary));
                }
            }
        }

        Ok(None)
    }

    async fn publish_result(
        &self,
        task_id: &str,
        result: Vec<u8>,
        error: Option<String>,
    ) -> Result<()> {
        let proplet_id = self.proplet.lock().await.id;

        let result_msg = ResultMessage {
            task_id: task_id.to_string(),
            proplet_id,
            result,
            error,
        };

        let topic = build_topic(
            &self.config.domain_id,
            &self.config.channel_id,
            "control/proplet/results",
        );

        self.pubsub.publish(&topic, &result_msg).await?;
        Ok(())
    }
}
