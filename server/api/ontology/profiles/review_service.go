package profiles

import (
	"context"
	"encoding/json"
	"fmt"
)

const noAssertionsWatermark = "none"

type FindingWriter interface {
	Persist(context.Context, OntologyFinding) error
}

type FrozenRuleLoader interface {
	LoadReleasedRules(context.Context, string, int, int64) ([]ProfileRule, error)
}

// AssertionLoader reads governed, accepted assertions about one target
// object from kb.semantic_assertions. EvaluatePinnedScope uses it to derive
// the assertion set from the scope's own pinned target_object_ids, rather
// than trusting a caller-supplied assertion list: a review scope's result
// must be reproducible from the scope's own frozen selection, and an
// evaluation input a caller can inject is neither reproducible nor auditable.
type AssertionLoader interface {
	LoadAcceptedAssertions(ctx context.Context, objectID string) ([]ReviewAssertion, error)
}

// ReviewRunWriter records one concrete evaluation of a frozen scope, pinning
// the assertion watermark it evaluated against (kb.ontology_review_runs).
type ReviewRunWriter interface {
	CreateRun(ctx context.Context, run ReviewRun) (ReviewRun, error)
}

// ReviewService evaluates only the rules supplied from a frozen scope's
// activated profile release and persists one auditable finding per rule.
type ReviewService struct {
	Findings   FindingWriter
	Rules      FrozenRuleLoader
	Assertions AssertionLoader
	Runs       ReviewRunWriter
}

// EvaluatePinnedScope loads rules using the profile version and release id
// recorded in the scope, never the current module activation pointer, and
// loads assertions from the scope's own pinned target_object_ids, never from
// caller input. It then creates a review run pinning the highest loaded
// assertion id as the watermark, so this evaluation's result stays
// reproducible even as later assertions are accepted for the same targets.
func (s ReviewService) EvaluatePinnedScope(ctx context.Context, scope ReviewScope, inputRecordID, runID int64) ([]RuleEvaluationResult, ReviewRun, error) {
	if s.Rules == nil {
		return nil, ReviewRun{}, fmt.Errorf("frozen rule loader is required")
	}
	if s.Assertions == nil {
		return nil, ReviewRun{}, fmt.Errorf("assertion loader is required")
	}
	if s.Runs == nil {
		return nil, ReviewRun{}, fmt.Errorf("review run writer is required")
	}
	var selected []struct {
		ProfileID      string `json:"profile_id"`
		ProfileVersion int    `json:"profile_version"`
		ReleaseID      int64  `json:"release_id"`
	}
	if err := json.Unmarshal(scope.SelectedProfiles, &selected); err != nil {
		return nil, ReviewRun{}, fmt.Errorf("selected_profiles: %w", err)
	}
	var all []ProfileRule
	for _, p := range selected {
		if p.ProfileID == "" || p.ProfileVersion < 1 || p.ReleaseID == 0 {
			return nil, ReviewRun{}, fmt.Errorf("selected profile lacks pinned identity/release")
		}
		rules, err := s.Rules.LoadReleasedRules(ctx, p.ProfileID, p.ProfileVersion, p.ReleaseID)
		if err != nil {
			return nil, ReviewRun{}, err
		}
		all = append(all, rules...)
	}

	var targetObjectIDs []string
	if err := json.Unmarshal(scope.TargetObjectIDs, &targetObjectIDs); err != nil {
		return nil, ReviewRun{}, fmt.Errorf("target_object_ids: %w", err)
	}
	var assertions []ReviewAssertion
	var watermarkID int64
	for _, objectID := range targetObjectIDs {
		got, err := s.Assertions.LoadAcceptedAssertions(ctx, objectID)
		if err != nil {
			return nil, ReviewRun{}, err
		}
		assertions = append(assertions, got...)
		for _, a := range got {
			if a.AssertionID > watermarkID {
				watermarkID = a.AssertionID
			}
		}
	}
	watermark := noAssertionsWatermark
	if watermarkID > 0 {
		watermark = fmt.Sprintf("assertion:%d", watermarkID)
	}
	run, err := s.Runs.CreateRun(ctx, ReviewRun{ReviewScopeID: scope.ReviewScopeID, InputRecordID: inputRecordID, AssertionWatermark: watermark})
	if err != nil {
		return nil, ReviewRun{}, err
	}

	results, err := s.EvaluateAndPersist(ctx, scope, all, assertions, inputRecordID, runID, run.ID)
	return results, run, err
}

func (s ReviewService) EvaluateAndPersist(ctx context.Context, scope ReviewScope, rules []ProfileRule, assertions []ReviewAssertion, inputRecordID, runID, reviewRunID int64) ([]RuleEvaluationResult, error) {
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
		if err := s.Findings.Persist(ctx, OntologyFinding{InputRecordID: inputRecordID, RunID: runID, ReviewRunID: reviewRunID, ScopeID: scope.ReviewScopeID, ProfileRuleID: rule.ID, AssertionID: assertionID, Category: result.Category, Severity: rule.Severity, Title: rule.RuleID, Description: result.Reason}); err != nil {
			return nil, err
		}
	}
	return results, nil
}
