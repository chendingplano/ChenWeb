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

func TestDocPipelineModeFromEnvDefaultsToPlanOnly(t *testing.T) {
	mode, err := normalizeDocPipelineMode("")
	if err != nil {
		t.Fatalf("normalizeDocPipelineMode: %v", err)
	}
	if mode != DocPipelineModePlanOnly {
		t.Fatalf("mode=%q want=%q", mode, DocPipelineModePlanOnly)
	}

	mode, err = normalizeDocPipelineMode(" true ")
	if err != nil {
		t.Fatalf("normalizeDocPipelineMode(true): %v", err)
	}
	if mode != DocPipelineModePlanOnly {
		t.Fatalf("mode=%q want=%q", mode, DocPipelineModePlanOnly)
	}
}

func TestDocPipelineModeFromEnvResolvesEnforcedRequest(t *testing.T) {
	mode, err := normalizeDocPipelineMode("false")
	if err != nil {
		t.Fatalf("normalizeDocPipelineMode(false): %v", err)
	}
	if mode != DocPipelineModeEnforced {
		t.Fatalf("mode=%q want=%q", mode, DocPipelineModeEnforced)
	}
}

func TestDocPipelineModeFromEnvRejectsNonBoolean(t *testing.T) {
	if _, err := normalizeDocPipelineMode("plan_only"); err == nil {
		t.Fatal("expected error for non-boolean DOC_PIPELINE_PLAN_ONLY value")
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

func TestNewProductionRuntimeAcceptsEnforcedPipelineModeRequest(t *testing.T) {
	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		return productionRuntimeComponents{
			fixed:      &FixedSizeChunkingService{},
			processors: []Processor{runtimeSelectionProcessor("static_analyzer"), runtimeSelectionProcessor("chunking")},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })
	t.Setenv("DOC_PIPELINE_PLAN_ONLY", "false")

	runtime, err := NewProductionRuntime()
	if err != nil {
		t.Fatalf("NewProductionRuntime: %v", err)
	}
	if got, want := runtime.ResolvedConfig().Values["processor_pipeline_mode"], DocPipelineModeEnforced; got != want {
		t.Fatalf("processor_pipeline_mode=%v want=%v", got, want)
	}
}

func TestProductionRuntimeResolvedConfigExposesPipelineMode(t *testing.T) {
	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		return productionRuntimeComponents{
			fixed:      &FixedSizeChunkingService{},
			processors: []Processor{runtimeSelectionProcessor("static_analyzer"), runtimeSelectionProcessor("chunking")},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })

	runtime, err := NewProductionRuntime()
	if err != nil {
		t.Fatalf("NewProductionRuntime: %v", err)
	}
	if got, want := runtime.ResolvedConfig().Values["processor_pipeline_mode"], DocPipelineModePlanOnly; got != want {
		t.Fatalf("processor_pipeline_mode=%v want=%v", got, want)
	}
}

func TestNewProductionRuntimeReordersSelectedProcessorsToLegacyExecutionOrder(t *testing.T) {
	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		return productionRuntimeComponents{
			fixed: &FixedSizeChunkingService{},
			processors: []Processor{
				runtimeSelectionProcessor("static_analyzer"),
				runtimeSelectionProcessor("chunking"),
				runtimeSelectionProcessor("generate_topics"),
				runtimeSelectionProcessor("extract_doc_metadata"),
				runtimeSelectionProcessor("extract_provisions"),
			},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })

	runtime, err := NewProductionRuntime(ProductionRuntimeOptions{RequiredProcessors: []string{"generate_topics", "extract_doc_metadata", "extract_provisions"}})
	if err != nil {
		t.Fatalf("NewProductionRuntime: %v", err)
	}

	if got, want := processorNames(runtime.Processors), []string{"static_analyzer", "chunking", "extract_doc_metadata", "generate_topics", "extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime processors=%v want=%v", got, want)
	}

	if got, want := runtime.Plan().ExecutionOrder(), []string{"static_analyzer", "chunking", "extract_doc_metadata", "generate_topics", "extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime plan execution order=%v want=%v", got, want)
	}

	cfg := runtime.ResolvedConfig()
	rawSteps, ok := cfg.Values["processor_plan_steps"]
	if !ok {
		t.Fatal("resolved config missing processor_plan_steps")
	}
	steps, ok := rawSteps.([]ProcessorPlanStep)
	if !ok {
		t.Fatalf("processor_plan_steps has type %T", rawSteps)
	}
	if got, want := len(steps), 5; got != want {
		t.Fatalf("processor_plan_steps len=%d want=%d", got, want)
	}
	if got, want := steps[2].Name, "extract_doc_metadata"; got != want {
		t.Fatalf("processor_plan_steps[2].Name=%q want=%q", got, want)
	}
}

