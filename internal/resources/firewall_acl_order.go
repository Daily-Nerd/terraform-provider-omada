package resources

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &FirewallACLOrderResource{}
var _ resource.ResourceWithImportState = &FirewallACLOrderResource{}

// FirewallACLOrderResource manages the ordering of firewall ACL rules on the
// Omada Controller. It owns the global ACL order for a given site+type pair:
// given an ordered list of ACL rule IDs, it sets each rule's index to its
// 1-based position using the batch modifyIndex command.
//
// Delete is a no-op because ordering is not a deletable controller object —
// removing this resource simply stops managing order.
type FirewallACLOrderResource struct {
	client *client.Client
}

// FirewallACLOrderModel maps the resource schema to Go types.
type FirewallACLOrderModel struct {
	ID      types.String `tfsdk:"id"`
	SiteID  types.String `tfsdk:"site_id"`
	Type    types.Int64  `tfsdk:"type"`
	RuleIDs types.List   `tfsdk:"rule_ids"`
}

// buildIndexMap returns a 1-based position map for the given ordered rule IDs.
// ["a","b","c"] → {"a":1,"b":2,"c":3}
func buildIndexMap(ruleIDs []string) map[string]int {
	m := make(map[string]int, len(ruleIDs))
	for i, id := range ruleIDs {
		m[id] = i + 1
	}
	return m
}

func NewFirewallACLOrderResource() resource.Resource {
	return &FirewallACLOrderResource{}
}

func (r *FirewallACLOrderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_acl_order"
}

func (r *FirewallACLOrderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the ordering of firewall ACL rules on the Omada Controller. " +
			"Omada assigns rule index (first-match order) by creation order; this resource " +
			"owns the global order for a site+type pair by issuing a batch modifyIndex command. " +
			"Delete is a no-op — removing this resource stops managing order without altering rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Resource identifier in the form '{site_id}:{type}'.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": siteIDResourceSchema(),
			"type": schema.Int64Attribute{
				Description: "ACL type: 0=gateway (default), 1=switch, 2=eap.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"rule_ids": schema.ListAttribute{
				Description: "Ordered list of ACL rule IDs. Position in the list sets the " +
					"first-match index (1-based). On Read the list reflects the controller's " +
					"current ordering so drift surfaces as a plan diff.",
				Required:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *FirewallACLOrderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FirewallACLOrderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallACLOrderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := plan.SiteID.ValueString()
	aclType := int(plan.Type.ValueInt64())

	var ruleIDs []string
	resp.Diagnostics.Append(plan.RuleIDs.ElementsAs(ctx, &ruleIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.ModifyACLIndex(ctx, siteID, aclType, buildIndexMap(ruleIDs)); err != nil {
		resp.Diagnostics.AddError("Error setting ACL order", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s:%d", siteID, aclType))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallACLOrderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallACLOrderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()
	aclType := int(state.Type.ValueInt64())

	rules, err := r.client.ListACLRules(ctx, siteID, aclType)
	if err != nil {
		resp.Diagnostics.AddError("Error reading ACL rules", err.Error())
		return
	}

	// Sort by index to reflect the controller's current first-match order.
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Index < rules[j].Index
	})

	ids := make([]string, len(rules))
	for i, rule := range rules {
		ids[i] = rule.ID
	}

	ruleIDs, diags := types.ListValueFrom(ctx, types.StringType, ids)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.RuleIDs = ruleIDs
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallACLOrderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallACLOrderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state FirewallACLOrderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()
	aclType := int(state.Type.ValueInt64())

	var ruleIDs []string
	resp.Diagnostics.Append(plan.RuleIDs.ElementsAs(ctx, &ruleIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.ModifyACLIndex(ctx, siteID, aclType, buildIndexMap(ruleIDs)); err != nil {
		resp.Diagnostics.AddError("Error updating ACL order", err.Error())
		return
	}

	plan.ID = state.ID
	plan.SiteID = state.SiteID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: ACL ordering is not a deletable controller object.
// Removing this resource stops managing order without altering rules.
func (r *FirewallACLOrderResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *FirewallACLOrderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: "{site_id}:{type}" (e.g., "siteId:0")
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in the format 'siteID:type' (e.g., 'siteId:0').",
		)
		return
	}

	siteID := parts[0]
	var aclType int64
	if _, err := fmt.Sscanf(parts[1], "%d", &aclType); err != nil {
		resp.Diagnostics.AddError(
			"Invalid ACL type in import ID",
			fmt.Sprintf("ACL type must be an integer (0=gateway, 1=switch, 2=eap), got: %s", parts[1]),
		)
		return
	}

	// Seed an empty rule_ids list; Read will populate it from the controller.
	emptyRuleIDs, diags := types.ListValueFrom(ctx, types.StringType, []string{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := FirewallACLOrderModel{
		ID:      types.StringValue(req.ID),
		SiteID:  types.StringValue(siteID),
		Type:    types.Int64Value(aclType),
		RuleIDs: emptyRuleIDs,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delegate to Read to populate rule_ids from the controller.
	readReq := resource.ReadRequest{State: resp.State}
	readResp := resource.ReadResponse{State: resp.State}
	r.Read(ctx, readReq, &readResp)
	resp.Diagnostics.Append(readResp.Diagnostics...)
	resp.State = readResp.State
}
