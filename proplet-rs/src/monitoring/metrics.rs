use serde::{Deserialize, Serialize};
use std::time::SystemTime;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessMetrics {
    #[serde(rename = "cpu_percent")]
    pub cpu_usage_percent: f64,
    #[serde(rename = "memory_bytes")]
    pub memory_usage_bytes: u64,
    #[serde(rename = "memory_percent")]
    pub memory_usage_percent: f64,
    pub disk_read_bytes: u64,
    pub disk_write_bytes: u64,
    pub network_rx_bytes: u64,
    pub network_tx_bytes: u64,
    pub uptime_seconds: u64,
    pub thread_count: u32,
    pub file_descriptor_count: u32,
    #[serde(with = "systemtime_as_rfc3339")]
    pub timestamp: SystemTime,
}

mod systemtime_as_rfc3339 {
    use serde::{self, Deserialize, Deserializer, Serializer};
    use std::time::{SystemTime, UNIX_EPOCH};

    pub fn serialize<S>(time: &SystemTime, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        match time.duration_since(UNIX_EPOCH) {
            Ok(duration) => {
                let secs = duration.as_secs();
                let nanos = duration.subsec_nanos();
                // RFC3339 format
                let datetime = chrono::DateTime::from_timestamp(secs as i64, nanos)
                    .ok_or_else(|| serde::ser::Error::custom("Invalid timestamp"))?;
                serializer.serialize_str(&datetime.to_rfc3339())
            }
            Err(_) => serializer.serialize_str("0001-01-01T00:00:00Z"),
        }
    }

    pub fn deserialize<'de, D>(deserializer: D) -> Result<SystemTime, D::Error>
    where
        D: Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        let datetime = chrono::DateTime::parse_from_rfc3339(&s)
            .map_err(serde::de::Error::custom)?;
        let secs = datetime.timestamp() as u64;
        let nanos = datetime.timestamp_subsec_nanos();
        Ok(UNIX_EPOCH + std::time::Duration::new(secs, nanos))
    }
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

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricsSnapshot {
    pub process: ProcessMetrics,
    pub aggregated: Option<AggregatedMetrics>,
}
