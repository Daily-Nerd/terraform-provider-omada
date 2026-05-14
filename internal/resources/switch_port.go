package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SwitchPortResource{}
var _ resource.ResourceWithImportState = &SwitchPortResource{}

// SwitchPortResource manages a single switch port via PATCH
// /switches/{mac}/ports/{port}. Compared to omada_device_switch (which
// manages the entire switch + ports[] in one resource), this resource
// gives per-port granularity for cleaner for_each iteration, single-port
// plans, and per-port import.
type SwitchPortResource struct {
	client *client.Client
}

// SwitchPortResourceModel maps the resource schema to Go types.
type SwitchPortResourceModel struct {
	ID        types.String `tfsdk:"id"`
	SiteID    types.String `tfsdk:"site_id"`
	DeviceMAC types.String `tfsdk:"device_mac"`
	Port      types.Int64  `tfsdk:"port"`

	Name                  types.String `tfsdk:"name"`
	Disable               types.Bool   `tfsdk:"disable"`
	ProfileID             types.String `tfsdk:"profile_id"`
	ProfileOverrideEnable types.Bool   `tfsdk:"profile_override_enable"`
	NativeNetworkID       types.String `tfsdk:"native_network_id"`
	NetworkTagsSetting    types.Int64  `tfsdk:"network_tags_setting"`
	TagNetworkIDs         types.List   `tfsdk:"tag_network_ids"`
	UntagNetworkIDs       types.List   `tfsdk:"untag_network_ids"`
	VoiceNetworkEnable    types.Bool   `tfsdk:"voice_network_enable"`
	VoiceDscpEnable       types.Bool   `tfsdk:"voice_dscp_enable"`
	Speed                 types.Int64  `tfsdk:"speed"`
}

func NewSwitchPortResource() resource.Resource {
	return &SwitchPortResource{}
}

func (r *SwitchPortResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_switch_port"
}

func (r *SwitchPortResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a single port on an Omada-managed switch via PATCH /switches/{mac}/ports/{port}. " +
			"Use this when you want per-port granularity (one resource per port) — the alternative " +
			"`omada_device_switch.ports[]` nested attribute manages the whole switch in one resource.\n\n" +
			"Switch ports are not creatable or destroyable — they always exist on adopted hardware. " +
			"Resource semantics:\n" +
			"  - Create = upsert: PATCH the port with the declared settings.\n" +
			"  - Update = PATCH the port with the new settings.\n" +
			"  - Delete = remove from Terraform state. The port keeps its current settings on the " +
			"switch (no API call). To revert a port to defaults, set `profile_override_enable=false` " +
			"and clear the override fields, then `terraform apply`, then `terraform state rm`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Synthetic resource ID — '{device_mac}:{port}'.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": siteIDResourceSchema(),
			"device_mac": schema.StringAttribute{
				Description: "MAC address of the switch device. Forces replacement when changed.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"port": schema.Int64Attribute{
				Description: "1-based port number on the switch. Forces replacement when changed.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64RequiresReplace{},
				},
			},
			"name": schema.StringAttribute{
				Description: "Friendly port name shown in the Omada UI.",
				Optional:    true,
				Computed:    true,
			},
			"disable": schema.BoolAttribute{
				Description: "Administratively shut down the port.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"profile_id": schema.StringAttribute{
				Description: "ID of the omada_port_profile to apply to this port. " +
					"Mutually exclusive with `profile_override_enable=true` + per-port VLAN fields.",
				Optional: true,
				Computed: true,
			},
			"profile_override_enable": schema.BoolAttribute{
				Description: "When true, this port uses the per-port `native_network_id` / " +
					"`tag_network_ids` / `untag_network_ids` settings instead of the assigned profile.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"native_network_id": schema.StringAttribute{
				Description: "Native (untagged / PVID) network ID for this port. Only honored when " +
					"`profile_override_enable=true`.",
				Optional: true,
				Computed: true,
			},
			"network_tags_setting": schema.Int64Attribute{
				Description: "VLAN tagging mode: 0=general (controller default), 1=trunk, 2=access. " +
					"Only honored when `profile_override_enable=true`. When override is off, " +
					"the controller derives this value from the port profile and a user-supplied " +
					"value is ignored — leave unset to track the controller's value cleanly.",
				Optional: true,
				Computed: true,
			},
			"tag_network_ids": schema.ListAttribute{
				Description: "List of tagged VLAN network IDs. Only honored when " +
					"`profile_override_enable=true`.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"untag_network_ids": schema.ListAttribute{
				Description: "List of untagged (member) VLAN network IDs in addition to the native VLAN. " +
					"Only honored when `profile_override_enable=true`.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"voice_network_enable": schema.BoolAttribute{
				Description: "Enable voice VLAN on this port. Requires controller voice VLAN configuration.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"voice_dscp_enable": schema.BoolAttribute{
				Description: "Enable DSCP marking for voice traffic on this port.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"speed": schema.Int64Attribute{
				Description: "Port speed code: 0=auto-negotiate, 1=10Mb HD, 2=10Mb FD, 3=100Mb HD, " +
					"4=100Mb FD, 5=1Gb FD, 6=2.5Gb FD, 7=5Gb FD, 8=10Gb FD. Specific code support " +
					"depends on the switch model. Leave unset to accept the controller's reported " +
					"value (avoids plan/apply drift). Suspected to behave as a status field on read " +
					"— see GitHub issue #40 for the API discovery thread.",
				Optional: true,
				Computed: true,
			},
		},
	}
}

