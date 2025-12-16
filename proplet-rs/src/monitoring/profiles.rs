use crate::types::MonitoringProfile;
use std::time::Duration;

impl MonitoringProfile {
    pub fn minimal() -> Self {
        Self {
            enabled: true,
            interval: Duration::from_secs(60),
            collect_cpu: true,
            collect_memory: true,
            collect_disk_io: false,
            collect_network_io: false,
            collect_threads: false,
            collect_file_descriptors: false,
            export_to_mqtt: false,
            retain_history: false,
            history_size: 0,
        }
    }

    pub fn standard() -> Self {
        Self {
            enabled: true,
            interval: Duration::from_secs(10),
            collect_cpu: true,
            collect_memory: true,
            collect_disk_io: true,
            collect_network_io: true,
            collect_threads: true,
            collect_file_descriptors: true,
            export_to_mqtt: true,
            retain_history: true,
            history_size: 100,
        }
    }

    pub fn intensive() -> Self {
        Self {
            enabled: true,
            interval: Duration::from_secs(1),
            collect_cpu: true,
            collect_memory: true,
            collect_disk_io: true,
            collect_network_io: true,
            collect_threads: true,
            collect_file_descriptors: true,
            export_to_mqtt: true,
            retain_history: true,
            history_size: 1000,
        }
    }

    pub fn batch_processing() -> Self {
        Self {
            enabled: true,
            interval: Duration::from_secs(30),
            collect_cpu: true,
            collect_memory: true,
            collect_disk_io: true,
            collect_network_io: false,
            collect_threads: false,
            collect_file_descriptors: false,
            export_to_mqtt: true,
            retain_history: true,
            history_size: 200,
        }
    }

    pub fn real_time_api() -> Self {
        Self {
            enabled: true,
            interval: Duration::from_secs(5),
            collect_cpu: true,
            collect_memory: true,
            collect_disk_io: false,
            collect_network_io: true,
            collect_threads: true,
            collect_file_descriptors: true,
            export_to_mqtt: true,
            retain_history: true,
            history_size: 500,
        }
    }

    pub fn long_running_daemon() -> Self {
        Self {
            enabled: true,
            interval: Duration::from_secs(120),
            collect_cpu: true,
            collect_memory: true,
            collect_disk_io: true,
            collect_network_io: true,
            collect_threads: true,
            collect_file_descriptors: true,
            export_to_mqtt: true,
            retain_history: true,
            history_size: 500,
        }
    }

    pub fn disabled() -> Self {
        Self {
            enabled: false,
            interval: Duration::from_secs(60),
            collect_cpu: false,
            collect_memory: false,
            collect_disk_io: false,
            collect_network_io: false,
            collect_threads: false,
            collect_file_descriptors: false,
            export_to_mqtt: false,
            retain_history: false,
            history_size: 0,
        }
    }
}
