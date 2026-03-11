#include "credentials.h"

#include <string.h>
#include <zephyr/logging/log.h>
#include <zephyr/settings/settings.h>

#ifdef CONFIG_SHELL
#include <zephyr/shell/shell.h>
#endif

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

int credentials_save_field(const char *key, const char *value)
{
	char full_key[80];

	snprintf(full_key, sizeof(full_key), "proplet/%s", key);
	return settings_save_one(full_key, value, strlen(value));
}

const struct proplet_credentials *credentials_get(void)
{
	return g_creds;
}

#ifdef CONFIG_SHELL

static const char *const valid_fields[] = {
	"wifi_ssid", "wifi_psk", "proplet_id",
	"client_key", "domain_id", "channel_id", NULL,
};

static int cmd_creds_set(const struct shell *sh, size_t argc, char **argv)
{
	if (argc != 3) {
		shell_error(sh, "Usage: proplet creds set <field> <value>");
		return -EINVAL;
	}

	bool valid = false;

	for (int i = 0; valid_fields[i]; i++) {
		if (strcmp(argv[1], valid_fields[i]) == 0) {
			valid = true;
			break;
		}
	}
	if (!valid) {
		shell_error(sh, "Unknown field '%s'. Valid: wifi_ssid wifi_psk "
			    "proplet_id client_key domain_id channel_id",
			    argv[1]);
		return -EINVAL;
	}

	int rc = credentials_save_field(argv[1], argv[2]);

	if (rc) {
		shell_error(sh, "Failed to save %s: %d", argv[1], rc);
		return rc;
	}

	shell_print(sh, "Saved %s. Reboot to apply.", argv[1]);
	return 0;
}

static int cmd_creds_show(const struct shell *sh, size_t argc, char **argv)
{
	ARG_UNUSED(argc);
	ARG_UNUSED(argv);

	const struct proplet_credentials *c = credentials_get();

	if (!c) {
		shell_error(sh, "Credentials not loaded");
		return -ENODATA;
	}

	shell_print(sh, "wifi_ssid  : %s", c->wifi_ssid);
	shell_print(sh, "wifi_psk   : %s", c->wifi_psk);
	shell_print(sh, "proplet_id : %s", c->proplet_id);
	shell_print(sh, "client_key : %s", c->client_key);
	shell_print(sh, "domain_id  : %s", c->domain_id);
	shell_print(sh, "channel_id : %s", c->channel_id);

	return 0;
}

SHELL_STATIC_SUBCMD_SET_CREATE(creds_cmds,
	SHELL_CMD_ARG(set, NULL, "set <field> <value>", cmd_creds_set, 3, 0),
	SHELL_CMD(show, NULL, "Show current credentials", cmd_creds_show),
	SHELL_SUBCMD_SET_END);

SHELL_STATIC_SUBCMD_SET_CREATE(proplet_cmds,
	SHELL_CMD(creds, &creds_cmds, "Credential management", NULL),
	SHELL_SUBCMD_SET_END);

SHELL_CMD_REGISTER(proplet, &proplet_cmds, "Proplet commands", NULL);

#endif /* CONFIG_SHELL */
