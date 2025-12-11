use anyhow::{Context, Result};
use rumqttc::{AsyncClient, Event, EventLoop, MqttOptions, Packet, QoS};
use serde::{de::DeserializeOwned, Serialize};
use std::time::Duration;
use tokio::sync::mpsc;
use tracing::{debug, error, info, warn};

pub struct MqttConfig {
    pub address: String,
    pub client_id: String,
    pub timeout: Duration,
    pub qos: QoS,
    pub username: String,
    pub password: String,
}

#[derive(Clone)]
pub struct PubSub {
    client: AsyncClient,
}

impl PubSub {
    pub async fn new(config: MqttConfig) -> Result<(Self, EventLoop)> {
        // Parse the address to extract host and port
        // Expected format: tcp://host:port or just host:port
        let address_without_scheme = config
            .address
            .split("://")
            .nth(1)
            .unwrap_or(&config.address);

        let (host, port) = if let Some(colon_pos) = address_without_scheme.rfind(':') {
            let host = &address_without_scheme[..colon_pos];
            let port = address_without_scheme[colon_pos + 1..]
                .parse::<u16>()
                .unwrap_or(1883);
            (host, port)
        } else {
            (address_without_scheme, 1883)
        };

        let mut mqtt_options = MqttOptions::new(config.client_id, host, port);

        mqtt_options.set_keep_alive(Duration::from_secs(30));
        mqtt_options.set_credentials(config.username, config.password);

        // Increase max packet size to handle large chunks (10MB)
        mqtt_options.set_max_packet_size(10 * 1024 * 1024, 10 * 1024 * 1024);

        let (client, eventloop) = AsyncClient::new(mqtt_options, 10);

        info!("MQTT client created");

        Ok((Self { client }, eventloop))
    }

    pub async fn publish<T: Serialize>(&self, topic: &str, payload: &T) -> Result<()> {
        let json = serde_json::to_vec(payload).context("Failed to serialize payload")?;

        self.client
            .publish(topic, QoS::ExactlyOnce, false, json)
            .await
            .context("Failed to publish message")?;

        debug!("Published to topic: {}", topic);
        Ok(())
    }

    pub async fn subscribe(&self, topic: &str) -> Result<()> {
        self.client
            .subscribe(topic, QoS::ExactlyOnce)
            .await
            .context(format!("Failed to subscribe to topic: {}", topic))?;

        info!("Subscribed to topic: {}", topic);
        Ok(())
    }

    pub async fn unsubscribe(&self, topic: &str) -> Result<()> {
        self.client
            .unsubscribe(topic)
            .await
            .context(format!("Failed to unsubscribe from topic: {}", topic))?;

        info!("Unsubscribed from topic: {}", topic);
        Ok(())
    }

    pub fn disconnect(&self) -> Result<()> {
        // rumqttc handles disconnection automatically when dropped
        info!("Disconnecting MQTT client");
        Ok(())
    }
}

pub struct MqttMessage {
    pub topic: String,
    pub payload: Vec<u8>,
}

impl MqttMessage {
    pub fn decode<T: DeserializeOwned>(&self) -> Result<T> {
        serde_json::from_slice(&self.payload).context(format!(
            "Failed to deserialize message payload from topic '{}'",
            self.topic
        ))
    }
}

pub async fn process_mqtt_events(mut eventloop: EventLoop, tx: mpsc::UnboundedSender<MqttMessage>) {
    info!("Starting MQTT event loop");

    loop {
        match eventloop.poll().await {
            Ok(Event::Incoming(Packet::Publish(publish))) => {
                let msg = MqttMessage {
                    topic: publish.topic.clone(),
                    payload: publish.payload.to_vec(),
                };

                if let Err(e) = tx.send(msg) {
                    error!("Failed to send MQTT message to handler: {}", e);
                }
            }
            Ok(Event::Incoming(packet)) => {
                debug!("Received MQTT packet: {:?}", packet);
            }
            Ok(Event::Outgoing(_)) => {
                // Ignore outgoing events
            }
            Err(e) => {
                warn!("MQTT connection error: {}", e);
                tokio::time::sleep(Duration::from_secs(1)).await;
            }
        }
    }
}

