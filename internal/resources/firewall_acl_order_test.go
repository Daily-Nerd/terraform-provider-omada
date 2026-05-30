package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// =============================================================================
// Step 1 — buildIndexMap unit tests
// =============================================================================

// TestFirewallACLOrder_BuildIndexMap verifies that buildIndexMap produces
// 1-based dense positions matching the input order.
func TestFirewallACLOrder_BuildIndexMap(t *testing.T) {
	got := buildIndexMap([]string{"a", "b", "c"})
	want := map[string]int{"a": 1, "b": 2, "c": 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildIndexMap([a,b,c]) = %v, want %v", got, want)
	}
}

func TestFirewallACLOrder_BuildIndexMap_Empty(t *testing.T) {
	got := buildIndexMap([]string{})
	if len(got) != 0 {
		t.Errorf("buildIndexMap([]) = %v, want empty map", got)
	}
}

func TestFirewallACLOrder_BuildIndexMap_Single(t *testing.T) {
	got := buildIndexMap([]string{"only"})
	want := map[string]int{"only": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildIndexMap([only]) = %v, want %v", got, want)
	}
}

// =============================================================================
// Step 2 — resource-level wiring tests (Create + Read)
// =============================================================================

// aclOrderSchemaAttrTypes returns the tftypes attribute map for
// FirewallACLOrderResource's schema. Must stay in sync with Schema().
func aclOrderSchemaAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":       tftypes.String,
		"site_id":  tftypes.String,
		"type":     tftypes.Number,
		"rule_ids": tftypes.List{ElementType: tftypes.String},
	}
}

// buildACLOrderPlan constructs a tfsdk.Plan for FirewallACLOrderResource.
func buildACLOrderPlan(t *testing.T, r *FirewallACLOrderResource, siteID string, aclType int64, ruleIDs []string) tfsdk.Plan {
	t.Helper()

	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	attrTypes := aclOrderSchemaAttrTypes()

	ruleIDVals := make([]tftypes.Value, len(ruleIDs))
	for i, id := range ruleIDs {
		ruleIDVals[i] = tftypes.NewValue(tftypes.String, id)
	}

	rawVal := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, nil), // computed
		"site_id":  tftypes.NewValue(tftypes.String, siteID),
		"type":     tftypes.NewValue(tftypes.Number, new(big.Float).SetInt64(aclType)),
		"rule_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, ruleIDVals),
	})

	return tfsdk.Plan{
		Raw:    rawVal,
		Schema: schemaResp.Schema,
	}
}

