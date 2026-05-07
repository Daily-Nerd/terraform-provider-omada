package resources

import (
	"context"
	"fmt"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SiteResource{}
var _ resource.ResourceWithImportState = &SiteResource{}

// SiteResource manages an Omada site.
type SiteResource struct {
	client *client.Client
}

// SiteResourceModel maps the resource schema to Go types.
type SiteResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Region                types.String `tfsdk:"region"`
	TimeZone              types.String `tfsdk:"time_zone"`
	Scenario              types.String `tfsdk:"scenario"`
	Type                  types.Int64  `tfsdk:"type"`
	DeviceAccountUsername types.String `tfsdk:"device_account_username"`
	DeviceAccountPassword types.String `tfsdk:"device_account_password"`
}

func NewSiteResource() resource.Resource {
	return &SiteResource{}
}

func (r *SiteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site"
}

func (r *SiteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a site on the Omada Controller.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the site.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the site (max 64 characters).",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "The country/region for the site (e.g., 'Romania', 'United States').",
				Required:    true,
			},
			"time_zone": schema.StringAttribute{
				Description: "The timezone for the site (IANA format, e.g., 'Europe/Athens', 'America/New_York').",
				Required:    true,
			},
			"scenario": schema.StringAttribute{
				Description: "The deployment scenario for the site (e.g., 'Home', 'Office', 'Hotel', 'Campus').",
				Required:    true,
			},
			"type": schema.Int64Attribute{
				Description: "The site type (0 = normal). Defaults to 0.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"device_account_username": schema.StringAttribute{
				Description: "The device management account username. Required for site creation. Used to SSH/manage adopted devices.",
				Optional:    true,
			},
			"device_account_password": schema.StringAttribute{
				Description: "The device management account password. Required for site creation. Must contain uppercase, lowercase, digits, and special characters.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (r *SiteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &client.SiteCreateRequest{
		Name:     plan.Name.ValueString(),
		Region:   plan.Region.ValueString(),
		TimeZone: plan.TimeZone.ValueString(),
		Scenario: plan.Scenario.ValueString(),
		Type:     int(plan.Type.ValueInt64()),
	}

	if !plan.DeviceAccountUsername.IsNull() && !plan.DeviceAccountUsername.IsUnknown() &&
		!plan.DeviceAccountPassword.IsNull() && !plan.DeviceAccountPassword.IsUnknown() {
		createReq.DeviceAccountSetting = &client.DeviceAccountInput{
			Username: plan.DeviceAccountUsername.ValueString(),
			Password: plan.DeviceAccountPassword.ValueString(),
		}
	}

	siteID, err := r.client.CreateSite(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating site", err.Error())
		return
	}

	// Read back the created site to populate all computed fields
	site, err := r.client.GetSite(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created site", err.Error())
		return
	}

	plan.ID = types.StringValue(site.ID)
	plan.Name = types.StringValue(site.Name)
	plan.Region = types.StringValue(site.Region)
	plan.TimeZone = types.StringValue(site.TimeZone)
	plan.Scenario = types.StringValue(site.Scenario)
	plan.Type = types.Int64Value(int64(site.Type))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	site, err := r.client.GetSite(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading site", err.Error())
		return
	}

	state.Name = types.StringValue(site.Name)
	state.Region = types.StringValue(site.Region)
	state.TimeZone = types.StringValue(site.TimeZone)
	state.Scenario = types.StringValue(site.Scenario)
	state.Type = types.Int64Value(int64(site.Type))
	// device_account_username and device_account_password are NOT returned by the site GET endpoint,
	// so we preserve whatever is in state (set during create or import of settings).

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state SiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	fields := &client.SiteSettingFields{
		Name:     plan.Name.ValueString(),
		Region:   plan.Region.ValueString(),
		TimeZone: plan.TimeZone.ValueString(),
		Scenario: plan.Scenario.ValueString(),
	}

	if err := r.client.UpdateSite(ctx, state.ID.ValueString(), fields); err != nil {
		resp.Diagnostics.AddError("Error updating site", err.Error())
		return
	}

	// Read back the updated site
	site, err := r.client.GetSite(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated site", err.Error())
		return
	}

	plan.ID = state.ID
	plan.Name = types.StringValue(site.Name)
	plan.Region = types.StringValue(site.Region)
	plan.TimeZone = types.StringValue(site.TimeZone)
	plan.Scenario = types.StringValue(site.Scenario)
	plan.Type = types.Int64Value(int64(site.Type))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSite(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting site", err.Error())
		return
	}
}

func (r *SiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	site, err := r.client.GetSite(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing site", err.Error())
		return
	}

	state := SiteResourceModel{
		ID:       types.StringValue(site.ID),
		Name:     types.StringValue(site.Name),
		Region:   types.StringValue(site.Region),
		TimeZone: types.StringValue(site.TimeZone),
		Scenario: types.StringValue(site.Scenario),
		Type:     types.Int64Value(int64(site.Type)),
		// device_account fields are not available from site GET — they come from site settings.
		// User should set these in their TF config if they want to manage them.
		DeviceAccountUsername: types.StringNull(),
		DeviceAccountPassword: types.StringNull(),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
