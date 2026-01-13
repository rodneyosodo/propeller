use std::fs;
use std::path::Path;
use tracing::{debug, info};

/// Supported TEE (Trusted Execution Environment) types
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TeeType {
    /// Intel Trust Domain Extensions
    Tdx,
    /// Intel Software Guard Extensions
    Sgx,
    /// AMD Secure Encrypted Virtualization - Secure Nested Paging
    Snp,
    /// AMD Secure Encrypted Virtualization (including ES)
    Sev,
    /// IBM Secure Execution
    Se,
    /// ARM Confidential Compute Architecture
    Cca,
    /// No TEE detected (running on regular hardware/VM)
    None,
}

impl TeeType {
    /// Returns the string representation of the TEE type
    pub fn as_str(&self) -> &str {
        match self {
            TeeType::Tdx => "tdx",
            TeeType::Sgx => "sgx",
            TeeType::Snp => "snp",
            TeeType::Sev => "sev",
            TeeType::Se => "se",
            TeeType::Cca => "cca",
            TeeType::None => "none",
        }
    }

    /// Returns whether this TEE type is a real TEE (not None)
    pub fn is_tee(&self) -> bool {
        !matches!(self, TeeType::None)
    }
}

/// TEE detection utility
pub struct TeeDetector;

impl TeeDetector {
    /// Detect the TEE type based on system characteristics
    ///
    /// This function checks for the presence of TEE-specific device files
    /// and system information to determine if the proplet is running inside
    /// a Trusted Execution Environment.
    ///
    /// # Returns
    /// The detected TEE type, or TeeType::None if no TEE is detected
    pub fn detect() -> TeeType {
        info!("Detecting TEE environment...");

        // Check for Intel TDX
        if Self::check_tdx() {
            info!("Detected Intel TDX environment");
            return TeeType::Tdx;
        }

        // Check for Intel SGX
        if Self::check_sgx() {
            info!("Detected Intel SGX environment");
            return TeeType::Sgx;
        }

        // Check for AMD SEV-SNP
        if Self::check_snp() {
            info!("Detected AMD SEV-SNP environment");
            return TeeType::Snp;
        }

        // Check for AMD SEV/SEV-ES
        if Self::check_sev() {
            info!("Detected AMD SEV environment");
            return TeeType::Sev;
        }

        // Check for IBM SE
        if Self::check_se() {
            info!("Detected IBM Secure Execution environment");
            return TeeType::Se;
        }

        // Check for ARM CCA
        if Self::check_cca() {
            info!("Detected ARM CCA environment");
            return TeeType::Cca;
        }

        info!("No TEE detected - running on regular hardware");
        TeeType::None
    }

    /// Check if running in Intel TDX
    fn check_tdx() -> bool {
        // TDX guest has /dev/tdx-guest device
        if Path::new("/dev/tdx-guest").exists() {
            debug!("Found /dev/tdx-guest device");
            return true;
        }

        // Check cpuinfo for TDX features
        if let Ok(cpuinfo) = fs::read_to_string("/proc/cpuinfo") {
            if cpuinfo.contains("tdx_guest") {
                debug!("Found tdx_guest in /proc/cpuinfo");
                return true;
            }
        }

        // Check for TDX in /sys/firmware
        if Path::new("/sys/firmware/tdx_guest").exists() {
            debug!("Found /sys/firmware/tdx_guest");
            return true;
        }

        false
    }

    /// Check if running in Intel SGX
    fn check_sgx() -> bool {
        // SGX has specific device files
        if Path::new("/dev/sgx_enclave").exists() {
            debug!("Found /dev/sgx_enclave device");
            return true;
        }

        if Path::new("/dev/sgx/enclave").exists() {
            debug!("Found /dev/sgx/enclave device");
            return true;
        }

        // Check cpuinfo for SGX features
        if let Ok(cpuinfo) = fs::read_to_string("/proc/cpuinfo") {
            if cpuinfo.contains("sgx") {
                debug!("Found sgx in /proc/cpuinfo");
                return true;
            }
        }

        false
    }

