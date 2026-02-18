# HAL

Builds and runs an Ubuntu CVM with Propeller services pre-installed via QEMU.

For full documentation, see [propeller.absmach.eu](https://propeller.absmach.eu/docs/hal).

## Prerequisites

```bash
sudo apt-get update
sudo apt-get install -y \
  qemu-system-x86 \
  cloud-image-utils \
  ovmf \
  wget
```

## Configure

Set these before running the script:

| Variable               | Description                     | Default                |
| ---------------------- | ------------------------------- | ---------------------- |
| `PROPLET_DOMAIN_ID`    | SuperMQ domain ID               |                        |
| `PROPLET_CLIENT_ID`    | SuperMQ client ID               |                        |
| `PROPLET_CLIENT_KEY`   | SuperMQ client key              |                        |
| `PROPLET_CHANNEL_ID`   | SuperMQ channel ID              |                        |
| `PROPLET_MQTT_ADDRESS` | MQTT broker address             | `tcp://localhost:1883` |
| `KBS_URL`              | Key Broker Service URL          | `http://10.0.2.2:8082` |
| `ENABLE_CVM`           | `auto`, `tdx`, `sev`, or `none` | `auto`                 |
| `RAM`                  | VM memory                       | `16384M`               |
| `CPU`                  | vCPU count                      | `4`                    |
| `DISK_SIZE`            | Disk image size                 | `40G`                  |

## Run

The script re-executes itself with `sudo -E` to preserve exported variables.

```bash
export PROPLET_DOMAIN_ID="your-domain-id"
export PROPLET_CLIENT_ID="your-client-id"
export PROPLET_CLIENT_KEY="your-client-key"
export PROPLET_CHANNEL_ID="your-channel-id"
export PROPLET_MQTT_ADDRESS="tcp://mqtt.example.com:1883"
export KBS_URL="http://10.0.2.2:8082"

# Build and run (default)
./qemu.sh

# Build only
./qemu.sh build

# Run an existing image
./qemu.sh run
```

CVM mode is auto-detected. Override with `ENABLE_CVM=tdx`, `ENABLE_CVM=sev`, or `ENABLE_CVM=none`.

First boot takes 10–15 minutes while cloud-init compiles Wasmtime, Attestation Agent, CoCo Keyprovider, and Proplet from source. Subsequent boots start all services immediately.

## Access

```bash
ssh -p 2222 propeller@localhost
# password: propeller
```

| Host port | Service           |
| --------- | ----------------- |
| `2222`    | SSH               |
| `50010`   | Attestation Agent |
| `50011`   | CoCo Keyprovider  |

```bash
sudo systemctl status attestation-agent coco-keyprovider proplet
sudo journalctl -u proplet -f
```
