use anyhow::Result;
use http_body_util::Full;
use hyper::body::{Bytes, Incoming};
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{Request, Response};
use hyper_util::rt::{TokioIo, TokioTimer};
use prometheus::{Encoder, Gauge, IntCounter, IntGauge, Opts, Registry, TextEncoder};
use std::sync::Arc;
use tokio::net::TcpListener;

#[allow(clippy::new_without_default)]
pub struct PropletMetrics {
    registry: Registry,
    pub tasks_started: IntCounter,
    pub tasks_completed: IntCounter,
    pub tasks_failed: IntCounter,
    pub tasks_running: IntGauge,
    pub mqtt_reconnects: IntCounter,
    pub wasm_fetch_bytes: IntCounter,
    pub cpu_usage: Gauge,
    pub memory_rss_bytes: Gauge,
}

impl PropletMetrics {
    pub fn new() -> Result<Self> {
        let registry = Registry::new();

        let tasks_started = IntCounter::with_opts(Opts::new(
            "proplet_tasks_started_total",
            "Total tasks received and started",
        ))?;
        registry.register(Box::new(tasks_started.clone()))?;

        let tasks_completed = IntCounter::with_opts(Opts::new(
            "proplet_tasks_completed_total",
            "Tasks completed successfully",
        ))?;
        registry.register(Box::new(tasks_completed.clone()))?;

        let tasks_failed = IntCounter::with_opts(Opts::new(
            "proplet_tasks_failed_total",
            "Tasks that errored or were rejected",
        ))?;
        registry.register(Box::new(tasks_failed.clone()))?;

        let tasks_running = IntGauge::with_opts(Opts::new(
            "proplet_tasks_running",
            "Tasks currently executing",
        ))?;
        registry.register(Box::new(tasks_running.clone()))?;

        let mqtt_reconnects = IntCounter::with_opts(Opts::new(
            "proplet_mqtt_reconnects_total",
            "MQTT reconnection events",
        ))?;
        registry.register(Box::new(mqtt_reconnects.clone()))?;

        let wasm_fetch_bytes = IntCounter::with_opts(Opts::new(
            "proplet_wasm_fetch_bytes_total",
            "Bytes fetched for wasm binaries",
        ))?;
        registry.register(Box::new(wasm_fetch_bytes.clone()))?;

        let cpu_usage = Gauge::with_opts(Opts::new(
            "proplet_cpu_usage_ratio",
            "CPU usage ratio (0.0–1.0)",
        ))?;
        registry.register(Box::new(cpu_usage.clone()))?;

        let memory_rss_bytes = Gauge::with_opts(Opts::new(
            "proplet_memory_rss_bytes",
            "Process RSS memory in bytes",
        ))?;
        registry.register(Box::new(memory_rss_bytes.clone()))?;

        Ok(Self {
            registry,
            tasks_started,
            tasks_completed,
            tasks_failed,
            tasks_running,
            mqtt_reconnects,
            wasm_fetch_bytes,
            cpu_usage,
            memory_rss_bytes,
        })
    }
}

pub async fn serve_telemetry(port: u16, metrics: Arc<PropletMetrics>) {
    let listener = match TcpListener::bind(format!("0.0.0.0:{port}")).await {
        Ok(l) => l,
        Err(e) => {
            tracing::error!("Failed to bind telemetry server on port {port}: {e}");
            return;
        }
    };
    tracing::info!("Telemetry server listening on 0.0.0.0:{port}");

    loop {
        match listener.accept().await {
            Ok((stream, _)) => {
                let metrics = metrics.clone();
                tokio::spawn(async move {
                    let io = TokioIo::new(stream);
                    let svc = service_fn(move |req: Request<Incoming>| {
                        let metrics = metrics.clone();
                        async move { handle(req, metrics).await }
                    });
                    if let Err(e) = http1::Builder::new()
                        .keep_alive(true)
                        .timer(TokioTimer::new())
                        .serve_connection(io, svc)
                        .await
                    {
                        tracing::warn!("Telemetry connection error: {e}");
                    }
                });
            }
            Err(e) => tracing::warn!("Telemetry accept error: {e}"),
        }
    }
}

async fn handle(
    req: Request<Incoming>,
    metrics: Arc<PropletMetrics>,
) -> Result<Response<Full<Bytes>>, std::convert::Infallible> {
    match req.uri().path() {
        "/metrics" => {
            let encoder = TextEncoder::new();
            let mut buf = Vec::new();
            if let Err(e) = encoder.encode(&metrics.registry.gather(), &mut buf) {
                tracing::error!("Failed to encode metrics: {e}");
                return Ok(Response::builder()
                    .status(500)
                    .body(Full::new(Bytes::from("metrics encoding error")))
                    .unwrap());
            }
            Ok(Response::builder()
                .header("Content-Type", encoder.format_type())
                .body(Full::new(Bytes::from(buf)))
                .unwrap())
        }
        "/health" => {
            let running = metrics.tasks_running.get();
            let body = format!(r#"{{"status":"ok","running_tasks":{running}}}"#);
            Ok(Response::builder()
                .header("Content-Type", "application/json")
                .body(Full::new(Bytes::from(body)))
                .unwrap())
        }
        _ => Ok(Response::builder()
            .status(404)
            .body(Full::new(Bytes::from("not found")))
            .unwrap()),
    }
}
