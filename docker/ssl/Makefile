# Copyright (c) Abstract Machines
# SPDX-License-Identifier: Apache-2.0

CRT_LOCATION = certs
O ?= Propeller
OU_CA ?= propeller_ca
OU_CRT ?= propeller_crt
EA ?= propeller@absmach.eu
CN_CA ?= Propeller_Self_Signed_CA
CN_SRV ?= localhost
CLIENT_SECRET ?= "" # e.g. 8f65ed04-0770-4ce4-a291-6d1bf2000f4d
CRT_FILE_NAME ?= proplet
COAP_DTLS_SERVER_CONF_FILE_NAME=coap-server.conf
COAP_DTLS_SERVER_CN=coap
COAP_DTLS_SERVER_CRT_FILE_NAME=coap-server
# Atom gRPC broker-auth listener. Atom terminates mutual TLS on its gRPC
# listener when broker auth is enabled (ATOM_BROKER_AUTH_ENABLED=true requires
# ATOM_GRPC_TLS_*), so FluxMQ needs a server certificate whose SAN covers the
# "atom" host it dials, plus a client certificate it presents. Both are signed
# by the deployment CA (docker/ssl/certs/ca.crt), which Atom also configures as
# its gRPC client CA.
ATOM_GRPC_SERVER_CONF_FILE_NAME=atom-grpc-server.conf
ATOM_GRPC_SERVER_CN=atom
ATOM_GRPC_SERVER_CRT_FILE_NAME=atom-grpc-server
FLUXMQ_ATOM_CLIENT_CONF_FILE_NAME=fluxmq-atom-client.conf
FLUXMQ_ATOM_CLIENT_CN=fluxmq
FLUXMQ_ATOM_CLIENT_CRT_FILE_NAME=fluxmq-atom-client

define GRPC_CERT_CONFIG
[req]
req_extensions = v3_req
distinguished_name = dn
prompt = no

[dn]
CN = propeller.svc
C  = RS
ST = RS
L  = BELGRADE
O  = Propeller
OU = Propeller

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = <<SERVICE_NAME>>
endef

# Server certificate for Atom's gRPC listener. FluxMQ dials it as "atom", so
# that name must be in the SAN; localhost covers host-side debugging.
define ATOM_GRPC_SERVER_CERT_CONFIG
[req]
req_extensions = v3_req
distinguished_name = dn
prompt = no

[dn]
CN = atom-grpc-server
C  = RS
ST = RS
L  = BELGRADE
O  = Propeller
OU = Propeller

[v3_req]
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = atom
DNS.2 = localhost
endef

# Client certificate FluxMQ presents to Atom's gRPC listener.
define FLUXMQ_ATOM_CLIENT_CERT_CONFIG
[req]
req_extensions = v3_req
distinguished_name = dn
prompt = no

[dn]
CN = fluxmq-atom-client
C  = RS
ST = RS
L  = BELGRADE
O  = Propeller
OU = Propeller

[v3_req]
extendedKeyUsage = clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = fluxmq
endef

define ANNOUNCE_BODY
Version $(VERSION) of $(PACKAGE_NAME) has been released.

It can be downloaded from $(DOWNLOAD_URL).

etc, etc.
endef
all: clean_certs ca server_cert client_cert coap_dtls_certs atom_grpc_certs

$(CRT_LOCATION):
	mkdir -p $@

ca server_cert client_cert: | $(CRT_LOCATION)

# CA name and key is "ca".
ca:
	openssl req -newkey rsa:2048 -x509 -nodes -sha512 -days 1095 \
				-keyout $(CRT_LOCATION)/ca.key -out $(CRT_LOCATION)/ca.crt -subj "/CN=$(CN_CA)/O=$(O)/OU=$(OU_CA)/emailAddress=$(EA)"