func (r *SwitchPortResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// buildSwitchPortPatchPayload converts the model into a PATCH map for the
// /switches/{mac}/ports/{port} endpoint. Returns nil when the model has no
// changeable fields populated.
func buildSwitchPortPatchPayload(ctx context.Context, m *SwitchPortResourceModel, diags *[]error) map[string]interface{} {
	payload := map[string]interface{}{
		"port":                  m.Port.ValueInt64(),
		"disable":               m.Disable.ValueBool(),
		"profileOverrideEnable": m.ProfileOverrideEnable.ValueBool(),
		"voiceNetworkEnable":    m.VoiceNetworkEnable.ValueBool(),
		"voiceDscpEnable":       m.VoiceDscpEnable.ValueBool(),
		"networkTagsSetting":    m.NetworkTagsSetting.ValueInt64(),
		"speed":                 m.Speed.ValueInt64(),
	}

	if !m.Name.IsNull() && !m.Name.IsUnknown() {
		payload["name"] = m.Name.ValueString()
	}
	if !m.ProfileID.IsNull() && !m.ProfileID.IsUnknown() && m.ProfileID.ValueString() != "" {
		payload["profileId"] = m.ProfileID.ValueString()
	}
	if !m.NativeNetworkID.IsNull() && !m.NativeNetworkID.IsUnknown() && m.NativeNetworkID.ValueString() != "" {
		payload["nativeNetworkId"] = m.NativeNetworkID.ValueString()
	}
	if !m.TagNetworkIDs.IsNull() && !m.TagNetworkIDs.IsUnknown() {
		var ids []string
		d := m.TagNetworkIDs.ElementsAs(ctx, &ids, false)
		if d.HasError() {
			for _, e := range d.Errors() {
				*diags = append(*diags, fmt.Errorf("%s: %s", e.Summary(), e.Detail()))
			}
			return nil
		}
		payload["tagNetworkIds"] = ids
	}
	if !m.UntagNetworkIDs.IsNull() && !m.UntagNetworkIDs.IsUnknown() {
		var ids []string
		d := m.UntagNetworkIDs.ElementsAs(ctx, &ids, false)
		if d.HasError() {
			for _, e := range d.Errors() {
				*diags = append(*diags, fmt.Errorf("%s: %s", e.Summary(), e.Detail()))
			}
			return nil
		}
		payload["untagNetworkIds"] = ids
	}
	return payload
}

// applySwitchPortToModel writes the API SwitchPort struct back into the
// Terraform model. Preserves null vs empty-list semantics for
// tag_network_ids and untag_network_ids.
func applySwitchPortToModel(ctx context.Context, m *SwitchPortResourceModel, p *client.SwitchPort) error {
	m.Port = types.Int64Value(int64(p.Port))
	m.Name = types.StringValue(p.Name)
	m.Disable = types.BoolValue(p.Disable)
	m.ProfileID = types.StringValue(p.ProfileID)
	m.ProfileOverrideEnable = types.BoolValue(p.ProfileOverrideEnable)
	m.NativeNetworkID = types.StringValue(p.NativeNetworkID)
	m.NetworkTagsSetting = types.Int64Value(int64(p.NetworkTagsSetting))
	m.VoiceNetworkEnable = types.BoolValue(p.VoiceNetworkEnable)
	m.VoiceDscpEnable = types.BoolValue(p.VoiceDscpEnable)
	m.Speed = types.Int64Value(int64(p.Speed))

	if len(p.TagNetworkIDs) == 0 && m.TagNetworkIDs.IsNull() {
		m.TagNetworkIDs = types.ListNull(types.StringType)
	} else {
		ids, diags := types.ListValueFrom(ctx, types.StringType, p.TagNetworkIDs)
		if diags.HasError() {
			return fmt.Errorf("decoding tag_network_ids: %v", diags)
		}
		m.TagNetworkIDs = ids
	}

	if len(p.UntagNetworkIDs) == 0 && m.UntagNetworkIDs.IsNull() {
		m.UntagNetworkIDs = types.ListNull(types.StringType)
	} else {
		ids, diags := types.ListValueFrom(ctx, types.StringType, p.UntagNetworkIDs)
		if diags.HasError() {
			return fmt.Errorf("decoding untag_network_ids: %v", diags)
		}
		m.UntagNetworkIDs = ids
	}
	return nil
}

func (r *SwitchPortResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SwitchPortResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := plan.SiteID.ValueString()
	mac := plan.DeviceMAC.ValueString()
	port := int(plan.Port.ValueInt64())

	var buildErrs []error
	payload := buildSwitchPortPatchPayload(ctx, &plan, &buildErrs)
	if len(buildErrs) > 0 {
		for _, e := range buildErrs {
			resp.Diagnostics.AddError("Building switch port payload", e.Error())
		}
		return
	}

	if err := r.client.UpdateSwitchPort(ctx, siteID, mac, port, payload); err != nil {
		resp.Diagnostics.AddError("Error creating (PATCHing) switch port", err.Error())
		return
	}

	// Read back to populate computed fields.
	current, err := r.client.GetSwitchPort(ctx, siteID, mac, port)
	if err != nil {
		resp.Diagnostics.AddError("Error reading switch port after create", err.Error())
		return
	}
	if err := applySwitchPortToModel(ctx, &plan, current); err != nil {
		resp.Diagnostics.AddError("Error decoding switch port", err.Error())
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s:%d", mac, port))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SwitchPortResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SwitchPortResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()
	mac := state.DeviceMAC.ValueString()
	port := int(state.Port.ValueInt64())

	current, err := r.client.GetSwitchPort(ctx, siteID, mac, port)
	if err != nil {
		// Switch removed from controller (un-adopted), or port out of range.
		// Drop from state so terraform plan reports a recreate / remove.
		resp.State.RemoveResource(ctx)
		return
	}
	if err := applySwitchPortToModel(ctx, &state, current); err != nil {
		resp.Diagnostics.AddError("Error decoding switch port", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SwitchPortResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SwitchPortResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state SwitchPortResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()
	mac := state.DeviceMAC.ValueString()
	port := int(state.Port.ValueInt64())

	var buildErrs []error
	payload := buildSwitchPortPatchPayload(ctx, &plan, &buildErrs)
	if len(buildErrs) > 0 {
		for _, e := range buildErrs {
			resp.Diagnostics.AddError("Building switch port payload", e.Error())
		}
		return
	}

	if err := r.client.UpdateSwitchPort(ctx, siteID, mac, port, payload); err != nil {
		resp.Diagnostics.AddError("Error updating switch port", err.Error())
		return
	}

	current, err := r.client.GetSwitchPort(ctx, siteID, mac, port)
	if err != nil {
		resp.Diagnostics.AddError("Error reading switch port after update", err.Error())
		return
	}
	plan.ID = state.ID
	plan.SiteID = state.SiteID
	plan.DeviceMAC = state.DeviceMAC
	if err := applySwitchPortToModel(ctx, &plan, current); err != nil {
		resp.Diagnostics.AddError("Error decoding switch port", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the controller — switch ports always exist as long
// as the switch is adopted. We simply drop the resource from Terraform state
// without modifying the port. To revert a port to defaults, the user should
// declare those defaults in HCL, apply, and only then delete the resource.
func (r *SwitchPortResource) Delete(_ context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Switch port resource removed from state — port settings on the switch unchanged",
		"Switch ports cannot be physically deleted; they always exist as long as the switch is "+
			"adopted by the controller. The port keeps its current settings. To revert this port "+
			"to default values, declare the desired defaults in HCL and `terraform apply` BEFORE "+
			"removing the resource block.",
	)
}

func (r *SwitchPortResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: '{site_id}/{device_mac}/{port}'
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in the format '{site_id}/{device_mac}/{port}', e.g. "+
				"'69fd06da.../AA-BB-CC-DD-EE-FF/5'.",
		)
		return
	}
	siteID, mac, portStr := parts[0], parts[1], parts[2]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		resp.Diagnostics.AddError("Invalid port number in import ID", err.Error())
		return
	}

	current, err := r.client.GetSwitchPort(ctx, siteID, mac, port)
	if err != nil {
		resp.Diagnostics.AddError("Error reading switch port for import", err.Error())
		return
	}

	state := SwitchPortResourceModel{
		ID:              types.StringValue(fmt.Sprintf("%s:%d", mac, port)),
		SiteID:          types.StringValue(siteID),
		DeviceMAC:       types.StringValue(mac),
		Port:            types.Int64Value(int64(port)),
		TagNetworkIDs:   types.ListNull(types.StringType),
		UntagNetworkIDs: types.ListNull(types.StringType),
	}
	if err := applySwitchPortToModel(ctx, &state, current); err != nil {
		resp.Diagnostics.AddError("Error decoding imported switch port", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// int64RequiresReplace forces a resource recreation when the int64 attribute
// changes. Mirrors stringplanmodifier.RequiresReplace() but for Int64
// attributes. Used on the `port` attribute since the API key is the port
// number — changing it means a different port.
type int64RequiresReplace struct{}

func (m int64RequiresReplace) Description(_ context.Context) string {
	return "If the value of this attribute changes, Terraform will destroy and recreate the resource."
}
func (m int64RequiresReplace) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (m int64RequiresReplace) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.StateValue.IsNull() {
		return
	}
	if req.PlanValue.Equal(req.StateValue) {
		return
	}
	resp.RequiresReplace = true
}
