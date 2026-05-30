package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockOmadaServer creates a test HTTP server that mimics the Omada Controller API.
// It handles /api/info, login, and configurable site-scoped endpoints.
func mockOmadaServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	omadacID := "test-omadac-id"
	token := "test-csrf-token"

	mux := http.NewServeMux()

	// /api/info — return controller metadata
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: 0,
			Msg:       "Success.",
			Result: mustMarshal(t, ControllerInfo{
				OmadacID:      omadacID,
				ControllerVer: "6.1.0.19",
				APIVer:        "3",
			}),
		})
	})

	// Login
	mux.HandleFunc(fmt.Sprintf("/%s/api/v2/login", omadacID), func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: 0,
			Msg:       "Success.",
			Result:    mustMarshal(t, LoginResult{Token: token}),
		})
	})

	// Custom handlers
	for pattern, handler := range handlers {
		prefix := fmt.Sprintf("/%s/api/v2", omadacID)
		mux.HandleFunc(prefix+pattern, handler)
	}

	return httptest.NewServer(mux)
}

// mustMarshal marshals v to json.RawMessage, failing the test on error.
func mustMarshal(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return data
}

// paginatedResponse wraps data in the standard paginated envelope.
func paginatedResponse(t *testing.T, data interface{}) json.RawMessage {
	t.Helper()
	return mustMarshal(t, PaginatedResult{
		TotalRows:   1,
		CurrentPage: 1,
		CurrentSize: 100,
		Data:        mustMarshal(t, data),
	})
}

// newTestClient creates a Client connected to the mock server.
func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// =============================================================================
// NewClient / Auth Tests
// =============================================================================

// TestNewClient_LazyAuth verifies that NewClient does NOT issue any HTTP
// requests during construction. Authentication is deferred to first API call.
func TestNewClient_LazyAuth(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient should succeed without controller round-trip: %v", err)
	}
	if requestCount != 0 {
		t.Errorf("NewClient issued %d HTTP request(s); want 0 (lazy auth)", requestCount)
	}
}

// TestLazyAuth_FiresOnFirstAPICall verifies that auth happens on the first
// real API call and that omadacID + token are populated afterward.
func TestLazyAuth_FiresOnFirstAPICall(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, []Site{{ID: "site-1", Name: "Test"}}),
			})
		},
	})
	defer server.Close()

	c := newTestClient(t, server)

	// Before any API call, identity fields should be empty.
	if c.omadacID != "" || c.token != "" {
		t.Errorf("pre-call state: omadacID=%q token=%q, want both empty", c.omadacID, c.token)
	}

	// First API call triggers auth.
	if _, err := c.ListSites(context.Background()); err != nil {
		t.Fatalf("ListSites: %v", err)
	}

	if c.omadacID != "test-omadac-id" {
		t.Errorf("post-call omadacID = %q, want %q", c.omadacID, "test-omadac-id")
	}
	if c.token != "test-csrf-token" {
		t.Errorf("post-call token = %q, want %q", c.token, "test-csrf-token")
	}
}

// TestLazyAuth_OnlyAuthsOnce verifies that ensureAuth is idempotent — once
// omadacID + token are populated, subsequent calls do not re-fire /api/info
// or /login.
func TestLazyAuth_OnlyAuthsOnce(t *testing.T) {
	infoHits := 0
	loginHits := 0
	siteHits := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		infoHits++
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: 0,
			Result:    mustMarshal(t, ControllerInfo{OmadacID: "test-omadac-id"}),
		})
	})
	mux.HandleFunc("/test-omadac-id/api/v2/login", func(w http.ResponseWriter, r *http.Request) {
		loginHits++
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: 0,
			Result:    mustMarshal(t, LoginResult{Token: "test-csrf-token"}),
		})
	})
	mux.HandleFunc("/test-omadac-id/api/v2/sites", func(w http.ResponseWriter, r *http.Request) {
		siteHits++
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: 0,
			Result:    paginatedResponse(t, []Site{{ID: "s1", Name: "A"}}),
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c, err := NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := c.ListSites(context.Background()); err != nil {
			t.Fatalf("ListSites[%d]: %v", i, err)
		}
	}

	if infoHits != 1 {
		t.Errorf("/api/info hits = %d, want 1 (cached after first auth)", infoHits)
	}
	if loginHits != 1 {
		t.Errorf("/login hits = %d, want 1 (cached after first auth)", loginHits)
	}
	if siteHits != 3 {
		t.Errorf("/sites hits = %d, want 3 (one per ListSites call)", siteHits)
	}
}

// TestLazyAuth_ControllerInfoError verifies that controller info errors are
// surfaced on the first API call (not at NewClient time).
func TestLazyAuth_ControllerInfoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: -1,
			Msg:       "Controller unavailable",
		})
	}))
	defer server.Close()

	c, err := NewClient(server.URL, "admin", "password", true)
	if err != nil {
		t.Fatalf("NewClient should succeed (lazy auth): %v", err)
	}

	_, err = c.ListSites(context.Background())
	if err == nil {
		t.Fatal("expected error from ListSites when /api/info fails, got nil")
	}
	if !strings.Contains(err.Error(), "controller info") {
		t.Errorf("error = %q, expected to contain 'controller info'", err.Error())
	}
}

// TestLazyAuth_LoginError verifies that login errors surface on the first
// API call (not at NewClient time).
func TestLazyAuth_LoginError(t *testing.T) {
	omadacID := "test-omadac-id"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: 0,
			Result: mustMarshal(t, ControllerInfo{
				OmadacID: omadacID,
			}),
		})
	})
	mux.HandleFunc(fmt.Sprintf("/%s/api/v2/login", omadacID), func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: -30109,
			Msg:       "Invalid username or password.",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c, err := NewClient(server.URL, "admin", "wrong", true)
	if err != nil {
		t.Fatalf("NewClient should succeed (lazy auth): %v", err)
	}

	_, err = c.ListSites(context.Background())
	if err == nil {
		t.Fatal("expected error from ListSites with bad credentials, got nil")
	}
	if !strings.Contains(err.Error(), "logging in") {
		t.Errorf("error = %q, expected to contain 'logging in'", err.Error())
	}
}

func TestGetOmadacID(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, []Site{}),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	// Trigger lazy auth via any API call.
	if _, err := c.ListSites(context.Background()); err != nil {
		t.Fatalf("ListSites (auth trigger): %v", err)
	}

	if got := c.GetOmadacID(); got != "test-omadac-id" {
		t.Errorf("GetOmadacID() = %q, want %q", got, "test-omadac-id")
	}
}

// =============================================================================
// ListSites Tests
// =============================================================================

func TestListSites(t *testing.T) {
	sites := []Site{
		{ID: "site-1", Name: "Iasi"},
		{ID: "site-2", Name: "Darabani"},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, sites),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListSites(context.Background())
	if err != nil {
		t.Fatalf("ListSites: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sites, want 2", len(got))
	}
	if got[0].Name != "Iasi" {
		t.Errorf("sites[0].Name = %q, want %q", got[0].Name, "Iasi")
	}
	if got[1].Name != "Darabani" {
		t.Errorf("sites[1].Name = %q, want %q", got[1].Name, "Darabani")
	}
}

// =============================================================================
// ResolveSiteID Tests
// =============================================================================

func TestResolveSiteID_ByName(t *testing.T) {
	sites := []Site{
		{ID: "site-1", Name: "Iasi"},
		{ID: "site-2", Name: "Darabani"},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, sites),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	id, err := c.ResolveSiteID(context.Background(), "Darabani")
	if err != nil {
		t.Fatalf("ResolveSiteID: %v", err)
	}
	if id != "site-2" {
		t.Errorf("ResolveSiteID('Darabani') = %q, want %q", id, "site-2")
	}
}

func TestResolveSiteID_ByID(t *testing.T) {
	sites := []Site{
		{ID: "site-1", Name: "Iasi"},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, sites),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	id, err := c.ResolveSiteID(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("ResolveSiteID: %v", err)
	}
	if id != "site-1" {
		t.Errorf("ResolveSiteID('site-1') = %q, want %q", id, "site-1")
	}
}

func TestResolveSiteID_NotFound(t *testing.T) {
	sites := []Site{
		{ID: "site-1", Name: "Iasi"},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, sites),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.ResolveSiteID(context.Background(), "NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent site, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, expected to contain 'not found'", err.Error())
	}
}

func TestResolveSiteID_CaseInsensitive(t *testing.T) {
	sites := []Site{
		{ID: "site-1", Name: "Iasi"},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, sites),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	id, err := c.ResolveSiteID(context.Background(), "iasi")
	if err != nil {
		t.Fatalf("ResolveSiteID: %v", err)
	}
	if id != "site-1" {
		t.Errorf("ResolveSiteID('iasi') = %q, want %q", id, "site-1")
	}
}

// =============================================================================
// ListNetworks Tests
// =============================================================================

func TestListNetworks(t *testing.T) {
	networks := []Network{
		{ID: "net-1", Name: "Default", Purpose: "interface", Vlan: 1, GatewaySubnet: "192.168.0.1/24"},
		{ID: "net-2", Name: "AP_30_IOT", Purpose: "vlan", Vlan: 30},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/lan/networks": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, networks),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListNetworks(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d networks, want 2", len(got))
	}
	if got[0].Name != "Default" {
		t.Errorf("networks[0].Name = %q, want %q", got[0].Name, "Default")
	}
	if got[1].Vlan != 30 {
		t.Errorf("networks[1].Vlan = %d, want %d", got[1].Vlan, 30)
	}
}

// =============================================================================
// CreateNetwork Adopt Pattern Tests
// =============================================================================

