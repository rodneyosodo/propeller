#ifndef CREDENTIALS_H
#define CREDENTIALS_H

#include <stdbool.h>
#include <stdint.h>

#define CRED_MAX_LEN 64
#define CRED_HOST_LEN 128
#define CRED_CERT_LEN 2048

struct proplet_credentials {
	char wifi_ssid[CRED_MAX_LEN];
	char wifi_psk[CRED_MAX_LEN];
	char proplet_id[CRED_MAX_LEN];
	char client_key[CRED_MAX_LEN];
	char domain_id[CRED_MAX_LEN];
	char channel_id[CRED_MAX_LEN];
	char mqtt_broker_host[CRED_HOST_LEN];
	uint16_t mqtt_broker_port;
	bool mqtt_tls_enabled;
	char tls_client_cert[CRED_CERT_LEN];
	char tls_client_key[CRED_CERT_LEN];
};

int credentials_load(struct proplet_credentials *creds,
		     const struct proplet_credentials *defaults);

const struct proplet_credentials *credentials_get(void);

#endif /* CREDENTIALS_H */
