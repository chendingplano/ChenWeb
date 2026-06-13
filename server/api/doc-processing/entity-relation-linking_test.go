package docprocessing

import (
	"strings"
	"testing"
)

func TestConsolidateEntitiesMergesByNameAndAlias(t *testing.T) {
	entities := []map[string]any{
		{
			"entity":       "Acme Corporation",
			"entity_type":  "org",
			"aliases":      []string{"Acme"},
			"keywords":     []string{"vendor"},
			"line_spans":   []string{"10-12"},
			"confidence":   0.8,
			"chunk_seq_no": 1,
		},
		{
			// Same entity referenced by its alias in another chunk.
			"entity":       "Acme",
			"entity_type":  "",
			"desc":         "A supplier.",
			"keywords":     []string{"supplier"},
			"line_spans":   []string{"40"},
			"confidence":   0.9,
			"chunk_seq_no": 3,
		},
		{
			"entity":       "Globex",
			"line_spans":   []string{"15"},
			"confidence":   0.5,
			"chunk_seq_no": 1,
		},
	}

	got := consolidateEntities(entities)
	if len(got) != 2 {
		t.Fatalf("expected 2 consolidated entities, got %d: %+v", len(got), got)
	}

	acme := got[0]
	if acme["entity"] != "Acme Corporation" {
		t.Errorf("canonical name = %q, want %q", acme["entity"], "Acme Corporation")
	}
	if acme["desc"] != "A supplier." {
		t.Errorf("desc should be filled from member, got %q", acme["desc"])
	}
	if c := toFloat(acme["confidence"]); c != 0.9 {
		t.Errorf("confidence = %v, want 0.9 (max)", c)
	}
	if cs, _ := chunkSeqNoOf(acme); cs != 1 {
		t.Errorf("chunk_seq_no = %v, want 1 (min)", cs)
	}
	spans := toStringSlice(acme["line_spans"])
	if len(spans) != 2 || spans[0] != "10-12" || spans[1] != "40" {
		t.Errorf("line_spans = %v, want [10-12 40]", spans)
	}
	kw := toStringSlice(acme["keywords"])
	if len(kw) != 2 {
		t.Errorf("keywords union = %v, want 2 entries", kw)
	}
	// "Acme" became an alias of the canonical "Acme Corporation".
	aliases := toStringSlice(acme["aliases"])
	foundAcme := false
	for _, a := range aliases {
		if a == "Acme" {
			foundAcme = true
		}
	}
	if !foundAcme {
		t.Errorf("aliases = %v, want to contain %q", aliases, "Acme")
	}
}

func TestConsolidateEntitiesNoDuplicates(t *testing.T) {
	entities := []map[string]any{
		{"entity": "A", "line_spans": []string{"1"}},
		{"entity": "B", "line_spans": []string{"2"}},
	}
	got := consolidateEntities(entities)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestBuildRelationWindowsSingleWindowForSmallDoc(t *testing.T) {
	entities := []map[string]any{
		{"entity": "A", "line_spans": []string{"5"}},
		{"entity": "B", "line_spans": []string{"20"}},
	}
	windows := buildRelationWindows(entities, 200, 10)
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	if len(windows[0]) != 2 {
		t.Errorf("window should hold both entities, got %d", len(windows[0]))
	}
}

func TestBuildRelationWindowsSplitsDistantClusters(t *testing.T) {
	entities := []map[string]any{
		{"entity": "A", "line_spans": []string{"5"}},
		{"entity": "B", "line_spans": []string{"8"}},
		{"entity": "C", "line_spans": []string{"500"}},
		{"entity": "D", "line_spans": []string{"505"}},
	}
	windows := buildRelationWindows(entities, 100, 10)
	if len(windows) < 2 {
		t.Fatalf("expected distant clusters to split into >=2 windows, got %d", len(windows))
	}
	// No single window should contain an entity from both clusters.
	for _, w := range windows {
		names := map[string]bool{}
		for _, e := range w {
			names[asString(e["entity"])] = true
		}
		if (names["A"] || names["B"]) && (names["C"] || names["D"]) {
			t.Errorf("window mixes distant clusters: %v", names)
		}
	}
}

func TestBuildRelationWindowsOverlapSharesBoundaryEntity(t *testing.T) {
	// Entities spaced so a boundary falls between them; overlap should put the
	// boundary pair together in at least one window.
	entities := []map[string]any{
		{"entity": "A", "line_spans": []string{"1"}},
		{"entity": "B", "line_spans": []string{"95"}},
		{"entity": "C", "line_spans": []string{"104"}},
		{"entity": "D", "line_spans": []string{"200"}},
	}
	windows := buildRelationWindows(entities, 100, 20)
	together := false
	for _, w := range windows {
		names := map[string]bool{}
		for _, e := range w {
			names[asString(e["entity"])] = true
		}
		if names["B"] && names["C"] {
			together = true
		}
	}
	if !together {
		t.Errorf("overlap should keep boundary entities B and C in a common window; windows=%v", windows)
	}
}

func TestResolveAndLinkRelationsLinksAndDedups(t *testing.T) {
	entities := []map[string]any{
		{"entity_id": "7_ent_1", "entity": "Acme Corporation", "aliases": []string{"Acme"}},
		{"entity_id": "7_ent_2", "entity": "Globex"},
	}
	idx := buildEntityResolutionIndex(entities)
	relations := []map[string]any{
		{"subject": "Acme", "predicate": "supplies", "object": "Globex"},
		// Duplicate after resolution (Acme -> 7_ent_1), should be dropped.
		{"subject": "Acme Corporation", "predicate": "supplies", "object": "Globex"},
	}
	got, outEntities := resolveAndLinkRelations(relations, entities, idx, 7, len(entities)+1, "2026-06-13T00:00:00Z")

	if len(got) != 1 {
		t.Fatalf("expected 1 deduped relation, got %d", len(got))
	}
	if got[0]["subject_entity_id"] != "7_ent_1" {
		t.Errorf("subject_entity_id = %v, want 7_ent_1", got[0]["subject_entity_id"])
	}
	if got[0]["object_entity_id"] != "7_ent_2" {
		t.Errorf("object_entity_id = %v, want 7_ent_2", got[0]["object_entity_id"])
	}
	if len(outEntities) != 2 {
		t.Errorf("no provisional entity should be created, got %d entities", len(outEntities))
	}
}

func TestResolveAndLinkRelationsGroundsSpansFromEndpoints(t *testing.T) {
	entities := []map[string]any{
		{"entity_id": "9_ent_1", "entity": "Acme", "line_spans": []string{"10-12"}},
		{"entity_id": "9_ent_2", "entity": "Globex", "line_spans": []string{"40"}},
	}
	idx := buildEntityResolutionIndex(entities)
	relations := []map[string]any{
		// No line_spans of its own (Phase 2 window input lacks line numbers).
		{"subject": "Acme", "predicate": "supplies", "object": "Globex"},
	}
	got, _ := resolveAndLinkRelations(relations, entities, idx, 9, len(entities)+1, "2026-06-13T00:00:00Z")
	if len(got) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(got))
	}
	spans := toStringSlice(got[0]["line_spans"])
	if len(spans) != 2 || spans[0] != "10-12" || spans[1] != "40" {
		t.Errorf("relation line_spans = %v, want union of endpoints [10-12 40]", spans)
	}
}

