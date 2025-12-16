use super::metrics::ProcessMetrics;
use super::ProcessMonitor;
use crate::types::MonitoringProfile;
use anyhow::Result;
use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::Arc;
use std::time::SystemTime;
use sysinfo::{Pid, ProcessRefreshKind, ProcessesToUpdate, System};
use tokio::sync::Mutex;
use tokio::time::interval;
use tracing::{debug, warn};
use uuid::Uuid;

struct MonitoredTask {
    pid: Option<u32>,
    profile: MonitoringProfile,
    metrics_history: Vec<ProcessMetrics>,
    start_time: SystemTime,
}

pub struct SystemMonitor {
    tasks: Arc<Mutex<HashMap<Uuid, MonitoredTask>>>,
    system: Arc<Mutex<System>>,
    default_profile: MonitoringProfile,
}

impl SystemMonitor {
    pub fn new(default_profile: MonitoringProfile) -> Self {
        Self {
            tasks: Arc::new(Mutex::new(HashMap::new())),
            system: Arc::new(Mutex::new(System::new_all())),
            default_profile,
        }
    }

    async fn collect_process_metrics(
        sys: &mut System,
        pid: u32,
        profile: &MonitoringProfile,
    ) -> Result<ProcessMetrics> {
        let pid = Pid::from_u32(pid);

        sys.refresh_processes_specifics(
            ProcessesToUpdate::Some(&[pid]),
            true,
            ProcessRefreshKind::new()
                .with_cpu()
                .with_memory()
                .with_disk_usage(),
        );

        let process = sys
            .process(pid)
            .ok_or_else(|| anyhow::anyhow!("Process not found"))?;

        let mut metrics = ProcessMetrics::default();
        metrics.timestamp = SystemTime::now();

        if profile.collect_cpu {
            metrics.cpu_usage_percent = process.cpu_usage() as f64;
        }

        if profile.collect_memory {
            metrics.memory_usage_bytes = process.memory();
        }

        if profile.collect_disk_io {
            let disk_usage = process.disk_usage();
            metrics.disk_read_bytes = disk_usage.total_read_bytes;
            metrics.disk_write_bytes = disk_usage.total_written_bytes;
        }

        if profile.collect_threads {
            #[cfg(not(target_os = "windows"))]
            {
                metrics.thread_count = sys
                    .process(pid)
                    .and_then(|p| p.tasks())
                    .map(|t| t.len() as u32)
                    .unwrap_or(0);
            }
            #[cfg(target_os = "windows")]
            {
                metrics.thread_count = 1;
            }
        }

        if profile.collect_file_descriptors {
            #[cfg(target_os = "linux")]
            {
                if let Ok(entries) = std::fs::read_dir(format!("/proc/{}/fd", pid.as_u32())) {
                    metrics.file_descriptor_count = entries.count() as u32;
                }
            }
            #[cfg(target_os = "macos")]
            {
                use std::process::Command;
                if let Ok(output) = Command::new("lsof")
                    .args(["-p", &pid.as_u32().to_string()])
                    .output()
                {
                    metrics.file_descriptor_count =
                        output.stdout.split(|&b| b == b'\n').count() as u32;
                }
            }
            #[cfg(target_os = "windows")]
            {
                metrics.file_descriptor_count = 0;
            }
        }

        Ok(metrics)
    }

    async fn monitor_task(
        task_id: Uuid,
        pid: u32,
        profile: MonitoringProfile,
        tasks: Arc<Mutex<HashMap<Uuid, MonitoredTask>>>,
        system: Arc<Mutex<System>>,
    ) {
        let mut interval = interval(profile.interval);

        loop {
            interval.tick().await;

            let should_continue = {
                let tasks_guard = tasks.lock().await;
                tasks_guard.contains_key(&task_id)
            };

            if !should_continue {
                debug!("Stopping monitoring for task {}", task_id);
                break;
            }

            let mut sys = system.lock().await;
            match Self::collect_process_metrics(&mut sys, pid, &profile).await {
                Ok(metrics) => {
                    let mut tasks_guard = tasks.lock().await;
                    if let Some(task) = tasks_guard.get_mut(&task_id) {
                        if profile.retain_history {
                            task.metrics_history.push(metrics.clone());
                            if task.metrics_history.len() > profile.history_size {
                                task.metrics_history.remove(0);
                            }
                        }

                        debug!(
                            "Task {} metrics: CPU={:.2}%, MEM={} bytes",
                            task_id, metrics.cpu_usage_percent, metrics.memory_usage_bytes
                        );
                    }
                }
                Err(e) => {
                    warn!("Failed to collect metrics for task {}: {}", task_id, e);
                    let mut tasks_guard = tasks.lock().await;
                    tasks_guard.remove(&task_id);
                    break;
                }
            }
        }
    }
}

#[async_trait]
impl ProcessMonitor for SystemMonitor {
    async fn start_monitoring(&self, task_id: Uuid, profile: MonitoringProfile) -> Result<()> {
        if !profile.enabled {
            debug!("Monitoring disabled for task {}", task_id);
            return Ok(());
        }

        debug!("Starting monitoring for task {}", task_id);

        let mut tasks = self.tasks.lock().await;
        tasks.insert(
            task_id,
            MonitoredTask {
                pid: None,
                profile: profile.clone(),
                metrics_history: Vec::new(),
                start_time: SystemTime::now(),
            },
        );

        Ok(())
    }

    async fn stop_monitoring(&self, task_id: Uuid) -> Result<()> {
        debug!("Stopping monitoring for task {}", task_id);
        let mut tasks = self.tasks.lock().await;
        tasks.remove(&task_id);
        Ok(())
    }

    async fn get_metrics(&self, task_id: Uuid) -> Result<ProcessMetrics> {
        let tasks = self.tasks.lock().await;
        let task = tasks
            .get(&task_id)
            .ok_or_else(|| anyhow::anyhow!("Task not found"))?;

        task.metrics_history
            .last()
            .cloned()
            .ok_or_else(|| anyhow::anyhow!("No metrics available"))
    }
}

pub async fn attach_pid_to_monitor(
    tasks: Arc<Mutex<HashMap<Uuid, MonitoredTask>>>,
    system: Arc<Mutex<System>>,
    task_id: Uuid,
    pid: u32,
) {
    let profile = {
        let mut tasks_guard = tasks.lock().await;
        if let Some(task) = tasks_guard.get_mut(&task_id) {
            task.pid = Some(pid);
            task.profile.clone()
        } else {
            return;
        }
    };

    tokio::spawn(async move {
        SystemMonitor::monitor_task(task_id, pid, profile, tasks, system).await;
    });
}
