package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
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

// TestACLRule_Build_WanInIDs_LanToWan verifies that wan_in_ids from the plan
// is forwarded into Direction.WanInIDs and serializes correctly in JSON.
func TestACLRule_Build_WanInIDs_LanToWan(t *testing.T) {
	ctx := context.Background()

	protocols, _ := types.ListValueFrom(ctx, types.Int64Type, []int64{256})
	sourceIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"6a1a9eea44a75c2be56118a6"})
	destIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"6a1a9eeb44a75c2be56118bc"})
	wanInIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"3_155873a0d67448d880cc94324407c515"})

	plan := &ACLRuleResourceModel{
		Name:            types.StringValue("demo"),
		Type:            types.Int64Value(0),
		Status:          types.BoolValue(true),
		Policy:          types.Int64Value(0),
		Protocols:       protocols,
		SourceType:      types.Int64Value(1),
		SourceIDs:       sourceIDs,
		DestinationType: types.Int64Value(1),
		DestinationIDs:  destIDs,
		LanToWan:        types.BoolValue(true),
		LanToLan:        types.BoolValue(false),
		BiDirectional:   types.BoolValue(false),
		WanInIDs:        wanInIDs,
	}

	var errs []error
	got := buildACLRuleFromPlan(ctx, plan, &errs)
	if len(errs) > 0 {
		t.Fatalf("buildACLRuleFromPlan errors: %v", errs)
	}
	if got == nil {
		t.Fatal("buildACLRuleFromPlan returned nil")
	}

	// Direction.WanInIDs must contain the supplied UUID.
	if len(got.Direction.WanInIDs) != 1 || got.Direction.WanInIDs[0] != "3_155873a0d67448d880cc94324407c515" {
		t.Errorf("Direction.WanInIDs = %v; want [3_155873a0d67448d880cc94324407c515]", got.Direction.WanInIDs)
	}
	if !got.Direction.LanToWan {
		t.Error("Direction.LanToWan should be true")
	}

	// Verify JSON serialization contains correct wanInIds value.
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	direction, ok := decoded["direction"].(map[string]interface{})
	if !ok {
		t.Fatalf("direction field missing or wrong type in JSON: %s", body)
	}
	wanInIDsJSON, ok := direction["wanInIds"].([]interface{})
	if !ok {
		t.Fatalf("wanInIds missing or wrong type in direction JSON: %v", direction)
	}
	if len(wanInIDsJSON) != 1 || wanInIDsJSON[0] != "3_155873a0d67448d880cc94324407c515" {
		t.Errorf("wanInIds in JSON = %v; want [3_155873a0d67448d880cc94324407c515]", wanInIDsJSON)
	}
	lanToWan, _ := direction["lanToWan"].(bool)
	if !lanToWan {
		t.Error("direction.lanToWan should be true in JSON")
	}
}

// TestACLRule_Build_WanInIDs_Empty verifies that when wan_in_ids is an empty
// list, Direction.WanInIDs marshals as [] (not null).
func TestACLRule_Build_WanInIDs_Empty(t *testing.T) {
	ctx := context.Background()

	protocols, _ := types.ListValueFrom(ctx, types.Int64Type, []int64{256})
	sourceIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{})
	destIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{})
	wanInIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{})

	plan := &ACLRuleResourceModel{
		Name:            types.StringValue("empty-wan"),
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
		WanInIDs:        wanInIDs,
	}

	var errs []error
	got := buildACLRuleFromPlan(ctx, plan, &errs)
	if len(errs) > 0 {
		t.Fatalf("buildACLRuleFromPlan errors: %v", errs)
	}

	// Empty list must serialize as [] not null.
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	direction, ok := decoded["direction"].(map[string]interface{})
	if !ok {
		t.Fatalf("direction field missing in JSON: %s", body)
	}
	wanInIDsJSON, ok := direction["wanInIds"].([]interface{})
	if !ok {
		// Null or wrong type.
		t.Fatalf("wanInIds should be [] not null/missing in JSON: %v", direction)
	}
	if len(wanInIDsJSON) != 0 {
		t.Errorf("wanInIds should be empty, got %v", wanInIDsJSON)
	}
}

