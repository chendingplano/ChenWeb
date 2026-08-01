package semrules

import (
	"reflect"
	"testing"
)

func TestFactRegistryInitialPathsAndTypes(t *testing.T) {
	want := map[string]FactType{
		"document.input_doc_type":                FactTypeString,
		"document.source_language":               FactTypeString,
		"document.knowledge_store_binding_state": FactTypeString,
		"document.has_document_number":           FactTypeBoolean,
		"document.numeric_unit_density":          FactTypeNumber,
		"document.doc_kind":                      FactTypeString,
		"document.domain":                        FactTypeString,
		"document.normative_status":              FactTypeString,
		"document.jurisdiction":                  FactTypeString,
		"object.class":                           FactTypeStringSet,
		"review.as_of":                           FactTypeDate,
		"review.jurisdiction":                    FactTypeString,
		"review.operating_context":               FactTypeString,
		"review.purpose":                         FactTypeString,
		"deployment.workspace":                   FactTypeString,
		"deployment.tenant":                      FactTypeString,
		"deployment.knowledge_store":             FactTypeString,
		"deployment.user":                        FactTypeString,
		"deployment.corpus":                      FactTypeString,
	}

	got := RegisteredFactPaths()
	if len(got) != len(want) {
		t.Fatalf("registered path count = %d, want %d", len(got), len(want))
	}
	for path, wantType := range want {
		spec, ok := got[path]
		if !ok {
			t.Errorf("path %q is not registered", path)
			continue
		}
		if spec.Type != wantType {
			t.Errorf("path %q type = %q, want %q", path, spec.Type, wantType)
		}
	}

	for _, path := range []string{
		"document.doc_kind",
		"document.domain",
		"document.normative_status",
		"document.jurisdiction",
	} {
		if !got[path].Tier3Producible {
			t.Errorf("path %q must be marked tier-3-producible", path)
		}
	}
	if got["document.numeric_unit_density"].Tier3Producible {
		t.Error("deterministic numeric density must not be marked tier-3-producible")
	}

	wantNumberOps := []string{"eq", "neq", "in", "not_in", "gt", "gte", "lt", "lte", "exists"}
	if gotOps := got["document.numeric_unit_density"].Operators; !reflect.DeepEqual(gotOps, wantNumberOps) {
		t.Errorf("number operators = %q, want %q", gotOps, wantNumberOps)
	}
	wantSetOps := []string{"contains", "exists"}
	if gotOps := got["object.class"].Operators; !reflect.DeepEqual(gotOps, wantSetOps) {
		t.Errorf("string_set operators = %q, want %q", gotOps, wantSetOps)
	}
}

func TestFactRegistryRejectsDuplicatePath(t *testing.T) {
	registry := NewFactRegistry()
	spec := PathSpec{
		Path:      "document.example",
		Namespace: "document",
		Type:      FactTypeString,
		Operators: []string{"eq", "exists"},
	}
	if err := registry.Register(spec); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if err := registry.Register(spec); err == nil {
		t.Fatal("duplicate registration succeeded, want error")
	}
}

func TestFactRegistrySnapshotIsImmutable(t *testing.T) {
	registry := NewFactRegistry()
	original := PathSpec{
		Path:      "review.example",
		Namespace: "review",
		Type:      FactTypeString,
		Operators: []string{"eq", "exists"},
	}
	if err := registry.Register(original); err != nil {
		t.Fatalf("register: %v", err)
	}

	first := registry.Snapshot()
	mutated := first[original.Path]
	mutated.Type = FactTypeBoolean
	mutated.Operators[0] = "neq"
	first[original.Path] = mutated
	delete(first, original.Path)

	second := registry.Snapshot()
	if got, ok := second[original.Path]; !ok {
		t.Fatal("mutating snapshot removed registry entry")
	} else if !reflect.DeepEqual(got, original) {
		t.Fatalf("mutating snapshot changed registry entry: got %#v want %#v", got, original)
	}
}
