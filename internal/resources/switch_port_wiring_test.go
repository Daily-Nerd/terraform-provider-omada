package resources

// switch_port_wiring_test.go — CI-runnable wiring tests for the switch port resource.
//
// These tests run in the internal/resources package (go test ./internal/resources/)
// and are part of the standard `make test` lane. No Terraform binary or provider
// server is required — they use a *client.Client pointed at an httptest.Server.
//
// Complement in the provider package:
//   - TestSwitchPort_UntagNetworkIDs_HCLSetRaisesPlanError (make testacc lane)
//     verifies the plan-error at the Terraform HCL level (requires terraform binary).

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// =============================================================================
// Shared test helpers (wiring tests only — no provider import needed)
// =============================================================================

// mustMarshalWiring marshals v to json.RawMessage, failing the test on error.
func mustMarshalWiring(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshalWiring: %v", err)
	}
	return b
}

// mockSwitchPortWiringServer builds a minimal mock controller that covers the
// paths exercised by SwitchPortResource.Create:
//
//   - GET  /api/info            → ControllerInfo (omadacID bootstrap)
//   - POST /{id}/api/v2/login   → LoginResult (CSRF token)
//   - PATCH /openapi/v1/{id}/sites/{site}/switches/{mac}/ports/{port}
//     → SUCCESS; openapiHits.Add(1)
//   - GET  /{id}/api/v2/sites/{site}/switches/{mac}
//     → SwitchConfig with the requested port (re-read inside UpdateSwitchPortV2)
//   - PATCH /{id}/api/v2/sites/{site}/switches/{mac}/ports/{port}
//     → ERROR; v2WriteHits.Add(1) and t.Errorf (old path — must never be called)
func mockSwitchPortWiringServer(
	t *testing.T,
	siteID, mac string,
	port int,
	openapiHits, v2WriteHits *atomic.Int32,
) *httptest.Server {
	t.Helper()
	omadacID := "test-omadac-id"
	token := "test-csrf-token"

	switchCfg := client.SwitchConfig{
		MAC:  mac,
		Name: "test-switch",
		Ports: []client.SwitchPort{
			{
				Port:                      port,
				Name:                      "port-3",
				ProfileID:                 "",
				ProfileOverrideEnable:     false,
				ProfileVlanOverrideEnable: false,
				NativeNetworkID:           "",
				NetworkTagsSetting:        0,
				Speed:                     0,
				Disable:                   false,
			},
		},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0, Msg: "Success.",
			Result: mustMarshalWiring(t, client.ControllerInfo{
				OmadacID:      omadacID,
				ControllerVer: "6.1.0.19",
				APIVer:        "3",
			}),
		})
	})

	mux.HandleFunc(fmt.Sprintf("/%s/api/v2/login", omadacID), func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0, Msg: "Success.",
			Result: mustMarshalWiring(t, client.LoginResult{Token: token}),
		})
	})

	// openapi/v1 PATCH — correct write path; must be hit by Create.
	openAPIPath := fmt.Sprintf("/openapi/v1/%s/sites/%s/switches/%s/ports/%d", omadacID, siteID, mac, port)
	mux.HandleFunc(openAPIPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			openapiHits.Add(1)
		}
		json.NewEncoder(w).Encode(client.APIResponse{ErrorCode: 0, Msg: "Success."})
	})

	// api/v2 GET — GetSwitchPort re-read inside UpdateSwitchPortV2.
	getSwitchPath := fmt.Sprintf("/%s/api/v2/sites/%s/switches/%s", omadacID, siteID, mac)
	mux.HandleFunc(getSwitchPath, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0,
			Result:    mustMarshalWiring(t, switchCfg),
		})
	})

	// api/v2 PATCH on the port path — old UpdateSwitchPort path; must NOT be hit.
	v2PortPath := fmt.Sprintf("/%s/api/v2/sites/%s/switches/%s/ports/%d", omadacID, siteID, mac, port)
	mux.HandleFunc(v2PortPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			v2WriteHits.Add(1)
			t.Errorf("PATCH reached api/v2 port path %q — Create/Update must use openapi/v1 only", v2PortPath)
		}
		json.NewEncoder(w).Encode(client.APIResponse{ErrorCode: 0, Msg: "Success."})
	})

	return httptest.NewServer(mux)
}

