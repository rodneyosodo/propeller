#[allow(warnings)]
mod bindings {
    wit_bindgen::generate!({
        world: "hal-consumer",
        path: "wit",
    });
}

use bindings::elastic::hal::{attestation, platform, random};
use bindings::exports::elastic::hal::run::Guest;

struct Component;

impl Guest for Component {
    fn run() -> Vec<u8> {
        let info = platform::get_platform_info();
        eprintln!("platform: {} v{}", info.platform_type, info.version);

        let report_data = match random::get_random_bytes(32) {
            Ok(bytes) => bytes,
            Err(_) => vec![0x42u8; 32],
        };

        match attestation::attestation(&report_data) {
            Ok(evidence) => {
                eprintln!("attestation: ok (evidence len={})", evidence.len());
                evidence
            }
            Err(e) => {
                eprintln!("attestation: failed — {}", e);
                format!("attestation-error:{}", e).into_bytes()
            }
        }
    }
}

bindings::export!(Component with_types_in bindings);