func TestCreateNetwork_AdoptExisting(t *testing.T) {
	existingNetworks := []Network{
		{ID: "net-1", Name: "Default", Purpose: "interface", Vlan: 1},
		{ID: "net-2", Name: "AP_30_IOT", Purpose: "vlan", Vlan: 30},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/lan/networks": func(w http.ResponseWriter, r *http.Request) {
			// Only handle GET (list) for adopt check
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    paginatedResponse(t, existingNetworks),
				})
				return
			}
			// POST should not be reached for adopt
			t.Error("unexpected POST to create network — adopt should have returned existing")
			w.WriteHeader(http.StatusInternalServerError)
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	// Trying to create "AP_30_IOT" should adopt the existing one
	got, err := c.CreateNetwork(context.Background(), "site-1", &Network{Name: "AP_30_IOT", Purpose: "vlan", Vlan: 30})
	if err != nil {
		t.Fatalf("CreateNetwork (adopt): %v", err)
	}
	if got.ID != "net-2" {
		t.Errorf("adopted network ID = %q, want %q", got.ID, "net-2")
	}
}

// =============================================================================
// URL Builder Tests
// =============================================================================

func TestGlobalURL(t *testing.T) {
	c := &Client{
		baseURL:  "https://10.0.20.7:8043",
		omadacID: "abc123",
		token:    "mytoken",
	}
	got := c.globalURL("/sites")
	want := "https://10.0.20.7:8043/abc123/api/v2/sites?token=mytoken"
	if got != want {
		t.Errorf("globalURL = %q, want %q", got, want)
	}
}

func TestSiteURL(t *testing.T) {
	c := &Client{
		baseURL:  "https://10.0.20.7:8043",
		omadacID: "abc123",
		token:    "mytoken",
	}
	got := c.siteURL("site-1", "/setting/lan/networks")
	want := "https://10.0.20.7:8043/abc123/api/v2/sites/site-1/setting/lan/networks?token=mytoken"
	if got != want {
		t.Errorf("siteURL = %q, want %q", got, want)
	}
}

// =============================================================================
// decodePaginatedData Tests
// =============================================================================

func TestDecodePaginatedData_Paginated(t *testing.T) {
	data := []Site{{ID: "s1", Name: "Site1"}}
	paginated := PaginatedResult{
		TotalRows:   1,
		CurrentPage: 1,
		CurrentSize: 100,
		Data:        mustMarshal(t, data),
	}
	raw := mustMarshal(t, paginated)

	var result []Site
	if err := decodePaginatedData(raw, &result); err != nil {
		t.Fatalf("decodePaginatedData: %v", err)
	}
	if len(result) != 1 || result[0].Name != "Site1" {
		t.Errorf("got %+v, want [{ID:s1 Name:Site1}]", result)
	}
}

func TestDecodePaginatedData_DirectArray(t *testing.T) {
	data := []Site{{ID: "s1", Name: "Site1"}}
	raw := mustMarshal(t, data)

	var result []Site
	if err := decodePaginatedData(raw, &result); err != nil {
		t.Fatalf("decodePaginatedData: %v", err)
	}
	if len(result) != 1 || result[0].Name != "Site1" {
		t.Errorf("got %+v, want [{ID:s1 Name:Site1}]", result)
	}
}

// =============================================================================
// isEmptyResult Tests
// =============================================================================

func TestIsEmptyResult(t *testing.T) {
	tests := []struct {
		name  string
		input json.RawMessage
		want  bool
	}{
		{"nil", nil, true},
		{"empty bytes", json.RawMessage{}, true},
		{"null string", json.RawMessage(`null`), true},
		{"empty object", json.RawMessage(`{}`), true},
		{"empty string", json.RawMessage(`""`), true},
		{"empty array", json.RawMessage(`[]`), true},
		{"whitespace", json.RawMessage(`  `), true},
		{"non-empty object", json.RawMessage(`{"id":"123"}`), false},
		{"non-empty array", json.RawMessage(`[1]`), false},
		{"non-empty string", json.RawMessage(`"hello"`), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEmptyResult(tt.input)
			if got != tt.want {
				t.Errorf("isEmptyResult(%q) = %v, want %v", string(tt.input), got, tt.want)
			}
		})
	}
}

// =============================================================================
// isAgileSeriesError Tests
// =============================================================================

func TestIsAgileSeriesError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"unrelated error", fmt.Errorf("network timeout"), false},
		{"agile series error", fmt.Errorf("API error -39742: switch requires ES path"), true},
		{"agile in message", fmt.Errorf("code -39742 not supported"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAgileSeriesError(tt.err)
			if got != tt.want {
				t.Errorf("isAgileSeriesError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// =============================================================================
// ListWlanGroups Tests
// =============================================================================

func TestListWlanGroups(t *testing.T) {
	groups := []WlanGroup{
		{ID: "wg-1", Name: "Default"},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/wlans": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, groups),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListWlanGroups(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("ListWlanGroups: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d groups, want 1", len(got))
	}
	if got[0].Name != "Default" {
		t.Errorf("groups[0].Name = %q, want %q", got[0].Name, "Default")
	}
}

func TestGetDefaultWlanGroupID(t *testing.T) {
	groups := []WlanGroup{
		{ID: "wg-default", Name: "Default"},
		{ID: "wg-2", Name: "Custom"},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/wlans": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, groups),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	id, err := c.GetDefaultWlanGroupID(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("GetDefaultWlanGroupID: %v", err)
	}
	if id != "wg-default" {
		t.Errorf("GetDefaultWlanGroupID = %q, want %q", id, "wg-default")
	}
}

func TestGetDefaultWlanGroupID_Empty(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/wlans": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, []WlanGroup{}),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.GetDefaultWlanGroupID(context.Background(), "site-1")
	if err == nil {
		t.Fatal("expected error for empty WLAN groups, got nil")
	}
}

// =============================================================================
// BaseURL Normalization Test
// =============================================================================

func TestNewClient_TrailingSlashNormalization(t *testing.T) {
	server := mockOmadaServer(t, nil)
	defer server.Close()

	// The URL from the test server won't have trailing slash,
	// but we test that the client handles it
	c := newTestClient(t, server)
	if strings.HasSuffix(c.baseURL, "/") {
		t.Errorf("baseURL should not have trailing slash: %q", c.baseURL)
	}
}

// =============================================================================
// ACL Rules Tests
// =============================================================================

func TestListACLRules(t *testing.T) {
	rules := []ACLRule{
		{ID: "acl-1", Name: "Block IoT", Type: 0, Status: true, Policy: 0,
			Protocols: []int{6, 17}, SourceType: 0, SourceIDs: []string{"net-1"},
			DestinationType: 0, DestinationIDs: []string{"net-2"},
			Direction: ACLDirection{LanToWan: false, LanToLan: true}, Index: 0},
		{ID: "acl-2", Name: "Allow DNS", Type: 0, Status: true, Policy: 1,
			Protocols: []int{17}, SourceType: 0, SourceIDs: []string{"net-1"},
			DestinationType: 2, DestinationIDs: []string{"ipg-1"},
			Direction: ACLDirection{LanToWan: true, LanToLan: false}, Index: 1},
	}
	listResult := ACLListResult{
		TotalRows:   2,
		CurrentPage: 1,
		CurrentSize: 100,
		Data:        rules,
		ACLDisable:  false,
		SupportVPN:  true,
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			// Verify type query param
			if got := r.URL.Query().Get("type"); got != "0" {
				t.Errorf("type query param = %q, want %q", got, "0")
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, listResult),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListACLRules(context.Background(), "site-1", 0)
	if err != nil {
		t.Fatalf("ListACLRules: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rules, want 2", len(got))
	}
	if got[0].Name != "Block IoT" {
		t.Errorf("rules[0].Name = %q, want %q", got[0].Name, "Block IoT")
	}
	if got[1].Policy != 1 {
		t.Errorf("rules[1].Policy = %d, want %d", got[1].Policy, 1)
	}
	if !got[0].Direction.LanToLan {
		t.Error("rules[0].Direction.LanToLan = false, want true")
	}
}

func TestListACLRules_Empty(t *testing.T) {
	listResult := ACLListResult{
		TotalRows:   0,
		CurrentPage: 1,
		CurrentSize: 100,
		Data:        []ACLRule{},
		ACLDisable:  true,
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, listResult),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListACLRules(context.Background(), "site-1", 0)
	if err != nil {
		t.Fatalf("ListACLRules (empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rules, want 0", len(got))
	}
}

func TestGetACLRule_Found(t *testing.T) {
	rules := []ACLRule{
		{ID: "acl-1", Name: "Block IoT", Type: 0},
		{ID: "acl-2", Name: "Allow DNS", Type: 0},
	}
	listResult := ACLListResult{TotalRows: 2, CurrentPage: 1, CurrentSize: 100, Data: rules}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, listResult),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.GetACLRule(context.Background(), "site-1", "acl-2", 0)
	if err != nil {
		t.Fatalf("GetACLRule: %v", err)
	}
	if got.Name != "Allow DNS" {
		t.Errorf("Name = %q, want %q", got.Name, "Allow DNS")
	}
}

func TestGetACLRule_NotFound(t *testing.T) {
	listResult := ACLListResult{TotalRows: 0, CurrentPage: 1, CurrentSize: 100, Data: []ACLRule{}}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, listResult),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.GetACLRule(context.Background(), "site-1", "nonexistent", 0)
	if err == nil {
		t.Fatal("expected error for non-existent ACL rule, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, expected to contain 'not found'", err.Error())
	}
}

func TestCreateACLRule(t *testing.T) {
	created := ACLRule{
		ID: "acl-new", Name: "New Rule", Type: 0, Status: true,
		Policy: 0, Protocols: []int{6}, SourceType: 0, SourceIDs: []string{"net-1"},
		DestinationType: 0, DestinationIDs: []string{"net-2"}, Index: 5,
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, created),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &ACLRule{
		Name: "New Rule", Type: 0, Status: true, Policy: 0,
		Protocols: []int{6}, SourceType: 0, SourceIDs: []string{"net-1"},
		DestinationType: 0, DestinationIDs: []string{"net-2"},
	}
	got, err := c.CreateACLRule(context.Background(), "site-1", input)
	if err != nil {
		t.Fatalf("CreateACLRule: %v", err)
	}
	if got.ID != "acl-new" {
		t.Errorf("ID = %q, want %q", got.ID, "acl-new")
	}
	if got.Index != 5 {
		t.Errorf("Index = %d, want %d", got.Index, 5)
	}
}

func TestUpdateACLRule(t *testing.T) {
	updated := ACLRule{
		ID: "acl-1", Name: "Updated Rule", Type: 0, Status: true,
		Policy: 1, Protocols: []int{6, 17},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls/acl-1": func(w http.ResponseWriter, r *http.Request) {
			// ACL update must use PUT (PATCH returns -1600 on v6 controllers)
			if r.Method != http.MethodPut {
				t.Errorf("expected PUT, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, updated),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &ACLRule{Name: "Updated Rule", Type: 0, Policy: 1, Protocols: []int{6, 17}}
	got, err := c.UpdateACLRule(context.Background(), "site-1", "acl-1", input)
	if err != nil {
		t.Fatalf("UpdateACLRule: %v", err)
	}
	if got.Name != "Updated Rule" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated Rule")
	}
}

func TestUpdateACLRule_EmptyResult(t *testing.T) {
	rules := []ACLRule{
		{ID: "acl-1", Name: "Refreshed Rule", Type: 0, Status: true, Policy: 1},
	}
	listResult := ACLListResult{TotalRows: 1, CurrentPage: 1, CurrentSize: 100, Data: rules}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls/acl-1": func(w http.ResponseWriter, r *http.Request) {
			// ACL update must use PUT (PATCH returns -1600 on v6 controllers)
			if r.Method == http.MethodPut {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    json.RawMessage(`{}`),
				})
				return
			}
		},
		"/sites/site-1/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, listResult),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &ACLRule{Name: "Refreshed Rule", Type: 0, Policy: 1}
	got, err := c.UpdateACLRule(context.Background(), "site-1", "acl-1", input)
	if err != nil {
		t.Fatalf("UpdateACLRule (empty result): %v", err)
	}
	if got.Name != "Refreshed Rule" {
		t.Errorf("Name = %q, want %q", got.Name, "Refreshed Rule")
	}
}

func TestDeleteACLRule(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls/acl-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("expected DELETE, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    json.RawMessage(`{}`),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	err := c.DeleteACLRule(context.Background(), "site-1", "acl-1")
	if err != nil {
		t.Fatalf("DeleteACLRule: %v", err)
	}
}

// TestCreateACLRule_EmptyResult verifies that CreateACLRule handles a controller
// response where result is empty/null (the live bug: "unexpected end of JSON input").
// The impl should fall back to a list-then-match-by-id strategy using the rule
// returned by the follow-up GET (ListACLRules).
func TestCreateACLRule_EmptyResult(t *testing.T) {
	fullRule := ACLRule{
		ID: "acl-created", Name: "tf_block_iot", Type: 0, Status: true,
		Policy: 0, Protocols: []int{6, 17},
		SourceIDs: []string{"net-1"}, DestinationIDs: []string{"net-2"},
		CustomAclOsws: []string{}, CustomAclStacks: []string{}, CustomAclDevices: []string{},
	}
	listResult := ACLListResult{TotalRows: 1, CurrentPage: 1, CurrentSize: 100, Data: []ACLRule{fullRule}}

	listCallCount := 0
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				// Controller returns empty result (the live bug trigger).
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Msg:       "Success.",
					Result:    json.RawMessage(`""`),
				})
			case http.MethodGet:
				listCallCount++
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, listResult),
				})
			default:
				t.Errorf("unexpected method %s", r.Method)
			}
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &ACLRule{
		Name: "tf_block_iot", Type: 0, Status: true, Policy: 0,
		Protocols: []int{6, 17},
		SourceIDs: []string{"net-1"}, DestinationIDs: []string{"net-2"},
	}
	got, err := c.CreateACLRule(context.Background(), "site-1", input)
	if err != nil {
		t.Fatalf("CreateACLRule (empty result): %v", err)
	}
	if got.ID != "acl-created" {
		t.Errorf("ID = %q, want %q", got.ID, "acl-created")
	}
	if got.Name != "tf_block_iot" {
		t.Errorf("Name = %q, want %q", got.Name, "tf_block_iot")
	}
	if listCallCount == 0 {
		t.Error("expected at least one GET list call to re-fetch after empty create; got 0")
	}
}

