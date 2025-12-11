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
    /// Creates a new PropletService configured to manage proplet lifecycle, messaging, and runtime tasks.
    ///
    /// Initializes an internal Proplet using `config.instance_id` and the agent string `"proplet-rs"`,
    /// and prepares empty, thread-safe maps for received chunks, chunk metadata, and running tasks.
    ///
    /// # Examples
    ///
    /// ```
    /// // Construct required components (types shown for clarity; replace with real values).
    /// let config = PropletConfig::default();
    /// let pubsub = PubSub::new(); // or a test/dummy PubSub
    /// let runtime: std::sync::Arc<dyn Runtime> = std::sync::Arc::new(MyRuntime::new());
    ///
    /// let service = PropletService::new(config, pubsub, runtime);
    /// assert_eq!(service.config.instance_id, /* expected id value */ service.config.instance_id);
    /// ```
    pub fn new(config: PropletConfig, pubsub: PubSub, runtime: Arc<dyn Runtime>) -> Self {
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

    /// Start the PropletService: publish discovery, subscribe to control/registry topics, spawn periodic liveliness updates,
    /// and process incoming MQTT messages by dispatching each to a handler task.
    ///
    /// This method consumes an `Arc<Self>` and runs until the provided `mqtt_rx` channel is closed. For each incoming
    /// MQTT message a new task is spawned to handle the message; liveliness updates are published on a background task.
    ///
    /// # Examples
    ///
    /// ```no_run
    /// use std::sync::Arc;
    /// use tokio::sync::mpsc;
    ///
    /// // Assume `service` is an already constructed `Arc<PropletService>` and `MqttMessage` is in scope.
    /// let (tx, rx) = mpsc::unbounded_channel();
    /// let service: Arc<PropletService> = /* constructed elsewhere */ unimplemented!();
    ///
    /// // Run the service (typically spawned on the Tokio runtime)
    /// let svc = service.clone();
    /// tokio::spawn(async move {
    ///     let _ = svc.run(rx).await;
    /// });
    /// ```
    ///
    /// # Returns
    ///
    /// `Ok(())` when the receiver closes and the run loop exits, or an error if initialization (discovery/subscription)
    /// fails.
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

    /// Subscribe the service to control and registry topics for its configured domain and channel.
    ///
    /// Subscribes to the following topics derived from `self.config`:
    /// - `control/manager/start`
    /// - `control/manager/stop`
    /// - `registry/server`
    ///
    /// # Returns
    ///
    /// `Ok(())` on success, or an error if subscribing to any topic fails.
    ///
    /// # Examples
    ///
    /// ```
    /// # use anyhow::Result;
    /// # #[tokio::test]
    /// # async fn example() -> Result<()> {
    /// # let service: crate::PropletService = /* construct service */ unimplemented!();
    /// service.subscribe_topics().await?;
    /// # Ok(())
    /// # }
    /// ```
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

    /// Publishes a discovery message containing this proplet's ID and namespace to the control/proplet/create topic.
    ///
    /// The message's `proplet_id` is taken from `config.client_id`. The `namespace` uses `config.k8s_namespace` if set, otherwise `"default"`.
    ///
    /// # Examples
    ///
    /// ```ignore
    /// // Given an initialized `service: PropletService` and an async runtime:
    /// service.publish_discovery().await?;
    /// ```
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

    /// Publish a liveliness update for this proplet.
    ///
    /// The function marks the proplet as alive, updates the proplet's task count from
    /// the tracked running tasks, and publishes a `LivelinessMessage` on the
    /// `control/proplet/alive` topic derived from the service configuration.
    ///
    /// # Returns
    ///
    /// `Ok(())` if the liveliness message was published successfully, `Err(...)` if
    /// the publish operation fails.
    ///
    /// # Examples
    ///
    /// ```no_run
    /// # use anyhow::Result;
    /// # async fn example(svc: &crate::PropletService) -> Result<()> {
    /// svc.publish_liveliness().await?;
    /// # Ok(())
    /// # }
    /// ```
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

    /// Dispatches an incoming MQTT message to the appropriate handler based on its topic.
    ///
    /// Tries to log the UTF-8 representation of the payload for debugging, then routes messages whose
    /// topics contain "control/manager/start", "control/manager/stop", or "registry/server" to their
    /// respective handlers. Messages from unknown topics are ignored.
    ///
    /// # Parameters
    ///
    /// - `msg`: The MQTT message whose topic and payload determine which internal handler is invoked.
    ///
    /// # Returns
    ///
    /// `Ok(())` if the message was handled or ignored successfully, or an error returned by a specific handler.
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

    /// Handle an incoming "start" control message and initiate the requested task.
    ///
    /// Decodes the MQTT payload as a `StartRequest`, validates it, and records the task as running.
    /// The function obtains the WebAssembly binary either by base64-decoding the `file` field or by
    /// requesting and assembling chunks from the registry using `image_url`. If the binary is acquired,
    /// the function spawns a background task that executes the app via the configured runtime and
    /// publishes a `ResultMessage` (success or failure) to the results topic. On any failure prior to
    /// spawning execution (decode/validation/obtaining binary), the function publishes an error result
    /// and removes the task from the running set.
    ///
    /// # Parameters
    ///
    /// - `msg`: MQTT message containing a serialized `StartRequest` payload.
    ///
    /// # Returns
    ///
    /// `Ok(())` if handling was initiated (the runtime task was spawned or an error result was published).
    /// `Err(...)` if decoding/validation or obtaining the binary failed and the error was propagated.
    ///
    /// # Examples
    ///
    /// ```rust,no_run
    /// # async fn example(service: &crate::PropletService, msg: crate::mqtt::MqttMessage) -> anyhow::Result<()> {
    /// service.handle_start_command(msg).await?;
    /// # Ok(())
    /// # }
    /// ```
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
            match STANDARD.decode(&req.file) {
                Ok(decoded) => {
                    info!("Decoded wasm binary, size: {} bytes", decoded.len());
                    decoded
                }
                Err(e) => {
                    error!("Failed to decode base64 file for task {}: {}", req.id, e);
                    self.running_tasks.lock().await.remove(&req.id);
                    self.publish_result(&req.id, Vec::new(), Some(e.to_string()))
                        .await?;
                    return Err(e.into());
                }
            }
        } else if !req.image_url.is_empty() {
            // Request binary from registry
            info!("Requesting binary from registry: {}", req.image_url);
            self.request_binary_from_registry(&req.image_url).await?;

            // Wait for chunks to be assembled
            match self.wait_for_binary(&req.image_url).await {
                Ok(binary) => binary,
                Err(e) => {
                    error!("Failed to get binary for task {}: {}", req.id, e);
                    self.running_tasks.lock().await.remove(&req.id);
                    self.publish_result(&req.id, Vec::new(), Some(e.to_string()))
                        .await?;
                    return Err(e);
                }
            }
        } else {
            let err = anyhow::anyhow!("No wasm binary or image URL provided");
            error!("Validation error for task {}: {}", req.id, err);
            self.running_tasks.lock().await.remove(&req.id);
            self.publish_result(&req.id, Vec::new(), Some(err.to_string()))
                .await?;
            return Err(err);
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

            // Publish result using standardized ResultMessage
            let (result_data, error) = match result {
                Ok(data) => {
                    let result_str = String::from_utf8_lossy(&data).to_string();
                    info!("Task {} completed successfully. Result: {}", task_id, result_str);
                    (data, None)
                }
                Err(e) => {
                    error!("Task {} failed: {}", task_id, e);
                    (Vec::new(), Some(e.to_string()))
                }
            };

            let result_msg = ResultMessage {
                task_id: task_id.clone(),
                proplet_id,
                result: result_data,
                error,
            };

            let topic = build_topic(&domain_id, &channel_id, "control/proplet/results");

            info!("Publishing result for task {}", task_id);

            if let Err(e) = pubsub.publish(&topic, &result_msg).await {
                error!("Failed to publish result for task {}: {}", task_id, e);
            } else {
                info!("Successfully published result for task {}", task_id);
            }

            // Remove from running tasks
            running_tasks.lock().await.remove(&task_id);
        });

        Ok(())
    }

    /// Stops the running task specified by the incoming MQTT `StopRequest`.
    ///
    /// The message payload is decoded to a `StopRequest` and validated; the runtime is
    /// instructed to stop the task with the request's `id`, and the task id is removed
    /// from the service's running task registry.
    ///
    /// # Parameters
    ///
    /// - `msg`: MQTT message whose payload must deserialize into a `StopRequest`.
    ///
    /// # Returns
    ///
    /// `Ok(())` when the stop request was handled and the task was removed, or an error
    /// if decoding, validation, or runtime stop fails.
    ///
    /// # Examples
    ///
    /// ```
    /// // Assuming `svc` is a PropletService and `msg` is an MqttMessage containing a valid StopRequest:
    /// // svc.handle_stop_command(msg).await.unwrap();
    /// ```
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

    /// Stores an incoming chunk message for later assembly keyed by the chunk's `app_name`.
    ///
    /// The message payload is decoded as a `Chunk`; the function records `ChunkMetadata` (total
    /// chunks for the app) if missing and appends the chunk bytes to the in-memory chunk list
    /// for that app.
    ///
    /// # Errors
    ///
    /// Returns an error if decoding the MQTT message into a `Chunk` fails.
    ///
    /// # Examples
    ///
    /// ```
    /// # async fn example(svc: &crate::PropletService, msg: crate::mqtt::MqttMessage) -> anyhow::Result<()> {
    /// svc.handle_chunk(msg).await?;
    /// # Ok(())
    /// # }
    /// ```
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

    /// Requests a binary from the registry by publishing a registry request for the specified app.
    ///
    /// Publishes a `RegistryRequest { app_name }` message on the `registry/proplet` topic built
    /// from the service configuration.
    ///
    /// # Examples
    ///
    /// ```no_run
    /// # async fn example(service: &crate::PropletService) -> anyhow::Result<()> {
    /// service.request_binary_from_registry("my-app").await?;
    /// # Ok(())
    /// # }
    /// ```
    ///
    /// # Returns
    ///
    /// `Ok(())` on successful publish, or an error if publishing fails.
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

    /// Waits until all chunks for `app_name` are available and returns the assembled binary.
    ///
    /// Polls every 5 seconds and times out after 60 seconds. If the chunks are assembled before the
    /// timeout, returns the concatenated bytes in order; otherwise returns an error.
    ///
    /// # Examples
    ///
    /// ```no_run
    /// // Given a `svc: &PropletService` in an async context:
    /// // let binary = svc.wait_for_binary("example-app").await?;
    /// // assert!(!binary.is_empty());
    /// ```
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

    /// Attempts to assemble stored chunks for `app_name` into a single contiguous binary.
    ///
    /// If metadata exists for `app_name` and the number of stored chunks equals the expected
    /// total, concatenates the chunks in order and returns `Ok(Some(binary))`. If the
    /// chunks are not yet complete or metadata is missing, returns `Ok(None)`.
    ///
    /// # Examples
    ///
    /// ```
    /// # use futures::executor::block_on;
    /// # // `svc` is a `&PropletService` prepared with stored chunks and metadata for "my-app".
    /// # let svc: &PropletService = unsafe { std::mem::zeroed() }; // placeholder for example only
    /// let result = block_on(svc.try_assemble_chunks("my-app")).unwrap();
    /// if let Some(binary) = result {
    ///     assert!(binary.len() > 0);
    /// } else {
    ///     // not all chunks received yet
    /// }
    /// ```
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

    /// Publish a task result message to the proplet results topic.
    ///
    /// The message includes the proplet's id, the provided `task_id`, binary `result`, and an optional `error`
    /// string; it is sent on the "control/proplet/results" topic derived from the service config.
    ///
    /// # Parameters
    ///
    /// - `task_id`: Identifier of the task whose result is being published.
    /// - `result`: Binary payload produced by the task.
    /// - `error`: Optional error message describing a failure for the task.
    ///
    /// # Returns
    ///
    /// `Ok(())` on successful publication, `Err` if publishing fails.
    ///
    /// # Examples
    ///
    /// ```
    /// # async fn example(service: &crate::PropletService) -> anyhow::Result<()> {
    /// service.publish_result("task-123", vec![1, 2, 3], None).await?;
    /// # Ok(())
    /// # }
    /// ```
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