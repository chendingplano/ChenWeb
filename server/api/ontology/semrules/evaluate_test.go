package semrules

import (
	"errors"
	"reflect"
	"testing"
)

func TestEvaluateDocumentTruthTablesAndDecisionRelevantMissingPaths(t *testing.T) {
	tests := []struct {
		name             string
		doc              Document
		facts            FactSet
		wantTruth        Truth
		wantMissing      []string
		wantRelevant     []bool
		wantChildReasons []string
		wantChildTruths  []Truth
	}{
		{
			name: "all false masks later missing child",
			doc: Document{Version: 1, Expression: Predicate{Kind: "all", Items: []Predicate{
				{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "manual"},
				{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"},
			}}},
			facts: FactSet{
				"document.doc_kind": {Path: "document.doc_kind", State: FactKnown, Value: "standard"},
			},
			wantTruth:        TruthFalse,
			wantRelevant:     []bool{true, false},
			wantChildReasons: []string{ReasonNotMatched, ReasonMissingFact},
			wantChildTruths:  []Truth{TruthFalse, TruthIndeterminate},
		},
		{
			name: "any true masks later missing child",
			doc: Document{Version: 1, Expression: Predicate{Kind: "any", Items: []Predicate{
				{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"},
				{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"},
			}}},
			facts: FactSet{
				"document.doc_kind": {Path: "document.doc_kind", State: FactKnown, Value: "standard"},
			},
			wantTruth:        TruthTrue,
			wantRelevant:     []bool{true, false},
			wantChildReasons: []string{ReasonMatched, ReasonMissingFact},
			wantChildTruths:  []Truth{TruthTrue, TruthIndeterminate},
		},
		{
			name: "all indeterminate keeps missing child relevant",
			doc: Document{Version: 1, Expression: Predicate{Kind: "all", Items: []Predicate{
				{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"},
				{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"},
			}}},
			facts: FactSet{
				"document.doc_kind": {Path: "document.doc_kind", State: FactKnown, Value: "standard"},
			},
			wantTruth:        TruthIndeterminate,
			wantMissing:      []string{"document.domain"},
			wantRelevant:     []bool{true, true},
			wantChildReasons: []string{ReasonMatched, ReasonMissingFact},
			wantChildTruths:  []Truth{TruthTrue, TruthIndeterminate},
		},
		{
			name: "not indeterminate propagates child",
			doc: Document{Version: 1, Expression: Predicate{Kind: "not", Items: []Predicate{
				{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"},
			}}},
			facts:            FactSet{},
			wantTruth:        TruthIndeterminate,
			wantMissing:      []string{"document.domain"},
			wantRelevant:     []bool{true},
			wantChildReasons: []string{ReasonMissingFact},
			wantChildTruths:  []Truth{TruthIndeterminate},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateDocument(tt.doc, tt.facts)
			if got.Truth != tt.wantTruth {
				t.Fatalf("Truth = %s, want %s", got.Truth, tt.wantTruth)
			}
			if !reflect.DeepEqual(got.DecisionRelevantMissingPaths, tt.wantMissing) {
				t.Fatalf("DecisionRelevantMissingPaths = %v, want %v", got.DecisionRelevantMissingPaths, tt.wantMissing)
			}
			if len(got.TraceTree.Children) != len(tt.wantRelevant) {
				t.Fatalf("trace child count = %d, want %d", len(got.TraceTree.Children), len(tt.wantRelevant))
			}
			for i, child := range got.TraceTree.Children {
				if child.DecisionRelevant != tt.wantRelevant[i] {
					t.Fatalf("child[%d].DecisionRelevant = %v, want %v", i, child.DecisionRelevant, tt.wantRelevant[i])
				}
				if child.ReasonCode != tt.wantChildReasons[i] {
					t.Fatalf("child[%d].ReasonCode = %q, want %q", i, child.ReasonCode, tt.wantChildReasons[i])
				}
				if child.Truth != tt.wantChildTruths[i] {
					t.Fatalf("child[%d].Truth = %s, want %s", i, child.Truth, tt.wantChildTruths[i])
				}
			}
		})
	}
}

// TestEvaluateDocumentMaskingIsOrderIndependent proves decision-relevance
// masking depends on logical position (whether a sibling already forces the
// node's truth), not authored position. P5 review 2026080302 finding P5-7:
// the original forward-only short-circuit reported a missing fact as
// decision-relevant whenever it was authored before the deciding sibling,
// even though the deciding sibling makes it moot regardless of order.
func TestEvaluateDocumentMaskingIsOrderIndependent(t *testing.T) {
	missingChild := Predicate{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"}
	falseChild := Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "manual"}
	trueChild := Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"}
	facts := FactSet{
		"document.doc_kind": {Path: "document.doc_kind", State: FactKnown, Value: "standard"},
	}

	tests := []struct {
		name        string
		items       []Predicate
		kind        string
		wantTruth   Truth
		wantMissing []string
	}{
		{"all: missing before false", []Predicate{missingChild, falseChild}, "all", TruthFalse, nil},
		{"all: false before missing", []Predicate{falseChild, missingChild}, "all", TruthFalse, nil},
		{"any: missing before true", []Predicate{missingChild, trueChild}, "any", TruthTrue, nil},
		{"any: true before missing", []Predicate{trueChild, missingChild}, "any", TruthTrue, nil},
	}

	results := make([]Result, len(tests))
	for i, tt := range tests {
		tt, i := tt, i
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateDocument(Document{Version: 1, Expression: Predicate{Kind: tt.kind, Items: tt.items}}, facts)
			results[i] = got
			if got.Truth != tt.wantTruth {
				t.Errorf("Truth = %s, want %s", got.Truth, tt.wantTruth)
			}
			if !reflect.DeepEqual(got.DecisionRelevantMissingPaths, tt.wantMissing) {
				t.Errorf("DecisionRelevantMissingPaths = %v, want %v (masking must not depend on authored order)", got.DecisionRelevantMissingPaths, tt.wantMissing)
			}
			for _, child := range got.TraceTree.Children {
				if child.Path == "document.domain" && child.DecisionRelevant {
					t.Errorf("missing child at path %q must be masked non-decision-relevant regardless of authored position", child.Path)
				}
			}
		})
	}

	// The "missing before deciding" and "deciding before missing" variants of
	// the same logical predicate must produce byte-identical results.
	if !reflect.DeepEqual(results[0].DecisionRelevantMissingPaths, results[1].DecisionRelevantMissingPaths) {
		t.Fatalf("all: order changed DecisionRelevantMissingPaths: %v vs %v", results[0].DecisionRelevantMissingPaths, results[1].DecisionRelevantMissingPaths)
	}
	if !reflect.DeepEqual(results[2].DecisionRelevantMissingPaths, results[3].DecisionRelevantMissingPaths) {
		t.Fatalf("any: order changed DecisionRelevantMissingPaths: %v vs %v", results[2].DecisionRelevantMissingPaths, results[3].DecisionRelevantMissingPaths)
	}
}

// TestEvaluateDocumentMaskingCascadesToNestedNodes proves that when a sibling
// definitively decides an "all"/"any" node, a nested all/any child is masked
// as a whole -- its own children never surface as decision-relevant missing
// paths, regardless of the nested predicate's internal authored order.
func TestEvaluateDocumentMaskingCascadesToNestedNodes(t *testing.T) {
	falseChild := Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "manual"}
	nestedMissing := Predicate{Kind: "any", Items: []Predicate{
		{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"},
		{Kind: "fact", Path: "document.jurisdiction", Op: "eq", Value: "US"},
	}}
	facts := FactSet{
		"document.doc_kind": {Path: "document.doc_kind", State: FactKnown, Value: "standard"},
	}

	got := EvaluateDocument(Document{Version: 1, Expression: Predicate{
		Kind: "all", Items: []Predicate{falseChild, nestedMissing},
	}}, facts)

	if got.Truth != TruthFalse {
		t.Fatalf("Truth = %s, want %s", got.Truth, TruthFalse)
	}
	if len(got.DecisionRelevantMissingPaths) != 0 {
		t.Fatalf("DecisionRelevantMissingPaths = %v, want none (nested any must be masked)", got.DecisionRelevantMissingPaths)
	}
	nestedNode := got.TraceTree.Children[1]
	if nestedNode.DecisionRelevant {
		t.Fatalf("nested any node.DecisionRelevant = true, want false (masked by deciding sibling)")
	}
	for _, grandchild := range nestedNode.Children {
		if grandchild.DecisionRelevant {
			t.Fatalf("masked nested node's grandchild %q.DecisionRelevant = true, want false", grandchild.Path)
		}
	}
}

func TestEvaluateDocumentTypedOperators(t *testing.T) {
	tests := []struct {
		name  string
		pred  Predicate
		facts FactSet
		want  Truth
	}{
		{
			name:  "eq string true",
			pred:  Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"},
			facts: FactSet{"document.doc_kind": knownFact("document.doc_kind", "standard")},
			want:  TruthTrue,
		},
		{
			name:  "neq string false",
			pred:  Predicate{Kind: "fact", Path: "document.doc_kind", Op: "neq", Value: "standard"},
			facts: FactSet{"document.doc_kind": knownFact("document.doc_kind", "standard")},
			want:  TruthFalse,
		},
		{
			name:  "in string true",
			pred:  Predicate{Kind: "fact", Path: "document.doc_kind", Op: "in", Value: []any{"manual", "standard"}},
			facts: FactSet{"document.doc_kind": knownFact("document.doc_kind", "standard")},
			want:  TruthTrue,
		},
		{
			name:  "not_in string true",
			pred:  Predicate{Kind: "fact", Path: "document.doc_kind", Op: "not_in", Value: []any{"manual", "guide"}},
			facts: FactSet{"document.doc_kind": knownFact("document.doc_kind", "standard")},
			want:  TruthTrue,
		},
		{
			name:  "gt number true",
			pred:  Predicate{Kind: "fact", Path: "document.numeric_unit_density", Op: "gt", Value: 1},
			facts: FactSet{"document.numeric_unit_density": knownFact("document.numeric_unit_density", 2)},
			want:  TruthTrue,
		},
		{
			name:  "gte date true",
			pred:  Predicate{Kind: "fact", Path: "review.as_of", Op: "gte", Value: "2026-08-01"},
			facts: FactSet{"review.as_of": knownFact("review.as_of", "2026-08-02")},
			want:  TruthTrue,
		},
		{
			name:  "lt number true",
			pred:  Predicate{Kind: "fact", Path: "document.numeric_unit_density", Op: "lt", Value: 3},
			facts: FactSet{"document.numeric_unit_density": knownFact("document.numeric_unit_density", 2)},
			want:  TruthTrue,
		},
		{
			name:  "lte date true",
			pred:  Predicate{Kind: "fact", Path: "review.as_of", Op: "lte", Value: "2026-08-02"},
			facts: FactSet{"review.as_of": knownFact("review.as_of", "2026-08-02")},
			want:  TruthTrue,
		},
		{
			name:  "contains string set true",
			pred:  Predicate{Kind: "fact", Path: "object.class", Op: "contains", Value: "component:display"},
			facts: FactSet{"object.class": knownFact("object.class", []string{"component:display", "component:module"})},
			want:  TruthTrue,
		},
		{
			name:  "exists known true",
			pred:  Predicate{Kind: "fact", Path: "deployment.workspace", Op: "exists"},
			facts: FactSet{"deployment.workspace": knownFact("deployment.workspace", "semos")},
			want:  TruthTrue,
		},
		{
			name:  "exists missing false",
			pred:  Predicate{Kind: "fact", Path: "deployment.workspace", Op: "exists"},
			facts: FactSet{"deployment.workspace": {Path: "deployment.workspace", State: FactMissing}},
			want:  TruthFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateDocument(Document{Version: 1, Expression: tt.pred}, tt.facts)
			if got.Truth != tt.want {
				t.Fatalf("Truth = %s, want %s", got.Truth, tt.want)
			}
		})
	}
}

func TestEvaluateDocumentIndeterminateReasons(t *testing.T) {
	minimum := 0.8
	const opName = "test_operator_error"
	t.Cleanup(func() {
		_ = defaultFactRegistry.setOperatorPaths(nil, opName)
		operatorsMu.Lock()
		delete(operators, opName)
		operatorsMu.Unlock()
	})
	if err := RegisterTypedOperatorForPaths(opName, []string{"document.doc_kind"}, func(KnownValue, any) (bool, error) {
		return false, errors.New("boom")
	}); err != nil {
		t.Fatalf("RegisterTypedOperatorForPaths: %v", err)
	}

	tests := []struct {
		name       string
		pred       Predicate
		fact       Fact
		wantTruth  Truth
		wantReason string
	}{
		{
			name:       "low confidence",
			pred:       Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard", MinConfidence: &minimum},
			fact:       factWithConfidence(knownFact("document.doc_kind", "standard"), 0.4),
			wantTruth:  TruthIndeterminate,
			wantReason: ReasonConfidenceBelowMinimum,
		},
		{
			name:       "conflicting fact",
			pred:       Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"},
			fact:       Fact{Path: "document.doc_kind", State: FactConflicting},
			wantTruth:  TruthIndeterminate,
			wantReason: ReasonConflictingFact,
		},
		{
			name:       "invalid fact",
			pred:       Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"},
			fact:       Fact{Path: "document.doc_kind", State: FactInvalid},
			wantTruth:  TruthIndeterminate,
			wantReason: ReasonInvalidFact,
		},
		{
			name:       "operator error",
			pred:       Predicate{Kind: "fact", Path: "document.doc_kind", Op: opName, Value: "standard"},
			fact:       knownFact("document.doc_kind", "standard"),
			wantTruth:  TruthIndeterminate,
			wantReason: ReasonOperatorError,
		},
		{
			// P5 review 2026080302 finding P5-14: a KNOWN fact whose observed
			// value contradicts its declared FactType is a producer defect,
			// not an operator failure -- spec §11 routes the two differently
			// (invalid facts stay indeterminate/trace-only; operator failure
			// is fail-closed alarm territory).
			name:       "wrong-typed known fact value is invalid_fact, not operator_error",
			pred:       Predicate{Kind: "fact", Path: "document.has_document_number", Op: "eq", Value: true},
			fact:       Fact{Path: "document.has_document_number", State: FactKnown, Value: "not-a-bool"},
			wantTruth:  TruthIndeterminate,
			wantReason: ReasonInvalidFact,
		},
		{
			// The authored/expected-side operand is a different failure
			// class: the observed fact is well-typed, but the predicate's
			// declared expected value cannot be compared against it. This
			// stays operator_error (Validate should catch this before
			// activation; evaluation-time is a defense-in-depth path).
			name:       "authored expected-value mismatch stays operator_error",
			pred:       Predicate{Kind: "fact", Path: "document.has_document_number", Op: "eq", Value: "not-a-bool"},
			fact:       Fact{Path: "document.has_document_number", State: FactKnown, Value: true},
			wantTruth:  TruthIndeterminate,
			wantReason: ReasonOperatorError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateDocument(Document{Version: 1, Expression: tt.pred}, FactSet{
				tt.pred.Path: tt.fact,
			})
			if got.Truth != tt.wantTruth {
				t.Fatalf("Truth = %s, want %s", got.Truth, tt.wantTruth)
			}
			if got.TraceTree.ReasonCode != tt.wantReason {
				t.Fatalf("ReasonCode = %q, want %q", got.TraceTree.ReasonCode, tt.wantReason)
			}
		})
	}
}

func knownFact(path string, value any) Fact {
	return Fact{Path: path, State: FactKnown, Value: value}
}

func factWithConfidence(fact Fact, confidence float64) Fact {
	fact.Confidence = &confidence
	return fact
}
