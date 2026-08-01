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

// stubAssertionLoader stands in for the governed kb.semantic_assertions
// lookup: it records which object ids the scope asked for and returns only
// the assertions seeded for that id, never anything a caller supplies
// out-of-band.
type stubAssertionLoader struct {
	gotObjectIDs []string
	byObjectID   map[string][]ReviewAssertion
}

func (l *stubAssertionLoader) LoadAcceptedAssertions(_ context.Context, objectID string) ([]ReviewAssertion, error) {
	l.gotObjectIDs = append(l.gotObjectIDs, objectID)
	return l.byObjectID[objectID], nil
}

// stubReviewRunWriter records the run it was asked to create (in particular
// the computed assertion_watermark) and returns a fixed generated id.
type stubReviewRunWriter struct {
	got      ReviewRun
	returnID int64
}

func (w *stubReviewRunWriter) CreateRun(_ context.Context, run ReviewRun) (ReviewRun, error) {
	w.got = run
	run.ID = w.returnID
	return run, nil
}

func TestReviewServiceEvaluatesFrozenScopeAndPersistsFinding(t *testing.T) {
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	results, err := s.EvaluateAndPersist(context.Background(), ReviewScope{ReviewScopeID: "scope-1", ClosedDimensions: json.RawMessage(`["display_metrics"]`)}, []ProfileRule{{ID: 9, RuleKind: "required_assertion_pattern", Severity: "error", RuleConfig: json.RawMessage(`{"dimension":"display_metrics","predicate_term_id":"measurement:luminance","quantifier":"exists_conforming"}`)}}, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Category != ResultMissing || len(w.findings) != 1 || w.findings[0].ScopeID != "scope-1" || w.findings[0].ProfileRuleID != 9 || w.findings[0].ReviewRunID != 6 {
		t.Fatalf("results=%#v findings=%#v", results, w.findings)
	}
}

func TestReviewServiceEvaluatesRulesPinnedInScope(t *testing.T) {
	w := &memoryFindingWriter{}
	rules := &frozenRuleLoader{rules: []ProfileRule{{ID: 9, RuleID: "r", RuleKind: "required_assertion_pattern", Severity: "error", RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"p","quantifier":"exists_conforming"}`)}}}
	assertionLoader := &stubAssertionLoader{}
	runs := &stubReviewRunWriter{returnID: 55}
	s := ReviewService{Findings: w, Rules: rules, Assertions: assertionLoader, Runs: runs}
	_, run, err := s.EvaluatePinnedScope(context.Background(), ReviewScope{
		ReviewScopeID: "scope", ClosedDimensions: json.RawMessage(`["d"]`),
		TargetObjectIDs:  json.RawMessage(`["obj-1"]`),
		SelectedProfiles: json.RawMessage(`[{"profile_id":"p","profile_version":2,"release_id":42}]`),
	}, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if rules.gotProfile != "p" || rules.gotVersion != 2 || rules.gotRelease != 42 || len(w.findings) != 1 {
		t.Fatalf("rules=%#v findings=%#v", rules, w.findings)
	}
	if runs.got.ReviewScopeID != "scope" || runs.got.InputRecordID != 2 || runs.got.AssertionWatermark != "none" {
		t.Fatalf("run created = %#v, want a run pinned to the scope with watermark 'none' (no assertions loaded)", runs.got)
	}
	if run.ID != 55 {
		t.Fatalf("returned run = %#v, want the generated run id 55", run)
	}
	if w.findings[0].ReviewRunID != 55 {
		t.Fatalf("finding ReviewRunID = %d, want the generated run id 55", w.findings[0].ReviewRunID)
	}
}

func TestReviewServiceLoadsAssertionsFromScopeTargetsNotACaller(t *testing.T) {
	w := &memoryFindingWriter{}
	rules := &frozenRuleLoader{rules: []ProfileRule{{ID: 9, RuleID: "r", RuleKind: "required_assertion_pattern", Severity: "error", RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:luminance","quantifier":"exists_conforming"}`)}}}
	assertionLoader := &stubAssertionLoader{byObjectID: map[string][]ReviewAssertion{
		"obj-1": {{AssertionID: 77, PredicateTermID: "measurement:luminance", Status: "accepted"}},
	}}
	runs := &stubReviewRunWriter{returnID: 1}
	s := ReviewService{Findings: w, Rules: rules, Assertions: assertionLoader, Runs: runs}
	results, _, err := s.EvaluatePinnedScope(context.Background(), ReviewScope{
		ReviewScopeID: "scope", ClosedDimensions: json.RawMessage(`["d"]`),
		TargetObjectIDs:  json.RawMessage(`["obj-1", "obj-2"]`),
		SelectedProfiles: json.RawMessage(`[{"profile_id":"p","profile_version":1,"release_id":1}]`),
	}, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(assertionLoader.gotObjectIDs) != 2 || assertionLoader.gotObjectIDs[0] != "obj-1" || assertionLoader.gotObjectIDs[1] != "obj-2" {
		t.Fatalf("assertion loader queried = %v, want exactly the scope's pinned target_object_ids", assertionLoader.gotObjectIDs)
	}
	if len(results) != 1 || results[0].Category != ResultSatisfied || len(results[0].AssertionIDs) != 1 || results[0].AssertionIDs[0] != 77 {
		t.Fatalf("results = %#v, want satisfied using the loader's assertion 77", results)
	}
	if runs.got.AssertionWatermark != "assertion:77" {
		t.Fatalf("run watermark = %q, want assertion:77 (the highest loaded assertion id)", runs.got.AssertionWatermark)
	}
}

func TestReviewServiceWatermarkTracksHighestLoadedAssertionAcrossTargets(t *testing.T) {
	w := &memoryFindingWriter{}
	rules := &frozenRuleLoader{rules: []ProfileRule{{ID: 9, RuleID: "r", RuleKind: "required_assertion_pattern", Severity: "error", RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:x","quantifier":"exists_conforming"}`)}}}
	assertionLoader := &stubAssertionLoader{byObjectID: map[string][]ReviewAssertion{
		"obj-1": {{AssertionID: 5, PredicateTermID: "measurement:x", Status: "accepted"}},
		"obj-2": {{AssertionID: 90, PredicateTermID: "measurement:x", Status: "accepted"}, {AssertionID: 12, PredicateTermID: "measurement:x", Status: "accepted"}},
	}}
	runs := &stubReviewRunWriter{returnID: 1}
	s := ReviewService{Findings: w, Rules: rules, Assertions: assertionLoader, Runs: runs}
	_, _, err := s.EvaluatePinnedScope(context.Background(), ReviewScope{
		ReviewScopeID: "scope", ClosedDimensions: json.RawMessage(`["d"]`),
		TargetObjectIDs:  json.RawMessage(`["obj-1", "obj-2"]`),
		SelectedProfiles: json.RawMessage(`[{"profile_id":"p","profile_version":1,"release_id":1}]`),
	}, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if runs.got.AssertionWatermark != "assertion:90" {
		t.Fatalf("watermark = %q, want assertion:90 (the highest assertion id across all target objects)", runs.got.AssertionWatermark)
	}
}
