package profiles

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRequiredAssertionPatternMissingOnlyInClosedDimension(t *testing.T) {
	rule := ProfileRule{
		RuleKind: "required_assertion_pattern",
		RuleConfig: json.RawMessage(`{
			"dimension":"display_metrics",
			"predicate_term_id":"measurement:luminance",
			"assertion_kind_term_id":"measurement:lower_bound_requirement",
			"quantifier":"exists_conforming"
		}`),
	}

	closed, err := EvaluateRule(context.Background(), RuleEvaluationInput{
		Rule: rule, ClosedDimensions: map[string]bool{"display_metrics": true},
	})
	if err != nil {
		t.Fatalf("closed EvaluateRule: %v", err)
	}
	if closed.Category != ResultMissing {
		t.Fatalf("closed result = %#v, want missing", closed)
	}

	open, err := EvaluateRule(context.Background(), RuleEvaluationInput{
		Rule: rule, ClosedDimensions: map[string]bool{},
	})
	if err != nil {
		t.Fatalf("open EvaluateRule: %v", err)
	}
	if open.Category != ResultIndeterminate {
		t.Fatalf("open result = %#v, want indeterminate", open)
	}
}

func TestRequiredAssertionPatternExistsConformingSatisfiesWithAcceptedMatch(t *testing.T) {
	value := 250.0
	result, err := EvaluateRule(context.Background(), RuleEvaluationInput{
		Rule: ProfileRule{
			RuleKind: "required_assertion_pattern",
			RuleConfig: json.RawMessage(`{
				"dimension":"display_metrics",
				"predicate_term_id":"measurement:luminance",
				"assertion_kind_term_id":"measurement:lower_bound_requirement",
				"quantifier":"exists_conforming"
			}`),
		},
		ClosedDimensions: map[string]bool{"display_metrics": true},
		Assertions: []ReviewAssertion{{
			AssertionID: 7, PredicateTermID: "measurement:luminance", AssertionKindTermID: "measurement:lower_bound_requirement", NumericValue: &value, Status: "accepted",
		}},
	})
	if err != nil {
		t.Fatalf("EvaluateRule: %v", err)
	}
	if result.Category != ResultSatisfied || len(result.AssertionIDs) != 1 || result.AssertionIDs[0] != 7 {
		t.Fatalf("result = %#v", result)
	}
}
