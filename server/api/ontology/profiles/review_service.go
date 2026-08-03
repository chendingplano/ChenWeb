package profiles

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
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
	// ApplicabilityContext is the review context used to evaluate each pinned
	// rule's own applicability predicate (spec 2026080102 section 6 last
	// paragraph). When nil, a rule with a non-trivial applicability predicate is
	// treated as inapplicable (excluded) -- a conservative default -- while a
	// rule without one is always applicable.
	ApplicabilityContext *ReviewApplicabilityContext
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
	gateFacts, err := ruleApplicabilityGateFacts(scope, s.ApplicabilityContext, inputRecordID)
	if err != nil {
		return nil, err
	}
	applicability, decisionRelevant, err := ruleApplicabilityGate(scope, gateFacts)
	if err != nil {
		return nil, err
	}
	profileOutcomes, err := pinnedProfileOutcomes(scope)
	if err != nil {
		return nil, err
	}
	results := make([]RuleEvaluationResult, 0, len(rules))
	for _, rule := range rules {
		if profileOutcomes[profileKey(rule.ProfileID, rule.ProfileVersion)] == semrules.TruthIndeterminate {
			// The rule's own profile was pinned only because it is
			// indeterminate on a decision-relevant closed dimension for
			// every subject (P5 review 2026080302 finding P5-9) -- the
			// profile's membership in this scope is itself unresolved, so
			// the rule's own applicability gate and assertion-pattern
			// evaluation are skipped entirely rather than producing a
			// misleadingly normal result (spec section 11).
			result := RuleEvaluationResult{Category: ResultIndeterminate, Reason: "profile selection is indeterminate on a requested closed dimension"}
			results = append(results, result)
			if err := s.Findings.Persist(ctx, OntologyFinding{InputRecordID: inputRecordID, RunID: runID, ReviewRunID: reviewRunID, ScopeID: scope.ReviewScopeID, ProfileRuleID: rule.ID, Category: result.Category, Severity: rule.Severity, Title: rule.RuleID, Description: result.Reason}); err != nil {
				return nil, err
			}
			continue
		}
		gate, err := applicability(rule)
		if err != nil {
			return nil, err
		}
		switch gate {
		case gateInapplicable:
			// The rule's own applicability predicate is false: exclude only
			// this rule -- no result, no finding -- without adding any profile
			// or release that was not already pinned in the scope.
			continue
		case gateIndeterminate:
			if !decisionRelevant(rule) {
				// A non-decision-relevant indeterminate applicability stays
				// trace-only (spec section 11): no result, no finding.
				continue
			}
			// Decision-relevant indeterminate: emit an explicit indeterminate
			// applicability result and finding rather than a silent exclusion
			// (spec sections 6/11).
			result := RuleEvaluationResult{Category: ResultIndeterminate, Reason: "profile-rule applicability is indeterminate on a requested closed dimension"}
			results = append(results, result)
			if err := s.Findings.Persist(ctx, OntologyFinding{InputRecordID: inputRecordID, RunID: runID, ReviewRunID: reviewRunID, ScopeID: scope.ReviewScopeID, ProfileRuleID: rule.ID, Category: result.Category, Severity: rule.Severity, Title: rule.RuleID, Description: result.Reason}); err != nil {
				return nil, err
			}
			continue
		}
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

// ruleApplicabilityGate builds the per-rule applicability gate for the frozen
// scope, evaluating each pinned rule's applicability predicate against every
// frozen per-target fact base supplied by ruleApplicabilityGateFacts (one
// entry per pinned target object on the executed input record, or a single
// review-context-only entry as fallback). A rule is applicable if it is true
// for ANY pinned target -- consistent with how EvaluatePinnedScope already
// pools assertions and findings across all of a scope's targets rather than
// evaluating per target -- indeterminate if no target is true but at least
// one is indeterminate, and inapplicable only if it is false for every
// target. Matching on document id alone and consulting only the first
// snapshot entry silently picked an arbitrary target's facts to gate every
// rule (P5 review 2026080302 finding P5-8). The fact base is derived from
// the scope's own frozen fields, never from mutable current configuration,
// so a scope's applicability result stays reproducible from the scope
// record alone (acceptance criterion 14). decisionRelevant reports whether
// an indeterminate applicability is decision-relevant: a tier-3 missing path
// is decision-relevant exactly when the rule's profile closed dimensions
// intersect the scope's closed dimensions (spec section 6).
func ruleApplicabilityGate(scope ReviewScope, factsPerTarget []semrules.FactSet) (func(ProfileRule) (ruleApplicabilityGateResult, error), func(ProfileRule) bool, error) {
	profileClosed, err := pinnedProfileClosedDimensions(scope)
	if err != nil {
		return nil, nil, err
	}
	requestClosed, err := scopeClosedDimensions(scope)
	if err != nil {
		return nil, nil, err
	}

	applicability := func(rule ProfileRule) (ruleApplicabilityGateResult, error) {
		if isTrivialPredicate(rule.Applicability) {
			return gateApplicable, nil
		}
		var doc semrules.Document
		if err := json.Unmarshal(rule.Applicability, &doc); err != nil {
			return gateInapplicable, fmt.Errorf("rule %s applicability: %w", rule.RuleID, err)
		}
		sawIndeterminate := false
		for _, facts := range factsPerTarget {
			// Invalid predicates were rejected before activation (spec section
			// 11), so a malformed rule predicate here is a configuration
			// error, surfaced as an error rather than a silent exclusion.
			result, err := semrules.EvaluateDocumentValidated(doc, facts)
			if err != nil {
				return gateInapplicable, fmt.Errorf("rule %s applicability: %w", rule.RuleID, err)
			}
			switch result.Truth {
			case semrules.TruthTrue:
				return gateApplicable, nil
			case semrules.TruthIndeterminate:
				sawIndeterminate = true
			}
		}
		if sawIndeterminate {
			return gateIndeterminate, nil
		}
		return gateInapplicable, nil
	}
	decisionRelevant := func(rule ProfileRule) bool {
		// A rule inherits its profile's closed dimensions from the frozen
		// selection snapshot; an indeterminate applicability is
		// decision-relevant exactly when that profile's closed dimensions
		// intersect the scope's (spec section 6: "decision-relevant exactly
		// when the profile/request closed-dimension sets intersect").
		return slicesIntersect(profileClosed[profileKey(rule.ProfileID, rule.ProfileVersion)], requestClosed)
	}
	return applicability, decisionRelevant, nil
}

// ruleApplicabilityGateFacts resolves the fact base(s) for rule-level
// applicability: one frozen per-target fact snapshot entry per pinned target
// object on the executed input record (review context + document +
// deployment facts merged at selection time), so a rule whose applicability
// references document.*, object.*, or deployment.* (first-class paths per
// spec section 3.3) is evaluated against the facts that actually selected
// the profile for that target. Fallback: when the scope has no fact_snapshot
// or no entry matches the input record (explicit-mode scopes, and P4-era
// scopes), a single review-context-only fact set is returned, preserving the
// previous behavior exactly.
func ruleApplicabilityGateFacts(scope ReviewScope, ctx *ReviewApplicabilityContext, inputRecordID int64) ([]semrules.FactSet, error) {
	context := ReviewApplicabilityContext{}
	if ctx != nil {
		context = *ctx
	}
	if context.Jurisdiction == "" {
		context.Jurisdiction = scope.Jurisdiction
	}
	if context.AsOfDate == "" {
		context.AsOfDate = scope.AsOfDate
	}
	if context.OperatingContext == "" {
		context.OperatingContext = scope.OperatingContext
	}
	reviewFacts, err := BuildReviewContextFacts(context)
	if err != nil {
		return nil, err
	}
	frozen, ok, err := frozenSubjectFactsForDocument(scope, inputRecordID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []semrules.FactSet{reviewFacts}, nil
	}
	// The snapshot facts already include the review context (merged at
	// selection time), so the review context must not be re-added: a duplicate
	// known producer would be rejected. Use the frozen merged facts directly.
	return frozen, nil
}

// frozenSubjectFactsForDocument returns every frozen per-subject fact set
// from the scope's fact_snapshot whose subject's document id matches the
// executed input record -- one entry per pinned target object on that
// document, since a scope may pin several targets with differing per-target
// facts (e.g. object.class). ok is false when the scope has no fact_snapshot
// or no entry matches. Matching on document id alone and returning only the
// first entry silently picked an arbitrary target's facts to gate every
// rule (P5 review 2026080302 finding P5-8); ruleApplicabilityGate now
// consults every returned entry instead.
func frozenSubjectFactsForDocument(scope ReviewScope, inputRecordID int64) ([]semrules.FactSet, bool, error) {
	if len(scope.FactSnapshot) == 0 {
		return nil, false, nil
	}
	var snapshots []SubjectFactSnapshot
	if err := json.Unmarshal(scope.FactSnapshot, &snapshots); err != nil {
		return nil, false, fmt.Errorf("fact_snapshot: %w", err)
	}
	var matches []semrules.FactSet
	for _, entry := range snapshots {
		if entry.Subject.DocumentID == inputRecordID {
			matches = append(matches, entry.Facts)
		}
	}
	if len(matches) == 0 {
		return nil, false, nil
	}
	return matches, true, nil
}

// scopeClosedDimensions returns the request's closed dimensions frozen in the
// scope.
func scopeClosedDimensions(scope ReviewScope) (map[string]bool, error) {
	var dimensions []string
	if err := json.Unmarshal(scope.ClosedDimensions, &dimensions); err != nil {
		return nil, fmt.Errorf("closed_dimensions: %w", err)
	}
	set := make(map[string]bool, len(dimensions))
	for _, d := range dimensions {
		set[d] = true
	}
	return set, nil
}

// profileKey identifies a pinned profile within a scope by identity, so a
// rule's closed-dimension decision relevance resolves against the frozen
// snapshot rather than any mutable profile content.
func profileKey(profileID string, profileVersion int) string {
	return fmt.Sprintf("%s:%d", profileID, profileVersion)
}

func slicesIntersect(values []string, set map[string]bool) bool {
	for _, value := range values {
		if set[value] {
			return true
		}
	}
	return false
}

type ruleApplicabilityGateResult int

const (
	gateApplicable ruleApplicabilityGateResult = iota
	gateInapplicable
	gateIndeterminate
)

// pinnedProfileClosedDimensions reads the frozen profile/release entries from
// the scope's selection_snapshot, keyed by profile identity, and returns each
// profile's closed dimensions. A P4 explicit scope (no selection_snapshot) has
// no profile closed dimensions, so no indeterminate applicability is ever
// decision relevant there -- the explicit mode is preserved exactly.
func pinnedProfileClosedDimensions(scope ReviewScope) (map[string][]string, error) {
	if len(scope.SelectionSnapshot) == 0 {
		return map[string][]string{}, nil
	}
	var snapshot SelectionSnapshot
	if err := json.Unmarshal(scope.SelectionSnapshot, &snapshot); err != nil {
		return nil, fmt.Errorf("selection_snapshot: %w", err)
	}
	closed := make(map[string][]string, len(snapshot.Selected))
	for _, entry := range snapshot.Selected {
		closed[profileKey(entry.ProfileID, entry.ProfileVersion)] = entry.ClosedDimensions
	}
	return closed, nil
}

// pinnedProfileOutcomes reads the frozen profile/release entries from the
// scope's selection_snapshot, keyed by profile identity, and returns each
// profile's selection Outcome. A profile absent from the result (P4 explicit
// scopes, and any profile pinned before the Outcome field existed) is the
// zero value, never semrules.TruthIndeterminate, so EvaluateAndPersist falls
// through to normal rule evaluation for it -- preserving existing behavior
// exactly (P5 review 2026080302 finding P5-9).
func pinnedProfileOutcomes(scope ReviewScope) (map[string]semrules.Truth, error) {
	if len(scope.SelectionSnapshot) == 0 {
		return map[string]semrules.Truth{}, nil
	}
	var snapshot SelectionSnapshot
	if err := json.Unmarshal(scope.SelectionSnapshot, &snapshot); err != nil {
		return nil, fmt.Errorf("selection_snapshot: %w", err)
	}
	outcomes := make(map[string]semrules.Truth, len(snapshot.Selected))
	for _, entry := range snapshot.Selected {
		outcomes[profileKey(entry.ProfileID, entry.ProfileVersion)] = entry.Outcome
	}
	return outcomes, nil
}
