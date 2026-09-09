package productreviews

import "testing"

const vocabTOML = `
[budgets]
max_depth = 4

[grounding]
object_embedding_min = 0.8

[aspects.storage]
name_en = "Storage"
name_zh_cn = "贮存"
desc_en = "Storage limits."
relation_types = ["storage_requirement"]

[aspects.emc]
name_en = "EMC"
match_mode = "lexical"
`

// Scenario: Aspect labels are localized (spec: Aspect nodes from configured
// vocabulary) + Aspect vocabulary is served with locale fallback (spec: Read
// and export endpoints).
func TestAspectVocabularyLocale(t *testing.T) {
	cfg, err := ParseConfig([]byte(vocabTOML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	zh := cfg.AspectVocabulary("zh_cn")
	if len(zh) != 2 {
		t.Fatalf("got %d aspects, want 2", len(zh))
	}
	if zh[0].AspectKey != "storage" || zh[1].AspectKey != "emc" {
		t.Fatalf("aspect order not preserved: %+v", zh)
	}
	if zh[0].Name != "贮存" {
		t.Errorf("storage zh name = %q, want 贮存", zh[0].Name)
	}
	if zh[1].Name != "EMC" {
		t.Errorf("emc has no zh name; want English fallback %q, got %q", "EMC", zh[1].Name)
	}
	if zh[0].MatchMode != MatchModeJoin || zh[1].MatchMode != MatchModeLexical {
		t.Errorf("match modes = %q / %q, want join / lexical", zh[0].MatchMode, zh[1].MatchMode)
	}

	en := cfg.AspectVocabulary("")
	if en[0].Name != "Storage" || en[0].Description != "Storage limits." {
		t.Errorf("english resolution wrong: %+v", en[0])
	}
	if len(en[0].RelationTypes) != 1 || en[0].RelationTypes[0] != "storage_requirement" {
		t.Errorf("storage relation types = %v, want [storage_requirement]", en[0].RelationTypes)
	}
}

// Omitted budget values fall back to the built-in defaults.
func TestBudgetDefaults(t *testing.T) {
	cfg, err := ParseConfig([]byte(vocabTOML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Budgets.MaxDepth != 4 {
		t.Errorf("MaxDepth = %d, want 4 (from file)", cfg.Budgets.MaxDepth)
	}
	if cfg.Budgets.MaxNodes != defaultMaxNodes || cfg.Budgets.MaxResults != defaultMaxResults {
		t.Errorf("defaults not applied: %+v", cfg.Budgets)
	}
}
