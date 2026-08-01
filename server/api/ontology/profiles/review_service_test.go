package profiles

import (
	"context"
	"encoding/json"
	"testing"
)

type memoryFindingWriter struct{ findings []OntologyFinding }

func (m *memoryFindingWriter) Persist(_ context.Context, f OntologyFinding) error {
	m.findings = append(m.findings, f)
	return nil
}

func TestReviewServiceEvaluatesFrozenScopeAndPersistsFinding(t *testing.T) {
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	results, err := s.EvaluateAndPersist(context.Background(), ReviewScope{ReviewScopeID: "scope-1", ClosedDimensions: json.RawMessage(`["display_metrics"]`)}, []ProfileRule{{ID: 9, RuleKind: "required_assertion_pattern", Severity: "error", RuleConfig: json.RawMessage(`{"dimension":"display_metrics","predicate_term_id":"measurement:luminance","quantifier":"exists_conforming"}`)}}, nil, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Category != ResultMissing || len(w.findings) != 1 || w.findings[0].ScopeID != "scope-1" || w.findings[0].ProfileRuleID != 9 {
		t.Fatalf("results=%#v findings=%#v", results, w.findings)
	}
}
