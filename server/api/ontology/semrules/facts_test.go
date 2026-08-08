package semrules

import (
	"reflect"
	"testing"
)

func TestFactRegistryInitialPathsAndTypes(t *testing.T) {
	stringOps := []string{"eq", "neq", "in", "not_in", "exists"}
	numberOps := []string{"eq", "neq", "in", "not_in", "gt", "gte", "lt", "lte", "exists"}
	booleanOps := []string{"eq", "neq", "exists"}
	dateOps := []string{"eq", "neq", "in", "not_in", "gt", "gte", "lt", "lte", "exists"}
	stringSetOps := []string{"contains", "exists"}
	want := map[string]PathSpec{
		"document.input_doc_type":                {Path: "document.input_doc_type", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		"document.source_language":               {Path: "document.source_language", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		"document.knowledge_store_binding_state": {Path: "document.knowledge_store_binding_state", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		"document.has_document_number":           {Path: "document.has_document_number", Namespace: "document", Type: FactTypeBoolean, Operators: booleanOps},
		"document.numeric_unit_density":          {Path: "document.numeric_unit_density", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		"document.page_count":                    {Path: "document.page_count", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		"document.toc_presence":                  {Path: "document.toc_presence", Namespace: "document", Type: FactTypeBoolean, Operators: booleanOps},
		"document.heading_count":                 {Path: "document.heading_count", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		"document.table_line_ratio":              {Path: "document.table_line_ratio", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		"document.modal_verb_density":            {Path: "document.modal_verb_density", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		"document.figure_density":                {Path: "document.figure_density", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		"document.language_mix":                  {Path: "document.language_mix", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		"document.publish_date":                  {Path: "document.publish_date", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		"document.authority_hint":                {Path: "document.authority_hint", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		"document.doc_kind":                      {Path: "document.doc_kind", Namespace: "document", Type: FactTypeString, Operators: stringOps, Tier3Producible: true, GovernedValueScheme: "document.doc_kind"},
		"document.domain":                        {Path: "document.domain", Namespace: "document", Type: FactTypeString, Operators: stringOps, Tier3Producible: true, GovernedValueScheme: "document.domain"},
		"document.normative_status":              {Path: "document.normative_status", Namespace: "document", Type: FactTypeString, Operators: stringOps, Tier3Producible: true, GovernedValueScheme: "document.normative_status"},
		"document.jurisdiction":                  {Path: "document.jurisdiction", Namespace: "document", Type: FactTypeString, Operators: stringOps, Tier3Producible: true, GovernedValueScheme: "jurisdiction"},
		"object.class":                           {Path: "object.class", Namespace: "object", Type: FactTypeStringSet, Operators: stringSetOps},
		"review.as_of":                           {Path: "review.as_of", Namespace: "review", Type: FactTypeDate, Operators: dateOps},
		"review.jurisdiction":                    {Path: "review.jurisdiction", Namespace: "review", Type: FactTypeString, Operators: stringOps, GovernedValueScheme: "jurisdiction"},
		"review.operating_context":               {Path: "review.operating_context", Namespace: "review", Type: FactTypeString, Operators: stringOps},
		"review.purpose":                         {Path: "review.purpose", Namespace: "review", Type: FactTypeString, Operators: stringOps, GovernedValueScheme: "review.purpose"},
		"deployment.workspace":                   {Path: "deployment.workspace", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
		"deployment.tenant":                      {Path: "deployment.tenant", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
		"deployment.knowledge_store":             {Path: "deployment.knowledge_store", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
		"deployment.user":                        {Path: "deployment.user", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
		"deployment.corpus":                      {Path: "deployment.corpus", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
	}

	got := RegisteredFactPaths()
	if len(got) != len(want) {
		t.Fatalf("registered path count = %d, want %d", len(got), len(want))
	}
	for path, wantSpec := range want {
		spec, ok := got[path]
		if !ok {
			t.Errorf("path %q is not registered", path)
			continue
		}
		if !reflect.DeepEqual(spec, wantSpec) {
			t.Errorf("path %q spec = %#v, want %#v", path, spec, wantSpec)
		}
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
