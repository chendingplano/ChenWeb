package semrules

import (
	"errors"
	"sort"
)

const (
	ReasonMatched                = "matched"
	ReasonNotMatched             = "not_matched"
	ReasonMissingFact            = "missing_fact"
	ReasonConfidenceBelowMinimum = "confidence_below_minimum"
	ReasonConflictingFact        = "conflicting_fact"
	ReasonInvalidFact            = "invalid_fact"
	ReasonOperatorError          = "operator_error"
	// ReasonInvalidPredicate marks a structurally invalid predicate node
	// (wrong child arity, unknown kind) discovered during evaluation --
	// distinct from ReasonOperatorError (a well-formed predicate whose
	// operator/authored value failed) so traces are not silently ambiguous
	// about which failure class occurred. EvaluateDocumentValidated rejects
	// these before evaluation; EvaluateDocument (legacy, unvalidated
	// callers) still tags them this way rather than degrading to
	// operator_error (P5 review 2026080302 finding P5-15).
	ReasonInvalidPredicate = "invalid_predicate"
)

// FactSet is the runtime fact input for a predicate document.
type FactSet map[string]Fact

// TraceNode is one structured evaluation step in the trace tree.
type TraceNode struct {
	Kind              string      `json:"kind"`
	Path              string      `json:"path,omitempty"`
	Op                string      `json:"op,omitempty"`
	ExpectedValue     any         `json:"expected_value,omitempty"`
	MinConfidence     *float64    `json:"min_confidence,omitempty"`
	Truth             Truth       `json:"truth"`
	ReasonCode        string      `json:"reason_code,omitempty"`
	DecisionRelevant  bool        `json:"decision_relevant"`
	ObservedFactState FactState   `json:"observed_fact_state,omitempty"`
	ObservedFactValue any         `json:"observed_fact_value,omitempty"`
	Children          []TraceNode `json:"children,omitempty"`
}

// EvaluateDocument evaluates a predicate document against typed runtime
// facts and returns a structured three-valued trace. It does not itself
// validate doc -- callers evaluating a document from an unvalidated source
// should use EvaluateDocumentValidated instead. A structurally invalid node
// (wrong child arity, unknown kind) is tagged ReasonInvalidPredicate rather
// than silently degrading to the ambiguous ReasonOperatorError.
func EvaluateDocument(doc Document, facts FactSet) Result {
	root, missing := evaluatePredicate(doc.Expression, facts, true)
	return Result{
		Truth:                        root.Truth,
		Value:                        root.Truth == TruthTrue,
		TraceTree:                    root,
		DecisionRelevantMissingPaths: orderedUniqueStrings(missing),
	}
}

// EvaluateDocumentValidated validates doc before evaluating it, returning
// Validate's error for a structurally invalid document instead of silently
// degrading to indeterminate (P5 review 2026080302 finding P5-15). Predicates
// reaching evaluation are normally already validated at authoring/compile
// time (e.g. policy_compile.go, semrules.Canonicalize); this is the
// defense-in-depth entry point for any consumer evaluating a document whose
// provenance it cannot otherwise guarantee. New consumers should prefer this
// over the legacy EvaluateDocument.
func EvaluateDocumentValidated(doc Document, facts FactSet) (Result, error) {
	if err := Validate(doc); err != nil {
		return Result{}, err
	}
	return EvaluateDocument(doc, facts), nil
}

