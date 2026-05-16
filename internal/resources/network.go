package resources

import (
	"context"
	"fmt"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NetworkResource{}
var _ resource.ResourceWithImportState = &NetworkResource{}

// NetworkResource manages an Omada LAN network / VLAN.
type NetworkResource struct {
	client *client.Client
}

// NetworkResourceModel maps the resource schema to Go types.
type NetworkResourceModel struct {
	ID              types.String `tfsdk:"id"`
	SiteID          types.String `tfsdk:"site_id"`
	Name            types.String `tfsdk:"name"`
	Purpose         types.String `tfsdk:"purpose"`
	VlanID          types.Int64  `tfsdk:"vlan_id"`
	GatewaySubnet   types.String `tfsdk:"gateway_subnet"`
	DHCPEnabled     types.Bool   `tfsdk:"dhcp_enabled"`
	DHCPStart       types.String `tfsdk:"dhcp_start"`
	DHCPEnd         types.String `tfsdk:"dhcp_end"`
	IGMPSnoopEnable types.Bool   `tfsdk:"igmp_snoop_enable"`
	LanInterfaceIds types.List   `tfsdk:"lan_interface_ids"`

	// Network-level attrs newly surfaced from the controller API.
	Application        types.Int64 `tfsdk:"application"`
	VlanType           types.Int64 `tfsdk:"vlan_type"`
	Isolation          types.Bool  `tfsdk:"isolation"`
	FastLeaveEnable    types.Bool  `tfsdk:"fast_leave_enable"`
	MldSnoopEnable     types.Bool  `tfsdk:"mld_snoop_enable"`
	DhcpV6GuardEnable  types.Bool  `tfsdk:"dhcpv6_guard_enable"`
	DhcpGuardEnable    types.Bool  `tfsdk:"dhcp_guard_enable"`
	DhcpL2RelayEnable  types.Bool  `tfsdk:"dhcp_l2_relay_enable"`
	PortalEnable       types.Bool  `tfsdk:"portal_enable"`
	AccessControlRule  types.Bool  `tfsdk:"access_control_rule_enable"`
	RateLimitEnable    types.Bool  `tfsdk:"rate_limit_enable"`
	ArpDetectionEnable types.Bool  `tfsdk:"arp_detection_enable"`

	// DHCP-scoped extras
	DHCPLeaseTime    types.Int64  `tfsdk:"dhcp_lease_time"`
	DHCPDnsSource    types.String `tfsdk:"dhcp_dns_source"`
	DhcpDnsPrimary   types.String `tfsdk:"dhcp_dns_primary"`
	DhcpDnsSecondary types.String `tfsdk:"dhcp_dns_secondary"`

	// GatewayMAC is the MAC address of the gateway device (e.g. ER707) that
	// will host the L3 interface + DHCP for purpose=interface networks. Only
	// consumed when purpose=interface — the openapi/v1 endpoint requires it
	// to identify which device runs the routed VLAN. Format: dash-separated
	// uppercase hex, e.g. "AC-A7-F1-12-0C-6B".
	GatewayMAC types.String `tfsdk:"gateway_mac"`

	// ForceProvision controls whether Create() asks the controller to push
	// the new config to the gateway device immediately after creating a
	// purpose=interface network. Defaults to true; has no effect for
	// purpose=vlan networks or on Update/Delete flows.
	ForceProvision types.Bool `tfsdk:"force_provision"`
}

func NewNetworkResource() resource.Resource {
	return &NetworkResource{}
}

func (r *NetworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *NetworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a LAN network (VLAN) on the Omada Controller.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the network.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": siteIDResourceSchema(),
			"name": schema.StringAttribute{
				Description: "The name of the network.",
				Required:    true,
			},
			"purpose": schema.StringAttribute{
				Description: "The purpose of the network ('interface' for gateway networks, 'vlan' for VLAN-only). " +
					"NOT migratable after creation — empirically the Omada controller returns API -1 General error " +
					"when a `PATCH /setting/lan/networks/{id}` body changes `purpose` on an existing network, " +
					"mirroring the OC200 UI which forces a delete+recreate to switch network type. The provider " +
					"plans a destroy+create when this attribute changes.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("vlan"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vlan_id": schema.Int64Attribute{
				Description: "The VLAN ID for the network (1-4094).",
				Required:    true,
			},
			"gateway_subnet": schema.StringAttribute{
				Description: "The gateway IP and subnet in CIDR notation (e.g., '192.168.0.1/24'). Only applicable for 'interface' purpose networks.",
				Optional:    true,
				Computed:    true,
			},
			"dhcp_enabled": schema.BoolAttribute{
				Description: "Whether DHCP is enabled on this network. Only applicable for 'interface' purpose networks.",
				Optional:    true,
				Computed:    true,
			},
			"dhcp_start": schema.StringAttribute{
				Description: "The start of the DHCP range. Only applicable when DHCP is enabled.",
				Optional:    true,
				Computed:    true,
			},
			"dhcp_end": schema.StringAttribute{
				Description: "The end of the DHCP range. Only applicable when DHCP is enabled.",
				Optional:    true,
				Computed:    true,
			},
			"igmp_snoop_enable": schema.BoolAttribute{
				Description: "Enable IGMP snooping on this network. The Omada controller treats this field as required at the API level (returns -1001 if omitted), but the provider sends `false` as the default zero value, so omitting it from Terraform config is safe. If you ever change the underlying Go type to a pointer, you must also send a default value to avoid breaking creates.",
				Optional:    true,
				Computed:    true,
			},
			"lan_interface_ids": schema.ListAttribute{
				Description: "List of gateway LAN interface IDs the network is bound to. Required when purpose='interface' and a gateway is adopted; without it the controller returns API error -33515 (\"LAN interfaces could not be none\"). Maps to the controller's 'interfaceIds' API field.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"application": schema.Int64Attribute{
				Description: "Network application classification (controller-internal). Defaults to 0 (LAN). Change with caution — 1 typically maps to Guest.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"vlan_type": schema.Int64Attribute{
				Description: "VLAN type variant: 0=standard, others reserved for voice/IPTV/etc.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"isolation": schema.BoolAttribute{
				Description: "Enable client isolation within this network (intra-network client-to-client traffic is dropped).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"fast_leave_enable": schema.BoolAttribute{
				Description: "Enable IGMP fast-leave at the network/L3 level (distinct from the port_profile fast-leave field, which is L2/port-scoped).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"mld_snoop_enable": schema.BoolAttribute{
				Description: "Enable MLD snooping (IPv6 multicast) on this network.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"dhcpv6_guard_enable": schema.BoolAttribute{
				Description: "Enable DHCPv6 guard. Drops rogue DHCPv6 server responses on this network.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"dhcp_guard_enable": schema.BoolAttribute{
				Description: "Enable DHCPv4 guard. Drops rogue DHCPv4 server responses on this network.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"dhcp_l2_relay_enable": schema.BoolAttribute{
				Description: "Enable DHCP Layer-2 relay on this network.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"portal_enable": schema.BoolAttribute{
				Description: "Enable captive portal authentication on this network.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"access_control_rule_enable": schema.BoolAttribute{
				Description: "Enable access control rules (firewall ACLs) on this network.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"rate_limit_enable": schema.BoolAttribute{
				Description: "Enable rate limiting on this network.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"arp_detection_enable": schema.BoolAttribute{
				Description: "Enable ARP attack detection (drops gratuitous / spoofed ARP packets).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"dhcp_lease_time": schema.Int64Attribute{
				Description: "DHCP lease duration in minutes. Only applicable when DHCP is enabled. Controller default is 120 (2 hours) on most firmware.",
				Optional:    true,
				Computed:    true,
			},
			"dhcp_dns_source": schema.StringAttribute{
				Description: "DHCP DNS source: 'auto' (use gateway-provided DNS) or 'manual' (use dhcp_dns_primary / dhcp_dns_secondary). Only applicable when DHCP is enabled.",
				Optional:    true,
				Computed:    true,
			},
			"dhcp_dns_primary": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Description: "Primary DNS server (DHCP option 6) handed out to clients on this network. Only effective when dhcp_dns_source = \"manual\". Empty string means unset.",
			},
			"dhcp_dns_secondary": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Description: "Secondary DNS server. Same conditions as dhcp_dns_primary.",
			},
			"gateway_mac": schema.StringAttribute{
				Description: "MAC address of the gateway device (e.g. ER707) that will host the L3 interface + DHCP for this network. REQUIRED when purpose='interface' — the v6 controller's openapi/v1 endpoint needs it to identify which device runs the routed VLAN. Format: dash-separated uppercase hex, e.g. \"AC-A7-F1-12-0C-6B\". Ignored when purpose='vlan'.",
				Optional:    true,
			},
			"force_provision": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "When the network is created with purpose=interface, ask the controller to push the new config to the gateway device immediately after creation. Without this, the controller stores the network in its DB but the gateway does not pick it up until manually force-provisioned via the OC200 UI. Defaults to true. Has no effect for purpose=vlan networks.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *NetworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// buildNetworkFromModel converts the Terraform model into the API client
// struct. Shared between Create and Update.
func buildNetworkFromModel(ctx context.Context, m *NetworkResourceModel, diags *[]error) *client.Network {
	network := &client.Network{
		Name:               m.Name.ValueString(),
		Vlan:               int(m.VlanID.ValueInt64()),
		GatewaySubnet:      m.GatewaySubnet.ValueString(),
		IGMPSnoopEnable:    m.IGMPSnoopEnable.ValueBool(),
		Application:        int(m.Application.ValueInt64()),
		VlanType:           int(m.VlanType.ValueInt64()),
		Isolation:          m.Isolation.ValueBool(),
		FastLeaveEnable:    m.FastLeaveEnable.ValueBool(),
		MldSnoopEnable:     m.MldSnoopEnable.ValueBool(),
		DhcpL2RelayEnable:  m.DhcpL2RelayEnable.ValueBool(),
		Portal:             m.PortalEnable.ValueBool(),
		AccessControlRule:  m.AccessControlRule.ValueBool(),
		RateLimit:          m.RateLimitEnable.ValueBool(),
		ArpDetectionEnable: m.ArpDetectionEnable.ValueBool(),
		DhcpV6Guard:        &client.DhcpGuardSettings{Enable: m.DhcpV6GuardEnable.ValueBool()},
		DhcpGuard:          &client.DhcpGuardSettings{Enable: m.DhcpGuardEnable.ValueBool()},
	}
	if !m.Purpose.IsNull() && !m.Purpose.IsUnknown() {
		network.Purpose = m.Purpose.ValueString()
	}
	// DHCPSettings only built when dhcp_enabled is set explicitly. The
	// controller treats the absence of dhcpSettings as "leave alone" on
	// purpose=vlan networks; that's the safe default.
	//
	// When DHCP is ENABLED but leasetime or dhcpns are unset (null/unknown),
	// inject the controller's own default values so the PATCH body is
	// well-formed. Empirically: the controller rejects PATCH bodies with
	// dhcpSettings.enable=true but missing leasetime/dhcpns with API error
	// -1001 ("Invalid request parameters"). Captured against ER707 + OC200
	// — see dist/api-discover/networks-lan.json for the reference values.
	//
	// Injection is intentionally apply-time (not a schema-level Default) to
	// avoid the inconsistent-result-after-apply trap (see #40): the static
	// default would collide with whatever the controller actually stores if
	// it differs.
	if !m.DHCPEnabled.IsNull() && !m.DHCPEnabled.IsUnknown() {
		enabled := m.DHCPEnabled.ValueBool()
		leaseTime := int(m.DHCPLeaseTime.ValueInt64())
		dhcpNs := m.DHCPDnsSource.ValueString()
		if enabled {
			if leaseTime == 0 {
				// Omada controller's documented LAN DHCP default lease time
				// (minutes). The "Default" network ships with this value
				// out of the box on OC200 + ER707.
				leaseTime = 120
			}
			if dhcpNs == "" {
				// "auto" = inherit gateway DNS. "manual" requires extra
				// fields (dhcpns1, dhcpns2). "auto" is the safe default
				// and matches the controller's out-of-box behavior.
				dhcpNs = "auto"
			}
		}
		// Per-network DNS (DHCP option 6) is only meaningful when the source
		// is "manual"; in "auto" mode the controller falls back to gateway
		// DNS and we strip the fields via omitempty by sending empty strings.
		var dns1, dns2 string
		if dhcpNs == "manual" {
			dns1 = m.DhcpDnsPrimary.ValueString()
			dns2 = m.DhcpDnsSecondary.ValueString()
		}
		network.DHCPSettings = &client.DHCPSettings{
			Enable:      enabled,
			IPAddrStart: m.DHCPStart.ValueString(),
			IPAddrEnd:   m.DHCPEnd.ValueString(),
			LeaseTime:   leaseTime,
			DhcpNs:      dhcpNs,
			Dhcpns1:     dns1,
			Dhcpns2:     dns2,
		}
	}
	if !m.LanInterfaceIds.IsNull() && !m.LanInterfaceIds.IsUnknown() {
		var ids []string
		d := m.LanInterfaceIds.ElementsAs(ctx, &ids, false)
		if d.HasError() {
			for _, e := range d.Errors() {
				*diags = append(*diags, fmt.Errorf("%s: %s", e.Summary(), e.Detail()))
			}
			return nil
		}
		network.InterfaceIds = ids
	}
	return network
}

// applyNetworkToModel writes the API client struct back into the Terraform
// model. Shared between Read, Create-after-API-roundtrip, Update, ImportState.
// Honors purpose=vlan semantics: gateway/dhcp fields stay null in that mode.
func applyNetworkToModel(ctx context.Context, m *NetworkResourceModel, n *client.Network) error {
	m.Name = types.StringValue(n.Name)
	m.Purpose = types.StringValue(n.Purpose)
	m.VlanID = types.Int64Value(int64(n.Vlan))
	m.IGMPSnoopEnable = types.BoolValue(n.IGMPSnoopEnable)
	m.Application = types.Int64Value(int64(n.Application))
	m.VlanType = types.Int64Value(int64(n.VlanType))
	m.Isolation = types.BoolValue(n.Isolation)
	m.FastLeaveEnable = types.BoolValue(n.FastLeaveEnable)
	m.MldSnoopEnable = types.BoolValue(n.MldSnoopEnable)
	m.DhcpL2RelayEnable = types.BoolValue(n.DhcpL2RelayEnable)
	m.PortalEnable = types.BoolValue(n.Portal)
	m.AccessControlRule = types.BoolValue(n.AccessControlRule)
	m.RateLimitEnable = types.BoolValue(n.RateLimit)
	m.ArpDetectionEnable = types.BoolValue(n.ArpDetectionEnable)
	if n.DhcpV6Guard != nil {
		m.DhcpV6GuardEnable = types.BoolValue(n.DhcpV6Guard.Enable)
	} else {
		m.DhcpV6GuardEnable = types.BoolValue(false)
	}
	if n.DhcpGuard != nil {
		m.DhcpGuardEnable = types.BoolValue(n.DhcpGuard.Enable)
	} else {
		m.DhcpGuardEnable = types.BoolValue(false)
	}

	ifaceIDs, diag := types.ListValueFrom(ctx, types.StringType, n.InterfaceIds)
	if diag.HasError() {
		return fmt.Errorf("decoding interface_ids: %v", diag)
	}
	m.LanInterfaceIds = ifaceIDs

	if n.Purpose == "vlan" {
		m.GatewaySubnet = types.StringNull()
		m.DHCPEnabled = types.BoolNull()
		m.DHCPStart = types.StringNull()
		m.DHCPEnd = types.StringNull()
		m.DHCPLeaseTime = types.Int64Null()
		m.DHCPDnsSource = types.StringNull()
		// dhcp_dns_primary / dhcp_dns_secondary have schema Default = "" so
		// the plan side always materializes them as empty string. Reading
		// them back as types.StringValue("") (NOT Null) keeps state in
		// lockstep with the Default and avoids plan-drift on Read.
		m.DhcpDnsPrimary = types.StringValue("")
		m.DhcpDnsSecondary = types.StringValue("")
		return nil
	}

	m.GatewaySubnet = types.StringValue(n.GatewaySubnet)
	if n.DHCPSettings != nil {
		m.DHCPEnabled = types.BoolValue(n.DHCPSettings.Enable)
		m.DHCPStart = types.StringValue(n.DHCPSettings.IPAddrStart)
		m.DHCPEnd = types.StringValue(n.DHCPSettings.IPAddrEnd)
		m.DHCPLeaseTime = types.Int64Value(int64(n.DHCPSettings.LeaseTime))
		if n.DHCPSettings.DhcpNs != "" {
			m.DHCPDnsSource = types.StringValue(n.DHCPSettings.DhcpNs)
		} else {
			m.DHCPDnsSource = types.StringNull()
		}
		// Always set as StringValue (empty when unset) — see Default in
		// schema above for the rationale.
		m.DhcpDnsPrimary = types.StringValue(n.DHCPSettings.Dhcpns1)
		m.DhcpDnsSecondary = types.StringValue(n.DHCPSettings.Dhcpns2)
	} else {
		m.DHCPEnabled = types.BoolNull()
		m.DHCPStart = types.StringNull()
		m.DHCPEnd = types.StringNull()
		m.DHCPLeaseTime = types.Int64Null()
		m.DHCPDnsSource = types.StringNull()
		m.DhcpDnsPrimary = types.StringValue("")
		m.DhcpDnsSecondary = types.StringValue("")
	}
	return nil
}

func (r *NetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NetworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := plan.SiteID.ValueString()

	// Branch on purpose: interface networks use the v6 openapi/v1 endpoint
	// (legacy /api/v2/setting/lan/networks POST cannot create L3 networks —
	// it silently strips gatewaySubnet/dhcpSettings and returns purpose=vlan).
	purpose := plan.Purpose.ValueString()
	var created *client.Network
	var err error
	if purpose == "interface" {
		created, err = r.createInterfaceNetwork(ctx, &plan, resp)
		if err != nil {
			return // diagnostics already added by helper
		}

		// Force-provision follow-up: the openapi/v1 confirm endpoint persists
		// the new interface in the controller DB but does NOT push the
		// device-side config — the gateway stays "half-provisioned" until
		// someone clicks Force Provision in the OC200 UI. Best-effort: failure
		// is surfaced as a warning, not an error, because the network itself
		// was created successfully.
		forceProvision := plan.ForceProvision.IsNull() || plan.ForceProvision.IsUnknown() || plan.ForceProvision.ValueBool()
		gwMac := plan.GatewayMAC.ValueString()
		if forceProvision && gwMac != "" {
			if perr := r.client.ForceProvisionDevice(ctx, siteID, gwMac); perr != nil {
				resp.Diagnostics.AddWarning(
					"Force provision failed",
					fmt.Sprintf("Network %q was created successfully, but the post-create force-provision call to device %s failed: %s. The gateway may not pick up the new VLAN until you click Force Provision in the OC200 UI.", plan.Name.ValueString(), gwMac, perr.Error()),
				)
			}
		}
	} else {
		var buildErrs []error
		network := buildNetworkFromModel(ctx, &plan, &buildErrs)
		if len(buildErrs) > 0 {
			for _, e := range buildErrs {
				resp.Diagnostics.AddError("Building network payload", e.Error())
			}
			return
		}
		created, err = r.client.CreateNetwork(ctx, siteID, network)
		if err != nil {
			resp.Diagnostics.AddError("Error creating network", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(created.ID)
	if err := applyNetworkToModel(ctx, &plan, created); err != nil {
		resp.Diagnostics.AddError("Error decoding created network", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// buildInterfaceNetworkRequest builds the openapi/v1 body shared by Create
// (POST /networks/confirm) and Update (POST /networks/{id}/confirm). Returns
// (request, nil) on success, (nil, errors) on validation failure — caller
// is responsible for surfacing the errors to its diagnostics sink.
func buildInterfaceNetworkRequest(ctx context.Context, plan *NetworkResourceModel) (*client.InterfaceNetworkCreateRequest, []error) {
	var errs []error
	if plan.GatewayMAC.IsNull() || plan.GatewayMAC.IsUnknown() || plan.GatewayMAC.ValueString() == "" {
		errs = append(errs, fmt.Errorf("gateway_mac is required when purpose=\"interface\""))
	}

	var ports []string
	if !plan.LanInterfaceIds.IsNull() && !plan.LanInterfaceIds.IsUnknown() {
		d := plan.LanInterfaceIds.ElementsAs(ctx, &ports, false)
		if d.HasError() {
			for _, e := range d.Errors() {
				errs = append(errs, fmt.Errorf("%s: %s", e.Summary(), e.Detail()))
			}
		}
	}
	if len(ports) == 0 {
		errs = append(errs, fmt.Errorf("lan_interface_ids must contain at least one gateway LAN port"))
	}

	if len(errs) > 0 {
		return nil, errs
	}

	dhcpEnabled := !plan.DHCPEnabled.IsNull() && plan.DHCPEnabled.ValueBool()
	var dhcp *client.InterfaceDHCPSettings
	if dhcpEnabled {
		leaseTime := int(plan.DHCPLeaseTime.ValueInt64())
		if leaseTime == 0 {
			leaseTime = 120
		}
		dhcpNs := plan.DHCPDnsSource.ValueString()
		if dhcpNs == "" {
			dhcpNs = "auto"
		}
		// Per-network DNS (DHCP option 6) only forwarded when source is
		// "manual"; otherwise omitempty strips them and the controller
		// keeps falling back to gateway DNS.
		var dns1, dns2 string
		if dhcpNs == "manual" {
			dns1 = plan.DhcpDnsPrimary.ValueString()
			dns2 = plan.DhcpDnsSecondary.ValueString()
		}
		dhcp = &client.InterfaceDHCPSettings{
			Enable: true,
			IPRangePool: []client.DhcpIPRange{{
				IPAddrStart: plan.DHCPStart.ValueString(),
				IPAddrEnd:   plan.DHCPEnd.ValueString(),
			}},
			Dhcpns:       dhcpNs,
			PriDns:       dns1,
			SecondaryDns: dns2,
			LeaseTime:    leaseTime,
			GatewayMode:  "auto",
			Options:      []interface{}{},
		}
	}

	gwMAC := plan.GatewayMAC.ValueString()
	body := &client.InterfaceNetworkCreateRequest{
		DeviceConfig: client.InterfaceDeviceConfig{
			PortIsolationEnable: false,
			FlowControlEnable:   false,
			DeviceList: []client.InterfaceDeviceEntry{{
				Mac:   gwMAC,
				Type:  1,
				Ports: ports,
				Lags:  []string{},
			}},
			TagIDs: []string{},
		},
		LanNetwork: client.InterfaceLanNetwork{
			Name:                 plan.Name.ValueString(),
			DeviceMac:            gwMAC,
			DeviceType:           1,
			VlanType:             int(plan.VlanType.ValueInt64()),
			Vlan:                 int(plan.VlanID.ValueInt64()),
			GatewaySubnet:        plan.GatewaySubnet.ValueString(),
			DHCPSettings:         dhcp,
			UpnpLanEnable:        false,
			IGMPSnoopEnable:      plan.IGMPSnoopEnable.ValueBool(),
			DhcpGuard:            client.DhcpGuardSettings{Enable: plan.DhcpGuardEnable.ValueBool()},
			DhcpV6Guard:          client.DhcpGuardSettings{Enable: plan.DhcpV6GuardEnable.ValueBool()},
			LanNetworkIPv6Config: client.LanNetworkIPv6Config{Proto: 0, Enable: 0},
			QosQueueEnable:       false,
			Isolation:            plan.Isolation.ValueBool(),
			MldSnoopEnable:       plan.MldSnoopEnable.ValueBool(),
			ArpDetectionEnable:   plan.ArpDetectionEnable.ValueBool(),
			DhcpL2RelayEnable:    plan.DhcpL2RelayEnable.ValueBool(),
		},
	}
	return body, nil
}

// createInterfaceNetwork builds the openapi/v1 confirm body and creates an
// L3 (purpose=interface) network. Returns the Network read back from the
// legacy /api/v2 list. Requires plan.GatewayMAC to be set.
func (r *NetworkResource) createInterfaceNetwork(ctx context.Context, plan *NetworkResourceModel, resp *resource.CreateResponse) (*client.Network, error) {
	body, errs := buildInterfaceNetworkRequest(ctx, plan)
	if len(errs) > 0 {
		for _, e := range errs {
			resp.Diagnostics.AddError("Building interface network request", e.Error())
		}
		return nil, errs[0]
	}

	created, err := r.client.CreateInterfaceNetwork(ctx, plan.SiteID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Error creating interface network", err.Error())
		return nil, err
	}
	return created, nil
}

func (r *NetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()

	network, err := r.client.GetNetwork(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading network", err.Error())
		return
	}

	if err := applyNetworkToModel(ctx, &state, network); err != nil {
		resp.Diagnostics.AddError("Error decoding network", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NetworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state NetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()
	networkID := state.ID.ValueString()

	// Branch by purpose. purpose has RequiresReplace, so state.Purpose ==
	// plan.Purpose here; reading state is safe. Interface networks go
	// through the openapi/v1 3-step check/ports-check/confirm flow — the
	// legacy /api/v2 PATCH endpoint categorically rejects mutations on
	// interface-purpose networks with "API error -1: General error".
	var updated *client.Network
	var err error
	if state.Purpose.ValueString() == "interface" {
		body, errs := buildInterfaceNetworkRequest(ctx, &plan)
		if len(errs) > 0 {
			for _, e := range errs {
				resp.Diagnostics.AddError("Building interface network payload", e.Error())
			}
			return
		}
		updated, err = r.client.UpdateInterfaceNetwork(ctx, siteID, networkID, body)
		if err != nil {
			resp.Diagnostics.AddError("Error updating interface network", err.Error())
			return
		}

		// Mirror Create: force-provision so the gateway picks up the new
		// config without manual UI intervention. Best-effort; surface as
		// a warning on failure since the update itself succeeded.
		forceProvision := plan.ForceProvision.IsNull() || plan.ForceProvision.IsUnknown() || plan.ForceProvision.ValueBool()
		gwMac := plan.GatewayMAC.ValueString()
		if forceProvision && gwMac != "" {
			if perr := r.client.ForceProvisionDevice(ctx, siteID, gwMac); perr != nil {
				resp.Diagnostics.AddWarning(
					"Force provision failed",
					fmt.Sprintf("Network %q was updated successfully, but the post-update force-provision call to device %s failed: %s. The gateway may not pick up the change until you click Force Provision in the OC200 UI.", plan.Name.ValueString(), gwMac, perr.Error()),
				)
			}
		}
	} else {
		var buildErrs []error
		network := buildNetworkFromModel(ctx, &plan, &buildErrs)
		if len(buildErrs) > 0 {
			for _, e := range buildErrs {
				resp.Diagnostics.AddError("Building network payload", e.Error())
			}
			return
		}
		network.ID = networkID

		updated, err = r.client.UpdateNetwork(ctx, siteID, networkID, network)
		if err != nil {
			resp.Diagnostics.AddError("Error updating network", err.Error())
			return
		}
	}

	plan.ID = state.ID
	plan.SiteID = state.SiteID
	if err := applyNetworkToModel(ctx, &plan, updated); err != nil {
		resp.Diagnostics.AddError("Error decoding updated network", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteNetwork(ctx, state.SiteID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error deleting network", err.Error())
		return
	}
}

func (r *NetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	siteID, networkID, ok := parseImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in the format 'siteID/networkID'.",
		)
		return
	}

	network, err := r.client.GetNetwork(ctx, siteID, networkID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing network", err.Error())
		return
	}

	state := NetworkResourceModel{
		ID:     types.StringValue(network.ID),
		SiteID: types.StringValue(siteID),
	}
	if err := applyNetworkToModel(ctx, &state, network); err != nil {
		resp.Diagnostics.AddError("Error decoding imported network", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