func TestNewProductionRuntimeCarriesPlanFacts(t *testing.T) {
	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		return productionRuntimeComponents{
			fixed: &FixedSizeChunkingService{},
			processors: []Processor{
				runtimeSelectionProcessor("static_analyzer"),
				runtimeSelectionProcessor("chunking"),
				runtimeSelectionProcessor("generate_topics"),
				runtimeSelectionProcessor("extract_provisions"),
			},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })

	facts := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics", "extract_provisions"},
		KnowledgeStoreID:    42,
		KnowledgeStoreType:  "research",
		RequestedPipeline:   "legacy_default",
	}

	runtime, err := NewProductionRuntime(ProductionRuntimeOptions{
		RequiredProcessors: facts.RequestedProcessors,
		PlanFacts:          facts,
	})
	if err != nil {
		t.Fatalf("NewProductionRuntime: %v", err)
	}

	if got, want := runtime.Plan().Facts(), facts; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime plan facts=%#v want=%#v", got, want)
	}

	cfg := runtime.ResolvedConfig()
	rawFacts, ok := cfg.Values["processor_plan_facts"]
	if !ok {
		t.Fatal("resolved config missing processor_plan_facts")
	}
	gotFacts, ok := rawFacts.(ProductionPlanFacts)
	if !ok {
		t.Fatalf("processor_plan_facts has type %T", rawFacts)
	}
	if !reflect.DeepEqual(gotFacts, facts) {
		t.Fatalf("processor_plan_facts=%#v want=%#v", gotFacts, facts)
	}
	rawSelection, ok := cfg.Values["processor_pipeline_selection"]
	if !ok {
		t.Fatal("resolved config missing processor_pipeline_selection")
	}
	gotSelection, ok := rawSelection.(ProductionPipelineSelection)
	if !ok {
		t.Fatalf("processor_pipeline_selection has type %T", rawSelection)
	}
	wantSelection := ProductionPipelineSelection{
		PipelineName: "legacy_default",
		Reason:       "explicit_request",
	}
	if !reflect.DeepEqual(gotSelection, wantSelection) {
		t.Fatalf("processor_pipeline_selection=%#v want=%#v", gotSelection, wantSelection)
	}
	rawSpec, ok := cfg.Values["processor_pipeline_spec"]
	if !ok {
		t.Fatal("resolved config missing processor_pipeline_spec")
	}
	gotSpec, ok := rawSpec.(ProductionPipelineSpec)
	if !ok {
		t.Fatalf("processor_pipeline_spec has type %T", rawSpec)
	}
	wantSpec := ProductionPipelineSpec{
		Name:             "legacy_default",
		DisplayName:      "Legacy Default",
		LegacyEquivalent: true,
	}
	if !reflect.DeepEqual(gotSpec, wantSpec) {
		t.Fatalf("processor_pipeline_spec=%#v want=%#v", gotSpec, wantSpec)
	}
	rawBinding, ok := cfg.Values["processor_pipeline_binding"]
	if !ok {
		t.Fatal("resolved config missing processor_pipeline_binding")
	}
	gotBinding, ok := rawBinding.(ProductionPipelineBindingResolution)
	if !ok {
		t.Fatalf("processor_pipeline_binding has type %T", rawBinding)
	}
	wantBinding := ProductionPipelineBindingResolution{
		RequestedPipeline: "legacy_default",
		Source:            "explicit_request",
		SelectedPipeline:  "legacy_default",
	}
	if !reflect.DeepEqual(gotBinding, wantBinding) {
		t.Fatalf("processor_pipeline_binding=%#v want=%#v", gotBinding, wantBinding)
	}
}

