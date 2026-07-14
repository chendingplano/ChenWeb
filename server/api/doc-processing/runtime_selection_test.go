package docprocessing

import (
	"reflect"
	"strings"
	"testing"
)

func TestProductionRuntimeSelectedProcessorDependencyClosure(t *testing.T) {
	tests := []struct {
		name      string
		requested []string
		want      []string
	}{
		{"chunking", []string{"chunking"}, []string{"static_analyzer", "chunking"}},
		{"metrics", []string{"extract_metrics"}, []string{"static_analyzer", "chunking", "extract_metrics"}},
		{"preserves optional selection", []string{"extract_entity", "extract_relation"}, []string{"static_analyzer", "chunking", "extract_entity", "extract_relation"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveRequiredProcessors(tc.requested)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
			for _, unrelated := range []string{"generate_summaries", "generate_topics", "extract_entities", "extract_relations", "extract_inventory_items", "extract_provisions"} {
				for _, name := range got {
					if name == unrelated {
						t.Fatalf("unexpected optional processor %q in %v", unrelated, got)
					}
				}
			}
		})
	}
}

func TestNewProductionRuntimeOptionsRejectUnknownExplicitProcessorBeforeInitialization(t *testing.T) {
	_, err := NewProductionRuntime(ProductionRuntimeOptions{RequiredProcessors: []string{"not_a_processor"}})
	if err == nil || !strings.Contains(err.Error(), "not_a_processor") {
		t.Fatalf("err=%v", err)
	}
}
