---
page_title: "omada_switch_port Resource - omada"
subcategory: ""
description: |-
  Manages a single port on an Omada-managed switch. Write path uses openapi/v1;
  read path uses api/v2. One resource per port for per-port granularity.
---

# omada_switch_port (Resource)

Manages a single port on an Omada-managed switch via the openapi/v1 PATCH endpoint.

Switch ports are not creatable or destroyable — they always exist on adopted hardware.
Resource semantics:
- **Create** = upsert: PATCH the port with the declared settings.
- **Update** = PATCH the port with the new settings.
- **Delete** = remove from Terraform state only. The port keeps its current settings on
  the switch (no API call). To revert a port to defaults, set `profile_override_enable=false`
  and clear the override fields, then `terraform apply`, then `terraform state rm`.

## Migration note (upgrading from v0.2.0 or earlier)

`untag_network_ids` is now **Computed-only** (read-only). Remove it from your HCL:

```diff
 resource "omada_switch_port" "uplink" {
   site_id    = var.site_id
   device_mac = var.switch_mac
   port       = 1
-  untag_network_ids = ["net-trusted"]
 }
```

Run `terraform plan` after the upgrade. All fields other than `untag_network_ids` should
show no diff for brownfield ports. If `profile_vlan_override_enable` appears as
known-after-apply on the first plan, run `terraform apply` once to settle state.

## Example Usage

```terraform
# Access port with per-port VLAN override (access_* profile)
resource "omada_switch_port" "k8s_node" {
  site_id    = omada_site.home.id
  device_mac = "AA-BB-CC-DD-EE-FF"
  port       = 5

  name                   = "k8s-node-1"
  profile_id             = omada_port_profile.access_trusted.id
  profile_override_enable = true
  native_network_id      = omada_network.trusted.id
  network_tags_setting   = 2 # access mode
  speed                  = 5 # 1Gb FD
}

# Trunk port — no override, profile controls VLAN membership
resource "omada_switch_port" "uplink" {
  site_id    = omada_site.home.id
  device_mac = "AA-BB-CC-DD-EE-FF"
  port       = 24

  profile_id = omada_port_profile.trunk_all.id
}
```

## Schema

### Required

- `device_mac` (String) MAC address of the switch device. Forces replacement when changed.
- `port` (Number) 1-based port number on the switch. Forces replacement when changed.
- `site_id` (String) The ID of the site this resource belongs to.

### Optional

- `disable` (Boolean) Administratively shut down the port. Default: `false`.
- `name` (String) Friendly port name shown in the Omada UI.
- `native_network_id` (String) Native (untagged / PVID) network ID for this port. Only honored when `profile_override_enable=true`.
- `network_tags_setting` (Number) VLAN tagging mode: 0=general (controller default), 1=trunk, 2=access. Only honored when `profile_override_enable=true`.
- `profile_id` (String) ID of the `omada_port_profile` to apply to this port.
- `profile_override_enable` (Boolean) When true, this port uses the per-port VLAN fields instead of the assigned profile. Default: `false`.
- `profile_vlan_override_enable` (Boolean) Per-port VLAN override enable. Automatically forced to `true` when `profile_override_enable=true` and `native_network_id` is set (required by access_\* profiles; omitting it returns controller error -39840). Computed from the controller on Read.
- `speed` (Number) Port speed code: 0=auto-negotiate, 1=10Mb HD, 2=10Mb FD, 3=100Mb HD, 4=100Mb FD, 5=1Gb FD, 6=2.5Gb FD, 7=5Gb FD, 8=10Gb FD. **Note:** codes 1, 2, 7, 8 have no confirmed openapi/v1 mapping on currently tested hardware (SG3218XP-M2) and silently fall back to auto-negotiate. Specific code support depends on the switch model.
- `tag_network_ids` (List of String) List of tagged VLAN network IDs. Only honored when `profile_override_enable=true`.
- `voice_dscp_enable` (Boolean) Enable DSCP marking for voice traffic on this port. Default: `false`.
- `voice_network_enable` (Boolean) Enable voice VLAN on this port. Default: `false`.

### Read-Only

- `id` (String) Synthetic resource ID — `{device_mac}:{port}`.
- `untag_network_ids` (List of String) Read-only list of untagged VLAN network IDs returned by the controller. The openapi/v1 write path does not accept this field — the controller derives untag=[native] automatically. **BREAKING CHANGE from v0.2.0:** remove `untag_network_ids` from HCL when upgrading.

## Import

Import using the format `{site_id}/{device_mac}/{port}`:

```shell
terraform import omada_switch_port.k8s_node "69fd06da.../AA-BB-CC-DD-EE-FF/5"
```