// TestCreateACLRule_StringIDResult verifies that CreateACLRule handles a controller
// response where result is a bare quoted string ID (like CreateIPGroup).
// The impl should follow up with GetACLRule (list+match) and return the full rule.
func TestCreateACLRule_StringIDResult(t *testing.T) {
	createdID := "64a1b2c3d4e5f6a7b8c9d0e1"
	fullRule := ACLRule{
		ID: createdID, Name: "tf_allow_dns", Type: 0, Status: true,
		Policy: 1, Protocols: []int{17},
		SourceIDs: []string{"net-1"}, DestinationIDs: []string{"ipg-1"},
		CustomAclOsws: []string{}, CustomAclStacks: []string{}, CustomAclDevices: []string{},
	}
	listResult := ACLListResult{TotalRows: 1, CurrentPage: 1, CurrentSize: 100, Data: []ACLRule{fullRule}}

	listCallCount := 0
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/firewall/acls": func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				// Controller returns bare string ID.
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    json.RawMessage(`"` + createdID + `"`),
				})
			case http.MethodGet:
				listCallCount++
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, listResult),
				})
			default:
				t.Errorf("unexpected method %s", r.Method)
			}
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &ACLRule{
		Name: "tf_allow_dns", Type: 0, Status: true, Policy: 1,
		Protocols: []int{17},
		SourceIDs: []string{"net-1"}, DestinationIDs: []string{"ipg-1"},
	}
	got, err := c.CreateACLRule(context.Background(), "site-1", input)
	if err != nil {
		t.Fatalf("CreateACLRule (string-id result): %v", err)
	}
	if got.ID != createdID {
		t.Errorf("ID = %q, want %q", got.ID, createdID)
	}
	if got.Name != "tf_allow_dns" {
		t.Errorf("Name = %q, want %q", got.Name, "tf_allow_dns")
	}
	if listCallCount == 0 {
		t.Error("expected at least one GET list call to re-fetch after string-id create; got 0")
	}
}

// =============================================================================
// ACL Rule nil-IDs serialization tests
// =============================================================================

// TestACLRule_NilSourceDestIds_MarshalAsEmptyArray verifies that an ACLRule with
// nil SourceIDs / DestinationIDs (the "any" source/destination case) serializes
// both fields as [] rather than null. The Omada controller rejects null with
// -33609 "Choose the source and destination".
//
// RED: current normalizeACLRule does NOT initialize SourceIDs / DestinationIDs,
// so json.Marshal produces "sourceIds":null which the controller rejects.
func TestACLRule_NilSourceDestIds_MarshalAsEmptyArray(t *testing.T) {
	rule := &ACLRule{
		Name:            "Allow Any to Any",
		Type:            0,
		Status:          true,
		Policy:          1,
		Protocols:       []int{256},
		SourceType:      0,
		SourceIDs:       nil, // "any" — nil slice
		DestinationType: 0,
		DestinationIDs:  nil, // "any" — nil slice
	}

	normalizeACLRule(rule)

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	body := string(data)

	// nil slice must produce [] not null
	if !strings.Contains(body, `"sourceIds":[]`) {
		t.Errorf("want sourceIds:[] in JSON, got: %s", body)
	}
	if !strings.Contains(body, `"destinationIds":[]`) {
		t.Errorf("want destinationIds:[] in JSON, got: %s", body)
	}
}

// TestACLRule_PopulatedIds_Preserved verifies that non-nil (populated) IDs still
// serialize correctly after normalization.
func TestACLRule_PopulatedIds_Preserved(t *testing.T) {
	rule := &ACLRule{
		Name:            "Block IoT to WAN",
		Type:            0,
		Status:          true,
		Policy:          0,
		Protocols:       []int{6},
		SourceType:      0,
		SourceIDs:       []string{"net-iot"},
		DestinationType: 2,
		DestinationIDs:  []string{"ipg-internet"},
	}

	normalizeACLRule(rule)

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	body := string(data)

	if !strings.Contains(body, `"sourceIds":["net-iot"]`) {
		t.Errorf("want sourceIds:[net-iot] in JSON, got: %s", body)
	}
	if !strings.Contains(body, `"destinationIds":["ipg-internet"]`) {
		t.Errorf("want destinationIds:[ipg-internet] in JSON, got: %s", body)
	}
}

// =============================================================================
// IP Groups Tests
// =============================================================================

func TestListIPGroups(t *testing.T) {
	groups := []IPGroup{
		{ID: "ipg-1", Name: "DNS Servers", Type: 0, IPList: []IPGroupEntry{
			{IP: "8.8.8.8", Mask: 32, Description: ""},
			{IP: "1.1.1.1", Mask: 32, Description: ""},
		}},
		{ID: "ipg-2", Name: "Web Servers", Type: 0, IPList: []IPGroupEntry{
			{IP: "10.0.0.0", Mask: 24, Description: ""},
		}},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, groups),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListIPGroups(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("ListIPGroups: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2", len(got))
	}
	if got[0].Name != "DNS Servers" {
		t.Errorf("groups[0].Name = %q, want %q", got[0].Name, "DNS Servers")
	}
	if len(got[0].IPList) != 2 {
		t.Errorf("groups[0].IPList length = %d, want 2", len(got[0].IPList))
	}
	if got[0].IPList[0].IP != "8.8.8.8" {
		t.Errorf("groups[0].IPList[0].IP = %q, want %q", got[0].IPList[0].IP, "8.8.8.8")
	}
	if got[0].IPList[0].Mask != 32 {
		t.Errorf("groups[0].IPList[0].Mask = %d, want 32", got[0].IPList[0].Mask)
	}
}

func TestListIPGroups_Empty(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, []IPGroup{}),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListIPGroups(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("ListIPGroups (empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d groups, want 0", len(got))
	}
}

