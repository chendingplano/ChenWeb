package profiles

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
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
	called     []string
	rules      []ProfileRule
}

func (l *frozenRuleLoader) LoadReleasedRules(_ context.Context, profileID string, version int, releaseID int64) ([]ProfileRule, error) {
	l.gotProfile, l.gotVersion, l.gotRelease = profileID, version, releaseID
	l.called = append(l.called, profileID)
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

// rule-level applicability fixtures. The rules below use the review context
// facts only (jurisdiction US) so the applicability gate is decided purely by
// the frozen scope, mirroring spec 2026080102 section 6 last paragraph.

const (
	ruleApplicabilityUS  = `{"version":1,"expression":{"kind":"fact","path":"review.jurisdiction","op":"eq","value":"US"}}`
	ruleApplicabilityCN  = `{"version":1,"expression":{"kind":"fact","path":"review.jurisdiction","op":"eq","value":"CN"}}`
	ruleApplicabilityInd = `{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"standard"}}`
)

func applicabilityRule(id int64, ruleID, applicability string) ProfileRule {
	return ProfileRule{
		ID: id, RuleID: ruleID, ProfileID: "p", ProfileVersion: 1, ReleaseID: 42,
		RuleKind: "required_assertion_pattern", Severity: "error",
		RuleConfig:    json.RawMessage(`{"dimension":"display_metrics","predicate_term_id":"x","quantifier":"exists_conforming"}`),
		Applicability: json.RawMessage(applicability),
	}
}

// TestReviewServiceRuleApplicabilityExcludesOnlyThatRuleWhenFalse proves that
// a pinned rule whose own applicability predicate evaluates false against the
// frozen review context is excluded from the run -- no result, no finding --
// while the other pinned rule still evaluates normally.
func TestReviewServiceRuleApplicabilityExcludesOnlyThatRuleWhenFalse(t *testing.T) {
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	scope := ReviewScope{ReviewScopeID: "scope-1", Jurisdiction: "US", ClosedDimensions: json.RawMessage(`["display_metrics"]`)}
	rules := []ProfileRule{
		applicabilityRule(1, "r-inapplicable", ruleApplicabilityCN),
		applicabilityRule(2, "r-applicable", ruleApplicabilityUS),
	}
	results, err := s.EvaluateAndPersist(context.Background(), scope, rules, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Category != ResultMissing {
		t.Fatalf("results = %#v, want only the applicable rule evaluated", results)
	}
	if len(w.findings) != 1 || w.findings[0].ProfileRuleID != 2 {
		t.Fatalf("findings = %#v, want exactly one finding for the applicable rule only", w.findings)
	}
}

// TestReviewServiceRuleApplicabilityEvaluatesNormallyOnTrue proves a pinned
// rule whose applicability is true against the frozen review context is
// evaluated normally (the existing rule-evaluation path).
func TestReviewServiceRuleApplicabilityEvaluatesNormallyOnTrue(t *testing.T) {
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	scope := ReviewScope{ReviewScopeID: "scope-1", Jurisdiction: "US", ClosedDimensions: json.RawMessage(`["display_metrics"]`)}
	results, err := s.EvaluateAndPersist(context.Background(), scope, []ProfileRule{applicabilityRule(2, "r-applicable", ruleApplicabilityUS)}, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Category != ResultMissing || len(w.findings) != 1 || w.findings[0].ProfileRuleID != 2 {
		t.Fatalf("results=%#v findings=%#v, want the rule evaluated normally", results, w.findings)
	}
}

// TestReviewServiceRuleApplicabilityEmitsIndeterminateOnDecisionRelevant
// proves that a decision-relevant indeterminate applicability (the rule's
// profile closed dimensions intersect the scope's closed dimensions, per spec
// section 6/11) yields an explicit indeterminate applicability result and a
// finding, not a silent exclusion.
func TestReviewServiceRuleApplicabilityEmitsIndeterminateOnDecisionRelevant(t *testing.T) {
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	scope := ReviewScope{
		ReviewScopeID: "scope-1", Jurisdiction: "US", ClosedDimensions: json.RawMessage(`["display_metrics"]`),
		SelectionSnapshot: json.RawMessage(`{"releases":[{"module_id":"m","release_id":42,"version":"0.1.0","content_checksum":"sha256:aaa"}],"selected":[{"profile_id":"p","profile_version":1,"release_id":42,"closed_dimensions":["display_metrics"]}],"evaluations":[],"status":"indeterminate"}`),
	}
	results, err := s.EvaluateAndPersist(context.Background(), scope, []ProfileRule{applicabilityRule(9, "r", ruleApplicabilityInd)}, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Category != ResultIndeterminate {
		t.Fatalf("results = %#v, want one indeterminate applicability result", results)
	}
	if len(w.findings) != 1 || w.findings[0].Category != ResultIndeterminate || w.findings[0].ProfileRuleID != 9 {
		t.Fatalf("findings = %#v, want one indeterminate finding for the affected rule", w.findings)
	}
}

// TestReviewServiceRuleApplicabilityNonDecisionRelevantIsTraceOnly proves an
// indeterminate applicability whose profile closed dimensions do NOT intersect
// the scope's closed dimensions stays trace-only: no result, no finding.
func TestReviewServiceRuleApplicabilityNonDecisionRelevantIsTraceOnly(t *testing.T) {
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	scope := ReviewScope{
		ReviewScopeID: "scope-1", Jurisdiction: "US", ClosedDimensions: json.RawMessage(`["display_metrics"]`),
		SelectionSnapshot: json.RawMessage(`{"releases":[{"module_id":"m","release_id":42,"version":"0.1.0","content_checksum":"sha256:aaa"}],"selected":[{"profile_id":"p","profile_version":1,"release_id":42,"closed_dimensions":["dimension_x"]}],"evaluations":[],"status":"complete"}`),
	}
	results, err := s.EvaluateAndPersist(context.Background(), scope, []ProfileRule{applicabilityRule(9, "r", ruleApplicabilityInd)}, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %#v, want the non-decision-relevant indeterminate rule trace-only (no result)", results)
	}
	if len(w.findings) != 0 {
		t.Fatalf("findings = %#v, want no finding for a trace-only indeterminate rule", w.findings)
	}
}

// TestReviewServiceNeverLoadsUnpinnedProfileRules proves EvaluatePinnedScope
// requests rules only for the profiles pinned in the scope's selected_profiles
// and can never add a profile/release that was not already pinned: an unpinned
// profile that has rules in the loader is never loaded or evaluated.
func TestReviewServiceNeverLoadsUnpinnedProfileRules(t *testing.T) {
	w := &memoryFindingWriter{}
	rules := &frozenRuleLoader{rules: []ProfileRule{{ID: 9, RuleID: "r", RuleKind: "required_assertion_pattern", Severity: "error", RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"p","quantifier":"exists_conforming"}`)}}}
	assertionLoader := &stubAssertionLoader{}
	runs := &stubReviewRunWriter{returnID: 55}
	s := ReviewService{Findings: w, Rules: rules, Assertions: assertionLoader, Runs: runs}
	_, _, err := s.EvaluatePinnedScope(context.Background(), ReviewScope{
		ReviewScopeID: "scope", ClosedDimensions: json.RawMessage(`["d"]`),
		TargetObjectIDs:  json.RawMessage(`["obj-1"]`),
		SelectedProfiles: json.RawMessage(`[{"profile_id":"p1","profile_version":1,"release_id":42}]`),
	}, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.called) != 1 || rules.called[0] != "p1" {
		t.Fatalf("rule loader asked for %v, want exactly the pinned profile p1 (never an unpinned profile)", rules.called)
	}
}

// TestReviewServiceRuleApplicabilityUsesFrozenSubjectFactSnapshot proves a
// rule whose applicability references a document.* path is evaluated against
// the scope's frozen per-subject fact snapshot for the executed input record
// (review context + document + deployment facts merged at selection time), not
// review-context facts alone. With a matching frozen entry carrying
// document.doc_kind=standard, the rule's applicability evaluates true and the
// rule is evaluated normally -- a finding persists -- instead of being silently
// dropped or spuriously indeterminate.
func TestReviewServiceRuleApplicabilityUsesFrozenSubjectFactSnapshot(t *testing.T) {
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	facts, err := BuildReviewContextFacts(ReviewApplicabilityContext{Jurisdiction: "US"})
	if err != nil {
		t.Fatal(err)
	}
	facts["document.doc_kind"] = semrules.Fact{Path: "document.doc_kind", State: semrules.FactKnown, Value: "standard"}
	snapshot, err := json.Marshal([]SubjectFactSnapshot{
		{Subject: SelectionSubject{DocumentID: 2}, Facts: facts},
		{Subject: SelectionSubject{DocumentID: 99}, Facts: facts},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := ReviewScope{
		ReviewScopeID:    "scope-1",
		Jurisdiction:     "US",
		ClosedDimensions: json.RawMessage(`["display_metrics"]`),
		FactSnapshot:     snapshot,
	}
	results, err := s.EvaluateAndPersist(context.Background(), scope, []ProfileRule{applicabilityRule(9, "r", ruleApplicabilityInd)}, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	// The rule evaluates normally (true path): the evaluator ran and the finding
	// persisted, not a silent exclusion and not a spurious indeterminate.
	if len(results) != 1 || results[0].Category != ResultMissing {
		t.Fatalf("results = %#v, want the rule evaluated normally against the frozen facts", results)
	}
	if len(w.findings) != 1 || w.findings[0].ProfileRuleID != 9 {
		t.Fatalf("findings = %#v, want one normal finding for the rule", w.findings)
	}
}

// TestReviewServiceRuleApplicabilityConsultsEveryPinnedTargetNotJustFirst
// proves a rule's applicability is decided by consulting every pinned
// target's own frozen facts, not by matching on document id alone and
// arbitrarily using whichever entry the snapshot lists first. A scope with
// two targets on the same document, whose object.class facts differ, must
// find a rule applicable when the SECOND target's facts satisfy it even
// though the FIRST target's do not (P5 review 2026080302 finding P5-8).
func TestReviewServiceRuleApplicabilityConsultsEveryPinnedTargetNotJustFirst(t *testing.T) {
	baseFacts, err := BuildReviewContextFacts(ReviewApplicabilityContext{Jurisdiction: "US"})
	if err != nil {
		t.Fatal(err)
	}
	nonMatchingClassFacts := semrules.FactSet{}
	for k, v := range baseFacts {
		nonMatchingClassFacts[k] = v
	}
	nonMatchingClassFacts["object.class"] = semrules.Fact{Path: "object.class", State: semrules.FactKnown, Value: []string{"housing"}}

	matchingClassFacts := semrules.FactSet{}
	for k, v := range baseFacts {
		matchingClassFacts[k] = v
	}
	matchingClassFacts["object.class"] = semrules.Fact{Path: "object.class", State: semrules.FactKnown, Value: []string{"display_module"}}

	const applicabilityDisplayModule = `{"version":1,"expression":{"kind":"fact","path":"object.class","op":"contains","value":"display_module"}}`

	// The non-matching target is listed FIRST in the snapshot -- the exact
	// shape that made the pre-fix code pick the wrong target's facts.
	snapshot, err := json.Marshal([]SubjectFactSnapshot{
		{Subject: SelectionSubject{DocumentID: 2, TargetObjectID: "obj-housing"}, Facts: nonMatchingClassFacts},
		{Subject: SelectionSubject{DocumentID: 2, TargetObjectID: "obj-display"}, Facts: matchingClassFacts},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := ReviewScope{
		ReviewScopeID:    "scope-1",
		Jurisdiction:     "US",
		ClosedDimensions: json.RawMessage(`["display_metrics"]`),
		FactSnapshot:     snapshot,
	}
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	results, err := s.EvaluateAndPersist(context.Background(), scope, []ProfileRule{applicabilityRule(9, "r", applicabilityDisplayModule)}, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Category != ResultMissing {
		t.Fatalf("results = %#v, want the rule applicable (evaluated normally) because the second target's object.class matches", results)
	}
	if len(w.findings) != 1 {
		t.Fatalf("findings = %#v, want one normal finding -- a rule applicable to any pinned target must not be silently excluded", w.findings)
	}
}

// TestReviewServiceRuleApplicabilityFallsBackWithoutFrozenSubjectFacts proves
// a scope with no matching FactSnapshot subject (empty fact_snapshot, or no
// entry whose document id matches the input record) falls back to
// review-context-only facts, preserving the existing behavior: the existing
// four rule-applicability tests construct scopes without FactSnapshot and stay
// unchanged.
func TestReviewServiceRuleApplicabilityFallsBackWithoutFrozenSubjectFacts(t *testing.T) {
	// No fact_snapshot at all: review-context-only fallback keeps the rule
	// (jurisdiction US) applicable and evaluates it normally.
	w := &memoryFindingWriter{}
	s := ReviewService{Findings: w}
	scope := ReviewScope{ReviewScopeID: "scope-1", Jurisdiction: "US", ClosedDimensions: json.RawMessage(`["display_metrics"]`)}
	results, err := s.EvaluateAndPersist(context.Background(), scope, []ProfileRule{applicabilityRule(2, "r", ruleApplicabilityUS)}, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Category != ResultMissing || len(w.findings) != 1 {
		t.Fatalf("no-fact_snapshot results = %#v findings = %#v, want the review-context fallback to keep the rule applicable", results, w.findings)
	}
	// A fact_snapshot that exists but has no entry for the executed input
	// record (document 2): also falls back to review-context-only.
	w2 := &memoryFindingWriter{}
	s2 := ReviewService{Findings: w2}
	snapshot, err := json.Marshal([]SubjectFactSnapshot{
		{Subject: SelectionSubject{DocumentID: 99}, Facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope2 := ReviewScope{ReviewScopeID: "scope-2", Jurisdiction: "US", ClosedDimensions: json.RawMessage(`["display_metrics"]`), FactSnapshot: snapshot}
	results2, err := s2.EvaluateAndPersist(context.Background(), scope2, []ProfileRule{applicabilityRule(2, "r", ruleApplicabilityUS)}, nil, 2, 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(results2) != 1 || results2[0].Category != ResultMissing || len(w2.findings) != 1 {
		t.Fatalf("no-matching-entry results = %#v findings = %#v, want the review-context fallback", results2, w2.findings)
	}
}
