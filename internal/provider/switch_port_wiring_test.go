package provider

// switch_port_wiring_test.go — provider-layer switch port wiring tests.
//
// This file runs in the internal/provider package under `make testacc`
// (TF_ACC=1 + live credentials, or resource.UnitTest which bypasses TF_ACC but
// requires the terraform binary in PATH).
//
// Relocated CI-runnable tests (no terraform binary required):
//   - TestSwitchPortCreate_WiresOpenAPIPath_ResourcesPackage
//     → internal/resources/switch_port_wiring_test.go (make test lane)
//   - TestSwitchPort_UntagNetworkIDs_ComputedOnly_Schema_ResourcesPackage
//     → internal/resources/switch_port_wiring_test.go (make test lane)
//
// Tests kept here complement the schema-level guarantee with a full HCL
// plan-error verification that requires the terraform binary.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// mockSwitchPortServer builds a full mock controller for provider-layer tests.
// It handles /api/info, login, the openapi PATCH, the api/v2 GET re-read, and
// a catch-all that fails the test if a legacy api/v2 PATCH is called.
func mockSwitchPortServer(t *testing.T, siteID, mac string, port int, openapiHits, v2WriteHits *atomic.Int32) *httptest.Server {
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

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0, Msg: "Success.",
			Result: mustMarshalHelper(t, client.ControllerInfo{OmadacID: omadacID, ControllerVer: "6.1.0.19", APIVer: "3"}),
		})
	})

	mux.HandleFunc(fmt.Sprintf("/%s/api/v2/login", omadacID), func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0, Msg: "Success.",
			Result: mustMarshalHelper(t, client.LoginResult{Token: token}),
		})
	})

	openAPIPath := fmt.Sprintf("/openapi/v1/%s/sites/%s/switches/%s/ports/%d", omadacID, siteID, mac, port)
	mux.HandleFunc(openAPIPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			openapiHits.Add(1)
		}
		json.NewEncoder(w).Encode(client.APIResponse{ErrorCode: 0, Msg: "Success."})
	})

	getSwitchPath := fmt.Sprintf("/%s/api/v2/sites/%s/switches/%s", omadacID, siteID, mac)
	mux.HandleFunc(getSwitchPath, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(client.APIResponse{
			ErrorCode: 0,
			Result:    mustMarshalHelper(t, switchCfg),
		})
	})

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

func mustMarshalHelper(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshalHelper: %v", err)
	}
	return b
}

// =============================================================================
// W-2 (provider layer): untag_network_ids plan-error — make testacc lane
//
// This test complements TestSwitchPort_UntagNetworkIDs_ComputedOnly_Schema_ResourcesPackage
// (internal/resources, make test lane) by verifying the plan-time error at the
// Terraform HCL level. It uses resource.UnitTest (bypasses TF_ACC) but requires
// the terraform binary in PATH, so it only runs under make testacc.
// =============================================================================

// TestSwitchPort_UntagNetworkIDs_HCLSetRaisesPlanError verifies that a
// Terraform config that explicitly sets untag_network_ids causes a plan-time
// error from the framework ("Can't configure a value for" / "unconfigurable").
// PlanOnly:true ensures no API call is made — the error fires during plan.
func TestSwitchPort_UntagNetworkIDs_HCLSetRaisesPlanError(t *testing.T) {
	siteID := "site-1"
	mac := "aa:bb:cc:dd:ee:ff"
	port := 3

	var openapiHits atomic.Int32
	var v2WriteHits atomic.Int32
	server := mockSwitchPortServer(t, siteID, mac, port, &openapiHits, &v2WriteHits)
	defer server.Close()

	providerFactories := map[string]func() (tfprotov6.ProviderServer, error){
		"omada": providerserver.NewProtocol6WithError(New()),
	}

	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		Steps: []tfresource.TestStep{
			{
				PlanOnly: true,
				Config: fmt.Sprintf(`
provider "omada" {
  url      = %q
  username = "admin"
  password = "password"
}

resource "omada_switch_port" "test" {
  site_id           = %q
  device_mac        = %q
  port              = %d
  untag_network_ids = ["net-3"]
}
`, server.URL, siteID, mac, port),
				// The framework rejects user-supplied values for Computed-only attributes with
				// "Invalid Configuration for Read-Only Attribute" / "Cannot set value for this
				// attribute as the provider has marked it as read-only."
				ExpectError: regexp.MustCompile(`(?i)(read.only|cannot set value|unconfigurable|value will be decided automatically)`),
			},
		},
	})
}
