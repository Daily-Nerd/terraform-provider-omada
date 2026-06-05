# Access port with per-port VLAN override (access_* profile)
resource "omada_switch_port" "k8s_node" {
  site_id    = omada_site.example.id
  device_mac = "AA-BB-CC-DD-EE-FF"
  port       = 5

  name                    = "k8s-node-1"
  profile_id              = omada_port_profile.access_trusted.id
  profile_override_enable = true
  native_network_id       = omada_network.trusted.id
  network_tags_setting    = 2 # access mode
  speed                   = 5 # 1Gb FD
}

# Trunk port — no override, profile controls VLAN membership
resource "omada_switch_port" "uplink" {
  site_id    = omada_site.example.id
  device_mac = "AA-BB-CC-DD-EE-FF"
  port       = 24

  profile_id = omada_port_profile.trunk_all.id
}

# Port mirroring (SPAN): mirror traffic from source ports 1,3,5,14,16 to
# destination port 12 (e.g. feeding a network sensor / IDS NIC).
resource "omada_switch_port" "sensor_mirror_dst" {
  site_id        = omada_site.example.id
  device_mac     = "B8-FB-B3-7F-45-C8"
  port           = 12
  operation      = "mirroring"
  mirrored_ports = [1, 3, 5, 14, 16]
}
