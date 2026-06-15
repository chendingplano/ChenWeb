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
	categoryIDs := map[string]int64{"company": 5}

	conns := buildEntityNameGraphConnections(100, rows, categoryIDs)
	// ent_1 name edge + ent_1 company edge + ent_2 name edge = 3.
	if len(conns) != 3 {
		t.Fatalf("want 3 connections, got %d: %+v", len(conns), conns)
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

	// Edge 2 — belong-to-category edge for the resolved "company" category.
	if conns[1].SourceType != searchArtifactEntity ||
		conns[1].SourceID != "100_ent_1" ||
		conns[1].TargetType != artifactTypeCategory ||
		conns[1].TargetID != "5" ||
		conns[1].RelationName != RelationBelongToCategory ||
		conns[1].RelationMethod != RelationMethodEntityName ||
		conns[1].ExtraInfo["category_key"] != "company" {
		t.Errorf("category connection wrong: %+v", conns[1])
	}

	// Third entity (no ID) is skipped; second entity's name edge is last.
	if conns[2].SourceID != "fallback_name" || conns[2].TargetID != "100_ent_2" {
		t.Errorf("fallback connection wrong: %+v", conns[2])
	}
}
