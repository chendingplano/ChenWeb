package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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

// TestResolveExtractionFactsAttemptKeyIsStablePerAttemptDistinctAcrossAttempts
// proves ResolveExtractionFacts derives DecisionAttemptID/InvocationID from
// an attempt key independent of any run id -- runID=0 at the control.go call
// site (no kb.doc_process_runs row exists yet when routing must be decided)
// previously collapsed every invocation for a record to the same
// invocation_id, permanently short-circuiting later genuine reprocessing
// attempts via the stable-retry path (P5 review 2026080302 finding P5-2). A
// retry using the SAME attempt key must reuse the classifier's cached
// observation (no second LLM call); a genuinely new attempt (different key)
// must invoke the classifier again.
func TestResolveExtractionFactsAttemptKeyIsStablePerAttemptDistinctAcrossAttempts(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineBindings(nil) })
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

	planFacts := ProductionPlanFacts{InputDocType: "pdf"}
	_, r1, err := resolver.ResolveExtractionFacts(context.Background(), planFacts, 1, "line-file-aaa.txt", "sample")
	if err != nil {
		t.Fatalf("ResolveExtractionFacts (attempt 1): %v", err)
	}
	if !r1.Classified || callCount != 1 {
		t.Fatalf("attempt 1: Classified=%v callCount=%d, want true/1", r1.Classified, callCount)
	}

	// Same attempt key (a retry of the same event/message): must not
	// invoke the classifier again.
	_, r2, err := resolver.ResolveExtractionFacts(context.Background(), planFacts, 1, "line-file-aaa.txt", "sample")
	if err != nil {
		t.Fatalf("ResolveExtractionFacts (retry): %v", err)
	}
	if !r2.Classified || callCount != 1 {
		t.Fatalf("retry: Classified=%v callCount=%d, want true/1 (stable retry must not re-invoke)", r2.Classified, callCount)
	}

	// Different attempt key (a genuinely new reprocessing attempt): must
	// invoke the classifier again, not silently reuse the stale observation.
	_, r3, err := resolver.ResolveExtractionFacts(context.Background(), planFacts, 1, "line-file-bbb.txt", "sample")
	if err != nil {
		t.Fatalf("ResolveExtractionFacts (attempt 2): %v", err)
	}
	if !r3.Classified || callCount != 2 {
		t.Fatalf("attempt 2: Classified=%v callCount=%d, want true/2 (a new attempt must not reuse a prior attempt's stale observation)", r3.Classified, callCount)
	}
}

// TestVocabularyReleaseSQLStoreReadsActiveDocumentAuthorityRelease proves
// the production VocabularyReleaseStore reads the active document-authority
// module release id from kb.ontology_active_releases -- the same table
// ProfileStore.LoadReleasedProfiles already pins releases from -- and
// treats "no active release yet" (sql.ErrNoRows) as 0, not an error.
func TestVocabularyReleaseSQLStoreReadsActiveDocumentAuthorityRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT release_id FROM kb.ontology_active_releases WHERE module_id = 'document-authority' AND deactivated_at IS NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"release_id"}).AddRow(int64(7)))

	got, err := (VocabularyReleaseSQLStore{DB: db}).ActiveDocumentAuthorityReleaseID(context.Background())
	if err != nil {
		t.Fatalf("ActiveDocumentAuthorityReleaseID: %v", err)
	}
	if got != 7 {
		t.Fatalf("got=%d, want 7", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestVocabularyReleaseSQLStoreNoActiveReleaseReturnsZero(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT release_id FROM kb.ontology_active_releases WHERE module_id = 'document-authority' AND deactivated_at IS NULL")).
		WillReturnError(sql.ErrNoRows)

	got, err := (VocabularyReleaseSQLStore{DB: db}).ActiveDocumentAuthorityReleaseID(context.Background())
	if err != nil {
		t.Fatalf("expected no error for sql.ErrNoRows, got: %v", err)
	}
	if got != 0 {
		t.Fatalf("got=%d, want 0", got)
	}
}

func TestVocabularyReleaseSQLStoreRejectsNilDB(t *testing.T) {
	if _, err := (VocabularyReleaseSQLStore{}).ActiveDocumentAuthorityReleaseID(context.Background()); err == nil {
		t.Fatal("expected error for nil db")
	}
}

// stubVocabularyReleaseStore returns a fixed release id for testing.
type stubVocabularyReleaseStore struct {
	releaseID int64
	err       error
}

func (s stubVocabularyReleaseStore) ActiveDocumentAuthorityReleaseID(context.Context) (int64, error) {
	return s.releaseID, s.err
}

// TestResolveExtractionFactsResolvesVocabularyReleaseID proves the resolver
// pins the active document-authority module release id on classifier
// observations instead of always hardcoding 0 -- "no governed-vocabulary
// source exists" was the previous state (P5 review 2026080302 finding
// P5-2).
func TestResolveExtractionFactsResolvesVocabularyReleaseID(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineBindings(nil) })
	SetProductionPipelineBindings([]PipelineBinding{
		{
			BindingKind: PipelineBindingKindConditional, PipelineName: "regulated_reference", Active: true,
			Predicate: semrules.Document{Version: 1, Expression: semrules.Predicate{
				Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "regulated_reference",
			}},
		},
	})

	ext := &countingExtractor{
		count: new(int),
		result: map[string]any{
			"classifications": []any{
				map[string]any{"path": "document.doc_kind", "value": "regulated_reference", "confidence": 0.9, "evidence": "x"},
			},
		},
	}
	store := &stubFacetStore{}
	classifier := &DocumentClassifier{Extractor: ext, PromptText: "test", ModelName: "test", Vocabulary: testVocabulary(), Facets: store}
	resolver := &ApplicabilityResolver{Classifier: classifier, Facets: store, VocabularyReleases: stubVocabularyReleaseStore{releaseID: 42}}

	_, result, err := resolver.ResolveExtractionFacts(context.Background(), ProductionPlanFacts{InputDocType: "pdf"}, 1, "attempt-1", "sample")
	if err != nil {
		t.Fatalf("ResolveExtractionFacts: %v", err)
	}
	if result == nil || !result.Classified {
		t.Fatalf("expected the classifier to be invoked, result=%#v", result)
	}
	if len(store.inserted) != 1 || store.inserted[0].VocabularyReleaseID != 42 {
		t.Fatalf("inserted observations = %#v, want VocabularyReleaseID=42", store.inserted)
	}
}

