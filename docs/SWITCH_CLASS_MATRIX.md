# Switch Class Compatibility Matrix

TP-Link Omada-managed switches come in multiple classes with distinct feature
support. The Omada controller exposes a unified port-profile schema (see
`omada_port_profile`), but **some fields are silently ignored on lower-tier
switches** — set them in HCL, the controller accepts the request, but the
switch never honors the field.

This document catalogues which fields are honored per class so users don't
spend hours debugging why `dot1x = 0` "didn't work" on an Easy Managed switch.

> **Empirical source.** The Easy Managed exclusion list below comes from the
> Omada controller's own UI text on a TL-SG3218 (Build 20260401), captured
> while editing a port profile attached to that switch:
>
> > "Agile (Easy Managed) Switch does not support 802.1X Control, 802.1p
> > Priority, LLDP-MED, DHCP L2 Relay nor the Trust Mode function."
>
> The Smart Managed and Easy Smart rows are inferred from TP-Link's product
> line documentation. Community testing on specific models is welcome —
> open a PR amending the model lists below.

## Switch class taxonomy

| Class | Series | Feature tier |
|-------|--------|--------------|
| **Smart Managed** | JetStream L2+ (TL-SG3xxx, certain TL-SG2xxx) | Full feature set |
| **Easy Managed (Agile)** | JetStream L2 (most TL-SG2xxx, including SG3218 base) | Subset — no 802.1X, no QoS trust, no LLDP-MED, no DHCP L2 relay, no 802.1p priority |
| **Easy Smart** | JetStream Easy Smart (TL-SG1xxx, certain TL-SG10xx) | L2-lite — limited VLAN, no STP per port, no QoS |
| **Omada SDN Managed** | EAP / OC / ER series ports | Distinct flow — port profiles route via the AP / gateway resources, not `omada_port_profile` |

**Tip:** Check your switch's effective class in the Omada UI. Edit a port
profile attached to the switch — the dialog will surface a notice listing
unsupported fields if any.

## Field support matrix

`omada_port_profile` field mapped to switch-class behavior. ✅ = honored,
❌ = silently ignored, ⚠️ = honored with model-specific caveats.

| `omada_port_profile` attribute | Smart Managed | Easy Managed | Easy Smart |
|--------------------------------|:-------------:|:------------:|:----------:|
| `name` | ✅ | ✅ | ✅ |
| `native_network_id` | ✅ | ✅ | ✅ |
| `tag_network_ids` | ✅ | ✅ | ⚠️ (limited count) |
| `untag_network_ids` | ✅ | ✅ | ⚠️ |
| `poe` | ✅ (PoE models only) | ✅ (PoE models only) | ⚠️ |
| `dot1x` (802.1X Control) | ✅ | ❌ | ❌ |
| `port_isolation_enable` | ✅ | ✅ | ⚠️ |
| `lldp_med_enable` | ✅ | ❌ | ❌ |
| `topo_notify_enable` | ✅ | ✅ | ⚠️ |
| `spanning_tree_enable` | ✅ | ✅ | ⚠️ (global only on some) |
| `loopback_detect_enable` | ✅ | ✅ | ⚠️ |
| `bandwidth_ctrl_type` | ✅ | ⚠️ | ❌ |
| `eee_enable` (Energy Efficient Ethernet) | ✅ | ✅ | ⚠️ |
| `flow_control_enable` | ✅ | ✅ | ⚠️ |
| `fast_leave_enable` (legacy) | ✅ | ✅ | ❌ |
| `loopback_detect_vlan_based_enable` | ✅ | ⚠️ | ❌ |
| `igmp_fast_leave_enable` | ✅ | ✅ | ❌ |
| `mld_fast_leave_enable` | ✅ | ✅ | ❌ |
| `dot1p_priority` (802.1p) | ✅ | ❌ | ❌ |
| `trust_mode` (QoS trust) | ✅ | ❌ | ❌ |
| `dhcp_l2_relay_enable` | ✅ | ❌ | ❌ |
| All `stp_*` fields | ✅ | ⚠️ (global STP works; per-port may not) | ❌ |

## Practical guidance

### Smart Managed switches
Set whatever you need. The controller will deliver it.

### Easy Managed (Agile) switches
**Treat the following fields as no-ops** in HCL — they're accepted by the
controller but never reach the switch:

```hcl
# These have NO EFFECT on an Easy Managed switch:
dot1x                = 0
lldp_med_enable      = true
dhcp_l2_relay_enable = true
dot1p_priority       = 5
trust_mode           = 2
```

Setting them won't break anything (no `terraform plan` drift, no error), but
**don't rely on them for security or QoS guarantees**. If your design needs
802.1X port-based authentication or QoS trust modes, upgrade to a Smart
Managed switch.

### Easy Smart switches
Treat as L2-only. Skip all the advanced toggles. VLAN tagging works; little
else does. Use Smart Managed if you need any QoS, advanced multicast, or
802.1X.

## How to verify your switch's class

### Via Omada UI
1. **Devices** → click your switch
2. Look at the model number prefix (`TL-SG1xxx`, `TL-SG2xxx`, `TL-SG3xxx`)
3. Edit a port profile attached to this switch — read the notice in the
   dialog if one appears

### Via API
```bash
# Fetch device info
curl -k -b "$JAR" -H "Csrf-Token: $TOKEN" \
  "$OMADA_URL/$OMADAC_ID/api/v2/sites/$SITE_ID/devices?token=$TOKEN" \
  | jq '.result[] | select(.type=="switch") | {model, mac, name, status}'
```

The `model` field tells you the exact hardware. Cross-reference TP-Link's
spec sheet for definitive feature support.

## Contributing model-specific data

Empirical feedback welcome. If you've tested a specific TL-SG model and
discovered:
- A field that the matrix says ❌ but actually works on your switch
- A field that the matrix says ✅ but silently fails on your switch
- A new switch class not listed above

Open a PR amending this file with:
1. Model number + firmware version tested
2. The field that behaved differently than documented
3. Evidence (controller UI screenshot, raw API response, packet capture)

## Future work

This document is Phase 1 of [#25](https://github.com/Daily-Nerd/terraform-provider-omada/issues/25).
Possible Phase 2/3 enhancements:

- **Phase 2 — soft warnings**: when the resource detects a field set on a
  switch class that doesn't support it, emit a plan-time warning. Requires
  the resource to know which switch model the profile will attach to —
  expensive cross-resource lookup.
- **Phase 3 — opt-in hard validation**: per-resource `validate_switch_class
  = true` flag that errors at apply time if any field is set on an
  unsupported class. Costs an extra API roundtrip per resource — opt-in
  only.

Both phases are tracked under #25.
