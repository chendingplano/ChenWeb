package docprocessing

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/spf13/viper"
)

type runtimeSelectionProcessor string

func (p runtimeSelectionProcessor) Name() string                              { return string(p) }
func (p runtimeSelectionProcessor) HandleEvent(context.Context, []byte) error { return nil }

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

func TestExplicitMetricsSelectionIgnoresUnselectedFixedServiceConfigs(t *testing.T) {
	fixed := &FixedSizeChunkingService{
		SummaryPromptErr:    errors.New("summary prompt unavailable"),
		SummaryModelErr:     errors.New("summary model unavailable"),
		TranslationModelErr: errors.New("translation model unavailable"),
		FallbackModelErr:    errors.New("topic fallback unavailable"),
	}
	if err := validateFixedRuntimeConfig(fixed, []string{"extract_metrics"}, true); err != nil {
		t.Fatalf("explicit metrics validation leaked unrelated config: %v", err)
	}
	if err := validateFixedRuntimeConfig(fixed, []string{"extract_metrics"}, false); err == nil {
		t.Fatal("default command validation must remain broad")
	}
}

func TestNewProductionRuntimeSuccessfulExplicitAndDefaultSelection(t *testing.T) {
	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		return productionRuntimeComponents{
			fixed: &FixedSizeChunkingService{},
			processors: []Processor{
				runtimeSelectionProcessor("static_analyzer"), runtimeSelectionProcessor("chunking"),
				runtimeSelectionProcessor("generate_summaries"), runtimeSelectionProcessor("extract_doc_metadata"),
				runtimeSelectionProcessor("extract_metrics"), runtimeSelectionProcessor("extract_provisions"),
			},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })

	explicit, err := NewProductionRuntime(ProductionRuntimeOptions{RequiredProcessors: []string{"extract_metrics"}})
	if err != nil {
		t.Fatalf("explicit builder: %v", err)
	}
	if got, want := processorNames(explicit.Processors), []string{"static_analyzer", "chunking", "extract_metrics"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit processors=%v want=%v", got, want)
	}

	viper.Set("doc-processing.required_processors", []string{"extract_provisions"})
	t.Cleanup(func() { viper.Set("doc-processing.required_processors", nil) })
	defaults, err := NewProductionRuntime()
	if err != nil {
		t.Fatalf("default builder: %v", err)
	}
	if got, want := processorNames(defaults.Processors), []string{"static_analyzer", "chunking", "extract_doc_metadata", "extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default processors=%v want=%v", got, want)
	}
}

func processorNames(processors []Processor) []string {
	out := make([]string, len(processors))
	for i, p := range processors {
		out[i] = p.Name()
	}
	return out
}
