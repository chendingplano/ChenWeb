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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Operator is the legacy, untyped operator signature retained for source
// compatibility with the original evaluator extension seam.
type Operator func(fact, expected any) (bool, error)

// LegacyOperator is an alias retained for callers that adopted the interim
// name while the P5 typed boundary was introduced.
type LegacyOperator = Operator

// TypedOperator evaluates a typed, known fact value against a predicate value.
type TypedOperator func(fact KnownValue, expected any) (bool, error)

type operatorEntry struct {
	typed  TypedOperator
	legacy Operator
}

var (
	operatorsMu sync.RWMutex
	operators   = map[string]operatorEntry{}
)

func init() {
	for name, op := range builtinOperators {
		operators[name] = operatorEntry{typed: op, legacy: legacyBuiltinOperators[name]}
	}
}

// RegisterOperator adds an operator to the registry (seam 3). An operator with
// the same name is replaced. This legacy entry point adapts untyped callers to
// the typed P5 operator boundary.
func RegisterOperator(name string, op Operator) error {
	name = strings.TrimSpace(name)
	if name == "" || op == nil {
		return errors.New("operator name and implementation are required")
	}
	operatorsMu.Lock()
	defer operatorsMu.Unlock()
	operators[name] = operatorEntry{
		typed: func(fact KnownValue, expected any) (bool, error) {
			return op(fact.Value, expected)
		},
		legacy: op,
	}
	return nil
}

// RegisterTypedOperator registers a P5 operator that receives the fact's
// declared type as well as its known value.
func RegisterTypedOperator(name string, op TypedOperator) error {
	name = strings.TrimSpace(name)
	if name == "" || op == nil {
		return errors.New("operator name and implementation are required")
	}
	operatorsMu.Lock()
	defer operatorsMu.Unlock()
	operators[name] = operatorEntry{
		typed: op,
		legacy: func(fact, expected any) (bool, error) {
			factType, err := inferLegacyFactType(fact)
			if err != nil {
				return false, err
			}
			return op(KnownValue{Type: factType, Value: fact}, expected)
		},
	}
	return nil
}

func eqValues(a, b any) (bool, error) {
	if a == nil || b == nil {
		return a == b, nil
	}
	return fmt.Sprint(a) == fmt.Sprint(b), nil
}

var builtinOperators = map[string]TypedOperator{
	"eq": typedEqual,
	"neq": func(fact KnownValue, expected any) (bool, error) {
		eq, err := typedEqual(fact, expected)
		if err != nil {
			return false, err
		}
		return !eq, nil
	},
	"in": typedIn,
	"not_in": func(fact KnownValue, expected any) (bool, error) {
		in, err := typedIn(fact, expected)
		if err != nil {
			return false, err
		}
		return !in, nil
	},
	"contains": typedContains,
	"gte": func(fact KnownValue, expected any) (bool, error) {
		return typedCompare(fact, expected, func(c int) bool { return c >= 0 })
	},
	"gt": func(fact KnownValue, expected any) (bool, error) {
		return typedCompare(fact, expected, func(c int) bool { return c > 0 })
	},
	"lte": func(fact KnownValue, expected any) (bool, error) {
		return typedCompare(fact, expected, func(c int) bool { return c <= 0 })
	},
	"lt": func(fact KnownValue, expected any) (bool, error) {
		return typedCompare(fact, expected, func(c int) bool { return c < 0 })
	},
	"exists": func(KnownValue, any) (bool, error) { return true, nil },
}

var legacyBuiltinOperators = map[string]Operator{
	"eq": eqValues,
	"neq": func(a, b any) (bool, error) {
		eq, err := eqValues(a, b)
		return !eq, err
	},
	"in": legacyIn,
	"not_in": func(a, b any) (bool, error) {
		in, err := legacyIn(a, b)
		return !in, err
	},
	"contains": func(a, b any) (bool, error) {
		rv := reflect.ValueOf(a)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return false, errors.New("contains operator expects a slice fact")
		}
		for i := 0; i < rv.Len(); i++ {
			if ok, _ := eqValues(rv.Index(i).Interface(), b); ok {
				return true, nil
			}
		}
		return false, nil
	},
	"gte": func(a, b any) (bool, error) { return compare(a, b, func(c int) bool { return c >= 0 }) },
	"gt":  func(a, b any) (bool, error) { return compare(a, b, func(c int) bool { return c > 0 }) },
	"lte": func(a, b any) (bool, error) { return compare(a, b, func(c int) bool { return c <= 0 }) },
	"lt":  func(a, b any) (bool, error) { return compare(a, b, func(c int) bool { return c < 0 }) },
	"exists": func(a, _ any) (bool, error) {
		return a != nil, nil
	},
}

func legacyIn(a, b any) (bool, error) {
	rv := reflect.ValueOf(b)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return false, errors.New("in operator expects a slice value")
	}
	for i := 0; i < rv.Len(); i++ {
		if ok, _ := eqValues(a, rv.Index(i).Interface()); ok {
			return true, nil
		}
	}
	return false, nil
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
			op, ok := lookupLegacyOperator(p.Op)
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

func lookupOperator(name string) (TypedOperator, bool) {
	operatorsMu.RLock()
	defer operatorsMu.RUnlock()
	entry, ok := operators[name]
	return entry.typed, ok
}

func lookupLegacyOperator(name string) (Operator, bool) {
	operatorsMu.RLock()
	defer operatorsMu.RUnlock()
	entry, ok := operators[name]
	return entry.legacy, ok
}

