package docprocessing

import (
	"strings"
	"testing"
)

func TestResolveProductionPipelineRuleMatchNameNoRulesInstalled(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRules(nil) })

	name, pipeline, matched, err := resolveProductionPipelineRuleMatchName(ProductionRoutingFacets{InputDocType: "pdf"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if matched || name != "" || pipeline != "" {
		t.Fatalf("expected no match, got name=%q pipeline=%q matched=%v", name, pipeline, matched)
	}
}

func TestResolveProductionPipelineRuleMatchNameWildcardAndSpecificFields(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRules(nil) })
	SetProductionPipelineRules([]ProductionPipelineRule{
		{Name: "pdf-any-language", Priority: 1, MatchInputDocType: "pdf", PipelineName: "narrative_default"},
	})

	name, pipeline, matched, err := resolveProductionPipelineRuleMatchName(ProductionRoutingFacets{InputDocType: "pdf", SourceLanguage: "zh"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !matched || name != "pdf-any-language" || pipeline != "narrative_default" {
		t.Fatalf("name=%q pipeline=%q matched=%v", name, pipeline, matched)
	}

	// A non-matching doc type should not match the wildcard-language rule.
	_, _, matched, err = resolveProductionPipelineRuleMatchName(ProductionRoutingFacets{InputDocType: "docx", SourceLanguage: "zh"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if matched {
		t.Fatal("expected no match for a different doc type")
	}
}

func TestResolveProductionPipelineRuleMatchNamePicksHighestPriority(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRules(nil) })
	SetProductionPipelineRules([]ProductionPipelineRule{
		{Name: "pdf-general", Priority: 1, MatchInputDocType: "pdf", PipelineName: "narrative_default"},
		{Name: "pdf-zh-specific", Priority: 10, MatchInputDocType: "pdf", MatchSourceLanguage: "zh", PipelineName: "regulated_reference"},
	})

	name, pipeline, matched, err := resolveProductionPipelineRuleMatchName(ProductionRoutingFacets{InputDocType: "pdf", SourceLanguage: "zh"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !matched || name != "pdf-zh-specific" || pipeline != "regulated_reference" {
		t.Fatalf("name=%q pipeline=%q matched=%v", name, pipeline, matched)
	}
}

func TestResolveProductionPipelineRuleMatchNameTieSamePipelineIsNotAConflict(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRules(nil) })
	SetProductionPipelineRules([]ProductionPipelineRule{
		{Name: "rule-a", Priority: 5, MatchInputDocType: "pdf", PipelineName: "narrative_default"},
		{Name: "rule-b", Priority: 5, MatchSourceLanguage: "en", PipelineName: "narrative_default"},
	})

	name, pipeline, matched, err := resolveProductionPipelineRuleMatchName(ProductionRoutingFacets{InputDocType: "pdf", SourceLanguage: "en"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !matched || pipeline != "narrative_default" {
		t.Fatalf("name=%q pipeline=%q matched=%v", name, pipeline, matched)
	}
}

func TestResolveProductionPipelineRuleMatchNameTieDifferentPipelineIsBlockingConflict(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRules(nil) })
	SetProductionPipelineRules([]ProductionPipelineRule{
		{Name: "rule-a", Priority: 5, MatchInputDocType: "pdf", PipelineName: "narrative_default"},
		{Name: "rule-b", Priority: 5, MatchSourceLanguage: "en", PipelineName: "regulated_reference"},
	})

	_, _, _, err := resolveProductionPipelineRuleMatchName(ProductionRoutingFacets{InputDocType: "pdf", SourceLanguage: "en"})
	if err == nil {
		t.Fatal("expected a blocking conflict error")
	}
	if !strings.Contains(err.Error(), "rule-a") || !strings.Contains(err.Error(), "rule-b") {
		t.Fatalf("err=%v, want both rule names named", err)
	}
}

func TestResolveProductionPipelineBindingCanonicalBindingSitsBetweenRequestAndStoreBinding(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil); SetProductionPipelineBindings(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "legacy_default", LegacyEquivalent: true},
		{Name: "store_default"},
		{Name: "binding_selected"},
	})
	SetProductionPipelineBindings([]PipelineBinding{
		mustLegacyBinding(t, "pdf-binding", "binding_selected", 1, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"}),
	})

	// Explicit request still wins over a matching canonical binding.
	got, err := ResolveProductionPipelineBinding(ProductionPlanFacts{
		RequestedPipeline:  "legacy_default",
		StoreBoundPipeline: "store_default",
		RoutingFacets:      ProductionRoutingFacets{InputDocType: "pdf"},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got.Source != "explicit_request" || got.SelectedPipeline != "legacy_default" {
		t.Fatalf("got=%#v, want explicit_request/legacy_default", got)
	}

	// No explicit request: a matching canonical binding beats the store binding.
	got, err = ResolveProductionPipelineBinding(ProductionPlanFacts{
		StoreBoundPipeline: "store_default",
		RoutingFacets:      ProductionRoutingFacets{InputDocType: "pdf"},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got.Source != "conditional_binding" || got.SelectedPipeline != "binding_selected" || got.RuleName != "pdf-binding" {
		t.Fatalf("got=%#v, want conditional_binding/binding_selected/pdf-binding", got)
	}

	// No explicit request, no matching rule: falls through to store binding.
	got, err = ResolveProductionPipelineBinding(ProductionPlanFacts{
		StoreBoundPipeline: "store_default",
		RoutingFacets:      ProductionRoutingFacets{InputDocType: "docx"},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got.Source != "knowledge_store_binding" || got.SelectedPipeline != "store_default" {
		t.Fatalf("got=%#v, want knowledge_store_binding/store_default", got)
	}
}

func TestResolveProductionPipelineBindingRejectsCanonicalBindingConflict(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil); SetProductionPipelineBindings(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "legacy_default", LegacyEquivalent: true},
		{Name: "pipeline_a"},
		{Name: "pipeline_b"},
	})
	SetProductionPipelineBindings([]PipelineBinding{
		mustLegacyBinding(t, "binding-a", "pipeline_a", 1, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"}),
		mustLegacyBinding(t, "binding-b", "pipeline_b", 1, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchSourceLanguage: "en"}),
	})

	_, err := ResolveProductionPipelineBinding(ProductionPlanFacts{
		RoutingFacets: ProductionRoutingFacets{InputDocType: "pdf", SourceLanguage: "en"},
	})
	if err == nil {
		t.Fatal("expected blocking conflict error to propagate through ResolveProductionPipelineBinding")
	}
}

func TestResolveProductionPipelineBindingRejectsCanonicalBindingSelectingUnknownPipeline(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil); SetProductionPipelineBindings(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{{Name: "legacy_default", LegacyEquivalent: true}})
	SetProductionPipelineBindings([]PipelineBinding{
		mustLegacyBinding(t, "bad-binding", "does_not_exist", 1, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"}),
	})

	_, err := ResolveProductionPipelineBinding(ProductionPlanFacts{
		RoutingFacets: ProductionRoutingFacets{InputDocType: "pdf"},
	})
	if err == nil || !strings.Contains(err.Error(), "does_not_exist") {
		t.Fatalf("err=%v, want an error naming the unknown pipeline", err)
	}
}