func evaluatePredicate(predicate Predicate, facts FactSet, decisionRelevant bool) (TraceNode, []string) {
	switch predicate.Kind {
	case "all":
		return evaluateAllOrAny(predicate, facts, decisionRelevant, true)
	case "any":
		return evaluateAllOrAny(predicate, facts, decisionRelevant, false)
	case "not":
		node := TraceNode{Kind: predicate.Kind, DecisionRelevant: decisionRelevant}
		if len(predicate.Items) != 1 {
			node.Truth = TruthIndeterminate
			node.ReasonCode = ReasonInvalidPredicate
			return node, nil
		}
		childNode, missing := evaluatePredicate(predicate.Items[0], facts, decisionRelevant)
		node.Children = []TraceNode{childNode}
		switch childNode.Truth {
		case TruthTrue:
			node.Truth = TruthFalse
			node.ReasonCode = ReasonNotMatched
		case TruthFalse:
			node.Truth = TruthTrue
			node.ReasonCode = ReasonMatched
		default:
			node.Truth = TruthIndeterminate
		}
		return node, missing
	case "fact":
		return evaluateFactPredicate(predicate, facts, decisionRelevant)
	default:
		return TraceNode{
			Kind:             predicate.Kind,
			Truth:            TruthIndeterminate,
			ReasonCode:       ReasonInvalidPredicate,
			DecisionRelevant: decisionRelevant,
		}, nil
	}
}

// evaluateAllOrAny handles "all" (isAll=true) and "any" (isAll=false) with
// decision-relevance masking that depends on logical position, not authored
// position: a child's missing fact is decision-relevant only when no sibling
// already forces the node's truth value (False for "all", True for "any"),
// regardless of which child was authored first. A pre-pass over
// predicateTruth determines whether a deciding sibling exists before the
// real pass builds the trace, so reordering children never changes the
// result (P5 review 2026080302 finding P5-7).
func evaluateAllOrAny(predicate Predicate, facts FactSet, decisionRelevant, isAll bool) (TraceNode, []string) {
	node := TraceNode{Kind: predicate.Kind, DecisionRelevant: decisionRelevant}
	if len(predicate.Items) == 0 {
		if isAll {
			node.Truth = TruthTrue
			node.ReasonCode = ReasonMatched
		} else {
			node.Truth = TruthFalse
			node.ReasonCode = ReasonNotMatched
		}
		return node, nil
	}

	deciding := TruthFalse
	if !isAll {
		deciding = TruthTrue
	}

	truths := make([]Truth, len(predicate.Items))
	hasDeciding := false
	for i, child := range predicate.Items {
		truths[i] = predicateTruth(child, facts)
		if truths[i] == deciding {
			hasDeciding = true
		}
	}

	var missing []string
	sawIndeterminate := false
	for i, child := range predicate.Items {
		// A child stays relevant if it is itself the deciding value (it
		// explains the outcome) or if no sibling decides the outcome at all.
		// Only a non-deciding child (e.g. a missing fact) gets masked once a
		// deciding sibling exists -- independent of authored order.
		childDecisionRelevant := decisionRelevant && (truths[i] == deciding || !hasDeciding)
		childNode, childMissing := evaluatePredicate(child, facts, childDecisionRelevant)
		node.Children = append(node.Children, childNode)
		if childNode.DecisionRelevant {
			missing = append(missing, childMissing...)
		}
		if truths[i] == TruthIndeterminate {
			sawIndeterminate = true
		}
	}

	switch {
	case hasDeciding:
		node.Truth = deciding
		if isAll {
			node.ReasonCode = ReasonNotMatched
		} else {
			node.ReasonCode = ReasonMatched
		}
	case sawIndeterminate:
		node.Truth = TruthIndeterminate
	default:
		if isAll {
			node.Truth = TruthTrue
			node.ReasonCode = ReasonMatched
		} else {
			node.Truth = TruthFalse
			node.ReasonCode = ReasonNotMatched
		}
	}
	return node, missing
}

// predicateTruth computes a predicate's truth value without building a trace
// or collecting decision-relevant missing paths. Used only by
// evaluateAllOrAny's pre-pass to find a deciding sibling independent of
// authored order; truth computation itself does not depend on
// decisionRelevant, so calling with decisionRelevant=false is safe.
func predicateTruth(predicate Predicate, facts FactSet) Truth {
	node, _ := evaluatePredicate(predicate, facts, false)
	return node.Truth
}

