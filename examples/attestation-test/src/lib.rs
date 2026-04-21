wit_bindgen::generate!({
    world: "hal-consumer",
    inline: "
        package elastic:hal@0.1.0;

        interface platform {
            record platform-info { platform-type: string, version: string, }
            get-platform-info: func() -> platform-info;
        }

        interface attestation {
            attestation: func(report-data: list<u8>) -> result<list<u8>, string>;
        }

        interface random {
            get-random-bytes: func(length: u32) -> result<list<u8>, string>;
        }

        interface run {
            run: func() -> list<u8>;
        }

        world hal-consumer {
            import platform;
            import attestation;
            import random;
            export run;
        }
    ",
});

use exports::elastic::hal::run::Guest;

struct Component;

impl Guest for Component {
    fn run() -> Vec<u8> {
        let info = elastic::hal::platform::get_platform_info();
        eprintln!("platform: {} v{}", info.platform_type, info.version);

        let report_data = match elastic::hal::random::get_random_bytes(32) {
            Ok(bytes) => bytes,
            Err(_) => vec![0x42u8; 32],
        };

        match elastic::hal::attestation::attestation(&report_data) {
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

export!(Component with_types_in self);
