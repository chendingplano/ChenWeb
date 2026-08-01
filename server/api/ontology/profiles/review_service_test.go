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

type frozenRuleLoader struct {
	gotProfile string
	gotVersion int
	gotRelease int64
	rules      []ProfileRule
}

func (l *frozenRuleLoader) LoadReleasedRules(_ context.Context, profileID string, version int, releaseID int64) ([]ProfileRule, error) {
	l.gotProfile, l.gotVersion, l.gotRelease = profileID, version, releaseID
	return l.rules, nil
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

func TestReviewServiceEvaluatesRulesPinnedInScope(t *testing.T) {
	w := &memoryFindingWriter{}
	loader := &frozenRuleLoader{rules: []ProfileRule{{ID: 9, RuleID: "r", RuleKind: "required_assertion_pattern", Severity: "error", RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"p","quantifier":"exists_conforming"}`)}}}
	s := ReviewService{Findings: w, Rules: loader}
	_, err := s.EvaluatePinnedScope(context.Background(), ReviewScope{ReviewScopeID: "scope", ClosedDimensions: json.RawMessage(`["d"]`), SelectedProfiles: json.RawMessage(`[{"profile_id":"p","profile_version":2,"release_id":42}]`)}, nil, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if loader.gotProfile != "p" || loader.gotVersion != 2 || loader.gotRelease != 42 || len(w.findings) != 1 {
		t.Fatalf("loader=%#v findings=%#v", loader, w.findings)
	}
}
