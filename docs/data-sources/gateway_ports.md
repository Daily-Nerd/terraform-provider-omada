---
page_title: "omada_gateway_ports Data Source - omada"
subcategory: ""
description: |-
  Lists the WAN/LAN ports configurable on the site's gateway. Each port has a stable UUID that can be passed to omada_network's lan_interface_ids field to bind a network to that port.
---

# omada_gateway_ports (Data Source)

Lists the WAN/LAN ports configurable on the site's gateway. Each port has a stable UUID (the `id` attribute) that can be passed to `omada_network.lan_interface_ids` to bind a network to that port.

Returns the controller's port template even when no gateway has been adopted yet — useful for declaring `omada_network` resources that will be bound to specific ports once hardware is provisioned.

## Example Usage

```terraform
data "omada_gateway_ports" "all" {
  site_id = data.omada_sites.all.sites[0].id
}

resource "omada_network" "iot" {
  site_id        = data.omada_sites.all.sites[0].id
  name           = "iot"
  purpose        = "interface"
  vlan_id        = 50
  gateway_subnet = "10.10.50.1/24"
  dhcp_enabled   = true
  dhcp_start     = "10.10.50.100"
  dhcp_end       = "10.10.50.250"

  # Bind the network to LAN1 of the gateway
  lan_interface_ids = [
    for port in data.omada_gateway_ports.all.ports :
    port.id if port.name == "WAN/LAN1"
  ]
}
```

<!-- schema generated manually — keep in sync with internal/resources/data_sources.go -->
## Schema

### Required

- `site_id` (String) The ID of the site.

### Read-Only

- `ports` (Attributes List) List of gateway WAN/LAN ports. (see [below for nested schema](#nestedatt--ports))

<a id="nestedatt--ports"></a>
### Nested Schema for `ports`

Read-Only:

- `id` (String) Port UUID. Pass to `omada_network.lan_interface_ids`.
- `name` (String) Human-readable port name (e.g., `WAN`, `WAN/LAN1`, `LAN1`).
- `type` (Number) Port type. `0` = WAN-only, `1` = WAN/LAN dual-purpose.
- `mode` (Number) Current port mode. `0` = WAN, `1` = LAN.
- `lan_network_names` (List of String) Names of networks currently bound to this port.
