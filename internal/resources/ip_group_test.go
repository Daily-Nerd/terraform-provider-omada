package resources

import (
	"context"
	"fmt"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// =============================================================================
// normalizeIPEntry — unit tests for the pure canonicalization helper
// =============================================================================

// TestNormalizeIPEntry_BareHostNormalizesToSlash32 asserts that a bare host IP
// (no "/" present) is normalized to "ip/32".
func TestNormalizeIPEntry_BareHostNormalizesToSlash32(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"10.10.70.98", "10.10.70.98/32"},
		{"8.8.8.8", "8.8.8.8/32"},
		{"192.168.1.1", "192.168.1.1/32"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeIPEntry(tc.input)
			if got != tc.want {
				t.Errorf("normalizeIPEntry(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeIPEntry_CIDRUnchanged asserts that a CIDR string is returned
// unchanged (it already contains a slash and a mask).
func TestNormalizeIPEntry_CIDRUnchanged(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"10.10.10.0/24", "10.10.10.0/24"},
		{"192.168.1.0/24", "192.168.1.0/24"},
		{"10.10.70.98/32", "10.10.70.98/32"},
		{"0.0.0.0/0", "0.0.0.0/0"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeIPEntry(tc.input)
			if got != tc.want {
				t.Errorf("normalizeIPEntry(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// =============================================================================
// Round-trip: config → wire → readback semantic equality
//
// These tests replace the former "plan modifier round-trip" tests. The plan
// modifier approach was invalid (plan modifier cannot change a config-set known
// value). The correct contract is now: config value is PRESERVED in state as-is
// (e.g. "10.10.70.98"), and when the API returns the canonical form
// ("10.10.70.98/32"), StringSemanticEquals treats them as equal — no diff.
// =============================================================================

// TestIPGroupEntry_RoundTrip_BareHostSemanticEqualsReadback verifies that the
// config value ("10.10.70.98") and the API readback ("10.10.70.98/32") are
// semantically equal via IPCIDRStringValue.StringSemanticEquals.
// This replaces the former round-trip test that relied on the (invalid) plan
// modifier normalizing the config value before comparison.
func TestIPGroupEntry_RoundTrip_BareHostSemanticEqualsReadback(t *testing.T) {
	ctx := context.Background()

	configValue := "10.10.70.98"

	// Step 1: SplitCIDR accepts bare host and produces {ip, mask:32}.
	ip, mask, err := client.SplitCIDR(configValue)
	if err != nil {
		t.Fatalf("SplitCIDR(%q): %v", configValue, err)
	}

	// Step 2: setStateFromAPI reconstructs CIDR string "10.10.70.98/32".
	readback := fmt.Sprintf("%s/%d", ip, mask)

	// Step 3: semantic equality must hold between config and readback.
	configVal := IPCIDRStringValue{StringValue: basetypes.NewStringValue(configValue)}
	readbackVal := IPCIDRStringValue{StringValue: basetypes.NewStringValue(readback)}

	equal, diags := configVal.StringSemanticEquals(ctx, readbackVal)
	if diags.HasError() {
		t.Fatalf("StringSemanticEquals diagnostics: %v", diags)
	}
	if !equal {
		t.Errorf("semantic equality: config=%q readback=%q — want semantically equal, got not equal", configValue, readback)
	}
}

// TestIPGroupEntry_RoundTrip_CIDRSemanticEqualsReadback verifies that CIDR
// inputs ("10.10.10.0/24") survive the round-trip and compare as semantically
// equal to their readback form (which is identical).
func TestIPGroupEntry_RoundTrip_CIDRSemanticEqualsReadback(t *testing.T) {
	ctx := context.Background()

	cases := []string{
		"10.10.10.0/24",
		"192.168.1.0/24",
		"10.10.70.98/32",
	}
	for _, configValue := range cases {
		t.Run(configValue, func(t *testing.T) {
			ip, mask, err := client.SplitCIDR(configValue)
			if err != nil {
				t.Fatalf("SplitCIDR(%q): %v", configValue, err)
			}
			readback := fmt.Sprintf("%s/%d", ip, mask)

			configVal := IPCIDRStringValue{StringValue: basetypes.NewStringValue(configValue)}
			readbackVal := IPCIDRStringValue{StringValue: basetypes.NewStringValue(readback)}

			equal, diags := configVal.StringSemanticEquals(ctx, readbackVal)
			if diags.HasError() {
				t.Fatalf("StringSemanticEquals diagnostics: %v", diags)
			}
			if !equal {
				t.Errorf("semantic equality: config=%q readback=%q — want semantically equal, got not equal", configValue, readback)
			}
		})
	}
}

// =============================================================================
// setStateFromAPI — always produces canonical ip/mask strings
// =============================================================================

// TestSetStateFromAPI_AlwaysProducesCanonicalCIDR verifies that setStateFromAPI
// always produces "ip/mask" strings for both /24 subnets and /32 host entries.
func TestSetStateFromAPI_AlwaysProducesCanonicalCIDR(t *testing.T) {
	r := &IPGroupResource{}
	ctx := context.Background()

	group := &client.IPGroup{
		ID:   "grp-1",
		Name: "Test Group",
		Type: 0,
		IPList: []client.IPGroupEntry{
			{IP: "10.10.50.0", Mask: 24, Description: ""},
			{IP: "10.10.70.98", Mask: 32, Description: ""},
		},
	}

	var model IPGroupResourceModel
	r.setStateFromAPI(ctx, &model, group)

	if len(model.IPList) != 2 {
		t.Fatalf("IPList len = %d, want 2", len(model.IPList))
	}

	want0 := "10.10.50.0/24"
	if got := model.IPList[0].IP.ValueString(); got != want0 {
		t.Errorf("IPList[0].IP = %q, want %q", got, want0)
	}

	want1 := "10.10.70.98/32"
	if got := model.IPList[1].IP.ValueString(); got != want1 {
		t.Errorf("IPList[1].IP = %q, want %q", got, want1)
	}
}
