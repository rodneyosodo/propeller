#include "credentials.h"
#include "mqtt_client.h"
#include "task_monitor.h"
#include "wifi_manager.h"
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>

LOG_MODULE_REGISTER(main);

#define PROPLET_NAMESPACE "embedded"

#define PROPLET_DESCRIPTION "ESP32-S3 embed-proplet"
#define PROPLET_TAGS        "embedded,esp32s3,zephyr"
#define PROPLET_LOCATION    "dev-bench"
#define PROPLET_VERSION     "0.1.0-PROP-103"

#ifndef PROPLET_LIVELINESS_INTERVAL_MS
#define PROPLET_LIVELINESS_INTERVAL_MS 10000
#endif

#ifndef PROPLET_METRICS_INTERVAL_MS
#define PROPLET_METRICS_INTERVAL_MS 30000
#endif

#ifndef PROPLET_TASK_METRICS_INTERVAL_MS
#define PROPLET_TASK_METRICS_INTERVAL_MS 10000
#endif

static const struct proplet_credentials defaults = {
	.wifi_ssid   = "<YOUR_WIFI_SSID>",
	.wifi_psk    = "<YOUR_WIFI_PSK>",
	.proplet_id  = "<YOUR_PROPLET_ID>",
	.client_key  = "<YOUR_CLIENT_KEY>",
	.domain_id   = "<YOUR_DOMAIN_ID>",
	.channel_id  = "<YOUR_CHANNEL_ID>",
};

int main(void)
{
	LOG_INF("Starting Proplet...");

	struct proplet_credentials creds;

	if (credentials_load(&creds, &defaults) != 0) {
		LOG_ERR("Failed to load credentials");
		return -1;
	}

	wifi_manager_init();
	wifi_manager_connect(creds.wifi_ssid, creds.wifi_psk);

	if (task_monitor_init() != 0) {
		LOG_WRN("Task monitor init failed, metrics may be unavailable");
	}

	if (mqtt_client_connect(creds.domain_id, creds.proplet_id,
				creds.client_key, creds.channel_id) != 0) {
		LOG_ERR("MQTT connect failed");
		return -1;
	}

	if (subscribe(creds.domain_id, creds.channel_id) != 0) {
		LOG_ERR("MQTT subscribe failed");
		return -1;
	}

	if (publish_discovery(creds.domain_id, creds.proplet_id,
			      creds.channel_id,
			      PROPLET_DESCRIPTION, PROPLET_TAGS,
			      PROPLET_LOCATION, PROPLET_VERSION) != 0) {
		LOG_ERR("MQTT discovery publish failed");
		return -1;
	}

	for (int i = 0; i < 20; i++) {
		mqtt_client_process();
		k_sleep(K_MSEC(100));
	}

	int64_t next_alive = k_uptime_get() + PROPLET_LIVELINESS_INTERVAL_MS;
	int64_t next_metrics = k_uptime_get() + PROPLET_METRICS_INTERVAL_MS;
	int64_t next_task_metrics = k_uptime_get() + PROPLET_TASK_METRICS_INTERVAL_MS;

	while (1) {
		mqtt_client_process();

		int64_t now = k_uptime_get();

		if (now >= next_alive) {
			publish_alive_message(creds.domain_id, creds.channel_id);
			next_alive = now + PROPLET_LIVELINESS_INTERVAL_MS;
		}

		if (now >= next_metrics) {
			publish_metrics_message(creds.domain_id, creds.channel_id,
						creds.proplet_id, PROPLET_NAMESPACE);
			next_metrics = now + PROPLET_METRICS_INTERVAL_MS;
		}

		if (now >= next_task_metrics) {
			publish_active_task_metrics(creds.domain_id,
						    creds.channel_id,
						    creds.proplet_id);
			next_task_metrics = now + PROPLET_TASK_METRICS_INTERVAL_MS;
		}

		k_sleep(K_MSEC(200));
	}
}