func TestResolveProductionPipelineSelectionDefaultsToLegacyPipeline(t *testing.T) {
	got, err := ResolveProductionPipelineSelection(ProductionPlanFacts{})
	if err != nil {
		t.Fatalf("ResolveProductionPipelineSelection: %v", err)
	}
	want := ProductionPipelineSelection{
		PipelineName: "legacy_default",
		Reason:       "system_default",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selection=%#v want=%#v", got, want)
	}
}

func TestResolveProductionPipelineSelectionHonorsPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		facts ProductionPlanFacts
		want  ProductionPipelineSelection
	}{
		{
			name: "explicit request wins",
			facts: ProductionPlanFacts{
				RequestedPipeline:  "request_override",
				StoreBoundPipeline: "store_default",
			},
			want: ProductionPipelineSelection{PipelineName: "request_override", Reason: "explicit_request"},
		},
		{
			name: "store binding beats default",
			facts: ProductionPlanFacts{
				StoreBoundPipeline: "store_default",
			},
			want: ProductionPipelineSelection{PipelineName: "store_default", Reason: "knowledge_store_binding"},
		},
		{
			name:  "default fallback",
			facts: ProductionPlanFacts{},
			want:  ProductionPipelineSelection{PipelineName: "legacy_default", Reason: "system_default"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveProductionPipelineSelection(tc.facts)
			if err != nil {
				t.Fatalf("ResolveProductionPipelineSelection: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("selection=%#v want=%#v", got, tc.want)
			}
		})
	}
}

func TestResolveProductionPipelineSelectionRejectsUnknownPipeline(t *testing.T) {
	_, err := ResolveProductionPipelineSelection(ProductionPlanFacts{RequestedPipeline: "missing_pipeline"})
	if err == nil || !strings.Contains(err.Error(), "missing_pipeline") {
		t.Fatalf("err=%v", err)
	}
}

func TestLookupProductionPipelineReturnsStructuredSpec(t *testing.T) {
	got, ok := LookupProductionPipeline("legacy_default")
	if !ok {
		t.Fatal("expected legacy_default pipeline")
	}
	want := ProductionPipelineSpec{
		Name:             "legacy_default",
		DisplayName:      "Legacy Default",
		LegacyEquivalent: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pipeline spec=%#v want=%#v", got, want)
	}
}

func TestSetProductionPipelineRegistryOverridesLookupAndResetsToDefault(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })

	if _, ok := LookupProductionPipeline("authored_only"); ok {
		t.Fatal("did not expect authored_only before registry is installed")
	}

	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "authored_only", DisplayName: "Authored Only", LegacyEquivalent: true},
	})

	got, ok := LookupProductionPipeline("authored_only")
	if !ok {
		t.Fatal("expected authored_only after installing override registry")
	}
	want := ProductionPipelineSpec{Name: "authored_only", DisplayName: "Authored Only", LegacyEquivalent: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pipeline spec=%#v want=%#v", got, want)
	}
	if _, ok := LookupProductionPipeline("legacy_default"); ok {
		t.Fatal("legacy_default should not resolve once the override registry replaced the fallback")
	}

	SetProductionPipelineRegistry(nil)
	if _, ok := LookupProductionPipeline("legacy_default"); !ok {
		t.Fatal("expected legacy_default to resolve again after resetting the registry override")
	}
}

