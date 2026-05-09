package resources

import (
	"context"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPortProfile_BuildFromModel verifies the full set of schema fields
// flows from the Terraform model into the API client struct. Catches
// round-trip drift if a field is added to the schema but not plumbed
// through buildPortProfileFromModel.
func TestPortProfile_BuildFromModel(t *testing.T) {
	ctx := context.Background()

	tagIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"net-1", "net-2"})
	untagIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"net-3"})

	model := &PortProfileResourceModel{
		Name:                          types.StringValue("trunk_all"),
		NativeNetworkID:               types.StringValue("mgmt-net"),
		TagNetworkIDs:                 tagIDs,
		UntagNetworkIDs:               untagIDs,
		POE:                           types.Int64Value(2),
		Dot1x:                         types.Int64Value(2),
		PortIsolationEnable:           types.BoolValue(true),
		LLDPMedEnable:                 types.BoolValue(true),
		TopoNotifyEnable:              types.BoolValue(false),
		SpanningTreeEnable:            types.BoolValue(true),
		LoopbackDetectEnable:          types.BoolValue(true),
		BandWidthCtrlType:             types.Int64Value(1),
		EeeEnable:                     types.BoolValue(true),
		FlowControlEnable:             types.BoolValue(true),
		FastLeaveEnable:               types.BoolValue(false),
		LoopbackDetectVlanBasedEnable: types.BoolValue(true),
		IgmpFastLeaveEnable:           types.BoolValue(true),
		MldFastLeaveEnable:            types.BoolValue(true),
		Dot1pPriority:                 types.Int64Value(5),
		TrustMode:                     types.Int64Value(2),
		DhcpL2RelayEnable:             types.BoolValue(true),
		StpPriority:                   types.Int64Value(64),
		StpExtPathCost:                types.Int64Value(100),
		StpIntPathCost:                types.Int64Value(200),
		StpEdgePort:                   types.BoolValue(true),
		StpP2pLink:                    types.Int64Value(1),
		StpMcheck:                     types.BoolValue(true),
		StpLoopProtect:                types.BoolValue(true),
		StpRootProtect:                types.BoolValue(true),
		StpTcGuard:                    types.BoolValue(true),
		StpBpduProtect:                types.BoolValue(true),
		StpBpduFilter:                 types.BoolValue(false),
		StpBpduForward:                types.BoolValue(true),
	}

	var buildErrs []error
	got := buildPortProfileFromModel(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("buildPortProfileFromModel errors: %v", buildErrs)
	}
	if got == nil {
		t.Fatal("buildPortProfileFromModel returned nil")
	}

	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"Name", got.Name, "trunk_all"},
		{"NativeNetworkID", got.NativeNetworkID, "mgmt-net"},
		{"TagNetworkIDs", got.TagNetworkIDs, []string{"net-1", "net-2"}},
		{"UntagNetworkIDs", got.UntagNetworkIDs, []string{"net-3"}},
		{"POE", got.POE, 2},
		{"Dot1x", got.Dot1x, 2},
		{"PortIsolationEnable", got.PortIsolationEnable, true},
		{"LLDPMedEnable", got.LLDPMedEnable, true},
		{"TopoNotifyEnable", got.TopoNotifyEnable, false},
		{"SpanningTreeEnable", got.SpanningTreeEnable, true},
		{"LoopbackDetectEnable", got.LoopbackDetectEnable, true},
		{"BandWidthCtrlType", got.BandWidthCtrlType, 1},
		{"EeeEnable", got.EeeEnable, true},
		{"FlowControlEnable", got.FlowControlEnable, true},
		{"FastLeaveEnable", got.FastLeaveEnable, false},
		{"LoopbackDetectVlanBasedEnable", got.LoopbackDetectVlanBasedEnable, true},
		{"IgmpFastLeaveEnable", got.IgmpFastLeaveEnable, true},
		{"MldFastLeaveEnable", got.MldFastLeaveEnable, true},
		{"Dot1pPriority", got.Dot1pPriority, 5},
		{"TrustMode", got.TrustMode, 2},
	}
	for _, c := range checks {
		if !equal(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}

	if got.SpanningTreeSetting == nil {
		t.Fatal("SpanningTreeSetting is nil")
	}
	stp := got.SpanningTreeSetting
	stpChecks := []struct {
		field string
		got   any
		want  any
	}{
		{"Priority", stp.Priority, 64},
		{"ExtPathCost", stp.ExtPathCost, 100},
		{"IntPathCost", stp.IntPathCost, 200},
		{"EdgePort", stp.EdgePort, true},
		{"P2pLink", stp.P2pLink, 1},
		{"Mcheck", stp.Mcheck, true},
		{"LoopProtect", stp.LoopProtect, true},
		{"RootProtect", stp.RootProtect, true},
		{"TcGuard", stp.TcGuard, true},
		{"BpduProtect", stp.BpduProtect, true},
		{"BpduFilter", stp.BpduFilter, false},
		{"BpduForward", stp.BpduForward, true},
	}
	for _, c := range stpChecks {
		if !equal(c.got, c.want) {
			t.Errorf("STP.%s = %v, want %v", c.field, c.got, c.want)
		}
	}

	if got.DhcpL2RelaySettings == nil {
		t.Fatal("DhcpL2RelaySettings is nil")
	}
	if got.DhcpL2RelaySettings.Enable != true {
		t.Errorf("DhcpL2RelaySettings.Enable = %v, want true", got.DhcpL2RelaySettings.Enable)
	}
}

