#[link(wasm_import_module = "elastic:tee-hal/platform")]
unsafe extern "C" {
    fn attestation(data_ptr: i32, data_len: i32, out_ptr: i32, out_len_ptr: i32) -> i32;

    #[link_name = "platform-info"]
    fn platform_info(out_ptr: i32, out_len_ptr: i32) -> i32;
}

#[link(wasm_import_module = "elastic:tee-hal/random")]
unsafe extern "C" {
    #[link_name = "get-random-bytes"]
    fn get_random_bytes(length: i32, out_ptr: i32, out_len_ptr: i32) -> i32;
}

const BUF: usize = 4096;

fn main() {
    let mut platform_buf = [0u8; BUF];
    let mut platform_len: u32 = 0;
    let rc = unsafe {
        platform_info(
            platform_buf.as_mut_ptr() as i32,
            &mut platform_len as *mut u32 as i32,
        )
    };
    if rc >= 0 {
        println!(
            "platform-info: {}",
            String::from_utf8_lossy(&platform_buf[..platform_len as usize])
        );
    } else {
        eprintln!("platform-info: failed (rc={})", rc);
    }

    let mut rnd_buf = [0u8; 32];
    let mut rnd_len: u32 = 0;
    let rc = unsafe {
        get_random_bytes(
            32,
            rnd_buf.as_mut_ptr() as i32,
            &mut rnd_len as *mut u32 as i32,
        )
    };
    let report_data: &[u8] = if rc >= 0 {
        &rnd_buf[..rnd_len as usize]
    } else {
        &[0x42u8; 32]
    };

    let mut out_buf = [0u8; BUF];
    let mut out_len: u32 = 0;
    let rc = unsafe {
        attestation(
            report_data.as_ptr() as i32,
            report_data.len() as i32,
            out_buf.as_mut_ptr() as i32,
            &mut out_len as *mut u32 as i32,
        )
    };
    if rc < 0 {
        eprintln!("attestation: failed (rc={})", rc);
    } else {
        println!("attestation: ok (evidence len={})", out_len);
        println!(
            "evidence: {}",
            String::from_utf8_lossy(&out_buf[..out_len as usize])
        );
    }
}
