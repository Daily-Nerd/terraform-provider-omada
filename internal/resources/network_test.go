package resources

import (
	"context"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestNetwork_BuildFromModel_AllFields verifies every schema field on
// NetworkResourceModel flows into the API client struct. Catches drift
// when a field is added to the schema but not plumbed into the request
// payload.
func TestNetwork_BuildFromModel_AllFields(t *testing.T) {
	ctx := context.Background()

	ifaceIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"1_b59d", "2_2b95"})

	model := &NetworkResourceModel{
		Name:               types.StringValue("trusted"),
		Purpose:            types.StringValue("interface"),
		VlanID:             types.Int64Value(10),
		GatewaySubnet:      types.StringValue("10.10.10.1/24"),
		DHCPEnabled:        types.BoolValue(true),
		DHCPStart:          types.StringValue("10.10.10.100"),
		DHCPEnd:            types.StringValue("10.10.10.250"),
		DHCPLeaseTime:      types.Int64Value(720),
		DHCPDnsSource:      types.StringValue("auto"),
		IGMPSnoopEnable:    types.BoolValue(true),
		LanInterfaceIds:    ifaceIDs,
		Application:        types.Int64Value(0),
		VlanType:           types.Int64Value(0),
		Isolation:          types.BoolValue(true),
		FastLeaveEnable:    types.BoolValue(true),
		MldSnoopEnable:     types.BoolValue(true),
		DhcpV6GuardEnable:  types.BoolValue(true),
		DhcpGuardEnable:    types.BoolValue(true),
		DhcpL2RelayEnable:  types.BoolValue(true),
		PortalEnable:       types.BoolValue(false),
		AccessControlRule:  types.BoolValue(true),
		RateLimitEnable:    types.BoolValue(true),
		ArpDetectionEnable: types.BoolValue(true),
	}

	var buildErrs []error
	got := buildNetworkFromModel(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("buildNetworkFromModel errors: %v", buildErrs)
	}
	if got == nil {
		t.Fatal("buildNetworkFromModel returned nil")
	}

	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"Name", got.Name, "trusted"},
		{"Purpose", got.Purpose, "interface"},
		{"Vlan", got.Vlan, 10},
		{"GatewaySubnet", got.GatewaySubnet, "10.10.10.1/24"},
		{"IGMPSnoopEnable", got.IGMPSnoopEnable, true},
		{"Application", got.Application, 0},
		{"VlanType", got.VlanType, 0},
		{"Isolation", got.Isolation, true},
		{"FastLeaveEnable", got.FastLeaveEnable, true},
		{"MldSnoopEnable", got.MldSnoopEnable, true},
		{"DhcpL2RelayEnable", got.DhcpL2RelayEnable, true},
		{"Portal", got.Portal, false},
		{"AccessControlRule", got.AccessControlRule, true},
		{"RateLimit", got.RateLimit, true},
		{"ArpDetectionEnable", got.ArpDetectionEnable, true},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}

	if got.DhcpV6Guard == nil || !got.DhcpV6Guard.Enable {
		t.Error("DhcpV6Guard should be enabled")
	}
	if got.DhcpGuard == nil || !got.DhcpGuard.Enable {
		t.Error("DhcpGuard should be enabled")
	}

	if got.DHCPSettings == nil {
		t.Fatal("DHCPSettings is nil")
	}
	d := got.DHCPSettings
	if !d.Enable || d.IPAddrStart != "10.10.10.100" || d.IPAddrEnd != "10.10.10.250" {
		t.Errorf("DHCP basics wrong: %+v", d)
	}
	if d.LeaseTime != 720 {
		t.Errorf("LeaseTime = %d, want 720", d.LeaseTime)
	}
	if d.Dhcpns != "auto" {
		t.Errorf("Dhcpns = %q, want auto", d.Dhcpns)
	}

	if len(got.InterfaceIds) != 2 || got.InterfaceIds[0] != "1_b59d" || got.InterfaceIds[1] != "2_2b95" {
		t.Errorf("InterfaceIds = %v", got.InterfaceIds)
	}
}

