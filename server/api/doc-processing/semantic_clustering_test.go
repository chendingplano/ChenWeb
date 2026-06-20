package docprocessing

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSameEntityType(t *testing.T) {
	tests := []struct {
		name                   string
		typeA, typeAEN, typeB, typeBEN string
		want                   bool
	}{
		{"exact match", "facility", "", "facility", "", true},
		{"normalized match", "Chemical Compound", "", "chemical_compound", "", true},
		{"EN match", "", "facility", "", "facility", true},
		{"cross language match", "设施", "facility", "", "facility", true},
		{"different types", "organization", "", "facility", "", false},
		{"one missing type (allow)", "", "", "facility", "", true},
		{"both empty", "", "", "", "", true},
		{"different EN types", "", "organization", "", "facility", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sameEntityType(tt.typeA, tt.typeAEN, tt.typeB, tt.typeBEN)
			if got != tt.want {
				t.Errorf("sameEntityType(%q,%q,%q,%q) = %v, want %v",
					tt.typeA, tt.typeAEN, tt.typeB, tt.typeBEN, got, tt.want)
			}
		})
	}
}

func TestHasCommonCategory(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"shared category", []string{"wastewater", "treatment"}, []string{"treatment", "sludge"}, true},
		{"normalized match", []string{"Wastewater Treatment"}, []string{"wastewater treatment"}, true},
		{"no shared category", []string{"wastewater"}, []string{"drinking_water"}, false},
		{"a empty (allow)", nil, []string{"wastewater"}, true},
		{"b empty (allow)", []string{"wastewater"}, nil, true},
		{"both empty (allow)", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCommonCategory(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("hasCommonCategory(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestElectSurvivorFromIDs(t *testing.T) {
	t.Run("single element", func(t *testing.T) {
		from, into := electSurvivorFromIDs([]string{"100_ent_1"})
		if len(from) != 0 || into != "" {
			t.Errorf("single element should return empty: got from=%v into=%q", from, into)
		}
	})

	t.Run("two elements lexicographic", func(t *testing.T) {
		from, into := electSurvivorFromIDs([]string{"100_ent_5", "100_ent_2"})
		if into != "100_ent_2" {
			t.Errorf("expected 100_ent_2 (lexicographically smaller), got %q", into)
		}
		if len(from) != 1 || from[0] != "100_ent_5" {
			t.Errorf("expected from=[100_ent_5], got %v", from)
		}
	})

	t.Run("three elements", func(t *testing.T) {
		from, into := electSurvivorFromIDs([]string{"200_ent_3", "100_ent_1", "150_ent_2"})
		if into != "100_ent_1" {
			t.Errorf("expected 100_ent_1, got %q", into)
		}
		if len(from) != 2 {
			t.Errorf("expected 2 absorbed, got %d", len(from))
		}
	})
}

func TestAllIDsInSet(t *testing.T) {
	set := map[string]bool{"a": true, "b": true, "c": true}
	if !allIDsInSet([]string{"a", "b"}, set) {
		t.Error("should be true")
	}
	if allIDsInSet([]string{"a", "d"}, set) {
		t.Error("should be false (d not in set)")
	}
}

func TestAllInSameGroup(t *testing.T) {
	batch := []semClusterGroup{
		{GroupID: "1", Members: []semClusterMember{
			{EntityID: "a"}, {EntityID: "b"},
		}},
		{GroupID: "2", Members: []semClusterMember{
			{EntityID: "c"}, {EntityID: "d"},
		}},
	}

	if !allInSameGroup(batch, "a", "b") {
		t.Error("a,b should be in same group")
	}
	if allInSameGroup(batch, "a", "c") {
		t.Error("a,c should be in different groups")
	}
	if allInSameGroup(batch, "x", "y") {
		t.Error("x,y should not be in any group")
	}
}

func TestParseAdjudicationResult_Merge(t *testing.T) {
	groups := []semClusterGroup{
		{GroupID: "100_ent_1", Members: []semClusterMember{
			{EntityID: "100_ent_1"}, {EntityID: "200_ent_2"},
		}},
	}

	payload := map[string]any{
		"100_ent_1": map[string]any{
			"groups": []any{
				map[string]any{
					"member_entity_ids": []any{"100_ent_1", "200_ent_2"},
					"canonical_name":    "Odor Treatment Facility",
					"confidence":        0.93,
					"rationale":         "Same facility",
					"evidence": map[string]any{
						"type_agree":    true,
						"keyword_match": []any{"odor", "deodorization"},
					},
				},
			},
			"keep_separate": []any{},
			"defer":         []any{},
			"uncertain":     []any{},
		},
	}

	result := parseAdjudicationResult(payload, groups)
	if len(result.Merges) != 1 {
		t.Fatalf("expected 1 merge, got %d", len(result.Merges))
	}
	m := result.Merges[0]
	if m.IntoEntityID != "100_ent_1" {
		t.Errorf("survivor should be 100_ent_1 (lex smaller), got %s", m.IntoEntityID)
	}
	if m.FromEntityID != "200_ent_2" {
		t.Errorf("absorbed should be 200_ent_2, got %s", m.FromEntityID)
	}
	if m.Confidence != 0.93 {
		t.Errorf("confidence = %f, want 0.93", m.Confidence)
	}
	if m.Reason != "Same facility" {
		t.Errorf("reason = %q", m.Reason)
	}
}

func TestParseAdjudicationResult_BelowThreshold(t *testing.T) {
	groups := []semClusterGroup{
		{GroupID: "100_ent_1", Members: []semClusterMember{
			{EntityID: "100_ent_1"}, {EntityID: "200_ent_2"},
		}},
	}

	payload := map[string]any{
		"100_ent_1": map[string]any{
			"groups": []any{
				map[string]any{
					"member_entity_ids": []any{"100_ent_1", "200_ent_2"},
					"confidence":        0.75, // below default 0.90 merge_min
				},
			},
		},
	}

	result := parseAdjudicationResult(payload, groups)
	if len(result.Merges) != 0 {
		t.Errorf("expected 0 merges (confidence below threshold), got %d", len(result.Merges))
	}
}

func TestParseAdjudicationResult_CrossGroupGuard(t *testing.T) {
	groups := []semClusterGroup{
		{GroupID: "g1", Members: []semClusterMember{
			{EntityID: "a"}, {EntityID: "b"},
		}},
	}

	// LLM fabricates a merge that spans two groups (a and c, but c is not in g1).
	payload := map[string]any{
		"g1": map[string]any{
			"groups": []any{
				map[string]any{
					"member_entity_ids": []any{"a", "c"}, // c is not in g1
					"confidence":        0.95,
				},
			},
		},
	}

	result := parseAdjudicationResult(payload, groups)
	if len(result.Merges) != 0 {
		t.Errorf("expected 0 merges (cross-group guard), got %d", len(result.Merges))
	}
}

func TestParseAdjudicationResult_MissingGroupID(t *testing.T) {
	groups := []semClusterGroup{
		{GroupID: "g1", Members: []semClusterMember{
			{EntityID: "a"}, {EntityID: "b"},
		}},
	}

	// LLM didn't return a verdict for g1.
	payload := map[string]any{}

	result := parseAdjudicationResult(payload, groups)
	if len(result.Merges) != 0 {
		t.Errorf("expected 0 merges, got %d", len(result.Merges))
	}
}

func TestSemClusterEnabled_EnvVar(t *testing.T) {
	orig := os.Getenv("SEMCLUSTER_ENABLED")
	defer os.Setenv("SEMCLUSTER_ENABLED", orig)

	os.Setenv("SEMCLUSTER_ENABLED", "")
	if semClusterEnabled() {
		t.Error("should be false when unset")
	}

	os.Setenv("SEMCLUSTER_ENABLED", "true")
	if !semClusterEnabled() {
		t.Error("should be true")
	}

	os.Setenv("SEMCLUSTER_ENABLED", "false")
	if semClusterEnabled() {
		t.Error("should be false")
	}

	os.Setenv("SEMCLUSTER_ENABLED", "1")
	if !semClusterEnabled() {
		t.Error("should be true for 1")
	}
}

func TestSemClusterEnvVarDefaults(t *testing.T) {
	// Save and clear relevant env vars so defaults are tested.
	save := map[string]string{}
	for _, k := range []string{
		"SEMCLUSTER_MIN_BLOCK_COSINE", "SEMCLUSTER_MERGE_MIN", "SEMCLUSTER_HUMAN_MIN",
		"SEMCLUSTER_GROUP_SIZE", "SEMCLUSTER_BATCH_MAX_ENTITIES",
	} {
		save[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range save {
			os.Setenv(k, v)
		}
	}()

	if v := semClusterMinBlockCosine(); v != defaultSemClusterMinBlockCosine {
		t.Errorf("min block cosine default: got %f, want %f", v, defaultSemClusterMinBlockCosine)
	}
	if v := semClusterMergeMin(); v != defaultSemClusterMergeMin {
		t.Errorf("merge min default: got %f, want %f", v, defaultSemClusterMergeMin)
	}
	if v := semClusterHumanMin(); v != defaultSemClusterHumanMin {
		t.Errorf("human min default: got %f, want %f", v, defaultSemClusterHumanMin)
	}
	if v := semClusterGroupSize(); v != defaultSemClusterGroupSize {
		t.Errorf("group size default: got %d, want %d", v, defaultSemClusterGroupSize)
	}
	if v := semClusterBatchMaxEntities(); v != defaultSemClusterBatchMaxEnts {
		t.Errorf("batch max entities default: got %d, want %d", v, defaultSemClusterBatchMaxEnts)
	}
}

func TestSemClusterMinBlockCosine_EnvVar(t *testing.T) {
	orig := os.Getenv("SEMCLUSTER_MIN_BLOCK_COSINE")
	defer os.Setenv("SEMCLUSTER_MIN_BLOCK_COSINE", orig)

	os.Setenv("SEMCLUSTER_MIN_BLOCK_COSINE", "0.95")
	if v := semClusterMinBlockCosine(); v != 0.95 {
		t.Errorf("got %f, want 0.95", v)
	}

	os.Setenv("SEMCLUSTER_MIN_BLOCK_COSINE", "")
	if v := semClusterMinBlockCosine(); v != defaultSemClusterMinBlockCosine {
		t.Errorf("default should be %f, got %f", defaultSemClusterMinBlockCosine, v)
	}
}

func TestClamp01(t *testing.T) {
	if v := clamp01(-0.5); v != 0 {
		t.Errorf("clamp(-0.5) = %f, want 0", v)
	}
	if v := clamp01(0.5); v != 0.5 {
		t.Errorf("clamp(0.5) = %f, want 0.5", v)
	}
	if v := clamp01(1.5); v != 1 {
		t.Errorf("clamp(1.5) = %f, want 1", v)
	}
}

func TestParseVectorLiteralSafe(t *testing.T) {
	if v := parseVectorLiteralSafe(""); v != nil {
		t.Errorf("empty string should return nil, got %v", v)
	}
	vec := parseVectorLiteralSafe("[0.1,0.2,0.3]")
	if len(vec) != 3 || vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Errorf("got %v, want [0.1,0.2,0.3]", vec)
	}
	if v := parseVectorLiteralSafe("garbage"); v != nil {
		t.Errorf("garbage should return nil, got %v", v)
	}
}

func TestElectSurvivorFromIDs_Empty(t *testing.T) {
	from, into := electSurvivorFromIDs(nil)
	if len(from) != 0 || into != "" {
		t.Error("nil input should return empty")
	}
	from, into = electSurvivorFromIDs([]string{})
	if len(from) != 0 || into != "" {
		t.Error("empty input should return empty")
	}
}

func TestEntityRowToSemClusterMember(t *testing.T) {
	e := pendingEntityRow{
		EntityID:     "100_ent_1",
		Entity:       "Test Entity",
		EntityEN:     "Test Entity",
		EntityType:   "facility",
		Aliases:      []string{"alias1"},
		Categories:   []string{"cat1"},
		EntityStatus: "extracted",
	}
	m := entityRowToSemClusterMember(e)
	if m.EntityID != e.EntityID {
		t.Errorf("EntityID mismatch")
	}
	if len(m.Aliases) != 1 || m.Aliases[0] != "alias1" {
		t.Errorf("aliases mismatch")
	}
	if len(m.Categories) != 1 || m.Categories[0] != "cat1" {
		t.Errorf("categories mismatch")
	}
}

func TestMarkPendingAsClustered_EmptySlice(t *testing.T) {
	// Should not panic on empty input.
	if err := markPendingAsClustered(t.Context(), nil, nil); err == nil {
		// Expected to fail with nil DB; the function itself shouldn't panic.
	}
}

func TestResolveSemClusterConfig_Disabled(t *testing.T) {
	orig := os.Getenv("SEMCLUSTER_ENABLED")
	defer os.Setenv("SEMCLUSTER_ENABLED", orig)
	os.Setenv("SEMCLUSTER_ENABLED", "false")

	cfg := ResolveSemClusterConfig()
	if cfg.Enabled {
		t.Error("should be disabled")
	}
}

func TestMapFromAny(t *testing.T) {
	if m := mapFromAny(nil); m != nil {
		t.Error("nil should return nil")
	}
	if m := mapFromAny("string"); m != nil {
		t.Error("string should return nil")
	}
	input := map[string]any{"key": "value"}
	if m := mapFromAny(input); m == nil || m["key"] != "value" {
		t.Error("map should be returned")
	}
}

// TestSemClusterGroupJSONSerialization verifies the JSON tags on semClusterGroup
// and semClusterMember match the adjudication prompt contract.
func TestSemClusterGroupJSONSerialization(t *testing.T) {
	group := semClusterGroup{
		GroupID: "g1",
		Members: []semClusterMember{
			{EntityID: "100_ent_1", Entity: "Test", EntityStatus: "extracted"},
		},
	}

	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(group); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	data := buf.String()
	if !strings.Contains(data, `"group_id":"g1"`) {
		t.Errorf("expected group_id in JSON, got %s", data)
	}
	if !strings.Contains(data, `"entity_id":"100_ent_1"`) {
		t.Errorf("expected entity_id in JSON, got %s", data)
	}
}