// TestResolveExtractionFactsVocabularyReleaseIDStaysZeroWhenNoResolver
// proves a nil VocabularyReleases (the default) preserves today's behavior
// exactly -- VocabularyReleaseID stays 0.
func TestResolveExtractionFactsVocabularyReleaseIDStaysZeroWhenNoResolver(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineBindings(nil) })
	SetProductionPipelineBindings([]PipelineBinding{
		{
			BindingKind: PipelineBindingKindConditional, PipelineName: "regulated_reference", Active: true,
			Predicate: semrules.Document{Version: 1, Expression: semrules.Predicate{
				Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "regulated_reference",
			}},
		},
	})
	ext := &countingExtractor{
		count: new(int),
		result: map[string]any{
			"classifications": []any{
				map[string]any{"path": "document.doc_kind", "value": "regulated_reference", "confidence": 0.9, "evidence": "x"},
			},
		},
	}
	store := &stubFacetStore{}
	classifier := &DocumentClassifier{Extractor: ext, PromptText: "test", ModelName: "test", Vocabulary: testVocabulary(), Facets: store}
	resolver := &ApplicabilityResolver{Classifier: classifier, Facets: store}

	_, _, err := resolver.ResolveExtractionFacts(context.Background(), ProductionPlanFacts{InputDocType: "pdf"}, 1, "attempt-1", "sample")
	if err != nil {
		t.Fatalf("ResolveExtractionFacts: %v", err)
	}
	if len(store.inserted) != 1 || store.inserted[0].VocabularyReleaseID != 0 {
		t.Fatalf("inserted observations = %#v, want VocabularyReleaseID=0", store.inserted)
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

	_, result, err := resolver.ResolveExtractionFacts(context.Background(), ProductionPlanFacts{InputDocType: "pdf"}, 1, "attempt-1", "sample document text")
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

	_, result, err := resolver.ResolveExtractionFacts(context.Background(), ProductionPlanFacts{InputDocType: "pdf"}, 1, "attempt-1", "sample")
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

// TestResolveReviewFactsEnrichesWithClassification proves the review-side
// entry point performs the same two-pass orchestration as extraction routing:
// a decision-relevant missing tier-3 path triggers the classifier, whose
// validated observation is persisted (only as a facet observation) and merged
// into the returned fact set the selector evaluates against.
func TestResolveReviewFactsEnrichesWithClassification(t *testing.T) {
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{"path": "document.doc_kind", "value": "product_specification", "confidence": 0.9, "evidence": "x"},
			},
		},
	}
	store := &stubFacetStore{}
	classifier := &DocumentClassifier{Extractor: ext, PromptText: "test", ModelName: "test", Vocabulary: testVocabulary(), Facets: store}
	resolver := &ApplicabilityResolver{Classifier: classifier, Facets: store, VocabularyReleases: stubVocabularyReleaseStore{releaseID: 42}}

	baseFacts := semrules.FactSet{
		"review.jurisdiction": {Path: "review.jurisdiction", Value: "US", State: semrules.FactKnown},
	}
	predicates := []semrules.Document{
		{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}},
	}
	enriched, err := resolver.ResolveReviewFacts(context.Background(), 11, "scope-abc", baseFacts, predicates, "sample")
	if err != nil {
		t.Fatalf("ResolveReviewFacts: %v", err)
	}
	fact, ok := enriched["document.doc_kind"]
	if !ok || fact.State != semrules.FactKnown || fact.Value != "product_specification" {
		t.Fatalf("enriched doc_kind fact = %#v, want product_specification", fact)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("inserted facet observations = %d, want 1 (review-time writes only facet observations)", len(store.inserted))
	}
	if store.inserted[0].Method != FacetMethodClassifier {
		t.Fatalf("inserted method = %q, want classifier", store.inserted[0].Method)
	}
	if store.inserted[0].VocabularyReleaseID != 42 {
		t.Fatalf("inserted vocabulary_release_id = %d, want 42", store.inserted[0].VocabularyReleaseID)
	}
}

