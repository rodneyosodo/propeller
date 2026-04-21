use anyhow::Result;
use elastic_tee_hal::interfaces::{HalProvider, RandomInterface};
use elastic_tee_hal::providers::DefaultRandomProvider;
use std::sync::Arc;
use tracing::debug;
use wasmtime::component::{ComponentType, Lift, Linker, Lower};

use crate::runtime::wasmtime_runtime::StoreData;

#[derive(ComponentType, Lift, Lower, Clone)]
#[component(record)]
struct PlatformInfo {
    #[component(name = "platform-type")]
    platform_type: String,
    version: String,
}

pub fn add_to_linker(linker: &mut Linker<StoreData>, provider: Arc<HalProvider>) -> Result<()> {
    add_attestation(linker, provider.clone())?;
    add_platform(linker, provider.clone())?;
    add_random(linker)?;
    Ok(())
}

fn add_attestation(linker: &mut Linker<StoreData>, provider: Arc<HalProvider>) -> Result<()> {
    let mut instance = linker.instance("elastic:hal/attestation@0.1.0")?;
    instance.func_wrap(
        "attestation",
        move |_store, (report_data,): (Vec<u8>,)| {
            let outcome: Result<Vec<u8>, String> = match provider.platform.as_ref() {
                Some(platform) => platform.attestation(&report_data).map_err(|e| e.to_string()),
                None => {
                    debug!("elastic:hal/attestation: no TEE platform, returning stub");
                    Ok(b"{}".to_vec())
                }
            };
            Ok((outcome,))
        },
    )?;
    Ok(())
}

fn add_platform(linker: &mut Linker<StoreData>, provider: Arc<HalProvider>) -> Result<()> {
    let mut instance = linker.instance("elastic:hal/platform@0.1.0")?;
    instance.func_wrap(
        "get-platform-info",
        move |_store, ()| {
            let (platform_type, version) = match provider.platform.as_ref() {
                Some(p) => p
                    .platform_info()
                    .map(|(pt, v, _)| (pt, v))
                    .unwrap_or_else(|_| ("unknown".to_string(), "0.0.0".to_string())),
                None => ("none".to_string(), "0.0.0".to_string()),
            };
            Ok((PlatformInfo { platform_type, version },))
        },
    )?;
    Ok(())
}

fn add_random(linker: &mut Linker<StoreData>) -> Result<()> {
    let mut instance = linker.instance("elastic:hal/random@0.1.0")?;
    instance.func_wrap(
        "get-random-bytes",
        |_store, (length,): (u32,)| {
            let rng = DefaultRandomProvider::new();
            let outcome: Result<Vec<u8>, String> = rng.get_random_bytes(length);
            Ok((outcome,))
        },
    )?;
    Ok(())
}
