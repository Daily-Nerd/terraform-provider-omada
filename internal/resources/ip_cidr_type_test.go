package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// =============================================================================
// IPCIDRStringValue — StringSemanticEquals tests (RED → GREEN)
// =============================================================================

// TestIPCIDRStringValue_SemanticEquals_BareHostEqualsSlash32 asserts that a
// bare host config value ("10.10.70.98") is semantically equal to the
// canonical readback ("10.10.70.98/32") produced by setStateFromAPI.
func TestIPCIDRStringValue_SemanticEquals_BareHostEqualsSlash32(t *testing.T) {
	ctx := context.Background()

	config := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.70.98")}
	readback := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.70.98/32")}

	equal, diags := config.StringSemanticEquals(ctx, readback)
	if diags.HasError() {
		t.Fatalf("StringSemanticEquals diagnostics: %v", diags)
	}
	if !equal {
		t.Errorf("StringSemanticEquals(%q, %q) = false, want true", "10.10.70.98", "10.10.70.98/32")
	}
}

// TestIPCIDRStringValue_SemanticEquals_CIDRIdentical asserts that a CIDR value
// is semantically equal to itself.
func TestIPCIDRStringValue_SemanticEquals_CIDRIdentical(t *testing.T) {
	ctx := context.Background()

	v := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.10.0/24")}

	equal, diags := v.StringSemanticEquals(ctx, v)
	if diags.HasError() {
		t.Fatalf("StringSemanticEquals diagnostics: %v", diags)
	}
	if !equal {
		t.Errorf("StringSemanticEquals(%q, %q) = false, want true", "10.10.10.0/24", "10.10.10.0/24")
	}
}

// TestIPCIDRStringValue_SemanticEquals_DifferentHostNotEqual asserts that two
// different host addresses with /32 are NOT semantically equal.
func TestIPCIDRStringValue_SemanticEquals_DifferentHostNotEqual(t *testing.T) {
	ctx := context.Background()

	a := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.70.98")}
	b := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.70.99/32")}

	equal, diags := a.StringSemanticEquals(ctx, b)
	if diags.HasError() {
		t.Fatalf("StringSemanticEquals diagnostics: %v", diags)
	}
	if equal {
		t.Errorf("StringSemanticEquals(%q, %q) = true, want false", "10.10.70.98", "10.10.70.99/32")
	}
}

// TestIPCIDRStringValue_SemanticEquals_DifferentMaskNotEqual asserts that the
// same base address with different prefix lengths is NOT semantically equal.
func TestIPCIDRStringValue_SemanticEquals_DifferentMaskNotEqual(t *testing.T) {
	ctx := context.Background()

	a := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.10.0/24")}
	b := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.10.0/16")}

	equal, diags := a.StringSemanticEquals(ctx, b)
	if diags.HasError() {
		t.Fatalf("StringSemanticEquals diagnostics: %v", diags)
	}
	if equal {
		t.Errorf("StringSemanticEquals(%q, %q) = true, want false", "10.10.10.0/24", "10.10.10.0/16")
	}
}

// TestIPCIDRStringValue_SemanticEquals_ReversedBareHostEqualsSlash32 asserts
// symmetry: readback value ("/32") is also semantically equal to config ("")
// regardless of which side initiates the comparison.
func TestIPCIDRStringValue_SemanticEquals_ReversedBareHostEqualsSlash32(t *testing.T) {
	ctx := context.Background()

	readback := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.70.98/32")}
	config := IPCIDRStringValue{StringValue: basetypes.NewStringValue("10.10.70.98")}

	equal, diags := readback.StringSemanticEquals(ctx, config)
	if diags.HasError() {
		t.Fatalf("StringSemanticEquals diagnostics: %v", diags)
	}
	if !equal {
		t.Errorf("StringSemanticEquals(%q, %q) = false, want true", "10.10.70.98/32", "10.10.70.98")
	}
}
