// Copyright (c) Abstract Machines

//! Proplet - WebAssembly Runtime for Propeller
//!
//! This library provides the core functionality for running WebAssembly workloads
//! in the Propeller edge computing platform, with support for confidential computing
//! and remote attestation.

pub mod attestation;
pub mod config;
pub mod crypto;
pub mod mqtt;
pub mod runtime;
pub mod service;
pub mod tee;
pub mod types;

// Re-export commonly used types
pub use config::PropletConfig;
pub use service::PropletService;
pub use tee::{detect_tee_type, describe_environment, TeeType};
pub use types::{StartRequest, StopRequest, TaskState};
