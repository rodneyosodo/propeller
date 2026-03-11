#include "credentials.h"

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

	char *dst = NULL;

	if (strcmp(key, "wifi_ssid") == 0) {
		dst = g_creds->wifi_ssid;
	} else if (strcmp(key, "wifi_psk") == 0) {
		dst = g_creds->wifi_psk;
	} else if (strcmp(key, "proplet_id") == 0) {
		dst = g_creds->proplet_id;
	} else if (strcmp(key, "client_key") == 0) {
		dst = g_creds->client_key;
	} else if (strcmp(key, "domain_id") == 0) {
		dst = g_creds->domain_id;
	} else if (strcmp(key, "channel_id") == 0) {
		dst = g_creds->channel_id;
	} else {
		return -ENOENT;
	}

	size_t max = CRED_MAX_LEN - 1;
	ssize_t rc = read_cb(cb_arg, dst, MIN(len, max));

	if (rc < 0) {
		return (int)rc;
	}
	dst[rc] = '\0';

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

