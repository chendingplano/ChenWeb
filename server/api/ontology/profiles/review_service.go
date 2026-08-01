package profiles

import (
	"context"
	"encoding/json"
	"fmt"
)

type FindingWriter interface {
	Persist(context.Context, OntologyFinding) error
}

// ReviewService evaluates only the rules supplied from a frozen scope's
// activated profile release and persists one auditable finding per rule.
type ReviewService struct{ Findings FindingWriter }

func (s ReviewService) EvaluateAndPersist(ctx context.Context, scope ReviewScope, rules []ProfileRule, assertions []ReviewAssertion, inputRecordID, runID int64) ([]RuleEvaluationResult, error) {
	if s.Findings == nil {
		return nil, fmt.Errorf("finding writer is required")
	}
	var dimensions []string
	if err := json.Unmarshal(scope.ClosedDimensions, &dimensions); err != nil {
		return nil, fmt.Errorf("closed_dimensions: %w", err)
	}
	closed := make(map[string]bool, len(dimensions))
	for _, d := range dimensions {
		closed[d] = true
	}
	results := make([]RuleEvaluationResult, 0, len(rules))
	for _, rule := range rules {
		result, err := EvaluateRule(ctx, RuleEvaluationInput{Rule: rule, ClosedDimensions: closed, Assertions: assertions})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
		assertionID := int64(0)
		if len(result.AssertionIDs) > 0 {
			assertionID = result.AssertionIDs[0]
		}
		if err := s.Findings.Persist(ctx, OntologyFinding{InputRecordID: inputRecordID, RunID: runID, ScopeID: scope.ReviewScopeID, ProfileRuleID: rule.ID, AssertionID: assertionID, Category: result.Category, Severity: rule.Severity, Title: rule.RuleID, Description: result.Reason}); err != nil {
			return nil, err
		}
	}
	return results, nil
}