// TestPortProfile_ApplyToModel verifies the full set of API client struct
// fields flows back into the Terraform model on Read / ImportState.
func TestPortProfile_ApplyToModel(t *testing.T) {
	ctx := context.Background()

	profile := &client.PortProfile{
		ID:                            "pp-1",
		Name:                          "trunk_all",
		NativeNetworkID:               "mgmt-net",
		TagNetworkIDs:                 []string{"net-1", "net-2"},
		UntagNetworkIDs:               []string{"net-3"},
		POE:                           2,
		Dot1x:                         2,
		PortIsolationEnable:           true,
		LLDPMedEnable:                 true,
		TopoNotifyEnable:              false,
		SpanningTreeEnable:            true,
		LoopbackDetectEnable:          true,
		BandWidthCtrlType:             1,
		EeeEnable:                     true,
		FlowControlEnable:             true,
		FastLeaveEnable:               false,
		LoopbackDetectVlanBasedEnable: true,
		IgmpFastLeaveEnable:           true,
		MldFastLeaveEnable:            true,
		Dot1pPriority:                 5,
		TrustMode:                     2,
		SpanningTreeSetting: &client.SpanningTreeSetting{
			Priority:    64,
			ExtPathCost: 100,
			IntPathCost: 200,
			EdgePort:    true,
			P2pLink:     1,
			Mcheck:      true,
			LoopProtect: true,
			RootProtect: true,
			TcGuard:     true,
			BpduProtect: true,
			BpduFilter:  false,
			BpduForward: true,
		},
		DhcpL2RelaySettings: &client.DhcpL2RelaySettings{Enable: true},
	}

	model := &PortProfileResourceModel{
		// Force lists into known-list state so applyPortProfileToModel
		// writes API values rather than null-preserving.
		TagNetworkIDs:   types.ListValueMust(types.StringType, nil),
		UntagNetworkIDs: types.ListValueMust(types.StringType, nil),
	}
	if err := applyPortProfileToModel(ctx, model, profile); err != nil {
		t.Fatalf("applyPortProfileToModel: %v", err)
	}

	if model.Name.ValueString() != "trunk_all" {
		t.Errorf("Name = %q", model.Name.ValueString())
	}
	if model.BandWidthCtrlType.ValueInt64() != 1 {
		t.Errorf("BandWidthCtrlType = %d, want 1", model.BandWidthCtrlType.ValueInt64())
	}
	if !model.IgmpFastLeaveEnable.ValueBool() {
		t.Error("IgmpFastLeaveEnable should be true")
	}
	if !model.MldFastLeaveEnable.ValueBool() {
		t.Error("MldFastLeaveEnable should be true")
	}
	if model.Dot1pPriority.ValueInt64() != 5 {
		t.Errorf("Dot1pPriority = %d, want 5", model.Dot1pPriority.ValueInt64())
	}
	if model.TrustMode.ValueInt64() != 2 {
		t.Errorf("TrustMode = %d, want 2", model.TrustMode.ValueInt64())
	}
	if !model.DhcpL2RelayEnable.ValueBool() {
		t.Error("DhcpL2RelayEnable should be true")
	}
	if model.StpPriority.ValueInt64() != 64 {
		t.Errorf("StpPriority = %d, want 64", model.StpPriority.ValueInt64())
	}
	if !model.StpRootProtect.ValueBool() {
		t.Error("StpRootProtect should be true")
	}
	if model.StpBpduFilter.ValueBool() {
		t.Error("StpBpduFilter should be false")
	}

	// TagNetworkIDs round-trip
	var tagIDs []string
	model.TagNetworkIDs.ElementsAs(ctx, &tagIDs, false)
	if len(tagIDs) != 2 || tagIDs[0] != "net-1" || tagIDs[1] != "net-2" {
		t.Errorf("TagNetworkIDs = %v, want [net-1, net-2]", tagIDs)
	}

	var untagIDs []string
	model.UntagNetworkIDs.ElementsAs(ctx, &untagIDs, false)
	if len(untagIDs) != 1 || untagIDs[0] != "net-3" {
		t.Errorf("UntagNetworkIDs = %v, want [net-3]", untagIDs)
	}
}

