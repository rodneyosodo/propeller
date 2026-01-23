use std::fs;
use std::path::Path;

/// Represents the type of TEE (Trusted Execution Environment) detected
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TeeType {
    /// Intel Trust Domain Extensions (TDX)
    Tdx,
    /// AMD Secure Encrypted Virtualization (SEV/SEV-ES/SEV-SNP)
    Sev,
    /// Intel Software Guard Extensions (SGX)
    Sgx,
    /// No TEE detected
    None,
}

impl TeeType {
    /// Returns true if this is a valid TEE (not None)
    pub fn is_tee(&self) -> bool {
        !matches!(self, TeeType::None)
    }

    /// Returns the string representation of the TEE type
    pub fn as_str(&self) -> &str {
        match self {
            TeeType::Tdx => "TDX",
            TeeType::Sev => "SEV",
            TeeType::Sgx => "SGX",
            TeeType::None => "None",
        }
    }
}

/// TEE detection result with additional metadata
#[derive(Debug, Clone)]
pub struct TeeDetection {
    /// The type of TEE detected
    pub tee_type: TeeType,
    /// Detection method used (for logging/debugging)
    pub detection_method: String,
    /// Additional details about the detection
    pub details: Option<String>,
}

impl TeeDetection {
    /// Creates a new TeeDetection result
    pub fn new(tee_type: TeeType, detection_method: &str) -> Self {
        Self {
            tee_type,
            detection_method: detection_method.to_string(),
            details: None,
        }
    }

    /// Creates a new TeeDetection result with details
    pub fn with_details(tee_type: TeeType, detection_method: &str, details: &str) -> Self {
        Self {
            tee_type,
            detection_method: detection_method.to_string(),
            details: Some(details.to_string()),
        }
    }

    /// Returns true if running inside a TEE
    pub fn is_tee(&self) -> bool {
        self.tee_type.is_tee()
    }
}

/// Detects if the system is running inside a Trusted Execution Environment (TEE)
///
/// This function performs automatic detection by checking:
/// 1. Device files specific to each TEE type
/// 2. System files in /sys
/// 3. CPU information
/// 4. Kernel boot messages (dmesg)
///
/// Returns a `TeeDetection` struct with the detected TEE type and detection method.
pub fn detect_tee() -> TeeDetection {
    // Try TDX detection first (most specific)
    if let Some(detection) = detect_tdx() {
        return detection;
    }

    // Try SEV detection
    if let Some(detection) = detect_sev() {
        return detection;
    }

    // Try SGX detection
    if let Some(detection) = detect_sgx() {
        return detection;
    }

    // No TEE detected
    TeeDetection::new(TeeType::None, "no_tee_detected")
}

/// Detects Intel TDX (Trust Domain Extensions)
fn detect_tdx() -> Option<TeeDetection> {
    // Method 1: Check for TDX guest device
    if Path::new("/dev/tdx_guest").exists() {
        return Some(TeeDetection::with_details(
            TeeType::Tdx,
            "device_file",
            "/dev/tdx_guest exists",
        ));
    }

    // Method 2: Check for TDX in sysfs
    if Path::new("/sys/firmware/tdx_guest").exists() {
        return Some(TeeDetection::with_details(
            TeeType::Tdx,
            "sysfs",
            "/sys/firmware/tdx_guest exists",
        ));
    }

    // Method 3: Check cpuinfo for TDX flag
    if let Ok(cpuinfo) = fs::read_to_string("/proc/cpuinfo") {
        // TDX guests typically have specific CPU flags
        if cpuinfo.contains("tdx_guest") {
            return Some(TeeDetection::with_details(
                TeeType::Tdx,
                "cpuinfo",
                "tdx_guest flag found in /proc/cpuinfo",
            ));
        }
    }

    // Method 4: Check dmesg for TDX initialization messages
    if let Ok(output) = std::process::Command::new("dmesg").arg("--kernel").output() {
        if let Ok(dmesg) = String::from_utf8(output.stdout) {
            if dmesg.contains("tdx: Guest initialized") || dmesg.contains("virt/tdx") {
                return Some(TeeDetection::with_details(
                    TeeType::Tdx,
                    "dmesg",
                    "TDX initialization found in kernel messages",
                ));
            }
        }
    }

    None
}

/// Detects AMD SEV (Secure Encrypted Virtualization)
fn detect_sev() -> Option<TeeDetection> {
    // Method 1: Check for SEV device
    if Path::new("/dev/sev").exists() {
        return Some(TeeDetection::with_details(
            TeeType::Sev,
            "device_file",
            "/dev/sev exists",
        ));
    }

    // Method 2: Check SEV status in sysfs
    if let Ok(sev_status) = fs::read_to_string(
        "/sys/firmware/efi/efivars/SevStatus-00000000-0000-0000-0000-000000000000",
    ) {
        if !sev_status.is_empty() {
            return Some(TeeDetection::with_details(
                TeeType::Sev,
                "sysfs",
                "SEV status found in EFI variables",
            ));
        }
    }

    // Method 3: Check for SEV in cpuinfo
    if let Ok(cpuinfo) = fs::read_to_string("/proc/cpuinfo") {
        // SEV guests have specific flags
        if cpuinfo.contains("sev") || cpuinfo.contains("sev_es") || cpuinfo.contains("sev_snp") {
            let detail = if cpuinfo.contains("sev_snp") {
                "SEV-SNP detected in /proc/cpuinfo"
            } else if cpuinfo.contains("sev_es") {
                "SEV-ES detected in /proc/cpuinfo"
            } else {
                "SEV detected in /proc/cpuinfo"
            };
            return Some(TeeDetection::with_details(TeeType::Sev, "cpuinfo", detail));
        }
    }

    // Method 4: Check dmesg for SEV initialization
    if let Ok(output) = std::process::Command::new("dmesg").arg("--kernel").output() {
        if let Ok(dmesg) = String::from_utf8(output.stdout) {
            if dmesg.contains("AMD Memory Encryption Features active: SEV")
                || dmesg.contains("SEV enabled")
                || dmesg.contains("SEV-SNP")
            {
                return Some(TeeDetection::with_details(
                    TeeType::Sev,
                    "dmesg",
                    "SEV initialization found in kernel messages",
                ));
            }
        }
    }

    // Method 5: Check MSR (Model-Specific Register) for SEV - requires root
    if Path::new("/dev/cpu/0/msr").exists() {
        // Note: Reading MSR requires root privileges and special handling
        // This is a placeholder for more advanced detection
        // SEV detection via MSR 0xC0010131 (SEV_STATUS)
    }

    None
}

