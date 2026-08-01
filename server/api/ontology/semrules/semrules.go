// Package semrules is the DR3 applicability predicate evaluator (ADR
// 2026072901 DR3): a small predicate language evaluated against a fact set,
// shared by the extraction planner and (in P4+) the review-scope resolver.
//
// This is extension seam 3: operators are registered through
// RegisterOperator, so adding an operator never requires editing the
// mechanism. The language is intentionally minimal in P2 -- the flat-column
// pipeline-rule path stays active until P5 adopts JSONB predicates.
package semrules

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Predicate is a node in the applicability predicate tree.
type Predicate struct {
	Kind  string      `json:"kind"` // "all" | "any" | "not" | "facet" | "object_class"
	Facet string      `json:"facet,omitempty"`
	Op    string      `json:"op,omitempty"` // registered operator name, e.g. eq, in, gte
	Value any         `json:"value,omitempty"`
	Items []Predicate `json:"items,omitempty"` // children for all/any/not
}

// Operator evaluates a fact value against the predicate value.
type Operator func(fact, expected any) (bool, error)

var (
	operatorsMu sync.RWMutex
	operators   = map[string]Operator{}
)

func init() {
	for name, op := range builtinOperators {
		operators[name] = op
	}
}

// RegisterOperator adds an operator to the registry (seam 3). An operator with
// the same name is replaced.
func RegisterOperator(name string, op Operator) error {
	name = strings.TrimSpace(name)
	if name == "" || op == nil {
		return errors.New("operator name and implementation are required")
	}
	operatorsMu.Lock()
	defer operatorsMu.Unlock()
	operators[name] = op
	return nil
}

func eqValues(a, b any) (bool, error) {
	if a == nil || b == nil {
		return a == b, nil
	}
	return fmt.Sprint(a) == fmt.Sprint(b), nil
}

var builtinOperators = map[string]Operator{
	"eq": eqValues,
	"neq": func(a, b any) (bool, error) {
		eq, err := eqValues(a, b)
		return !eq, err
	},
	"in": func(a, b any) (bool, error) {
		rv := reflect.ValueOf(b)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return false, errors.New("in operator expects a slice value")
		}
		for i := 0; i < rv.Len(); i++ {
			if ok, _ := eqValues(a, rv.Index(i).Interface()); ok {
				return true, nil
			}
		}
		return false, nil
	},
	"gte": func(a, b any) (bool, error) { return compare(a, b, func(c int) bool { return c >= 0 }) },
	"gt":  func(a, b any) (bool, error) { return compare(a, b, func(c int) bool { return c > 0 }) },
	"lte": func(a, b any) (bool, error) { return compare(a, b, func(c int) bool { return c <= 0 }) },
	"lt":  func(a, b any) (bool, error) { return compare(a, b, func(c int) bool { return c < 0 }) },
	"exists": func(a, b any) (bool, error) {
		return a != nil, nil
	},
}

// Result is the outcome of evaluating a predicate: the boolean value plus a
// trace of the evaluations that produced it.
type Result struct {
	Value bool     `json:"value"`
	Trace []string `json:"trace"`
}

// Evaluate evaluates a predicate tree against a fact set. Facts are looked up
// by facet key (case-insensitive) or by the special key "object_class".
func Evaluate(pred Predicate, facts map[string]any) (Result, error) {
	res := Result{}
	var trace func(p Predicate) (bool, error)
	trace = func(p Predicate) (bool, error) {
		switch p.Kind {
		case "all":
			if len(p.Items) == 0 {
				return true, nil
			}
			for _, child := range p.Items {
				ok, err := trace(child)
				if err != nil {
					return false, err
				}
				if !ok {
					res.Trace = append(res.Trace, fmt.Sprintf("all:false (child %s)", child.Kind))
					return false, nil
				}
			}
			res.Trace = append(res.Trace, "all:true")
			return true, nil
		case "any":
			for _, child := range p.Items {
				ok, err := trace(child)
				if err != nil {
					return false, err
				}
				if ok {
					res.Trace = append(res.Trace, fmt.Sprintf("any:true (child %s)", child.Kind))
					return true, nil
				}
			}
			res.Trace = append(res.Trace, "any:false")
			return false, nil
		case "not":
			if len(p.Items) != 1 {
				return false, errors.New("not expects exactly one child")
			}
			ok, err := trace(p.Items[0])
			return !ok, err
		case "facet":
			op, ok := lookupOperator(p.Op)
			if !ok {
				return false, fmt.Errorf("unknown operator %q", p.Op)
			}
			fact := facts[p.Facet]
			if fact == nil {
				// case-insensitive fallback
				for k, v := range facts {
					if strings.EqualFold(k, p.Facet) {
						fact = v
						break
					}
				}
			}
			ok, err := op(fact, p.Value)
			res.Trace = append(res.Trace, fmt.Sprintf("%s %s %v => %v (fact=%v)", p.Facet, p.Op, p.Value, ok, fact))
			return ok, err
		default:
			return false, fmt.Errorf("unknown predicate kind %q", p.Kind)
		}
	}
	ok, err := trace(pred)
	res.Value = ok
	return res, err
}

func lookupOperator(name string) (Operator, bool) {
	operatorsMu.RLock()
	defer operatorsMu.RUnlock()
	op, ok := operators[name]
	return op, ok
}

func compare(a, b any, want func(int) bool) (bool, error) {
	var af, bf float64
	switch v := a.(type) {
	case float64:
		af = v
	case int:
		af = float64(v)
	default:
		if n, ok := toFloat(a); ok {
			af = n
		} else {
			return false, fmt.Errorf("non-numeric fact %v", a)
		}
	}
	switch v := b.(type) {
	case float64:
		bf = v
	case int:
		bf = float64(v)
	default:
		if n, ok := toFloat(b); ok {
			bf = n
		} else {
			return false, fmt.Errorf("non-numeric expected %v", b)
		}
	}
	return want(sign(af - bf)), nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	}
	return 0, false
}

func sign(x float64) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}