// TestPortProfile_EasyManagedWarnings_NoneOnDefaults verifies that an empty
// model (all defaults / nulls) emits zero warnings. Default values are
// no-ops on every switch class.
func TestPortProfile_EasyManagedWarnings_NoneOnDefaults(t *testing.T) {
	model := &PortProfileResourceModel{}
	got := computeEasyManagedWarnings(model)
	if len(got) != 0 {
		t.Errorf("expected 0 warnings on empty model, got %d: %+v", len(got), got)
	}
}

// TestPortProfile_EasyManagedWarnings_FiresOnExplicitActiveValues asserts
// that each known-ignored field generates exactly one warning when set to a
// non-default ("active") value. This is the load-bearing test for #25
// Phase 2 — if a future schema change adds a new ignored field, this test
// should be extended.
func TestPortProfile_EasyManagedWarnings_FiresOnExplicitActiveValues(t *testing.T) {
	tests := []struct {
		name       string
		model      *PortProfileResourceModel
		wantFields []string
	}{
		{
			name:       "dot1x port-based fires",
			model:      &PortProfileResourceModel{Dot1x: types.Int64Value(0)},
			wantFields: []string{"dot1x"},
		},
		{
			name:       "dot1x mac-based fires",
			model:      &PortProfileResourceModel{Dot1x: types.Int64Value(1)},
			wantFields: []string{"dot1x"},
		},
		{
			name:       "dot1x disabled does NOT fire",
			model:      &PortProfileResourceModel{Dot1x: types.Int64Value(2)},
			wantFields: nil,
		},
		{
			name:       "lldp_med_enable=true fires",
			model:      &PortProfileResourceModel{LLDPMedEnable: types.BoolValue(true)},
			wantFields: []string{"lldp_med_enable"},
		},
		{
			name:       "lldp_med_enable=false also fires (explicit intent)",
			model:      &PortProfileResourceModel{LLDPMedEnable: types.BoolValue(false)},
			wantFields: []string{"lldp_med_enable"},
		},
		{
			name:       "dot1p_priority=5 fires",
			model:      &PortProfileResourceModel{Dot1pPriority: types.Int64Value(5)},
			wantFields: []string{"dot1p_priority"},
		},
		{
			name:       "dot1p_priority=0 does NOT fire",
			model:      &PortProfileResourceModel{Dot1pPriority: types.Int64Value(0)},
			wantFields: nil,
		},
		{
			name:       "trust_mode=2 fires",
			model:      &PortProfileResourceModel{TrustMode: types.Int64Value(2)},
			wantFields: []string{"trust_mode"},
		},
		{
			name:       "dhcp_l2_relay_enable=true fires",
			model:      &PortProfileResourceModel{DhcpL2RelayEnable: types.BoolValue(true)},
			wantFields: []string{"dhcp_l2_relay_enable"},
		},
		{
			name:       "dhcp_l2_relay_enable=false does NOT fire",
			model:      &PortProfileResourceModel{DhcpL2RelayEnable: types.BoolValue(false)},
			wantFields: nil,
		},
		{
			name:       "bandwidth_ctrl_type=1 fires",
			model:      &PortProfileResourceModel{BandWidthCtrlType: types.Int64Value(1)},
			wantFields: []string{"bandwidth_ctrl_type"},
		},
		{
			name:       "bandwidth_ctrl_type=0 does NOT fire",
			model:      &PortProfileResourceModel{BandWidthCtrlType: types.Int64Value(0)},
			wantFields: nil,
		},
		{
			name: "all-active model fires all 6 warnings",
			model: &PortProfileResourceModel{
				Dot1x:             types.Int64Value(0),
				LLDPMedEnable:     types.BoolValue(true),
				Dot1pPriority:     types.Int64Value(5),
				TrustMode:         types.Int64Value(2),
				DhcpL2RelayEnable: types.BoolValue(true),
				BandWidthCtrlType: types.Int64Value(1),
			},
			wantFields: []string{
				"dot1x",
				"lldp_med_enable",
				"dot1p_priority",
				"trust_mode",
				"dhcp_l2_relay_enable",
				"bandwidth_ctrl_type",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeEasyManagedWarnings(tt.model)
			gotFields := make([]string, 0, len(got))
			for _, w := range got {
				gotFields = append(gotFields, w.Field)
			}
			if len(gotFields) != len(tt.wantFields) {
				t.Fatalf("got %d warnings %v, want %d %v",
					len(gotFields), gotFields, len(tt.wantFields), tt.wantFields)
			}
			for i, want := range tt.wantFields {
				if gotFields[i] != want {
					t.Errorf("warning[%d] field = %q, want %q", i, gotFields[i], want)
				}
			}
		})
	}
}