func TestResolveProductionPipelineResolutionReturnsSelectionAndSpec(t *testing.T) {
	got, err := ResolveProductionPipelineResolution(ProductionPlanFacts{
		RequestedPipeline: "request_override",
	})
	if err != nil {
		t.Fatalf("ResolveProductionPipelineResolution: %v", err)
	}
	want := ProductionPipelineResolution{
		Binding: ProductionPipelineBindingResolution{
			RequestedPipeline: "request_override",
			Source:            "explicit_request",
			SelectedPipeline:  "request_override",
		},
		Selection: ProductionPipelineSelection{
			PipelineName: "request_override",
			Reason:       "explicit_request",
		},
		Spec: ProductionPipelineSpec{
			Name:        "request_override",
			DisplayName: "Request Override",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pipeline resolution=%#v want=%#v", got, want)
	}
}

func TestResolveProductionPipelineBindingReturnsBindingRecord(t *testing.T) {
	got, err := ResolveProductionPipelineBinding(ProductionPlanFacts{
		RequestedPipeline:  "request_override",
		StoreBoundPipeline: "store_default",
	})
	if err != nil {
		t.Fatalf("ResolveProductionPipelineBinding: %v", err)
	}
	want := ProductionPipelineBindingResolution{
		RequestedPipeline:  "request_override",
		StoreBoundPipeline: "store_default",
		Source:             "explicit_request",
		SelectedPipeline:   "request_override",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pipeline binding=%#v want=%#v", got, want)
	}
}

func TestNewProductionRuntimeWiresPlanStore(t *testing.T) {
	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		return productionRuntimeComponents{
			fixed: &FixedSizeChunkingService{},
			processors: []Processor{
				runtimeSelectionProcessor("static_analyzer"),
				runtimeSelectionProcessor("chunking"),
				runtimeSelectionProcessor("extract_metrics"),
			},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })

	runtime, err := NewProductionRuntime(ProductionRuntimeOptions{RequiredProcessors: []string{"extract_metrics"}})
	if err != nil {
		t.Fatalf("NewProductionRuntime: %v", err)
	}
	if runtime.Control == nil || runtime.Control.PlanStore == nil {
		t.Fatal("expected production runtime to wire a plan store")
	}
}

func TestBuildProductionProcessorPlanMatchesLegacySelectionAndPhases(t *testing.T) {
	plan, err := BuildProductionProcessorPlan([]string{"extract_metrics", "extract_relation"})
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlan: %v", err)
	}

	if got, want := plan.AllNames(), []string{"static_analyzer", "chunking", "extract_relation", "extract_metrics"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("all names=%v want=%v", got, want)
	}

	if got, want := plan.PhaseNames("A"), []string{"static_analyzer", "chunking"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("phase A=%v want=%v", got, want)
	}

	if got, want := plan.PhaseNames("B"), []string{"extract_relation", "extract_metrics"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("phase B=%v want=%v", got, want)
	}

	if got := plan.PhaseNames("C"); len(got) != 0 {
		t.Fatalf("phase C=%v want empty", got)
	}
}

func TestProductionProcessorSpecsStayInSyncWithLegacyPhaseClassifier(t *testing.T) {
	for _, spec := range productionProcessorSpecs {
		gotPhaseA := isPhaseAProcessor(spec.Name)
		wantPhaseA := spec.Phase == "A"
		if gotPhaseA != wantPhaseA {
			t.Fatalf("processor %q phase=%q but isPhaseAProcessor=%v", spec.Name, spec.Phase, gotPhaseA)
		}
	}
}

func TestBuildProductionProcessorPlanExposesLegacyExecutionOrder(t *testing.T) {
	plan, err := BuildProductionProcessorPlan([]string{"extract_provisions", "generate_topics", "extract_doc_metadata"})
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlan: %v", err)
	}

	if got, want := plan.ExecutionOrder(), []string{"static_analyzer", "chunking", "extract_doc_metadata", "generate_topics", "extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("execution order=%v want=%v", got, want)
	}
}

func TestBuildProductionProcessorPlanExposesDependencies(t *testing.T) {
	plan, err := BuildProductionProcessorPlan([]string{"generate_topics", "extract_provisions"})
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlan: %v", err)
	}

	if got, want := plan.Dependencies("static_analyzer"), []string{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("static_analyzer deps=%v want=%v", got, want)
	}

	if got, want := plan.Dependencies("chunking"), []string{"static_analyzer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("chunking deps=%v want=%v", got, want)
	}

	if got, want := plan.Dependencies("generate_topics"), []string{"chunking"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("generate_topics deps=%v want=%v", got, want)
	}

	if got, want := plan.Dependencies("extract_provisions"), []string{"chunking"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("extract_provisions deps=%v want=%v", got, want)
	}
}

