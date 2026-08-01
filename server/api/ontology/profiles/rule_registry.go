package profiles

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// ResultCategory is the closed review vocabulary from spec §12.4.
type ResultCategory string

const (
	ResultSatisfied     ResultCategory = "satisfied"
	ResultMissing       ResultCategory = "missing"
	ResultConflicting   ResultCategory = "conflicting"
	ResultNonconforming ResultCategory = "nonconforming"
	ResultInapplicable  ResultCategory = "inapplicable"
	ResultIndeterminate ResultCategory = "indeterminate"
)

// RuleEvaluationInput is the frozen review input passed to a rule evaluator.
// The scope service populates it from an immutable review scope, never from
// mutable current configuration.
type RuleEvaluationInput struct {
	Rule             ProfileRule
	ClosedDimensions map[string]bool
	Assertions       []ReviewAssertion
}

// ReviewAssertion is the stable evaluator view of an accepted assertion. It
// keeps the registry independent from the database store and makes pure-rule
// fixtures inexpensive.
type ReviewAssertion struct {
	AssertionID         int64
	SubjectObjectID     string
	PredicateTermID     string
	AssertionKindTermID string
	QuantityKindTermID  string
	UnitTermID          string
	NumericValue        *float64
	Status              string
}

// RuleEvaluationResult retains the outcome plus the assertions and reasons
// needed to persist an auditable finding.
type RuleEvaluationResult struct {
	Category         ResultCategory
	AssertionIDs     []int64
	NonconformingIDs []int64
	Reason           string
}

// RuleKind is extension seam 6. A kind is valid only when it has both the
// native evaluator and the corresponding SHACL emitter.
type RuleKind struct {
	Evaluate  func(context.Context, RuleEvaluationInput) (RuleEvaluationResult, error)
	EmitSHACL func(ProfileRule) (string, error)
}

var (
	ruleKindsMu sync.RWMutex
	ruleKinds   = map[string]RuleKind{}
)

// RegisterRuleKind registers or replaces a rule kind. Requiring the SHACL
// emitter here prevents a native-only rule from silently bypassing P7 parity.
func RegisterRuleKind(kind string, ruleKind RuleKind) error {
	kind = strings.TrimSpace(kind)
	if kind == "" || ruleKind.Evaluate == nil || ruleKind.EmitSHACL == nil {
		return errors.New("rule kind, evaluator, and SHACL emitter are required")
	}
	ruleKindsMu.Lock()
	defer ruleKindsMu.Unlock()
	ruleKinds[kind] = ruleKind
	return nil
}

// LookupRuleKind returns a registered evaluator/emitter pair.
func LookupRuleKind(kind string) (RuleKind, bool) {
	ruleKindsMu.RLock()
	defer ruleKindsMu.RUnlock()
	r, ok := ruleKinds[strings.TrimSpace(kind)]
	return r, ok
}
