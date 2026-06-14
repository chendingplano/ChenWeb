package docprocessing

import (
	"testing"
)

// TestBuildRelationGraphConnections_AllThreeEdgeKinds verifies one fully-linked,
// categorized relation materializes the three canonical triples of ADR 2026061401:
// subject->object, predicate-of, and belong-to-category.
func TestBuildRelationGraphConnections_AllThreeEdgeKinds(t *testing.T) {
	rows := []relationGraphRow{{
		RelationID:      "100_rel_1",
		SubjectEntityID: "100_ent_1",
		ObjectEntityID:  "100_ent_2",
		Predicate:       "依赖于",
		PredicateEN:     "Depends On",
		Categories:      []string{"Dependency"},
	}}
	categoryIDs := map[string]int64{normalizeCategoryKey("Dependency"): 7}

	conns := buildRelationGraphConnections(100, rows, categoryIDs)
	if len(conns) != 3 {
		t.Fatalf("want 3 connections, got %d: %+v", len(conns), conns)
	}

	var subjObj, predOf, belong *Connection
	for i := range conns {
		c := &conns[i]
		if c.RelationMethod != RelationMethodEntityRelation {
			t.Errorf("connection %d: relation_method = %q, want %q", i, c.RelationMethod, RelationMethodEntityRelation)
		}
		switch c.ExtraInfo["edge_kind"] {
		case "subject_object":
			subjObj = c
		case "predicate_of":
			predOf = c
		case "belong_to_category":
			belong = c
		}
	}

	if subjObj == nil || predOf == nil || belong == nil {
		t.Fatalf("missing an edge kind: subjObj=%v predOf=%v belong=%v", subjObj, predOf, belong)
	}

	// Edge 1: subject entity -> object entity, named by the normalized predicate.
	if subjObj.SourceType != searchArtifactEntity || subjObj.SourceID != "100_ent_1" ||
		subjObj.TargetType != searchArtifactEntity || subjObj.TargetID != "100_ent_2" {
		t.Errorf("subject_object endpoints wrong: %+v", subjObj)
	}
	if subjObj.RelationName != "depends_on" {
		t.Errorf("subject_object relation_name = %q, want %q", subjObj.RelationName, "depends_on")
	}

	// Edge 2: relation_predicate -> relation.
	if predOf.SourceType != artifactTypeRelationPredicate || predOf.SourceID != "depends_on" ||
		predOf.TargetType != searchArtifactRelation || predOf.TargetID != "100_rel_1" ||
		predOf.RelationName != RelationHasPredicate {
		t.Errorf("predicate_of edge wrong: %+v", predOf)
	}

	// Edge 3: relation -> category (by surrogate category_id).
	if belong.SourceType != searchArtifactRelation || belong.SourceID != "100_rel_1" ||
		belong.TargetType != artifactTypeCategory || belong.TargetID != "7" ||
		belong.RelationName != RelationBelongToCategory {
		t.Errorf("belong_to_category edge wrong: %+v", belong)
	}
}

// TestBuildRelationGraphConnections_UnlinkedAndUnresolved verifies edges are skipped
// gracefully: no subject->object edge without both entity ids, no category edge for an
// unresolved category, but the predicate-of edge is still emitted.
func TestBuildRelationGraphConnections_UnlinkedAndUnresolved(t *testing.T) {
	rows := []relationGraphRow{{
		RelationID:      "100_rel_9",
		SubjectEntityID: "", // unlinked subject
		ObjectEntityID:  "100_ent_5",
		PredicateEN:     "references",
		Categories:      []string{"Unmapped"}, // not in categoryIDs
	}}

	conns := buildRelationGraphConnections(100, rows, map[string]int64{})
	if len(conns) != 1 {
		t.Fatalf("want 1 connection (predicate_of only), got %d: %+v", len(conns), conns)
	}
	if conns[0].ExtraInfo["edge_kind"] != "predicate_of" {
		t.Errorf("expected predicate_of edge, got %+v", conns[0])
	}
}

// TestBuildRelationGraphConnections_PredicateFallback verifies predicate_en falling back
// to the original-language predicate when empty, normalized to a snake_case key.
func TestBuildRelationGraphConnections_PredicateFallback(t *testing.T) {
	rows := []relationGraphRow{{
		RelationID:      "100_rel_2",
		SubjectEntityID: "100_ent_1",
		ObjectEntityID:  "100_ent_2",
		Predicate:       "Is Part Of",
		PredicateEN:     "",
	}}
	conns := buildRelationGraphConnections(100, rows, map[string]int64{})
	for _, c := range conns {
		if c.ExtraInfo["edge_kind"] == "subject_object" && c.RelationName != "is_part_of" {
			t.Errorf("fallback predicate key = %q, want %q", c.RelationName, "is_part_of")
		}
	}
}

func TestParseRelationCategories(t *testing.T) {
	got := parseRelationCategories([]byte(`["A","B","A"]`))
	if len(got) != 2 {
		t.Fatalf("want 2 deduped categories, got %d: %v", len(got), got)
	}
	if parseRelationCategories(nil) != nil {
		t.Errorf("nil input should yield nil")
	}
	if parseRelationCategories([]byte(`{"x":1}`)) != nil {
		t.Errorf("non-array JSON should yield nil")
	}
}
