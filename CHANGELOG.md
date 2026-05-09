# Changelog

> **Fork point.** This changelog continues from `emanuelbesliu/terraform-provider-tplink-omada` v2.1.1. The Daily-Nerd fork resets versioning to `0.x.y` to signal a different lineage. Upstream history is preserved below for reference.

## [Unreleased] — Daily-Nerd fork

### Added
- Forked from `emanuelbesliu/terraform-provider-tplink-omada` v2.1.1 (commit `9398b07`, 2026-04-09).
- Renamed Go module path to `github.com/Daily-Nerd/terraform-provider-omada`.
- Renamed Terraform Registry address to `dailynerd/omada`.
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
