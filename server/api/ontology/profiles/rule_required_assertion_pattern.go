package profiles

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type requiredAssertionPatternConfig struct {
	Dimension           string `json:"dimension"`
	PredicateTermID     string `json:"predicate_term_id"`
	AssertionKindTermID string `json:"assertion_kind_term_id"`
	QuantityKindTermID  string `json:"quantity_kind_term_id"`
	Quantifier          string `json:"quantifier"`
	Minimum             int    `json:"minimum"`
	Maximum             *int   `json:"maximum"`
}

// validRequiredAssertionPatternQuantifiers is checked before matching begins
// so an unrecognized quantifier always errors, even when there happen to be
// zero matching assertions (a misconfigured rule must not silently resolve
// to "missing").
var validRequiredAssertionPatternQuantifiers = map[string]bool{
	"exists_conforming": true,
	"all_conforming":    true,
	"none_matching":     true,
	"count_conforming":  true,
}

func init() {
	if err := RegisterRuleKind("required_assertion_pattern", RuleKind{
		Evaluate:          evaluateRequiredAssertionPattern,
		EmitSHACL:         emitRequiredAssertionPatternSHACL,
		ReferencedTermIDs: requiredAssertionPatternReferencedTermIDs,
	}); err != nil {
		panic(err)
	}
}

// requiredAssertionPatternReferencedTermIDs returns every governed term id
// this rule's config depends on, so the module release gate can reject a
// rule referencing a term that doesn't exist (the same guarantee already
// given to axioms and mappings).
func requiredAssertionPatternReferencedTermIDs(rule ProfileRule) ([]string, error) {
	var cfg requiredAssertionPatternConfig
	if err := json.Unmarshal(rule.RuleConfig, &cfg); err != nil {
		return nil, fmt.Errorf("required_assertion_pattern config: %w", err)
	}
	ids := make([]string, 0, 3)
	if cfg.PredicateTermID != "" {
		ids = append(ids, cfg.PredicateTermID)
	}
	if cfg.AssertionKindTermID != "" {
		ids = append(ids, cfg.AssertionKindTermID)
	}
	if cfg.QuantityKindTermID != "" {
		ids = append(ids, cfg.QuantityKindTermID)
	}
	return ids, nil
}

func evaluateRequiredAssertionPattern(_ context.Context, input RuleEvaluationInput) (RuleEvaluationResult, error) {
	var cfg requiredAssertionPatternConfig
	if err := json.Unmarshal(input.Rule.RuleConfig, &cfg); err != nil {
		return RuleEvaluationResult{}, fmt.Errorf("required_assertion_pattern config: %w", err)
	}
	if cfg.Dimension == "" || cfg.PredicateTermID == "" || cfg.Quantifier == "" {
		return RuleEvaluationResult{}, fmt.Errorf("required_assertion_pattern requires dimension, predicate_term_id, and quantifier")
	}
	if !validRequiredAssertionPatternQuantifiers[cfg.Quantifier] {
		return RuleEvaluationResult{}, fmt.Errorf("unsupported required_assertion_pattern quantifier %q", cfg.Quantifier)
	}

	matched := make([]int64, 0)
	var matchedAssertions []ReviewAssertion
	for _, a := range input.Assertions {
		if a.Status != "accepted" || a.PredicateTermID != cfg.PredicateTermID {
			continue
		}
		if cfg.AssertionKindTermID != "" && a.AssertionKindTermID != cfg.AssertionKindTermID {
			continue
		}
		if cfg.QuantityKindTermID != "" && a.QuantityKindTermID != cfg.QuantityKindTermID {
			continue
		}
		matched = append(matched, a.AssertionID)
		matchedAssertions = append(matchedAssertions, a)
	}

	if len(matched) == 0 {
		switch cfg.Quantifier {
		case "none_matching":
			return RuleEvaluationResult{Category: ResultSatisfied, Reason: "no prohibited assertion is present"}, nil
		case "count_conforming":
			if cfg.Minimum == 0 {
				return RuleEvaluationResult{Category: ResultSatisfied, Reason: "minimum cardinality is zero"}, nil
			}
		}
		if input.ClosedDimensions[cfg.Dimension] {
			return RuleEvaluationResult{Category: ResultMissing, Reason: "no qualifying assertion in a closed dimension"}, nil
		}
		return RuleEvaluationResult{Category: ResultIndeterminate, Reason: "no qualifying assertion in an open dimension"}, nil
	}

	switch cfg.Quantifier {
	case "exists_conforming":
		return RuleEvaluationResult{Category: ResultSatisfied, AssertionIDs: matched, Reason: "qualifying accepted assertion found"}, nil
	case "all_conforming":
		if assertionsAgree(matchedAssertions) {
			return RuleEvaluationResult{Category: ResultSatisfied, AssertionIDs: matched, Reason: "every matching accepted assertion agrees"}, nil
		}
		return RuleEvaluationResult{Category: ResultConflicting, AssertionIDs: matched, Reason: "matching accepted assertions disagree in unit or value"}, nil
	case "none_matching":
		return RuleEvaluationResult{Category: ResultNonconforming, AssertionIDs: matched, Reason: "prohibited assertion is present"}, nil
	default: // count_conforming
		if len(matched) < cfg.Minimum || (cfg.Maximum != nil && len(matched) > *cfg.Maximum) {
			return RuleEvaluationResult{Category: ResultNonconforming, AssertionIDs: matched, Reason: "matching assertion count violates cardinality"}, nil
		}
		return RuleEvaluationResult{Category: ResultSatisfied, AssertionIDs: matched, Reason: "matching assertion count satisfies cardinality"}, nil
	}
}

