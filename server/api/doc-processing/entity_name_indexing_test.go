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
		{EntityID: "100_ent_1", Entity: "艾克米公司", EntityEN: "Acme Corporation"},
		{EntityID: "100_ent_2", Entity: "Fallback Name"},
		{EntityID: "", EntityEN: "Missing ID"},
	}

	conns := buildEntityNameGraphConnections(100, rows)
	if len(conns) != 2 {
		t.Fatalf("want 2 connections, got %d: %+v", len(conns), conns)
	}

	if conns[0].SourceType != artifactTypeEntityName ||
		conns[0].SourceID != "acme_corporation" ||
		conns[0].TargetType != searchArtifactEntity ||
		conns[0].TargetID != "100_ent_1" ||
		conns[0].RelationName != RelationHasInstance ||
		conns[0].RelationMethod != RelationMethodEntityName {
		t.Errorf("first connection wrong: %+v", conns[0])
	}

	if conns[1].SourceID != "fallback_name" || conns[1].TargetID != "100_ent_2" {
		t.Errorf("fallback connection wrong: %+v", conns[1])
	}
}

func TestEntityNameCategoryRequests(t *testing.T) {
	rows := []entityNameGraphRow{
		{EntityID: "100_ent_1", Entity: "农村生活垃圾分类处理规范", EntityEN: "Rural Domestic Waste Classification and Treatment Specification"},
		{EntityID: "100_ent_2", Entity: "设施配置"},
		{EntityID: "100_ent_3"},
	}

	reqs := entityNameCategoryRequests(rows)
	if len(reqs) != 2 {
		t.Fatalf("want 2 requests, got %d: %+v", len(reqs), reqs)
	}
	if reqs[0].RawKey != "Rural Domestic Waste Classification and Treatment Specification" {
		t.Errorf("first RawKey = %q", reqs[0].RawKey)
	}
	if reqs[1].RawKey != "设施配置" {
		t.Errorf("second RawKey = %q", reqs[1].RawKey)
	}
	if reqs[0].Evidence["artifact_kind"] != entityNameCategoryType ||
		reqs[0].Evidence["entity_id"] != "100_ent_1" {
		t.Errorf("evidence wrong: %+v", reqs[0].Evidence)
	}
}
