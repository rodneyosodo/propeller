use crate::runtime::wasmtime_runtime::StoreData;
use anyhow::Result;
use std::collections::HashMap;
use std::io::{Read, Write};
use std::path::PathBuf;
use std::time::Instant;
use wasmtime::component::{HasData, Linker};

mod generated {
    wasmtime::component::bindgen!({
        path: "elastic-wit",
        world: "elastic-hal",
    });
}

enum SocketEntry {
    TcpListener(std::net::TcpListener),
    TcpStream(std::net::TcpStream),
    UdpSocket(std::net::UdpSocket),
    PendingTcp,
    PendingUdp,
}

pub struct ElasticHalHost {
    sockets: HashMap<u64, SocketEntry>,
    next_socket: u64,
    storage_path: PathBuf,
    containers: HashMap<u64, String>,
    container_handles: HashMap<String, u64>,
    next_container: u64,
    objects: HashMap<u64, (u64, String)>,
    next_object: u64,
    process_start: Instant,
}

impl ElasticHalHost {
    pub fn new(storage_path: String) -> Self {
        Self {
            sockets: HashMap::new(),
            next_socket: 1,
            storage_path: PathBuf::from(storage_path),
            containers: HashMap::new(),
            container_handles: HashMap::new(),
            next_container: 1,
            objects: HashMap::new(),
            next_object: 1,
            process_start: Instant::now(),
        }
    }

    fn container_dir(&self, handle: u64) -> Result<PathBuf, String> {
        let dir = self.storage_path.join(format!("container_{handle}"));
        std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
        Ok(dir)
    }
}

fn sanitize_key(key: &str) -> Result<&str, String> {
    if key.is_empty()
        || key.contains('/')
        || key.contains('\\')
        || key.contains('\0')
        || key == "."
        || key == ".."
    {
        return Err(format!("Invalid object key: {key:?}"));
    }
    Ok(key)
}

struct ProjectToElasticHal;

impl HasData for ProjectToElasticHal {
    type Data<'a> = &'a mut ElasticHalHost;
}

pub fn add_elastic_to_linker(linker: &mut Linker<StoreData>) -> Result<()> {
    generated::ElasticHal::add_to_linker::<StoreData, ProjectToElasticHal>(
        linker,
        |store_data: &mut StoreData| -> &mut ElasticHalHost {
            store_data
                .elastic_hal
                .as_mut()
                .expect("elastic_hal not initialized")
        },
    )?;
    Ok(())
}

impl generated::elastic::sockets::sockets::Host for ElasticHalHost {
    fn create_socket(
        &mut self,
        protocol: generated::elastic::sockets::sockets::Protocol,
    ) -> Result<u64, String> {
        use generated::elastic::sockets::sockets::Protocol as P;
        let entry = match protocol {
            P::Tcp => SocketEntry::PendingTcp,
            P::Udp => SocketEntry::PendingUdp,
            _ => return Err(format!("Unsupported protocol: {protocol:?}")),
        };
        let id = self.next_socket;
        self.sockets.insert(id, entry);
        self.next_socket += 1;
        Ok(id)
    }

    fn bind(
        &mut self,
        socket: u64,
        addr: generated::elastic::sockets::sockets::Address,
    ) -> Result<(), String> {
        let addr_str = format!("{}:{}", addr.ip, addr.port);
        let new_entry = match self.sockets.get(&socket) {
            None => return Err(format!("Invalid socket handle: {socket}")),
            Some(SocketEntry::PendingTcp) => {
                let listener = std::net::TcpListener::bind(&addr_str).map_err(|e| e.to_string())?;
                SocketEntry::TcpListener(listener)
            }
            Some(SocketEntry::PendingUdp) => {
                let sock = std::net::UdpSocket::bind(&addr_str).map_err(|e| e.to_string())?;
                SocketEntry::UdpSocket(sock)
            }
            Some(_) => return Err("Socket already bound".to_string()),
        };
        self.sockets.insert(socket, new_entry);
        Ok(())
    }

