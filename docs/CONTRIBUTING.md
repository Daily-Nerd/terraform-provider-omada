# Contributing

## Development setup

### Prerequisites

- Go 1.25+ (`brew install go`)
- Terraform 1.11+ (`brew install terraform`)
- An Omada Software Controller for testing — Docker image works (`mbentley/omada-controller`), or a hardware controller (OC200, OC300)
- `jq`, `curl` for the discovery scripts

### Build the provider locally

```bash
make build
```

Drops the binary at `./terraform-provider-omada`.

### Use your local build with Terraform

Add to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "dailynerd/omada" = "/path/to/terraform-provider-omada"
  }
  direct {}
}
```

Use the **directory**, not the binary path. Terraform finds the binary in that directory by name.

When dev_overrides are active, `terraform init` warns and may behave oddly — skip it. Run `terraform plan` / `terraform apply` directly.

---

## Testing

### Unit tests (no controller required)

```bash
make test
```

Runs the client and resources unit tests. Safe to run anywhere, no network access.

### Acceptance tests (real controller required)

Acceptance tests create and destroy real resources on a controller. **Never run them against a production controller.**

#### Required environment

```bash
export OMADA_URL='https://<controller-ip>'        # OC200: port 443; Docker controller: port 8043
export OMADA_USERNAME='<admin-user>'
export OMADA_PASSWORD='<admin-password>'
export OMADA_TEST_SITE_ID='<24-char-site-hex-id>' # NOT the site name — the ID
export OMADA_SKIP_TLS_VERIFY='true'               # for self-signed dev certs
```

#### Find your site ID

```bash
./scripts/api-discover.sh sites
cat dist/api-discover/sites-list.json | jq '.result.data[] | {id, name}'
```

Use the `id` field, not `name`.

#### Run the suite

```bash
make testacc                    # all acceptance tests
make testacc-data               # data sources only (read-only, safest)
make testacc-network            # network resource lifecycle
```

The `testacc` target enforces required env vars and fails fast if missing.

#### Naming convention for test resources

All acceptance tests use `TF_ACC_TEST_*` prefixed names. Tests clean up automatically via `defer` / Terraform's destroy phase, but if a test crashes mid-run you may need to clean manually via the controller UI.

### Adding new acceptance tests

Pattern:

```go
func TestAccResourceFoo_CRUD(t *testing.T) {
    siteID := testSiteID(t)  // helper — fails the test if OMADA_TEST_SITE_ID unset

    resource.Test(t, resource.TestCase{
        PreCheck:                 func() { testAccPreCheck(t) },
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: fmt.Sprintf(`
resource "omada_foo" "test" {
  site_id = %q
  name    = "TF_ACC_TEST_FOO"
  // ...
}`, siteID),
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttrSet("omada_foo.test", "id"),
                    resource.TestCheckResourceAttr("omada_foo.test", "name", "TF_ACC_TEST_FOO"),
                ),
            },
            // Optional: update step
            // Optional: import step (ImportState/ImportStateVerify)
        },
    })
}
```

Helpers in `internal/provider/provider_acc_test.go`:

- `testAccProtoV6ProviderFactories` — provider factory map
- `testAccPreCheck(t)` — asserts required env vars
- `testSiteID(t)` — returns `OMADA_TEST_SITE_ID` or fails the test

### What the dev controller can and cannot validate

| Resource | Empty Docker controller | OC200 with no devices | Real hardware |
|----------|------------------------|----------------------|---------------|
| `omada_network` (`purpose=vlan`) | ✅ creates | ✅ creates | ✅ creates |
| `omada_network` (`purpose=interface`) | ❌ -33515 | ❌ -33515 | ✅ requires `lan_interface_ids` |
| `omada_wireless_network` | ✅ definition | ✅ definition | ✅ broadcasts |
| `omada_port_profile` | ✅ definition | ✅ definition | ✅ assignment (some fields silently ignored on Easy Managed switches — see below) |
| `omada_device_switch` | ❌ no device | ❌ no device | ✅ requires adoption |
| `omada_switch_port` | ❌ no device | ❌ no device | ✅ requires adoption |
| `omada_firewall_acl` | partial | partial | full |

`purpose=vlan` networks work everywhere because they're L2-only and don't need gateway binding. Use those as your default test fixture.

### Switch class compatibility

The Omada controller silently ignores certain `omada_port_profile` fields on Easy Managed (Agile) switches even though it accepts the API request. See [`docs/SWITCH_CLASS_MATRIX.md`](SWITCH_CLASS_MATRIX.md) for the per-class field-support matrix and the empirical evidence behind it.

Notable exclusions on Easy Managed switches: `dot1x`, `lldp_med_enable`, `dot1p_priority`, `trust_mode`, `dhcp_l2_relay_enable`. If you're adding a port-profile feature, consult the matrix and flag the affected fields in their schema descriptions.

---

## Discovery scripts

Use these when adding new resources or fields and you need to know the API shape.

### `scripts/api-discover.sh`

Logs into a controller and dumps key resource payloads to `dist/api-discover/`.

```bash
./scripts/api-discover.sh                # default: networks
./scripts/api-discover.sh all            # everything we know about
./scripts/api-discover.sh networks       # specific endpoint
```

Read `dist/api-discover/*.json` to find field names and shapes.

### `scripts/test-network-create.sh`

Probes how the controller validates `omada_network` payloads — useful for understanding which fields are truly required vs Optional in the API. Cleans up after each case.

```bash
./scripts/test-network-create.sh
```

### `scripts/audit-network-schema.sh`

Systematically tests each `omada_network` field by omitting it from a known-good payload and recording the controller's response. Output: `dist/audit/network-schema-report.{tsv,json}`.

```bash
./scripts/audit-network-schema.sh
```

Use this when you suspect a field marked `Optional` is actually required by the API. Last run found `igmpSnoopEnable` is the only such field; all others are truly optional.

---

## Commit conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for automated releases via release-please:

- `feat:` — new feature (minor version bump)
- `fix:` — bug fix (patch version bump)
- `feat!:` or `BREAKING CHANGE:` footer — breaking change (major version bump while in pre-1.0 → minor bump)
- `chore:`, `docs:`, `test:`, `refactor:` — no version bump

Reference issue numbers in commit body: `Closes #5`.

---

## Pull request checklist

- [ ] `make fmtcheck` passes
- [ ] `make vet` passes
- [ ] `make test` passes
- [ ] If touching a resource/data source: `make testacc-<name>` passes (or note in PR why it can't run)
- [ ] Documentation regenerated if schema changed: `tfplugindocs generate`
- [ ] CHANGELOG entry under Unreleased (or via Conventional Commit message)
- [ ] If new resource/field, add an acceptance test (see pattern above)

---

## Common pitfalls

### Test resource names collide

Acceptance tests run sequentially by default. If you `make testacc` and a previous run crashed leaving a `TF_ACC_TEST_NET` behind, the next run will conflict. Clean up manually via the controller UI or run `make testacc-data` first to confirm the controller is reachable.

### `terraform init` warns about dev_overrides

Expected. dev_overrides bypass init. Run `plan`/`apply` directly without `init`.

### `purpose=interface` networks fail with -33515 on dev

Controller requires non-empty `lan_interface_ids` for interface networks. Without an adopted gateway, no valid IDs exist. Use `purpose=vlan` for dev tests until hardware is adopted.

### OC200 vs Docker controller port

- OC200: HTTPS on port 443 (no port suffix needed in URL)
- Docker `mbentley/omada-controller`: HTTPS on port 8043

Verify with `curl -sk <url>/api/info` — should return JSON with `controllerVer`.
