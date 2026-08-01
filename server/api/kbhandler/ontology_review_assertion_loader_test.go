package kbhandler

import (
	"context"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
)

type fakeSubjectObjectAssertionLister struct {
	gotObjectID, gotStatus string
	rows                   []assertions.Assertion
}

func (f *fakeSubjectObjectAssertionLister) ListBySubjectObject(_ context.Context, subjectObjectID, status string) ([]assertions.Assertion, error) {
	f.gotObjectID, f.gotStatus = subjectObjectID, status
	return f.rows, nil
}

func TestReviewAssertionLoaderOnlyRequestsAcceptedAssertionsForTheGivenObject(t *testing.T) {
	value := 250.0
	fake := &fakeSubjectObjectAssertionLister{rows: []assertions.Assertion{{
		ID: 7, SubjectObjectID: "obj-1", PredicateTermID: "measurement:luminance",
		AssertionKindTermID: "measurement:lower_bound_requirement", QuantityKindTermID: "quantity:luminance",
		UnitTermID: "unit:cd_m2", NumericValue: &value, Status: "accepted",
	}}}
	loader := reviewAssertionLoader{Store: fake}
	got, err := loader.LoadAcceptedAssertions(context.Background(), "obj-1")
	if err != nil {
		t.Fatalf("LoadAcceptedAssertions: %v", err)
	}
	if fake.gotObjectID != "obj-1" || fake.gotStatus != "accepted" {
		t.Fatalf("store called with objectID=%q status=%q, want obj-1/accepted", fake.gotObjectID, fake.gotStatus)
	}
	if len(got) != 1 {
		t.Fatalf("got %d assertions, want 1", len(got))
	}
	ra := got[0]
	if ra.AssertionID != 7 || ra.PredicateTermID != "measurement:luminance" || ra.AssertionKindTermID != "measurement:lower_bound_requirement" ||
		ra.QuantityKindTermID != "quantity:luminance" || ra.UnitTermID != "unit:cd_m2" || ra.Status != "accepted" {
		t.Fatalf("translated assertion = %#v", ra)
	}
	if ra.NumericValue == nil || *ra.NumericValue != value {
		t.Fatalf("NumericValue = %v, want %v", ra.NumericValue, value)
	}
}
