package semrules

import "sort"

const (
	ReasonMatched                = "matched"
	ReasonNotMatched             = "not_matched"
	ReasonMissingFact            = "missing_fact"
	ReasonConfidenceBelowMinimum = "confidence_below_minimum"
	ReasonConflictingFact        = "conflicting_fact"
	ReasonInvalidFact            = "invalid_fact"
	ReasonOperatorError          = "operator_error"
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

// EvaluateDocument evaluates a validated predicate document against typed
// runtime facts and returns a structured three-valued trace.
func EvaluateDocument(doc Document, facts FactSet) Result {
	root, missing := evaluatePredicate(doc.Expression, facts, true)
	return Result{
		Truth:                        root.Truth,
		Value:                        root.Truth == TruthTrue,
		TraceTree:                    root,
		DecisionRelevantMissingPaths: orderedUniqueStrings(missing),
	}
}

func evaluatePredicate(predicate Predicate, facts FactSet, decisionRelevant bool) (TraceNode, []string) {
	switch predicate.Kind {
	case "all":
		node := TraceNode{Kind: predicate.Kind, DecisionRelevant: decisionRelevant}
		if len(predicate.Items) == 0 {
			node.Truth = TruthTrue
			node.ReasonCode = ReasonMatched
			return node, nil
		}
		var missing []string
		sawIndeterminate := false
		masked := false
		for _, child := range predicate.Items {
			childNode, childMissing := evaluatePredicate(child, facts, decisionRelevant && !masked)
			node.Children = append(node.Children, childNode)
			if childNode.DecisionRelevant {
				missing = append(missing, childMissing...)
				switch childNode.Truth {
				case TruthFalse:
					node.Truth = TruthFalse
					node.ReasonCode = ReasonNotMatched
					masked = true
				case TruthIndeterminate:
					sawIndeterminate = true
				}
			}
		}
		if node.Truth == TruthFalse {
			return node, missing
		}
		if sawIndeterminate {
			node.Truth = TruthIndeterminate
			return node, missing
		}
		node.Truth = TruthTrue
		node.ReasonCode = ReasonMatched
		return node, missing
	case "any":
		node := TraceNode{Kind: predicate.Kind, DecisionRelevant: decisionRelevant}
		var missing []string
		sawIndeterminate := false
		masked := false
		for _, child := range predicate.Items {
			childNode, childMissing := evaluatePredicate(child, facts, decisionRelevant && !masked)
			node.Children = append(node.Children, childNode)
			if childNode.DecisionRelevant {
				missing = append(missing, childMissing...)
				switch childNode.Truth {
				case TruthTrue:
					node.Truth = TruthTrue
					node.ReasonCode = ReasonMatched
					masked = true
				case TruthIndeterminate:
					sawIndeterminate = true
				}
			}
		}
		if node.Truth == TruthTrue {
			return node, missing
		}
		if sawIndeterminate {
			node.Truth = TruthIndeterminate
			return node, missing
		}
		node.Truth = TruthFalse
		node.ReasonCode = ReasonNotMatched
		return node, missing
	case "not":
		node := TraceNode{Kind: predicate.Kind, DecisionRelevant: decisionRelevant}
		if len(predicate.Items) != 1 {
			node.Truth = TruthIndeterminate
			node.ReasonCode = ReasonOperatorError
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
			ReasonCode:       ReasonOperatorError,
			DecisionRelevant: decisionRelevant,
		}, nil
	}
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
			node.ReasonCode = ReasonOperatorError
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

func belowConfidenceThreshold(fact Fact, minimum *float64) bool {
	if minimum == nil || fact.Confidence == nil {
		return false
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