// mockACLOrderServer builds a minimal mock controller for the ACL order resource.
// It handles:
//   - GET  /api/info
//   - POST /{id}/api/v2/login
//   - POST /{id}/api/v2/sites/{site}/cmd/acls/modifyIndex  → records the payload
//   - GET  /{id}/api/v2/sites/{site}/setting/firewall/acls → returns rules in a specified order
func mockACLOrderServer(
	t *testing.T,
	siteID string,
	storedRules []client.ACLRule,
	modifyHits *int,
	lastModifyBody *map[string]int,
) *httptest.Server {
	t.Helper()
	omadacID := "test-omadac-id"
	token := "test-csrf-token"

	mux := http.NewServeMux()

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0,
			Msg:       "Success.",
			Result:    mustMarshalWiring(t, client.ControllerInfo{OmadacID: omadacID, ControllerVer: "6.1.0", APIVer: "3"}),
		})
	})

	mux.HandleFunc(fmt.Sprintf("/%s/api/v2/login", omadacID), func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0,
			Msg:       "Success.",
			Result:    mustMarshalWiring(t, client.LoginResult{Token: token}),
		})
	})

	// POST modifyIndex
	modifyPath := fmt.Sprintf("/%s/api/v2/sites/%s/cmd/acls/modifyIndex", omadacID, siteID)
	mux.HandleFunc(modifyPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			*modifyHits++
			var body struct {
				Indexes map[string]int `json:"indexes"`
				Type    int            `json:"type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decoding modifyIndex body: %v", err)
			}
			*lastModifyBody = body.Indexes
		}
		json.NewEncoder(w).Encode(client.APIResponse{ErrorCode: 0, Msg: "Success."})
	})

	// GET ACL rules list
	aclListPath := fmt.Sprintf("/%s/api/v2/sites/%s/setting/firewall/acls", omadacID, siteID)
	mux.HandleFunc(aclListPath, func(w http.ResponseWriter, _ *http.Request) {
		listResult := client.ACLListResult{
			TotalRows:   len(storedRules),
			CurrentPage: 1,
			CurrentSize: len(storedRules),
			Data:        storedRules,
		}
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0,
			Msg:       "Success.",
			Result:    mustMarshalWiring(t, listResult),
		})
	})

	return httptest.NewServer(mux)
}

// TestFirewallACLOrder_Create_CallsModifyIndex verifies that Create calls
// ModifyACLIndex with the correct 1-based position map.
func TestFirewallACLOrder_Create_CallsModifyIndex(t *testing.T) {
	siteID := "site-abc"
	aclType := 0
	ruleIDs := []string{"rule-1", "rule-2", "rule-3"}

	var modifyHits int
	var lastModifyBody map[string]int

	storedRules := []client.ACLRule{
		{ID: "rule-1", Index: 1},
		{ID: "rule-2", Index: 2},
		{ID: "rule-3", Index: 3},
	}

	server := mockACLOrderServer(t, siteID, storedRules, &modifyHits, &lastModifyBody)
	defer server.Close()

	c, err := client.NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	r := &FirewallACLOrderResource{client: c}

	ctx := context.Background()
	plan := buildACLOrderPlan(t, r, siteID, int64(aclType), ruleIDs)

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	nullRaw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)

	req := resource.CreateRequest{Plan: plan}
	resp := resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: nullRaw},
	}

	r.Create(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned errors: %v", resp.Diagnostics)
	}

	if modifyHits != 1 {
		t.Errorf("modifyIndex called %d times, want 1", modifyHits)
	}

	wantIndexes := map[string]int{"rule-1": 1, "rule-2": 2, "rule-3": 3}
	if !reflect.DeepEqual(lastModifyBody, wantIndexes) {
		t.Errorf("modifyIndex payload = %v, want %v", lastModifyBody, wantIndexes)
	}

	// Verify computed id is set correctly.
	var state FirewallACLOrderModel
	resp.State.Get(ctx, &state)
	wantID := fmt.Sprintf("%s:0", siteID)
	if state.ID.ValueString() != wantID {
		t.Errorf("state.id = %q, want %q", state.ID.ValueString(), wantID)
	}
}

// TestFirewallACLOrder_Read_SortsRulesByIndex verifies that Read sorts the
// controller's rules by Index and reflects that order into state.rule_ids.
func TestFirewallACLOrder_Read_SortsRulesByIndex(t *testing.T) {
	siteID := "site-xyz"

	// Controller returns rules in a scrambled order; Read must sort by Index.
	storedRules := []client.ACLRule{
		{ID: "rule-c", Index: 3},
		{ID: "rule-a", Index: 1},
		{ID: "rule-b", Index: 2},
	}

	var modifyHits int
	var lastModifyBody map[string]int

	server := mockACLOrderServer(t, siteID, storedRules, &modifyHits, &lastModifyBody)
	defer server.Close()

	c, err := client.NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	r := &FirewallACLOrderResource{client: c}

	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	// Seed state as if a previous Create had run.
	attrTypes := aclOrderSchemaAttrTypes()
	existingRuleVals := []tftypes.Value{
		tftypes.NewValue(tftypes.String, "rule-c"),
		tftypes.NewValue(tftypes.String, "rule-a"),
		tftypes.NewValue(tftypes.String, "rule-b"),
	}
	stateRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, fmt.Sprintf("%s:0", siteID)),
		"site_id":  tftypes.NewValue(tftypes.String, siteID),
		"type":     tftypes.NewValue(tftypes.Number, new(big.Float).SetInt64(0)),
		"rule_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, existingRuleVals),
	})

	readReq := resource.ReadRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}
	readResp := resource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}

	r.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read returned errors: %v", readResp.Diagnostics)
	}

	var state FirewallACLOrderModel
	readResp.State.Get(ctx, &state)

	var gotIDs []string
	state.RuleIDs.ElementsAs(ctx, &gotIDs, false)

	// Sort storedRules by Index to get expected order.
	sorted := make([]client.ACLRule, len(storedRules))
	copy(sorted, storedRules)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })

	wantIDs := make([]string, len(sorted))
	for i, rule := range sorted {
		wantIDs[i] = rule.ID
	}

	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("Read rule_ids = %v, want %v (sorted by index)", gotIDs, wantIDs)
	}
}

// TestFirewallACLOrder_Read_ExcludesOutOfBandRules verifies that Read only ever
// reflects the managed rule IDs (those already in state.rule_ids) and never
// absorbs out-of-band rules that exist on the controller but are not managed.
//
// Regression contract: if Read reverts to setting rule_ids to ALL controller
// rules, OUTSIDER would leak into state and the next apply would reorder it —
// destructive.
func TestFirewallACLOrder_Read_ExcludesOutOfBandRules(t *testing.T) {
	siteID := "site-oob"

	// Controller reports four rules sorted by index: a, b, OUTSIDER, c.
	// OUTSIDER is not in the managed set and must be excluded.
	storedRules := []client.ACLRule{
		{ID: "managed-a", Index: 1},
		{ID: "managed-b", Index: 2},
		{ID: "OUTSIDER", Index: 3},
		{ID: "managed-c", Index: 4},
	}

	var modifyHits int
	var lastModifyBody map[string]int

	server := mockACLOrderServer(t, siteID, storedRules, &modifyHits, &lastModifyBody)
	defer server.Close()

	c, err := client.NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	r := &FirewallACLOrderResource{client: c}

	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	// Seed state with only the managed IDs (OUTSIDER absent).
	attrTypes := aclOrderSchemaAttrTypes()
	managedVals := []tftypes.Value{
		tftypes.NewValue(tftypes.String, "managed-a"),
		tftypes.NewValue(tftypes.String, "managed-b"),
		tftypes.NewValue(tftypes.String, "managed-c"),
	}
	stateRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, fmt.Sprintf("%s:0", siteID)),
		"site_id":  tftypes.NewValue(tftypes.String, siteID),
		"type":     tftypes.NewValue(tftypes.Number, new(big.Float).SetInt64(0)),
		"rule_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, managedVals),
	})

	readReq := resource.ReadRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}
	readResp := resource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}

	r.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read returned errors: %v", readResp.Diagnostics)
	}

	var state FirewallACLOrderModel
	readResp.State.Get(ctx, &state)

	var gotIDs []string
	state.RuleIDs.ElementsAs(ctx, &gotIDs, false)

	wantIDs := []string{"managed-a", "managed-b", "managed-c"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("Read rule_ids = %v, want %v (OUTSIDER must be excluded)", gotIDs, wantIDs)
	}
}

// TestFirewallACLOrder_Read_DropsMissingManagedRule verifies that a managed
// rule the controller no longer reports drops out of rule_ids (surfacing as
// drift on the next plan).
func TestFirewallACLOrder_Read_DropsMissingManagedRule(t *testing.T) {
	siteID := "site-drop"

	// managed-b is gone from the controller.
	storedRules := []client.ACLRule{
		{ID: "managed-a", Index: 1},
		{ID: "managed-c", Index: 2},
	}

	var modifyHits int
	var lastModifyBody map[string]int

	server := mockACLOrderServer(t, siteID, storedRules, &modifyHits, &lastModifyBody)
	defer server.Close()

	c, err := client.NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	r := &FirewallACLOrderResource{client: c}

	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	attrTypes := aclOrderSchemaAttrTypes()
	managedVals := []tftypes.Value{
		tftypes.NewValue(tftypes.String, "managed-a"),
		tftypes.NewValue(tftypes.String, "managed-b"),
		tftypes.NewValue(tftypes.String, "managed-c"),
	}
	stateRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, fmt.Sprintf("%s:0", siteID)),
		"site_id":  tftypes.NewValue(tftypes.String, siteID),
		"type":     tftypes.NewValue(tftypes.Number, new(big.Float).SetInt64(0)),
		"rule_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, managedVals),
	})

	readResp := resource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw}}, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read returned errors: %v", readResp.Diagnostics)
	}

	var state FirewallACLOrderModel
	readResp.State.Get(ctx, &state)
	var gotIDs []string
	state.RuleIDs.ElementsAs(ctx, &gotIDs, false)

	wantIDs := []string{"managed-a", "managed-c"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("Read rule_ids = %v, want %v (missing managed-b must drop out)", gotIDs, wantIDs)
	}
}

// TestFirewallACLOrder_OrderedManagedIDs_FiltersByType verifies that the
// orderedManagedIDs helper drops rules of a different ACL type even when their
// ID is in the managed set (defensive guard against controller type pollution).
func TestFirewallACLOrder_OrderedManagedIDs_FiltersByType(t *testing.T) {
	rules := []client.ACLRule{
		{ID: "gw-a", Type: 0, Index: 2},
		{ID: "sw-x", Type: 1, Index: 1}, // wrong type, must be excluded
		{ID: "gw-b", Type: 0, Index: 1},
	}
	managed := []string{"gw-a", "gw-b", "sw-x"}

	got := orderedManagedIDs(rules, 0, managed)
	want := []string{"gw-b", "gw-a"} // type 0 only, sorted by index
	if !reflect.DeepEqual(got, want) {
		t.Errorf("orderedManagedIDs = %v, want %v (type-1 rule must be excluded)", got, want)
	}
}

// TestFirewallACLOrder_Schema_TypeRequiresReplace verifies the type attribute
// is marked RequiresReplace so a type change replaces the resource instead of
// reordering the wrong ACL type in place.
func TestFirewallACLOrder_Schema_TypeRequiresReplace(t *testing.T) {
	r := NewFirewallACLOrderResource()
	sp, ok := r.(interface {
		Schema(context.Context, resource.SchemaRequest, *resource.SchemaResponse)
	})
	if !ok {
		t.Fatal("resource does not implement Schema")
	}

	var schemaResp resource.SchemaResponse
	sp.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	attr, ok := schemaResp.Schema.Attributes["type"]
	if !ok {
		t.Fatal("type attribute missing from schema")
	}
	int64Attr, ok := attr.(schema.Int64Attribute)
	if !ok {
		t.Fatalf("type attribute is not Int64Attribute, got %T", attr)
	}
	if len(int64Attr.PlanModifiers) == 0 {
		t.Fatal("type attribute has no plan modifiers — expected RequiresReplace")
	}
}
