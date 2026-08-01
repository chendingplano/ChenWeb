package semrules

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidateRejectsInvalidDocuments(t *testing.T) {
	confidenceBelow := -0.01
	confidenceAbove := 1.01
	tests := []struct {
		name string
		doc  Document
		want string
	}{
		{"unknown version", Document{Version: 2, Expression: Predicate{Kind: "all"}}, "version"},
		{"unknown root kind", Document{Version: 1, Expression: Predicate{Kind: "some"}}, "expression.kind"},
		{"unknown nested kind", Document{Version: 1, Expression: Predicate{Kind: "all", Items: []Predicate{{Kind: "all"}, {Kind: "some"}}}}, "expression.items[1].kind"},
		{"unknown path", factDocument("document.unknown", "eq", "x"), "expression.path"},
		{"case-sensitive path", factDocument("Document.doc_kind", "eq", "standard"), "expression.path"},
		{"unknown operator", factDocument("document.doc_kind", "matches", "standard"), "expression.op"},
		{"all has fact fields", Document{Version: 1, Expression: Predicate{Kind: "all", Path: "document.doc_kind"}}, "expression.path"},
		{"any has operator", Document{Version: 1, Expression: Predicate{Kind: "any", Op: "eq"}}, "expression.op"},
		{"not has no child", Document{Version: 1, Expression: Predicate{Kind: "not"}}, "expression.items"},
		{"not has two children", Document{Version: 1, Expression: Predicate{Kind: "not", Items: []Predicate{{Kind: "all"}, {Kind: "any"}}}}, "expression.items"},
		{"fact has children", Document{Version: 1, Expression: Predicate{Kind: "fact", Path: "document.doc_kind", Op: "exists", Items: []Predicate{{Kind: "all"}}}}, "expression.items"},
		{"fact missing path", Document{Version: 1, Expression: Predicate{Kind: "fact", Op: "eq", Value: "standard"}}, "expression.path"},
		{"fact missing operator", Document{Version: 1, Expression: Predicate{Kind: "fact", Path: "document.doc_kind", Value: "standard"}}, "expression.op"},
		{"in requires array", factDocument("document.doc_kind", "in", "standard"), "expression.value"},
		{"in must be homogeneous", factDocument("document.doc_kind", "in", []any{"standard", 1}), "expression.value[1]"},
		{"operator disallowed for type", factDocument("document.has_document_number", "gt", true), "expression.op"},
		{"string value mismatch", factDocument("document.doc_kind", "eq", 1), "expression.value"},
		{"number value mismatch", factDocument("document.numeric_unit_density", "gte", "0.2"), "expression.value"},
		{"boolean value mismatch", factDocument("document.has_document_number", "eq", "true"), "expression.value"},
		{"date value mismatch", factDocument("review.as_of", "eq", "2026-8-1"), "expression.value"},
		{"contains value mismatch", factDocument("object.class", "contains", 7), "expression.value"},
		{"exists rejects value", factDocument("document.doc_kind", "exists", "unused"), "expression.value"},
		{"non-exists requires value", factDocument("document.doc_kind", "eq", nil), "expression.value"},
		{"confidence below range", withConfidence(factDocument("document.doc_kind", "eq", "standard"), &confidenceBelow), "expression.min_confidence"},
		{"confidence above range", withConfidence(factDocument("document.doc_kind", "eq", "standard"), &confidenceAbove), "expression.min_confidence"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.doc)
			if err == nil {
				t.Fatal("Validate succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate error = %q, want path containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateAcceptsValidStructuresAndTypes(t *testing.T) {
	confidence := 0.8
	doc := Document{Version: 1, Expression: Predicate{Kind: "all", Items: []Predicate{
		{Kind: "all"},
		{Kind: "any"},
		{Kind: "not", Items: []Predicate{{Kind: "fact", Path: "document.has_document_number", Op: "eq", Value: false}}},
		{Kind: "fact", Path: "document.numeric_unit_density", Op: "in", Value: []any{json.Number("0.02"), 1}, MinConfidence: &confidence},
		{Kind: "fact", Path: "review.as_of", Op: "gte", Value: "2026-08-01"},
		{Kind: "fact", Path: "object.class", Op: "contains", Value: "component:display"},
		{Kind: "fact", Path: "deployment.workspace", Op: "exists"},
	}}}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejectsInvalidJSONNumberGrammar(t *testing.T) {
	tests := []string{"01", "+1", "1.", ".1", "1/2", "--1", "1e", "1e+"}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			err := Validate(factDocument("document.numeric_unit_density", "eq", json.Number(raw)))
			if err == nil {
				t.Fatal("Validate succeeded, want invalid JSON number error")
			}
			if !strings.Contains(err.Error(), "JSON number") {
				t.Fatalf("Validate error = %q, want JSON number grammar error", err)
			}
		})
	}
}

func TestValidateRejectsOversizedJSONNumbers(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"too many digits", "1" + strings.Repeat("0", 1024), "digits"},
		{"positive exponent", "1e10001", "exponent"},
		{"negative exponent", "1e-10001", "exponent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(factDocument("document.numeric_unit_density", "eq", json.Number(tt.raw)))
			if err == nil {
				t.Fatal("Validate succeeded, want numeric limit error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestRegisterTypedOperatorForPathsAuthorizesOnlyDeclaredPath(t *testing.T) {
	const name = "test_doc_kind_starts_with"
	t.Cleanup(func() {
		_ = defaultFactRegistry.setOperatorPaths(nil, name)
		operatorsMu.Lock()
		delete(operators, name)
		operatorsMu.Unlock()
	})
	err := RegisterTypedOperatorForPaths(name, []string{"document.doc_kind"}, func(fact KnownValue, expected any) (bool, error) {
		return strings.HasPrefix(fact.Value.(string), expected.(string)), nil
	})
	if err != nil {
		t.Fatalf("RegisterTypedOperatorForPaths: %v", err)
	}
	if err := Validate(factDocument("document.doc_kind", name, "stand")); err != nil {
		t.Fatalf("Validate authorized path: %v", err)
	}
	if err := Validate(factDocument("document.source_language", name, "en")); err == nil {
		t.Fatal("Validate accepted custom operator on another string path")
	}

	paths := RegisteredFactPaths()
	if !containsString(paths["document.doc_kind"].Operators, name) {
		t.Fatalf("document.doc_kind registry metadata does not include %q", name)
	}
	if containsString(paths["document.source_language"].Operators, name) {
		t.Fatalf("document.source_language registry metadata unexpectedly includes %q", name)
	}
}

func TestRegisterTypedOperatorForTypesDoesNotAuthorizeFactPaths(t *testing.T) {
	const name = "test_type_only_operator"
	if err := RegisterTypedOperatorForTypes(name, []FactType{FactTypeString}, func(KnownValue, any) (bool, error) {
		return true, nil
	}); err != nil {
		t.Fatalf("RegisterTypedOperatorForTypes: %v", err)
	}
	err := Validate(factDocument("document.doc_kind", name, "standard"))
	if err == nil {
		t.Fatal("Validate accepted a type-only operator without path authorization")
	}
	if !strings.Contains(err.Error(), "expression.op") {
		t.Fatalf("Validate error = %q, want local operator error", err)
	}
	if containsString(RegisteredFactPaths()["document.doc_kind"].Operators, name) {
		t.Fatalf("type-only operator %q leaked into path registry", name)
	}
	op, ok := lookupOperator(name)
	if !ok {
		t.Fatalf("evaluation operator %q is not registered", name)
	}
	if matched, err := op(KnownValue{Type: FactTypeNumber, Value: 1}, "standard"); err == nil || matched {
		t.Fatalf("type-only operator accepted undeclared runtime type: matched=%v err=%v", matched, err)
	}
}

func TestRegisterTypedOperatorRemainsEvaluationOnly(t *testing.T) {
	const name = "test_legacy_typed_evaluation_only"
	if err := RegisterTypedOperator(name, func(KnownValue, any) (bool, error) { return true, nil }); err != nil {
		t.Fatalf("RegisterTypedOperator: %v", err)
	}
	if _, ok := lookupOperator(name); !ok {
		t.Fatalf("evaluation operator %q is not registered", name)
	}
	if err := Validate(factDocument("document.doc_kind", name, "standard")); err == nil {
		t.Fatal("legacy typed registration unexpectedly authorized a P5 fact path")
	}
}

func TestRegisterTypedOperatorForTypesRejectsInvalidMetadata(t *testing.T) {
	validOperator := func(KnownValue, any) (bool, error) { return true, nil }
	tests := []struct {
		name  string
		types []FactType
		op    TypedOperator
	}{
		{"missing types", nil, validOperator},
		{"unknown type", []FactType{"imaginary"}, validOperator},
		{"missing implementation", []FactType{FactTypeString}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterTypedOperatorForTypes("test_invalid_"+strings.ReplaceAll(tt.name, " ", "_"), tt.types, tt.op)
			if err == nil {
				t.Fatal("RegisterTypedOperatorForTypes succeeded, want metadata error")
			}
		})
	}
}

func TestAnalyzeReturnsStableRequirementsAndDistinctSpecificity(t *testing.T) {
	doc := Document{Version: 1, Expression: Predicate{Kind: "all", Items: []Predicate{
		{Kind: "fact", Path: "review.purpose", Op: "eq", Value: "compliance"},
		{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"},
		{Kind: "fact", Path: "deployment.tenant", Op: "exists"},
		{Kind: "fact", Path: "document.input_doc_type", Op: "eq", Value: "pdf"},
		{Kind: "fact", Path: "document.domain", Op: "eq", Value: "mechanical"},
		{Kind: "fact", Path: "document.domain", Op: "neq", Value: "electrical"},
	}}}

	got := Analyze(doc)
	wantPaths := []string{"deployment.tenant", "document.domain", "document.input_doc_type", "review.purpose"}
	wantFacets := []string{"document.domain", "document.input_doc_type"}
	if !reflect.DeepEqual(got.RequiredPaths, wantPaths) {
		t.Fatalf("RequiredPaths = %q, want %q", got.RequiredPaths, wantPaths)
	}
	if !reflect.DeepEqual(got.RequiredDocumentFacets, wantFacets) {
		t.Fatalf("RequiredDocumentFacets = %q, want %q", got.RequiredDocumentFacets, wantFacets)
	}
	if got.Specificity != 5 {
		t.Fatalf("Specificity = %d, want 5 distinct constraints", got.Specificity)
	}
	if !got.RequiresTier3 {
		t.Fatal("RequiresTier3 = false, want true for document.domain")
	}
}

func TestAnalyzeDoesNotRequireTier3ForDeterministicFacts(t *testing.T) {
	got := Analyze(factDocument("document.input_doc_type", "eq", "pdf"))
	if got.RequiresTier3 {
		t.Fatal("RequiresTier3 = true for deterministic-only predicate")
	}
}

func factDocument(path, op string, value any) Document {
	return Document{Version: 1, Expression: Predicate{Kind: "fact", Path: path, Op: op, Value: value}}
}

func withConfidence(doc Document, confidence *float64) Document {
	doc.Expression.MinConfidence = confidence
	return doc
}
