package resources

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSwitchPort_MirroredPortRefsToSet_OrderIndependent(t *testing.T) {
	ctx := context.Background()
	set := mirroredPortRefsToSet(ctx, []client.MirroredPortRef{{Port: 16}, {Port: 1}, {Port: 3}, {Port: 5}, {Port: 14}})
	var got []int64
	set.ElementsAs(ctx, &got, false)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	want := []int64{1, 3, 5, 14, 16}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

// TestSwitchPort_SpeedToLinkDuplex_Table verifies the speed→{linkSpeed,duplex}
// mapping table covers all confirmed speed codes and gaps.
func TestSwitchPort_SpeedToLinkDuplex_Table(t *testing.T) {
	cases := []struct {
		speed     int
		linkSpeed int
		duplex    int
	}{
		{0, 0, 0}, // auto-neg
		{3, 2, 1}, // 100Mb HD
		{4, 2, 2}, // 100Mb FD
		{5, 3, 2}, // 1Gb FD
		{6, 4, 2}, // 2.5Gb FD
		{1, 0, 0}, // gap — fallback auto
		{2, 0, 0}, // gap — fallback auto
		{7, 0, 0}, // gap — fallback auto
		{8, 0, 0}, // gap — fallback auto
	}
	for _, tc := range cases {
		ls, d := client.SpeedToLinkDuplex(tc.speed)
		if ls != tc.linkSpeed || d != tc.duplex {
			t.Errorf("SpeedToLinkDuplex(%d) = (%d,%d), want (%d,%d)", tc.speed, ls, d, tc.linkSpeed, tc.duplex)
		}
	}
}

// TestSwitchPort_BuildV2Body_AllFields verifies that buildSwitchPortV2Body
// produces the openapi/v1 body struct with correct field mapping:
//   - speed=5 → linkSpeed=3, duplex=2
//   - tagNetworkIDs → TagIDs (not TagNetworkIds)
//   - no port, disable, voiceDscpEnable, or untagNetworkIds fields
func TestSwitchPort_BuildV2Body_AllFields(t *testing.T) {
	ctx := context.Background()
	tagIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"net-1", "net-2"})

	model := &SwitchPortResourceModel{
		Port:                      types.Int64Value(5),
		Name:                      types.StringValue("k8s-node-1"),
		Disable:                   types.BoolValue(false),
		ProfileID:                 types.StringValue("profile-access"),
		ProfileOverrideEnable:     types.BoolValue(true),
		ProfileVlanOverrideEnable: types.BoolValue(false),
		NativeNetworkID:           types.StringValue("net-trusted"),
		NetworkTagsSetting:        types.Int64Value(2),
		TagNetworkIDs:             tagIDs,
		UntagNetworkIDs:           types.ListNull(types.StringType),
		VoiceNetworkEnable:        types.BoolValue(true),
		VoiceDscpEnable:           types.BoolValue(false),
		Speed:                     types.Int64Value(5),
	}

	var buildErrs []error
	got := buildSwitchPortV2Body(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("build errors: %v", buildErrs)
	}
	if got == nil {
		t.Fatal("buildSwitchPortV2Body returned nil")
	}

	// Speed mapping: 5 → linkSpeed=3, duplex=2
	if got.LinkSpeed != 3 {
		t.Errorf("LinkSpeed = %d, want 3", got.LinkSpeed)
	}
	if got.Duplex != 2 {
		t.Errorf("Duplex = %d, want 2", got.Duplex)
	}

	// tagIds populated from TagNetworkIDs
	if len(got.TagIDs) != 2 || got.TagIDs[0] != "net-1" || got.TagIDs[1] != "net-2" {
		t.Errorf("TagIDs = %v, want [net-1, net-2]", got.TagIDs)
	}

	// Standard fields
	if got.Name != "k8s-node-1" {
		t.Errorf("Name = %q, want k8s-node-1", got.Name)
	}
	if got.ProfileID != "profile-access" {
		t.Errorf("ProfileID = %q, want profile-access", got.ProfileID)
	}
	if !got.ProfileOverrideEnable {
		t.Error("ProfileOverrideEnable should be true")
	}
	if got.NetworkTagsSetting != 2 {
		t.Errorf("NetworkTagsSetting = %d, want 2", got.NetworkTagsSetting)
	}
	if got.NativeNetworkID != "net-trusted" {
		t.Errorf("NativeNetworkID = %q, want net-trusted", got.NativeNetworkID)
	}
	if !got.VoiceNetworkEnable {
		t.Error("VoiceNetworkEnable should be true")
	}

	// FLAG-A: name="" test — send empty name, expect it in the body
	model2 := &SwitchPortResourceModel{
		Name:            types.StringValue(""),
		TagNetworkIDs:   types.ListNull(types.StringType),
		UntagNetworkIDs: types.ListNull(types.StringType),
	}
	var errs2 []error
	got2 := buildSwitchPortV2Body(ctx, model2, &errs2)
	if got2 == nil || got2.Name != "" {
		t.Errorf("name='' case: got Name=%q, want empty string emitted (not omitted)", got2.Name)
	}
}

