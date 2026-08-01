package profiles

import (
	"context"
	"encoding/json"
	"strings"
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

func TestRequiredAssertionPatternNoneMatchingSatisfiedWhenAbsentNonconformingWhenPresent(t *testing.T) {
	rule := ProfileRule{
		RuleKind:   "required_assertion_pattern",
		RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:banned","quantifier":"none_matching"}`),
	}

	absent, err := EvaluateRule(context.Background(), RuleEvaluationInput{Rule: rule, ClosedDimensions: map[string]bool{"d": true}})
	if err != nil {
		t.Fatalf("absent EvaluateRule: %v", err)
	}
	if absent.Category != ResultSatisfied {
		t.Fatalf("absent result = %#v, want satisfied", absent)
	}

	present, err := EvaluateRule(context.Background(), RuleEvaluationInput{
		Rule: rule, ClosedDimensions: map[string]bool{"d": true},
		Assertions: []ReviewAssertion{{AssertionID: 1, PredicateTermID: "measurement:banned", Status: "accepted"}},
	})
	if err != nil {
		t.Fatalf("present EvaluateRule: %v", err)
	}
	if present.Category != ResultNonconforming || len(present.AssertionIDs) != 1 {
		t.Fatalf("present result = %#v, want nonconforming with the matched assertion", present)
	}
}

func TestRequiredAssertionPatternCountConformingChecksCardinality(t *testing.T) {
	rule := ProfileRule{
		RuleKind:   "required_assertion_pattern",
		RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:x","quantifier":"count_conforming","minimum":1,"maximum":1}`),
	}
	twoMatches := []ReviewAssertion{
		{AssertionID: 1, PredicateTermID: "measurement:x", Status: "accepted"},
		{AssertionID: 2, PredicateTermID: "measurement:x", Status: "accepted"},
	}
	result, err := EvaluateRule(context.Background(), RuleEvaluationInput{Rule: rule, ClosedDimensions: map[string]bool{"d": true}, Assertions: twoMatches})
	if err != nil {
		t.Fatalf("EvaluateRule: %v", err)
	}
	if result.Category != ResultNonconforming {
		t.Fatalf("result = %#v, want nonconforming (2 matches exceeds maximum 1)", result)
	}
}

func TestRequiredAssertionPatternAllConformingSatisfiedWhenMatchesAgree(t *testing.T) {
	a, b := 120.0, 120.0
	rule := ProfileRule{
		RuleKind:   "required_assertion_pattern",
		RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:x","quantifier":"all_conforming"}`),
	}
	result, err := EvaluateRule(context.Background(), RuleEvaluationInput{
		Rule: rule, ClosedDimensions: map[string]bool{"d": true},
		Assertions: []ReviewAssertion{
			{AssertionID: 1, PredicateTermID: "measurement:x", UnitTermID: "unit:ms", NumericValue: &a, Status: "accepted"},
			{AssertionID: 2, PredicateTermID: "measurement:x", UnitTermID: "unit:ms", NumericValue: &b, Status: "accepted"},
		},
	})
	if err != nil {
		t.Fatalf("EvaluateRule: %v", err)
	}
	if result.Category != ResultSatisfied || len(result.AssertionIDs) != 2 {
		t.Fatalf("result = %#v, want satisfied with both assertions", result)
	}
}

func TestRequiredAssertionPatternAllConformingConflictingWhenMatchesDisagree(t *testing.T) {
	a, b := 120.0, 150.0
	rule := ProfileRule{
		RuleKind:   "required_assertion_pattern",
		RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:x","quantifier":"all_conforming"}`),
	}
	result, err := EvaluateRule(context.Background(), RuleEvaluationInput{
		Rule: rule, ClosedDimensions: map[string]bool{"d": true},
		Assertions: []ReviewAssertion{
			{AssertionID: 1, PredicateTermID: "measurement:x", UnitTermID: "unit:ms", NumericValue: &a, Status: "accepted"},
			{AssertionID: 2, PredicateTermID: "measurement:x", UnitTermID: "unit:ms", NumericValue: &b, Status: "accepted"},
		},
	})
	if err != nil {
		t.Fatalf("EvaluateRule: %v", err)
	}
	if result.Category != ResultConflicting || len(result.AssertionIDs) != 2 {
		t.Fatalf("result = %#v, want conflicting with both disagreeing assertions", result)
	}
}

func TestRequiredAssertionPatternExistsConformingIgnoresDisagreementAmongMatches(t *testing.T) {
	a, b := 120.0, 150.0
	rule := ProfileRule{
		RuleKind:   "required_assertion_pattern",
		RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:x","quantifier":"exists_conforming"}`),
	}
	result, err := EvaluateRule(context.Background(), RuleEvaluationInput{
		Rule: rule, ClosedDimensions: map[string]bool{"d": true},
		Assertions: []ReviewAssertion{
			{AssertionID: 1, PredicateTermID: "measurement:x", UnitTermID: "unit:ms", NumericValue: &a, Status: "accepted"},
			{AssertionID: 2, PredicateTermID: "measurement:x", UnitTermID: "unit:ms", NumericValue: &b, Status: "accepted"},
		},
	})
	if err != nil {
		t.Fatalf("EvaluateRule: %v", err)
	}
	if result.Category != ResultSatisfied {
		t.Fatalf("result = %#v, want satisfied (exists_conforming does not require agreement)", result)
	}
}

func TestRequiredAssertionPatternUnsupportedQuantifierAlwaysErrors(t *testing.T) {
	base := `{"dimension":"d","predicate_term_id":"measurement:x","quantifier":"bogus_quantifier"}`
	rule := ProfileRule{RuleKind: "required_assertion_pattern", RuleConfig: json.RawMessage(base)}

	if _, err := EvaluateRule(context.Background(), RuleEvaluationInput{Rule: rule, ClosedDimensions: map[string]bool{"d": true}}); err == nil {
		t.Fatal("expected an error for an unsupported quantifier with zero matches, got nil")
	}
	if _, err := EvaluateRule(context.Background(), RuleEvaluationInput{
		Rule: rule, ClosedDimensions: map[string]bool{"d": true},
		Assertions: []ReviewAssertion{{AssertionID: 1, PredicateTermID: "measurement:x", Status: "accepted"}},
	}); err == nil {
		t.Fatal("expected an error for an unsupported quantifier with a match present, got nil")
	}
}

func TestRequiredAssertionPatternEmitsDeterministicSHACLShape(t *testing.T) {
	shape, err := emitRequiredAssertionPatternSHACL(ProfileRule{RuleID: "example:requires", RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:luminance","quantifier":"count_conforming","minimum":1,"maximum":2}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"@prefix sh:", "sh:NodeShape", "sh:qualifiedMinCount 1", "sh:qualifiedMaxCount 2", "measurement:luminance"} {
		if !strings.Contains(shape, want) {
			t.Fatalf("shape missing %q: %s", want, shape)
		}
	}
}

func TestRequiredAssertionPatternSHACLNoneMatchingForbidsAnyMatch(t *testing.T) {
	shape, err := emitRequiredAssertionPatternSHACL(ProfileRule{RuleID: "example:forbids", RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:banned","quantifier":"none_matching"}`)})
	if err != nil {
		t.Fatal(err)
	}
	// none_matching must forbid every match (native evaluator: any match => nonconforming),
	// not permit an unbounded count as sh:qualifiedMinCount 0 alone would.
	for _, want := range []string{"sh:qualifiedMinCount 0", "sh:qualifiedMaxCount 0"} {
		if !strings.Contains(shape, want) {
			t.Fatalf("shape missing %q: %s", want, shape)
		}
	}
}

func TestRequiredAssertionPatternSHACLIncludesAssertionAndQuantityKindFilters(t *testing.T) {
	shape, err := emitRequiredAssertionPatternSHACL(ProfileRule{RuleID: "example:requires", RuleConfig: json.RawMessage(`{
		"dimension":"d",
		"predicate_term_id":"measurement:luminance",
		"assertion_kind_term_id":"measurement:lower_bound_requirement",
		"quantity_kind_term_id":"quantity:luminance",
		"quantifier":"exists_conforming"
	}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"measurement:lower_bound_requirement", "quantity:luminance", "sh:qualifiedMinCount 1"} {
		if !strings.Contains(shape, want) {
			t.Fatalf("shape missing %q: %s", want, shape)
		}
	}
}

func TestRequiredAssertionPatternReferencedTermIDsIncludesAllConfiguredTerms(t *testing.T) {
	got, err := requiredAssertionPatternReferencedTermIDs(ProfileRule{RuleConfig: json.RawMessage(`{
		"dimension":"d",
		"predicate_term_id":"measurement:luminance",
		"assertion_kind_term_id":"measurement:lower_bound_requirement",
		"quantity_kind_term_id":"quantity:luminance",
		"quantifier":"exists_conforming"
	}`)})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"measurement:luminance": true, "measurement:lower_bound_requirement": true, "quantity:luminance": true}
	if len(got) != len(want) {
		t.Fatalf("ReferencedTermIDs = %v, want %v", got, want)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected referenced term id %q", id)
		}
	}
}

func TestRequiredAssertionPatternReferencedTermIDsOmitsUnsetOptionalFields(t *testing.T) {
	got, err := requiredAssertionPatternReferencedTermIDs(ProfileRule{RuleConfig: json.RawMessage(`{"dimension":"d","predicate_term_id":"measurement:luminance","quantifier":"exists_conforming"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "measurement:luminance" {
		t.Fatalf("ReferencedTermIDs = %v, want just the predicate term id", got)
	}
}