// switchPortSchemaAttrTypes returns the tftypes.Type map for all attributes in
// SwitchPortResource's schema. This must stay in sync with the schema defined
// in Schema(). Mismatches cause tftypes.Value construction to panic.
func switchPortSchemaAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":                           tftypes.String,
		"site_id":                      tftypes.String,
		"device_mac":                   tftypes.String,
		"port":                         tftypes.Number,
		"name":                         tftypes.String,
		"disable":                      tftypes.Bool,
		"profile_id":                   tftypes.String,
		"profile_override_enable":      tftypes.Bool,
		"profile_vlan_override_enable": tftypes.Bool,
		"native_network_id":            tftypes.String,
		"network_tags_setting":         tftypes.Number,
		"tag_network_ids":              tftypes.List{ElementType: tftypes.String},
		"untag_network_ids":            tftypes.List{ElementType: tftypes.String},
		"voice_network_enable":         tftypes.Bool,
		"voice_dscp_enable":            tftypes.Bool,
		"speed":                        tftypes.Number,
		"operation":                    tftypes.String,
		"mirrored_ports":               tftypes.Set{ElementType: tftypes.Number},
	}
}

// buildSwitchPortPlan constructs a tfsdk.Plan for SwitchPortResource with the
// given required fields. Optional/Computed fields are set to their zero values;
// Computed-only fields (id, untag_network_ids) are set to null.
//
// tftypes.Number requires a *big.Float value (not int64). Zero-value numbers
// use big.NewFloat(0); nil means null (not set).
func buildSwitchPortPlan(t *testing.T, r *SwitchPortResource, siteID, mac string, port int64) tfsdk.Plan {
	t.Helper()

	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	attrTypes := switchPortSchemaAttrTypes()

	rawVal := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		// Computed-only (no user input allowed)
		"id":                tftypes.NewValue(tftypes.String, nil),
		"untag_network_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),

		// Required
		"site_id":    tftypes.NewValue(tftypes.String, siteID),
		"device_mac": tftypes.NewValue(tftypes.String, mac),
		"port":       tftypes.NewValue(tftypes.Number, new(big.Float).SetInt64(port)),

		// Optional+Computed with defaults (provide explicit values matching defaults)
		"disable":                      tftypes.NewValue(tftypes.Bool, false),
		"profile_override_enable":      tftypes.NewValue(tftypes.Bool, false),
		"profile_vlan_override_enable": tftypes.NewValue(tftypes.Bool, false),
		"voice_network_enable":         tftypes.NewValue(tftypes.Bool, false),
		"voice_dscp_enable":            tftypes.NewValue(tftypes.Bool, false),

		// Optional+Computed (null = not set by user)
		"name":                 tftypes.NewValue(tftypes.String, nil),
		"profile_id":           tftypes.NewValue(tftypes.String, nil),
		"native_network_id":    tftypes.NewValue(tftypes.String, nil),
		"network_tags_setting": tftypes.NewValue(tftypes.Number, nil),
		"tag_network_ids":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"speed":                tftypes.NewValue(tftypes.Number, nil),
		"operation":            tftypes.NewValue(tftypes.String, nil),
		"mirrored_ports":       tftypes.NewValue(tftypes.Set{ElementType: tftypes.Number}, nil),
	})

	return tfsdk.Plan{
		Raw:    rawVal,
		Schema: schemaResp.Schema,
	}
}

// =============================================================================
// W-1: Create→openapi/v1 wiring, runnable under go test ./internal/resources/
// =============================================================================