func TestBuildProductionProcessorPlanExposesOrderedStepsWithReasons(t *testing.T) {
	plan, err := BuildProductionProcessorPlan([]string{"generate_topics", "extract_provisions"})
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlan: %v", err)
	}

	got := plan.Steps()
	want := []ProcessorPlanStep{
		{Name: "static_analyzer", Phase: "A", DependsOn: []string{}, Reason: "mandatory_baseline"},
		{Name: "chunking", Phase: "A", DependsOn: []string{"static_analyzer"}, Reason: "implicit_dependency"},
		{Name: "generate_topics", Phase: "B", DependsOn: []string{"chunking"}, Reason: "explicit_request"},
		{Name: "extract_provisions", Phase: "B", DependsOn: []string{"chunking"}, Reason: "explicit_request"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("steps=%#v want=%#v", got, want)
	}
}

func TestBuildProductionProcessorPlanMarksImplicitDependenciesSeparately(t *testing.T) {
	plan, err := BuildProductionProcessorPlan([]string{"extract_metrics"})
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlan: %v", err)
	}

	steps := plan.Steps()
	if got, want := steps[0].Reason, "mandatory_baseline"; got != want {
		t.Fatalf("steps[0].Reason=%q want=%q", got, want)
	}
	if got, want := steps[1].Reason, "implicit_dependency"; got != want {
		t.Fatalf("steps[1].Reason=%q want=%q", got, want)
	}
	if got, want := steps[2].Reason, "explicit_request"; got != want {
		t.Fatalf("steps[2].Reason=%q want=%q", got, want)
	}
}

func TestBuildProductionProcessorPlanFromFactsPreservesInputsForFutureRouting(t *testing.T) {
	facts := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics", "extract_provisions"},
		KnowledgeStoreID:    42,
		KnowledgeStoreType:  "research",
	}

	plan, err := BuildProductionProcessorPlanFromFacts(facts)
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlanFromFacts: %v", err)
	}

	if got, want := plan.Facts(), facts; !reflect.DeepEqual(got, want) {
		t.Fatalf("facts=%#v want=%#v", got, want)
	}

	if got, want := plan.ExecutionOrder(), []string{"static_analyzer", "chunking", "generate_topics", "extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("execution order=%v want=%v", got, want)
	}
}

func TestApplyPolicyFilterOnlyFiltersInEnforcedModeWithNonEmptyProcessors(t *testing.T) {
	requested := []string{"generate_topics", "extract_provisions", "extract_metrics"}
	spec := ProductionPipelineSpec{Name: "narrative_default", Processors: []string{"generate_topics", "extract_metrics"}}

	tests := []struct {
		name          string
		mode          string
		spec          ProductionPipelineSpec
		wantEffective []string
		wantExcluded  []string
	}{
		{name: "plan-only never filters", mode: DocPipelineModePlanOnly, spec: spec, wantEffective: requested, wantExcluded: nil},
		{name: "empty mode never filters", mode: "", spec: spec, wantEffective: requested, wantExcluded: nil},
		{name: "enforced with empty pipeline processors never filters", mode: DocPipelineModeEnforced, spec: ProductionPipelineSpec{Name: "legacy_default"}, wantEffective: requested, wantExcluded: nil},
		{name: "enforced filters to pipeline's declared processors", mode: DocPipelineModeEnforced, spec: spec, wantEffective: []string{"generate_topics", "extract_metrics"}, wantExcluded: []string{"extract_provisions"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			effective, excluded := applyPolicyFilter(requested, tc.mode, tc.spec)
			if !reflect.DeepEqual(effective, tc.wantEffective) {
				t.Fatalf("effective=%v want=%v", effective, tc.wantEffective)
			}
			if !reflect.DeepEqual(excluded, tc.wantExcluded) {
				t.Fatalf("excluded=%v want=%v", excluded, tc.wantExcluded)
			}
		})
	}
}

