/// Information about the task being executed.
#[derive(Clone, Debug)]
pub struct TaskInfo {
    pub id: String,
    pub name: String,
    pub image_url: String,
    pub cli_args: Vec<String>,
    pub env: Vec<(String, String)>,
    pub daemon: bool,
    pub encrypted: bool,
}

/// Outcome of a completed task.
#[derive(Clone, Debug)]
pub struct TaskResult {
    pub task_id: String,
    pub success: bool,
    pub output: Option<String>,
    pub error: Option<String>,
}

/// Result of an authorize call.
#[derive(Clone, Debug)]
pub struct AuthorizeResponse {
    pub allow: bool,
    pub reason: Option<String>,
}

/// Additional data a plugin injects before task execution.
#[derive(Clone, Debug)]
pub struct EnrichResponse {
    pub env: Vec<(String, String)>,
}

/// Implement this trait to define plugin behaviour.
///
/// All methods have default no-op / allow-all implementations, so you only
/// override what you need.
pub trait Plugin: Default + 'static {
    fn authorize(&self, task: &TaskInfo) -> AuthorizeResponse {
        let _ = task;
        AuthorizeResponse {
            allow: true,
            reason: None,
        }
    }

    fn enrich(&self, task: &TaskInfo) -> EnrichResponse {
        let _ = task;
        EnrichResponse { env: Vec::new() }
    }

    fn on_task_start(&self, task: &TaskInfo) {
        let _ = task;
    }

    fn on_task_complete(&self, result: &TaskResult) {
        let _ = result;
    }
}