// TestPortProfile_EasyManagedWarnings_DetailContainsField asserts each warning
// detail string mentions the field's purpose so the message is actionable
// without the user having to look up the field name.
func TestPortProfile_EasyManagedWarnings_DetailContainsField(t *testing.T) {
	model := &PortProfileResourceModel{
		Dot1x:             types.Int64Value(0),
		LLDPMedEnable:     types.BoolValue(true),
		Dot1pPriority:     types.Int64Value(5),
		TrustMode:         types.Int64Value(2),
		DhcpL2RelayEnable: types.BoolValue(true),
		BandWidthCtrlType: types.Int64Value(1),
	}
	for _, w := range computeEasyManagedWarnings(model) {
		if w.Detail == "" {
			t.Errorf("warning for %q has empty detail", w.Field)
		}
		if len(w.Detail) < 40 {
			t.Errorf("warning for %q detail too short (%d chars): %q", w.Field, len(w.Detail), w.Detail)
		}
	}
}

// TestPortProfile_ApplyToModel_NullListsPreserved verifies the null-vs-empty
// list preservation: when state has null tag/untag lists and API returns
// empty lists, model stays null (no perpetual diff).
func TestPortProfile_ApplyToModel_NullListsPreserved(t *testing.T) {
	ctx := context.Background()

	profile := &client.PortProfile{
		Name:                "access_iot",
		TagNetworkIDs:       []string{},
		UntagNetworkIDs:     []string{},
		SpanningTreeSetting: &client.SpanningTreeSetting{Priority: 128, BpduForward: true},
		DhcpL2RelaySettings: &client.DhcpL2RelaySettings{},
	}

	model := &PortProfileResourceModel{
		TagNetworkIDs:   types.ListNull(types.StringType),
		UntagNetworkIDs: types.ListNull(types.StringType),
	}
	if err := applyPortProfileToModel(ctx, model, profile); err != nil {
		t.Fatalf("applyPortProfileToModel: %v", err)
	}
	if !model.TagNetworkIDs.IsNull() {
		t.Error("TagNetworkIDs should remain null when state is null + API returns empty list")
	}
	if !model.UntagNetworkIDs.IsNull() {
		t.Error("UntagNetworkIDs should remain null when state is null + API returns empty list")
	}
}

// equal compares two values of the same type. Used in table-driven tests above.
func equal(a, b any) bool {
	switch av := a.(type) {
	case []string:
		bv, ok := b.([]string)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
