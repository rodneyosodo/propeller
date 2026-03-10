#include "wifi_manager.h"
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>
#include <zephyr/net/dhcpv4_server.h>
#include <zephyr/net/net_if.h>
#include <zephyr/net/wifi_mgmt.h>

LOG_MODULE_REGISTER(wifi_manager);

static struct net_if *ap_iface;
static struct net_if *sta_iface;
static struct net_mgmt_event_callback wifi_cb;

#define MACSTR "%02X:%02X:%02X:%02X:%02X:%02X"
#define MAC2STR(mac) mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]

static void wifi_event_handler(struct net_mgmt_event_callback *cb,
                               uint32_t mgmt_event, struct net_if *iface)
{
  switch (mgmt_event)
  {
  case NET_EVENT_WIFI_CONNECT_RESULT:
    LOG_INF("Wi-Fi connected");
    break;
  case NET_EVENT_WIFI_DISCONNECT_RESULT:
    LOG_INF("Wi-Fi disconnected");
    break;
  case NET_EVENT_WIFI_AP_ENABLE_RESULT:
    LOG_INF("AP mode enabled");
    break;
  case NET_EVENT_WIFI_AP_DISABLE_RESULT:
    LOG_INF("AP mode disabled");
    break;
  case NET_EVENT_WIFI_AP_STA_CONNECTED:
  {
    struct wifi_ap_sta_info *sta = (struct wifi_ap_sta_info *)cb->info;
    LOG_INF("Station " MACSTR " connected", MAC2STR(sta->mac));
    break;
  }
  case NET_EVENT_WIFI_AP_STA_DISCONNECTED:
  {
    struct wifi_ap_sta_info *sta = (struct wifi_ap_sta_info *)cb->info;
    LOG_INF("Station " MACSTR " disconnected", MAC2STR(sta->mac));
    break;
  }
  default:
    break;
  }
}

static void enable_dhcpv4_server(const char *ip_address, const char *netmask)
{
  struct in_addr addr, netmask_addr;

  if (net_addr_pton(AF_INET, ip_address, &addr) != 0)
  {
    LOG_ERR("Invalid IP address: %s", ip_address);
    return;
  }

  if (net_addr_pton(AF_INET, netmask, &netmask_addr) != 0)
  {
    LOG_ERR("Invalid netmask: %s", netmask);
    return;
  }

  net_if_ipv4_set_gw(ap_iface, &addr);
  net_if_ipv4_addr_add(ap_iface, &addr, NET_ADDR_MANUAL, 0);
  net_if_ipv4_set_netmask(ap_iface, &netmask_addr);

  addr.s4_addr[3] += 10;
  if (net_dhcpv4_server_start(ap_iface, &addr) != 0)
  {
    LOG_ERR("Failed to start DHCPv4 server");
  }

  LOG_INF("DHCPv4 server started");
}

void wifi_manager_init(void)
{
  net_mgmt_init_event_callback(
      &wifi_cb, wifi_event_handler,
      NET_EVENT_WIFI_CONNECT_RESULT | NET_EVENT_WIFI_DISCONNECT_RESULT |
          NET_EVENT_WIFI_AP_ENABLE_RESULT | NET_EVENT_WIFI_AP_DISABLE_RESULT |
          NET_EVENT_WIFI_AP_STA_CONNECTED | NET_EVENT_WIFI_AP_STA_DISCONNECTED);
  net_mgmt_add_event_callback(&wifi_cb);

  ap_iface = net_if_get_wifi_sap();
  sta_iface = net_if_get_wifi_sta();
}

int wifi_manager_connect(const char *ssid, const char *psk)
{
  if (!sta_iface)
  {
    LOG_ERR("STA interface not initialized");
    return -EIO;
  }

  struct wifi_connect_req_params params = {
      .ssid = ssid,
      .ssid_length = strlen(ssid),
      .psk = psk,
      .psk_length = strlen(psk),
      .security = WIFI_SECURITY_TYPE_PSK,
      .channel = WIFI_CHANNEL_ANY,
      .band = WIFI_FREQ_BAND_UNKNOWN,
  };

  while (1)
  {
    LOG_INF("Connecting to Wi-Fi: %s", ssid);

    int ret = net_mgmt(NET_REQUEST_WIFI_CONNECT, sta_iface,
                       &params, sizeof(params));
    if (ret != 0) {
      LOG_ERR("Connection request failed (%d). Retrying in 5s...", ret);
      k_sleep(K_SECONDS(5));
      continue;
    }

    bool associated = false;
    for (int i = 0; i < 40; i++) {
      k_sleep(K_MSEC(500));
      struct wifi_iface_status status = {0};
      if (net_mgmt(NET_REQUEST_WIFI_IFACE_STATUS, sta_iface,
                   &status, sizeof(status)) == 0) {
        if (status.state >= WIFI_STATE_COMPLETED) {
          associated = true;
          break;
        }
      }
    }

    if (associated) {
      k_sleep(K_SECONDS(5));
      LOG_INF("Connected to Wi-Fi: %s", ssid);
      return 0;
    }

    LOG_WRN("Wi-Fi connect timed out. Retrying...");
    net_mgmt(NET_REQUEST_WIFI_DISCONNECT, sta_iface, NULL, 0);
    k_sleep(K_SECONDS(3));
  }
}

int wifi_manager_enable_ap(const char *ssid, const char *psk,
                           const char *ip_address, const char *netmask)
{
  if (!ap_iface)
  {
    LOG_ERR("AP interface not initialized");
    return -EIO;
  }

  struct wifi_connect_req_params params = {
      .ssid = ssid,
      .ssid_length = strlen(ssid),
      .psk = psk,
      .psk_length = strlen(psk),
      .security =
          (strlen(psk) > 0) ? WIFI_SECURITY_TYPE_PSK : WIFI_SECURITY_TYPE_NONE,
      .channel = WIFI_CHANNEL_ANY,
      .band = WIFI_FREQ_BAND_2_4_GHZ,
  };

  enable_dhcpv4_server(ip_address, netmask);

  LOG_INF("Enabling AP mode with SSID: %s", ssid);
  return net_mgmt(NET_REQUEST_WIFI_AP_ENABLE, ap_iface, &params,
                  sizeof(params));
}
