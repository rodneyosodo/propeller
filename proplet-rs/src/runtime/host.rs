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


#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_host_runtime_new() {
        let runtime = HostRuntime::new("/usr/bin/wasmtime".to_string());
        assert_eq!(runtime.runtime_path, "/usr/bin/wasmtime");
    }

    #[test]
    fn test_temp_file_path_generation() {
        let temp_dir = std::env::temp_dir();
        let task_id = "task-123";
        let expected_path = temp_dir.join(format!("proplet_{}.wasm", task_id));

        assert!(expected_path.to_string_lossy().contains("proplet_task-123.wasm"));
    }

    #[test]
    fn test_temp_file_path_with_special_chars() {
        let temp_dir = std::env::temp_dir();
        let task_id = "task-with-dashes-123";
        let file_path = temp_dir.join(format!("proplet_{}.wasm", task_id));

        assert!(file_path.to_string_lossy().contains("proplet_task-with-dashes-123.wasm"));
    }

    #[tokio::test]
    async fn test_create_and_cleanup_temp_file() {
        let runtime = HostRuntime::new("/usr/bin/wasmtime".to_string());
        let task_id = "test-cleanup-task";
        let wasm_data = vec![0x00, 0x61, 0x73, 0x6d]; // WASM magic number

        // Create temp file
        let file_path = runtime.create_temp_wasm_file(task_id, &wasm_data).await.unwrap();
        
        // Verify file exists
        assert!(file_path.exists());

        // Verify file content
        let content = tokio::fs::read(&file_path).await.unwrap();
        assert_eq!(content, wasm_data);

        // Cleanup
        runtime.cleanup_temp_file(file_path.clone()).await.unwrap();
        
        // Verify file is removed
        assert!(!file_path.exists());
    }

    #[tokio::test]
    async fn test_cleanup_nonexistent_file() {
        let runtime = HostRuntime::new("/usr/bin/wasmtime".to_string());
        let fake_path = std::env::temp_dir().join("nonexistent-file.wasm");

        // Should not error when cleaning up non-existent file
        let result = runtime.cleanup_temp_file(fake_path).await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_create_temp_file_with_empty_data() {
        let runtime = HostRuntime::new("/usr/bin/wasmtime".to_string());
        let task_id = "empty-task";
        let wasm_data = vec![];

        let file_path = runtime.create_temp_wasm_file(task_id, &wasm_data).await.unwrap();
        
        assert!(file_path.exists());
        
        let content = tokio::fs::read(&file_path).await.unwrap();
        assert_eq!(content.len(), 0);

        runtime.cleanup_temp_file(file_path).await.unwrap();
    }

    #[tokio::test]
    async fn test_create_temp_file_with_large_data() {
        let runtime = HostRuntime::new("/usr/bin/wasmtime".to_string());
        let task_id = "large-task";
        let wasm_data = vec![0xAB; 1024 * 1024]; // 1 MB of data

        let file_path = runtime.create_temp_wasm_file(task_id, &wasm_data).await.unwrap();
        
        assert!(file_path.exists());
        
        let content = tokio::fs::read(&file_path).await.unwrap();
        assert_eq!(content.len(), 1024 * 1024);

        runtime.cleanup_temp_file(file_path).await.unwrap();
    }
}
