pub mod metrics;
pub mod profiles;
pub mod system;

use crate::types::MonitoringProfile;
use anyhow::Result;
use async_trait::async_trait;
use uuid::Uuid;

#[async_trait]
pub trait ProcessMonitor: Send + Sync {
    async fn start_monitoring(&self, task_id: Uuid, profile: MonitoringProfile) -> Result<()>;
    async fn stop_monitoring(&self, task_id: Uuid) -> Result<()>;
    async fn get_metrics(&self, task_id: Uuid) -> Result<metrics::ProcessMetrics>;
}

pub fn create_monitor(profile: MonitoringProfile) -> Box<dyn ProcessMonitor> {
    Box::new(system::SystemMonitor::new(profile))
}
