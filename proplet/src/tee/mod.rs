pub mod detector;

pub use detector::{TeeDetector, TeeType};

/// Convenience function to detect TEE type
pub fn detect_tee_type() -> TeeType {
    TeeDetector::detect()
}

/// Convenience function to describe environment
pub fn describe_environment() -> String {
    let tee_type = detect_tee_type();
    format!("TEE Detection: {} - {}", 
        tee_type.as_str(),
        if tee_type.is_tee() { "Running in trusted execution environment" } else { "Running on regular hardware" }
    )
}
