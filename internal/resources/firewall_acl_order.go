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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
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
				Description: "ACL type: 0=gateway (default), 1=switch, 2=eap. " +
					"Changing the type replaces the resource — a different ACL type is a distinct ordered set.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(0),
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
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

	// The set of rule IDs this resource manages. Read must only ever reflect
	// these IDs back into state — never absorb out-of-band rules that exist
	// on the controller but are not part of the managed order.
	var managed []string
	resp.Diagnostics.Append(state.RuleIDs.ElementsAs(ctx, &managed, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rules, err := r.client.ListACLRules(ctx, siteID, aclType)
	if err != nil {
		resp.Diagnostics.AddError("Error reading ACL rules", err.Error())
		return
	}

	ids := orderedManagedIDs(rules, aclType, managed)

	ruleIDs, diags := types.ListValueFrom(ctx, types.StringType, ids)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.RuleIDs = ruleIDs
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// orderedManagedIDs returns the managed rule IDs in the controller's current
// first-match order. It:
//   - filters controller rules to the requested aclType (defends against the
//     controller ever returning mixed types),
//   - sorts the surviving rules by their .Index,
//   - keeps only IDs that are present in the managed set (out-of-band rules
//     are ignored so they are never absorbed into state and reordered),
//   - drops managed IDs the controller no longer reports (surfaces as drift).
func orderedManagedIDs(rules []client.ACLRule, aclType int, managed []string) []string {
	managedSet := make(map[string]struct{}, len(managed))
	for _, id := range managed {
		managedSet[id] = struct{}{}
	}

	filtered := make([]client.ACLRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Type != aclType {
			continue
		}
		if _, ok := managedSet[rule.ID]; !ok {
			continue
		}
		filtered = append(filtered, rule)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Index < filtered[j].Index
	})

	ids := make([]string, len(filtered))
	for i, rule := range filtered {
		ids[i] = rule.ID
	}
	return ids
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

	// Read siteID/aclType from the PLAN. site_id and type are both
	// RequiresReplace, so within Update they always match state — but the
	// plan is the authoritative source of intent and avoids any chance of
	// targeting the wrong ACL type.
	siteID := plan.SiteID.ValueString()
	aclType := int(plan.Type.ValueInt64())

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

	// Fetch the controller's current rules and adopt their order as the
	// managed set. On import there is no prior state to filter against, so
	// every rule of this type becomes managed (the user can prune the list
	// afterwards). This mirrors firewall_acl.go ImportState: fetch inline,
	// populate state from the API, and let resp.State.Set persist it.
	rules, err := r.client.ListACLRules(ctx, siteID, int(aclType))
	if err != nil {
		resp.Diagnostics.AddError("Error importing ACL order", err.Error())
		return
	}

	filtered := make([]client.ACLRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Type == int(aclType) {
			filtered = append(filtered, rule)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Index < filtered[j].Index
	})
	ids := make([]string, len(filtered))
	for i, rule := range filtered {
		ids[i] = rule.ID
	}

	ruleIDs, diags := types.ListValueFrom(ctx, types.StringType, ids)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := FirewallACLOrderModel{
		ID:      types.StringValue(fmt.Sprintf("%s:%d", siteID, aclType)),
		SiteID:  types.StringValue(siteID),
		Type:    types.Int64Value(aclType),
		RuleIDs: ruleIDs,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
