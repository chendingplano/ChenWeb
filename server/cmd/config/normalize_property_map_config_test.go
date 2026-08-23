package config

import (
	"strings"
	"testing"
)

func withPropertyMapEntries(t *testing.T, ontology, semantic map[string][]OntologyTermPropertyMapEntry) {
	t.Helper()
	oldOntology := AppConfig.OntologyTermPropertyMap
	oldSemantic := AppConfig.SemanticAssertionPropertyMap
	t.Cleanup(func() {
		AppConfig.OntologyTermPropertyMap = oldOntology
		AppConfig.SemanticAssertionPropertyMap = oldSemantic
	})
	AppConfig.OntologyTermPropertyMap = ontology
	AppConfig.SemanticAssertionPropertyMap = semantic
}

func TestValidateNormalizePropertyMaps(t *testing.T) {
	tests := []struct {
		name      string
		normalize string
		wantErr   bool
		wantMsg   string
	}{
		{name: "unset", normalize: ""},
		{name: "system", normalize: "system"},
		{name: "simple", normalize: "simple"},
		{name: "strong", normalize: "strong"},
		{name: "moderate is recognized but unimplemented", normalize: "moderate", wantErr: true, wantMsg: "no implementation yet"},
		{name: "unrecognized value", normalize: "bogus", wantErr: true, wantMsg: "unrecognized normalize value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPropertyMapEntries(t,
				map[string][]OntologyTermPropertyMapEntry{
					"metric": {{Field: "value_class", Property: "value_class", Normalize: tt.normalize}},
				},
				map[string][]OntologyTermPropertyMapEntry{},
			)
			err := validateNormalizePropertyMaps()
			if tt.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestValidateNormalizePropertyMapsChecksSemanticAssertionSection(t *testing.T) {
	withPropertyMapEntries(t,
		map[string][]OntologyTermPropertyMapEntry{},
		map[string][]OntologyTermPropertyMapEntry{
			"metric": {{Field: "subject", Property: "subject", Normalize: "moderate"}},
		},
	)
	err := validateNormalizePropertyMaps()
	if err == nil {
		t.Fatal("expected an error for moderate in semantic_assertion_property_map, got nil")
	}
	if !strings.Contains(err.Error(), "semantic_assertion_property_map") {
		t.Fatalf("error %q does not name the semantic_assertion_property_map section", err.Error())
	}
}