// TestNetwork_ApplyToModel_VlanPurposeNullsDhcp verifies that purpose=vlan
// networks have gateway/dhcp fields nulled in state to avoid perpetual
// diff against L2-only VLANs.
func TestNetwork_ApplyToModel_VlanPurposeNullsDhcp(t *testing.T) {
	ctx := context.Background()

	n := &client.Network{
		ID:      "net-1",
		Name:    "iot",
		Purpose: "vlan",
		Vlan:    50,
		// Even if API returns these, they should be nulled because
		// purpose=vlan means L2-only.
		GatewaySubnet: "10.10.50.1/24",
		DHCPSettings:  &client.DHCPSettings{Enable: true, IPAddrStart: "x"},
	}

	state := &NetworkResourceModel{}
	if err := applyNetworkToModel(ctx, state, n); err != nil {
		t.Fatalf("applyNetworkToModel: %v", err)
	}

	if !state.GatewaySubnet.IsNull() {
		t.Error("GatewaySubnet should be null for purpose=vlan")
	}
	if !state.DHCPEnabled.IsNull() {
		t.Error("DHCPEnabled should be null for purpose=vlan")
	}
	if !state.DHCPStart.IsNull() {
		t.Error("DHCPStart should be null for purpose=vlan")
	}
	if !state.DHCPLeaseTime.IsNull() {
		t.Error("DHCPLeaseTime should be null for purpose=vlan")
	}
	if !state.DHCPDnsSource.IsNull() {
		t.Error("DHCPDnsSource should be null for purpose=vlan")
	}
}

// TestNetwork_ApplyToModel_InterfacePurposeAllFields verifies the full
// purpose=interface read path covers every newly-surfaced field.
func TestNetwork_ApplyToModel_InterfacePurposeAllFields(t *testing.T) {
	ctx := context.Background()

	n := &client.Network{
		ID:                 "net-1",
		Name:               "trusted",
		Purpose:            "interface",
		Vlan:               10,
		GatewaySubnet:      "10.10.10.1/24",
		IGMPSnoopEnable:    true,
		InterfaceIds:       []string{"1_b59d"},
		Application:        0,
		VlanType:           0,
		Isolation:          true,
		FastLeaveEnable:    true,
		MldSnoopEnable:     true,
		DhcpL2RelayEnable:  true,
		Portal:             true,
		AccessControlRule:  true,
		RateLimit:          true,
		ArpDetectionEnable: true,
		DhcpV6Guard:        &client.DhcpGuardSettings{Enable: true},
		DhcpGuard:          &client.DhcpGuardSettings{Enable: true},
		DHCPSettings: &client.DHCPSettings{
			Enable:      true,
			IPAddrStart: "10.10.10.100",
			IPAddrEnd:   "10.10.10.250",
			LeaseTime:   720,
			Dhcpns:      "manual",
		},
	}

	state := &NetworkResourceModel{}
	if err := applyNetworkToModel(ctx, state, n); err != nil {
		t.Fatalf("applyNetworkToModel: %v", err)
	}

	if !state.Isolation.ValueBool() {
		t.Error("Isolation should be true")
	}
	if !state.FastLeaveEnable.ValueBool() {
		t.Error("FastLeaveEnable should be true")
	}
	if !state.MldSnoopEnable.ValueBool() {
		t.Error("MldSnoopEnable should be true")
	}
	if !state.DhcpV6GuardEnable.ValueBool() {
		t.Error("DhcpV6GuardEnable should be true")
	}
	if !state.DhcpGuardEnable.ValueBool() {
		t.Error("DhcpGuardEnable should be true")
	}
	if !state.DhcpL2RelayEnable.ValueBool() {
		t.Error("DhcpL2RelayEnable should be true")
	}
	if !state.PortalEnable.ValueBool() {
		t.Error("PortalEnable should be true")
	}
	if !state.AccessControlRule.ValueBool() {
		t.Error("AccessControlRule should be true")
	}
	if !state.RateLimitEnable.ValueBool() {
		t.Error("RateLimitEnable should be true")
	}
	if !state.ArpDetectionEnable.ValueBool() {
		t.Error("ArpDetectionEnable should be true")
	}
	if state.DHCPLeaseTime.ValueInt64() != 720 {
		t.Errorf("DHCPLeaseTime = %d, want 720", state.DHCPLeaseTime.ValueInt64())
	}
	if state.DHCPDnsSource.ValueString() != "manual" {
		t.Errorf("DHCPDnsSource = %q, want manual", state.DHCPDnsSource.ValueString())
	}
}

