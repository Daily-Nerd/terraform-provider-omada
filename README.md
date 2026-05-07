# Terraform Provider for TP-Link Omada — Daily-Nerd fork

Terraform provider for managing [TP-Link Omada Software Controller](https://www.tp-link.com/us/omada-sdn/) v6.x resources as infrastructure as code.

> **Fork notice.** This project is an actively maintained fork of [`emanuelbesliu/terraform-provider-tplink-omada`](https://github.com/emanuelbesliu/terraform-provider-tplink-omada) (last upstream tag `v2.1.1`, 2026-04-09). It exists because the upstream is community-maintained at low velocity and several resources we need (`omada_network` LAN interface binding, DHCP reservations, DNS records, advanced AP config) require gaps to be filled. Where reasonable, fixes will also be sent upstream.
>
> The upstream repo has **no LICENSE file**. Our fork is licensed under MPL 2.0 (see `LICENSE`); the lineage and attribution are recorded in `NOTICE`.

---

## Status

Pre-release. Versioning starts at `0.1.0` to signal a different lineage from upstream's `2.x` line. Breaking schema changes possible until `1.0.0`.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.11
- [Go](https://golang.org/doc/install) >= 1.25 (to build the provider)
- TP-Link Omada Software Controller 6.x (Docker image `mbentley/omada-controller` works for dev)

## Resources

| Resource | Description |
|---|---|
| `omada_controller_certificate` | Manages the TLS certificate for the Omada SDN Controller |
| `omada_site` | Manages sites on the controller |
| `omada_network` | Manages LAN networks (VLANs) |
| `omada_wireless_network` | Manages wireless SSIDs |
| `omada_wlan_group` | Manages WLAN groups |
| `omada_port_profile` | Manages switch port profiles |
| `omada_site_settings` | Manages site-level settings |
| `omada_device_ap` | Manages access point device configurations |
| `omada_device_switch` | Manages managed switch device configurations |
| `omada_firewall_acl` | Manages firewall ACL rules (gateway/switch/EAP) |
| `omada_ip_group` | Manages IP/port groups for use in ACLs |
| `omada_mdns_reflector` | Manages mDNS reflector configuration |
| `omada_saml_idp` | Manages SAML identity provider integration |
| `omada_saml_role` | Manages SAML role mappings |

## Data Sources

| Data Source | Description |
|---|---|
| `omada_sites` | Lists all sites |
| `omada_networks` | Lists networks for a site |
| `omada_wireless_networks` | Lists wireless SSIDs for a site |
| `omada_port_profiles` | Lists port profiles for a site |
| `omada_site_settings` | Reads site settings |
| `omada_devices` | Lists devices for a site |
| `omada_firewall_acls` | Lists firewall ACL rules |
| `omada_ip_groups` | Lists IP groups |
| `omada_mdns_reflectors` | Lists mDNS reflector entries |

## Installation

### From the Terraform Registry (when published)

```hcl
terraform {
  required_providers {
    omada = {
      source  = "dailynerd/omada"
      version = "~> 0.1"
    }
  }
}
```

### Building from Source

```sh
git clone https://github.com/Daily-Nerd/terraform-provider-omada.git
cd terraform-provider-omada
make build
```

### Local Development with `dev_overrides`

Add to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "dailynerd/omada" = "/path/to/terraform-provider-omada"
  }
  direct {}
}
```

Then build with `make dev`. `terraform init` warns when dev_overrides are active — expected; run `terraform plan` and `terraform apply` directly.

## Provider Configuration

```hcl
provider "omada" {
  url             = "https://192.168.1.1:8043"
  username        = "admin"
  password        = var.omada_password
  site            = "Default"
  skip_tls_verify = true
}
```

All attributes can also be set via environment variables:

| Attribute | Environment Variable |
|---|---|
| `url` | `OMADA_URL` |
| `username` | `OMADA_USERNAME` |
| `password` | `OMADA_PASSWORD` |
| `site` | `OMADA_SITE` |
| `skip_tls_verify` | `OMADA_SKIP_TLS_VERIFY` |

## Quick Example

```hcl
data "omada_sites" "all" {}

resource "omada_network" "iot" {
  site_id        = [for s in data.omada_sites.all.sites : s.id if s.name == "Default"][0]
  name           = "iot"
  purpose        = "interface"
  vlan_id        = 50
  gateway_subnet = "10.10.50.1/24"
  dhcp_enabled   = true
  dhcp_start     = "10.10.50.100"
  dhcp_end       = "10.10.50.250"
  # NOTE: lan_interface_ids field will be added in 0.1.0 — required by the
  # underlying API but missing in upstream v2.1.1. See issue tracker.
}
```

See the [`docs/`](docs/) directory for full resource and data source schemas.

## Development

### Build and Test

```sh
make build      # Build the provider binary
make test       # Run unit tests
make fmt        # Format Go source files
make fmtcheck   # Check formatting without modifying files
make vet        # Run go vet
make dev        # Build and print dev_overrides path
make clean      # Remove built binary
```

### Acceptance tests

Acceptance tests CREATE and DESTROY real controller resources. Run only against a dev controller, NEVER prod:

```sh
export TF_ACC=1
export OMADA_URL='https://<dev-controller>:8043'
export OMADA_USERNAME='admin'
export OMADA_PASSWORD='...'
export OMADA_SKIP_TLS_VERIFY=true

go test ./internal/resources/... -v -run TestAccOmadaNetwork
```

### Commit Conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for automated releases via [release-please](https://github.com/googleapis/release-please):

- `fix:` — patch release (bug fix)
- `feat:` — minor release (new feature)
- `feat!:` or `BREAKING CHANGE:` — major release

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Commit using conventional commits (`git commit -m 'feat: add my feature'`)
4. Push to your branch (`git push origin feat/my-feature`)
5. Open a Pull Request

## License

MPL 2.0 — see [`LICENSE`](LICENSE). Original upstream code from [`emanuelbesliu/terraform-provider-tplink-omada`](https://github.com/emanuelbesliu/terraform-provider-tplink-omada) was unlicensed; lineage and attribution recorded in [`NOTICE`](NOTICE).

## Acknowledgments

Built on the foundation laid by [Emanuel Besliu](https://github.com/emanuelbesliu)'s work. We're maintaining a fork because the homelab use case demands faster turnaround on a few specific gaps; not because the upstream is bad.