func typedEqual(fact KnownValue, expected any) (bool, error) {
	switch fact.Type {
	case FactTypeString:
		observed, ok := fact.Value.(string)
		if !ok {
			return false, fmt.Errorf("declared string fact has value of type %T", fact.Value)
		}
		want, ok := expected.(string)
		if !ok {
			return false, fmt.Errorf("string fact cannot be compared with %T", expected)
		}
		return observed == want, nil
	case FactTypeNumber:
		observed, err := numericValue(fact.Value)
		if err != nil {
			return false, fmt.Errorf("declared number fact: %w", err)
		}
		want, err := numericValue(expected)
		if err != nil {
			return false, fmt.Errorf("number expectation: %w", err)
		}
		return observed.Cmp(want) == 0, nil
	case FactTypeBoolean:
		observed, ok := fact.Value.(bool)
		if !ok {
			return false, fmt.Errorf("declared boolean fact has value of type %T", fact.Value)
		}
		want, ok := expected.(bool)
		if !ok {
			return false, fmt.Errorf("boolean fact cannot be compared with %T", expected)
		}
		return observed == want, nil
	case FactTypeDate:
		observed, err := dateValue(fact.Value)
		if err != nil {
			return false, fmt.Errorf("declared date fact: %w", err)
		}
		want, err := dateValue(expected)
		if err != nil {
			return false, fmt.Errorf("date expectation: %w", err)
		}
		return observed.Equal(want), nil
	case FactTypeStringSet:
		return false, errors.New("eq is not permitted for string_set facts")
	default:
		return false, fmt.Errorf("unknown fact type %q", fact.Type)
	}
}

func typedIn(fact KnownValue, expected any) (bool, error) {
	if fact.Type == FactTypeBoolean || fact.Type == FactTypeStringSet {
		return false, fmt.Errorf("in is not permitted for %s facts", fact.Type)
	}
	rv := reflect.ValueOf(expected)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return false, errors.New("in operator expects an array value")
	}
	for i := 0; i < rv.Len(); i++ {
		matched, err := typedEqual(fact, rv.Index(i).Interface())
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func typedContains(fact KnownValue, expected any) (bool, error) {
	if fact.Type != FactTypeStringSet {
		return false, fmt.Errorf("contains is not permitted for %s facts", fact.Type)
	}
	want, ok := expected.(string)
	if !ok {
		return false, fmt.Errorf("string_set membership cannot be compared with %T", expected)
	}
	rv := reflect.ValueOf(fact.Value)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return false, fmt.Errorf("declared string_set fact has value of type %T", fact.Value)
	}
	for i := 0; i < rv.Len(); i++ {
		item, ok := rv.Index(i).Interface().(string)
		if !ok {
			return false, fmt.Errorf("declared string_set fact contains %T", rv.Index(i).Interface())
		}
		if item == want {
			return true, nil
		}
	}
	return false, nil
}

func typedCompare(fact KnownValue, expected any, want func(int) bool) (bool, error) {
	switch fact.Type {
	case FactTypeNumber:
		observed, err := numericValue(fact.Value)
		if err != nil {
			return false, fmt.Errorf("declared number fact: %w", err)
		}
		expectedNumber, err := numericValue(expected)
		if err != nil {
			return false, fmt.Errorf("number expectation: %w", err)
		}
		return want(observed.Cmp(expectedNumber)), nil
	case FactTypeDate:
		observed, err := dateValue(fact.Value)
		if err != nil {
			return false, fmt.Errorf("declared date fact: %w", err)
		}
		expectedDate, err := dateValue(expected)
		if err != nil {
			return false, fmt.Errorf("date expectation: %w", err)
		}
		comparison := 0
		if observed.Before(expectedDate) {
			comparison = -1
		} else if observed.After(expectedDate) {
			comparison = 1
		}
		return want(comparison), nil
	default:
		return false, fmt.Errorf("ordered comparison is not permitted for %s facts", fact.Type)
	}
}

func numericValue(value any) (*big.Rat, error) {
	result := new(big.Rat)
	switch number := value.(type) {
	case json.Number:
		if _, ok := result.SetString(number.String()); !ok {
			return nil, fmt.Errorf("invalid JSON number %q", number)
		}
	case float64:
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, fmt.Errorf("invalid number %v", number)
		}
		result.SetString(strconv.FormatFloat(number, 'g', -1, 64))
	case float32:
		if math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) {
			return nil, fmt.Errorf("invalid number %v", number)
		}
		result.SetString(strconv.FormatFloat(float64(number), 'g', -1, 32))
	case int:
		result.SetInt64(int64(number))
	case int8:
		result.SetInt64(int64(number))
	case int16:
		result.SetInt64(int64(number))
	case int32:
		result.SetInt64(int64(number))
	case int64:
		result.SetInt64(number)
	case uint:
		result.SetUint64(uint64(number))
	case uint8:
		result.SetUint64(uint64(number))
	case uint16:
		result.SetUint64(uint64(number))
	case uint32:
		result.SetUint64(uint64(number))
	case uint64:
		result.SetUint64(number)
	default:
		return nil, fmt.Errorf("value has non-numeric type %T", value)
	}
	return result, nil
}

func dateValue(value any) (time.Time, error) {
	raw, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("value has non-date type %T", value)
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil || parsed.Format("2006-01-02") != raw {
		return time.Time{}, fmt.Errorf("value %q is not strict ISO YYYY-MM-DD", raw)
	}
	return parsed, nil
}

func inferLegacyFactType(value any) (FactType, error) {
	switch value.(type) {
	case string:
		return FactTypeString, nil
	case bool:
		return FactTypeBoolean, nil
	case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return FactTypeNumber, nil
	}
	rv := reflect.ValueOf(value)
	if rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) {
		return FactTypeStringSet, nil
	}
	return "", fmt.Errorf("cannot infer legacy fact type from %T", value)
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
