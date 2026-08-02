package profiles

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func TestReviewContextFacts(t *testing.T) {
	facts, err := BuildReviewContextFacts(ReviewApplicabilityContext{
		AsOfDate:         "2026-08-02",
		Jurisdiction:     "US",
		OperatingContext: "production",
		Purpose:          "compliance",
		ReleaseID:        42,
	})
	if err != nil {
		t.Fatalf("BuildReviewContextFacts: %v", err)
	}
	for _, path := range []string{"review.as_of", "review.jurisdiction", "review.operating_context", "review.purpose"} {
		if facts[path].State != semrules.FactKnown {
			t.Fatalf("%s=%+v, want known", path, facts[path])
		}
	}
	if facts["review.jurisdiction"].ReleaseID != "42" || facts["review.purpose"].ReleaseID != "42" {
		t.Fatalf("governed review facts missing release pin: %+v", facts)
	}
}

func TestReviewContextFactsMarksMissingFields(t *testing.T) {
	facts, err := BuildReviewContextFacts(ReviewApplicabilityContext{})
	if err != nil {
		t.Fatalf("BuildReviewContextFacts: %v", err)
	}
	for _, path := range []string{"review.as_of", "review.jurisdiction", "review.operating_context", "review.purpose"} {
		if facts[path].State != semrules.FactMissing {
			t.Fatalf("%s=%+v, want missing", path, facts[path])
		}
	}
}
