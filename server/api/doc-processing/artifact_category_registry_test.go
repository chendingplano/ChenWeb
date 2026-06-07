package docprocessing

import (
	"reflect"
	"testing"
)

// ---- categoryIndex tests ----

func TestCategoryIndexLookupMiss(t *testing.T) {
	idx := newCategoryIndex()
	if _, ok := idx.lookup("metric", "latency"); ok {
		t.Fatal("expected miss on empty index")
	}
}

func TestCategoryIndexPutAndLookup(t *testing.T) {
	idx := newCategoryIndex()
	idx.put("metric", "latency", 7, 0)
	id, ok := idx.lookup("metric", "latency")
	if !ok || id != 7 {
		t.Fatalf("lookup = (%d, %v), want (7, true)", id, ok)
	}
}

func TestCategoryIndexPutConflictHigherSeenCountWins(t *testing.T) {
	idx := newCategoryIndex()
	idx.put("metric", "rt", 1, 5)   // seen_count=5
	conflict := idx.put("metric", "rt", 2, 10) // seen_count=10 wins
	if !conflict {
		t.Fatal("expected conflict=true")
	}
	id, _ := idx.lookup("metric", "rt")
	if id != 2 {
		t.Fatalf("id = %d, want 2 (higher seen_count wins)", id)
	}
}

func TestCategoryIndexPutConflictExistingWinsWhenHigher(t *testing.T) {
	idx := newCategoryIndex()
	idx.put("metric", "rt", 1, 10) // seen_count=10
	conflict := idx.put("metric", "rt", 2, 5) // seen_count=5 loses
	if !conflict {
		t.Fatal("expected conflict=true")
	}
	id, _ := idx.lookup("metric", "rt")
	if id != 1 {
		t.Fatalf("id = %d, want 1 (existing higher seen_count kept)", id)
	}
}

func TestCategoryIndexPutAllSetsMultipleKeys(t *testing.T) {
	idx := newCategoryIndex()
	idx.putAll("metric", []string{"latency", "response time", "rt"}, 7)
	for _, k := range []string{"latency", "response time", "rt"} {
		if id, ok := idx.lookup("metric", k); !ok || id != 7 {
			t.Errorf("lookup(%q) = (%d, %v), want (7, true)", k, id, ok)
		}
	}
}

func TestCategoryIndexIsLoadedAndMarkLoaded(t *testing.T) {
	idx := newCategoryIndex()
	if idx.isLoaded("metric") {
		t.Fatal("expected not loaded initially")
	}
	idx.markLoaded("metric")
	if !idx.isLoaded("metric") {
		t.Fatal("expected loaded after markLoaded")
	}
}

func TestCategoryIndexTypeIsolation(t *testing.T) {
	idx := newCategoryIndex()
	idx.put("metric", "latency", 1, 0)
	if _, ok := idx.lookup("inventory_item", "latency"); ok {
		t.Fatal("key in 'metric' should not be visible under 'inventory_item'")
	}
}

func TestNormalizeCategoryKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Response Time", "response time"},
		{"  API  Latency!  ", "api latency"},
		{"energy_efficiency", "energy efficiency"},
		{"CPU-Usage (%)", "cpu usage"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := normalizeCategoryKey(c.in); got != c.want {
			t.Errorf("normalizeCategoryKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildArtifactMatchKeys(t *testing.T) {
	got := buildArtifactMatchKeys(
		"response time",
		[]string{"Response Time", "Latency"},
		[]string{"response latency"},
		[]string{"RT", "TTFB"},
	)
	want := []string{"response time", "latency", "response latency", "rt", "ttfb"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildArtifactMatchKeys = %#v, want %#v", got, want)
	}
}

func TestBuildArtifactMatchKeysDropsEmptyAndDuplicates(t *testing.T) {
	got := buildArtifactMatchKeys(
		"response time",
		[]string{"response time", "  ", "Latency"},
		nil,
		[]string{"latency"},
	)
	want := []string{"response time", "latency"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildArtifactMatchKeys = %#v, want %#v", got, want)
	}
}

func TestBuildCategorySearchDocument(t *testing.T) {
	got := buildCategorySearchDocument(
		"response time",
		[]string{"Response Time", "Latency"},
		[]string{"response latency"},
		[]string{"RT"},
		[]string{"delay", "ms"},
		"How long to respond.",
	)
	// "Response Time" is a case-insensitive duplicate of the key and is dropped.
	want := "response time Latency response latency RT delay ms How long to respond."
	if got != want {
		t.Errorf("buildCategorySearchDocument = %q, want %q", got, want)
	}
}

func TestBuildCategorySearchDocumentSkipsEmptyFields(t *testing.T) {
	got := buildCategorySearchDocument("shoes", nil, nil, nil, nil, "")
	if got != "shoes" {
		t.Errorf("buildCategorySearchDocument = %q, want %q", got, "shoes")
	}
}

func TestParseCreateCategoryResponse(t *testing.T) {
	payload := map[string]any{
		"canonical_key":       "response_time",
		"canonical_name":      "Response Time",
		"aliases":             []any{"response latency"},
		"acronyms":            []any{"RT"},
		"description":         "How long to respond.",
		"keywords":            []any{"delay", "ms"},
		"typical_attributes":  []any{map[string]any{"name": "unit", "description": "Milliseconds"}},
		"typical_specs":       map[string]any{"unit": "ms"},
		"common_value_ranges": []any{map[string]any{"range": "0-5000 ms"}},
		"subcategory_of":      []any{"performance_metric", "api_metric"},
		"related_categories":  []any{"throughput", "error_rate"},
	}
	got, err := parseCreateCategoryResponse(payload)
	if err != nil {
		t.Fatalf("parseCreateCategoryResponse error: %v", err)
	}
	if got.CategoryKey != "response time" {
		t.Errorf("CategoryKey = %q, want %q", got.CategoryKey, "response time")
	}
	if !reflect.DeepEqual(got.DisplayNames, []string{"Response Time"}) {
		t.Errorf("DisplayNames = %#v", got.DisplayNames)
	}
	if !reflect.DeepEqual(got.Aliases, []string{"response latency"}) {
		t.Errorf("Aliases = %#v", got.Aliases)
	}
	if !reflect.DeepEqual(got.Acronyms, []string{"RT"}) {
		t.Errorf("Acronyms = %#v", got.Acronyms)
	}
	if got.Description != "How long to respond." {
		t.Errorf("Description = %q", got.Description)
	}
	if !reflect.DeepEqual(got.Keywords, []string{"delay", "ms"}) {
		t.Errorf("Keywords = %#v", got.Keywords)
	}
	if !reflect.DeepEqual(got.RequiredAttrs, []any{map[string]any{"name": "unit", "description": "Milliseconds"}}) {
		t.Errorf("RequiredAttrs = %#v", got.RequiredAttrs)
	}
	if !reflect.DeepEqual(got.Specs, map[string]any{"unit": "ms"}) {
		t.Errorf("Specs = %#v", got.Specs)
	}
	if !reflect.DeepEqual(got.PlausibleRanges, []any{map[string]any{"range": "0-5000 ms"}}) {
		t.Errorf("PlausibleRanges = %#v", got.PlausibleRanges)
	}
	if !reflect.DeepEqual(got.ParentCategories, []string{"performance metric", "api metric"}) {
		t.Errorf("ParentCategories = %#v", got.ParentCategories)
	}
	if !reflect.DeepEqual(got.RelatedCategories, []string{"throughput", "error rate"}) {
		t.Errorf("RelatedCategories = %#v", got.RelatedCategories)
	}
}

func TestParseCreateCategoryResponseRejectsMissingKey(t *testing.T) {
	if _, err := parseCreateCategoryResponse(map[string]any{"description": "x"}); err == nil {
		t.Fatal("expected error for missing category_key, got nil")
	}
}

func TestMatchCategoryInSnapshotMatchesAlias(t *testing.T) {
	snap := []artifactCategoryRecord{
		{CategoryID: 1, CategoryKey: "throughput", MatchKeys: []string{"throughput"}},
		{CategoryID: 2, CategoryKey: "response time", MatchKeys: []string{"response time", "rt"}},
	}
	got, ok := matchCategoryInSnapshot("rt", snap)
	if !ok || got.CategoryID != 2 {
		t.Fatalf("matchCategoryInSnapshot = (%+v, %v), want id 2", got, ok)
	}
}

func TestMatchCategoryInSnapshotNoMatch(t *testing.T) {
	snap := []artifactCategoryRecord{{CategoryID: 1, CategoryKey: "throughput", MatchKeys: []string{"throughput"}}}
	if _, ok := matchCategoryInSnapshot("latency", snap); ok {
		t.Fatal("expected no match")
	}
}

func TestResolveCanonicalCategoryFollowsMerged(t *testing.T) {
	recA := artifactCategoryRecord{CategoryID: 1, CategoryKey: "resp time", Status: "merged", CanonicalOf: "response time"}
	recB := artifactCategoryRecord{CategoryID: 2, CategoryKey: "response time", Status: "approved"}
	byKey := map[string]artifactCategoryRecord{"resp time": recA, "response time": recB}
	got := resolveCanonicalCategory(recA, byKey)
	if got.CategoryID != 2 {
		t.Fatalf("resolveCanonicalCategory = %+v, want id 2", got)
	}
}

func TestResolveCanonicalCategoryReturnsSelfWhenNotMerged(t *testing.T) {
	rec := artifactCategoryRecord{CategoryID: 5, CategoryKey: "latency", Status: "pending_review"}
	got := resolveCanonicalCategory(rec, map[string]artifactCategoryRecord{"latency": rec})
	if got.CategoryID != 5 {
		t.Fatalf("resolveCanonicalCategory = %+v, want id 5", got)
	}
}
