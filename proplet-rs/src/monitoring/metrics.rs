use serde::{Deserialize, Serialize};
use std::time::SystemTime;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessMetrics {
    pub cpu_usage_percent: f64,
    pub memory_usage_bytes: u64,
    pub memory_usage_percent: f64,
    pub disk_read_bytes: u64,
    pub disk_write_bytes: u64,
    pub network_rx_bytes: u64,
    pub network_tx_bytes: u64,
    pub uptime_seconds: u64,
    pub thread_count: u32,
    pub file_descriptor_count: u32,
    pub timestamp: SystemTime,
}

impl Default for ProcessMetrics {
    fn default() -> Self {
        Self {
            cpu_usage_percent: 0.0,
            memory_usage_bytes: 0,
            memory_usage_percent: 0.0,
            disk_read_bytes: 0,
            disk_write_bytes: 0,
            network_rx_bytes: 0,
            network_tx_bytes: 0,
            uptime_seconds: 0,
            thread_count: 0,
            file_descriptor_count: 0,
            timestamp: SystemTime::now(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AggregatedMetrics {
    pub avg_cpu_usage: f64,
    pub max_cpu_usage: f64,
    pub avg_memory_usage: u64,
    pub max_memory_usage: u64,
    pub total_disk_read: u64,
    pub total_disk_write: u64,
    pub total_network_rx: u64,
    pub total_network_tx: u64,
    pub sample_count: usize,
}

impl AggregatedMetrics {
    pub fn from_samples(samples: &[ProcessMetrics]) -> Self {
        if samples.is_empty() {
            return Self {
                avg_cpu_usage: 0.0,
                max_cpu_usage: 0.0,
                avg_memory_usage: 0,
                max_memory_usage: 0,
                total_disk_read: 0,
                total_disk_write: 0,
                total_network_rx: 0,
                total_network_tx: 0,
                sample_count: 0,
            };
        }

        let count = samples.len() as f64;
        let avg_cpu = samples.iter().map(|s| s.cpu_usage_percent).sum::<f64>() / count;
        let max_cpu = samples
            .iter()
            .map(|s| s.cpu_usage_percent)
            .fold(0.0, f64::max);
        let avg_mem = (samples.iter().map(|s| s.memory_usage_bytes).sum::<u64>() as f64 / count) as u64;
        let max_mem = samples
            .iter()
            .map(|s| s.memory_usage_bytes)
            .max()
            .unwrap_or(0);

        let last = samples.last().unwrap();
        let first = samples.first().unwrap();

        Self {
            avg_cpu_usage: avg_cpu,
            max_cpu_usage: max_cpu,
            avg_memory_usage: avg_mem,
            max_memory_usage: max_mem,
            total_disk_read: last.disk_read_bytes.saturating_sub(first.disk_read_bytes),
            total_disk_write: last.disk_write_bytes.saturating_sub(first.disk_write_bytes),
            total_network_rx: last.network_rx_bytes.saturating_sub(first.network_rx_bytes),
            total_network_tx: last.network_tx_bytes.saturating_sub(first.network_tx_bytes),
            sample_count: samples.len(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricsSnapshot {
    pub process: ProcessMetrics,
    pub aggregated: Option<AggregatedMetrics>,
}