// assertionsAgree reports whether every matched assertion shares the same
// unit and numeric value. all_conforming requires this; exists_conforming
// does not, so multiple accepted assertions may legitimately disagree there.
func assertionsAgree(as []ReviewAssertion) bool {
	if len(as) <= 1 {
		return true
	}
	first := as[0]
	for _, a := range as[1:] {
		if a.UnitTermID != first.UnitTermID {
			return false
		}
		if (a.NumericValue == nil) != (first.NumericValue == nil) {
			return false
		}
		if a.NumericValue != nil && *a.NumericValue != *first.NumericValue {
			return false
		}
	}
	return true
}

// emitRequiredAssertionPatternSHACL mirrors evaluateRequiredAssertionPattern's
// native semantics as a paired SHACL shape (seam 6 requires both). It uses
// sh:qualifiedValueShape/-MinCount/-MaxCount rather than a bare sh:property
// cardinality, because the native evaluator's match set is a compound filter
// (predicate + optional assertion/quantity kind + accepted status), not a
// single-property constraint.
//
// Known parity gap: all_conforming additionally requires every matching
// assertion to agree on unit and value once at least one exists. That
// agreement check has no compact SHACL cardinality equivalent and is not
// encoded here; the emitted shape only captures its presence requirement.
func emitRequiredAssertionPatternSHACL(rule ProfileRule) (string, error) {
	var cfg requiredAssertionPatternConfig
	if err := json.Unmarshal(rule.RuleConfig, &cfg); err != nil {
		return "", err
	}
	if cfg.PredicateTermID == "" || cfg.Quantifier == "" {
		return "", fmt.Errorf("required_assertion_pattern requires predicate_term_id")
	}

	min, max := cfg.Minimum, cfg.Maximum
	switch cfg.Quantifier {
	case "exists_conforming", "all_conforming":
		min, max = 1, nil
	case "none_matching":
		min, max = 0, intPtr(0)
	}

	esc := func(s string) string { return strings.ReplaceAll(s, "\"", "\\\"") }

	var b strings.Builder
	b.WriteString("@prefix sh: <http://www.w3.org/ns/shacl#> .\n")
	b.WriteString("@prefix semos: <urn:semos:> .\n\n")
	b.WriteString("[] a sh:NodeShape ;\n  sh:property [\n    sh:path semos:assertion ;\n    sh:qualifiedValueShape [\n")
	fmt.Fprintf(&b, "      sh:property [ sh:path semos:status ; sh:hasValue \"accepted\" ] ;\n")
	fmt.Fprintf(&b, "      sh:property [ sh:path semos:predicate_term_id ; sh:hasValue \"%s\" ]", esc(cfg.PredicateTermID))
	if cfg.AssertionKindTermID != "" {
		fmt.Fprintf(&b, " ;\n      sh:property [ sh:path semos:assertion_kind_term_id ; sh:hasValue \"%s\" ]", esc(cfg.AssertionKindTermID))
	}
	if cfg.QuantityKindTermID != "" {
		fmt.Fprintf(&b, " ;\n      sh:property [ sh:path semos:quantity_kind_term_id ; sh:hasValue \"%s\" ]", esc(cfg.QuantityKindTermID))
	}
	b.WriteString("\n    ] ;\n")
	fmt.Fprintf(&b, "    sh:qualifiedMinCount %d ;\n", min)
	if max != nil {
		fmt.Fprintf(&b, "    sh:qualifiedMaxCount %d ;\n", *max)
	}
	fmt.Fprintf(&b, "    sh:message \"required_assertion_pattern %s (%s): %s\"\n  ] .\n", esc(rule.RuleID), esc(cfg.Dimension), esc(cfg.Quantifier))
	return b.String(), nil
}

func intPtr(n int) *int { return &n }