func TestApplyPolicyFilterNeverExcludesMandatoryBaseline(t *testing.T) {
	effective, excluded := applyPolicyFilter(
		[]string{"static_analyzer", "chunking", "extract_metrics"},
		DocPipelineModeEnforced,
		ProductionPipelineSpec{Name: "narrow", Processors: []string{"extract_provisions"}},
	)
	if !reflect.DeepEqual(effective, []string{"static_analyzer", "chunking"}) {
		t.Fatalf("effective=%v", effective)
	}
	if !reflect.DeepEqual(excluded, []string{"extract_metrics"}) {
		t.Fatalf("excluded=%v", excluded)
	}
}

func TestBuildProductionProcessorPlanFromFactsEnforcesPipelineProcessorsAndRecordsExclusions(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "narrative_default", DisplayName: "Narrative Default", Processors: []string{"generate_topics", "extract_metrics"}},
	})

	facts := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics", "extract_provisions", "extract_metrics"},
		RequestedPipeline:   "narrative_default",
		Mode:                DocPipelineModeEnforced,
	}
	plan, err := BuildProductionProcessorPlanFromFacts(facts)
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlanFromFacts: %v", err)
	}

	if got, want := plan.ExecutionOrder(), []string{"static_analyzer", "chunking", "generate_topics", "extract_metrics"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("execution order=%v want=%v", got, want)
	}
	if got, want := plan.ExcludedByPolicy(), []string{"extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("excluded by policy=%v want=%v", got, want)
	}
}

func TestBuildProductionProcessorPlanFromFactsPlanOnlyModeNeverExcludes(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "narrative_default", DisplayName: "Narrative Default", Processors: []string{"generate_topics"}},
	})

	facts := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics", "extract_provisions"},
		RequestedPipeline:   "narrative_default",
		Mode:                DocPipelineModePlanOnly,
	}
	plan, err := BuildProductionProcessorPlanFromFacts(facts)
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlanFromFacts: %v", err)
	}

	if got := plan.ExcludedByPolicy(); got != nil {
		t.Fatalf("excluded by policy=%v want nil", got)
	}
	if got, want := plan.ExecutionOrder(), []string{"static_analyzer", "chunking", "generate_topics", "extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("execution order=%v want=%v", got, want)
	}
}

func TestBuildProductionPlanFactsFromInputRecordCapturesDeterministicStoreFacts(t *testing.T) {
	rec := DocMetadataInputRecord{
		ID:             91,
		ParserName:     "benchmark",
		Title:          "Ventilator display module",
		KSStoreID:      42,
		InputDocType:   "pdf",
		SourceLanguage: "en",
		DocumentNumber: "YY 9706.252-2021",
	}

	got := BuildProductionPlanFactsFromInputRecord([]string{"generate_topics", "extract_provisions"}, rec)
	want := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics", "extract_provisions"},
		KnowledgeStoreID:    42,
		ParserName:          "benchmark",
		DocumentTitle:       "Ventilator display module",
		InputDocType:        "pdf",
		SourceLanguage:      "en",
		DocumentNumber:      "YY 9706.252-2021",
		RoutingFacets: ProductionRoutingFacets{
			KnowledgeStoreBinding: "bound",
			InputDocType:          "pdf",
			SourceLanguage:        "en",
			HasDocumentNumber:     true,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facts=%#v want=%#v", got, want)
	}
}

func TestBuildProductionRoutingFacetsFromInputRecordNormalizesDeterministicVocabulary(t *testing.T) {
	rec := DocMetadataInputRecord{
		KSStoreID:      42,
		InputDocType:   " PDF ",
		SourceLanguage: " EN ",
		DocumentNumber: "YY 9706.252-2021",
	}

	got := BuildProductionRoutingFacetsFromInputRecord(rec)
	want := ProductionRoutingFacets{
		KnowledgeStoreBinding: "bound",
		InputDocType:          "pdf",
		SourceLanguage:        "en",
		HasDocumentNumber:     true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facets=%#v want=%#v", got, want)
	}
}

func processorNames(processors []Processor) []string {
	out := make([]string, len(processors))
	for i, p := range processors {
		out[i] = p.Name()
	}
	return out
}