func evaluateFactPredicate(predicate Predicate, facts FactSet, decisionRelevant bool) (TraceNode, []string) {
	node := TraceNode{
		Kind:             predicate.Kind,
		Path:             predicate.Path,
		Op:               predicate.Op,
		ExpectedValue:    predicate.Value,
		MinConfidence:    predicate.MinConfidence,
		DecisionRelevant: decisionRelevant,
	}
	fact, ok := facts[predicate.Path]
	if !ok || fact.State == "" {
		fact = Fact{Path: predicate.Path, State: FactMissing}
	}
	if fact.Path == "" {
		fact.Path = predicate.Path
	}
	node.ObservedFactState = fact.State
	node.ObservedFactValue = fact.Value

	switch predicate.Op {
	case "exists":
		switch fact.State {
		case FactKnown:
			if belowConfidenceThreshold(fact, predicate.MinConfidence) {
				node.Truth = TruthIndeterminate
				node.ReasonCode = ReasonConfidenceBelowMinimum
				return node, nil
			}
			node.Truth = TruthTrue
			node.ReasonCode = ReasonMatched
			return node, nil
		case FactMissing:
			node.Truth = TruthFalse
			node.ReasonCode = ReasonMissingFact
			return node, nil
		case FactConflicting:
			node.Truth = TruthIndeterminate
			node.ReasonCode = ReasonConflictingFact
			return node, nil
		case FactInvalid:
			node.Truth = TruthIndeterminate
			node.ReasonCode = ReasonInvalidFact
			return node, nil
		}
	}

	switch fact.State {
	case FactKnown:
		if belowConfidenceThreshold(fact, predicate.MinConfidence) {
			node.Truth = TruthIndeterminate
			node.ReasonCode = ReasonConfidenceBelowMinimum
			return node, nil
		}
		spec, ok := LookupFactPath(predicate.Path)
		if !ok {
			node.Truth = TruthIndeterminate
			node.ReasonCode = ReasonInvalidFact
			return node, nil
		}
		op, ok := lookupOperator(predicate.Op)
		if !ok {
			node.Truth = TruthIndeterminate
			node.ReasonCode = ReasonOperatorError
			return node, nil
		}
		matched, err := op(KnownValue{Type: spec.Type, Value: fact.Value}, predicate.Value)
		if err != nil {
			node.Truth = TruthIndeterminate
			if errors.Is(err, ErrInvalidFactValue) {
				// The OBSERVED fact value contradicts its declared type -- a
				// producer defect, not an operator/authoring defect. Spec
				// §11 keeps this indeterminate/trace-only rather than the
				// fail-closed alarm path operator_error can trigger.
				node.ReasonCode = ReasonInvalidFact
			} else {
				node.ReasonCode = ReasonOperatorError
			}
			return node, nil
		}
		if matched {
			node.Truth = TruthTrue
			node.ReasonCode = ReasonMatched
			return node, nil
		}
		node.Truth = TruthFalse
		node.ReasonCode = ReasonNotMatched
		return node, nil
	case FactMissing:
		node.Truth = TruthIndeterminate
		node.ReasonCode = ReasonMissingFact
		if decisionRelevant {
			return node, []string{predicate.Path}
		}
		return node, nil
	case FactConflicting:
		node.Truth = TruthIndeterminate
		node.ReasonCode = ReasonConflictingFact
		return node, nil
	case FactInvalid:
		node.Truth = TruthIndeterminate
		node.ReasonCode = ReasonInvalidFact
		return node, nil
	default:
		node.Truth = TruthIndeterminate
		node.ReasonCode = ReasonInvalidFact
		return node, nil
	}
}

// belowConfidenceThreshold reports whether fact fails a declared
// min_confidence bar. A fact carrying no confidence at all does not satisfy
// any declared threshold -- absence of confidence is not proof it meets one
// (P5 review 2026080302 finding P5-16). Producers of facts with genuinely
// unconditional certainty (e.g. deterministic facts computed directly from
// document metadata) must set Confidence explicitly rather than relying on
// nil to mean "certain".
func belowConfidenceThreshold(fact Fact, minimum *float64) bool {
	if minimum == nil {
		return false
	}
	if fact.Confidence == nil {
		return true
	}
	return *fact.Confidence < *minimum
}

func orderedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
