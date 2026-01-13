// Copyright (c) Abstract Machines

//! Cryptographic operations for confidential workloads
//!
//! This module provides encryption and decryption functionality for WebAssembly workloads
//! using AES-256-GCM authenticated encryption.

use aes_gcm::{
    aead::{Aead, AeadCore, KeyInit, OsRng},
    Aes256Gcm, Key, Nonce,
};
use anyhow::{anyhow, Context, Result};

/// Size of AES-256 key in bytes
pub const KEY_SIZE: usize = 32;

/// Size of nonce/IV in bytes (96 bits for GCM)
pub const NONCE_SIZE: usize = 12;

/// Encrypted workload format:
/// - First 12 bytes: Nonce/IV
/// - Remaining bytes: Ciphertext with authentication tag
pub struct EncryptedWorkload {
    pub nonce: [u8; NONCE_SIZE],
    pub ciphertext: Vec<u8>,
}

impl EncryptedWorkload {
    /// Parse an encrypted workload from bytes
    ///
    /// Expected format: [nonce (12 bytes)][ciphertext + tag]
    pub fn from_bytes(data: &[u8]) -> Result<Self> {
        if data.len() < NONCE_SIZE {
            return Err(anyhow!(
                "Encrypted workload too short: {} bytes (minimum {})",
                data.len(),
                NONCE_SIZE
            ));
        }

        let mut nonce = [0u8; NONCE_SIZE];
        nonce.copy_from_slice(&data[..NONCE_SIZE]);
        let ciphertext = data[NONCE_SIZE..].to_vec();

        Ok(Self { nonce, ciphertext })
    }

    /// Convert to bytes for storage
    #[allow(dead_code)]
    pub fn to_bytes(&self) -> Vec<u8> {
        let mut result = Vec::with_capacity(NONCE_SIZE + self.ciphertext.len());
        result.extend_from_slice(&self.nonce);
        result.extend_from_slice(&self.ciphertext);
        result
    }
}

/// Decrypt a workload using AES-256-GCM
///
/// # Arguments
/// * `encrypted_data` - The encrypted workload (nonce + ciphertext + tag)
/// * `key` - The 32-byte decryption key retrieved from KBS
///
/// # Returns
/// The decrypted WebAssembly binary
pub fn decrypt_workload(encrypted_data: &[u8], key: &[u8]) -> Result<Vec<u8>> {
    // Validate key size
    if key.len() != KEY_SIZE {
        return Err(anyhow!(
            "Invalid key size: {} bytes (expected {})",
            key.len(),
            KEY_SIZE
        ));
    }

    // Parse the encrypted workload
    let workload = EncryptedWorkload::from_bytes(encrypted_data)
        .context("Failed to parse encrypted workload")?;

    // Initialize cipher
    let key = Key::<Aes256Gcm>::from_slice(key);
    let cipher = Aes256Gcm::new(key);

    // Decrypt
    let nonce = Nonce::from_slice(&workload.nonce);
    let plaintext = cipher
        .decrypt(nonce, workload.ciphertext.as_ref())
        .map_err(|e| anyhow!("Decryption failed: {}", e))
        .context("Failed to decrypt workload - key mismatch or corrupted data")?;

    Ok(plaintext)
}

/// Encrypt a workload using AES-256-GCM
///
/// This is a utility function for creating encrypted workloads during development/testing.
///
/// # Arguments
/// * `plaintext` - The WebAssembly binary to encrypt
/// * `key` - The 32-byte encryption key
///
/// # Returns
/// The encrypted workload (nonce + ciphertext + tag)
#[allow(dead_code)]
pub fn encrypt_workload(plaintext: &[u8], key: &[u8]) -> Result<Vec<u8>> {
    // Validate key size
    if key.len() != KEY_SIZE {
        return Err(anyhow!(
            "Invalid key size: {} bytes (expected {})",
            key.len(),
            KEY_SIZE
        ));
    }

    // Initialize cipher
    let key = Key::<Aes256Gcm>::from_slice(key);
    let cipher = Aes256Gcm::new(key);

    // Generate random nonce
    let nonce = Aes256Gcm::generate_nonce(&mut OsRng);

    // Encrypt
    let ciphertext = cipher
        .encrypt(&nonce, plaintext)
        .map_err(|e| anyhow!("Encryption failed: {}", e))?;

    // Combine nonce + ciphertext
    let workload = EncryptedWorkload {
        nonce: nonce.into(),
        ciphertext,
    };

    Ok(workload.to_bytes())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_encrypt_decrypt_roundtrip() {
        let plaintext = b"Hello, confidential world!";
        let key = [0x42u8; KEY_SIZE];

        // Encrypt
        let encrypted = encrypt_workload(plaintext, &key).expect("encryption failed");
        assert!(encrypted.len() > plaintext.len()); // Should include nonce + tag

        // Decrypt
        let decrypted = decrypt_workload(&encrypted, &key).expect("decryption failed");
        assert_eq!(decrypted, plaintext);
    }

    #[test]
    fn test_decrypt_with_wrong_key() {
        let plaintext = b"Secret data";
        let key1 = [0x42u8; KEY_SIZE];
        let key2 = [0x43u8; KEY_SIZE];

        let encrypted = encrypt_workload(plaintext, &key1).expect("encryption failed");
        let result = decrypt_workload(&encrypted, &key2);

        assert!(result.is_err());
        let err_msg = result.unwrap_err().to_string();
        // The error could be either "Decryption failed" or "Failed to decrypt workload"
        assert!(
            err_msg.contains("Decryption failed") || err_msg.contains("Failed to decrypt"),
            "Expected decryption error but got: {}",
            err_msg
        );
    }

    #[test]
    fn test_invalid_key_size() {
        let plaintext = b"test";
        let short_key = [0x42u8; 16]; // Only 128-bit key

        let result = encrypt_workload(plaintext, &short_key);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("Invalid key size"));
    }

    #[test]
    fn test_encrypted_workload_too_short() {
        let short_data = [0u8; 5]; // Less than NONCE_SIZE
        let result = EncryptedWorkload::from_bytes(&short_data);
        assert!(result.is_err());
    }

    #[test]
    fn test_encrypted_workload_roundtrip() {
        let nonce = [0x42u8; NONCE_SIZE];
        let ciphertext = vec![1, 2, 3, 4, 5];

        let workload = EncryptedWorkload {
            nonce,
            ciphertext: ciphertext.clone(),
        };

        let bytes = workload.to_bytes();
        let parsed = EncryptedWorkload::from_bytes(&bytes).expect("parse failed");

        assert_eq!(parsed.nonce, nonce);
        assert_eq!(parsed.ciphertext, ciphertext);
    }
}