    fn listen(&mut self, _socket: u64, _backlog: u32) -> Result<(), String> {
        Ok(())
    }

    fn connect(
        &mut self,
        socket: u64,
        addr: generated::elastic::sockets::sockets::Address,
    ) -> Result<(), String> {
        let addr_str = format!("{}:{}", addr.ip, addr.port);
        match self.sockets.get(&socket) {
            None => return Err(format!("Invalid socket handle: {socket}")),
            Some(SocketEntry::PendingTcp) => {}
            Some(_) => return Err("Socket not in pending state".to_string()),
        }
        let stream = std::net::TcpStream::connect(&addr_str).map_err(|e| e.to_string())?;
        self.sockets.insert(socket, SocketEntry::TcpStream(stream));
        Ok(())
    }

    fn accept(&mut self, socket: u64) -> Result<u64, String> {
        let stream = match self.sockets.get(&socket) {
            None => return Err(format!("Invalid socket handle: {socket}")),
            Some(SocketEntry::TcpListener(listener)) => {
                let (stream, _) = listener.accept().map_err(|e| e.to_string())?;
                stream
            }
            Some(_) => return Err("Socket is not a TCP listener".to_string()),
        };
        let id = self.next_socket;
        self.sockets.insert(id, SocketEntry::TcpStream(stream));
        self.next_socket += 1;
        Ok(id)
    }

    fn send(&mut self, socket: u64, data: Vec<u8>) -> Result<u32, String> {
        let entry = self
            .sockets
            .get_mut(&socket)
            .ok_or_else(|| format!("Invalid socket handle: {socket}"))?;
        match entry {
            SocketEntry::TcpStream(stream) => {
                let n = stream.write(&data).map_err(|e| e.to_string())?;
                Ok(n as u32)
            }
            SocketEntry::UdpSocket(sock) => {
                let n = sock.send(&data).map_err(|e| e.to_string())?;
                Ok(n as u32)
            }
            _ => Err("Socket not writable".to_string()),
        }
    }

    fn receive(&mut self, socket: u64, max_len: u32) -> Result<Vec<u8>, String> {
        let entry = self
            .sockets
            .get_mut(&socket)
            .ok_or_else(|| format!("Invalid socket handle: {socket}"))?;
        let mut buf = vec![0u8; max_len as usize];
        match entry {
            SocketEntry::TcpStream(stream) => {
                let n = stream.read(&mut buf).map_err(|e| e.to_string())?;
                buf.truncate(n);
                Ok(buf)
            }
            SocketEntry::UdpSocket(sock) => {
                let n = sock.recv(&mut buf).map_err(|e| e.to_string())?;
                buf.truncate(n);
                Ok(buf)
            }
            _ => Err("Socket not readable".to_string()),
        }
    }

    fn close(&mut self, socket: u64) -> Result<(), String> {
        self.sockets.remove(&socket);
        Ok(())
    }
}

impl generated::elastic::storage::storage::Host for ElasticHalHost {
    fn create_container(&mut self, name: String) -> Result<u64, String> {
        if let Some(&existing) = self.container_handles.get(&name) {
            return Ok(existing);
        }
        let handle = self.next_container;
        self.container_dir(handle)?;
        self.containers.insert(handle, name.clone());
        self.container_handles.insert(name, handle);
        self.next_container += 1;
        Ok(handle)
    }

    fn open_container(&mut self, name: String) -> Result<u64, String> {
        if let Some(&existing) = self.container_handles.get(&name) {
            return Ok(existing);
        }
        Err(format!("Container not found: {name}"))
    }

    fn delete_container(&mut self, handle: u64) -> Result<(), String> {
        if let Some(name) = self.containers.remove(&handle) {
            self.container_handles.remove(&name);
        }
        self.objects.retain(|_, (c, _)| *c != handle);
        let dir = self.storage_path.join(format!("container_{handle}"));
        let _ = std::fs::remove_dir_all(&dir);
        Ok(())
    }