// TestResolveReviewFactsSkipsTrivialPredicates proves the review entry point
// tolerates a trivial/unconditional applicability predicate (empty expression,
// produced by profileApplicability) alongside real ones.
func TestResolveReviewFactsSkipsTrivialPredicates(t *testing.T) {
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{"path": "document.doc_kind", "value": "product_specification", "confidence": 0.9, "evidence": "x"},
			},
		},
	}
	store := &stubFacetStore{}
	classifier := &DocumentClassifier{Extractor: ext, PromptText: "test", ModelName: "test", Vocabulary: testVocabulary(), Facets: store}
	resolver := &ApplicabilityResolver{Classifier: classifier, Facets: store}

	predicates := []semrules.Document{
		{Version: 1}, // trivial: no expression
		{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}},
	}
	enriched, err := resolver.ResolveReviewFacts(context.Background(), 11, "scope-abc", semrules.FactSet{}, predicates, "sample")
	if err != nil {
		t.Fatalf("ResolveReviewFacts: %v", err)
	}
	fact, ok := enriched["document.doc_kind"]
	if !ok || fact.Value != "product_specification" {
		t.Fatalf("enriched doc_kind fact = %#v, want product_specification", fact)
	}
}

// TestResolveReviewFactsFailurePreservesBaseFacts proves a genuine LLM
// failure on the review side preserves indeterminate: base facts come back
// unchanged and no facet observation is persisted.
func TestResolveReviewFactsFailurePreservesBaseFacts(t *testing.T) {
	ext := &stubExtractor{err: errors.New("LLM timeout")}
	store := &stubFacetStore{}
	classifier := &DocumentClassifier{Extractor: ext, PromptText: "test", ModelName: "test", Vocabulary: testVocabulary(), Facets: store}
	resolver := &ApplicabilityResolver{Classifier: classifier, Facets: store}

	baseFacts := semrules.FactSet{
		"review.jurisdiction": {Path: "review.jurisdiction", Value: "US", State: semrules.FactKnown},
	}
	predicates := []semrules.Document{
		{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}},
	}
	enriched, err := resolver.ResolveReviewFacts(context.Background(), 11, "scope-abc", baseFacts, predicates, "sample")
	if err != nil {
		t.Fatalf("ResolveReviewFacts: %v", err)
	}
	if len(enriched) != 1 {
		t.Fatalf("facts = %#v, want base facts unchanged", enriched)
	}
	if _, ok := enriched["document.doc_kind"]; ok {
		t.Fatal("doc_kind must not be added after classifier failure")
	}
	if len(store.inserted) != 0 {
		t.Fatalf("inserted observations = %d, want 0 after failure", len(store.inserted))
	}
}