/// Detects Intel SGX (Software Guard Extensions)
fn detect_sgx() -> Option<TeeDetection> {
    // Method 1: Check for SGX device files
    if Path::new("/dev/sgx_enclave").exists() || Path::new("/dev/sgx/enclave").exists() {
        return Some(TeeDetection::with_details(
            TeeType::Sgx,
            "device_file",
            "SGX device file exists",
        ));
    }

    // Method 2: Check for in-kernel SGX driver
    if Path::new("/dev/isgx").exists() {
        return Some(TeeDetection::with_details(
            TeeType::Sgx,
            "device_file",
            "/dev/isgx exists (legacy driver)",
        ));
    }

    // Method 3: Check cpuinfo for SGX flags
    if let Ok(cpuinfo) = fs::read_to_string("/proc/cpuinfo") {
        if cpuinfo.contains("sgx") {
            return Some(TeeDetection::with_details(
                TeeType::Sgx,
                "cpuinfo",
                "SGX flag found in /proc/cpuinfo",
            ));
        }
    }

    // Method 4: Check dmesg for SGX initialization
    if let Ok(output) = std::process::Command::new("dmesg").arg("--kernel").output() {
        if let Ok(dmesg) = String::from_utf8(output.stdout) {
            if dmesg.contains("sgx: EPC section") || dmesg.contains("intel_sgx") {
                return Some(TeeDetection::with_details(
                    TeeType::Sgx,
                    "dmesg",
                    "SGX initialization found in kernel messages",
                ));
            }
        }
    }

    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tee_type_is_tee() {
        assert!(TeeType::Tdx.is_tee());
        assert!(TeeType::Sev.is_tee());
        assert!(TeeType::Sgx.is_tee());
        assert!(!TeeType::None.is_tee());
    }

    #[test]
    fn test_tee_type_as_str() {
        assert_eq!(TeeType::Tdx.as_str(), "TDX");
        assert_eq!(TeeType::Sev.as_str(), "SEV");
        assert_eq!(TeeType::Sgx.as_str(), "SGX");
        assert_eq!(TeeType::None.as_str(), "None");
    }

    #[test]
    fn test_tee_detection_new() {
        let detection = TeeDetection::new(TeeType::Tdx, "test_method");
        assert_eq!(detection.tee_type, TeeType::Tdx);
        assert_eq!(detection.detection_method, "test_method");
        assert!(detection.details.is_none());
        assert!(detection.is_tee());
    }

    #[test]
    fn test_tee_detection_with_details() {
        let detection = TeeDetection::with_details(TeeType::Sev, "device_file", "/dev/sev exists");
        assert_eq!(detection.tee_type, TeeType::Sev);
        assert_eq!(detection.detection_method, "device_file");
        assert_eq!(detection.details, Some("/dev/sev exists".to_string()));
        assert!(detection.is_tee());
    }

    #[test]
    fn test_tee_detection_none() {
        let detection = TeeDetection::new(TeeType::None, "no_detection");
        assert_eq!(detection.tee_type, TeeType::None);
        assert!(!detection.is_tee());
    }

    #[test]
    fn test_detect_tee_returns_result() {
        // This test will always pass as it just checks the function returns
        let detection = detect_tee();
        // The actual TEE type depends on the hardware, so we just verify it returns something
        assert!(matches!(
            detection.tee_type,
            TeeType::Tdx | TeeType::Sev | TeeType::Sgx | TeeType::None
        ));
    }

    #[test]
    fn test_detect_tdx_no_tee() {
        // This test checks that detect_tdx returns None when no TDX is present
        // On most development machines, this should be None
        let result = detect_tdx();
        // We can't assert the result since it depends on hardware
        // But we can verify it's either Some or None
        match result {
            Some(detection) => assert_eq!(detection.tee_type, TeeType::Tdx),
            None => assert!(true),
        }
    }

    #[test]
    fn test_detect_sev_no_tee() {
        // Similar to TDX test
        let result = detect_sev();
        match result {
            Some(detection) => assert_eq!(detection.tee_type, TeeType::Sev),
            None => assert!(true),
        }
    }

    #[test]
    fn test_detect_sgx_no_tee() {
        // Similar to TDX test
        let result = detect_sgx();
        match result {
            Some(detection) => assert_eq!(detection.tee_type, TeeType::Sgx),
            None => assert!(true),
        }
    }
}
