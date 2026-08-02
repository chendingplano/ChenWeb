package profiles

import (
	"reflect"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// TestB3CrossConsumerPredicateFixture is the B3 shared predicate fixture that
// was never delivered by B3 (its plan checkboxes are unticked). Task F2
// created it here to satisfy spec acceptance criterion 9: "automatic
// review-profile selection and extraction routing evaluate identical
// predicates to identical truth results and traces."
//
// One governed fact path (review.jurisdiction) is produced through BOTH
// consumers' fact builders -- the extraction fact builder
// (docprocessing.BuildApplicabilityFactSet) and the review context builder
// (BuildReviewContextFacts) -- and the same predicate document is then
// evaluated against each consumer's fact set through the one shared
// semrules.EvaluateDocument engine. The fixture asserts the builders produce
// identical fact values and pinned provenance (release id), and that the
// shared engine returns identical truth results, identical trace trees, and
// identical decision-relevant missing paths for true, false, and
// indeterminate outcomes.
func TestB3CrossConsumerPredicateFixtureIdenticalTruthTraceAndProvenance(t *testing.T) {
	confidence := 0.9
	// Extraction consumer: reduce immutable facet observations to facts. The
	// observation carries the vocabulary release id as pinned provenance.
	extractionFacts := docprocessing.BuildApplicabilityFactSet([]docprocessing.FacetObservation{{
		Path: "review.jurisdiction", Value: "US", State: semrules.FactKnown,
		Method:              docprocessing.FacetMethodDeterministic,
		Confidence:          &confidence,
		SourceFingerprint:   "facet-fp",
		VocabularyReleaseID: 42,
	}})
	// Review consumer: build the review context facts, pinning the release id.
	reviewFacts, err := BuildReviewContextFacts(ReviewApplicabilityContext{
		AsOfDate: "2026-08-02", Jurisdiction: "US", OperatingContext: "production", Purpose: "compliance", ReleaseID: 42,
	})
	if err != nil {
		t.Fatalf("BuildReviewContextFacts: %v", err)
	}

	// Identical values and identical pinned provenance for the governed path.
	for _, path := range []string{"review.jurisdiction"} {
		extraction := extractionFacts[path]
		review := reviewFacts[path]
		if extraction.State != semrules.FactKnown || review.State != semrules.FactKnown {
			t.Fatalf("%s state: extraction=%s review=%s want known", path, extraction.State, review.State)
		}
		if extraction.Value != review.Value {
			t.Fatalf("%s value: extraction=%v review=%v want identical", path, extraction.Value, review.Value)
		}
		if extraction.ReleaseID != "42" || review.ReleaseID != "42" {
			t.Fatalf("%s pinned provenance: extraction=%q review=%q want 42", path, extraction.ReleaseID, review.ReleaseID)
		}
	}
	// The extraction consumer additionally retains its observation provenance.
	if got := extractionFacts["review.jurisdiction"]; got.Method != docprocessing.FacetMethodDeterministic || got.EvidenceRef != "facet-fp" {
		t.Fatalf("extraction provenance = method %q evidence %q, want deterministic/facet-fp", got.Method, got.EvidenceRef)
	}

	// Identical truth results and identical traces through the shared engine.
	cases := []struct {
		name string
		doc  semrules.Document
		want semrules.Truth
	}{
		{"true", semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "review.jurisdiction", Op: "eq", Value: "US"}}, semrules.TruthTrue},
		{"false", semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "review.jurisdiction", Op: "eq", Value: "CN"}}, semrules.TruthFalse},
	}
	for _, tc := range cases {
		extractionResult := semrules.EvaluateDocument(tc.doc, extractionFacts)
		reviewResult := semrules.EvaluateDocument(tc.doc, reviewFacts)
		if extractionResult.Truth != tc.want || reviewResult.Truth != tc.want {
			t.Fatalf("%s truth: extraction=%s review=%s want %s", tc.name, extractionResult.Truth, reviewResult.Truth, tc.want)
		}
		if !reflect.DeepEqual(extractionResult.TraceTree, reviewResult.TraceTree) {
			t.Fatalf("%s trace tree differs:\nextraction=%+v\nreview=%+v", tc.name, extractionResult.TraceTree, reviewResult.TraceTree)
		}
	}

	// Indeterminate branch: a governed path absent from BOTH consumers' fact
	// sets (no observation recorded; empty review context) is indistinguishable
	// to the shared engine -- same missing-fact trace and same decision-relevant
	// path list.
	indeterminate := semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "review.purpose", Op: "eq", Value: "compliance"}}
	emptyExtraction := docprocessing.BuildApplicabilityFactSet(nil)
	indeterminateReview, err := BuildReviewContextFacts(ReviewApplicabilityContext{
		AsOfDate: "2026-08-02", Jurisdiction: "US", OperatingContext: "production", ReleaseID: 42,
	}) // purpose empty -> review.purpose missing
	if err != nil {
		t.Fatalf("BuildReviewContextFacts: %v", err)
	}
	extractionResult := semrules.EvaluateDocument(indeterminate, emptyExtraction)
	reviewResult := semrules.EvaluateDocument(indeterminate, indeterminateReview)
	if extractionResult.Truth != semrules.TruthIndeterminate || reviewResult.Truth != semrules.TruthIndeterminate {
		t.Fatalf("indeterminate truth: extraction=%s review=%s want indeterminate", extractionResult.Truth, reviewResult.Truth)
	}
	if !reflect.DeepEqual(extractionResult.TraceTree, reviewResult.TraceTree) {
		t.Fatalf("indeterminate trace tree differs:\nextraction=%+v\nreview=%+v", extractionResult.TraceTree, reviewResult.TraceTree)
	}
	wantMissing := []string{"review.purpose"}
	if !reflect.DeepEqual(extractionResult.DecisionRelevantMissingPaths, wantMissing) || !reflect.DeepEqual(reviewResult.DecisionRelevantMissingPaths, wantMissing) {
		t.Fatalf("decision-relevant missing paths: extraction=%v review=%v want %v", extractionResult.DecisionRelevantMissingPaths, reviewResult.DecisionRelevantMissingPaths, wantMissing)
	}
}