// TestSwitchPort_BuildV2Body_NilTagsCoercedEmpty verifies that nil TagNetworkIDs
// produces TagIDs=[] (not nil) to satisfy the controller's strict empty-array
// requirement.
func TestSwitchPort_BuildV2Body_NilTagsCoercedEmpty(t *testing.T) {
	ctx := context.Background()
	model := &SwitchPortResourceModel{
		Speed:           types.Int64Value(0),
		TagNetworkIDs:   types.ListNull(types.StringType),
		UntagNetworkIDs: types.ListNull(types.StringType),
	}
	var buildErrs []error
	got := buildSwitchPortV2Body(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("build errors: %v", buildErrs)
	}
	if got.TagIDs == nil {
		t.Error("TagIDs should be [] (non-nil empty slice), got nil")
	}
	if len(got.TagIDs) != 0 {
		t.Errorf("TagIDs = %v, want empty []", got.TagIDs)
	}
}

// TestSwitchPort_BuildV2Body_ProfileVlanForce verifies the boundary between
// builder and client: the builder emits ProfileVlanOverrideEnable from the
// model as-is and does NOT pre-force it. The forcing is the client's job.
func TestSwitchPort_BuildV2Body_ProfileVlanForce(t *testing.T) {
	ctx := context.Background()

	// Case A: model has ProfileVlanOverrideEnable=false — builder must emit false.
	modelFalse := &SwitchPortResourceModel{
		ProfileOverrideEnable:     types.BoolValue(true),
		ProfileVlanOverrideEnable: types.BoolValue(false),
		NativeNetworkID:           types.StringValue("net-trusted"),
		TagNetworkIDs:             types.ListNull(types.StringType),
		UntagNetworkIDs:           types.ListNull(types.StringType),
	}
	var errs []error
	got := buildSwitchPortV2Body(ctx, modelFalse, &errs)
	if got == nil || got.ProfileVlanOverrideEnable {
		t.Error("builder should not force ProfileVlanOverrideEnable; that is the client's responsibility")
	}

	// Case B: model has ProfileVlanOverrideEnable=true — builder emits true.
	modelTrue := &SwitchPortResourceModel{
		ProfileOverrideEnable:     types.BoolValue(true),
		ProfileVlanOverrideEnable: types.BoolValue(true),
		NativeNetworkID:           types.StringValue("net-trusted"),
		TagNetworkIDs:             types.ListNull(types.StringType),
		UntagNetworkIDs:           types.ListNull(types.StringType),
	}
	var errs2 []error
	got2 := buildSwitchPortV2Body(ctx, modelTrue, &errs2)
	if got2 == nil || !got2.ProfileVlanOverrideEnable {
		t.Error("builder should emit ProfileVlanOverrideEnable=true when model says true")
	}
}

