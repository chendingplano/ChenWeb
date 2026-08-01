package profiles

import (
	"context"
	"fmt"
)

// EvaluateRule dispatches a frozen rule through its registered kind. A draft
// or unknown rule cannot be evaluated as normative content by callers that
// use the review service's active-release loader.
func EvaluateRule(ctx context.Context, input RuleEvaluationInput) (RuleEvaluationResult, error) {
	kind, ok := LookupRuleKind(input.Rule.RuleKind)
	if !ok {
		return RuleEvaluationResult{}, fmt.Errorf("profile rule kind %q is not registered", input.Rule.RuleKind)
	}
	return kind.Evaluate(ctx, input)
}
