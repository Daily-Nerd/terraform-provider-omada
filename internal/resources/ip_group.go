package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// normalizeIPEntry converts an ip attribute value to canonical "ip/mask" form.
// A bare host address ("10.10.70.98") becomes "10.10.70.98/32".
// An existing CIDR ("10.10.10.0/24") is returned unchanged.
// This ensures the planned value always matches the string reconstructed by
// setStateFromAPI after the API stores and returns the entry as {ip, mask}.
func normalizeIPEntry(s string) string {
	if strings.Contains(s, "/") {
		return s
	}
	return s + "/32"
}

// ipGroupTypeForEntries returns 0 (IP-only) when no entries carry ports, 1 otherwise.
func ipGroupTypeForEntries(entries []IPGroupEntryModel) int {
	for _, e := range entries {
		if !e.PortList.IsNull() && !e.PortList.IsUnknown() {
			var ports []string
			// We ignore the error here — a non-nil list means ports are set.
			_ = e.PortList.ElementsAs(context.Background(), &ports, false)
			if len(ports) > 0 {
				return 1
			}
		}
	}
	return 0
}

var _ resource.Resource = &IPGroupResource{}
var _ resource.ResourceWithImportState = &IPGroupResource{}

// IPGroupResource manages an IP/Port group on the Omada Controller.
type IPGroupResource struct {
	client *client.Client
}

// IPGroupResourceModel maps the resource schema to Go types.
type IPGroupResourceModel struct {
	ID     types.String        `tfsdk:"id"`
	SiteID types.String        `tfsdk:"site_id"`
	Name   types.String        `tfsdk:"name"`
	Type   types.Int64         `tfsdk:"type"`
	IPList []IPGroupEntryModel `tfsdk:"ip_list"`
}

// IPGroupEntryModel represents a single IP + port combination.
type IPGroupEntryModel struct {
	IP       IPCIDRStringValue `tfsdk:"ip"`
	PortList types.List        `tfsdk:"port_list"`
}

func NewIPGroupResource() resource.Resource {
	return &IPGroupResource{}
}

func (r *IPGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_group"
}

