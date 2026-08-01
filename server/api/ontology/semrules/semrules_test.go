package semrules

import (
	"encoding/json"
	"testing"
)

func TestTypedOperatorRegistrationSeesDeclaredType(t *testing.T) {
	const name = "test_declared_type"
	var got KnownValue
	if err := RegisterTypedOperator(name, func(fact KnownValue, _ any) (bool, error) {
		got = fact
		return true, nil
	}); err != nil {
		t.Fatalf("RegisterTypedOperator: %v", err)
	}

	op, ok := lookupOperator(name)
	if !ok {
		t.Fatalf("typed operator %q is not registered", name)
	}
	want := KnownValue{Type: FactTypeDate, Value: "2026-08-01"}
	if matched, err := op(want, nil); err != nil || !matched {
		t.Fatalf("typed operator result = %v, %v", matched, err)
	}
	if got.Type != want.Type || got.Value != want.Value {
		t.Fatalf("typed operator saw %#v, want %#v", got, want)
	}
}

func TestTypedOperatorEqualityDoesNotCoerceAcrossTypes(t *testing.T) {
	op, ok := lookupOperator("eq")
	if !ok {
		t.Fatal("eq operator is not registered")
	}
	if matched, err := op(KnownValue{Type: FactTypeString, Value: "1"}, 1); err == nil || matched {
		t.Fatalf("string eq number = %v, %v; want false with type error", matched, err)
	}
	if matched, err := op(KnownValue{Type: FactTypeNumber, Value: float64(0.1)}, json.Number("0.1")); err != nil || !matched {
		t.Fatalf("numeric common representation equality = %v, %v; want true", matched, err)
	}
}

func TestLegacyOperatorEvaluationPreservesCoercion(t *testing.T) {
	res, err := Evaluate(
		Predicate{Kind: "facet", Facet: "legacy_number", Op: "eq", Value: "1"},
		map[string]any{"legacy_number": 1},
	)
	if err != nil {
		t.Fatalf("legacy Evaluate: %v", err)
	}
	if !res.Value {
		t.Fatal("legacy eq must preserve its historical string coercion")
	}
}

func TestEvaluateFacetPredicates(t *testing.T) {
	facts := map[string]any{"doc_kind": "standard", "numeric_unit_density": 0.043}

	cases := []struct {
		name string
		pred Predicate
		want bool
	}{
		{"eq", Predicate{Kind: "facet", Facet: "doc_kind", Op: "eq", Value: "standard"}, true},
		{"neq", Predicate{Kind: "facet", Facet: "doc_kind", Op: "neq", Value: "manual"}, true},
		{"in", Predicate{Kind: "facet", Facet: "doc_kind", Op: "in", Value: []string{"standard", "specification"}}, true},
		{"gte", Predicate{Kind: "facet", Facet: "numeric_unit_density", Op: "gte", Value: 0.02}, true},
		{"gte-miss", Predicate{Kind: "facet", Facet: "numeric_unit_density", Op: "gte", Value: 0.1}, false},
		{"missing-fact", Predicate{Kind: "facet", Facet: "domain", Op: "exists"}, false},
	}
	for _, c := range cases {
		res, err := Evaluate(c.pred, facts)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if res.Value != c.want {
			t.Fatalf("%s: got %v want %v (trace %v)", c.name, res.Value, c.want, res.Trace)
		}
	}
}

func TestEvaluateAllAnyNot(t *testing.T) {
	facts := map[string]any{"doc_kind": "standard", "numeric_unit_density": 0.043}

	all := Predicate{Kind: "all", Items: []Predicate{
		{Kind: "facet", Facet: "doc_kind", Op: "eq", Value: "standard"},
		{Kind: "facet", Facet: "numeric_unit_density", Op: "gte", Value: 0.02},
	}}
	res, err := Evaluate(all, facts)
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if !res.Value {
		t.Fatalf("all should be true, got %v (trace %v)", res.Value, res.Trace)
	}

	anyP := Predicate{Kind: "any", Items: []Predicate{
		{Kind: "facet", Facet: "domain", Op: "eq", Value: "mechanical"},
		{Kind: "facet", Facet: "doc_kind", Op: "eq", Value: "standard"},
	}}
	res, err = Evaluate(anyP, facts)
	if err != nil {
		t.Fatalf("any: %v", err)
	}
	if !res.Value {
		t.Fatalf("any should be true, got %v", res.Value)
	}

	notP := Predicate{Kind: "not", Items: []Predicate{
		{Kind: "facet", Facet: "doc_kind", Op: "eq", Value: "manual"},
	}}
	res, err = Evaluate(notP, facts)
	if err != nil {
		t.Fatalf("not: %v", err)
	}
	if !res.Value {
		t.Fatalf("not should be true, got %v", res.Value)
	}
}

func TestRegisterOperatorIsTheSeam(t *testing.T) {
	// Seam 3: adding an operator never requires editing the mechanism.
	if err := RegisterOperator("has_prefix", func(a, b any) (bool, error) {
		return a != nil && stringContains(a.(string), b.(string)), nil
	}); err != nil {
		t.Fatalf("RegisterOperator: %v", err)
	}
	res, err := Evaluate(Predicate{Kind: "facet", Facet: "doc_kind", Op: "has_prefix", Value: "spec"}, map[string]any{"doc_kind": "specification"})
	if err != nil {
		t.Fatalf("custom op: %v", err)
	}
	if !res.Value {
		t.Fatal("custom has_prefix operator should have matched")
	}
}

func stringContains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && haystack[:len(needle)] == needle
}
