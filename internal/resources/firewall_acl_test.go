package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestACLRule_Create_SendsEmptyCustomArrays verifies that the ACLRule built
// from a plan always has non-nil (empty) slices for the custom-ACL and
// direction arrays so they serialize as [] rather than null.
func TestACLRule_Create_SendsEmptyCustomArrays(t *testing.T) {
	ctx := context.Background()

	protocols, _ := types.ListValueFrom(ctx, types.Int64Type, []int64{256})
	sourceIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{})
	destIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{})

	plan := &ACLRuleResourceModel{
		Name:            types.StringValue("test-rule"),
		Type:            types.Int64Value(0),
		Status:          types.BoolValue(true),
		Policy:          types.Int64Value(1),
		Protocols:       protocols,
		SourceType:      types.Int64Value(0),
		SourceIDs:       sourceIDs,
		DestinationType: types.Int64Value(0),
		DestinationIDs:  destIDs,
		LanToWan:        types.BoolValue(false),
		LanToLan:        types.BoolValue(true),
		BiDirectional:   types.BoolValue(false),
	}

	var errs []error
	got := buildACLRuleFromPlan(ctx, plan, &errs)
	if len(errs) > 0 {
		t.Fatalf("buildACLRuleFromPlan errors: %v", errs)
	}
	if got == nil {
		t.Fatal("buildACLRuleFromPlan returned nil")
	}

	if got.CustomAclOsws == nil || len(got.CustomAclOsws) != 0 {
		t.Errorf("CustomAclOsws must be an empty non-nil slice (serialize as []), got %#v", got.CustomAclOsws)
	}
	if got.CustomAclStacks == nil || len(got.CustomAclStacks) != 0 {
		t.Errorf("CustomAclStacks must be an empty non-nil slice (serialize as []), got %#v", got.CustomAclStacks)
	}
	if got.CustomAclDevices == nil || len(got.CustomAclDevices) != 0 {
		t.Errorf("CustomAclDevices must be an empty non-nil slice (serialize as []), got %#v", got.CustomAclDevices)
	}
	if got.Direction.WanInIDs == nil || len(got.Direction.WanInIDs) != 0 {
		t.Errorf("Direction.WanInIDs must be an empty non-nil slice (serialize as []), got %#v", got.Direction.WanInIDs)
	}
	if got.Direction.VpnInIDs == nil || len(got.Direction.VpnInIDs) != 0 {
		t.Errorf("Direction.VpnInIDs must be an empty non-nil slice (serialize as []), got %#v", got.Direction.VpnInIDs)
	}
	if !got.Direction.LanToLan {
		t.Error("Direction.LanToLan should be true")
	}
}
