package docprocessing

import "testing"

// ADR 2026061702 DR3 — the standalone, line-anchored link step. Endpoints are
// matched to independently-extracted entities by name first (precision), then by
// unambiguous line-span overlap (recovers endpoints whose surface form differs
// from the entity name), and minted provisional when neither resolves.

func relForLink(subj, pred, obj string, sl, pl, ol []string) map[string]any {
	return map[string]any{
		"subject": subj, "predicate": pred, "object": obj,
		"subject_lines": sl, "predicate_lines": pl, "object_lines": ol,
	}
}

func TestLinkRelationEndpoints_NameMatch(t *testing.T) {
	entities := []map[string]any{
		{"entity_id": "r_ent_1", "entity": "Acme Corporation", "aliases": []string{"Acme"}, "line_spans": []string{"10-12"}},
		{"entity_id": "r_ent_2", "entity": "Globex", "line_spans": []string{"30"}},
	}
	rels := []map[string]any{
		relForLink("Acme", "supplies", "Globex", []string{"40"}, []string{"40"}, []string{"41"}),
	}
	linked, outEntities := linkRelationEndpoints(rels, entities, 99, len(entities)+1, "t0")

	if len(outEntities) != 2 {
		t.Fatalf("no provisional should be minted, got %d entities", len(outEntities))
	}
	if linked[0]["subject_entity_id"] != "r_ent_1" {
		t.Errorf("subject_entity_id = %v, want r_ent_1 (alias match)", linked[0]["subject_entity_id"])
	}
	if linked[0]["object_entity_id"] != "r_ent_2" {
		t.Errorf("object_entity_id = %v, want r_ent_2", linked[0]["object_entity_id"])
	}
}

func TestLinkRelationEndpoints_PositionalRecovery(t *testing.T) {
	// Subject surface ("the device") is not an entity name, but its lines overlap
	// exactly one entity's span -> recovered positionally (the line-anchor win).
	entities := []map[string]any{
		{"entity_id": "r_ent_1", "entity": "temperature monitoring device", "line_spans": []string{"20-24"}},
		{"entity_id": "r_ent_2", "entity": "excursion alarm", "line_spans": []string{"60"}},
	}
	rels := []map[string]any{
		relForLink("the device", "triggers", "excursion alarm", []string{"22"}, []string{"22"}, []string{"60"}),
	}
	linked, outEntities := linkRelationEndpoints(rels, entities, 99, len(entities)+1, "t0")

	if len(outEntities) != 2 {
		t.Fatalf("no provisional expected (positional recovery), got %d", len(outEntities))
	}
	if linked[0]["subject_entity_id"] != "r_ent_1" {
		t.Errorf("subject_entity_id = %v, want r_ent_1 (positional)", linked[0]["subject_entity_id"])
	}
}

func TestLinkRelationEndpoints_MintsProvisional(t *testing.T) {
	entities := []map[string]any{
		{"entity_id": "r_ent_1", "entity": "Acme", "line_spans": []string{"10"}},
	}
	rels := []map[string]any{
		relForLink("Acme", "competes_with", "Initech", []string{"10"}, []string{"11"}, []string{"12-13"}),
	}
	linked, outEntities := linkRelationEndpoints(rels, entities, 99, len(entities)+1, "t0")

	if len(outEntities) != 2 {
		t.Fatalf("expected one provisional minted, got %d entities", len(outEntities))
	}
	prov := outEntities[1]
	if prov["entity"] != "Initech" {
		t.Errorf("provisional name = %v, want Initech", prov["entity"])
	}
	if prov["entity_status"] != entityStatusProvisional {
		t.Errorf("entity_status = %v, want %q", prov["entity_status"], entityStatusProvisional)
	}
	// The unmatched endpoint's own line spans seed the provisional (DR3 improvement).
	if spans := toStringSlice(prov["line_spans"]); len(spans) != 1 || spans[0] != "12-13" {
		t.Errorf("provisional line_spans = %v, want [12-13] from object_lines", spans)
	}
	if linked[0]["object_entity_id"] != prov["entity_id"] {
		t.Errorf("object_entity_id = %v, want provisional id %v", linked[0]["object_entity_id"], prov["entity_id"])
	}
}

func TestLinkRelationEndpoints_AmbiguousPositionalStaysUnlinked(t *testing.T) {
	// Two entities overlap the endpoint lines and neither name-matches -> ambiguous,
	// so we do NOT guess; the endpoint becomes provisional (conservative).
	entities := []map[string]any{
		{"entity_id": "r_ent_1", "entity": "Alpha", "line_spans": []string{"5-9"}},
		{"entity_id": "r_ent_2", "entity": "Beta", "line_spans": []string{"7-11"}},
	}
	rels := []map[string]any{
		relForLink("it", "uses", "Gamma", []string{"8"}, []string{"8"}, []string{"50"}),
	}
	linked, outEntities := linkRelationEndpoints(rels, entities, 99, len(entities)+1, "t0")

	// subject "it" is ambiguous-positional + no name match -> provisional;
	// object "Gamma" no overlap, no name -> provisional. So 2 provisionals.
	if len(outEntities) != 4 {
		t.Fatalf("expected 2 provisionals (subject ambiguous, object missing), got %d", len(outEntities))
	}
	if linked[0]["subject_entity_id"] == "r_ent_1" || linked[0]["subject_entity_id"] == "r_ent_2" {
		t.Errorf("ambiguous positional must not pick a real entity, got %v", linked[0]["subject_entity_id"])
	}
}

func TestLinkRelationEndpoints_LineSpansFromPrompt(t *testing.T) {
	entities := []map[string]any{
		{"entity_id": "r_ent_1", "entity": "Acme", "line_spans": []string{"10"}},
		{"entity_id": "r_ent_2", "entity": "Globex", "line_spans": []string{"20"}},
	}
	rels := []map[string]any{
		relForLink("Acme", "supplies", "Globex", []string{"10"}, []string{"15"}, []string{"20"}),
	}
	linked, _ := linkRelationEndpoints(rels, entities, 99, len(entities)+1, "t0")
	spans := toStringSlice(linked[0]["line_spans"])
	// union of subject/predicate/object lines, sorted-unique, taken from the prompt.
	if len(spans) != 3 || spans[0] != "10" || spans[1] != "15" || spans[2] != "20" {
		t.Errorf("relation line_spans = %v, want [10 15 20] from prompt", spans)
	}
}

func TestLinkRelationEndpoints_DedupesTriples(t *testing.T) {
	entities := []map[string]any{
		{"entity_id": "r_ent_1", "entity": "Acme", "line_spans": []string{"10"}},
		{"entity_id": "r_ent_2", "entity": "Globex", "line_spans": []string{"20"}},
	}
	rels := []map[string]any{
		relForLink("Acme", "supplies", "Globex", []string{"10"}, []string{"15"}, []string{"20"}),
		relForLink("Acme", "supplies", "Globex", []string{"40"}, []string{"45"}, []string{"50"}),
	}
	linked, _ := linkRelationEndpoints(rels, entities, 99, len(entities)+1, "t0")
	if len(linked) != 1 {
		t.Fatalf("identical triples should dedup to 1, got %d", len(linked))
	}
}
