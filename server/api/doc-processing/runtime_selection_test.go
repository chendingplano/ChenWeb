package docprocessing

import (
	"reflect"
	"testing"
)

func TestProductionRuntimeSelectedProcessorDependencyClosure(t *testing.T) {
	tests := []struct {
		name      string
		requested []string
		want      []string
	}{
		{"chunking", []string{"chunking"}, []string{"static_analyzer", "chunking", "extract_doc_metadata"}},
		{"metrics", []string{"extract_metrics"}, []string{"static_analyzer", "chunking", "extract_doc_metadata", "extract_metrics"}},
		{"preserves optional selection", []string{"extract_entity", "extract_relation"}, []string{"static_analyzer", "chunking", "extract_doc_metadata", "extract_entity", "extract_relation"}},
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