// TestACLRule_SetStateFromAPI_WanInIDs verifies that setStateFromAPI
// maps rule.Direction.WanInIDs back into model.WanInIDs.
func TestACLRule_SetStateFromAPI_WanInIDs(t *testing.T) {
	ctx := context.Background()

	rule := &client.ACLRule{
		ID:              "rule-1",
		Name:            "demo",
		Type:            0,
		Status:          true,
		Policy:          0,
		Protocols:       []int{256},
		SourceType:      1,
		SourceIDs:       []string{"6a1a9eea44a75c2be56118a6"},
		DestinationType: 1,
		DestinationIDs:  []string{"6a1a9eeb44a75c2be56118bc"},
		Direction: client.ACLDirection{
			WanInIDs: []string{"3_155873a0d67448d880cc94324407c515"},
			VpnInIDs: []string{},
			LanToWan: true,
			LanToLan: false,
		},
	}

	model := &ACLRuleResourceModel{}
	r := &ACLRuleResource{}
	r.setStateFromAPI(ctx, model, rule)

	var wanIDs []string
	if diags := model.WanInIDs.ElementsAs(ctx, &wanIDs, false); diags.HasError() {
		t.Fatalf("ElementsAs error: %v", diags)
	}
	if len(wanIDs) != 1 || wanIDs[0] != "3_155873a0d67448d880cc94324407c515" {
		t.Errorf("model.WanInIDs = %v; want [3_155873a0d67448d880cc94324407c515]", wanIDs)
	}
}

// =============================================================================
// WAN-IN + Network source validation (BUG 2 — plan-time guard, -33792)
// =============================================================================

// TestACLRule_ValidateWanInNetworkSource_RejectsNetworkSourceOnLanToWan verifies
// that validateWanInNetworkSource returns an error when lan_to_wan=true and
// source_type=0 (Network). The Omada controller rejects this combination with
// -33792 ("If the ACL direction is set to WAN IN, then the source cannot select
// SSID, Network or ! Network"). Catching it at plan time avoids a cryptic
// apply-time error.
func TestACLRule_ValidateWanInNetworkSource_RejectsNetworkSourceOnLanToWan(t *testing.T) {
	plan := &ACLRuleResourceModel{
		LanToWan:   types.BoolValue(true),
		SourceType: types.Int64Value(0), // 0 = Network
	}

	err := validateWanInNetworkSource(plan)
	if err == nil {
		t.Fatal("expected error for lan_to_wan=true + source_type=0 (Network), got nil")
	}
	if !contains(err.Error(), "-33792") {
		t.Errorf("error message should reference -33792, got: %s", err.Error())
	}
}

// TestACLRule_ValidateWanInNetworkSource_AllowsIPGroupSourceOnLanToWan verifies
// that the guard does NOT fire for lan_to_wan=true + source_type=1 (IP group).
// This is the correct combination for internet-bound rules.
func TestACLRule_ValidateWanInNetworkSource_AllowsIPGroupSourceOnLanToWan(t *testing.T) {
	plan := &ACLRuleResourceModel{
		LanToWan:   types.BoolValue(true),
		SourceType: types.Int64Value(1), // 1 = IP group (valid for WAN-IN)
	}

	if err := validateWanInNetworkSource(plan); err != nil {
		t.Errorf("expected no error for lan_to_wan=true + source_type=1, got: %v", err)
	}
}

// TestACLRule_ValidateWanInNetworkSource_AllowsNetworkSourceOnLanToLan verifies
// that the guard does NOT fire for lan_to_lan rules regardless of source_type.
// source_type=0 (Network) is valid and common on LAN-to-LAN rules.
func TestACLRule_ValidateWanInNetworkSource_AllowsNetworkSourceOnLanToLan(t *testing.T) {
	plan := &ACLRuleResourceModel{
		LanToWan:   types.BoolValue(false),
		SourceType: types.Int64Value(0), // Network source — fine on lan_to_lan
	}

	if err := validateWanInNetworkSource(plan); err != nil {
		t.Errorf("expected no error for lan_to_wan=false + source_type=0, got: %v", err)
	}
}

// contains is a helper used by tests in this file.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
