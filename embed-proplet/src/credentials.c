#include "credentials.h"

#include <stdlib.h>
#include <string.h>
#include <zephyr/logging/log.h>
#include <zephyr/settings/settings.h>

LOG_MODULE_REGISTER(credentials);

static struct proplet_credentials *g_creds;

static int creds_set_cb(const char *key, size_t len, settings_read_cb read_cb,
			void *cb_arg)
{
	if (!g_creds) {
		return -EINVAL;
	}

	if (strcmp(key, "wifi_ssid") == 0) {
		size_t max = CRED_MAX_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->wifi_ssid, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->wifi_ssid[rc] = '\0';
	} else if (strcmp(key, "wifi_psk") == 0) {
		size_t max = CRED_MAX_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->wifi_psk, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->wifi_psk[rc] = '\0';
	} else if (strcmp(key, "proplet_id") == 0) {
		size_t max = CRED_MAX_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->proplet_id, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->proplet_id[rc] = '\0';
	} else if (strcmp(key, "client_key") == 0) {
		size_t max = CRED_MAX_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->client_key, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->client_key[rc] = '\0';
	} else if (strcmp(key, "domain_id") == 0) {
		size_t max = CRED_MAX_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->domain_id, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->domain_id[rc] = '\0';
	} else if (strcmp(key, "channel_id") == 0) {
		size_t max = CRED_MAX_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->channel_id, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->channel_id[rc] = '\0';
	} else if (strcmp(key, "mqtt_broker_host") == 0) {
		size_t max = CRED_HOST_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->mqtt_broker_host, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->mqtt_broker_host[rc] = '\0';
	} else if (strcmp(key, "mqtt_broker_port") == 0) {
		char buf[8];
		ssize_t rc = read_cb(cb_arg, buf, MIN(len, sizeof(buf) - 1));
		if (rc < 0) return (int)rc;
		buf[rc] = '\0';
		g_creds->mqtt_broker_port = (uint16_t)strtoul(buf, NULL, 10);
	} else if (strcmp(key, "mqtt_tls_enabled") == 0) {
		char buf[2];
		ssize_t rc = read_cb(cb_arg, buf, MIN(len, sizeof(buf) - 1));
		if (rc < 0) return (int)rc;
		buf[rc] = '\0';
		g_creds->mqtt_tls_enabled = (buf[0] == '1' || buf[0] == 't' || buf[0] == 'T');
	} else if (strcmp(key, "tls_client_cert") == 0) {
		size_t max = CRED_CERT_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->tls_client_cert, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->tls_client_cert[rc] = '\0';
	} else if (strcmp(key, "tls_client_key") == 0) {
		size_t max = CRED_CERT_LEN - 1;
		ssize_t rc = read_cb(cb_arg, g_creds->tls_client_key, MIN(len, max));
		if (rc < 0) return (int)rc;
		g_creds->tls_client_key[rc] = '\0';
	} else {
		return -ENOENT;
	}

	return 0;
}

static struct settings_handler creds_handler = {
	.name = "proplet",
	.h_set = creds_set_cb,
};

int credentials_load(struct proplet_credentials *creds,
		     const struct proplet_credentials *defaults)
{
	g_creds = creds;
	memcpy(creds, defaults, sizeof(*creds));

	int rc = settings_subsys_init();

	if (rc) {
		LOG_ERR("settings_subsys_init failed: %d", rc);
		return rc;
	}

	rc = settings_register(&creds_handler);
	if (rc) {
		LOG_ERR("settings_register failed: %d", rc);
		return rc;
	}

	rc = settings_load_subtree("proplet");
	if (rc) {
		LOG_WRN("settings_load failed: %d (using defaults)", rc);
	}

	return 0;
}

const struct proplet_credentials *credentials_get(void)
{
	return g_creds;
}

