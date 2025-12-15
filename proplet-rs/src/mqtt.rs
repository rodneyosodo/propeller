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
    pub keep_alive: Duration,
    pub max_packet_size: usize,
    pub inflight: u16,
    pub request_channel_capacity: usize,
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

        mqtt_options.set_keep_alive(config.keep_alive);
        mqtt_options.set_credentials(config.username, config.password);
        mqtt_options.set_max_packet_size(config.max_packet_size, config.max_packet_size);
        mqtt_options.set_inflight(config.inflight);

        let (client, eventloop) = AsyncClient::new(mqtt_options, config.request_channel_capacity);

        info!("MQTT client created");

        Ok((Self { client }, eventloop))
    }

    pub async fn publish<T: Serialize>(&self, topic: &str, payload: &T, qos: QoS) -> Result<()> {
        let json = serde_json::to_vec(payload).context("Failed to serialize payload")?;

        self.client
            .publish(topic, qos, false, json)
            .await
            .context("Failed to publish message")?;

        debug!("Published to topic: {}", topic);
        Ok(())
    }

    pub async fn subscribe(&self, topic: &str, qos: QoS) -> Result<()> {
        self.client
            .subscribe(topic, qos)
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

pub async fn process_mqtt_events(mut eventloop: EventLoop, tx: mpsc::Sender<MqttMessage>) {
    info!("Starting MQTT event loop");

    loop {
        match eventloop.poll().await {
            Ok(Event::Incoming(Packet::Publish(publish))) => {
                let msg = MqttMessage {
                    topic: publish.topic.clone(),
                    payload: publish.payload.to_vec(),
                };

                if let Err(e) = tx.send(msg).await {
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
