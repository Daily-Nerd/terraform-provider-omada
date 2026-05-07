# Changelog

> **Fork point.** This changelog continues from `emanuelbesliu/terraform-provider-tplink-omada` v2.1.1. The Daily-Nerd fork resets versioning to `0.x.y` to signal a different lineage. Upstream history is preserved below for reference.

## 1.0.0 (2026-05-07)


### Features

* initial fork from emanuelbesliu/terraform-provider-tplink-omada v2.1.1 ([5a31cc7](https://github.com/Daily-Nerd/terraform-provider-omada/commit/5a31cc7f9abd4f33939689765d93d7f17bc56d42))

## [Unreleased] — Daily-Nerd fork

### Added
- Forked from `emanuelbesliu/terraform-provider-tplink-omada` v2.1.1 (commit `9398b07`, 2026-04-09).
- Renamed Go module path to `github.com/Daily-Nerd/terraform-provider-omada`.
- Renamed Terraform Registry address to `dailynerd/omada`.
- Renamed binary to `terraform-provider-omada`.
- Added MPL 2.0 LICENSE (upstream had no LICENSE file).
- Added NOTICE attributing upstream and recording fork lineage.

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
