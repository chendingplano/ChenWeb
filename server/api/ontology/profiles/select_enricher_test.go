package profiles

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// enrichCall records one EnrichReviewFacts invocation so the selector tests
// can assert the review-time tier-3 pass runs once per subject with the stable
// scope identity (spec 2026080102 section 7: "optional classify_document for
// newly decision-relevant missing facets").
type enrichCall struct {
	subject    SelectionSubject
	attemptKey string
	base       semrules.FactSet
	predicates []semrules.Document
}

// stubReviewEnricher is a test double for ReviewFactEnricher. With facts set,
// it returns those facts merged over the base (simulating a classifier that
// resolved decision-relevant missing tier-3 paths); otherwise it returns the
// base unchanged.
type stubReviewEnricher struct {
	calls []enrichCall
	facts semrules.FactSet
	err   error
}

func (e *stubReviewEnricher) EnrichReviewFacts(_ context.Context, subject SelectionSubject, attemptKey string, base semrules.FactSet, predicates []semrules.Document) (semrules.FactSet, error) {
	e.calls = append(e.calls, enrichCall{subject: subject, attemptKey: attemptKey, base: base, predicates: predicates})
	if e.err != nil {
		return nil, e.err
	}
	if e.facts == nil {
		return base, nil
	}
	merged := make(semrules.FactSet, len(base)+len(e.facts))
	for path, fact := range base {
		merged[path] = fact
	}
	for path, fact := range e.facts {
		merged[path] = fact
	}
	return merged, nil
}

// TestSelectCallsEnricherOncePerSubjectWithStableAttemptKey proves the
// review-time tier-3 pass runs exactly once per reviewed subject, and that
// the attempt key passed through is the stable review-scope id (not the
// random selection-attempt id), so the classifier's stable-retry can dedupe
// across retries of the same scope creation.
func TestSelectCallsEnricherOncePerSubjectWithStableAttemptKey(t *testing.T) {
	enricher := &stubReviewEnricher{}
	selector := Selector{
		Source: &stubSelectionSource{
			storeID:  7,
			released: selectionFixture(selectionProfile("p1", applicabilityTrueUS, "[]")),
		},
		Enricher: enricher,
	}
	_, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-abc",
		ReviewedDocumentIDs: []int64{11, 22},
		ReviewContext:       selectionReviewContext(),
		ClosedDimensions:    []string{},
		SelectedBy:          "tester",
		SelectionReason:     "test",
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(enricher.calls) != 2 {
		t.Fatalf("enricher calls = %d, want 2", len(enricher.calls))
	}
	for _, c := range enricher.calls {
		if c.attemptKey != "scope-abc" {
			t.Fatalf("attempt key = %q, want stable scope id scope-abc", c.attemptKey)
		}
	}
	gotSubjects := []int64{enricher.calls[0].subject.DocumentID, enricher.calls[1].subject.DocumentID}
	if gotSubjects[0] != 11 || gotSubjects[1] != 22 {
		t.Fatalf("enriched subjects = %v, want [11 22]", gotSubjects)
	}
}

// TestSelectCallsEnricherWithProfilePredicates proves the enricher receives
// every non-trivial released profile applicability predicate, so its
// decision-relevance analysis spans the whole profile set and it can
// classify a path any profile needs (not just the first).
func TestSelectCallsEnricherWithProfilePredicates(t *testing.T) {
	enricher := &stubReviewEnricher{}
	selector := Selector{
		Source: &stubSelectionSource{
			storeID: 7,
			released: selectionFixture(
				selectionProfile("p1", applicabilityTrueUS, "[]"),
				selectionProfile("p2", applicabilityIndet, "[]"),
			),
		},
		Enricher: enricher,
	}
	_, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-abc",
		ReviewedDocumentIDs: []int64{11},
		ReviewContext:       selectionReviewContext(),
		ClosedDimensions:    []string{},
		SelectedBy:          "tester",
		SelectionReason:     "test",
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(enricher.calls) != 1 {
		t.Fatalf("enricher calls = %d, want 1", len(enricher.calls))
	}
	if len(enricher.calls[0].predicates) != 2 {
		t.Fatalf("predicates passed to enricher = %d, want 2", len(enricher.calls[0].predicates))
	}
}

