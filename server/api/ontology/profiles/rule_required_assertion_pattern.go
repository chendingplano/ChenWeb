package profiles

import (
	"context"
	"encoding/json"
	"fmt"
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

func init() {
	if err := RegisterRuleKind("required_assertion_pattern", RuleKind{
		Evaluate:  evaluateRequiredAssertionPattern,
		EmitSHACL: emitRequiredAssertionPatternSHACL,
	}); err != nil {
		panic(err)
	}
}

func evaluateRequiredAssertionPattern(_ context.Context, input RuleEvaluationInput) (RuleEvaluationResult, error) {
	var cfg requiredAssertionPatternConfig
	if err := json.Unmarshal(input.Rule.RuleConfig, &cfg); err != nil {
		return RuleEvaluationResult{}, fmt.Errorf("required_assertion_pattern config: %w", err)
	}
	if cfg.Dimension == "" || cfg.PredicateTermID == "" || cfg.Quantifier == "" {
		return RuleEvaluationResult{}, fmt.Errorf("required_assertion_pattern requires dimension, predicate_term_id, and quantifier")
	}

	matched := make([]int64, 0)
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
	case "exists_conforming", "all_conforming":
		return RuleEvaluationResult{Category: ResultSatisfied, AssertionIDs: matched, Reason: "qualifying accepted assertion found"}, nil
	case "none_matching":
		return RuleEvaluationResult{Category: ResultNonconforming, AssertionIDs: matched, Reason: "prohibited assertion is present"}, nil
	case "count_conforming":
		if len(matched) < cfg.Minimum || (cfg.Maximum != nil && len(matched) > *cfg.Maximum) {
			return RuleEvaluationResult{Category: ResultNonconforming, AssertionIDs: matched, Reason: "matching assertion count violates cardinality"}, nil
		}
		return RuleEvaluationResult{Category: ResultSatisfied, AssertionIDs: matched, Reason: "matching assertion count satisfies cardinality"}, nil
	default:
		return RuleEvaluationResult{}, fmt.Errorf("unsupported required_assertion_pattern quantifier %q", cfg.Quantifier)
	}
}

func emitRequiredAssertionPatternSHACL(rule ProfileRule) (string, error) {
	var cfg requiredAssertionPatternConfig
	if err := json.Unmarshal(rule.RuleConfig, &cfg); err != nil {
		return "", err
	}
	if cfg.PredicateTermID == "" {
		return "", fmt.Errorf("required_assertion_pattern requires predicate_term_id")
	}
	return "# SemOS required_assertion_pattern\n# predicate: " + cfg.PredicateTermID, nil
}
