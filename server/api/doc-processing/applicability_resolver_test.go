package docprocessing

import (
	"context"
	"errors"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestResolverDecisionRelevantVsMaskedMissingPaths(t *testing.T) {
	// An `all` with an already-false first child masks the second child's
	// missing tier-3 path. Only the unmasked missing path should trigger
	// classification.
	predicates := []semrules.Document{
		{
			Version: 1,
			Expression: semrules.Predicate{
				Kind: "all",
				Items: []semrules.Predicate{
					{Kind: "fact", Path: "document.input_doc_type", Op: "eq", Value: "impossible_value"},
					{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"},
				},
			},
		},
	}
	baseFacts := semrules.FactSet{
		"document.input_doc_type": {Path: "document.input_doc_type", Value: "other", State: semrules.FactKnown},
	}

	// Pass 1: the first child is false, masking the second. No tier-3
	// path should be decision-relevant.
	results, err := evaluateAll(predicates, baseFacts)
	if err != nil {
		t.Fatalf("evaluateAll: %v", err)
	}
	missing := decisionRelevantTier3Paths(results)
	if len(missing) != 0 {
		t.Fatalf("expected no decision-relevant paths when masked, got %v", missing)
	}
}

func TestResolverDecisionRelevantUnmaskedTier3Path(t *testing.T) {
	// When the tier-3 path is the only missing fact and is decision-relevant,
	// the resolver should identify it.
	predicates := []semrules.Document{
		{
			Version: 1,
			Expression: semrules.Predicate{
				Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification",
			},
		},
	}
	baseFacts := semrules.FactSet{
		"document.input_doc_type": {Path: "document.input_doc_type", Value: "standard", State: semrules.FactKnown},
	}

	results, err := evaluateAll(predicates, baseFacts)
	if err != nil {
		t.Fatalf("evaluateAll: %v", err)
	}
	missing := decisionRelevantTier3Paths(results)
	if len(missing) != 1 || missing[0] != "document.doc_kind" {
		t.Fatalf("expected [document.doc_kind], got %v", missing)
	}
}

func TestResolverOneInvocationPerRecordExtractionRun(t *testing.T) {
	// The resolver must invoke the classifier at most once per
	// (record, decision_attempt_id, invocation_id).
	callCount := 0
	ext := &countingExtractor{
		count: &callCount,
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.doc_kind", "value": "product_specification",
					"confidence": 0.9, "evidence": "specs",
				},
			},
		},
	}
	store := &stubFacetStore{}
	classifier := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test",
		ModelName:  "test",
		Vocabulary: testVocabulary(),
		Facets:     store,
	}
	resolver := &ApplicabilityResolver{
		Classifier: classifier,
		Facets:     store,
	}

	predicates := []semrules.Document{
		{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}},
	}
	req := ResolverRequest{
		RecordID:          1,
		DecisionAttemptID: "run-1",
		InvocationID:      "ext-1-1",
		BaseFacts:         semrules.FactSet{},
		Predicates:        predicates,
		DocumentSample:    "test document",
	}

	result, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Classified {
		t.Fatal("expected classifier to be invoked")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 LLM call, got %d", callCount)
	}

	// Second call with same invocation_id should not invoke LLM again.
	result2, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected still 1 LLM call after retry, got %d", callCount)
	}
	if !result2.Classified {
		t.Fatal("expected classified=true on retry")
	}
}

