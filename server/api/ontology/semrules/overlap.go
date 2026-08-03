package semrules

import (
	"fmt"
	"sort"
)

// Overlap describes whether two predicates can both match the same fact set.
type Overlap struct {
	MayOverlap bool   `json:"may_overlap"`
	Analyzable bool   `json:"analyzable"`
	Reason     string `json:"reason"`
}

type overlapConstraint struct {
	Path   string
	Values []string
}

// AnalyzeOverlap performs a bounded, conservative overlap analysis over the
// analyzable subset: conjunctions of scalar fact predicates using eq/in.
func AnalyzeOverlap(left, right Document) Overlap {
	leftConstraints, ok, reason := analyzableConstraints(left.Expression)
	if !ok {
		return Overlap{MayOverlap: true, Analyzable: false, Reason: reason}
	}
	rightConstraints, ok, reason := analyzableConstraints(right.Expression)
	if !ok {
		return Overlap{MayOverlap: true, Analyzable: false, Reason: reason}
	}

	leftEqual := true
	unconstrained := false

	paths := make(map[string]struct{}, len(leftConstraints)+len(rightConstraints))
	for path := range leftConstraints {
		paths[path] = struct{}{}
	}
	for path := range rightConstraints {
		paths[path] = struct{}{}
	}

	for path := range paths {
		leftConstraint, leftOK := leftConstraints[path]
		rightConstraint, rightOK := rightConstraints[path]
		if !leftOK || !rightOK {
			unconstrained = true
			leftEqual = false
			continue
		}
		shared := intersectValues(leftConstraint.Values, rightConstraint.Values)
		if len(shared) == 0 {
			return Overlap{MayOverlap: false, Analyzable: true, Reason: "disjoint_path_values"}
		}
		if !sameValues(leftConstraint.Values, rightConstraint.Values) {
			leftEqual = false
		}
	}

	if leftEqual {
		return Overlap{MayOverlap: true, Analyzable: true, Reason: "equal_constraints"}
	}
	if unconstrained {
		return Overlap{MayOverlap: true, Analyzable: true, Reason: "unconstrained_path"}
	}
	// leftEqual is false and unconstrained is false: every path was
	// constrained on both sides with overlapping but non-identical value
	// sets -- the only remaining outcome the loop above can produce.
	return Overlap{MayOverlap: true, Analyzable: true, Reason: "intersecting_constraints"}
}

func analyzableConstraints(predicate Predicate) (map[string]overlapConstraint, bool, string) {
	switch predicate.Kind {
	case "all":
		constraints := map[string]overlapConstraint{}
		for _, child := range predicate.Items {
			childConstraints, ok, reason := analyzableConstraints(child)
			if !ok {
				return nil, false, reason
			}
			for path, childConstraint := range childConstraints {
				if existing, ok := constraints[path]; ok {
					shared := intersectValues(existing.Values, childConstraint.Values)
					if len(shared) == 0 {
						constraints[path] = overlapConstraint{Path: path, Values: nil}
						continue
					}
					constraints[path] = overlapConstraint{Path: path, Values: shared}
					continue
				}
				constraints[path] = childConstraint
			}
		}
		for path, constraint := range constraints {
			if len(constraint.Values) == 0 {
				return map[string]overlapConstraint{path: constraint}, true, ""
			}
		}
		return constraints, true, ""
	case "fact":
		spec, ok := LookupFactPath(predicate.Path)
		if !ok {
			return nil, false, "unknown_path"
		}
		if !isAnalyzableScalarType(spec.Type) {
			return nil, false, "unsupported_type"
		}
		values, ok, reason := analyzableValues(predicate)
		if !ok {
			return nil, false, reason
		}
		return map[string]overlapConstraint{
			predicate.Path: {Path: predicate.Path, Values: values},
		}, true, ""
	default:
		return nil, false, "unsupported_kind"
	}
}

func analyzableValues(predicate Predicate) ([]string, bool, string) {
	switch predicate.Op {
	case "eq":
		value, ok := scalarConstraintValue(predicate.Value)
		if !ok {
			return nil, false, "unsupported_value"
		}
		return []string{value}, true, ""
	case "in":
		values, err := scalarConstraintValues(predicate.Value)
		if err != nil {
			return nil, false, "unsupported_value"
		}
		return values, true, ""
	default:
		return nil, false, "unsupported_operator"
	}
}

func isAnalyzableScalarType(factType FactType) bool {
	switch factType {
	case FactTypeString, FactTypeNumber, FactTypeBoolean, FactTypeDate:
		return true
	default:
		return false
	}
}

func scalarConstraintValues(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		switch typed := value.(type) {
		case []string:
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		default:
			return nil, fmt.Errorf("unsupported constraint value type %T", value)
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("empty constraint values")
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		normalized, ok := scalarConstraintValue(item)
		if !ok {
			return nil, fmt.Errorf("unsupported constraint element %T", item)
		}
		values = append(values, normalized)
	}
	sort.Strings(values)
	values = compactSorted(values)
	return values, nil
}

func scalarConstraintValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return "s:" + typed, true
	case bool:
		if typed {
			return "b:true", true
		}
		return "b:false", true
	default:
		number, err := parseNumber(value)
		if err == nil {
			return "n:" + number.canonical, true
		}
	}
	return "", false
}

func intersectValues(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	shared := make([]string, 0)
	for _, value := range right {
		if _, ok := leftSet[value]; ok {
			shared = append(shared, value)
		}
	}
	sort.Strings(shared)
	return compactSorted(shared)
}

func sameValues(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func compactSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}
