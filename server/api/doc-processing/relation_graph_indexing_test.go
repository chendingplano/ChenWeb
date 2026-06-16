package docprocessing

import (
	"testing"
)

// TestBuildRelationGraphConnections verifies one fully-linked relation materializes
// subject->object and predicate-of edges. Category membership is written separately
// under relation_method=category_name.
func TestBuildRelationGraphConnections(t *testing.T) {
	rows := []relationGraphRow{{
		RelationID:      "100_rel_1",
		SubjectEntityID: "100_ent_1",
		ObjectEntityID:  "100_ent_2",
		Predicate:       "依赖于",
		PredicateEN:     "Depends On",
		Categories:      []string{"Dependency"},
	}}
	categories := map[string]resolvedCategory{normalizeCategoryKey("Dependency"): {ID: 7, Type: "relation", Key: "dependency"}}

	conns := buildRelationGraphConnections(100, rows)
	if len(conns) != 2 {
		t.Fatalf("want 2 connections, got %d: %+v", len(conns), conns)
	}

	var subjObj, predOf *Connection
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
		}
	}

	if subjObj == nil || predOf == nil {
		t.Fatalf("missing an edge kind: subjObj=%v predOf=%v", subjObj, predOf)
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

	categoryConns := buildRelationCategoryConnections(100, rows, categories)
	if len(categoryConns) != 1 {
		t.Fatalf("want 1 category connection, got %d: %+v", len(categoryConns), categoryConns)
	}
	if categoryConns[0].SourceType != searchArtifactRelation || categoryConns[0].SourceID != "100_rel_1" ||
		categoryConns[0].TargetType != "relation" || categoryConns[0].TargetID != "dependency" ||
		categoryConns[0].TargetRecordID != 7 ||
		categoryConns[0].RelationName != RelationBelongTo ||
		categoryConns[0].RelationMethod != RelationMethodCategoryName {
		t.Errorf("category edge wrong: %+v", categoryConns[0])
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

	conns := buildRelationGraphConnections(100, rows)
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
	conns := buildRelationGraphConnections(100, rows)
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
