package docprocessing

import "testing"

func TestNormalizeEntityNameKey(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"   ":              "",
		"Acme Corporation": "acme_corporation",
		"OpenAI, Inc.":     "openai_inc",
		"  Has  Many  ":    "has_many",
		"FDA/CBER Product": "fda_cber_product",
	}
	for in, want := range cases {
		if got := normalizeEntityNameKey(in); got != want {
			t.Errorf("normalizeEntityNameKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildEntityNameGraphConnections(t *testing.T) {
	rows := []entityNameGraphRow{
		{EntityID: "100_ent_1", Entity: "艾克米公司", EntityEN: "Acme Corporation", Categories: []string{"Company", "Vendor"}},
		{EntityID: "100_ent_2", Entity: "Fallback Name"},
		{EntityID: "", EntityEN: "Missing ID"},
	}
	// "company" resolves; "vendor" is unresolved (absent from the map) and is skipped.
	categories := map[string]resolvedCategory{"company": {ID: 5, Type: "entity", Key: "company"}}

	conns := buildEntityNameGraphConnections(100, rows)
	// ent_1 name edge + ent_1 company edge + ent_2 name edge = 3.
	if len(conns) != 2 {
		t.Fatalf("want 2 name connections, got %d: %+v", len(conns), conns)
	}

	// Edge 1 — name dictionary edge for the first entity.
	if conns[0].SourceType != artifactTypeEntityName ||
		conns[0].SourceID != "acme_corporation" ||
		conns[0].TargetType != searchArtifactEntity ||
		conns[0].TargetID != "100_ent_1" ||
		conns[0].RelationName != RelationHasInstance ||
		conns[0].RelationMethod != RelationMethodEntityName {
		t.Errorf("name connection wrong: %+v", conns[0])
	}

	categoryConns := buildEntityCategoryConnections(100, rows, categories)
	if len(categoryConns) != 1 {
		t.Fatalf("want 1 category connection, got %d: %+v", len(categoryConns), categoryConns)
	}
	if categoryConns[0].SourceType != searchArtifactEntity ||
		categoryConns[0].SourceID != "100_ent_1" ||
		categoryConns[0].TargetType != "entity" ||
		categoryConns[0].TargetID != "company" ||
		categoryConns[0].TargetRecordID != 5 ||
		categoryConns[0].RelationName != RelationBelongTo ||
		categoryConns[0].RelationMethod != RelationMethodCategoryName {
		t.Errorf("category connection wrong: %+v", categoryConns[0])
	}

	// Third entity (no ID) is skipped; second entity's name edge is last.
	if conns[1].SourceID != "fallback_name" || conns[1].TargetID != "100_ent_2" {
		t.Errorf("fallback connection wrong: %+v", conns[1])
	}
}
