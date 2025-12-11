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
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub cli_args: Vec<String>,
    pub name: String,
    #[serde(default)]
    pub state: u8,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub file: String,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub image_url: String,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub inputs: Vec<u64>,
    #[serde(default)]
    pub daemon: bool,
    #[serde(default)]
    pub env: Option<HashMap<String, String>>,
}

// Helper function to deserialize null as default value
/// Deserializes a value treating JSON `null` as the type's default.

///

/// This helper can be used with `#[serde(default, deserialize_with = "deserialize_null_default")]`

/// to map an explicit `null` in the input to `T::default()`.

///

/// # Examples

///

/// ```

/// use serde::Deserialize;

///

/// #[derive(Deserialize)]

/// struct S {

///     #[serde(default, deserialize_with = "deserialize_null_default")]

///     v: Vec<i32>,

/// }

///

/// let s: S = serde_json::from_str(r#"{"v": null}"#).unwrap();

/// assert!(s.v.is_empty());

/// ```
fn deserialize_null_default<'de, D, T>(deserializer: D) -> std::result::Result<T, D::Error>
where
    T: Default + Deserialize<'de>,
    D: serde::Deserializer<'de>,
{
    let opt = Option::deserialize(deserializer)?;
    Ok(opt.unwrap_or_default())
}

impl StartRequest {
    /// Validates that the start request contains required identifying and payload fields.
    ///
    /// Returns `Ok(())` when `id` and `name` are non-empty and at least one of `file` or `image_url` is provided.
    /// Returns an `Err(anyhow::Error)` with one of the following messages when validation fails:
    /// - `"id is required"` if `id` is empty.
    /// - `"function name is required"` if `name` is empty.
    /// - `"either file or image_url must be provided"` if both `file` and `image_url` are empty.
    ///
    /// # Examples
    ///
    /// ```rust
    /// use anyhow::Result;
    ///
    /// let req = StartRequest {
    ///     id: "req-1".to_string(),
    ///     cli_args: Vec::new(),
    ///     name: "my_func".to_string(),
    ///     state: 0,
    ///     file: "handler.wasm".to_string(),
    ///     image_url: String::new(),
    ///     inputs: Vec::new(),
    ///     daemon: false,
    ///     env: None,
    /// };
    ///
    /// assert!(req.validate().is_ok());
    /// ```
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
    /// Validate that the stop request contains a non-empty id.
    ///
    /// Returns `Ok(())` when `id` is non-empty, otherwise returns an `Err` indicating
    /// that the `id` is required.
    ///
    /// # Examples
    ///
    /// ```
    /// let req = StopRequest { id: "proplet-123".to_string() };
    /// assert!(req.validate().is_ok());
    /// ```
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
    #[serde(deserialize_with = "deserialize_base64")]
    pub data: Vec<u8>,
}

// Helper function to deserialize base64 string to Vec<u8>
/// Deserializes a base64-encoded string into raw bytes.
///
/// Decodes a base64 `String` produced by the deserializer using the standard
/// base64 alphabet and returns the resulting `Vec<u8>` or a serde deserialization
/// error when decoding fails.
///
/// # Examples
///
/// ```
/// use serde::Deserialize;
///
/// #[derive(Deserialize)]
/// struct S {
///     #[serde(deserialize_with = "crate::deserialize_base64")]
///     data: Vec<u8>,
/// }
///
/// let json = r#"{ "data": "aGVsbG8=" }"#; // "hello"
/// let s: S = serde_json::from_str(json).unwrap();
/// assert_eq!(s.data, b"hello");
/// ```
fn deserialize_base64<'de, D>(deserializer: D) -> std::result::Result<Vec<u8>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    use base64::{engine::general_purpose::STANDARD, Engine};
    use serde::de::Error;

    let s = String::deserialize(deserializer)?;
    STANDARD.decode(&s).map_err(Error::custom)
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