func TestGetIPGroup_Found(t *testing.T) {
	groups := []IPGroup{
		{ID: "ipg-1", Name: "DNS Servers", Type: 0},
		{ID: "ipg-2", Name: "Web Servers", Type: 0},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, groups),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.GetIPGroup(context.Background(), "site-1", "ipg-2")
	if err != nil {
		t.Fatalf("GetIPGroup: %v", err)
	}
	if got.Name != "Web Servers" {
		t.Errorf("Name = %q, want %q", got.Name, "Web Servers")
	}
}

func TestGetIPGroup_NotFound(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, []IPGroup{}),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.GetIPGroup(context.Background(), "site-1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent IP group, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, expected to contain 'not found'", err.Error())
	}
}

// TestGetIPGroup_NotFoundReturnsSentinel verifies that when the API returns a
// list that does NOT contain the requested group ID, GetIPGroup returns an
// error that wraps ErrNotFound so callers can detect drift via errors.Is.
// RED: fails until GetIPGroup wraps ErrNotFound on the not-found path.
func TestGetIPGroup_NotFoundReturnsSentinel(t *testing.T) {
	// List contains a different group — the requested ID is absent.
	other := IPGroup{ID: "ipg-other", Name: "Other Group", Type: 0,
		IPList: []IPGroupEntry{{IP: "10.0.0.0", Mask: 8}}}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, []IPGroup{other}),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.GetIPGroup(context.Background(), "site-1", "6a1a9ee744a75c2be561188a")
	if err == nil {
		t.Fatal("GetIPGroup: expected error for absent group ID, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetIPGroup not-found error does not wrap ErrNotFound: %v", err)
	}
}

func TestCreateIPGroup(t *testing.T) {
	// The v6/ER707 API returns a bare string ID on POST; the full object comes
	// from the follow-up GET (list + filter).
	fullGroup := json.RawMessage(`{"groupId":"ipg-new","name":"New Group","type":0,"ipList":[{"ip":"192.168.1.0","mask":24,"description":""}],"count":1}`)
	paginatedRaw := mustMarshal(t, map[string]interface{}{
		"totalRows": 1, "currentPage": 1, "currentSize": 100,
		"data": []json.RawMessage{fullGroup},
	})

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    json.RawMessage(`"ipg-new"`),
				})
				return
			}
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    paginatedRaw,
				})
				return
			}
			t.Errorf("unexpected method %s", r.Method)
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &IPGroup{
		Name: "New Group", Type: 0,
		IPList: []IPGroupEntry{{IP: "192.168.1.0", Mask: 24, Description: ""}},
	}
	got, err := c.CreateIPGroup(context.Background(), "site-1", input)
	if err != nil {
		t.Fatalf("CreateIPGroup: %v", err)
	}
	if got.ID != "ipg-new" {
		t.Errorf("ID = %q, want %q", got.ID, "ipg-new")
	}
	if got.IPList[0].IP != "192.168.1.0" {
		t.Errorf("IPList[0].IP = %q, want %q", got.IPList[0].IP, "192.168.1.0")
	}
	if got.IPList[0].Mask != 24 {
		t.Errorf("IPList[0].Mask = %d, want 24", got.IPList[0].Mask)
	}
}

func TestUpdateIPGroup(t *testing.T) {
	updated := IPGroup{
		ID: "ipg-1", Name: "Updated Group", Type: 0,
		IPList: []IPGroupEntry{{IP: "10.0.0.0", Mask: 8}},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups/ipg-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				t.Errorf("expected PUT, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, updated),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &IPGroup{Name: "Updated Group", Type: 0, IPList: []IPGroupEntry{{IP: "10.0.0.0", Mask: 8}}}
	got, err := c.UpdateIPGroup(context.Background(), "site-1", "ipg-1", input)
	if err != nil {
		t.Fatalf("UpdateIPGroup: %v", err)
	}
	if got.Name != "Updated Group" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated Group")
	}
}

func TestUpdateIPGroup_EmptyResult(t *testing.T) {
	groups := []IPGroup{
		{ID: "ipg-1", Name: "Refreshed Group", Type: 0, IPList: []IPGroupEntry{{IP: "10.0.0.0", Mask: 8}}},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups/ipg-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    json.RawMessage(`{}`),
				})
				return
			}
		},
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedResponse(t, groups),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &IPGroup{Name: "Refreshed Group", Type: 0}
	got, err := c.UpdateIPGroup(context.Background(), "site-1", "ipg-1", input)
	if err != nil {
		t.Fatalf("UpdateIPGroup (empty result): %v", err)
	}
	if got.Name != "Refreshed Group" {
		t.Errorf("Name = %q, want %q", got.Name, "Refreshed Group")
	}
}

func TestDeleteIPGroup(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups/ipg-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("expected DELETE, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    json.RawMessage(`{}`),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	err := c.DeleteIPGroup(context.Background(), "site-1", "ipg-1")
	if err != nil {
		t.Fatalf("DeleteIPGroup: %v", err)
	}
}

// =============================================================================
// IP Group v6 / profiles/groups path tests (RED → GREEN with v6 fix)
// =============================================================================

// TestIPGroup_CreateUsesProfilesGroupsPath asserts that CreateIPGroup POSTs to
// /setting/profiles/groups (the v6/ER707 path) and NOT to /setting/firewall/ipGroups.
func TestIPGroup_CreateUsesProfilesGroupsPath(t *testing.T) {
	fullGroup := json.RawMessage(`{"groupId":"ipg-v6","name":"Trusted Nets","type":0,"ipList":[{"ip":"10.10.50.0","mask":24,"description":""}],"count":1}`)
	paginatedRaw := mustMarshal(t, map[string]interface{}{
		"totalRows": 1, "currentPage": 1, "currentSize": 100,
		"data": []json.RawMessage{fullGroup},
	})

	hitPost := false
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				hitPost = true
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    json.RawMessage(`"ipg-v6"`),
				})
				return
			}
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    paginatedRaw,
				})
				return
			}
		},
		// Any hit to the old path should 404 so the test catches regressions.
		"/sites/site-1/setting/firewall/ipGroups": func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("CreateIPGroup hit legacy path /setting/firewall/ipGroups — should use /setting/profiles/groups")
			http.NotFound(w, r)
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &IPGroup{
		Name: "Trusted Nets",
		Type: 0,
		IPList: []IPGroupEntry{
			{IP: "10.10.50.0", Mask: 24, Description: ""},
		},
	}
	got, err := c.CreateIPGroup(context.Background(), "site-1", input)
	if err != nil {
		t.Fatalf("CreateIPGroup v6: %v", err)
	}
	if !hitPost {
		t.Error("CreateIPGroup did not hit /setting/profiles/groups with POST")
	}
	if got.ID != "ipg-v6" {
		t.Errorf("ID = %q, want %q", got.ID, "ipg-v6")
	}
}

