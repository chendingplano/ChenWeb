package profiles

import (
	"reflect"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// TestB3CrossConsumerPredicateFixtureIdenticalTruthTraceAndProvenance is the
// B3 shared predicate fixture that was never delivered by B3. Task F2 created
// it here to satisfy spec acceptance criterion 9: "automatic review-profile
// selection and extraction routing evaluate identical predicates to identical
// truth results and traces."
//
// The extraction consumer uses the REAL extraction-routing fact builder,
// BuildPipelineBindingFactSet -- the one ResolveExtractionFacts evaluates
// binding/gate predicates against. The review consumer mirrors the review
// path's actual composition: review-context facts (BuildReviewContextFacts)
// merged with the subject's facet-observation facts (BuildApplicabilityFactSet,
// as reviewDocumentFactsLoader does). The shared governed path
// document.input_doc_type is produced by both, and the same predicate document
// is then evaluated against each consumer's fact set through the one shared
// semrules.EvaluateDocument engine. The fixture asserts identical fact values,
// identical truth results, identical trace trees, and identical
// decision-relevant missing paths for true, false, and indeterminate outcomes.
//
// Previously this fixture compared two review-side builders: it used
// BuildApplicabilityFactSet (which is called only from the review path) as
// "the extraction fact builder" (P5 review 2026080302 criterion-9 note).
func TestB3CrossConsumerPredicateFixtureIdenticalTruthTraceAndProvenance(t *testing.T) {
	// Extraction consumer: the real extraction-routing fact builder.
	extractionFacts := docprocessing.BuildPipelineBindingFactSet(docprocessing.ProductionPlanFacts{
		InputDocType:     "pdf",
		SourceLanguage:   "en",
		KnowledgeStoreID: 42,
		DocumentNumber:   "DOC-1",
	})

	// Review consumer: the review path's actual composition -- review-context
	// facts merged with the subject's facet-observation facts.
	reviewCtxFacts, err := BuildReviewContextFacts(ReviewApplicabilityContext{
		AsOfDate: "2026-08-02", Jurisdiction: "US", OperatingContext: "production", Purpose: "compliance", ReleaseID: 42,
	})
	if err != nil {
		t.Fatalf("BuildReviewContextFacts: %v", err)
	}
	confidence := 0.9
	subjectFacts := docprocessing.BuildApplicabilityFactSet([]docprocessing.FacetObservation{{
		Path: "document.input_doc_type", Value: "pdf", State: semrules.FactKnown,
		Method:              docprocessing.FacetMethodDeterministic,
		Confidence:          &confidence,
		SourceFingerprint:   "facet-fp",
		VocabularyReleaseID: 42,
	}})
	reviewFacts, err := mergeFactSets(reviewCtxFacts, subjectFacts)
	if err != nil {
		t.Fatalf("mergeFactSets: %v", err)
	}

	// Identical values for the shared governed path both consumers produce.
	for _, path := range []string{"document.input_doc_type"} {
		extraction := extractionFacts[path]
		review := reviewFacts[path]
		if extraction.State != semrules.FactKnown || review.State != semrules.FactKnown {
			t.Fatalf("%s state: extraction=%s review=%s want known", path, extraction.State, review.State)
		}
		if extraction.Value != review.Value {
			t.Fatalf("%s value: extraction=%v review=%v want identical", path, extraction.Value, review.Value)
		}
	}

	// Identical truth results and identical traces through the shared engine.
	cases := []struct {
		name string
		doc  semrules.Document
		want semrules.Truth
	}{
		{"true", semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.input_doc_type", Op: "eq", Value: "pdf"}}, semrules.TruthTrue},
		{"false", semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.input_doc_type", Op: "eq", Value: "docx"}}, semrules.TruthFalse},
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
	// sets (no facet observation; no extraction metadata) is indistinguishable
	// to the shared engine -- same missing-fact trace and same decision-relevant
	// path list.
	indeterminate := semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "product_specification"}}
	emptyExtraction := docprocessing.BuildPipelineBindingFactSet(docprocessing.ProductionPlanFacts{})
	indeterminateReview, err := mergeFactSets(reviewCtxFacts, docprocessing.BuildApplicabilityFactSet(nil))
	if err != nil {
		t.Fatalf("mergeFactSets: %v", err)
	}
	extractionResult := semrules.EvaluateDocument(indeterminate, emptyExtraction)
	reviewResult := semrules.EvaluateDocument(indeterminate, indeterminateReview)
	if extractionResult.Truth != semrules.TruthIndeterminate || reviewResult.Truth != semrules.TruthIndeterminate {
		t.Fatalf("indeterminate truth: extraction=%s review=%s want indeterminate", extractionResult.Truth, reviewResult.Truth)
	}
	if !reflect.DeepEqual(extractionResult.TraceTree, reviewResult.TraceTree) {
		t.Fatalf("indeterminate trace tree differs:\nextraction=%+v\nreview=%+v", extractionResult.TraceTree, reviewResult.TraceTree)
	}
	wantMissing := []string{"document.doc_kind"}
	if !reflect.DeepEqual(extractionResult.DecisionRelevantMissingPaths, wantMissing) || !reflect.DeepEqual(reviewResult.DecisionRelevantMissingPaths, wantMissing) {
		t.Fatalf("decision-relevant missing paths: extraction=%v review=%v want %v", extractionResult.DecisionRelevantMissingPaths, reviewResult.DecisionRelevantMissingPaths, wantMissing)
	}
}
