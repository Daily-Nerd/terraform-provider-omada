package resources

import (
	"context"
	"fmt"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &PortProfileResource{}
var _ resource.ResourceWithImportState = &PortProfileResource{}
var _ resource.ResourceWithValidateConfig = &PortProfileResource{}

type PortProfileResource struct {
	client *client.Client
}

type PortProfileResourceModel struct {
	ID                            types.String `tfsdk:"id"`
	SiteID                        types.String `tfsdk:"site_id"`
	Name                          types.String `tfsdk:"name"`
	NativeNetworkID               types.String `tfsdk:"native_network_id"`
	TagNetworkIDs                 types.List   `tfsdk:"tag_network_ids"`
	UntagNetworkIDs               types.List   `tfsdk:"untag_network_ids"`
	POE                           types.Int64  `tfsdk:"poe"`
	Dot1x                         types.Int64  `tfsdk:"dot1x"`
	PortIsolationEnable           types.Bool   `tfsdk:"port_isolation_enable"`
	LLDPMedEnable                 types.Bool   `tfsdk:"lldp_med_enable"`
	TopoNotifyEnable              types.Bool   `tfsdk:"topo_notify_enable"`
	SpanningTreeEnable            types.Bool   `tfsdk:"spanning_tree_enable"`
	LoopbackDetectEnable          types.Bool   `tfsdk:"loopback_detect_enable"`
	BandWidthCtrlType             types.Int64  `tfsdk:"bandwidth_ctrl_type"`
	EeeEnable                     types.Bool   `tfsdk:"eee_enable"`
	FlowControlEnable             types.Bool   `tfsdk:"flow_control_enable"`
	FastLeaveEnable               types.Bool   `tfsdk:"fast_leave_enable"`
	LoopbackDetectVlanBasedEnable types.Bool   `tfsdk:"loopback_detect_vlan_based_enable"`
	IgmpFastLeaveEnable           types.Bool   `tfsdk:"igmp_fast_leave_enable"`
	MldFastLeaveEnable            types.Bool   `tfsdk:"mld_fast_leave_enable"`
	Dot1pPriority                 types.Int64  `tfsdk:"dot1p_priority"`
	TrustMode                     types.Int64  `tfsdk:"trust_mode"`
	DhcpL2RelayEnable             types.Bool   `tfsdk:"dhcp_l2_relay_enable"`

	// SpanningTreeSetting flattened with stp_ prefix.
	StpPriority    types.Int64 `tfsdk:"stp_priority"`
	StpExtPathCost types.Int64 `tfsdk:"stp_ext_path_cost"`
	StpIntPathCost types.Int64 `tfsdk:"stp_int_path_cost"`
	StpEdgePort    types.Bool  `tfsdk:"stp_edge_port"`
	StpP2pLink     types.Int64 `tfsdk:"stp_p2p_link"`
	StpMcheck      types.Bool  `tfsdk:"stp_mcheck"`
	StpLoopProtect types.Bool  `tfsdk:"stp_loop_protect"`
	StpRootProtect types.Bool  `tfsdk:"stp_root_protect"`
	StpTcGuard     types.Bool  `tfsdk:"stp_tc_guard"`
	StpBpduProtect types.Bool  `tfsdk:"stp_bpdu_protect"`
	StpBpduFilter  types.Bool  `tfsdk:"stp_bpdu_filter"`
	StpBpduForward types.Bool  `tfsdk:"stp_bpdu_forward"`
}

func NewPortProfileResource() resource.Resource {
	return &PortProfileResource{}
}

func (r *PortProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_profile"
}

func (r *PortProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a switch port profile on the Omada Controller.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the port profile.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": siteIDResourceSchema(),
			"name": schema.StringAttribute{
				Description: "The name of the port profile.",
				Required:    true,
			},
			"native_network_id": schema.StringAttribute{
				Description: "The native (untagged) network ID. Required for trunk profiles.",
				Required:    true,
			},
			"tag_network_ids": schema.ListAttribute{
				Description: "List of tagged network IDs for trunk profiles.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"poe": schema.Int64Attribute{
				Description: "PoE setting: 0=disabled, 1=enabled, 2=use profile default.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(2),
			},
			"dot1x": schema.Int64Attribute{
				Description: "802.1X setting: 0=port-based, 1=mac-based, 2=disabled. " +
					"Not supported on Easy Managed (Agile) switches — see docs/SWITCH_CLASS_MATRIX.md.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(2),
			},
			"port_isolation_enable": schema.BoolAttribute{
				Description: "Enable port isolation.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"lldp_med_enable": schema.BoolAttribute{
				Description: "Enable LLDP-MED. " +
					"Not supported on Easy Managed (Agile) switches — see docs/SWITCH_CLASS_MATRIX.md.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"topo_notify_enable": schema.BoolAttribute{
				Description: "Enable topology change notification.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"spanning_tree_enable": schema.BoolAttribute{
				Description: "Enable Spanning Tree Protocol.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"loopback_detect_enable": schema.BoolAttribute{
				Description: "Enable loopback detection.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"untag_network_ids": schema.ListAttribute{
				Description: "List of network IDs to untag on this port (separate from native_network_id). " +
					"Used by some controller versions to express multiple untagged VLANs on a single profile.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"bandwidth_ctrl_type": schema.Int64Attribute{
				Description: "Bandwidth control type: 0=disabled, 1=rate-limit, 2=storm-control. " +
					"Not supported on Easy Managed (Agile) switches — see docs/SWITCH_CLASS_MATRIX.md.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(0),
			},
			"eee_enable": schema.BoolAttribute{
				Description: "Enable Energy Efficient Ethernet (802.3az).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"flow_control_enable": schema.BoolAttribute{
				Description: "Enable 802.3x flow control.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"fast_leave_enable": schema.BoolAttribute{
				Description: "Legacy multicast fast-leave toggle. Newer controllers use " +
					"`igmp_fast_leave_enable` and `mld_fast_leave_enable` instead — set those " +
					"and leave this at the default unless you know your controller relies on it.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"loopback_detect_vlan_based_enable": schema.BoolAttribute{
				Description: "Enable per-VLAN loopback detection.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"igmp_fast_leave_enable": schema.BoolAttribute{
				Description: "Enable IGMP (IPv4 multicast) fast-leave.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"mld_fast_leave_enable": schema.BoolAttribute{
				Description: "Enable MLD (IPv6 multicast) fast-leave.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"dot1p_priority": schema.Int64Attribute{
				Description: "Default 802.1p priority (0..7). " +
					"Not supported on Easy Managed switches — see docs/SWITCH_CLASS_MATRIX.md.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(0),
			},
			"trust_mode": schema.Int64Attribute{
				Description: "QoS trust mode: 0=untrusted, 1=trust 802.1p, 2=trust DSCP. " +
					"Not supported on Easy Managed switches — see docs/SWITCH_CLASS_MATRIX.md.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(0),
			},
			"dhcp_l2_relay_enable": schema.BoolAttribute{
				Description: "Enable DHCP Layer-2 relay. " +
					"Not supported on Easy Managed switches — see docs/SWITCH_CLASS_MATRIX.md.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"stp_priority": schema.Int64Attribute{
				Description: "STP bridge priority (0..240, must be a multiple of 16). Default 128.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(128),
			},
			"stp_ext_path_cost": schema.Int64Attribute{
				Description: "STP external path cost. 0 = use default.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"stp_int_path_cost": schema.Int64Attribute{
				Description: "STP internal path cost. 0 = use default.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"stp_edge_port": schema.BoolAttribute{
				Description: "Treat the port as an STP edge port (skip listening/learning, transition directly to forwarding).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"stp_p2p_link": schema.Int64Attribute{
				Description: "STP point-to-point link mode: 0=auto, 1=force-true, 2=force-false.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"stp_mcheck": schema.BoolAttribute{
				Description: "Force STP migration check (force-RSTP/MSTP renegotiation).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"stp_loop_protect": schema.BoolAttribute{
				Description: "Enable STP loop protection.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"stp_root_protect": schema.BoolAttribute{
				Description: "Enable STP root bridge protection (prevents this port from becoming root).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"stp_tc_guard": schema.BoolAttribute{
				Description: "Enable STP topology change guard.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"stp_bpdu_protect": schema.BoolAttribute{
				Description: "Enable BPDU protection — shut the port down on BPDU receipt.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"stp_bpdu_filter": schema.BoolAttribute{
				Description: "Filter BPDUs on this port (drop them silently). Use cautiously — interacts with `stp_edge_port`.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"stp_bpdu_forward": schema.BoolAttribute{
				Description: "Forward BPDUs on this port even when STP is disabled. Default true (matches controller default).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
		},
	}
}