// TestIPGroup_V6BodyShape asserts the marshaled create body matches the v6 wire shape:
//   - ipList entries have {ip, mask (int), description} — no CIDR string
//   - a /24 CIDR input produces mask:24; a bare host produces mask:32
//   - type:0 for IP-only groups
//   - null envelope fields present (ipv6List, macAddressList, portList, countryList, etc.)
func TestIPGroup_V6BodyShape(t *testing.T) {
	var capturedBody []byte
	// Full group returned by the follow-up GET (uses groupId as live API does).
	fullGroup := json.RawMessage(`{"groupId":"ipg-shape","name":"Shape Test","type":0,"ipList":[{"ip":"10.10.50.0","mask":24,"description":""},{"ip":"10.10.70.98","mask":32,"description":""}],"count":2}`)
	paginatedRaw := mustMarshal(t, map[string]interface{}{
		"totalRows": 1, "currentPage": 1, "currentSize": 100,
		"data": []json.RawMessage{fullGroup},
	})

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				var err error
				capturedBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("reading body: %v", err)
				}
				// Return string ID as real API does.
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    json.RawMessage(`"ipg-shape"`),
				})
				return
			}
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    paginatedRaw,
				})
				return
			}
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &IPGroup{
		Name: "Shape Test",
		Type: 0,
		IPList: []IPGroupEntry{
			{IP: "10.10.50.0", Mask: 24, Description: ""},
			{IP: "10.10.70.98", Mask: 32, Description: ""},
		},
	}
	if _, err := c.CreateIPGroup(context.Background(), "site-1", input); err != nil {
		t.Fatalf("CreateIPGroup body-shape: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	// type must be 0
	if tp, _ := body["type"].(float64); tp != 0 {
		t.Errorf("type = %v, want 0", body["type"])
	}

	// ipList entries must be {ip, mask (number), description}
	ipListRaw, ok := body["ipList"].([]interface{})
	if !ok || len(ipListRaw) != 2 {
		t.Fatalf("ipList = %v, want 2 entries", body["ipList"])
	}

	entry0, _ := ipListRaw[0].(map[string]interface{})
	if entry0["ip"] != "10.10.50.0" {
		t.Errorf("entry0.ip = %v, want 10.10.50.0", entry0["ip"])
	}
	if mask, _ := entry0["mask"].(float64); mask != 24 {
		t.Errorf("entry0.mask = %v, want 24", entry0["mask"])
	}
	if _, hasDesc := entry0["description"]; !hasDesc {
		t.Error("entry0 missing description field")
	}

	entry1, _ := ipListRaw[1].(map[string]interface{})
	if entry1["ip"] != "10.10.70.98" {
		t.Errorf("entry1.ip = %v, want 10.10.70.98", entry1["ip"])
	}
	if mask, _ := entry1["mask"].(float64); mask != 32 {
		t.Errorf("entry1.mask = %v, want 32 (bare host)", entry1["mask"])
	}

	// null envelope fields must be present (JSON null)
	for _, nullField := range []string{"ipv6List", "macAddressList", "portList", "countryList", "portType", "portMaskList", "domainNamePort", "ouiList"} {
		if _, exists := body[nullField]; !exists {
			t.Errorf("body missing null envelope field %q", nullField)
		}
	}

	// count must be present
	if _, exists := body["count"]; !exists {
		t.Error("body missing count field")
	}
}

// TestCIDRSplitHelper tests the splitCIDR helper that converts CIDR-or-bare-IP
// strings into separate (ip, mask) pairs for the v6 wire body.
func TestCIDRSplitHelper(t *testing.T) {
	cases := []struct {
		input    string
		wantIP   string
		wantMask int
		wantErr  bool
	}{
		{"10.10.50.0/24", "10.10.50.0", 24, false},
		{"192.168.1.0/24", "192.168.1.0", 24, false},
		{"10.10.70.98/32", "10.10.70.98", 32, false},
		{"10.10.70.98", "10.10.70.98", 32, false}, // bare host → mask 32
		{"8.8.8.8", "8.8.8.8", 32, false},
		{"not-an-ip", "", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			ip, mask, err := SplitCIDR(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("SplitCIDR(%q): expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("SplitCIDR(%q): unexpected error: %v", tc.input, err)
				return
			}
			if ip != tc.wantIP {
				t.Errorf("SplitCIDR(%q).ip = %q, want %q", tc.input, ip, tc.wantIP)
			}
			if mask != tc.wantMask {
				t.Errorf("SplitCIDR(%q).mask = %d, want %d", tc.input, mask, tc.wantMask)
			}
		})
	}
}

// TestIPGroup_UpdateUsesPUT asserts UpdateIPGroup uses PUT (not PATCH) to
// /setting/profiles/groups/{id}, mirroring the ACL update fix on this branch.
func TestIPGroup_UpdateUsesPUT(t *testing.T) {
	updated := IPGroup{
		ID: "ipg-1", Name: "Updated Group", Type: 0,
		IPList: []IPGroupEntry{{IP: "10.0.0.0", Mask: 8}},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups/ipg-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				t.Errorf("expected PUT on profiles/groups/{id}, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mustMarshal(t, updated),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &IPGroup{Name: "Updated Group", Type: 0, IPList: []IPGroupEntry{{IP: "10.0.0.0", Mask: 8}}}
	got, err := c.UpdateIPGroup(context.Background(), "site-1", "ipg-1", input)
	if err != nil {
		t.Fatalf("UpdateIPGroup v6: %v", err)
	}
	if got.Name != "Updated Group" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated Group")
	}
}

// =============================================================================
// IP Group v6 API field-name fix tests (groupId tag + string-id create)
// =============================================================================

// TestIPGroup_GroupIdTag_UnmarshalFromLiveAPI verifies that IPGroup.ID is
// populated when the API returns "groupId" (the real v6 wire field name).
// RED: fails with json:"id" tag; GREEN: passes with json:"groupId" tag.
func TestIPGroup_GroupIdTag_UnmarshalFromLiveAPI(t *testing.T) {
	// Realistic GET response from live ER707 controller.
	rawJSON := `{"groupId":"6a1a9aa744a75c2be56115d0","site":"Default","name":"tf_pihole","ipList":[{"ip":"10.10.70.98","mask":32,"description":""}],"count":1,"type":0,"resource":0}`

	var g IPGroup
	if err := json.Unmarshal([]byte(rawJSON), &g); err != nil {
		t.Fatalf("unmarshal IPGroup: %v", err)
	}
	if g.ID != "6a1a9aa744a75c2be56115d0" {
		t.Errorf("IPGroup.ID = %q, want %q (check json tag: should be groupId not id)",
			g.ID, "6a1a9aa744a75c2be56115d0")
	}
	if g.Name != "tf_pihole" {
		t.Errorf("IPGroup.Name = %q, want %q", g.Name, "tf_pihole")
	}
	if len(g.IPList) != 1 {
		t.Fatalf("IPGroup.IPList len = %d, want 1", len(g.IPList))
	}
	if g.IPList[0].IP != "10.10.70.98" {
		t.Errorf("IPList[0].IP = %q, want %q", g.IPList[0].IP, "10.10.70.98")
	}
	if g.IPList[0].Mask != 32 {
		t.Errorf("IPList[0].Mask = %d, want 32", g.IPList[0].Mask)
	}
}

// TestGetIPGroup_UsesGroupIdForMatch verifies that GetIPGroup correctly matches
// by ID when the server returns objects with the "groupId" field.
// RED: fails with json:"id" because g.ID is empty → never matches.
func TestGetIPGroup_UsesGroupIdForMatch(t *testing.T) {
	// Raw paginated payload using "groupId" as the live API does.
	rawGroup := json.RawMessage(`{"groupId":"abc123","name":"PiHole","type":0,"ipList":[{"ip":"10.10.70.98","mask":32,"description":""}],"count":1}`)
	paginatedRaw := mustMarshal(t, map[string]interface{}{
		"totalRows":   1,
		"currentPage": 1,
		"currentSize": 100,
		"data":        []json.RawMessage{rawGroup},
	})

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    paginatedRaw,
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.GetIPGroup(context.Background(), "site-1", "abc123")
	if err != nil {
		t.Fatalf("GetIPGroup with groupId field: %v", err)
	}
	if got.ID != "abc123" {
		t.Errorf("ID = %q, want %q", got.ID, "abc123")
	}
}

// TestCreateIPGroup_StringIDResult verifies that CreateIPGroup handles the real
// v6 API response where "result" is a bare string ID, not a full IPGroup object.
// Pattern mirrors CreateMDNSRule: unmarshal as string → follow-up GetIPGroup.
// RED: current code tries json.Unmarshal(resp.Result, &IPGroup{}) → error.
func TestCreateIPGroup_StringIDResult(t *testing.T) {
	createdID := "6a1a9aa744a75c2be56115d0"
	// Full object returned by the follow-up GET (uses "groupId" as real API does).
	fullGroupRaw := json.RawMessage(`{"groupId":"6a1a9aa744a75c2be56115d0","name":"tf_pihole","type":0,"ipList":[{"ip":"10.10.70.98","mask":32,"description":""}],"count":1}`)
	paginatedRaw := mustMarshal(t, map[string]interface{}{
		"totalRows":   1,
		"currentPage": 1,
		"currentSize": 100,
		"data":        []json.RawMessage{fullGroupRaw},
	})

	callCount := 0
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/profiles/groups": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				// Real v6 API: returns bare string ID.
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    json.RawMessage(`"` + createdID + `"`),
				})
				return
			}
			if r.Method == http.MethodGet {
				callCount++
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    paginatedRaw,
				})
				return
			}
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &IPGroup{
		Name:   "tf_pihole",
		Type:   0,
		IPList: []IPGroupEntry{{IP: "10.10.70.98", Mask: 32, Description: ""}},
	}
	got, err := c.CreateIPGroup(context.Background(), "site-1", input)
	if err != nil {
		t.Fatalf("CreateIPGroup (string-id result): %v", err)
	}
	if got.ID != createdID {
		t.Errorf("ID = %q, want %q", got.ID, createdID)
	}
	if got.Name != "tf_pihole" {
		t.Errorf("Name = %q, want %q", got.Name, "tf_pihole")
	}
	if len(got.IPList) != 1 || got.IPList[0].IP != "10.10.70.98" {
		t.Errorf("IPList = %v, want [{10.10.70.98 32 }]", got.IPList)
	}
	if callCount == 0 {
		t.Error("expected at least one GET call to re-fetch after create; got 0")
	}
}

// =============================================================================
// mDNS Reflector Tests
// =============================================================================

// mdnsListResponse wraps mDNS rules in the custom MDNSListResult envelope.
func mdnsListResponse(t *testing.T, rules []MDNSRule) json.RawMessage {
	t.Helper()
	return mustMarshal(t, MDNSListResult{
		TotalRows:    len(rules),
		CurrentPage:  1,
		CurrentSize:  100,
		Data:         rules,
		APRuleNum:    0,
		OSGRuleNum:   len(rules),
		APRuleLimit:  16,
		OSGRuleLimit: 20,
	})
}

func TestListMDNSRules(t *testing.T) {
	rules := []MDNSRule{
		{ID: "mdns-1", Name: "AirPlay Reflector", Status: true, Type: 1,
			OSG: &MDNSNetworkSetting{
				ProfileIDs:      []string{"buildIn-1"},
				ServiceNetworks: []string{"net-1"},
				ClientNetworks:  []string{"net-2"},
			}},
		{ID: "mdns-2", Name: "Chromecast Reflector", Status: false, Type: 1,
			OSG: &MDNSNetworkSetting{
				ProfileIDs:      []string{"buildIn-2"},
				ServiceNetworks: []string{"net-3"},
				ClientNetworks:  []string{"net-1", "net-2"},
			}},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/service/mdns": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mdnsListResponse(t, rules),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListMDNSRules(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("ListMDNSRules: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rules, want 2", len(got))
	}
	if got[0].Name != "AirPlay Reflector" {
		t.Errorf("rules[0].Name = %q, want %q", got[0].Name, "AirPlay Reflector")
	}
	if got[0].OSG == nil {
		t.Fatal("rules[0].OSG is nil, expected non-nil")
	}
	if got[0].OSG.ProfileIDs[0] != "buildIn-1" {
		t.Errorf("rules[0].OSG.ProfileIDs[0] = %q, want %q", got[0].OSG.ProfileIDs[0], "buildIn-1")
	}
	if got[0].OSG.ServiceNetworks[0] != "net-1" {
		t.Errorf("rules[0].OSG.ServiceNetworks[0] = %q, want %q", got[0].OSG.ServiceNetworks[0], "net-1")
	}
	if got[0].OSG.ClientNetworks[0] != "net-2" {
		t.Errorf("rules[0].OSG.ClientNetworks[0] = %q, want %q", got[0].OSG.ClientNetworks[0], "net-2")
	}
}

func TestListMDNSRules_Empty(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/service/mdns": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mdnsListResponse(t, []MDNSRule{}),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.ListMDNSRules(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("ListMDNSRules (empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rules, want 0", len(got))
	}
}