func (r *IPGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IP/Port group on the Omada Controller. " +
			"IP groups are used as source or destination in firewall ACL rules for port-based filtering. " +
			"Requires a gateway device adopted into the site.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the IP group.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": siteIDResourceSchema(),
			"name": schema.StringAttribute{
				Description: "The name of the IP group.",
				Required:    true,
			},
			"type": schema.Int64Attribute{
				Description: "The group type. 0 = IP-only group, 1 = IP/Port group. Computed from whether any entry has ports.",
				Computed:    true,
			},
			"ip_list": schema.ListNestedAttribute{
				Description: "List of IP address and port combinations.",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"ip": schema.StringAttribute{
							Description: "IP address or CIDR subnet (e.g., '192.168.1.100' or '192.168.1.0/24'). " +
								"A bare host address ('10.10.70.98') is treated as semantically equal to its " +
								"canonical CIDR form ('10.10.70.98/32') — no perpetual diff after apply.",
							Required:   true,
							CustomType: IPCIDRStringType{},
						},
						"port_list": schema.ListAttribute{
							Description: "List of port numbers or ranges as strings (e.g., '80', '7000-7100').",
							Optional:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (r *IPGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IPGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IPGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := plan.SiteID.ValueString()

	ipList := r.modelToIPList(ctx, plan.IPList, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	group := &client.IPGroup{
		Name:   plan.Name.ValueString(),
		Type:   ipGroupTypeForEntries(plan.IPList),
		IPList: ipList,
	}

	created, err := r.client.CreateIPGroup(ctx, siteID, group)
	if err != nil {
		resp.Diagnostics.AddError("Error creating IP group", err.Error())
		return
	}

	r.setStateFromAPI(ctx, &plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IPGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IPGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()

	group, err := r.client.GetIPGroup(ctx, siteID, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			// Group was deleted out-of-band. Remove from state so Terraform
			// plans to recreate it (drift detection) rather than hard-failing.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading IP group", err.Error())
		return
	}

	r.setStateFromAPI(ctx, &state, group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IPGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IPGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state IPGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()

	ipList := r.modelToIPList(ctx, plan.IPList, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	group := &client.IPGroup{
		Name:   plan.Name.ValueString(),
		Type:   ipGroupTypeForEntries(plan.IPList),
		IPList: ipList,
	}

	updated, err := r.client.UpdateIPGroup(ctx, siteID, state.ID.ValueString(), group)
	if err != nil {
		resp.Diagnostics.AddError("Error updating IP group", err.Error())
		return
	}

	plan.ID = state.ID
	plan.SiteID = state.SiteID
	r.setStateFromAPI(ctx, &plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IPGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IPGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteIPGroup(ctx, state.SiteID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error deleting IP group", err.Error())
		return
	}
}

func (r *IPGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	siteID, groupID, ok := parseImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in the format 'siteID/groupID'.",
		)
		return
	}

	group, err := r.client.GetIPGroup(ctx, siteID, groupID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing IP group", err.Error())
		return
	}

	state := IPGroupResourceModel{
		SiteID: types.StringValue(siteID),
	}
	r.setStateFromAPI(ctx, &state, group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// modelToIPList converts the Terraform model to the client IPGroupEntry slice.
// The ip attribute accepts either a CIDR string ("10.10.50.0/24") or a bare
// host address ("10.10.70.98"). splitCIDR splits it into the separate ip + mask
// integer required by the v6/ER707 wire body.
func (r *IPGroupResource) modelToIPList(ctx context.Context, entries []IPGroupEntryModel, diags *diag.Diagnostics) []client.IPGroupEntry {
	var ipList []client.IPGroupEntry
	for _, e := range entries {
		rawIP := e.IP.ValueString()
		ip, mask, err := client.SplitCIDR(rawIP)
		if err != nil {
			diags.AddError(
				"Invalid IP address in ip_list",
				fmt.Sprintf("ip_list entry %q is not a valid IP or CIDR: %v", rawIP, err),
			)
			return nil
		}
		entry := client.IPGroupEntry{
			IP:          ip,
			Mask:        mask,
			Description: "",
		}
		if !e.PortList.IsNull() && !e.PortList.IsUnknown() {
			var ports []string
			diags.Append(e.PortList.ElementsAs(ctx, &ports, false)...)
			if diags.HasError() {
				return nil
			}
			entry.PortList = ports
		}
		ipList = append(ipList, entry)
	}
	return ipList
}

// setStateFromAPI populates the resource model from an API response.
// The v6 API returns ip (bare address) and mask (integer) separately; we
// reconstruct the CIDR string stored in state so the HCL attribute stays stable.
func (r *IPGroupResource) setStateFromAPI(ctx context.Context, model *IPGroupResourceModel, group *client.IPGroup) {
	model.ID = types.StringValue(group.ID)
	model.Name = types.StringValue(group.Name)
	model.Type = types.Int64Value(int64(group.Type))

	model.IPList = make([]IPGroupEntryModel, len(group.IPList))
	for i, entry := range group.IPList {
		// Reconstruct CIDR string: "10.10.50.0/24" or "10.10.70.98/32".
		cidr := fmt.Sprintf("%s/%d", entry.IP, entry.Mask)
		model.IPList[i] = IPGroupEntryModel{
			IP: IPCIDRStringValue{StringValue: basetypes.NewStringValue(cidr)},
		}
		if len(entry.PortList) > 0 {
			portList, _ := types.ListValueFrom(ctx, types.StringType, entry.PortList)
			model.IPList[i].PortList = portList
		} else {
			model.IPList[i].PortList = types.ListNull(types.StringType)
		}
	}
}
