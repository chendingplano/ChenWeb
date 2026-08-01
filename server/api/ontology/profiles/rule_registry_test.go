package profiles

import (
	"context"
	"testing"
)

func TestRegisterRuleKindRequiresEvaluatorAndSHACLEmitter(t *testing.T) {
	const kind = "test:required"
	if err := RegisterRuleKind(kind, RuleKind{}); err == nil {
		t.Fatal("expected missing evaluator/emitter to be rejected")
	}

	want := RuleKind{
		Evaluate: func(context.Context, RuleEvaluationInput) (RuleEvaluationResult, error) {
			return RuleEvaluationResult{Category: ResultSatisfied}, nil
		},
		EmitSHACL: func(ProfileRule) (string, error) { return "shape", nil },
	}
	if err := RegisterRuleKind(kind, want); err != nil {
		t.Fatalf("RegisterRuleKind: %v", err)
	}
	got, ok := LookupRuleKind(kind)
	if !ok || got.Evaluate == nil || got.EmitSHACL == nil {
		t.Fatalf("LookupRuleKind(%q) = %#v, %v", kind, got, ok)
	}
}