func TestGetMDNSRule_Found(t *testing.T) {
	rules := []MDNSRule{
		{ID: "mdns-1", Name: "Rule A", Status: true, Type: 1},
		{ID: "mdns-2", Name: "Rule B", Status: false, Type: 1},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/service/mdns": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mdnsListResponse(t, rules),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	got, err := c.GetMDNSRule(context.Background(), "site-1", "mdns-2")
	if err != nil {
		t.Fatalf("GetMDNSRule: %v", err)
	}
	if got.Name != "Rule B" {
		t.Errorf("Name = %q, want %q", got.Name, "Rule B")
	}
}

func TestGetMDNSRule_NotFound(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/service/mdns": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mdnsListResponse(t, []MDNSRule{}),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.GetMDNSRule(context.Background(), "site-1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent mDNS rule, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, expected to contain 'not found'", err.Error())
	}
}

func TestCreateMDNSRule(t *testing.T) {
	createdRule := MDNSRule{
		ID: "mdns-new", Name: "New mDNS", Status: true, Type: 1,
		OSG: &MDNSNetworkSetting{
			ProfileIDs:      []string{"buildIn-1"},
			ServiceNetworks: []string{"net-1"},
			ClientNetworks:  []string{"net-2"},
		},
	}

	callCount := 0
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/service/mdns": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				// Create returns just the ID as a quoted string
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    json.RawMessage(`"mdns-new"`),
				})
				return
			}
			if r.Method == http.MethodGet {
				callCount++
				// List returns the full rule (for the re-fetch after create)
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mdnsListResponse(t, []MDNSRule{createdRule}),
				})
				return
			}
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &MDNSRule{
		Name: "New mDNS", Status: true, Type: 1,
		OSG: &MDNSNetworkSetting{
			ProfileIDs:      []string{"buildIn-1"},
			ServiceNetworks: []string{"net-1"},
			ClientNetworks:  []string{"net-2"},
		},
	}
	got, err := c.CreateMDNSRule(context.Background(), "site-1", input)
	if err != nil {
		t.Fatalf("CreateMDNSRule: %v", err)
	}
	if got.ID != "mdns-new" {
		t.Errorf("ID = %q, want %q", got.ID, "mdns-new")
	}
	if got.Name != "New mDNS" {
		t.Errorf("Name = %q, want %q", got.Name, "New mDNS")
	}
	if callCount == 0 {
		t.Error("expected at least one GET call to re-fetch after create")
	}
}

func TestUpdateMDNSRule(t *testing.T) {
	updatedRule := MDNSRule{
		ID: "mdns-1", Name: "Updated mDNS", Status: false, Type: 1,
		OSG: &MDNSNetworkSetting{
			ProfileIDs:      []string{"buildIn-1"},
			ServiceNetworks: []string{"net-1"},
			ClientNetworks:  []string{"net-2"},
		},
	}

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/service/mdns/mdns-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				t.Errorf("expected PUT, got %s", r.Method)
			}
			// PUT returns empty success
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    json.RawMessage(`{}`),
			})
		},
		"/sites/site-1/setting/service/mdns": func(w http.ResponseWriter, r *http.Request) {
			// List for re-fetch
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    mdnsListResponse(t, []MDNSRule{updatedRule}),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	input := &MDNSRule{
		Name: "Updated mDNS", Status: false, Type: 1,
		OSG: &MDNSNetworkSetting{
			ProfileIDs:      []string{"buildIn-1"},
			ServiceNetworks: []string{"net-1"},
			ClientNetworks:  []string{"net-2"},
		},
	}
	got, err := c.UpdateMDNSRule(context.Background(), "site-1", "mdns-1", input)
	if err != nil {
		t.Fatalf("UpdateMDNSRule: %v", err)
	}
	if got.Name != "Updated mDNS" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated mDNS")
	}
	if got.Status != false {
		t.Errorf("Status = %v, want false", got.Status)
	}
}

func TestDeleteMDNSRule(t *testing.T) {
	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		"/sites/site-1/setting/service/mdns/mdns-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("expected DELETE, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result:    json.RawMessage(`"Deleted Rule"`),
			})
		},
	})
	defer server.Close()
	c := newTestClient(t, server)

	err := c.DeleteMDNSRule(context.Background(), "site-1", "mdns-1")
	if err != nil {
		t.Fatalf("DeleteMDNSRule: %v", err)
	}
}

// =============================================================================
// UpdateSwitchPortV2 Tests
// =============================================================================

// mockOpenAPIServer creates a test server that handles both the standard
// Omada auth paths AND a custom openapi path. The openapi path cannot be
// registered via mockOmadaServer (which only adds /api/v2 prefixed handlers),
// so we build a raw mux here.
func mockOpenAPIServer(t *testing.T, openapiHandlers map[string]http.HandlerFunc, v2Handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	omadacID := "test-omadac-id"
	token := "test-csrf-token"

	mux := http.NewServeMux()

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: 0,
			Msg:       "Success.",
			Result: mustMarshal(t, ControllerInfo{
				OmadacID:      omadacID,
				ControllerVer: "6.1.0.19",
				APIVer:        "3",
			}),
		})
	})

	mux.HandleFunc(fmt.Sprintf("/%s/api/v2/login", omadacID), func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			ErrorCode: 0,
			Msg:       "Success.",
			Result:    mustMarshal(t, LoginResult{Token: token}),
		})
	})

	// Register openapi (non-v2) handlers directly on the mux.
	for pattern, handler := range openapiHandlers {
		mux.HandleFunc(pattern, handler)
	}

	// Register api/v2 handlers with prefix.
	for pattern, handler := range v2Handlers {
		prefix := fmt.Sprintf("/%s/api/v2", omadacID)
		mux.HandleFunc(prefix+pattern, handler)
	}

	return httptest.NewServer(mux)
}

// TestUpdateSwitchPortV2_OpenAPIPathAndBody verifies that UpdateSwitchPortV2:
//   - sends PATCH to the openapi/v1 URL (NOT api/v2)
//   - includes Csrf-Token header
//   - coerces nil TagIDs to []
//   - forces ProfileVlanOverrideEnable=true when ProfileOverrideEnable+NativeNetworkID
//   - does NOT include /api/v2 in the write URL
func TestUpdateSwitchPortV2_OpenAPIPathAndBody(t *testing.T) {
	omadacID := "test-omadac-id"
	siteID := "site-1"
	mac := "aa:bb:cc:dd:ee:ff"
	portNum := 3

	var capturedMethod string
	var capturedURL string
	var capturedCsrfToken string
	var capturedBody SwitchPortV2

	// openapi PATCH handler — path as built by UpdateSwitchPortV2:
	// {baseURL}/openapi/v1/{omadacID}/sites/{siteID}/switches/{mac}/ports/{port}
	// On the test server the path portion is everything after the host.
	openAPIPath := fmt.Sprintf("/openapi/v1/%s/sites/%s/switches/%s/ports/%d",
		omadacID, siteID, mac, portNum)

	// Build the switch config for the GetSwitchPort re-read
	switchCfg := SwitchConfig{
		MAC:  mac,
		Name: "test-switch",
		Ports: []SwitchPort{
			{
				Port:                      portNum,
				Name:                      "port-3",
				ProfileOverrideEnable:     true,
				ProfileVlanOverrideEnable: true,
				NativeNetworkID:           "net-trusted",
				ProfileID:                 "profile-access",
			},
		},
	}

	server := mockOpenAPIServer(t,
		map[string]http.HandlerFunc{
			openAPIPath: func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedURL = r.URL.String()
				capturedCsrfToken = r.Header.Get("Csrf-Token")

				if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
					t.Errorf("decoding body: %v", err)
				}
				json.NewEncoder(w).Encode(APIResponse{ErrorCode: 0, Msg: "Success."})
			},
		},
		map[string]http.HandlerFunc{
			fmt.Sprintf("/sites/%s/switches/%s", siteID, mac): func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, switchCfg),
				})
			},
		},
	)
	defer server.Close()
	c := newTestClient(t, server)

	body := &SwitchPortV2{
		Name:                  "port-3",
		ProfileID:             "profile-access",
		ProfileOverrideEnable: true,
		NativeNetworkID:       "net-trusted",
		// TagIDs intentionally nil — should be coerced to []
		TagIDs: nil,
	}

	got, err := c.UpdateSwitchPortV2(context.Background(), siteID, mac, portNum, body)
	if err != nil {
		t.Fatalf("UpdateSwitchPortV2: %v", err)
	}

	// Assert URL contains openapi/v1, not api/v2.
	if !strings.Contains(capturedURL, "/openapi/v1/") {
		t.Errorf("URL = %q, want it to contain /openapi/v1/", capturedURL)
	}
	if strings.Contains(capturedURL, "/api/v2") {
		t.Errorf("write URL = %q, must NOT contain /api/v2", capturedURL)
	}

	// Assert method is PATCH.
	if capturedMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", capturedMethod)
	}

	// Assert Csrf-Token header was sent.
	if capturedCsrfToken == "" {
		t.Error("Csrf-Token header was empty; doOpenAPIRequest should set it")
	}

	// Assert nil TagIDs coerced to empty slice (marshals as [] not null).
	if capturedBody.TagIDs == nil {
		t.Error("TagIDs should be coerced to [] before sending, got nil")
	}
	if len(capturedBody.TagIDs) != 0 {
		t.Errorf("TagIDs = %v, want []", capturedBody.TagIDs)
	}

	// Assert ProfileVlanOverrideEnable forced true (override=true + nativeNetworkId set).
	if !capturedBody.ProfileVlanOverrideEnable {
		t.Error("ProfileVlanOverrideEnable should be forced true when ProfileOverrideEnable=true + NativeNetworkID set")
	}

	// Assert re-read returns a populated SwitchPort.
	if got == nil {
		t.Fatal("got nil *SwitchPort from UpdateSwitchPortV2")
	}
	if got.Port != portNum {
		t.Errorf("re-read port = %d, want %d", got.Port, portNum)
	}
}

