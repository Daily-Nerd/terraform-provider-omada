package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Compile-time interface assertions.
var (
	_ basetypes.StringTypable                    = IPCIDRStringType{}
	_ basetypes.StringValuableWithSemanticEquals = IPCIDRStringValue{}
)

// IPCIDRStringType is a custom string type for IP/CIDR attributes. It creates
// IPCIDRStringValue instances that implement StringSemanticEquals so that a
// bare host address ("10.10.70.98") and its canonical CIDR form
// ("10.10.70.98/32") are treated as semantically equal — preventing perpetual
// diffs and "provider produced inconsistent result" errors without requiring a
// plan modifier to rewrite the config-set value.
type IPCIDRStringType struct {
	basetypes.StringType
}

// Equal returns true when the other type is also an IPCIDRStringType.
func (t IPCIDRStringType) Equal(o attr.Type) bool {
	_, ok := o.(IPCIDRStringType)
	return ok
}

// String returns a human-readable type name.
func (t IPCIDRStringType) String() string {
	return "IPCIDRStringType"
}

// ValueFromString wraps the base StringValue in an IPCIDRStringValue.
func (t IPCIDRStringType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return IPCIDRStringValue{StringValue: in}, nil
}

// ValueFromTerraform converts a tftypes.Value to an IPCIDRStringValue.
func (t IPCIDRStringType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}

	sv, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type of %T", attrValue)
	}

	valueable, diags := t.ValueFromString(ctx, sv)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to IPCIDRStringValue: %v", diags)
	}

	return valueable, nil
}

// ValueType returns the zero IPCIDRStringValue for type introspection.
func (t IPCIDRStringType) ValueType(_ context.Context) attr.Value {
	return IPCIDRStringValue{}
}

// IPCIDRStringValue is a string value that understands IP/CIDR semantic
// equality. It embeds basetypes.StringValue and adds StringSemanticEquals so
// that "10.10.70.98" and "10.10.70.98/32" compare as equal.
type IPCIDRStringValue struct {
	basetypes.StringValue
}

// Type returns IPCIDRStringType so Terraform associates this value with our
// custom type.
func (v IPCIDRStringValue) Type(_ context.Context) attr.Type {
	return IPCIDRStringType{}
}

// ToStringValue converts to the base StringValue (required by StringValuable).
func (v IPCIDRStringValue) ToStringValue(_ context.Context) (basetypes.StringValue, diag.Diagnostics) {
	return v.StringValue, nil
}

// Equal returns true only when the other value is an IPCIDRStringValue with
// the same raw string. Semantic equality is handled by StringSemanticEquals.
func (v IPCIDRStringValue) Equal(o attr.Value) bool {
	other, ok := o.(IPCIDRStringValue)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals returns true when both sides normalize to the same
// canonical "ip/mask" string. This is the mechanism that prevents Terraform
// from detecting a diff between config "10.10.70.98" and state "10.10.70.98/32".
func (v IPCIDRStringValue) StringSemanticEquals(_ context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	otherStr, d := other.ToStringValue(context.Background())
	diags.Append(d...)
	if diags.HasError() {
		return false, diags
	}

	return normalizeIPEntry(v.ValueString()) == normalizeIPEntry(otherStr.ValueString()), diags
}