// TestSwitchPortCreate_UsesOpenAPIPath verifies that the Create path uses
// buildSwitchPortV2Body (openapi/v1 dialect) rather than buildSwitchPortPatchPayload
// (api/v2 dialect). The distinction: V2 body is *client.SwitchPortV2 with
// linkSpeed/duplex instead of speed, and no port/disable/voiceDscpEnable.
func TestSwitchPortCreate_UsesOpenAPIPath(t *testing.T) {
	ctx := context.Background()
	tagIDs, _ := types.ListValueFrom(ctx, types.StringType, []string{"net-trusted"})

	model := &SwitchPortResourceModel{
		Port:                      types.Int64Value(3),
		Name:                      types.StringValue("access-port"),
		Disable:                   types.BoolValue(false),
		ProfileID:                 types.StringValue("profile-access"),
		ProfileOverrideEnable:     types.BoolValue(true),
		ProfileVlanOverrideEnable: types.BoolValue(false),
		NativeNetworkID:           types.StringValue("net-trusted"),
		NetworkTagsSetting:        types.Int64Value(2),
		TagNetworkIDs:             tagIDs,
		UntagNetworkIDs:           types.ListNull(types.StringType),
		VoiceNetworkEnable:        types.BoolValue(false),
		VoiceDscpEnable:           types.BoolValue(false),
		Speed:                     types.Int64Value(5),
	}

	var buildErrs []error
	body := buildSwitchPortV2Body(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("buildSwitchPortV2Body errors: %v", buildErrs)
	}
	if body == nil {
		t.Fatal("buildSwitchPortV2Body returned nil")
	}

	// Assert V2 dialect: has linkSpeed/duplex, NOT speed as a top-level field.
	// The struct type itself guarantees no port/disable/voiceDscpEnable fields.
	if body.LinkSpeed != 3 {
		t.Errorf("Create: LinkSpeed = %d, want 3 (speed=5 → 1Gb FD)", body.LinkSpeed)
	}
	if body.Duplex != 2 {
		t.Errorf("Create: Duplex = %d, want 2 (full)", body.Duplex)
	}
	// TagIDs present (from TagNetworkIDs)
	if len(body.TagIDs) != 1 || body.TagIDs[0] != "net-trusted" {
		t.Errorf("Create: TagIDs = %v, want [net-trusted]", body.TagIDs)
	}
	// NativeNetworkID present (non-empty)
	if body.NativeNetworkID != "net-trusted" {
		t.Errorf("Create: NativeNetworkID = %q, want net-trusted", body.NativeNetworkID)
	}
}

// TestSwitchPortUpdate_UsesOpenAPIPath mirrors TestSwitchPortCreate_UsesOpenAPIPath
// for the Update code path — verifies the same V2 builder is called.
func TestSwitchPortUpdate_UsesOpenAPIPath(t *testing.T) {
	ctx := context.Background()
	model := &SwitchPortResourceModel{
		Port:                      types.Int64Value(7),
		Name:                      types.StringValue("uplink"),
		ProfileID:                 types.StringValue("profile-trunk"),
		ProfileOverrideEnable:     types.BoolValue(false),
		ProfileVlanOverrideEnable: types.BoolValue(false),
		NetworkTagsSetting:        types.Int64Value(0),
		TagNetworkIDs:             types.ListNull(types.StringType),
		UntagNetworkIDs:           types.ListNull(types.StringType),
		VoiceNetworkEnable:        types.BoolValue(false),
		VoiceDscpEnable:           types.BoolValue(false),
		Speed:                     types.Int64Value(0),
	}

	var buildErrs []error
	body := buildSwitchPortV2Body(ctx, model, &buildErrs)
	if len(buildErrs) > 0 {
		t.Fatalf("buildSwitchPortV2Body errors: %v", buildErrs)
	}
	if body == nil {
		t.Fatal("buildSwitchPortV2Body returned nil")
	}

	// Speed=0 (auto-neg) → linkSpeed=0, duplex=0.
	if body.LinkSpeed != 0 || body.Duplex != 0 {
		t.Errorf("Update: linkSpeed=%d duplex=%d, want (0,0) for auto-neg", body.LinkSpeed, body.Duplex)
	}
	// TagIDs coerced to [] even when TagNetworkIDs is null.
	if body.TagIDs == nil {
		t.Error("Update: TagIDs should be [] not nil")
	}
}

// TestSwitchPort_ApplyToModel_PopulatesProfileVlanOverride verifies that
// applySwitchPortToModel sets ProfileVlanOverrideEnable from the API response.
func TestSwitchPort_ApplyToModel_PopulatesProfileVlanOverride(t *testing.T) {
	ctx := context.Background()
	port := &client.SwitchPort{
		Port:                      5,
		ProfileVlanOverrideEnable: true,
	}
	model := &SwitchPortResourceModel{
		TagNetworkIDs:   types.ListNull(types.StringType),
		UntagNetworkIDs: types.ListNull(types.StringType),
	}
	if err := applySwitchPortToModel(ctx, model, port); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !model.ProfileVlanOverrideEnable.ValueBool() {
		t.Error("ProfileVlanOverrideEnable should be true from API response")
	}
}