// TestUpdateSwitchPortV2_ErrorSurfacing verifies that a non-transient controller
// error (e.g. -39840: VLAN profile conflict) is returned as an error whose
// message contains both the numeric code and the controller's description.
func TestUpdateSwitchPortV2_ErrorSurfacing(t *testing.T) {
	omadacID := "test-omadac-id"
	siteID := "site-1"
	mac := "aa:bb:cc:dd:ee:ff"
	portNum := 3

	openAPIPath := fmt.Sprintf("/openapi/v1/%s/sites/%s/switches/%s/ports/%d",
		omadacID, siteID, mac, portNum)

	server := mockOpenAPIServer(t,
		map[string]http.HandlerFunc{
			openAPIPath: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: -39840,
					Msg:       "When the VLAN configuration in the profile bound to the port is disabled, the VLAN configuration of the port cannot follow the profile.",
				})
			},
		},
		nil,
	)
	defer server.Close()
	c := newTestClient(t, server)

	body := &SwitchPortV2{Name: "port-3", TagIDs: []string{}}
	_, err := c.UpdateSwitchPortV2(context.Background(), siteID, mac, portNum, body)
	if err == nil {
		t.Fatal("expected error from UpdateSwitchPortV2 when controller returns -39840, got nil")
	}
	if !strings.Contains(err.Error(), "-39840") {
		t.Errorf("error = %q, want it to contain -39840", err.Error())
	}
	if !strings.Contains(err.Error(), "VLAN") {
		t.Errorf("error = %q, want it to contain the controller message (VLAN)", err.Error())
	}
}

// TestUpdateSwitchPortV2_TransientRetry verifies the method retries on
// errorCode -1 and eventually returns success.
func TestUpdateSwitchPortV2_TransientRetry(t *testing.T) {
	omadacID := "test-omadac-id"
	siteID := "site-1"
	mac := "aa:bb:cc:dd:ee:ff"
	portNum := 3
	attempts := 0

	openAPIPath := fmt.Sprintf("/openapi/v1/%s/sites/%s/switches/%s/ports/%d",
		omadacID, siteID, mac, portNum)

	switchCfg := SwitchConfig{
		MAC:  mac,
		Name: "test-switch",
		Ports: []SwitchPort{
			{Port: portNum, Name: "port-3"},
		},
	}

	server := mockOpenAPIServer(t,
		map[string]http.HandlerFunc{
			openAPIPath: func(w http.ResponseWriter, r *http.Request) {
				attempts++
				if attempts < 3 {
					// Return transient -1 error.
					json.NewEncoder(w).Encode(APIResponse{ErrorCode: -1, Msg: "transient"})
					return
				}
				json.NewEncoder(w).Encode(APIResponse{ErrorCode: 0, Msg: "Success."})
			},
		},
		map[string]http.HandlerFunc{
			fmt.Sprintf("/sites/%s/switches/%s", siteID, mac): func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, switchCfg),
				})
			},
		},
	)
	defer server.Close()
	c := newTestClient(t, server)

	body := &SwitchPortV2{Name: "port-3", TagIDs: []string{}}
	_, err := c.UpdateSwitchPortV2(context.Background(), siteID, mac, portNum, body)
	if err != nil {
		t.Fatalf("UpdateSwitchPortV2 (retry): %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (2 transients then success)", attempts)
	}
}

// TestPortProfile_VlanFields verifies that PortProfile correctly decodes the
// vlanConfigEnable and networkTagsSetting fields from the controller JSON.
// These fields are required for the VLAN derivation path in UpdateSwitchPortV2.
func TestPortProfile_VlanFields(t *testing.T) {
	raw := `{
		"id": "prof-1",
		"name": "access_iot",
		"vlanConfigEnable": false,
		"networkTagsSetting": 2,
		"nativeNetworkId": "net-iot",
		"tagNetworkIds": [],
		"poe": 0,
		"dot1x": 0,
		"portIsolationEnable": false,
		"lldpMedEnable": false,
		"topoNotifyEnable": false,
		"spanningTreeEnable": false,
		"loopbackDetectEnable": false,
		"bandWidthCtrlType": 0,
		"eeeEnable": false,
		"flowControlEnable": false,
		"fastLeaveEnable": false,
		"loopbackDetectVlanBasedEnable": false,
		"igmpFastLeaveEnable": false,
		"mldFastLeaveEnable": false,
		"dot1pPriority": 0,
		"trustMode": 0
	}`

	var p PortProfile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if p.VlanConfigEnable != false {
		t.Errorf("VlanConfigEnable = %v, want false", p.VlanConfigEnable)
	}
	if p.NetworkTagsSetting != 2 {
		t.Errorf("NetworkTagsSetting = %d, want 2", p.NetworkTagsSetting)
	}
	if p.NativeNetworkID != "net-iot" {
		t.Errorf("NativeNetworkID = %q, want %q", p.NativeNetworkID, "net-iot")
	}
}

// TestPortProfile_VlanFields_Enabled triangulates with vlanConfigEnable=true so
// the decoder is exercised for both values (forces real field mapping).
func TestPortProfile_VlanFields_Enabled(t *testing.T) {
	raw := `{
		"id": "prof-2",
		"name": "trunk_uplink",
		"vlanConfigEnable": true,
		"networkTagsSetting": 1,
		"nativeNetworkId": "net-mgmt",
		"tagNetworkIds": ["net-iot", "net-trusted"],
		"poe": 0,
		"dot1x": 0,
		"portIsolationEnable": false,
		"lldpMedEnable": false,
		"topoNotifyEnable": false,
		"spanningTreeEnable": false,
		"loopbackDetectEnable": false,
		"bandWidthCtrlType": 0,
		"eeeEnable": false,
		"flowControlEnable": false,
		"fastLeaveEnable": false,
		"loopbackDetectVlanBasedEnable": false,
		"igmpFastLeaveEnable": false,
		"mldFastLeaveEnable": false,
		"dot1pPriority": 0,
		"trustMode": 0
	}`

	var p PortProfile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if p.VlanConfigEnable != true {
		t.Errorf("VlanConfigEnable = %v, want true", p.VlanConfigEnable)
	}
	if p.NetworkTagsSetting != 1 {
		t.Errorf("NetworkTagsSetting = %d, want 1", p.NetworkTagsSetting)
	}
}

// TestUpdateSwitchPortV2_VlanDerivation_VlanConfigDisabled verifies that when a
// profile has vlanConfigEnable=false and the caller sends no override/native,
// UpdateSwitchPortV2 automatically derives VLAN settings from the profile and
// sends profileVlanOverrideEnable=true + the profile's nativeNetworkId and
// networkTagsSetting in the PATCH body.
func TestUpdateSwitchPortV2_VlanDerivation_VlanConfigDisabled(t *testing.T) {
	omadacID := "test-omadac-id"
	siteID := "site-1"
	mac := "aa:bb:cc:dd:ee:ff"
	portNum := 5
	profileID := "prof-iot"

	openAPIPath := fmt.Sprintf("/openapi/v1/%s/sites/%s/switches/%s/ports/%d",
		omadacID, siteID, mac, portNum)

	// The profile returned by api/v2 GET /setting/lan/profiles.
	// vlanConfigEnable=false triggers the derivation path.
	iotProfile := PortProfile{
		ID:                 profileID,
		Name:               "access_iot",
		VlanConfigEnable:   false,
		NetworkTagsSetting: 2,
		NativeNetworkID:    "net-iot",
		TagNetworkIDs:      []string{},
	}
	profilesPage := PaginatedResult{
		TotalRows:   1,
		CurrentPage: 1,
		CurrentSize: 1,
	}
	profilesPageData, _ := json.Marshal([]PortProfile{iotProfile})
	profilesPage.Data = profilesPageData

	switchCfg := SwitchConfig{
		MAC:  mac,
		Name: "test-switch",
		Ports: []SwitchPort{
			{
				Port:                      portNum,
				Name:                      "port-5",
				ProfileID:                 profileID,
				ProfileVlanOverrideEnable: true,
				NativeNetworkID:           "net-iot",
				NetworkTagsSetting:        2,
			},
		},
	}

	var capturedBody SwitchPortV2

	server := mockOpenAPIServer(t,
		map[string]http.HandlerFunc{
			openAPIPath: func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
					t.Errorf("decoding PATCH body: %v", err)
				}
				json.NewEncoder(w).Encode(APIResponse{ErrorCode: 0, Msg: "Success."})
			},
		},
		map[string]http.HandlerFunc{
			fmt.Sprintf("/sites/%s/setting/lan/profiles", siteID): func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, profilesPage),
				})
			},
			fmt.Sprintf("/sites/%s/switches/%s", siteID, mac): func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, switchCfg),
				})
			},
		},
	)
	defer server.Close()
	c := newTestClient(t, server)

	// override=false, no NativeNetworkID — triggers derivation.
	body := &SwitchPortV2{
		Name:      "port-5",
		ProfileID: profileID,
		// ProfileOverrideEnable intentionally false (zero value)
		// NativeNetworkID intentionally empty (zero value)
	}

	got, err := c.UpdateSwitchPortV2(context.Background(), siteID, mac, portNum, body)
	if err != nil {
		t.Fatalf("UpdateSwitchPortV2: %v", err)
	}

	// Derivation must have set these three fields.
	if !capturedBody.ProfileVlanOverrideEnable {
		t.Error("ProfileVlanOverrideEnable should be true after VLAN derivation (vlanConfigEnable=false profile)")
	}
	if capturedBody.NativeNetworkID != "net-iot" {
		t.Errorf("NativeNetworkID = %q, want %q", capturedBody.NativeNetworkID, "net-iot")
	}
	if capturedBody.NetworkTagsSetting != 2 {
		t.Errorf("NetworkTagsSetting = %d, want 2", capturedBody.NetworkTagsSetting)
	}

	// Re-read should still succeed.
	if got == nil {
		t.Fatal("got nil *SwitchPort")
	}
}