# Server cert and key name is "propeller-server".
server_cert:
	# Create propeller server key and CSR.
	openssl req -new -sha256 -newkey rsa:4096 -nodes -keyout $(CRT_LOCATION)/propeller-server.key \
				-out $(CRT_LOCATION)/propeller-server.csr -subj "/CN=$(CN_SRV)/O=$(O)/OU=$(OU_CRT)/emailAddress=$(EA)"

	# Sign server CSR with SANs for container hostnames.
	printf '[v3_req]\nsubjectAltName=DNS:localhost,DNS:nginx,DNS:%s' "$(CN_SRV)" > $(CRT_LOCATION)/propeller-server.san
	openssl x509 -req -days 1000 -in $(CRT_LOCATION)/propeller-server.csr \
		-CA $(CRT_LOCATION)/ca.crt -CAkey $(CRT_LOCATION)/ca.key -CAcreateserial \
		-out $(CRT_LOCATION)/propeller-server.crt \
		-extfile $(CRT_LOCATION)/propeller-server.san -extensions v3_req

	# Remove CSR and SAN config.
	rm $(CRT_LOCATION)/propeller-server.csr $(CRT_LOCATION)/propeller-server.san

client_cert:
	# Create propeller client key and CSR.
	openssl req -new -sha256 -newkey rsa:4096 -nodes -keyout $(CRT_LOCATION)/$(CRT_FILE_NAME).key \
				-out $(CRT_LOCATION)/$(CRT_FILE_NAME).csr -subj "/CN=$(CLIENT_SECRET)/O=$(O)/OU=$(OU_CRT)/emailAddress=$(EA)"

	# Sign client CSR.
	openssl x509 -req -days 730 -in $(CRT_LOCATION)/$(CRT_FILE_NAME).csr -CA $(CRT_LOCATION)/ca.crt -CAkey $(CRT_LOCATION)/ca.key -CAcreateserial -out $(CRT_LOCATION)/$(CRT_FILE_NAME).crt

	# Remove CSR.
	rm $(CRT_LOCATION)/$(CRT_FILE_NAME).csr

# Function to generate a certificate from an openssl config template.
# Usage: $(call gen_cert,cert_file_name,openssl_config_template)
define gen_cert
	$(file > $(CRT_LOCATION)/$(1).conf,$(2))

	openssl req -new -sha256 -newkey rsa:4096 -nodes \
		-keyout $(CRT_LOCATION)/$(1).key \
		-out $(CRT_LOCATION)/$(1).csr \
		-config $(CRT_LOCATION)/$(1).conf \
		-extensions v3_req

	openssl x509 -req -sha256 \
		-in $(CRT_LOCATION)/$(1).csr \
		-CA $(CRT_LOCATION)/ca.crt \
		-CAkey $(CRT_LOCATION)/ca.key \
		-CAcreateserial \
		-out $(CRT_LOCATION)/$(1).crt \
		-days 365 \
		-extfile $(CRT_LOCATION)/$(1).conf \
		-extensions v3_req

	# The key is bind-mounted read-only into non-root containers (Atom runs as
	# its own "atom" user), so it must be world-readable for the process to load
	# it. These are self-signed development certificates.
	chmod 0644 $(CRT_LOCATION)/$(1).key

	rm -rf $(CRT_LOCATION)/$(1).csr $(CRT_LOCATION)/$(1).conf
endef

coap_dtls_certs:
	$(call gen_cert,$(COAP_DTLS_SERVER_CRT_FILE_NAME),$(subst <<SERVICE_NAME>>,$(COAP_DTLS_SERVER_CN),$(GRPC_CERT_CONFIG)))

# Certificates for Atom's broker-auth gRPC listener (mutual TLS): the server
# certificate Atom serves and the client certificate FluxMQ presents.
atom_grpc_certs: | $(CRT_LOCATION)
	$(call gen_cert,$(ATOM_GRPC_SERVER_CRT_FILE_NAME),$(ATOM_GRPC_SERVER_CERT_CONFIG))
	$(call gen_cert,$(FLUXMQ_ATOM_CLIENT_CRT_FILE_NAME),$(FLUXMQ_ATOM_CLIENT_CERT_CONFIG))

clean_certs:
	rm -rf $(CRT_LOCATION)/*.crt $(CRT_LOCATION)/*.key $(CRT_LOCATION)/*.srl $(CRT_LOCATION)/*.conf
