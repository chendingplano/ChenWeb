package kbhandler

import (
	"context"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
)

// subjectObjectAssertionLister is the subset of assertions.AssertionStore
// this adapter needs; assertions.AssertionStore satisfies it structurally.
// Declaring it here keeps reviewAssertionLoader testable with a fake instead
// of a full-column sqlmock fixture for kb.semantic_assertions.
type subjectObjectAssertionLister interface {
	ListBySubjectObject(ctx context.Context, subjectObjectID, status string) ([]assertions.Assertion, error)
}

// reviewAssertionLoader adapts assertions.AssertionStore to
// profiles.AssertionLoader, so ExecuteOntologyReviewScope derives its
// evaluation input from governed kb.semantic_assertions rows rather than
// trusting an assertion list supplied in the request body.
type reviewAssertionLoader struct {
	Store subjectObjectAssertionLister
}

func (l reviewAssertionLoader) LoadAcceptedAssertions(ctx context.Context, objectID string) ([]profiles.ReviewAssertion, error) {
	rows, err := l.Store.ListBySubjectObject(ctx, objectID, "accepted")
	if err != nil {
		return nil, err
	}
	out := make([]profiles.ReviewAssertion, 0, len(rows))
	for _, r := range rows {
		out = append(out, profiles.ReviewAssertion{
			AssertionID:         r.ID,
			SubjectObjectID:     r.SubjectObjectID,
			PredicateTermID:     r.PredicateTermID,
			AssertionKindTermID: r.AssertionKindTermID,
			QuantityKindTermID:  r.QuantityKindTermID,
			UnitTermID:          r.UnitTermID,
			NumericValue:        r.NumericValue,
			Status:              r.Status,
		})
	}
	return out, nil
}