func (r *PortProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

// easyManagedIgnoredHint is the standard tail appended to every warning emitted
// by ValidateConfig for fields known to be silently ignored on Easy Managed
// (Agile) switches.
const easyManagedIgnoredHint = "If this profile is attached only to Smart Managed switches, " +
	"this warning is safe to ignore. See docs/SWITCH_CLASS_MATRIX.md for the " +
	"full per-class field-support matrix and how to verify your switch's class."

// easyManagedWarning is a single plan-time warning produced by the
// Easy-Managed-aware config validator. The Field is the schema attribute
// path (a single root attribute name for now), Detail is the body of the
// warning. Both get appended to the standard `easyManagedIgnoredHint`
// when surfaced via diagnostics.
type easyManagedWarning struct {
	Field  string
	Detail string
}

// computeEasyManagedWarnings returns the list of warnings to emit for fields
// the controller accepts but Easy Managed (Agile) switches silently ignore.
// Pure function over the model — testable without a tfsdk Config harness.
func computeEasyManagedWarnings(m *PortProfileResourceModel) []easyManagedWarning {
	var out []easyManagedWarning

	// dot1x: 0=port-based, 1=mac-based, 2=disabled (default).
	// Active values (0, 1) intend to enforce 802.1X. Easy Managed silently drops.
	if !m.Dot1x.IsNull() && !m.Dot1x.IsUnknown() && m.Dot1x.ValueInt64() != 2 {
		out = append(out, easyManagedWarning{
			Field: "dot1x",
			Detail: "802.1X port authentication is not supported on Easy Managed switches. " +
				"The controller accepts the setting but the switch never enforces it.",
		})
	}

	// lldp_med_enable: both true and false are intent. Easy Managed lacks
	// LLDP-MED entirely. Warn whenever the user explicitly sets it.
	if !m.LLDPMedEnable.IsNull() && !m.LLDPMedEnable.IsUnknown() {
		out = append(out, easyManagedWarning{
			Field: "lldp_med_enable",
			Detail: "LLDP-MED is not supported on Easy Managed switches. " +
				"The controller accepts the toggle but the switch ignores it.",
		})
	}

	// dot1p_priority: 0..7. Default 0 = no marking. 1..7 means active intent.
	if !m.Dot1pPriority.IsNull() && !m.Dot1pPriority.IsUnknown() && m.Dot1pPriority.ValueInt64() != 0 {
		out = append(out, easyManagedWarning{
			Field:  "dot1p_priority",
			Detail: "802.1p default priority is not supported on Easy Managed switches.",
		})
	}

	// trust_mode: 0=untrusted (default), 1=trust 802.1p, 2=trust DSCP.
	if !m.TrustMode.IsNull() && !m.TrustMode.IsUnknown() && m.TrustMode.ValueInt64() != 0 {
		out = append(out, easyManagedWarning{
			Field: "trust_mode",
			Detail: "QoS trust mode is not supported on Easy Managed switches. " +
				"Tagged traffic will not be reclassified per the trust setting.",
		})
	}

	// dhcp_l2_relay_enable: default false. true is active intent.
	if !m.DhcpL2RelayEnable.IsNull() && !m.DhcpL2RelayEnable.IsUnknown() && m.DhcpL2RelayEnable.ValueBool() {
		out = append(out, easyManagedWarning{
			Field: "dhcp_l2_relay_enable",
			Detail: "DHCP Layer-2 relay is not supported on Easy Managed switches. " +
				"DHCP traffic will pass through but option-82 information will not be inserted.",
		})
	}

	// bandwidth_ctrl_type: 0=disabled (default), 1=rate-limit, 2=storm-control.
	// Easy Managed has partial support — warn on non-default to surface caveat.
	if !m.BandWidthCtrlType.IsNull() && !m.BandWidthCtrlType.IsUnknown() && m.BandWidthCtrlType.ValueInt64() != 0 {
		out = append(out, easyManagedWarning{
			Field: "bandwidth_ctrl_type",
			Detail: "Bandwidth control may not behave as expected on Easy Managed switches. " +
				"Specific support varies by switch model — verify in the controller UI.",
		})
	}

	return out
}

// ValidateConfig emits plan-time warnings (never errors) when the user sets a
// field that is silently ignored on Easy Managed (Agile) switches. Warnings
// are non-blocking and do not stop the apply — they're informational signals
// so users notice the controller-silent failure mode without having to debug
// it after the fact.
//
// Why warnings instead of errors: a port profile may legitimately target only
// Smart Managed switches in a heterogeneous environment, where these fields
// are honored. Erroring would block valid configurations.
//
// This is the HashiCorp-documented pattern for "suboptimal situations and
// possible unexpected behaviors" (Terraform Plugin Framework Diagnostics).
func (r *PortProfileResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data PortProfileResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, w := range computeEasyManagedWarnings(&data) {
		resp.Diagnostics.AddAttributeWarning(
			path.Root(w.Field),
			"Field silently ignored on Easy Managed (Agile) switches",
			w.Detail+"\n\n"+easyManagedIgnoredHint,
		)
	}
}

// buildPortProfileFromModel converts the Terraform plan / state model into the
// API client struct for create + update.
func buildPortProfileFromModel(ctx context.Context, m *PortProfileResourceModel, diags *[]error) *client.PortProfile {
	profile := &client.PortProfile{
		Name:                          m.Name.ValueString(),
		NativeNetworkID:               m.NativeNetworkID.ValueString(),
		POE:                           int(m.POE.ValueInt64()),
		Dot1x:                         int(m.Dot1x.ValueInt64()),
		PortIsolationEnable:           m.PortIsolationEnable.ValueBool(),
		LLDPMedEnable:                 m.LLDPMedEnable.ValueBool(),
		TopoNotifyEnable:              m.TopoNotifyEnable.ValueBool(),
		SpanningTreeEnable:            m.SpanningTreeEnable.ValueBool(),
		LoopbackDetectEnable:          m.LoopbackDetectEnable.ValueBool(),
		BandWidthCtrlType:             int(m.BandWidthCtrlType.ValueInt64()),
		EeeEnable:                     m.EeeEnable.ValueBool(),
		FlowControlEnable:             m.FlowControlEnable.ValueBool(),
		FastLeaveEnable:               m.FastLeaveEnable.ValueBool(),
		LoopbackDetectVlanBasedEnable: m.LoopbackDetectVlanBasedEnable.ValueBool(),
		IgmpFastLeaveEnable:           m.IgmpFastLeaveEnable.ValueBool(),
		MldFastLeaveEnable:            m.MldFastLeaveEnable.ValueBool(),
		Dot1pPriority:                 int(m.Dot1pPriority.ValueInt64()),
		TrustMode:                     int(m.TrustMode.ValueInt64()),
		SpanningTreeSetting: &client.SpanningTreeSetting{
			Priority:    int(m.StpPriority.ValueInt64()),
			ExtPathCost: int(m.StpExtPathCost.ValueInt64()),
			IntPathCost: int(m.StpIntPathCost.ValueInt64()),
			EdgePort:    m.StpEdgePort.ValueBool(),
			P2pLink:     int(m.StpP2pLink.ValueInt64()),
			Mcheck:      m.StpMcheck.ValueBool(),
			LoopProtect: m.StpLoopProtect.ValueBool(),
			RootProtect: m.StpRootProtect.ValueBool(),
			TcGuard:     m.StpTcGuard.ValueBool(),
			BpduProtect: m.StpBpduProtect.ValueBool(),
			BpduFilter:  m.StpBpduFilter.ValueBool(),
			BpduForward: m.StpBpduForward.ValueBool(),
		},
		DhcpL2RelaySettings: &client.DhcpL2RelaySettings{
			Enable: m.DhcpL2RelayEnable.ValueBool(),
		},
	}

	if !m.TagNetworkIDs.IsNull() && !m.TagNetworkIDs.IsUnknown() {
		var tagIDs []string
		d := m.TagNetworkIDs.ElementsAs(ctx, &tagIDs, false)
		if d.HasError() {
			for _, e := range d.Errors() {
				*diags = append(*diags, fmt.Errorf("%s: %s", e.Summary(), e.Detail()))
			}
			return nil
		}
		profile.TagNetworkIDs = tagIDs
	}

	if !m.UntagNetworkIDs.IsNull() && !m.UntagNetworkIDs.IsUnknown() {
		var untagIDs []string
		d := m.UntagNetworkIDs.ElementsAs(ctx, &untagIDs, false)
		if d.HasError() {
			for _, e := range d.Errors() {
				*diags = append(*diags, fmt.Errorf("%s: %s", e.Summary(), e.Detail()))
			}
			return nil
		}
		profile.UntagNetworkIDs = untagIDs
	}

	return profile
}

// applyPortProfileToModel writes the API client struct back into the
// Terraform model on Read / ImportState. Preserves null vs empty-list
// semantics for tag/untag lists when the user did not declare them.
func applyPortProfileToModel(ctx context.Context, m *PortProfileResourceModel, profile *client.PortProfile) error {
	m.Name = types.StringValue(profile.Name)
	m.NativeNetworkID = types.StringValue(profile.NativeNetworkID)
	m.POE = types.Int64Value(int64(profile.POE))
	m.Dot1x = types.Int64Value(int64(profile.Dot1x))
	m.PortIsolationEnable = types.BoolValue(profile.PortIsolationEnable)
	m.LLDPMedEnable = types.BoolValue(profile.LLDPMedEnable)
	m.TopoNotifyEnable = types.BoolValue(profile.TopoNotifyEnable)
	m.SpanningTreeEnable = types.BoolValue(profile.SpanningTreeEnable)
	m.LoopbackDetectEnable = types.BoolValue(profile.LoopbackDetectEnable)
	m.BandWidthCtrlType = types.Int64Value(int64(profile.BandWidthCtrlType))
	m.EeeEnable = types.BoolValue(profile.EeeEnable)
	m.FlowControlEnable = types.BoolValue(profile.FlowControlEnable)
	m.FastLeaveEnable = types.BoolValue(profile.FastLeaveEnable)
	m.LoopbackDetectVlanBasedEnable = types.BoolValue(profile.LoopbackDetectVlanBasedEnable)
	m.IgmpFastLeaveEnable = types.BoolValue(profile.IgmpFastLeaveEnable)
	m.MldFastLeaveEnable = types.BoolValue(profile.MldFastLeaveEnable)
	m.Dot1pPriority = types.Int64Value(int64(profile.Dot1pPriority))
	m.TrustMode = types.Int64Value(int64(profile.TrustMode))

	if profile.DhcpL2RelaySettings != nil {
		m.DhcpL2RelayEnable = types.BoolValue(profile.DhcpL2RelaySettings.Enable)
	} else {
		m.DhcpL2RelayEnable = types.BoolValue(false)
	}

	if profile.SpanningTreeSetting != nil {
		s := profile.SpanningTreeSetting
		m.StpPriority = types.Int64Value(int64(s.Priority))
		m.StpExtPathCost = types.Int64Value(int64(s.ExtPathCost))
		m.StpIntPathCost = types.Int64Value(int64(s.IntPathCost))
		m.StpEdgePort = types.BoolValue(s.EdgePort)
		m.StpP2pLink = types.Int64Value(int64(s.P2pLink))
		m.StpMcheck = types.BoolValue(s.Mcheck)
		m.StpLoopProtect = types.BoolValue(s.LoopProtect)
		m.StpRootProtect = types.BoolValue(s.RootProtect)
		m.StpTcGuard = types.BoolValue(s.TcGuard)
		m.StpBpduProtect = types.BoolValue(s.BpduProtect)
		m.StpBpduFilter = types.BoolValue(s.BpduFilter)
		m.StpBpduForward = types.BoolValue(s.BpduForward)
	}

	// Preserve null vs empty-list semantics for both tag lists.
	if len(profile.TagNetworkIDs) == 0 && m.TagNetworkIDs.IsNull() {
		m.TagNetworkIDs = types.ListNull(types.StringType)
	} else {
		tagIDs, diags := types.ListValueFrom(ctx, types.StringType, profile.TagNetworkIDs)
		if diags.HasError() {
			return fmt.Errorf("decoding tag_network_ids: %v", diags)
		}
		m.TagNetworkIDs = tagIDs
	}

	if len(profile.UntagNetworkIDs) == 0 && m.UntagNetworkIDs.IsNull() {
		m.UntagNetworkIDs = types.ListNull(types.StringType)
	} else {
		untagIDs, diags := types.ListValueFrom(ctx, types.StringType, profile.UntagNetworkIDs)
		if diags.HasError() {
			return fmt.Errorf("decoding untag_network_ids: %v", diags)
		}
		m.UntagNetworkIDs = untagIDs
	}

	return nil
}

func (r *PortProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PortProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := plan.SiteID.ValueString()

	var buildErrs []error
	profile := buildPortProfileFromModel(ctx, &plan, &buildErrs)
	if len(buildErrs) > 0 {
		for _, e := range buildErrs {
			resp.Diagnostics.AddError("Building port profile payload", e.Error())
		}
		return
	}

	created, err := r.client.CreatePortProfile(ctx, siteID, profile)
	if err != nil {
		resp.Diagnostics.AddError("Error creating port profile", err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PortProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PortProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()

	profile, err := r.client.GetPortProfile(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading port profile", err.Error())
		return
	}

	if err := applyPortProfileToModel(ctx, &state, profile); err != nil {
		resp.Diagnostics.AddError("Error decoding port profile", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PortProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PortProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state PortProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()

	var buildErrs []error
	profile := buildPortProfileFromModel(ctx, &plan, &buildErrs)
	if len(buildErrs) > 0 {
		for _, e := range buildErrs {
			resp.Diagnostics.AddError("Building port profile payload", e.Error())
		}
		return
	}

	// Route through the openapi/v2 endpoint. The legacy /api/v2
	// PATCH path returns errorCode -33854 ("The VLAN configuration
	// for this profile has been disabled in the new UI") on v6
	// controllers once the profile is marked as managed by the new
	// UI. UpdatePortProfileV2 is the documented unlock — see
	// internal/client/client.go for the rationale and capture trail.
	//
	// We read-merge against the current legacy GET to seed every
	// field the v6 body requires (including ones the user did not
	// declare in TF), then overlay the plan values for the attributes
	// the resource models. This keeps the rest of the controller's
	// per-profile state intact across a TF apply.
	profileID := state.ID.ValueString()
	current, err := r.client.GetPortProfile(ctx, siteID, profileID)
	if err != nil {
		resp.Diagnostics.AddError("Error loading current port profile for update", err.Error())
		return
	}

	body := portProfileV2FromPlan(current, profile, profileID)

	if _, err := r.client.UpdatePortProfileV2(ctx, siteID, profileID, body); err != nil {
		resp.Diagnostics.AddError("Error updating port profile", err.Error())
		return
	}

	plan.ID = state.ID
	plan.SiteID = state.SiteID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// portProfileV2FromPlan builds the openapi/v2 body from the current
// controller read-back and the plan-derived PortProfile. The read-back
// supplies safe defaults for v2-only fields; the plan overlays every
// attribute the resource models so user edits actually land.
//
// Lives in this file (not in the client package) because it depends on
// the resource's plan shape — the client package's portProfileToV2
// helper handles the pure read-back → v2 translation; we overlay the
// plan fields on top of it here.
func portProfileV2FromPlan(current, plan *client.PortProfile, profileID string) *client.PortProfileV2 {
	// Translate the GET read-back as the baseline (carries v2-only
	// defaults the legacy API does not surface).
	body := portProfileToV2Adapter(current, profileID)

	// Overlay every field the resource models so the v6 PATCH carries
	// the user's intent. UpdatePortProfileV2 will force
	// VlanConfigEnable = true regardless of what we set here.
	body.Name = plan.Name
	body.POE = plan.POE
	body.Dot1x = plan.Dot1x
	body.PortIsolationEnable = plan.PortIsolationEnable
	body.SpanningTreeEnable = plan.SpanningTreeEnable
	body.LoopbackDetectVlanBasedEnable = plan.LoopbackDetectVlanBasedEnable
	body.LldpMedEnable = plan.LLDPMedEnable
	body.FlowControlEnable = plan.FlowControlEnable
	body.EeeEnable = plan.EeeEnable
	body.IgmpFastLeaveEnable = plan.IgmpFastLeaveEnable
	body.MldFastLeaveEnable = plan.MldFastLeaveEnable
	body.FastLeaveEnable = plan.FastLeaveEnable
	body.BandWidthCtrlType = plan.BandWidthCtrlType
	body.NativeNetworkID = plan.NativeNetworkID
	body.UntagNetworkIDs = append([]string{}, plan.UntagNetworkIDs...)
	body.TagNetworkIDs = append([]string{}, plan.TagNetworkIDs...)
	body.TopoNotifyEnable = plan.TopoNotifyEnable
	body.LoopbackDetectEnable = plan.LoopbackDetectEnable

	if plan.DhcpL2RelaySettings != nil {
		body.DhcpL2RelaySettings = client.DhcpGuardSettings{Enable: plan.DhcpL2RelaySettings.Enable}
	}
	if plan.SpanningTreeSetting != nil {
		s := plan.SpanningTreeSetting
		body.SpanningTreeSetting = client.PortProfileSpanningTreeV2{
			Priority:       s.Priority,
			ExtPathCost:    s.ExtPathCost,
			IntPathCost:    s.IntPathCost,
			EdgePort:       s.EdgePort,
			P2pLink:        s.P2pLink,
			LoopProtect:    s.LoopProtect,
			RootProtect:    s.RootProtect,
			TcGuard:        s.TcGuard,
			BpduProtect:    s.BpduProtect,
			BpduFilter:     s.BpduFilter,
			BpduForward:    s.BpduForward,
			InstanceEnable: body.SpanningTreeSetting.InstanceEnable,
		}
	}

	return body
}

// portProfileToV2Adapter translates a legacy PortProfile read-back
// (the shape /api/v2 GET returns) into a PortProfileV2 baseline.
// Fields the legacy GET does not expose (supportESEnable,
// voiceDscpEnable, instanceEnable, instances, networkTagsSetting,
// esEnableTaggedNetworkIds, prohibitModify, flag) take safe defaults
// sourced from the UI capture at
// dist/probe-openapi-v2-port-profile/00-patch.body.json.
// networkTagsSetting defaults to 2 (the value the UI sent for a
// trunk-all profile); if a future capture shows the controller is
// picky here we can read it from a v2 GET instead. The caller
// (portProfileV2FromPlan) overlays the plan-controlled fields on top.
func portProfileToV2Adapter(p *client.PortProfile, profileID string) *client.PortProfileV2 {
	v2 := &client.PortProfileV2{
		ID:                            profileID,
		Name:                          p.Name,
		POE:                           p.POE,
		Dot1x:                         p.Dot1x,
		PortIsolationEnable:           p.PortIsolationEnable,
		SpanningTreeEnable:            p.SpanningTreeEnable,
		LoopbackDetectVlanBasedEnable: p.LoopbackDetectVlanBasedEnable,
		LldpMedEnable:                 p.LLDPMedEnable,
		FlowControlEnable:             p.FlowControlEnable,
		EeeEnable:                     p.EeeEnable,
		IgmpFastLeaveEnable:           p.IgmpFastLeaveEnable,
		MldFastLeaveEnable:            p.MldFastLeaveEnable,
		FastLeaveEnable:               p.FastLeaveEnable,
		SupportESEnable:               false,
		BandWidthCtrlType:             p.BandWidthCtrlType,
		VlanConfigEnable:              true,
		NativeNetworkID:               p.NativeNetworkID,
		UntagNetworkIDs:               append([]string{}, p.UntagNetworkIDs...),
		TagNetworkIDs:                 append([]string{}, p.TagNetworkIDs...),
		ESEnableTaggedNetworkIDs:      []string{},
		NetworkTagsSetting:            2,
		VoiceNetworkEnable:            false,
		VoiceDscpEnable:               false,
		InstanceEnable:                false,
		Instances:                     []interface{}{},
		Flag:                          0,
		ProhibitModify:                false,
		TopoNotifyEnable:              p.TopoNotifyEnable,
		LoopbackDetectEnable:          p.LoopbackDetectEnable,
		Type:                          p.Type,
		Resource:                      0,
	}
	if p.DhcpL2RelaySettings != nil {
		v2.DhcpL2RelaySettings = client.DhcpGuardSettings{Enable: p.DhcpL2RelaySettings.Enable}
	}
	if p.SpanningTreeSetting != nil {
		s := p.SpanningTreeSetting
		v2.SpanningTreeSetting = client.PortProfileSpanningTreeV2{
			Priority:       s.Priority,
			ExtPathCost:    s.ExtPathCost,
			IntPathCost:    s.IntPathCost,
			EdgePort:       s.EdgePort,
			P2pLink:        s.P2pLink,
			LoopProtect:    s.LoopProtect,
			RootProtect:    s.RootProtect,
			TcGuard:        s.TcGuard,
			BpduProtect:    s.BpduProtect,
			BpduFilter:     s.BpduFilter,
			BpduForward:    s.BpduForward,
			InstanceEnable: false,
		}
	}
	return v2
}

func (r *PortProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PortProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeletePortProfile(ctx, state.SiteID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error deleting port profile", err.Error())
		return
	}
}

func (r *PortProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	siteID, profileID, ok := parseImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in the format 'siteID/profileID'.",
		)
		return
	}

	profile, err := r.client.GetPortProfile(ctx, siteID, profileID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing port profile", err.Error())
		return
	}

	state := PortProfileResourceModel{
		ID:     types.StringValue(profile.ID),
		SiteID: types.StringValue(siteID),
		// Empty list defaults so applyPortProfileToModel writes the API
		// values into known-list state instead of treating as null-preserve.
		TagNetworkIDs:   types.ListNull(types.StringType),
		UntagNetworkIDs: types.ListNull(types.StringType),
	}

	if err := applyPortProfileToModel(ctx, &state, profile); err != nil {
		resp.Diagnostics.AddError("Error decoding imported port profile", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