// TestResolveReviewFactsIdentityIsStableAcrossRetries proves the review
// invocation identity is keyed on the stable review-scope id, so a retry of
// the same scope creation reuses the same invocation_id and the classifier's
// stable-retry returns the already-persisted observation without a second
// LLM call (at most one classifier invocation per record/review-selection-
// attempt).
func TestResolveReviewFactsIdentityIsStableAcrossRetries(t *testing.T) {
	ext := &countingExtractor{
		count: new(int),
		result: map[string]any{
			"classifications": []any{
				map[string]any{"path": "document.doc_kind", "value": "product_specification", "confidence": 0.9, "evidence": "x"},
			},
		},
	}
	store := &stubFacetStore{}
	classifier := &DocumentClassifier{Extractor: ext, PromptText: "test", ModelName: "test", Vocabulary: testVocabulary(), Facets: store}
	resolver := &ApplicabilityResolver{Classifier: classifier, Facets: store}

	predicates := []semrules.Document{
		{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}},
	}
	// First attempt: classifier runs once.
	if _, err := resolver.ResolveReviewFacts(context.Background(), 11, "scope-abc", semrules.FactSet{}, predicates, "sample"); err != nil {
		t.Fatalf("ResolveReviewFacts attempt 1: %v", err)
	}
	// Retry of the same scope creation: the classifier's stable-retry finds
	// the persisted invocation and does not call the LLM again.
	if _, err := resolver.ResolveReviewFacts(context.Background(), 11, "scope-abc", semrules.FactSet{}, predicates, "sample"); err != nil {
		t.Fatalf("ResolveReviewFacts attempt 2: %v", err)
	}
	if *ext.count != 1 {
		t.Fatalf("LLM calls = %d, want 1 (stable retry across the same scope)", *ext.count)
	}
}

func TestMergeTier12FactsMergesDeterministicAndMetadataObservations(t *testing.T) {
	conf := 1.0
	store := &stubFacetStore{observations: []FacetObservation{
		{RecordID: 1, Path: "document.page_count", Value: 12, Method: FacetMethodDeterministic, Confidence: &conf},
		{RecordID: 1, Path: "document.authority_hint", Value: "gb", Method: FacetMethodMetadata, Confidence: &conf},
	}}
	got := mergeTier12Facts(context.Background(), store, 1, semrules.FactSet{})
	if got["document.page_count"].Value != 12 {
		t.Errorf("page_count: got %+v", got["document.page_count"])
	}
	if got["document.authority_hint"].Value != "gb" {
		t.Errorf("authority_hint: got %+v", got["document.authority_hint"])
	}
}

func TestMergeTier12FactsExcludesClassifierObservations(t *testing.T) {
	// tier-3's own freshness contract is attempt-scoped; mergeTier12Facts
	// must never let a persisted classifier observation short-circuit a
	// later attempt's re-classification (see
	// TestResolveExtractionFactsAttemptKeyIsStablePerAttemptDistinctAcrossAttempts,
	// which depends on this exclusion).
	conf := 0.9
	store := &stubFacetStore{observations: []FacetObservation{
		{RecordID: 1, Path: "document.doc_kind", Value: "test_report", Method: FacetMethodClassifier, Confidence: &conf},
	}}
	got := mergeTier12Facts(context.Background(), store, 1, semrules.FactSet{})
	if _, ok := got["document.doc_kind"]; ok {
		t.Errorf("expected a classifier observation to be excluded, got %+v", got)
	}
}

func TestMergeTier12FactsFailsSafeToBaseOnPathCollision(t *testing.T) {
	// Two FactKnown producers for the same path is a builder error (should
	// not happen -- tier-1/2 and routing facts occupy disjoint path
	// namespaces by construction); mergeTier12Facts must fail closed to the
	// unmodified base rather than letting either value leak through
	// unpredictably.
	conf := 1.0
	store := &stubFacetStore{observations: []FacetObservation{
		{RecordID: 1, Path: "document.input_doc_type", Value: "should-not-apply", Method: FacetMethodDeterministic, Confidence: &conf},
	}}
	base := semrules.FactSet{"document.input_doc_type": {Path: "document.input_doc_type", State: semrules.FactKnown, Value: "pdf"}}
	got := mergeTier12Facts(context.Background(), store, 1, base)
	if got["document.input_doc_type"].Value != "pdf" {
		t.Errorf("expected fail-safe to the original base fact, got %+v", got["document.input_doc_type"])
	}
}

func TestMergeTier12FactsNilFacetsReturnsBaseUnchanged(t *testing.T) {
	base := semrules.FactSet{"document.input_doc_type": {Path: "document.input_doc_type", State: semrules.FactKnown, Value: "pdf"}}
	got := mergeTier12Facts(context.Background(), nil, 1, base)
	if len(got) != 1 || got["document.input_doc_type"].Value != "pdf" {
		t.Errorf("expected base unchanged, got %+v", got)
	}
}