func TestBuildRelationWindowInputText(t *testing.T) {
	window := []map[string]any{
		{
			"entity_id":      "7_ent_1",
			"entity":         "Acme Corporation",
			"entity_type":    "org",
			"aliases":        []string{"Acme"},
			"entity_context": "Summary: vendors.\nAcme Corporation supplies Globex.",
		},
		{
			"entity_id": "7_ent_2",
			"entity":    "Globex",
		},
	}
	out := buildRelationWindowInputText(window)
	for _, want := range []string{"7_ent_1", "Acme Corporation", "(org)", "aliases: Acme", "Globex", "supplies Globex"} {
		if !strings.Contains(out, want) {
			t.Errorf("window input text missing %q\n---\n%s", want, out)
		}
	}
}

func TestResolveAndLinkRelationsCreatesProvisionalEntity(t *testing.T) {
	entities := []map[string]any{
		{"entity_id": "7_ent_1", "entity": "Acme"},
	}
	idx := buildEntityResolutionIndex(entities)
	relations := []map[string]any{
		{"subject": "Acme", "predicate": "acquires", "object": "Initech"},
	}
	got, outEntities := resolveAndLinkRelations(relations, entities, idx, 7, len(entities)+1, "2026-06-13T00:00:00Z")

	if len(got) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(got))
	}
	if len(outEntities) != 2 {
		t.Fatalf("expected a provisional entity to be appended, got %d", len(outEntities))
	}
	prov := outEntities[1]
	if prov["entity"] != "Initech" {
		t.Errorf("provisional entity name = %v, want Initech", prov["entity"])
	}
	if prov["entity_status"] != entityStatusProvisional {
		t.Errorf("entity_status = %v, want %q", prov["entity_status"], entityStatusProvisional)
	}
	if got[0]["object_entity_id"] != prov["entity_id"] {
		t.Errorf("relation object should link to provisional id %v, got %v", prov["entity_id"], got[0]["object_entity_id"])
	}
}

func TestResolveAndLinkRelationsBackfillsProvisionalEntitySpans(t *testing.T) {
	entities := []map[string]any{
		{"entity_id": "7_ent_1", "entity": "Acme", "line_spans": []string{"10-12"}},
	}
	idx := buildEntityResolutionIndex(entities)
	relations := []map[string]any{
		// Object "Initech" is unresolved -> provisional; relation is grounded by Acme.
		{"subject": "Acme", "predicate": "acquires", "object": "Initech"},
	}
	_, outEntities := resolveAndLinkRelations(relations, entities, idx, 7, len(entities)+1, "2026-06-13T00:00:00Z")
	if len(outEntities) != 2 {
		t.Fatalf("expected a provisional entity, got %d", len(outEntities))
	}
	prov := outEntities[1]
	spans := toStringSlice(prov["line_spans"])
	if len(spans) != 1 || spans[0] != "10-12" {
		t.Errorf("provisional entity line_spans = %v, want [10-12] backfilled from the relation", spans)
	}
}
