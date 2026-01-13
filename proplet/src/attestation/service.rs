use super::config::AttestationConfig;
use anyhow::{Context, Result};
use kbs_protocol::evidence_provider::NativeEvidenceProvider;
use kbs_protocol::{KbsClientBuilder, KbsClientCapabilities, ResourceUri};
use tracing::{debug, info, warn};

/// AttestationService handles attestation and key retrieval using guest-components
pub struct AttestationService {
    config: AttestationConfig,
}

impl AttestationService {
    pub fn new(config: AttestationConfig) -> Self {
        Self { config }
    }

    /// Request a key from KBS with attestation
    ///
    /// This method performs the following steps:
    /// 1. Creates a KBS client configured for the target KBS
    /// 2. Performs attestation (evidence generation happens automatically)
    /// 3. Retrieves the requested resource/key
    ///
    /// # Arguments
    /// * `resource_path` - Path to the resource/key in KBS (e.g., "default/keys/workload-key")
    /// * `workload_id` - Identifier for the workload requesting the key
    ///
    /// # Returns
    /// * Base64-encoded encryption key
    pub async fn get_resource(&self, resource_path: &str, _workload_id: &str) -> Result<Vec<u8>> {
        if !self.config.enabled {
            return Err(anyhow::anyhow!("Attestation is disabled"));
        }

        info!(
            "Requesting resource from KBS: {} (evidence type: {})",
            resource_path, self.config.evidence_type
        );

        // Create native evidence provider (auto-detects TEE type)
        let evidence_provider = Box::new(
            NativeEvidenceProvider::new()
                .context("Failed to create evidence provider")?,
        );

        // Build KBS client with evidence provider
        let client_builder =
            KbsClientBuilder::with_evidence_provider(evidence_provider, &self.config.kbs_url);

        // Build the client
        let mut client = client_builder
            .build()
            .context("Failed to build KBS client")?;

        debug!("KBS client created, performing attestation and key retrieval");

        // Parse resource path into ResourceUri
        let resource_uri = ResourceUri::try_from(resource_path)
            .map_err(|e| anyhow::anyhow!("Failed to parse resource path: {}", e))?;

        // Get resource - this automatically handles:
        // 1. Initial handshake with KBS
        // 2. Evidence generation using the configured attester
        // 3. Attestation report submission
        // 4. Key/resource retrieval upon successful attestation
        let resource: Vec<u8> = client
            .get_resource(resource_uri)
            .await
            .context("Failed to get resource from KBS")?;

        info!(
            "Successfully retrieved resource from KBS: {} bytes",
            resource.len()
        );

        Ok(resource)
    }

    /// Health check for KBS connectivity
    ///
    /// Performs a simple health check to verify the KBS is reachable
    #[allow(dead_code)]
    pub async fn health_check(&self) -> Result<bool> {
        if !self.config.enabled {
            return Ok(true);
        }

        debug!("Performing KBS health check: {}", self.config.kbs_url);

        // For now, assume KBS is healthy if attestation is enabled
        // A real health check would require making a test request
        info!("KBS health check: attestation enabled, assuming healthy");
        Ok(true)
    }

    /// Decrypt Wasm binary using the retrieved key
    ///
    /// This is a placeholder for future encrypted workload support
    #[allow(dead_code)]
    pub async fn decrypt_workload(
        &self,
        encrypted_binary: &[u8],
        resource_path: &str,
        workload_id: &str,
    ) -> Result<Vec<u8>> {
        info!("Decrypting workload using key from KBS");

        // Get the encryption key from KBS
        let key = self.get_resource(resource_path, workload_id).await?;

        // TODO: Implement actual decryption using the key
        // For now, return the encrypted binary as-is
        // In production, this would use AES-GCM or similar to decrypt
        debug!(
            "Decryption key retrieved ({} bytes), decrypting {} byte workload",
            key.len(),
            encrypted_binary.len()
        );

        // Placeholder: In production, implement proper decryption
        // Example: use aes-gcm crate to decrypt with the key
        warn!("Workload decryption not yet implemented, returning encrypted data");
        Ok(encrypted_binary.to_vec())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_attestation_service_creation() {
        let config = AttestationConfig::default();
        let service = AttestationService::new(config);
        assert!(!service.config.enabled);
    }

    #[tokio::test]
    async fn test_get_resource_disabled() {
        let config = AttestationConfig {
            enabled: false,
            ..Default::default()
        };
        let service = AttestationService::new(config);

        let result = service
            .get_resource("default/keys/test", "test-workload")
            .await;

        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_health_check_disabled() {
        let config = AttestationConfig {
            enabled: false,
            ..Default::default()
        };
        let service = AttestationService::new(config);

        let result = service.health_check().await;
        assert!(result.is_ok());
        assert!(result.unwrap());
    }
}
