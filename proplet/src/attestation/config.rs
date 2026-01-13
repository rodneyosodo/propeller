use serde::{Deserialize, Serialize};

/// Configuration for attestation functionality
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AttestationConfig {
    /// Enable attestation
    pub enabled: bool,
    /// KBS (Key Broker Service) URL
    pub kbs_url: String,
    /// Attestation evidence type (e.g., "tdx", "sgx", "snp", "sample")
    pub evidence_type: String,
    /// Timeout for attestation operations in seconds
    pub timeout_secs: u64,
}

impl Default for AttestationConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            kbs_url: "http://localhost:8080".to_string(),
            evidence_type: "sample".to_string(),
            timeout_secs: 30,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_attestation_config_default() {
        let config = AttestationConfig::default();
        assert!(!config.enabled);
        assert_eq!(config.kbs_url, "http://localhost:8080");
        assert_eq!(config.evidence_type, "sample");
        assert_eq!(config.timeout_secs, 30);
    }
}
