use anyhow::Result;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::time::SystemTime;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum TaskState {
    Running,
    Completed,
    Failed,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Proplet {
    pub id: Uuid,
    pub name: String,
    pub task_count: usize,
    pub alive: bool,
    pub alive_history: Vec<SystemTime>,
}

impl Proplet {
    pub fn new(id: Uuid, name: String) -> Self {
        Self {
            id,
            name,
            task_count: 0,
            alive: false,
            alive_history: Vec::new(),
        }
    }

    pub fn set_alive(&mut self, alive: bool) {
        self.alive = alive;
        if alive {
            self.alive_history.push(SystemTime::now());
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StartRequest {
    pub id: String,
    #[serde(default)]
    pub cli_args: Vec<String>,
    pub name: String,
    #[serde(default)]
    pub state: u8,
    #[serde(default)]
    pub file: String,
    #[serde(default)]
    pub image_url: String,
    #[serde(default)]
    pub inputs: Vec<u64>,
    #[serde(default)]
    pub daemon: bool,
    #[serde(default)]
    pub env: Option<HashMap<String, String>>,
}

impl StartRequest {
    pub fn validate(&self) -> Result<()> {
        if self.id.is_empty() {
            return Err(anyhow::anyhow!("id is required"));
        }
        if self.name.is_empty() {
            return Err(anyhow::anyhow!("function name is required"));
        }
        if self.file.is_empty() && self.image_url.is_empty() {
            return Err(anyhow::anyhow!("either file or image_url must be provided"));
        }
        Ok(())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StopRequest {
    pub id: String,
}

impl StopRequest {
    pub fn validate(&self) -> Result<()> {
        if self.id.is_empty() {
            return Err(anyhow::anyhow!("id is required"));
        }

        Ok(())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Task {
    pub id: String,
    pub name: String,
    pub state: u8,
    pub image_url: String,
    pub file: Vec<u8>,
    pub cli_args: Vec<String>,
    pub inputs: Vec<u64>,
    pub env: HashMap<String, String>,
    pub daemon: bool,
    pub proplet_id: String,
    pub results: Value,
    pub error: String,
    pub start_time: SystemTime,
    pub finish_time: SystemTime,
    pub created_at: SystemTime,
    pub updated_at: SystemTime,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChunkMetadata {
    pub app_name: String,
    pub total_chunks: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Chunk {
    pub app_name: String,
    pub chunk_idx: usize,
    pub total_chunks: usize,
    pub data: Vec<u8>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LivelinessMessage {
    pub proplet_id: String,
    pub status: String,
    pub namespace: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DiscoveryMessage {
    pub proplet_id: String,
    pub namespace: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResultMessage {
    pub task_id: String,
    pub proplet_id: Uuid,
    pub result: Vec<u8>,
    pub error: Option<String>,
}
