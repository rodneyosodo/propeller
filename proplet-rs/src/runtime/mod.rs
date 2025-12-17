pub mod host;
pub mod wasmtime_runtime;

use anyhow::Result;
use async_trait::async_trait;
use std::collections::HashMap;
use uuid::Uuid;

pub struct StartConfig {
    pub ctx: RuntimeContext,
    pub wasm_binary: Vec<u8>,
    pub cli_args: Vec<String>,
    pub id: Uuid,
    pub function_name: String,
    pub daemon: bool,
    pub env: HashMap<String, String>,
    pub args: Vec<f64>,
}

#[async_trait]
pub trait Runtime: Send + Sync {
    async fn start_app(&self, config: StartConfig) -> Result<Vec<u8>>;

    async fn stop_app(&self, id: Uuid) -> Result<()>;
}

#[derive(Clone)]
pub struct RuntimeContext {
    pub proplet_id: Uuid,
}