    /// Check if running in AMD SEV-SNP
    fn check_snp() -> bool {
        // SEV-SNP has /dev/sev-guest device
        if Path::new("/dev/sev-guest").exists() {
            debug!("Found /dev/sev-guest device");
            return true;
        }

        // Check for SNP in cpuinfo
        if let Ok(cpuinfo) = fs::read_to_string("/proc/cpuinfo") {
            if cpuinfo.contains("sev_snp") {
                debug!("Found sev_snp in /proc/cpuinfo");
                return true;
            }
        }

        // Check dmesg for SNP messages (requires root)
        if let Ok(dmesg) = fs::read_to_string("/var/log/dmesg") {
            if dmesg.contains("SEV-SNP") {
                debug!("Found SEV-SNP in dmesg");
                return true;
            }
        }

        false
    }

    /// Check if running in AMD SEV/SEV-ES
    fn check_sev() -> bool {
        // SEV has /dev/sev device
        if Path::new("/dev/sev").exists() {
            debug!("Found /dev/sev device");
            return true;
        }

        // Check for SEV in cpuinfo
        if let Ok(cpuinfo) = fs::read_to_string("/proc/cpuinfo") {
            if cpuinfo.contains("sev") || cpuinfo.contains("sme") {
                debug!("Found sev/sme in /proc/cpuinfo");
                return true;
            }
        }

        // Check /sys/kernel/mm/transparent_hugepage/sev
        if Path::new("/sys/kernel/mm/mem_encrypt/active").exists() {
            if let Ok(active) = fs::read_to_string("/sys/kernel/mm/mem_encrypt/active") {
                if active.trim() == "1" {
                    debug!("Found active memory encryption");
                    return true;
                }
            }
        }

        false
    }

    /// Check if running in IBM Secure Execution
    fn check_se() -> bool {
        // IBM SE has specific sysfs entries
        if Path::new("/sys/firmware/uv").exists() {
            debug!("Found /sys/firmware/uv (IBM SE)");
            return true;
        }

        // Check for ultravisor in cpuinfo
        if let Ok(cpuinfo) = fs::read_to_string("/proc/cpuinfo") {
            if cpuinfo.contains("ultravisor") {
                debug!("Found ultravisor in /proc/cpuinfo");
                return true;
            }
        }

        false
    }

    /// Check if running in ARM CCA
    fn check_cca() -> bool {
        // ARM CCA detection (placeholder - update when devices are available)
        if Path::new("/dev/cca-guest").exists() {
            debug!("Found /dev/cca-guest device");
            return true;
        }

        // Check for CCA in device tree (ARM systems)
        if let Ok(dt) = fs::read_to_string("/proc/device-tree/compatible") {
            if dt.contains("arm,cca") {
                debug!("Found ARM CCA in device tree");
                return true;
            }
        }

        false
    }

    /// Check if running in any TEE
    #[allow(dead_code)]
    pub fn is_in_tee() -> bool {
        Self::detect().is_tee()
    }

    /// Get a detailed description of the current environment
    pub fn describe_environment() -> String {
        let tee_type = Self::detect();
        match tee_type {
            TeeType::None => {
                "Running on regular hardware/VM without TEE protection".to_string()
            }
            _ => {
                format!(
                    "Running in {} Trusted Execution Environment",
                    tee_type.as_str().to_uppercase()
                )
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tee_type_as_str() {
        assert_eq!(TeeType::Tdx.as_str(), "tdx");
        assert_eq!(TeeType::Sgx.as_str(), "sgx");
        assert_eq!(TeeType::Snp.as_str(), "snp");
        assert_eq!(TeeType::Sev.as_str(), "sev");
        assert_eq!(TeeType::None.as_str(), "none");
    }

    #[test]
    fn test_tee_type_is_tee() {
        assert!(TeeType::Tdx.is_tee());
        assert!(TeeType::Sgx.is_tee());
        assert!(TeeType::Snp.is_tee());
        assert!(!TeeType::None.is_tee());
    }

    #[test]
    fn test_detect_runs_without_panic() {
        // Should not panic even if no TEE is detected
        let tee_type = TeeDetector::detect();
        assert!(!tee_type.as_str().is_empty());
    }

    #[test]
    fn test_describe_environment() {
        let description = TeeDetector::describe_environment();
        assert!(!description.is_empty());
    }
}
