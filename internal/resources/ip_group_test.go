package resources

import (
	"context"
	"fmt"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
// Round-trip: config → wire → readback equality
// =============================================================================

// TestIPGroupEntry_RoundTrip_BareHostEqualsReadback verifies the full round-trip:
//
//	config "10.10.70.98" → normalizeIPEntry → "10.10.70.98/32"
//	SplitCIDR("10.10.70.98/32") → {ip:"10.10.70.98", mask:32}
//	API readback reconstructs → "10.10.70.98/32"
//	planned == readback
func TestIPGroupEntry_RoundTrip_BareHostEqualsReadback(t *testing.T) {
	configValue := "10.10.70.98"

	// Step 1: normalize at plan time (what the plan modifier does).
	planned := normalizeIPEntry(configValue)
	if planned != "10.10.70.98/32" {
		t.Fatalf("normalizeIPEntry(%q) = %q, want %q", configValue, planned, "10.10.70.98/32")
	}

	// Step 2: parse to wire format (what modelToIPList does).
	ip, mask, err := client.SplitCIDR(planned)
	if err != nil {
		t.Fatalf("SplitCIDR(%q): %v", planned, err)
	}

	// Step 3: reconstruct string (what setStateFromAPI does).
	readback := fmt.Sprintf("%s/%d", ip, mask)

	if readback != planned {
		t.Errorf("round-trip mismatch: planned=%q, readback=%q (want equal)", planned, readback)
	}
}

// TestIPGroupEntry_RoundTrip_CIDREqualsReadback verifies the same round-trip for
// CIDR inputs — "10.10.10.0/24" must survive unchanged.
func TestIPGroupEntry_RoundTrip_CIDREqualsReadback(t *testing.T) {
	cases := []string{
		"10.10.10.0/24",
		"192.168.1.0/24",
		"10.10.70.98/32",
	}
	for _, configValue := range cases {
		t.Run(configValue, func(t *testing.T) {
			planned := normalizeIPEntry(configValue)
			ip, mask, err := client.SplitCIDR(planned)
			if err != nil {
				t.Fatalf("SplitCIDR(%q): %v", planned, err)
			}
			readback := fmt.Sprintf("%s/%d", ip, mask)
			if readback != planned {
				t.Errorf("round-trip mismatch: planned=%q, readback=%q (want equal)", planned, readback)
			}
		})
	}
}

// =============================================================================
// Plan modifier — PlanModifyString behavior
// =============================================================================

// TestIPCIDRNormalizePlanModifier_BareHostNormalized asserts that the plan modifier
// rewrites a bare host IP to "ip/32" in the planned value.
func TestIPCIDRNormalizePlanModifier_BareHostNormalized(t *testing.T) {
	m := ipCIDRNormalize{}

	req := planmodifier.StringRequest{
		ConfigValue: types.StringValue("10.10.70.98"),
		PlanValue:   types.StringValue("10.10.70.98"),
		StateValue:  types.StringNull(),
	}
	resp := &planmodifier.StringResponse{
		PlanValue: req.PlanValue,
	}

	m.PlanModifyString(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("PlanModifyString diagnostics: %v", resp.Diagnostics)
	}
	want := "10.10.70.98/32"
	if got := resp.PlanValue.ValueString(); got != want {
		t.Errorf("PlanModifyString: PlanValue = %q, want %q", got, want)
	}
}

// TestIPCIDRNormalizePlanModifier_CIDRUnchanged asserts that an existing CIDR
// string passes through unchanged.
func TestIPCIDRNormalizePlanModifier_CIDRUnchanged(t *testing.T) {
	m := ipCIDRNormalize{}

	for _, cidr := range []string{"10.10.10.0/24", "10.10.70.98/32"} {
		t.Run(cidr, func(t *testing.T) {
			req := planmodifier.StringRequest{
				ConfigValue: types.StringValue(cidr),
				PlanValue:   types.StringValue(cidr),
				StateValue:  types.StringNull(),
			}
			resp := &planmodifier.StringResponse{
				PlanValue: req.PlanValue,
			}
			m.PlanModifyString(context.Background(), req, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("PlanModifyString diagnostics: %v", resp.Diagnostics)
			}
			if got := resp.PlanValue.ValueString(); got != cidr {
				t.Errorf("PlanModifyString(%q): PlanValue = %q, want unchanged %q", cidr, got, cidr)
			}
		})
	}
}

// TestIPCIDRNormalizePlanModifier_NullSkipped asserts that a null plan value is
// left untouched (no panic, no modification).
func TestIPCIDRNormalizePlanModifier_NullSkipped(t *testing.T) {
	m := ipCIDRNormalize{}

	req := planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		PlanValue:   types.StringNull(),
		StateValue:  types.StringNull(),
	}
	resp := &planmodifier.StringResponse{
		PlanValue: req.PlanValue,
	}
	m.PlanModifyString(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("PlanModifyString diagnostics: %v", resp.Diagnostics)
	}
	if !resp.PlanValue.IsNull() {
		t.Errorf("PlanModifyString(null): PlanValue = %q, want null", resp.PlanValue.ValueString())
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