// TestUpdateSwitchPortV2_VlanDerivation_VlanConfigEnabled verifies that when
// vlanConfigEnable=true the derivation path is NOT taken — profileVlanOverrideEnable
// stays false and NativeNetworkID stays empty in the PATCH body.
func TestUpdateSwitchPortV2_VlanDerivation_VlanConfigEnabled(t *testing.T) {
	omadacID := "test-omadac-id"
	siteID := "site-1"
	mac := "aa:bb:cc:dd:ee:ff"
	portNum := 6
	profileID := "prof-trunk"

	openAPIPath := fmt.Sprintf("/openapi/v1/%s/sites/%s/switches/%s/ports/%d",
		omadacID, siteID, mac, portNum)

	trunkProfile := PortProfile{
		ID:                 profileID,
		Name:               "trunk_uplink",
		VlanConfigEnable:   true,
		NetworkTagsSetting: 1,
		NativeNetworkID:    "net-mgmt",
		TagNetworkIDs:      []string{"net-iot"},
	}
	profilesPageData, _ := json.Marshal([]PortProfile{trunkProfile})
	profilesPage := PaginatedResult{
		TotalRows: 1, CurrentPage: 1, CurrentSize: 1,
		Data: profilesPageData,
	}

	switchCfg := SwitchConfig{
		MAC:  mac,
		Name: "test-switch",
		Ports: []SwitchPort{
			{Port: portNum, Name: "port-6", ProfileID: profileID},
		},
	}

	var capturedBody SwitchPortV2

	server := mockOpenAPIServer(t,
		map[string]http.HandlerFunc{
			openAPIPath: func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
					t.Errorf("decoding PATCH body: %v", err)
				}
				json.NewEncoder(w).Encode(APIResponse{ErrorCode: 0, Msg: "Success."})
			},
		},
		map[string]http.HandlerFunc{
			fmt.Sprintf("/sites/%s/setting/lan/profiles", siteID): func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, profilesPage),
				})
			},
			fmt.Sprintf("/sites/%s/switches/%s", siteID, mac): func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, switchCfg),
				})
			},
		},
	)
	defer server.Close()
	c := newTestClient(t, server)

	body := &SwitchPortV2{
		Name:      "port-6",
		ProfileID: profileID,
		// override off, no native — but profile has vlanConfigEnable=true so NO derivation
	}

	_, err := c.UpdateSwitchPortV2(context.Background(), siteID, mac, portNum, body)
	if err != nil {
		t.Fatalf("UpdateSwitchPortV2: %v", err)
	}

	// Derivation must NOT have fired.
	if capturedBody.ProfileVlanOverrideEnable {
		t.Error("ProfileVlanOverrideEnable should be false — no derivation for vlanConfigEnable=true profile")
	}
	if capturedBody.NativeNetworkID != "" {
		t.Errorf("NativeNetworkID = %q, want empty — derivation must not copy native when vlanConfigEnable=true", capturedBody.NativeNetworkID)
	}
}

// TestUpdateSwitchPortV2_VlanDerivation_ExplicitNativePreserved verifies that
// when the caller supplies an explicit NativeNetworkID the derivation is skipped
// and the user-supplied value is preserved in the PATCH body.
func TestUpdateSwitchPortV2_VlanDerivation_ExplicitNativePreserved(t *testing.T) {
	omadacID := "test-omadac-id"
	siteID := "site-1"
	mac := "aa:bb:cc:dd:ee:ff"
	portNum := 7
	profileID := "prof-iot"

	openAPIPath := fmt.Sprintf("/openapi/v1/%s/sites/%s/switches/%s/ports/%d",
		omadacID, siteID, mac, portNum)

	iotProfile := PortProfile{
		ID:                 profileID,
		Name:               "access_iot",
		VlanConfigEnable:   false,
		NetworkTagsSetting: 2,
		NativeNetworkID:    "net-iot",
		TagNetworkIDs:      []string{},
	}
	profilesPageData, _ := json.Marshal([]PortProfile{iotProfile})
	profilesPage := PaginatedResult{
		TotalRows: 1, CurrentPage: 1, CurrentSize: 1,
		Data: profilesPageData,
	}

	switchCfg := SwitchConfig{
		MAC:  mac,
		Name: "test-switch",
		Ports: []SwitchPort{
			{Port: portNum, Name: "port-7", ProfileID: profileID, NativeNetworkID: "net-override"},
		},
	}

	var capturedBody SwitchPortV2

	server := mockOpenAPIServer(t,
		map[string]http.HandlerFunc{
			openAPIPath: func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
					t.Errorf("decoding PATCH body: %v", err)
				}
				json.NewEncoder(w).Encode(APIResponse{ErrorCode: 0, Msg: "Success."})
			},
		},
		map[string]http.HandlerFunc{
			fmt.Sprintf("/sites/%s/setting/lan/profiles", siteID): func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, profilesPage),
				})
			},
			fmt.Sprintf("/sites/%s/switches/%s", siteID, mac): func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(APIResponse{
					ErrorCode: 0,
					Result:    mustMarshal(t, switchCfg),
				})
			},
		},
	)
	defer server.Close()
	c := newTestClient(t, server)

	// User supplies explicit NativeNetworkID — derivation must be skipped.
	body := &SwitchPortV2{
		Name:            "port-7",
		ProfileID:       profileID,
		NativeNetworkID: "net-override",
	}

	_, err := c.UpdateSwitchPortV2(context.Background(), siteID, mac, portNum, body)
	if err != nil {
		t.Fatalf("UpdateSwitchPortV2: %v", err)
	}

	// User value must be preserved, derivation skipped.
	if capturedBody.NativeNetworkID != "net-override" {
		t.Errorf("NativeNetworkID = %q, want %q (user value must be preserved)", capturedBody.NativeNetworkID, "net-override")
	}
	if capturedBody.ProfileVlanOverrideEnable {
		t.Error("ProfileVlanOverrideEnable should be false when user supplied explicit native (no derivation)")
	}
}

// =============================================================================
// Firewall ACL Tests
// =============================================================================

// TestACLRule_FullBodyMarshal verifies that ACLRule marshals the full
// controller body shape: direction arrays always serialize as [] (never
// omitted), and custom-ACL slices are present.
func TestACLRule_FullBodyMarshal(t *testing.T) {
	rule := ACLRule{
		Name:            "test-rule",
		Status:          true,
		Policy:          1,
		Type:            0,
		Protocols:       []int{256},
		SourceType:      0,
		SourceIDs:       []string{},
		DestinationType: 0,
		DestinationIDs:  []string{},
		Direction: ACLDirection{
			LanToWan: false,
			LanToLan: true,
			WanInIDs: []string{},
			VpnInIDs: []string{},
		},
		CustomAclOsws:    []string{},
		CustomAclStacks:  []string{},
		CustomAclDevices: []string{},
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	got := string(data)

	checks := []string{
		`"protocols":[256]`,
		`"direction":{"wanInIds":[],"vpnInIds":[],"lanToWan":false,"lanToLan":true}`,
		`"customAclOsws":[]`,
		`"customAclStacks":[]`,
		`"customAclDevices":[]`,
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("JSON missing %q\ngot: %s", want, got)
		}
	}
}

// TestUpdateACLRule_UsesPUT verifies that UpdateACLRule sends PUT (not PATCH).
func TestUpdateACLRule_UsesPUT(t *testing.T) {
	const siteID = "site-1"
	const ruleID = "rule-abc"
	capturedMethod := ""

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		fmt.Sprintf("/sites/%s/setting/firewall/acls/%s", siteID, ruleID): func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			if r.Method != http.MethodPut {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(APIResponse{
				ErrorCode: 0,
				Result: mustMarshal(t, ACLRule{
					ID:     ruleID,
					Name:   "test-rule",
					Status: true,
					Policy: 1,
					Type:   0,
				}),
			})
		},
	})
	defer server.Close()

	c := newTestClient(t, server)
	rule := &ACLRule{
		Name:             "test-rule",
		Status:           true,
		Policy:           1,
		Type:             0,
		Protocols:        []int{256},
		SourceIDs:        []string{},
		DestinationIDs:   []string{},
		CustomAclOsws:    []string{},
		CustomAclStacks:  []string{},
		CustomAclDevices: []string{},
		Direction: ACLDirection{
			WanInIDs: []string{},
			VpnInIDs: []string{},
		},
	}

	_, err := c.UpdateACLRule(context.Background(), siteID, ruleID, rule)
	if err != nil {
		t.Fatalf("UpdateACLRule: %v", err)
	}
	if capturedMethod != http.MethodPut {
		t.Errorf("UpdateACLRule used method %q, want PUT", capturedMethod)
	}
}

// TestModifyACLIndex verifies the ModifyACLIndex client method sends the
// correct POST to /cmd/acls/modifyIndex with the expected body shape.
func TestModifyACLIndex(t *testing.T) {
	const siteID = "site-1"

	type modifyIndexBody struct {
		Indexes map[string]int `json:"indexes"`
		Type    int            `json:"type"`
	}

	var capturedBody modifyIndexBody
	capturedPath := ""

	server := mockOmadaServer(t, map[string]http.HandlerFunc{
		fmt.Sprintf("/sites/%s/cmd/acls/modifyIndex", siteID): func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(APIResponse{ErrorCode: 0})
		},
	})
	defer server.Close()

	c := newTestClient(t, server)
	indexes := map[string]int{"id-a": 1, "id-b": 2}
	if err := c.ModifyACLIndex(context.Background(), siteID, 0, indexes); err != nil {
		t.Fatalf("ModifyACLIndex: %v", err)
	}

	omadacID := "test-omadac-id"
	wantPath := fmt.Sprintf("/%s/api/v2/sites/%s/cmd/acls/modifyIndex", omadacID, siteID)
	if capturedPath != wantPath {
		t.Errorf("path = %q, want %q", capturedPath, wantPath)
	}
	if capturedBody.Type != 0 {
		t.Errorf("type = %d, want 0", capturedBody.Type)
	}
	if capturedBody.Indexes["id-a"] != 1 {
		t.Errorf("indexes[id-a] = %d, want 1", capturedBody.Indexes["id-a"])
	}
	if capturedBody.Indexes["id-b"] != 2 {
		t.Errorf("indexes[id-b] = %d, want 2", capturedBody.Indexes["id-b"])
	}
}
