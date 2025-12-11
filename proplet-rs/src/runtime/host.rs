use super::{Runtime, RuntimeContext};
use anyhow::{Context, Result};
use async_trait::async_trait;
use std::collections::HashMap;
use std::path::PathBuf;
use std::process::Stdio;
use std::sync::Arc;
use tokio::fs;
use tokio::io::AsyncWriteExt;
use tokio::process::{Child, Command};
use tokio::sync::Mutex;
use tracing::{debug, error, info};

pub struct HostRuntime {
    runtime_path: String,
    processes: Arc<Mutex<HashMap<String, Child>>>,
}

impl HostRuntime {
    pub fn new(runtime_path: String) -> Self {
        Self {
            runtime_path,
            processes: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    async fn create_temp_wasm_file(&self, id: &str, wasm_binary: &[u8]) -> Result<PathBuf> {
        let temp_dir = std::env::temp_dir();
        let file_path = temp_dir.join(format!("proplet_{}.wasm", id));

        info!(
            "Creating temp wasm file: {:?}, size: {} bytes",
            file_path,
            wasm_binary.len()
        );

        let mut file = fs::File::create(&file_path)
            .await
            .context("Failed to create temporary wasm file")?;

        info!("File created, writing {} bytes", wasm_binary.len());

        file.write_all(wasm_binary)
            .await
            .context("Failed to write wasm binary to temp file")?;

        info!("Write complete, flushing");

        file.flush().await?;

        info!("File flushed successfully: {:?}", file_path);
        Ok(file_path)
    }

    async fn cleanup_temp_file(&self, file_path: PathBuf) -> Result<()> {
        if file_path.exists() {
            fs::remove_file(&file_path)
                .await
                .context("Failed to remove temporary wasm file")?;
            debug!("Cleaned up temporary file: {:?}", file_path);
        }
        Ok(())
    }
}

#[async_trait]
impl Runtime for HostRuntime {
    async fn start_app(
        &self,
        _ctx: RuntimeContext,
        wasm_binary: Vec<u8>,
        cli_args: Vec<String>,
        id: String,
        function_name: String,
        daemon: bool,
        env: HashMap<String, String>,
        args: Vec<u64>,
    ) -> Result<Vec<u8>> {
        info!(
            "Starting Host runtime app: task_id={}, function={}, daemon={}, wasm_size={}",
            id, function_name, daemon, wasm_binary.len()
        );

        info!("About to create temp wasm file for task: {}", id);

        // Create temporary wasm file
        let temp_file = self.create_temp_wasm_file(&id, &wasm_binary).await?;

        info!("Temp file created successfully: {:?}", temp_file);

        // Build command
        info!("Building command with runtime_path: {}", self.runtime_path);
        let mut cmd = Command::new(&self.runtime_path);

        // Add CLI arguments first (e.g., --invoke add)
        for arg in &cli_args {
            cmd.arg(arg);
        }

        // Add wasm file
        cmd.arg(&temp_file);

        // Add numeric parameters as additional arguments
        for arg in &args {
            cmd.arg(arg.to_string());
        }

        // Set environment variables
        cmd.envs(&env);

        // Configure stdio
        cmd.stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .stdin(Stdio::null());

        // Log the full command being executed
        info!(
            "Command to execute: {} {} {:?} {:?}",
            self.runtime_path,
            temp_file.display(),
            cli_args,
            args
        );

        info!("About to spawn command for task: {}", id);

        // Spawn the process
        let child = cmd.spawn().context(format!(
            "Failed to spawn host runtime process: {}. Command: {} {:?}",
            self.runtime_path,
            self.runtime_path,
            cli_args
        ))?;

        info!("Process spawned with PID: {:?}", child.id());

        if daemon {
            info!("Running in daemon mode for task: {}", id);
            // For daemon mode, store the process and return immediately
            let mut processes = self.processes.lock().await;
            processes.insert(id.clone(), child);

            // Spawn a background task to wait for the process and cleanup
            let processes = self.processes.clone();
            let temp_file_clone = temp_file.clone();
            let task_id = id.clone();

            tokio::spawn(async move {
                // Wait for the process to complete
                if let Some(mut process) = processes.lock().await.remove(&task_id) {
                    match process.wait().await {
                        Ok(status) => {
                            info!("Daemon task {} exited with status: {}", task_id, status);
                        }
                        Err(e) => {
                            error!("Daemon task {} wait error: {}", task_id, e);
                        }
                    }
                }

                // Cleanup temp file
                let _ = fs::remove_file(temp_file_clone).await;
            });

            info!("Daemon task {} started, returning immediately", id);
            Ok(Vec::new())
        } else {
            info!("Running in synchronous mode, waiting for task: {}", id);
            // Wait for process to complete
            let output = child
                .wait_with_output()
                .await
                .context("Failed to wait for host runtime process")?;

            info!("Process completed for task: {}", id);

            // Cleanup temp file
            self.cleanup_temp_file(temp_file).await?;

            if !output.status.success() {
                let stderr = String::from_utf8_lossy(&output.stderr);
                error!("Task {} failed with stderr: {}", id, stderr);
                return Err(anyhow::anyhow!(
                    "Process exited with status: {}, stderr: {}",
                    output.status,
                    stderr
                ));
            }

            // Print stdout and stderr
            let stdout_str = String::from_utf8_lossy(&output.stdout);
            let stderr_str = String::from_utf8_lossy(&output.stderr);

            info!(
                "Task {} completed successfully, output size: {} bytes, exit status: {}",
                id,
                output.stdout.len(),
                output.status
            );

            // Always print stdout/stderr even if empty to help debug
            info!("Task {} stdout: '{}'", id, stdout_str);
            info!("Task {} stderr: '{}'", id, stderr_str);

            // Also print raw bytes if output is not empty
            if !output.stdout.is_empty() {
                info!("Task {} stdout (raw bytes): {:?}", id, output.stdout);
            }

            Ok(output.stdout)
        }
    }

    async fn stop_app(&self, id: String) -> Result<()> {
        info!("Stopping Host runtime app: task_id={}", id);

        let mut processes = self.processes.lock().await;
        if let Some(mut child) = processes.remove(&id) {
            child.kill().await.context("Failed to kill process")?;
            debug!("Process for task {} killed", id);
            Ok(())
        } else {
            Err(anyhow::anyhow!("Task {} not found", id))
        }
    }
}