    fn store_object(&mut self, container: u64, key: String, data: Vec<u8>) -> Result<u64, String> {
        if !self.containers.contains_key(&container) {
            return Err(format!("Invalid container handle: {container}"));
        }
        let safe_key = sanitize_key(&key)?.to_string();
        let dir = self.container_dir(container)?;
        std::fs::write(dir.join(format!("{safe_key}.obj")), &data).map_err(|e| e.to_string())?;
        let handle = self.next_object;
        self.objects.insert(handle, (container, safe_key));
        self.next_object += 1;
        Ok(handle)
    }

    fn retrieve_object(&mut self, container: u64, key: String) -> Result<Vec<u8>, String> {
        if !self.containers.contains_key(&container) {
            return Err(format!("Invalid container handle: {container}"));
        }
        let safe_key = sanitize_key(&key)?;
        let dir = self.container_dir(container)?;
        std::fs::read(dir.join(format!("{safe_key}.obj"))).map_err(|e| e.to_string())
    }

    fn delete_object(&mut self, container: u64, key: String) -> Result<(), String> {
        if !self.containers.contains_key(&container) {
            return Err(format!("Invalid container handle: {container}"));
        }
        let safe_key = sanitize_key(&key)?.to_string();
        let dir = self.container_dir(container)?;
        std::fs::remove_file(dir.join(format!("{safe_key}.obj"))).map_err(|e| e.to_string())?;
        self.objects
            .retain(|_, (c, k)| !(*c == container && k == &safe_key));
        Ok(())
    }

    fn list_objects(&mut self, container: u64) -> Result<Vec<String>, String> {
        if !self.containers.contains_key(&container) {
            return Err(format!("Invalid container handle: {container}"));
        }
        let dir = self.container_dir(container)?;
        if !dir.exists() {
            return Ok(vec![]);
        }
        let mut result = Vec::new();
        for entry in std::fs::read_dir(&dir).map_err(|e| e.to_string())? {
            let entry = entry.map_err(|e| e.to_string())?;
            let file_name = entry.file_name();
            let name = file_name.to_string_lossy();
            if let Some(key) = name.strip_suffix(".obj") {
                result.push(key.to_string());
            }
        }
        Ok(result)
    }

    fn get_metadata(
        &mut self,
        container: u64,
        key: String,
    ) -> Result<generated::elastic::storage::storage::ObjectMetadata, String> {
        if !self.containers.contains_key(&container) {
            return Err(format!("Invalid container handle: {container}"));
        }
        let safe_key = sanitize_key(&key)?;
        let dir = self.container_dir(container)?;
        let meta_path = dir.join(format!("{safe_key}.meta"));

        if meta_path.exists() {
            let meta_data = std::fs::read_to_string(&meta_path).map_err(|e| e.to_string())?;
            if let Ok(meta) = serde_json::from_str::<serde_json::Value>(&meta_data) {
                return Ok(generated::elastic::storage::storage::ObjectMetadata {
                    size: meta["size"].as_u64().unwrap_or(0),
                    created_at: meta["created_at"].as_u64().unwrap_or(0),
                    content_type: meta["content_type"]
                        .as_str()
                        .unwrap_or("application/octet-stream")
                        .to_string(),
                });
            }
        }

        let obj_path = dir.join(format!("{safe_key}.obj"));
        let metadata = std::fs::metadata(&obj_path).map_err(|e| e.to_string())?;
        Ok(generated::elastic::storage::storage::ObjectMetadata {
            size: metadata.len(),
            created_at: 0,
            content_type: "application/octet-stream".to_string(),
        })
    }
}