// TestNetwork_BuildFromModel_DhcpDefaultsWhenUnset asserts that
// buildNetworkFromModel injects controller-friendly default values for
// `leasetime` and `dhcpns` when DHCP is enabled but those fields are null
// (i.e. unset by the caller). Without these defaults the controller
// rejects the PATCH with API error -1001 because `omitempty` drops the
// zero-value `leasetime: 0` and empty `dhcpns: ""`.
//
// Regression for: vlan->interface purpose flip on the live OC200 + ER707,
// homelab-network PR #23 first apply.
func TestNetwork_BuildFromModel_DhcpDefaultsWhenUnset(t *testing.T) {
	ctx := context.Background()

	ifaceIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"2_2b95"})

	model := &NetworkResourceModel{
		Name:          types.StringValue("cameras"),
		Purpose:       types.StringValue("interface"),
		VlanID:        types.Int64Value(60),
		GatewaySubnet: types.StringValue("10.10.60.1/24"),
		DHCPEnabled:   types.BoolValue(true),
		DHCPStart:     types.StringValue("10.10.60.100"),
		DHCPEnd:       types.StringValue("10.10.60.250"),
		// DHCPLeaseTime and DHCPDnsSource intentionally NOT set —
		// simulates the conf/home.tfvars usage where these are Computed
		// and unknown at plan time.
		DHCPLeaseTime:      types.Int64Null(),
		DHCPDnsSource:      types.StringNull(),
		IGMPSnoopEnable:    types.BoolValue(false),
		LanInterfaceIds:    ifaceIDs,
		Application:        types.Int64Value(0),
		VlanType:           types.Int64Value(0),
		Isolation:          types.BoolValue(false),
		FastLeaveEnable:    types.BoolValue(false),
		MldSnoopEnable:     types.BoolValue(false),
		DhcpV6GuardEnable:  types.BoolValue(false),
		DhcpGuardEnable:    types.BoolValue(false),
		DhcpL2RelayEnable:  types.BoolValue(false),
		PortalEnable:       types.BoolValue(false),
		AccessControlRule:  types.BoolValue(false),
		RateLimitEnable:    types.BoolValue(false),
		ArpDetectionEnable: types.BoolValue(false),
	}

	var buildErrs []error
	got := buildNetworkFromModel(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("buildNetworkFromModel errors: %v", buildErrs)
	}
	if got == nil || got.DHCPSettings == nil {
		t.Fatal("DHCPSettings is nil — should be populated with defaults")
	}

	if got.DHCPSettings.LeaseTime != 120 {
		t.Errorf("LeaseTime = %d, want 120 (controller default)", got.DHCPSettings.LeaseTime)
	}
	if got.DHCPSettings.Dhcpns != "auto" {
		t.Errorf("Dhcpns = %q, want \"auto\" (controller default)", got.DHCPSettings.Dhcpns)
	}
	if !got.DHCPSettings.Enable {
		t.Error("DHCPSettings.Enable should be true")
	}
	if got.DHCPSettings.IPAddrStart != "10.10.60.100" || got.DHCPSettings.IPAddrEnd != "10.10.60.250" {
		t.Errorf("DHCP range wrong: %+v", got.DHCPSettings)
	}
}

// TestNetwork_BuildFromModel_DhcpDefaultsRespectExplicitValues asserts
// that explicit user-supplied values for leasetime and dhcpns are NOT
// overridden by the default-injection logic. Defaults only fire when
// the field is zero/empty.
func TestNetwork_BuildFromModel_DhcpDefaultsRespectExplicitValues(t *testing.T) {
	ctx := context.Background()

	ifaceIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"2_2b95"})

	model := &NetworkResourceModel{
		Name:               types.StringValue("servers"),
		Purpose:            types.StringValue("interface"),
		VlanID:             types.Int64Value(70),
		GatewaySubnet:      types.StringValue("10.10.70.1/24"),
		DHCPEnabled:        types.BoolValue(true),
		DHCPStart:          types.StringValue("10.10.70.100"),
		DHCPEnd:            types.StringValue("10.10.70.250"),
		DHCPLeaseTime:      types.Int64Value(14400), // explicit
		DHCPDnsSource:      types.StringValue("manual"),
		IGMPSnoopEnable:    types.BoolValue(false),
		LanInterfaceIds:    ifaceIDs,
		Application:        types.Int64Value(0),
		VlanType:           types.Int64Value(0),
		Isolation:          types.BoolValue(false),
		FastLeaveEnable:    types.BoolValue(false),
		MldSnoopEnable:     types.BoolValue(false),
		DhcpV6GuardEnable:  types.BoolValue(false),
		DhcpGuardEnable:    types.BoolValue(false),
		DhcpL2RelayEnable:  types.BoolValue(false),
		PortalEnable:       types.BoolValue(false),
		AccessControlRule:  types.BoolValue(false),
		RateLimitEnable:    types.BoolValue(false),
		ArpDetectionEnable: types.BoolValue(false),
	}

	var buildErrs []error
	got := buildNetworkFromModel(ctx, model, &buildErrs)
	if len(buildErrs) > 0 || got == nil || got.DHCPSettings == nil {
		t.Fatalf("buildNetworkFromModel failed: errs=%v got=%v", buildErrs, got)
	}

	if got.DHCPSettings.LeaseTime != 14400 {
		t.Errorf("LeaseTime = %d, want 14400 (user-supplied, must not be overridden)", got.DHCPSettings.LeaseTime)
	}
	if got.DHCPSettings.Dhcpns != "manual" {
		t.Errorf("Dhcpns = %q, want \"manual\" (user-supplied, must not be overridden)", got.DHCPSettings.Dhcpns)
	}
}