// TestResolveExtractionFactsCollectsActiveRoutingPredicates proves
// ResolveExtractionFacts supplies the resolver the active policy's
// conditional-binding and processor-gate predicates instead of an always-
// empty list. Before this fix, Predicates was never populated, so pass 1
// always evaluated zero predicates, decisionRelevantTier3Paths always
// returned empty, and Resolve returned before ever reaching the classifier
// -- wiring ControlService.Resolver alone would have changed nothing (P5
// review 2026080302 finding P5-2). A registered conditional binding whose
// predicate references the unresolved tier-3 document.doc_kind facet must
// trigger exactly one classifier invocation.
func TestResolveExtractionFactsCollectsActiveRoutingPredicates(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineBindings(nil); SetProductionPipelineGates(nil) })
	SetProductionPipelineBindings([]PipelineBinding{
		{
			BindingKind: PipelineBindingKindConditional, PipelineName: "regulated_reference", Active: true,
			Predicate: semrules.Document{Version: 1, Expression: semrules.Predicate{
				Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "regulated_reference",
			}},
		},
	})

	callCount := 0
	ext := &countingExtractor{
		count: &callCount,
		result: map[string]any{
			"classifications": []any{
				map[string]any{"path": "document.doc_kind", "value": "regulated_reference", "confidence": 0.9, "evidence": "x"},
			},
		},
	}
	store := &stubFacetStore{}
	classifier := &DocumentClassifier{Extractor: ext, PromptText: "test", ModelName: "test", Vocabulary: testVocabulary(), Facets: store}
	resolver := &ApplicabilityResolver{Classifier: classifier, Facets: store}

	_, result, err := resolver.ResolveExtractionFacts(context.Background(), ProductionPlanFacts{InputDocType: "pdf"}, 1, 1, "sample document text")
	if err != nil {
		t.Fatalf("ResolveExtractionFacts: %v", err)
	}
	if result == nil || !result.Classified {
		t.Fatalf("expected the classifier to be invoked because the active binding's predicate references document.doc_kind, result=%#v", result)
	}
	if callCount != 1 {
		t.Fatalf("callCount=%d, want 1", callCount)
	}
}

// TestResolveExtractionFactsNoClassifierCallForResolvedTierOneTwoPredicate
// proves a predicate referencing only already-resolved tier-1/2 paths never
// triggers a classifier call.
func TestResolveExtractionFactsNoClassifierCallForResolvedTierOneTwoPredicate(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineBindings(nil) })
	SetProductionPipelineBindings([]PipelineBinding{
		{
			BindingKind: PipelineBindingKindConditional, PipelineName: "pdf_pipeline", Active: true,
			Predicate: semrules.Document{Version: 1, Expression: semrules.Predicate{
				Kind: "fact", Path: "document.input_doc_type", Op: "eq", Value: "pdf",
			}},
		},
	})

	callCount := 0
	ext := &countingExtractor{count: &callCount, result: map[string]any{}}
	classifier := &DocumentClassifier{Extractor: ext, PromptText: "test", ModelName: "test", Vocabulary: testVocabulary(), Facets: &stubFacetStore{}}
	resolver := &ApplicabilityResolver{Classifier: classifier, Facets: &stubFacetStore{}}

	_, result, err := resolver.ResolveExtractionFacts(context.Background(), ProductionPlanFacts{InputDocType: "pdf"}, 1, 1, "sample")
	if err != nil {
		t.Fatalf("ResolveExtractionFacts: %v", err)
	}
	if result != nil && result.Classified {
		t.Fatalf("expected no classifier invocation for an already-resolved tier-1/2 predicate, result=%#v", result)
	}
	if callCount != 0 {
		t.Fatalf("callCount=%d, want 0", callCount)
	}
}

func TestResolverClassifierFailurePreservesIndeterminate(t *testing.T) {
	// When the classifier fails, the resolver must return base facts
	// unchanged (preserving indeterminate for missing tier-3 paths).
	ext := &stubExtractor{err: errors.New("LLM timeout")}
	classifier := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test",
		ModelName:  "test",
		Vocabulary: testVocabulary(),
		Facets:     &stubFacetStore{},
	}
	resolver := &ApplicabilityResolver{
		Classifier: classifier,
		Facets:     &stubFacetStore{},
	}

	predicates := []semrules.Document{
		{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}},
	}
	baseFacts := semrules.FactSet{
		"document.input_doc_type": {Path: "document.input_doc_type", Value: "standard", State: semrules.FactKnown},
	}
	req := ResolverRequest{
		RecordID:          1,
		DecisionAttemptID: "run-1",
		InvocationID:      "ext-1-1",
		BaseFacts:         baseFacts,
		Predicates:        predicates,
		DocumentSample:    "test",
	}

	result, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Classified {
		t.Fatal("expected classified=false on classifier failure")
	}
	// Base facts should be unchanged.
	if _, ok := result.Facts["document.doc_kind"]; ok {
		t.Fatal("doc_kind should not be in facts after classifier failure")
	}
	if len(result.DecisionRelevant) != 1 || result.DecisionRelevant[0] != "document.doc_kind" {
		t.Fatalf("expected doc_kind in decision-relevant, got %v", result.DecisionRelevant)
	}
}

