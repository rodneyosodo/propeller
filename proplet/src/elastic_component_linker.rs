use crate::runtime::wasmtime_runtime::StoreData;
use anyhow::Result;
use std::collections::HashMap;
use std::io::{Read, Write};
use std::path::PathBuf;
use wasmtime::component::Linker;
use wasmtime::component::HasData;

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
    next_container: u64,
}

impl ElasticHalHost {
    pub fn new(storage_path: String) -> Self {
        Self {
            sockets: HashMap::new(),
            next_socket: 1,
            storage_path: PathBuf::from(storage_path),
            containers: HashMap::new(),
            next_container: 1,
        }
    }

    fn container_dir(&self, handle: u64) -> Result<PathBuf, String> {
        let dir_name = format!("container_{handle}");
        let dir = self.storage_path.join(&dir_name);
        std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
        Ok(dir)
    }
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
        let id = self.next_socket;
        match protocol {
            generated::elastic::sockets::sockets::Protocol::Tcp => {
                self.sockets.insert(id, SocketEntry::PendingTcp);
            }
            generated::elastic::sockets::sockets::Protocol::Udp => {
                self.sockets.insert(id, SocketEntry::PendingUdp);
            }
            _ => return Err(format!("Unsupported protocol: {protocol:?}")),
        };
        self.next_socket += 1;
        Ok(id)
    }

    fn bind(
        &mut self,
        socket: u64,
        addr: generated::elastic::sockets::sockets::Address,
    ) -> Result<(), String> {
        let entry = self
            .sockets
            .get_mut(&socket)
            .ok_or_else(|| format!("Invalid socket handle: {socket}"))?;
        let addr_str = format!("{}:{}", addr.ip, addr.port);
        match std::mem::replace(entry, SocketEntry::PendingTcp) {
            SocketEntry::PendingTcp => {
                let listener = std::net::TcpListener::bind(&addr_str).map_err(|e| e.to_string())?;
                *entry = SocketEntry::TcpListener(listener);
            }
            SocketEntry::PendingUdp => {
                let sock = std::net::UdpSocket::bind(&addr_str).map_err(|e| e.to_string())?;
                *entry = SocketEntry::UdpSocket(sock);
            }
            other => {
                *entry = other;
                return Err("Socket already bound".to_string());
            }
        }
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
        let entry = self
            .sockets
            .get_mut(&socket)
            .ok_or_else(|| format!("Invalid socket handle: {socket}"))?;
        let addr_str = format!("{}:{}", addr.ip, addr.port);
        match std::mem::replace(entry, SocketEntry::PendingTcp) {
            SocketEntry::PendingTcp => {
                let stream = std::net::TcpStream::connect(&addr_str).map_err(|e| e.to_string())?;
                *entry = SocketEntry::TcpStream(stream);
            }
            other => {
                *entry = other;
                return Err("Socket not in pending state".to_string());
            }
        }
        Ok(())
    }

    fn accept(&mut self, socket: u64) -> Result<u64, String> {
        let entry = self
            .sockets
            .get_mut(&socket)
            .ok_or_else(|| format!("Invalid socket handle: {socket}"))?;
        match entry {
            SocketEntry::TcpListener(listener) => {
                let (stream, _) = listener.accept().map_err(|e| e.to_string())?;
                let id = self.next_socket;
                self.sockets.insert(id, SocketEntry::TcpStream(stream));
                self.next_socket += 1;
                Ok(id)
            }
            _ => Err("Socket is not a TCP listener".to_string()),
        }
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
        let handle = self.next_container;
        let dir = self.container_dir(handle)?;
        std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
        self.containers.insert(handle, name);
        self.next_container += 1;
        Ok(handle)
    }

    fn open_container(&mut self, name: String) -> Result<u64, String> {
        let handle = self.next_container;
        let dir = self.container_dir(handle)?;
        std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
        self.containers.insert(handle, name);
        self.next_container += 1;
        Ok(handle)
    }

    fn delete_container(&mut self, handle: u64) -> Result<(), String> {
        self.containers.remove(&handle);
        let dir = self.container_dir(handle)?;
        let _ = std::fs::remove_dir_all(&dir);
        Ok(())
    }

    fn store_object(
        &mut self,
        container: u64,
        key: String,
        data: Vec<u8>,
    ) -> Result<u64, String> {
        let _name = self
            .containers
            .get(&container)
            .ok_or_else(|| format!("Invalid container handle: {container}"))?;
        let dir = self.container_dir(container)?;
        std::fs::write(dir.join(format!("{key}.obj")), &data)
            .map_err(|e| e.to_string())?;
        Ok(0)
    }

    fn retrieve_object(&mut self, container: u64, key: String) -> Result<Vec<u8>, String> {
        let _name = self
            .containers
            .get(&container)
            .ok_or_else(|| format!("Invalid container handle: {container}"))?;
        let dir = self.container_dir(container)?;
        std::fs::read(dir.join(format!("{key}.obj")))
            .map_err(|e| e.to_string())
    }

    fn delete_object(&mut self, container: u64, key: String) -> Result<(), String> {
        let _name = self
            .containers
            .get(&container)
            .ok_or_else(|| format!("Invalid container handle: {container}"))?;
        let dir = self.container_dir(container)?;
        std::fs::remove_file(dir.join(format!("{key}.obj")))
            .map_err(|e| e.to_string())
    }

    fn list_objects(&mut self, container: u64) -> Result<Vec<String>, String> {
        let _name = self
            .containers
            .get(&container)
            .ok_or_else(|| format!("Invalid container handle: {container}"))?;
        let dir = self.container_dir(container)?;
        if !dir.exists() {
            return Ok(vec![]);
        }
        let mut result = Vec::new();
        let entries = std::fs::read_dir(&dir).map_err(|e| e.to_string())?;
        for entry in entries {
            let entry = entry.map_err(|e| e.to_string())?;
            let file_name = entry.file_name();
            let name = file_name.to_string_lossy();
            if name.ends_with(".obj") {
                let key = name.strip_suffix(".obj").unwrap();
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
        let _name = self
            .containers
            .get(&container)
            .ok_or_else(|| format!("Invalid container handle: {container}"))?;
        let dir = self.container_dir(container)?;
        let meta_path = dir.join(format!("{key}.meta"));

        if meta_path.exists() {
            let meta_data = std::fs::read_to_string(&meta_path).map_err(|e| e.to_string())?;
            if let Ok(meta) = serde_json::from_str::<serde_json::Value>(&meta_data) {
                return Ok(generated::elastic::storage::storage::ObjectMetadata {
                    size: meta["size"].as_u64().unwrap_or(0),
                    created_at: meta["created_at"].as_u64().unwrap_or(0),
                    content_type: meta["content_type"].as_str().unwrap_or("application/octet-stream").to_string(),
                });
            }
        }

        let obj_path = dir.join(format!("{key}.obj"));
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

    fn generate_keypair(
        &mut self,
    ) -> Result<generated::elastic::crypto::crypto::KeyPair, String> {
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
    fn get_system_time(
        &mut self,
    ) -> Result<generated::elastic::clock::clock::SystemTime, String> {
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
        let dur = std::time::Instant::now().elapsed();
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