// TestSwitchPortCreate_WiresOpenAPIPath_ResourcesPackage proves that
// SwitchPortResource.Create routes its PATCH to /openapi/v1/... and NOT to
// /api/v2/... when calling UpdateSwitchPortV2.
//
// This test exercises the handler→client call path using a *client.Client
// pointed at an httptest.Server. It requires no Terraform binary, no provider
// server, and no import of the provider package, so it runs cleanly in the
// make test / go test ./internal/resources/ lane.
//
// Regression contract: if Create is reverted to call the old UpdateSwitchPort
// (api/v2 path), openapiHits will be 0 and v2WriteHits will be >0 and the
// mock will call t.Errorf for the unexpected PATCH.
func TestSwitchPortCreate_WiresOpenAPIPath_ResourcesPackage(t *testing.T) {
	siteID := "site-1"
	mac := "aa:bb:cc:dd:ee:ff"
	port := 3

	var openapiHits atomic.Int32
	var v2WriteHits atomic.Int32

	server := mockSwitchPortWiringServer(t, siteID, mac, port, &openapiHits, &v2WriteHits)
	defer server.Close()

	c, err := client.NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	r := &SwitchPortResource{client: c}

	ctx := context.Background()
	plan := buildSwitchPortPlan(t, r, siteID, mac, int64(port))

	// The framework normally pre-populates CreateResponse.State with the schema
	// and a null raw value before calling Create. We must do the same here,
	// otherwise resp.State.Set panics with a nil-schema dereference.
	var schemaForState resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaForState)
	nullStateRaw := tftypes.NewValue(schemaForState.Schema.Type().TerraformType(ctx), nil)

	req := resource.CreateRequest{Plan: plan}
	resp := resource.CreateResponse{
		State: tfsdk.State{
			Schema: schemaForState.Schema,
			Raw:    nullStateRaw,
		},
	}

	r.Create(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics errors: %v", resp.Diagnostics)
	}

	if openapiHits.Load() == 0 {
		t.Errorf("Create did not hit openapi/v1 PATCH (openapiHits=%d) — wiring may have been reverted to api/v2", openapiHits.Load())
	}
	if v2WriteHits.Load() > 0 {
		t.Errorf("Create hit api/v2 write path %d time(s) — must use openapi/v1 only", v2WriteHits.Load())
	}
}

// =============================================================================
// W-2: untag_network_ids schema check — Computed-only, no Optional flag
// =============================================================================

// TestSwitchPort_UntagNetworkIDs_ComputedOnly_Schema verifies that the
// untag_network_ids schema attribute is Computed-only (IsComputed==true,
// IsOptional==false). This is a fast, no-binary schema introspection test.
//
// Runs in the make test / go test ./internal/resources/ lane.
// The complementary plan-error test (TestSwitchPort_UntagNetworkIDs_HCLSetRaisesPlanError)
// lives in internal/provider/ and runs in the make testacc lane.
func TestSwitchPort_UntagNetworkIDs_ComputedOnly_Schema_ResourcesPackage(t *testing.T) {
	r := NewSwitchPortResource()

	type schemaProvider interface {
		Schema(context.Context, resource.SchemaRequest, *resource.SchemaResponse)
	}
	sp, ok := r.(schemaProvider)
	if !ok {
		t.Fatal("NewSwitchPortResource() does not implement Schema method")
	}

	var schemaResp resource.SchemaResponse
	sp.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	attr, ok := schemaResp.Schema.Attributes["untag_network_ids"]
	if !ok {
		t.Fatal("untag_network_ids attribute missing from schema")
	}
	if !attr.IsComputed() {
		t.Error("untag_network_ids must be Computed:true")
	}
	if attr.IsOptional() {
		t.Error("untag_network_ids must NOT be Optional — it is Computed-only; setting it in HCL must raise a plan error")
	}
}