// TestSelectEvaluatesProfilesAgainstEnrichedFacts proves a profile whose
// applicability predicate needs a facet the pipeline never extracted can be
// selected once the enricher resolves it: the base facts carry no
// document.doc_kind, so without enrichment the profile is indeterminate and
// (with no intersecting closed dimension) not pinned; with enrichment it is
// selected as true.
func TestSelectEvaluatesProfilesAgainstEnrichedFacts(t *testing.T) {
	enricher := &stubReviewEnricher{
		facts: semrules.FactSet{
			"document.doc_kind": {Path: "document.doc_kind", Value: "product_specification", State: semrules.FactKnown},
		},
	}
	selector := Selector{
		Source: &stubSelectionSource{
			storeID: 7,
			released: selectionFixture(selectionProfile("p1",
				`{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"product_specification"}}`, "[]")),
		},
		Enricher: enricher,
	}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-abc",
		ReviewedDocumentIDs: []int64{11},
		ReviewContext:       selectionReviewContext(),
		ClosedDimensions:    []string{},
		SelectedBy:          "tester",
		SelectionReason:     "test",
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(result.SelectedProfiles) != 1 || result.SelectedProfiles[0].ProfileID != "p1" {
		t.Fatalf("selected profiles = %#v, want [p1]", result.SelectedProfiles)
	}
	if result.SelectedProfiles[0].Outcome != semrules.TruthTrue {
		t.Fatalf("outcome = %v, want true", result.SelectedProfiles[0].Outcome)
	}
}

// TestSelectNilEnricherEvaluatesBaseFacts proves the selector is byte-identical
// to the pre-classifier build when no enricher is injected: the profile stays
// indeterminate on the missing tier-3 path and is not pinned.
func TestSelectNilEnricherEvaluatesBaseFacts(t *testing.T) {
	selector := Selector{
		Source: &stubSelectionSource{
			storeID:  7,
			released: selectionFixture(selectionProfile("p1", applicabilityIndet, "[]")),
		},
	}
	result, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-abc",
		ReviewedDocumentIDs: []int64{11},
		ReviewContext:       selectionReviewContext(),
		ClosedDimensions:    []string{},
		SelectedBy:          "tester",
		SelectionReason:     "test",
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(result.SelectedProfiles) != 0 {
		t.Fatalf("selected profiles = %#v, want none", result.SelectedProfiles)
	}
}

// TestSelectPropagatesEnricherFailure proves a classifier/enricher failure
// aborts selection rather than silently evaluating base facts -- the operator
// must see that the scope could not be decided (spec section 11's alarm path
// is for indeterminate outcomes, not for a broken classifier).
func TestSelectPropagatesEnricherFailure(t *testing.T) {
	enricher := &stubReviewEnricher{err: errors.New("classifier down")}
	selector := Selector{
		Source: &stubSelectionSource{
			storeID:  7,
			released: selectionFixture(selectionProfile("p1", applicabilityTrueUS, "[]")),
		},
		Enricher: enricher,
	}
	_, err := selector.Select(context.Background(), SelectionRequest{
		ReviewScopeID:       "scope-abc",
		ReviewedDocumentIDs: []int64{11},
		ReviewContext:       selectionReviewContext(),
		ClosedDimensions:    []string{},
		SelectedBy:          "tester",
		SelectionReason:     "test",
		SubjectFacts:        &stubSubjectFactsLoader{facts: semrules.FactSet{}},
	})
	if err == nil {
		t.Fatal("expected Select to fail when the enricher fails")
	}
	if !strings.Contains(err.Error(), "classifier down") {
		t.Fatalf("error does not carry the enricher failure: %v", err)
	}
}
