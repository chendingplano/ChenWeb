package docprocessing

import (
	"reflect"
	"testing"
)

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
		"category_key":  "Response Time",
		"display_names": []any{"Response Time", "Latency"},
		"aliases":       []any{"response latency"},
		"acronyms":      []any{"RT"},
		"description":   "How long to respond.",
		"keywords":      []any{"delay", "ms"},
	}
	got, err := parseCreateCategoryResponse(payload)
	if err != nil {
		t.Fatalf("parseCreateCategoryResponse error: %v", err)
	}
	if got.CategoryKey != "Response Time" {
		t.Errorf("CategoryKey = %q, want %q", got.CategoryKey, "Response Time")
	}
	if !reflect.DeepEqual(got.DisplayNames, []string{"Response Time", "Latency"}) {
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
	got, ok := matchCategoryInSnapshot("rt", nil, snap, 0.8)
	if !ok || got.CategoryID != 2 {
		t.Fatalf("matchCategoryInSnapshot = (%+v, %v), want id 2", got, ok)
	}
}

func TestMatchCategoryInSnapshotNoMatch(t *testing.T) {
	snap := []artifactCategoryRecord{{CategoryID: 1, CategoryKey: "throughput", MatchKeys: []string{"throughput"}}}
	if _, ok := matchCategoryInSnapshot("latency", nil, snap, 0.8); ok {
		t.Fatal("expected no match")
	}
}

func TestMatchCategoryInSnapshotCosineAboveThreshold(t *testing.T) {
	snap := []artifactCategoryRecord{
		{CategoryID: 1, CategoryKey: "throughput", MatchKeys: []string{"throughput"}, Embedding: []float64{0, 1}},
		{CategoryID: 2, CategoryKey: "response time", MatchKeys: []string{"response time"}, Embedding: []float64{1, 0}},
	}
	got, ok := matchCategoryInSnapshot("reaction time", []float64{0.99, 0.01}, snap, 0.8)
	if !ok || got.CategoryID != 2 {
		t.Fatalf("matchCategoryInSnapshot = (%+v, %v), want id 2 via cosine", got, ok)
	}
}

func TestMatchCategoryInSnapshotCosineBelowThreshold(t *testing.T) {
	snap := []artifactCategoryRecord{
		{CategoryID: 2, CategoryKey: "response time", MatchKeys: []string{"response time"}, Embedding: []float64{1, 0}},
	}
	if _, ok := matchCategoryInSnapshot("reaction time", []float64{0, 1}, snap, 0.8); ok {
		t.Fatal("expected no cosine match below threshold")
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
