data "omada_sites" "all" {}

data "omada_gateway_ports" "all" {
  site_id = data.omada_sites.all.sites[0].id
}

output "gateway_ports" {
  value = data.omada_gateway_ports.all.ports
}

# Use a specific port's id when binding a network
resource "omada_network" "iot" {
  site_id        = data.omada_sites.all.sites[0].id
  name           = "iot"
  purpose        = "interface"
  vlan_id        = 50
  gateway_subnet = "10.10.50.1/24"
  dhcp_enabled   = true
  dhcp_start     = "10.10.50.100"
  dhcp_end       = "10.10.50.250"

  lan_interface_ids = [
    for port in data.omada_gateway_ports.all.ports :
    port.id if port.name == "WAN/LAN1"
  ]
}