func TestResolverNoClassifierWhenNoMissingTier3Paths(t *testing.T) {
	// When all facts are known, the classifier must never be invoked.
	callCount := 0
	ext := &countingExtractor{count: &callCount, result: map[string]any{"classifications": []any{}}}
	classifier := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test",
		ModelName:  "test",
		Vocabulary: testVocabulary(),
		Facets:     &stubFacetStore{},
	}
	resolver := &ApplicabilityResolver{
		Classifier: classifier,
		Facets:     &stubFacetStore{},
	}

	predicates := []semrules.Document{
		{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}},
	}
	baseFacts := semrules.FactSet{
		"document.doc_kind": {Path: "document.doc_kind", Value: "product_specification", State: semrules.FactKnown},
	}
	req := ResolverRequest{
		RecordID:          1,
		DecisionAttemptID: "run-1",
		InvocationID:      "ext-1-1",
		BaseFacts:         baseFacts,
		Predicates:        predicates,
	}

	result, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Classified {
		t.Fatal("classifier should not have been invoked")
	}
	if callCount != 0 {
		t.Fatalf("expected 0 LLM calls, got %d", callCount)
	}
}

func TestResolverNilClassifierReturnsDecisionRelevant(t *testing.T) {
	// When no classifier is configured, the resolver returns base facts
	// and the decision-relevant paths for the caller to handle.
	resolver := &ApplicabilityResolver{}
	predicates := []semrules.Document{
		{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}},
	}
	req := ResolverRequest{
		RecordID:          1,
		DecisionAttemptID: "run-1",
		InvocationID:      "ext-1-1",
		BaseFacts:         semrules.FactSet{},
		Predicates:        predicates,
	}

	result, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Classified {
		t.Fatal("should not be classified with nil classifier")
	}
	if len(result.DecisionRelevant) != 1 || result.DecisionRelevant[0] != "document.doc_kind" {
		t.Fatalf("expected doc_kind decision-relevant, got %v", result.DecisionRelevant)
	}
}

func TestResolverEnrichedFactsDoNotOverwriteKnown(t *testing.T) {
	// A classifier observation must not overwrite an existing known fact
	// (deterministic > metadata > classifier reduction order).
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.doc_kind", "value": "test_report",
					"confidence": 0.9, "evidence": "test data",
				},
			},
		},
	}
	classifier := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test",
		ModelName:  "test",
		Vocabulary: testVocabulary(),
		Facets:     &stubFacetStore{},
	}
	resolver := &ApplicabilityResolver{
		Classifier: classifier,
		Facets:     &stubFacetStore{},
	}

	// doc_kind is already known as product_specification.
	predicates := []semrules.Document{
		{Version: 1, Expression: semrules.Predicate{
			Kind: "all",
			Items: []semrules.Predicate{
				{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"},
				{Kind: "fact", Path: "document.domain", Op: "eq", Value: "medical_devices"},
			},
		}},
	}
	baseFacts := semrules.FactSet{
		"document.doc_kind": {Path: "document.doc_kind", Value: "product_specification", State: semrules.FactKnown},
	}
	req := ResolverRequest{
		RecordID:          1,
		DecisionAttemptID: "run-1",
		InvocationID:      "ext-1-1",
		BaseFacts:         baseFacts,
		Predicates:        predicates,
		DocumentSample:    "test",
	}

	result, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// doc_kind must remain product_specification, not overwritten to test_report.
	if result.Facts["document.doc_kind"].Value != "product_specification" {
		t.Fatalf("doc_kind was overwritten: %v", result.Facts["document.doc_kind"].Value)
	}
}

func TestResolverValidationErrors(t *testing.T) {
	resolver := &ApplicabilityResolver{}
	tests := []struct {
		name string
		req  ResolverRequest
	}{
		{"missing record_id", ResolverRequest{DecisionAttemptID: "a", InvocationID: "b"}},
		{"missing decision_attempt_id", ResolverRequest{RecordID: 1, InvocationID: "b"}},
		{"missing invocation_id", ResolverRequest{RecordID: 1, DecisionAttemptID: "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.Resolve(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// countingExtractor wraps stubExtractor and counts LLM calls.
type countingExtractor struct {
	count  *int
	result map[string]any
	err    error
}

func (c *countingExtractor) ExtractJSON(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	*c.count++
	return c.result, c.err
}
