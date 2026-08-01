package semrules

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTruthValues(t *testing.T) {
	got := []Truth{TruthTrue, TruthFalse, TruthIndeterminate}
	want := []Truth{"true", "false", "indeterminate"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("truth values = %q, want %q", got, want)
	}
}

func TestPredicateJSONGrammarV1(t *testing.T) {
	minimum := 0.8
	doc := Document{
		Version: 1,
		Expression: Predicate{
			Kind: "all",
			Items: []Predicate{
				{
					Kind:  "fact",
					Path:  "document.doc_kind",
					Op:    "in",
					Value: []string{"standard", "specification"},
				},
				{
					Kind:          "fact",
					Path:          "document.numeric_unit_density",
					Op:            "gte",
					Value:         0.02,
					MinConfidence: &minimum,
				},
			},
		},
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal v1 document: %v", err)
	}

	var roundTrip Document
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("unmarshal v1 document: %v", err)
	}
	if roundTrip.Version != 1 || roundTrip.Expression.Kind != "all" {
		t.Fatalf("round trip document = %#v", roundTrip)
	}
	if got := roundTrip.Expression.Items[0]; got.Kind != "fact" || got.Path != "document.doc_kind" || got.Op != "in" {
		t.Fatalf("first predicate = %#v", got)
	}
	if got := roundTrip.Expression.Items[1].MinConfidence; got == nil || *got != minimum {
		t.Fatalf("minimum confidence = %v, want %v", got, minimum)
	}
}

func TestFactStates(t *testing.T) {
	got := []FactState{FactKnown, FactMissing, FactConflicting, FactInvalid}
	want := []FactState{"known", "missing", "conflicting", "invalid"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fact states = %q, want %q", got, want)
	}

	confidence := 0.91
	fact := Fact{
		Path:        "document.doc_kind",
		Value:       "standard",
		State:       FactKnown,
		Confidence:  &confidence,
		Method:      "metadata-facet-producer",
		EvidenceRef: "evidence:42",
		RunID:       "run-1",
		PolicyID:    "policy-1",
		ReleaseID:   "release-1",
	}
	raw, err := json.Marshal(fact)
	if err != nil {
		t.Fatalf("marshal fact: %v", err)
	}
	var roundTrip Fact
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("unmarshal fact: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, fact) {
		t.Fatalf("round trip fact = %#v, want %#v", roundTrip, fact)
	}
}