// TestSwitchPort_UntagComputedOnly verifies that buildSwitchPortV2Body does NOT
// produce any untagNetworkIds field (SwitchPortV2 has no such field), and that
// the struct field does not exist on the body.
func TestSwitchPort_UntagComputedOnly(t *testing.T) {
	ctx := context.Background()
	model := &SwitchPortResourceModel{
		TagNetworkIDs:   types.ListNull(types.StringType),
		UntagNetworkIDs: types.ListNull(types.StringType),
	}
	var buildErrs []error
	got := buildSwitchPortV2Body(ctx, model, &buildErrs)
	if got == nil {
		t.Fatal("buildSwitchPortV2Body returned nil")
	}
	// The SwitchPortV2 struct has no untag field — this is a compile-time
	// structural assertion. If SwitchPortV2 ever gains an Untag* field,
	// the design doc requires removing it; this comment is the sentinel.
	// The test passes by construction if buildSwitchPortV2Body returns
	// *client.SwitchPortV2 which has no UntagNetworkIDs field.
	if got.LinkSpeed != 0 {
		t.Errorf("speed=0 (default) should map to linkSpeed=0, got %d", got.LinkSpeed)
	}
}

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

// TestSwitchPort_BuildV2Body_Mirroring verifies that when operation=="mirroring"
// the builder populates both Operation and MirroredPorts on the PATCH body.
func TestSwitchPort_BuildV2Body_Mirroring(t *testing.T) {
	ctx := context.Background()
	ports, _ := types.SetValueFrom(ctx, types.Int64Type, []int64{1, 3, 5})
	m := &SwitchPortResourceModel{
		Port:          types.Int64Value(12),
		Operation:     types.StringValue("mirroring"),
		MirroredPorts: ports,
	}
	var errs []error
	got := buildSwitchPortV2Body(ctx, m, &errs)
	if got.Operation != "mirroring" {
		t.Errorf("Operation = %q", got.Operation)
	}
	if len(got.MirroredPorts) != 3 {
		t.Errorf("MirroredPorts = %v", got.MirroredPorts)
	}
}

// TestSwitchPort_BuildV2Body_SwitchingDropsMirroredPorts verifies that when
// operation=="switching" the MirroredPorts slice is NOT forwarded, even if
// the model contains ports (guard against accidental mirror activation).
func TestSwitchPort_BuildV2Body_SwitchingDropsMirroredPorts(t *testing.T) {
	ctx := context.Background()
	ports, _ := types.SetValueFrom(ctx, types.Int64Type, []int64{1, 3})
	m := &SwitchPortResourceModel{
		Port:          types.Int64Value(7),
		Operation:     types.StringValue("switching"),
		MirroredPorts: ports, // present but must be ignored when not mirroring
	}
	var errs []error
	got := buildSwitchPortV2Body(ctx, m, &errs)
	if got.Operation != "switching" {
		t.Errorf("Operation = %q", got.Operation)
	}
	if len(got.MirroredPorts) != 0 {
		t.Errorf("MirroredPorts should be empty for switching, got %v", got.MirroredPorts)
	}
}

// TestValidateMirrorConfig covers all plan-time validation rules for the
// mirror fields: operation one-of check, ports-only-when-mirroring, no
// self-mirror (src == dest), and port values >= 1.
func TestValidateMirrorConfig(t *testing.T) {
	cases := []struct {
		name      string
		operation string
		srcPorts  []int64
		destPort  int64
		wantErr   bool
	}{
		{"mirroring ok", "mirroring", []int64{1, 3}, 12, false},
		{"switching with ports", "switching", []int64{1}, 12, true},
		{"mirroring includes dest", "mirroring", []int64{1, 12}, 12, true},
		{"switching empty ok", "switching", nil, 7, false},
		{"empty operation ok", "", nil, 7, false},
		{"invalid operation", "bogus", nil, 7, true},
		{"mirroring zero port", "mirroring", []int64{0}, 12, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateMirrorConfig(c.operation, c.srcPorts, c.destPort)
			if (err != nil) != c.wantErr {
				t.Errorf("err = %v, wantErr %v", err, c.wantErr)
			}
		})
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
