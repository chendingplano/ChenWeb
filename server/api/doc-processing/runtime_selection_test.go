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
		{"test methods", []string{"extract_test_methods"}, []string{"static_analyzer", "chunking", "extract_test_methods"}},
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

// TestNormalizeDocPipelineOnConflict proves DOC_PIPELINE_ON_CONFLICT parses
// the shared block/fallback setting spec 2026080102 sections 5.1/5.2 use
// for both pipeline-binding and processor-gate conflict resolution --
// previously never read anywhere, leaving bindings hardcoded to block and
// gates hardcoded to fallback (P5 review 2026080302 finding P5-19).
func TestNormalizeDocPipelineOnConflict(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"", PipelineBindingOnConflictBlock, false},
		{"block", PipelineBindingOnConflictBlock, false},
		{"BLOCK", PipelineBindingOnConflictBlock, false},
		{" fallback ", PipelineBindingOnConflictFallback, false},
		{"retry", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := normalizeDocPipelineOnConflict(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeDocPipelineOnConflict(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("got=%q want=%q", got, tt.want)
			}
		})
	}
}

// TestResolveProductionPipelineBindingRespectsOnConflictEnv proves the
// binding resolver reads DOC_PIPELINE_ON_CONFLICT instead of always
// hardcoding block mode -- a same-rank true conflict blocks by default but
// resolves via the fallback ladder when the env var requests fallback (P5
// review 2026080302 finding P5-19).
func TestResolveProductionPipelineBindingRespectsOnConflictEnv(t *testing.T) {
	pdf := mustLegacyBinding(t, "pdf", "pipeline_a", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"})
	en := mustLegacyBinding(t, "en", "pipeline_b", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchSourceLanguage: "en"})
	lower := mustLegacyBinding(t, "lower", "pipeline_b", 1, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{})
	t.Cleanup(func() { SetProductionPipelineBindings(nil); SetProductionPipelineRegistry(nil) })
	SetProductionPipelineBindings([]PipelineBinding{pdf, en, lower})
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "pipeline_a", DisplayName: "Pipeline A"},
		{Name: "pipeline_b", DisplayName: "Pipeline B"},
	})

	facts := ProductionPlanFacts{
		RoutingFacets: ProductionRoutingFacets{InputDocType: "pdf", SourceLanguage: "en"},
	}

	if _, err := ResolveProductionPipelineBinding(facts); err == nil {
		t.Fatal("expected the default (unset env, block mode) to error on a true conflict")
	}

	t.Setenv("DOC_PIPELINE_ON_CONFLICT", "fallback")
	got, err := ResolveProductionPipelineBinding(facts)
	if err != nil {
		t.Fatalf("expected fallback mode to resolve via the lower rank, got: %v", err)
	}
	if got.SelectedPipeline != "pipeline_b" {
		t.Fatalf("SelectedPipeline = %q, want pipeline_b (the fallback ladder's lower-rank binding)", got.SelectedPipeline)
	}
}

// TestBuildProductionProcessorPlanFromFactsGateConflictRespectsOnConflictEnv
// mirrors the binding test for processor gates: an indeterminate gate
// (missing tier-3 document.doc_kind) blocks plan construction by default,
// but resolves via the processor's OnUndetermined default when the env var
// requests fallback (P5 review 2026080302 finding P5-19).
func TestBuildProductionProcessorPlanFromFactsGateConflictRespectsOnConflictEnv(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineGates(nil) })
	indeterminate := gateFixture(1, 10, GateEffectSkip, "standard")
	SetProductionPipelineGates([]PipelineGate{indeterminate})

	facts := ProductionPlanFacts{
		RequestedProcessors: []string{"extract_metrics"},
		Mode:                DocPipelineModeEnforced,
	}

	if _, err := BuildProductionProcessorPlanFromFacts(facts); err == nil {
		t.Fatal("expected the default (unset env, block mode) to error on an indeterminate gate")
	}

	t.Setenv("DOC_PIPELINE_ON_CONFLICT", "fallback")
	if _, err := BuildProductionProcessorPlanFromFacts(facts); err != nil {
		t.Fatalf("expected fallback mode to resolve via OnUndetermined, got: %v", err)
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

// TestProductionProcessorSpecsRequiresProducesAreConsistent is a data-only
// check on the DR5 Requires/Produces vocabulary populated 2026-08-08 (ADR
// 2026072901 S16.1 "DAG planner" row): nothing consumes these fields yet
// (that's the deferred part), but every declared value should still be
// internally coherent, so a typo here doesn't sit undetected until whatever
// eventually reads it. For every processor with a declared Requires, at
// least one of its DependsOn processors must Produce that artifact kind.
func TestProductionProcessorSpecsRequiresProducesAreConsistent(t *testing.T) {
	produces := map[string]map[string]bool{} // processor name -> set of artifact kinds it produces
	for _, spec := range productionProcessorSpecs {
		produces[spec.Name] = map[string]bool{}
		for _, kind := range spec.Produces {
			if kind == "" {
				t.Errorf("processor %q declares an empty Produces entry", spec.Name)
			}
			produces[spec.Name][kind] = true
		}
	}
	for _, spec := range productionProcessorSpecs {
		if spec.Class == "mandatory_gated" {
			// Dispatched outside the ordinary DependsOn-driven wave (see the
			// declaration's own comment: invoked directly by the G2 resolver,
			// not through the Phase A/B/C mechanism), so it has no DependsOn
			// entry to check against even though its Requires is accurate.
			continue
		}
		for _, required := range spec.Requires {
			if required == "" {
				t.Errorf("processor %q declares an empty Requires entry", spec.Name)
				continue
			}
			satisfied := false
			for _, dep := range spec.DependsOn {
				if produces[dep][required] {
					satisfied = true
					break
				}
			}
			if !satisfied {
				t.Errorf("processor %q requires %q, but none of its DependsOn %v produces it", spec.Name, required, spec.DependsOn)
			}
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

// TestBuildProductionProcessorPlanFromFactsGateShadowCoversBaselineFallbackProcessors
// proves the gate shadow includes a decision for a processor the selected
// pipeline excludes but the P5 baseline/store-default fallback pipeline
// would admit. Without this, an uncleared suppressive-binding fallback to
// baseline re-admits such a processor with no gate decision to consult,
// so FinalizeRoutingPlan runs it ungated instead of gate-evaluating it (P5
// review 2026080302 finding P5-13).
func TestBuildProductionProcessorPlanFromFactsGateShadowCoversBaselineFallbackProcessors(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "narrative_default", DisplayName: "Narrative Default", Processors: []string{"generate_topics"}},
	})

	facts := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics", "extract_provisions"},
		RequestedPipeline:   "narrative_default",
		Mode:                DocPipelineModeEnforced,
	}
	plan, err := BuildProductionProcessorPlanFromFacts(facts)
	if err != nil {
		t.Fatalf("BuildProductionProcessorPlanFromFacts: %v", err)
	}
	// Sanity check: extract_provisions really is excluded from the selected
	// pipeline (this is the P5-13 scenario, not a no-op).
	if got, want := plan.ExcludedByPolicy(), []string{"extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("excluded by policy=%v want=%v", got, want)
	}
	snapshot := plan.RoutingSnapshot()
	if snapshot == nil {
		t.Fatal("expected a routing snapshot")
	}
	found := false
	for _, decision := range snapshot.GateShadow.Decisions {
		if decision.Processor == "extract_provisions" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no gate decision for extract_provisions (baseline-fallback-only processor); GateShadow.Decisions=%#v", snapshot.GateShadow.Decisions)
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
