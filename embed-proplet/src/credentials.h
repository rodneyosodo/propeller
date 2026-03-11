#ifndef CREDENTIALS_H
#define CREDENTIALS_H

#define CRED_MAX_LEN 64

struct proplet_credentials {
	char wifi_ssid[CRED_MAX_LEN];
	char wifi_psk[CRED_MAX_LEN];
	char proplet_id[CRED_MAX_LEN];
	char client_key[CRED_MAX_LEN];
	char domain_id[CRED_MAX_LEN];
	char channel_id[CRED_MAX_LEN];
};

int credentials_load(struct proplet_credentials *creds,
		     const struct proplet_credentials *defaults);

int credentials_save_field(const char *key, const char *value);

const struct proplet_credentials *credentials_get(void);

#endif /* CREDENTIALS_H */
