package docprocessing

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func TestReduceFacetObservationsRanksMethods(t *testing.T) {
	observations := []FacetObservation{
		{RecordID: 91, Path: "document.doc_kind", Value: "classifier", Method: FacetMethodClassifier, Confidence: ptrFloat64(0.99)},
		{RecordID: 91, Path: "document.doc_kind", Value: "metadata", Method: FacetMethodMetadata, Confidence: ptrFloat64(0.8)},
		{RecordID: 91, Path: "document.doc_kind", Value: "deterministic", Method: FacetMethodDeterministic, Confidence: ptrFloat64(0.7)},
	}

	got := ReduceFacetObservations(observations)
	fact := got["document.doc_kind"]
	if fact.State != semrules.FactKnown || fact.Value != "deterministic" {
		t.Fatalf("fact=%+v, want deterministic known value", fact)
	}
	if fact.Confidence == nil || *fact.Confidence != 0.7 {
		t.Fatalf("confidence=%v, want 0.7", fact.Confidence)
	}
}

func TestReduceFacetObservationsUsesMinimumConfidenceForSameRankAgreement(t *testing.T) {
	got := ReduceFacetObservations([]FacetObservation{
		{RecordID: 91, Path: "document.domain", Value: "mechanical", Method: FacetMethodMetadata, Confidence: ptrFloat64(0.9)},
		{RecordID: 91, Path: "document.domain", Value: "mechanical", Method: FacetMethodMetadata, Confidence: ptrFloat64(0.6)},
	})

	fact := got["document.domain"]
	if fact.State != semrules.FactKnown || fact.Value != "mechanical" {
		t.Fatalf("fact=%+v, want known mechanical", fact)
	}
	if fact.Confidence == nil || *fact.Confidence != 0.6 {
		t.Fatalf("confidence=%v, want minimum 0.6", fact.Confidence)
	}
}

func TestReduceFacetObservationsConflictsSameRankDistinctCanonicalValues(t *testing.T) {
	got := ReduceFacetObservations([]FacetObservation{
		{RecordID: 91, Path: "document.domain", Value: "mechanical", Method: FacetMethodMetadata, Confidence: ptrFloat64(0.9)},
		{RecordID: 91, Path: "document.domain", Value: "electrical", Method: FacetMethodMetadata, Confidence: ptrFloat64(0.8)},
		{RecordID: 91, Path: "document.domain", Value: "mechanical", Method: FacetMethodClassifier, Confidence: ptrFloat64(1)},
	})

	fact := got["document.domain"]
	if fact.State != semrules.FactConflicting {
		t.Fatalf("state=%s, want conflicting", fact.State)
	}
	values, ok := fact.Value.([]any)
	if !ok || !reflect.DeepEqual(values, []any{"electrical", "mechanical"}) {
		t.Fatalf("conflict values=%#v, want sorted distinct values", fact.Value)
	}
}

func TestReduceFacetObservationsMalformedAtHighestRankBecomesInvalid(t *testing.T) {
	got := ReduceFacetObservations([]FacetObservation{
		{RecordID: 91, Path: "document.has_document_number", Value: true, Method: FacetMethodDeterministic, Confidence: ptrFloat64(1)},
		{RecordID: 91, Path: "document.has_document_number", Value: "yes", Method: FacetMethodDeterministic, Confidence: ptrFloat64(1), Malformed: true},
		{RecordID: 91, Path: "document.has_document_number", Value: false, Method: FacetMethodMetadata, Confidence: ptrFloat64(0.9)},
	})

	fact := got["document.has_document_number"]
	if fact.State != semrules.FactInvalid {
		t.Fatalf("state=%s, want invalid", fact.State)
	}
}

func TestReduceFacetObservationsLowerRankDoesNotInterfere(t *testing.T) {
	got := ReduceFacetObservations([]FacetObservation{
		{RecordID: 91, Path: "document.domain", Value: "mechanical", Method: FacetMethodMetadata, Confidence: ptrFloat64(0.9)},
		{RecordID: 91, Path: "document.domain", Value: "electrical", Method: FacetMethodClassifier, Confidence: ptrFloat64(1)},
	})

	fact := got["document.domain"]
	if fact.State != semrules.FactKnown || fact.Value != "mechanical" {
		t.Fatalf("fact=%+v, want metadata known value", fact)
	}
}

func TestBuildApplicabilityFactSetUsesCanonicalNamespaces(t *testing.T) {
	got := BuildApplicabilityFactSet([]FacetObservation{
		{RecordID: 91, Path: "document.doc_kind", Value: "standard", Method: FacetMethodClassifier, Confidence: ptrFloat64(0.8), VocabularyReleaseID: 42},
		{RecordID: 91, Path: "document.source_language", Value: "en", Method: FacetMethodDeterministic, Confidence: ptrFloat64(1)},
	})

	for _, path := range []string{"document.doc_kind", "document.source_language"} {
		fact, ok := got[path]
		if !ok {
			t.Fatalf("missing fact path %q", path)
		}
		if fact.Path != path || fact.State != semrules.FactKnown {
			t.Fatalf("fact[%s]=%+v", path, fact)
		}
	}
	if got["document.doc_kind"].ReleaseID != "42" {
		t.Fatalf("release id=%q, want 42", got["document.doc_kind"].ReleaseID)
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}

// TestNormalizeObservationValueJsonNumberMatchesGoFloat proves a JSONB-decoded
// json.Number("2.0") and a Go float64(2) produce the same fact-value key, so
// the same facet written by two code paths is not misflagged as a conflicting
// value (P5 review 2026080302 finding P5-28).
func TestNormalizeObservationValueJsonNumberMatchesGoFloat(t *testing.T) {
	k1 := canonicalFactValueKey(json.Number("2.0"))
	k2 := canonicalFactValueKey(float64(2))
	if k1 != k2 {
		t.Fatalf("json.Number(2.0) key %q != float64(2) key %q", k1, k2)
	}

	// End-to-end: two same-rank observations that are numerically equal reduce
	// to a single known fact, not a conflict.
	facts := ReduceFacetObservations([]FacetObservation{
		{Path: "document.threshold", Value: json.Number("2.0"), State: semrules.FactKnown, Method: FacetMethodMetadata},
		{Path: "document.threshold", Value: float64(2), State: semrules.FactKnown, Method: FacetMethodMetadata},
	})
	fact := facts["document.threshold"]
	if fact.State == semrules.FactConflicting {
		t.Fatalf("numerically equal observations must not conflict: %+v", fact)
	}
	if fact.State != semrules.FactKnown {
		t.Fatalf("state = %s, want known", fact.State)
	}
}
