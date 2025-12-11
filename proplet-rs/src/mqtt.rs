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
    /// Create a new `PubSub` and its associated MQTT `EventLoop` from the given `MqttConfig`.
    ///
    /// The function parses the configured address (accepting `tcp://host:port` or `host:port`, using port `1883` when absent or not parseable), applies client ID, credentials, a 30s keep-alive, and increases the maximum packet size to 10 MB for both sent and received packets.
    ///
    /// Returns `Ok((PubSub, EventLoop))` on success or an error if MQTT client construction fails.
    ///
    /// # Examples
    ///
    /// ```
    /// use proplet_rs::mqtt::{PubSub, MqttConfig};
    ///
    /// #[tokio::test]
    /// async fn create_pubsub() {
    ///     let cfg = MqttConfig {
    ///         address: "localhost:1883".into(),
    ///         client_id: "test-client".into(),
    ///         timeout: 30,
    ///         qos: 1,
    ///         username: "user".into(),
    ///         password: "pass".into(),
    ///     };
    ///
    ///     let (pubsub, _eventloop) = PubSub::new(cfg).await.unwrap();
    ///     // pubsub can now be used to publish/subscribe; eventloop drives incoming events.
    /// }
    /// ```
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
    /// Deserialize the message payload (JSON) into the requested type.
    ///
    /// On failure, the returned error includes context mentioning the message's topic.
    ///
    /// # Returns
    ///
    /// The deserialized value of type `T` on success.
    ///
    /// # Examples
    ///
    /// ```
    /// use serde::Deserialize;
    ///
    /// #[derive(Deserialize, Debug, PartialEq)]
    /// struct Item { id: u32, name: String }
    ///
    /// // construct a message with JSON payload
    /// let msg = MqttMessage {
    ///     topic: "m/domain/c/channel/path".to_string(),
    ///     payload: serde_json::to_vec(&Item { id: 1, name: "a".into() }).unwrap(),
    /// };
    ///
    /// let item: Item = msg.decode().unwrap();
    /// assert_eq!(item, Item { id: 1, name: "a".into() });
    /// ```
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