# Changelog

> **Fork point.** This changelog continues from `emanuelbesliu/terraform-provider-tplink-omada` v2.1.1. The Daily-Nerd fork resets versioning to `0.x.y` to signal a different lineage. Upstream history is preserved below for reference.

## [0.2.0](https://github.com/Daily-Nerd/terraform-provider-omada/compare/v0.1.0...v0.2.0) (2026-05-15)


### Features

* **network:** create purpose=interface networks via v6 openapi/v1 endpoint ([03d18fa](https://github.com/Daily-Nerd/terraform-provider-omada/commit/03d18faaad06cbd18c694b80d070711a8be7e313))


### Bug Fixes

* **network:** DHCP defaults ([#43](https://github.com/Daily-Nerd/terraform-provider-omada/issues/43)) + purpose RequiresReplace ([#45](https://github.com/Daily-Nerd/terraform-provider-omada/issues/45)) ([372707a](https://github.com/Daily-Nerd/terraform-provider-omada/commit/372707adb2b9768ca93ca021a8e8960fb04ce2ea))
* **network:** inject DHCP defaults to avoid API -1001 on interface flip ([039fbdd](https://github.com/Daily-Nerd/terraform-provider-omada/commit/039fbddd217eb16ada5df16d55ee8932df4e6ff2))
* **network:** mark `purpose` as RequiresReplace ([88487b3](https://github.com/Daily-Nerd/terraform-provider-omada/commit/88487b3f5626efbaf1ca23252448f2013a3f2585))
* **switch_port:** drop static defaults on speed and network_tags_setting ([f49af2e](https://github.com/Daily-Nerd/terraform-provider-omada/commit/f49af2e35c894c012fc9bccd5c6df3c5d7292d28))
* **switch_port:** drop static defaults on speed and network_tags_setting ([fb0fabe](https://github.com/Daily-Nerd/terraform-provider-omada/commit/fb0fabec1ee7f1c1f5cfcf052d409cc806135d78))

## 0.1.0 (2026-05-09)


### Features

* acceptance test infrastructure (closes [#7](https://github.com/Daily-Nerd/terraform-provider-omada/issues/7)) ([b807129](https://github.com/Daily-Nerd/terraform-provider-omada/commit/b807129f86644084e591d7eec8ecf8e7480b1dfa))
* acceptance test infrastructure (closes [#7](https://github.com/Daily-Nerd/terraform-provider-omada/issues/7)) ([d03db34](https://github.com/Daily-Nerd/terraform-provider-omada/commit/d03db3429956d30eb4c8b6d2a71165f32c5d27eb))
* add lan_interface_ids field to omada_network (closes [#5](https://github.com/Daily-Nerd/terraform-provider-omada/issues/5)) ([3f7bbb0](https://github.com/Daily-Nerd/terraform-provider-omada/commit/3f7bbb0e29fc867901e8340fa85f68825d9bb54d))
* add lan_interface_ids field to omada_network (closes [#5](https://github.com/Daily-Nerd/terraform-provider-omada/issues/5)) ([275b4f0](https://github.com/Daily-Nerd/terraform-provider-omada/commit/275b4f087d34e482f2949819b1bd93b1cf7ef0ab))
* add omada_gateway_ports data source (closes [#6](https://github.com/Daily-Nerd/terraform-provider-omada/issues/6)) ([949e7a2](https://github.com/Daily-Nerd/terraform-provider-omada/commit/949e7a28d6cc5a4c34dc336e5e8f7effc6db4d0b))
* add omada_gateway_ports data source (closes [#6](https://github.com/Daily-Nerd/terraform-provider-omada/issues/6)) ([90c4067](https://github.com/Daily-Nerd/terraform-provider-omada/commit/90c406754f69a45d2a21c73d437baceabcafe788))
* **client:** lazy provider authentication ([ed5815f](https://github.com/Daily-Nerd/terraform-provider-omada/commit/ed5815fcdd59b34e9e19abed774ecf23cf0759b5)), closes [#24](https://github.com/Daily-Nerd/terraform-provider-omada/issues/24)
* **client:** lazy provider authentication (closes [#24](https://github.com/Daily-Nerd/terraform-provider-omada/issues/24)) ([75b71a8](https://github.com/Daily-Nerd/terraform-provider-omada/commit/75b71a881105eed7db54fd9a533d5b12aab0712d))
* initial fork from emanuelbesliu/terraform-provider-tplink-omada v2.1.1 ([5a31cc7](https://github.com/Daily-Nerd/terraform-provider-omada/commit/5a31cc7f9abd4f33939689765d93d7f17bc56d42))
* **network:** surface 14 controller-exposed fields ([a5a0303](https://github.com/Daily-Nerd/terraform-provider-omada/commit/a5a0303805064ffaf060599a0ed5eb97976f0a62)), closes [#15](https://github.com/Daily-Nerd/terraform-provider-omada/issues/15)
* **network:** surface 14 missing controller fields (closes [#15](https://github.com/Daily-Nerd/terraform-provider-omada/issues/15)) ([3488872](https://github.com/Daily-Nerd/terraform-provider-omada/commit/3488872f6851e182a4b9de710937951da5a6cdce))
* **port_profile:** plan-time warnings for Easy-Managed-ignored fields ([8a68a6d](https://github.com/Daily-Nerd/terraform-provider-omada/commit/8a68a6d5e30189d21b1fd4acf48eabdff1b53e88))
* **port_profile:** plan-time warnings for Easy-Managed-ignored fields (Phase 2 of [#25](https://github.com/Daily-Nerd/terraform-provider-omada/issues/25)) ([26690e6](https://github.com/Daily-Nerd/terraform-provider-omada/commit/26690e6e6208532dd0f79b4de12c0cdd529f3e1b))
* **port_profile:** surface 24+ controller-exposed fields ([14f0370](https://github.com/Daily-Nerd/terraform-provider-omada/commit/14f03702d8e2ab96e727c2f0cdf7c5c8ae632207)), closes [#22](https://github.com/Daily-Nerd/terraform-provider-omada/issues/22)
* **port_profile:** surface 24+ missing controller fields (closes [#22](https://github.com/Daily-Nerd/terraform-provider-omada/issues/22)) ([731e098](https://github.com/Daily-Nerd/terraform-provider-omada/commit/731e098815dfca104e70d06d8d217939160594ab))
* **switch_port:** per-port resource for switch port config (closes [#23](https://github.com/Daily-Nerd/terraform-provider-omada/issues/23)) ([6886371](https://github.com/Daily-Nerd/terraform-provider-omada/commit/68863719da5f450d70e6ec48b8bfd669ff6dfa34))
* **switch_port:** per-port resource for switch port config (closes [#23](https://github.com/Daily-Nerd/terraform-provider-omada/issues/23)) ([94fbecc](https://github.com/Daily-Nerd/terraform-provider-omada/commit/94fbecca914a1d84763c9c91b6f95de12b2526c4))


### Bug Fixes

* data sources return empty list instead of null when controller has no items (closes [#16](https://github.com/Daily-Nerd/terraform-provider-omada/issues/16)) ([8ff43a3](https://github.com/Daily-Nerd/terraform-provider-omada/commit/8ff43a31ba964ab72387bf60167ea5838dd8f861))
* data sources return empty list instead of null when controller has no items (closes [#16](https://github.com/Daily-Nerd/terraform-provider-omada/issues/16)) ([aabd76d](https://github.com/Daily-Nerd/terraform-provider-omada/commit/aabd76dc63db6b1af692575b2189714c7872d43b))
* gofmt after module path rename ([36866fe](https://github.com/Daily-Nerd/terraform-provider-omada/commit/36866fe2d0b2d610d52994d2218fddef18d71404))
* registry namespace must be 'daily-nerd/omada' (hyphen preserved) ([0ab19ab](https://github.com/Daily-Nerd/terraform-provider-omada/commit/0ab19abac562b0cc237817efe221edae3ee48d02))
* run gofmt after module path rename ([0e9dfe4](https://github.com/Daily-Nerd/terraform-provider-omada/commit/0e9dfe41368163ad34a53bc12d69f18659a06761))
* use correct registry namespace 'daily-nerd/omada' (hyphen) ([0dfca97](https://github.com/Daily-Nerd/terraform-provider-omada/commit/0dfca978d196908c8202049c3b1fdfacee2f7e08))


### Miscellaneous Chores

* pin first release to 0.1.0 ([0afc589](https://github.com/Daily-Nerd/terraform-provider-omada/commit/0afc5894c066bef23434ccbd48481cd1dd0fe1ff))

## [Unreleased] — Daily-Nerd fork

### Added
- Forked from `emanuelbesliu/terraform-provider-tplink-omada` v2.1.1 (commit `9398b07`, 2026-04-09).
- Renamed Go module path to `github.com/Daily-Nerd/terraform-provider-omada`.
- Renamed Terraform Registry address to `daily-nerd/omada`.
- Renamed binary to `terraform-provider-omada`.
- Added MPL 2.0 LICENSE (upstream had no LICENSE file).
- Added NOTICE attributing upstream and recording fork lineage.

### Added
- `omada_port_profile` resource: surfaced 24+ previously-hidden controller fields. New top-level attributes:
  `untag_network_ids`, `bandwidth_ctrl_type`, `eee_enable`, `flow_control_enable`,
  `fast_leave_enable`, `loopback_detect_vlan_based_enable`, `igmp_fast_leave_enable`,
  `mld_fast_leave_enable`, `dot1p_priority`, `trust_mode`, `dhcp_l2_relay_enable`.
  Plus the full STP block flattened with `stp_` prefix: `stp_priority`, `stp_ext_path_cost`,
  `stp_int_path_cost`, `stp_edge_port`, `stp_p2p_link`, `stp_mcheck`, `stp_loop_protect`,
  `stp_root_protect`, `stp_tc_guard`, `stp_bpdu_protect`, `stp_bpdu_filter`,
  `stp_bpdu_forward`. Defaults match the controller's observed defaults so existing profiles
  round-trip cleanly. Easy Managed (Agile) switches silently ignore some of these — see #25.
  Closes [#22](https://github.com/Daily-Nerd/terraform-provider-omada/issues/22).
- `internal/resources/port_profile_test.go`: round-trip tests for `buildPortProfileFromModel`
  and `applyPortProfileToModel`, including null-vs-empty-list preservation for both
  `tag_network_ids` and `untag_network_ids`.
- `omada_network` resource: surfaced 14 controller-exposed fields previously hidden from the schema. New attributes:
  `application`, `vlan_type`, `isolation`, `fast_leave_enable`, `mld_snoop_enable`,
  `dhcpv6_guard_enable`, `dhcp_guard_enable`, `dhcp_l2_relay_enable`,
  `portal_enable`, `access_control_rule_enable`, `rate_limit_enable`,
  `arp_detection_enable`, `dhcp_lease_time`, `dhcp_dns_source`. All Optional+Computed
  with defaults matching the controller's observed defaults so existing networks
  round-trip cleanly. Closes [#15](https://github.com/Daily-Nerd/terraform-provider-omada/issues/15).
- `internal/resources/network_test.go`: round-trip tests for `buildNetworkFromModel`
  and `applyNetworkToModel`, including purpose=vlan null-preservation and full
  purpose=interface field coverage.
- `omada_switch_port` resource: per-port granular VLAN config via PATCH
  `/switches/{mac}/ports/{port}`. One Terraform resource per port, suitable
  for `for_each` iteration over a switch's port range. Distinct from
  `omada_device_switch.ports[]` which manages the whole switch in one resource.
  Schema covers profile_id, profile_override_enable, native_network_id,
  network_tags_setting, tag_network_ids, untag_network_ids, voice_network_enable,
  voice_dscp_enable, speed, name, disable. Delete is a no-op (ports cannot be
  destroyed) — drops resource from TF state with a warning. Import format:
  `{site_id}/{device_mac}/{port}`.
  Closes [#23](https://github.com/Daily-Nerd/terraform-provider-omada/issues/23).
- `client.GetSwitchPort(siteID, mac, port)` helper that fetches the full
  switch config and returns a single port by 1-based index.
- `internal/resources/switch_port_test.go`: 4 round-trip tests covering the
  full PATCH payload, optional-field omission semantics, model apply, and
  null-vs-empty-list preservation.
- `docs/SWITCH_CLASS_MATRIX.md`: per-switch-class compatibility matrix for
  `omada_port_profile` fields. Documents which fields are silently ignored
  on Easy Managed (Agile) and Easy Smart switches even though the controller
  accepts the API request — based on UI evidence captured on a TL-SG3218
  configured as Easy Managed. Closes Phase 1 of
  [#25](https://github.com/Daily-Nerd/terraform-provider-omada/issues/25).
  Phase 2 (plan-time soft warnings) and Phase 3 (opt-in hard validation)
  remain tracked under #25.
- `omada_port_profile` schema descriptions for `dot1x`, `lldp_med_enable`,
  `bandwidth_ctrl_type`, `dot1p_priority`, `trust_mode`, `dhcp_l2_relay_enable`
  now reference `docs/SWITCH_CLASS_MATRIX.md` so users see the caveat
  inline.
- `docs/CONTRIBUTING.md`: added switch-class compatibility section pointing
  to the new matrix; added `omada_switch_port` row to the dev-controller
  capability matrix.
- `omada_port_profile` `ValidateConfig`: emits non-blocking plan-time
  warnings when the user explicitly sets a field that the controller
  silently ignores on Easy Managed (Agile) switches. Covers `dot1x`,
  `lldp_med_enable`, `dot1p_priority`, `trust_mode`, `dhcp_l2_relay_enable`,
  `bandwidth_ctrl_type`. Defaults never trigger (zero-warning baseline);
  warnings fire only on "active" non-default values. Closes Phase 2 of
  [#25](https://github.com/Daily-Nerd/terraform-provider-omada/issues/25).
  Phase 3 (opt-in hard validation with switch-class API roundtrip) remains
  open if user demand surfaces.

### Changed
- **BEHAVIOR**: provider authentication is now lazy. The `Configure()` step no longer issues HTTP requests to `/api/info` or `/login`. Auth happens on the first real API call (resource read / write). `terraform validate` and `terraform plan` against configs whose resources resolve to `count = 0` or empty `for_each` no longer require controller credentials. Configuration errors (bad URL, bad credentials) surface at first API call instead of plan time. Closes [#24](https://github.com/Daily-Nerd/terraform-provider-omada/issues/24).
- All non-site-scoped API methods (sites CRUD, SAML IdP / role CRUD, controller certificate setting) now route through a new `doGlobalRequest` helper that gates auth via `ensureAuth`. This eliminates a class of latent races where URL construction read `c.token` before authentication completed.
- `UpdateSite` now uses the standard `doSiteRequest` helper instead of building its URL inline.
- `omada_port_profile` Create/Update no longer hardcode `SpanningTreeSetting{Priority: 128, BpduForward: true}`. STP fields are user-controllable (with backward-compatible defaults). Migration: existing state will round-trip cleanly because controller defaults already match the prior hardcoded values.
- `omada_network` Create/Update/Read/ImportState now share `buildNetworkFromModel` and `applyNetworkToModel` helpers. Eliminates the regression class where new client struct fields could land without schema exposure (the gap #15 was tracking).
- `client.Network` struct adds 11 new JSON-serialized fields and two nested guard structs (`DhcpV6Guard`, `DhcpGuard`). All zero-valued by default — no behavior change for callers that didn't set them.

### Planned for 0.1.0
- Add `lan_interface_ids` field to `omada_network` resource (fixes `-33515 LAN interfaces could not be none` on create).
- Add `omada_gateway_ports` data source for discovering valid LAN interface IDs.

---

## Upstream history (read-only)

## [2.1.1](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/compare/v2.1.0...v2.1.1) (2026-04-09)


### Bug Fixes

* **saml:** use list+filter for GetSAMLRole instead of unsupported direct GET ([760c71b](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/760c71b71ce72fa90ef0a5e0239483929cf43acd))

## [2.1.0](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/compare/v2.0.2...v2.1.0) (2026-04-09)


### Features

* add omada_controller_certificate resource for TLS certificate management ([2f3f5c7](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/2f3f5c757df5805299b2c4b7f093f4535b775f16))
* **saml:** add SAML IdP and SAML Role resources ([4cfdf80](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/4cfdf80de30a075f36b2bf540e20da163565508d))

## [2.0.1](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/compare/v2.0.0...v2.0.1) (2026-04-01)


### Bug Fixes

* make GPG passphrase optional in release workflows ([f11efba](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/f11efba299408f4ec584f11d2d61a61b0ffa1ba5))

## [2.0.0](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/compare/v1.0.0...v2.0.0) (2026-03-31)


### ⚠ BREAKING CHANGES

* All import ID formats changed to include siteID prefix. The provider 'site' attribute has been removed.

### Features

* add firewall ACL and IP group resources, data sources, and tests ([3a4b9dc](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/3a4b9dc9b5cf62515aa99b5bcde91ef017d8cc95))
* add mDNS reflector resource, data source, and tests ([836b1f1](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/836b1f180afd9fab9f1a01b3e516c76fde6c54d3))
* multi-site support and virtual/physical resource semantics ([ef13a9d](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/ef13a9d899a58993a3ca93286aac45eb2b8bbf5f))

## [1.0.0](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/compare/v0.1.3...v1.0.0) (2026-03-30)


### ⚠ BREAKING CHANGES

* correct provider registry address to emanuelbesliu/tplink-omada

### Features

* add CI workflow, release-please, dependabot, issue/PR templates, Makefile, and README ([ace2169](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/ace21697115699c8fb863f7c12b8ba011cd65e83))


### Bug Fixes

* correct provider registry address to emanuelbesliu/tplink-omada ([f482058](https://github.com/emanuelbesliu/terraform-provider-tplink-omada/commit/f482058b2dec101c854e4aa050d61e2ced68fa50))
