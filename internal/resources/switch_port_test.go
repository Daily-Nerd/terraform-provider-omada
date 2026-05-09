package resources

import (
	"context"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestSwitchPort_BuildPatchPayload_AllFields verifies every settable model
// field flows into the PATCH map sent to /switches/{mac}/ports/{port}.
func TestSwitchPort_BuildPatchPayload_AllFields(t *testing.T) {
	ctx := context.Background()
	tagIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"net-1", "net-2"})
	untagIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"net-3"})

	model := &SwitchPortResourceModel{
		Port:                  types.Int64Value(5),
		Name:                  types.StringValue("k8s-node-1"),
		Disable:               types.BoolValue(false),
		ProfileID:             types.StringValue("profile-trusted"),
		ProfileOverrideEnable: types.BoolValue(true),
		NativeNetworkID:       types.StringValue("net-trusted"),
		NetworkTagsSetting:    types.Int64Value(1),
		TagNetworkIDs:         tagIDs,
		UntagNetworkIDs:       untagIDs,
		VoiceNetworkEnable:    types.BoolValue(true),
		VoiceDscpEnable:       types.BoolValue(true),
		Speed:                 types.Int64Value(5),
	}

	var buildErrs []error
	got := buildSwitchPortPatchPayload(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("build errors: %v", buildErrs)
	}
	if got == nil {
		t.Fatal("payload nil")
	}

	checks := []struct {
		key  string
		want any
	}{
		{"port", int64(5)},
		{"name", "k8s-node-1"},
		{"disable", false},
		{"profileId", "profile-trusted"},
		{"profileOverrideEnable", true},
		{"nativeNetworkId", "net-trusted"},
		{"networkTagsSetting", int64(1)},
		{"voiceNetworkEnable", true},
		{"voiceDscpEnable", true},
		{"speed", int64(5)},
	}
	for _, c := range checks {
		if got[c.key] != c.want {
			t.Errorf("payload[%q] = %v, want %v", c.key, got[c.key], c.want)
		}
	}

	tagSlice, _ := got["tagNetworkIds"].([]string)
	if len(tagSlice) != 2 || tagSlice[0] != "net-1" || tagSlice[1] != "net-2" {
		t.Errorf("tagNetworkIds = %v", tagSlice)
	}
	untagSlice, _ := got["untagNetworkIds"].([]string)
	if len(untagSlice) != 1 || untagSlice[0] != "net-3" {
		t.Errorf("untagNetworkIds = %v", untagSlice)
	}
}

// TestSwitchPort_BuildPatchPayload_OmitsEmptyOptional verifies that empty
// optional string fields (profile_id, native_network_id, name) are omitted
// from the PATCH payload rather than sent as empty strings (which the
// controller may interpret differently from "leave alone").
func TestSwitchPort_BuildPatchPayload_OmitsEmptyOptional(t *testing.T) {
	ctx := context.Background()
	model := &SwitchPortResourceModel{
		Port:                  types.Int64Value(7),
		Disable:               types.BoolValue(false),
		ProfileOverrideEnable: types.BoolValue(false),
		NetworkTagsSetting:    types.Int64Value(0),
		Speed:                 types.Int64Value(0),
	}

	var buildErrs []error
	got := buildSwitchPortPatchPayload(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("build errors: %v", buildErrs)
	}

	for _, key := range []string{"name", "profileId", "nativeNetworkId", "tagNetworkIds", "untagNetworkIds"} {
		if _, ok := got[key]; ok {
			t.Errorf("payload should NOT contain %q when not set; got %v", key, got[key])
		}
	}
	// Required scalars should always be present.
	for _, key := range []string{"port", "disable", "profileOverrideEnable", "voiceNetworkEnable", "voiceDscpEnable", "networkTagsSetting", "speed"} {
		if _, ok := got[key]; !ok {
			t.Errorf("payload should contain %q always; missing", key)
		}
	}
}

// TestSwitchPort_ApplyToModel verifies the API SwitchPort struct flows back
// into the Terraform model on Read / ImportState.
func TestSwitchPort_ApplyToModel(t *testing.T) {
	ctx := context.Background()
	port := &client.SwitchPort{
		Port:                  5,
		Name:                  "k8s-node-1",
		Disable:               false,
		ProfileID:             "profile-trusted",
		ProfileOverrideEnable: true,
		NativeNetworkID:       "net-trusted",
		NetworkTagsSetting:    1,
		TagNetworkIDs:         []string{"net-1", "net-2"},
		UntagNetworkIDs:       []string{"net-3"},
		VoiceNetworkEnable:    true,
		VoiceDscpEnable:       true,
		Speed:                 5,
	}

	model := &SwitchPortResourceModel{
		// Force lists into known-list state so apply writes API values
		// rather than null-preserving.
		TagNetworkIDs:   types.ListValueMust(types.StringType, nil),
		UntagNetworkIDs: types.ListValueMust(types.StringType, nil),
	}
	if err := applySwitchPortToModel(ctx, model, port); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if model.Port.ValueInt64() != 5 {
		t.Errorf("Port = %d, want 5", model.Port.ValueInt64())
	}
	if model.Name.ValueString() != "k8s-node-1" {
		t.Errorf("Name = %q", model.Name.ValueString())
	}
	if !model.ProfileOverrideEnable.ValueBool() {
		t.Error("ProfileOverrideEnable should be true")
	}
	if model.NetworkTagsSetting.ValueInt64() != 1 {
		t.Errorf("NetworkTagsSetting = %d, want 1", model.NetworkTagsSetting.ValueInt64())
	}
	if model.Speed.ValueInt64() != 5 {
		t.Errorf("Speed = %d, want 5", model.Speed.ValueInt64())
	}
	if !model.VoiceNetworkEnable.ValueBool() || !model.VoiceDscpEnable.ValueBool() {
		t.Error("Voice toggles should be true")
	}

	var tags []string
	model.TagNetworkIDs.ElementsAs(ctx, &tags, false)
	if len(tags) != 2 || tags[0] != "net-1" {
		t.Errorf("TagNetworkIDs = %v", tags)
	}
}

// TestSwitchPort_ApplyToModel_NullListsPreserved verifies the null-vs-empty
// list preservation: state-null + API empty should remain null to avoid
// perpetual diff.
func TestSwitchPort_ApplyToModel_NullListsPreserved(t *testing.T) {
	ctx := context.Background()
	port := &client.SwitchPort{
		Port:            5,
		TagNetworkIDs:   []string{},
		UntagNetworkIDs: []string{},
	}
	model := &SwitchPortResourceModel{
		TagNetworkIDs:   types.ListNull(types.StringType),
		UntagNetworkIDs: types.ListNull(types.StringType),
	}
	if err := applySwitchPortToModel(ctx, model, port); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !model.TagNetworkIDs.IsNull() {
		t.Error("TagNetworkIDs should remain null")
	}
	if !model.UntagNetworkIDs.IsNull() {
		t.Error("UntagNetworkIDs should remain null")
	}
}