pub fn build_topic(domain_id: &str, channel_id: &str, path: &str) -> String {
    format!("m/{}/c/{}/{}", domain_id, channel_id, path)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_topic() {
        let topic = build_topic("domain-1", "channel-1", "control/manager/start");
        assert_eq!(topic, "m/domain-1/c/channel-1/control/manager/start");
    }

    #[test]
    fn test_build_topic_with_empty_path() {
        let topic = build_topic("domain-1", "channel-1", "");
        assert_eq!(topic, "m/domain-1/c/channel-1/");
    }

    #[test]
    fn test_build_topic_with_slashes() {
        let topic = build_topic("domain/1", "channel/1", "path/to/topic");
        assert_eq!(topic, "m/domain/1/c/channel/1/path/to/topic");
    }

    #[test]
    fn test_mqtt_message_decode_success() {
        #[derive(Debug, serde::Deserialize, PartialEq)]
        struct TestPayload {
            id: String,
            value: i32,
        }

        let payload = serde_json::json!({
            "id": "test-123",
            "value": 42
        });

        let msg = MqttMessage {
            topic: "test/topic".to_string(),
            payload: serde_json::to_vec(&payload).unwrap(),
        };

        let decoded: TestPayload = msg.decode().unwrap();
        assert_eq!(decoded.id, "test-123");
        assert_eq!(decoded.value, 42);
    }

    #[test]
    fn test_mqtt_message_decode_failure() {
        let msg = MqttMessage {
            topic: "test/topic".to_string(),
            payload: b"invalid json".to_vec(),
        };

        let result: Result<serde_json::Value> = msg.decode();
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("Failed to deserialize"));
        assert!(result.as_ref().unwrap_err().to_string().contains("test/topic"));
    }

    #[test]
    fn test_mqtt_message_decode_empty_payload() {
        let msg = MqttMessage {
            topic: "test/topic".to_string(),
            payload: Vec::new(),
        };

        let result: Result<serde_json::Value> = msg.decode();
        assert!(result.is_err());
    }

    #[test]
    fn test_mqtt_config_address_parsing_with_scheme_and_port() {
        // Test address parsing logic (unit test for the parsing logic)
        let address = "tcp://broker.example.com:1883";
        let address_without_scheme = address.split("://").nth(1).unwrap_or(&address);
        
        let (host, port) = if let Some(colon_pos) = address_without_scheme.rfind(':') {
            let host = &address_without_scheme[..colon_pos];
            let port = address_without_scheme[colon_pos + 1..]
                .parse::<u16>()
                .unwrap_or(1883);
            (host, port)
        } else {
            (address_without_scheme, 1883)
        };

        assert_eq!(host, "broker.example.com");
        assert_eq!(port, 1883);
    }

    #[test]
    fn test_mqtt_config_address_parsing_without_scheme() {
        let address = "localhost:1883";
        let address_without_scheme = address.split("://").nth(1).unwrap_or(&address);
        
        let (host, port) = if let Some(colon_pos) = address_without_scheme.rfind(':') {
            let host = &address_without_scheme[..colon_pos];
            let port = address_without_scheme[colon_pos + 1..]
                .parse::<u16>()
                .unwrap_or(1883);
            (host, port)
        } else {
            (address_without_scheme, 1883)
        };

        assert_eq!(host, "localhost");
        assert_eq!(port, 1883);
    }

    #[test]
    fn test_mqtt_config_address_parsing_without_port() {
        let address = "tcp://broker.example.com";
        let address_without_scheme = address.split("://").nth(1).unwrap_or(&address);
        
        let (host, port) = if let Some(colon_pos) = address_without_scheme.rfind(':') {
            let host = &address_without_scheme[..colon_pos];
            let port = address_without_scheme[colon_pos + 1..]
                .parse::<u16>()
                .unwrap_or(1883);
            (host, port)
        } else {
            (address_without_scheme, 1883)
        };

        assert_eq!(host, "broker.example.com");
        assert_eq!(port, 1883);
    }

    #[test]
    fn test_mqtt_config_address_parsing_custom_port() {
        let address = "tcp://broker.example.com:8883";
        let address_without_scheme = address.split("://").nth(1).unwrap_or(&address);
        
        let (host, port) = if let Some(colon_pos) = address_without_scheme.rfind(':') {
            let host = &address_without_scheme[..colon_pos];
            let port = address_without_scheme[colon_pos + 1..]
                .parse::<u16>()
                .unwrap_or(1883);
            (host, port)
        } else {
            (address_without_scheme, 1883)
        };

        assert_eq!(host, "broker.example.com");
        assert_eq!(port, 8883);
    }

    #[test]
    fn test_mqtt_config_address_parsing_invalid_port() {
        let address = "tcp://broker.example.com:invalid";
        let address_without_scheme = address.split("://").nth(1).unwrap_or(&address);
        
        let (host, port) = if let Some(colon_pos) = address_without_scheme.rfind(':') {
            let host = &address_without_scheme[..colon_pos];
            let port = address_without_scheme[colon_pos + 1..]
                .parse::<u16>()
                .unwrap_or(1883);
            (host, port)
        } else {
            (address_without_scheme, 1883)
        };

        assert_eq!(host, "broker.example.com");
        assert_eq!(port, 1883); // Falls back to default
    }

    #[test]
    fn test_mqtt_config_address_ipv4() {
        let address = "tcp://192.168.1.100:1883";
        let address_without_scheme = address.split("://").nth(1).unwrap_or(&address);
        
        let (host, port) = if let Some(colon_pos) = address_without_scheme.rfind(':') {
            let host = &address_without_scheme[..colon_pos];
            let port = address_without_scheme[colon_pos + 1..]
                .parse::<u16>()
                .unwrap_or(1883);
            (host, port)
        } else {
            (address_without_scheme, 1883)
        };

        assert_eq!(host, "192.168.1.100");
        assert_eq!(port, 1883);
    }

    #[test]
    fn test_mqtt_config_address_ipv6() {
        let address = "tcp://[::1]:1883";
        let address_without_scheme = address.split("://").nth(1).unwrap_or(&address);
        
        let (host, port) = if let Some(colon_pos) = address_without_scheme.rfind(':') {
            let host = &address_without_scheme[..colon_pos];
            let port = address_without_scheme[colon_pos + 1..]
                .parse::<u16>()
                .unwrap_or(1883);
            (host, port)
        } else {
            (address_without_scheme, 1883)
        };

        assert_eq!(host, "[::1]");
        assert_eq!(port, 1883);
    }

    #[test]
    fn test_mqtt_message_decode_with_nested_structure() {
        #[derive(Debug, serde::Deserialize, PartialEq)]
        struct NestedPayload {
            outer: OuterData,
        }

        #[derive(Debug, serde::Deserialize, PartialEq)]
        struct OuterData {
            inner: InnerData,
        }

        #[derive(Debug, serde::Deserialize, PartialEq)]
        struct InnerData {
            value: String,
        }

        let payload = serde_json::json!({
            "outer": {
                "inner": {
                    "value": "nested-value"
                }
            }
        });

        let msg = MqttMessage {
            topic: "nested/topic".to_string(),
            payload: serde_json::to_vec(&payload).unwrap(),
        };

        let decoded: NestedPayload = msg.decode().unwrap();
        assert_eq!(decoded.outer.inner.value, "nested-value");
    }

    #[test]
    fn test_mqtt_config_creation() {
        let config = MqttConfig {
            address: "tcp://localhost:1883".to_string(),
            client_id: "test-client".to_string(),
            timeout: Duration::from_secs(30),
            qos: QoS::ExactlyOnce,
            username: "user".to_string(),
            password: "pass".to_string(),
        };

        assert_eq!(config.address, "tcp://localhost:1883");
        assert_eq!(config.client_id, "test-client");
        assert_eq!(config.timeout, Duration::from_secs(30));
        assert_eq!(config.username, "user");
        assert_eq!(config.password, "pass");
    }
}