impl generated::elastic::crypto::crypto::Host for ElasticHalHost {
    fn hash(
        &mut self,
        data: Vec<u8>,
        algorithm: generated::elastic::crypto::crypto::HashAlgorithm,
    ) -> Result<Vec<u8>, String> {
        use generated::elastic::crypto::crypto::HashAlgorithm;
        match algorithm {
            HashAlgorithm::Sha256 => {
                use sha2::{Digest, Sha256};
                let mut hasher = Sha256::new();
                hasher.update(&data);
                Ok(hasher.finalize().to_vec())
            }
            HashAlgorithm::Sha512 => {
                use sha2::{Digest, Sha512};
                let mut hasher = Sha512::new();
                hasher.update(&data);
                Ok(hasher.finalize().to_vec())
            }
            HashAlgorithm::Blake3 => {
                let hash = blake3::hash(&data);
                Ok(hash.as_bytes().to_vec())
            }
        }
    }

    fn encrypt(
        &mut self,
        _data: Vec<u8>,
        _key: Vec<u8>,
        _algorithm: generated::elastic::crypto::crypto::CipherAlgorithm,
    ) -> Result<Vec<u8>, String> {
        Err("encrypt not yet implemented".to_string())
    }

    fn decrypt(
        &mut self,
        _data: Vec<u8>,
        _key: Vec<u8>,
        _algorithm: generated::elastic::crypto::crypto::CipherAlgorithm,
    ) -> Result<Vec<u8>, String> {
        Err("decrypt not yet implemented".to_string())
    }

    fn generate_keypair(&mut self) -> Result<generated::elastic::crypto::crypto::KeyPair, String> {
        Err("generate_keypair not yet implemented".to_string())
    }

    fn sign(&mut self, _data: Vec<u8>, _private_key: Vec<u8>) -> Result<Vec<u8>, String> {
        Err("sign not yet implemented".to_string())
    }

    fn verify(
        &mut self,
        _data: Vec<u8>,
        _signature: Vec<u8>,
        _public_key: Vec<u8>,
    ) -> Result<bool, String> {
        Err("verify not yet implemented".to_string())
    }

    fn create_context(&mut self) -> Result<u64, String> {
        Ok(1)
    }

    fn destroy_context(&mut self, _handle: u64) -> Result<(), String> {
        Ok(())
    }
}

impl generated::elastic::clock::clock::Host for ElasticHalHost {
    fn get_system_time(&mut self) -> Result<generated::elastic::clock::clock::SystemTime, String> {
        use std::time::{SystemTime, UNIX_EPOCH};
        let dur = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_err(|e| e.to_string())?;
        Ok(generated::elastic::clock::clock::SystemTime {
            seconds: dur.as_secs(),
            nanoseconds: dur.subsec_nanos(),
        })
    }

    fn get_monotonic_time(
        &mut self,
    ) -> Result<generated::elastic::clock::clock::MonotonicTime, String> {
        let dur = self.process_start.elapsed();
        Ok(generated::elastic::clock::clock::MonotonicTime {
            elapsed_seconds: dur.as_secs(),
            elapsed_nanoseconds: dur.subsec_nanos(),
        })
    }

    fn resolution(&mut self) -> Result<u64, String> {
        Ok(1_000_000_000)
    }

    fn sleep(&mut self, _duration_ns: u64) -> Result<(), String> {
        Err("sleep not yet implemented (sync)".to_string())
    }
}

impl generated::elastic::random::random::Host for ElasticHalHost {
    fn get_random_bytes(&mut self, length: u32) -> Result<Vec<u8>, String> {
        let mut buf = vec![0u8; length as usize];
        getrandom::getrandom(&mut buf).map_err(|e| e.to_string())?;
        Ok(buf)
    }

    fn get_secure_random(&mut self, length: u32) -> Result<Vec<u8>, String> {
        let mut buf = vec![0u8; length as usize];
        getrandom::getrandom(&mut buf).map_err(|e| e.to_string())?;
        Ok(buf)
    }

    fn get_entropy_info(
        &mut self,
    ) -> Result<generated::elastic::random::random::EntropyInfo, String> {
        Ok(generated::elastic::random::random::EntropyInfo {
            source: generated::elastic::random::random::EntropySource::Platform,
            quality: 100,
            available_bytes: 1024,
        })
    }

    fn reseed(&mut self, _additional_entropy: Vec<u8>) -> Result<(), String> {
        Ok(())
    }
}
