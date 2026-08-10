package docprocessing

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func TestResolveProcessorGateIteratesRanksAndUsesEffectPrecedence(t *testing.T) {
	facts := gateFacts("standard")
	gates := []PipelineGate{
		gateFixture(1, 30, "skip", "invoice"),
		gateFixture(2, 20, "enable", "standard"),
		gateFixture(3, 20, "skip", "standard"),
		gateFixture(4, 20, "require", "standard"),
		gateFixture(5, 10, "skip", "standard"),
	}
	got, err := ResolveProcessorGate(ProcessorSpec{Name: "extract_metrics", Class: "routed", OnUndetermined: "skip"}, gates, facts, GateResolutionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Effect != GateEffectRequire || got.WinningRuleID != 4 || got.Truth != semrules.TruthTrue {
		t.Fatalf("resolution=%+v", got)
	}
	if len(got.Trace) != 4 {
		t.Fatalf("trace=%+v; lower rank should be irrelevant once rank 20 wins", got.Trace)
	}
}

func TestResolveProcessorGateTrueOutranksOnlyNonStrongerIndeterminate(t *testing.T) {
	tests := []struct {
		name                string
		trueEffect          string
		indeterminateEffect string
		onConflict          string
		wantEffect          string
		wantErr             bool
	}{
		{name: "require beats weaker indeterminate enable", trueEffect: "require", indeterminateEffect: "enable", onConflict: PipelineBindingOnConflictBlock, wantEffect: "require"},
		{name: "skip cannot beat stronger indeterminate require, block mode", trueEffect: "skip", indeterminateEffect: "require", onConflict: PipelineBindingOnConflictBlock, wantErr: true},
		// ADR 2026081001 DR9: fallback mode no longer falls open here either --
		// both conflict modes hard-fail an indeterminate resolution now.
		{name: "skip cannot beat stronger indeterminate require, fallback mode also hard-fails", trueEffect: "skip", indeterminateEffect: "require", onConflict: PipelineBindingOnConflictFallback, wantErr: true},
		{name: "skip beats weaker indeterminate enable", trueEffect: "skip", indeterminateEffect: "enable", onConflict: PipelineBindingOnConflictBlock, wantEffect: "skip"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			known := gateFixture(1, 10, test.trueEffect, "standard")
			unknown := gateFixture(2, 10, test.indeterminateEffect, "missing")
			unknown.Predicate = semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.domain", Op: "eq", Value: "medical"}}
			got, err := ResolveProcessorGate(ProcessorSpec{Name: "extract_metrics", OnUndetermined: "skip"}, []PipelineGate{known, unknown}, gateFacts("standard"), GateResolutionOptions{OnConflict: test.onConflict})
			if (err != nil) != test.wantErr {
				t.Fatalf("err=%v resolution=%+v", err, got)
			}
			if !test.wantErr && got.Effect != test.wantEffect {
				t.Fatalf("effect=%q want=%q", got.Effect, test.wantEffect)
			}
		})
	}
}

func TestResolveProcessorGateIndeterminateAlwaysHardFailsEvenInFallbackMode(t *testing.T) {
	// ADR 2026081001 DR9: resolveIndeterminateGate's old fallback-to-enable/
	// skip branch is gone -- an indeterminate resolution is a hard failure
	// regardless of OnConflict, a safety net for a pipeline version that
	// should never reach this state once it has passed DR8 validation.
	unknown := gateFixture(8, 10, GateEffectRequire, "missing")
	unknown.Predicate = semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.domain", Op: "eq", Value: "medical"}}
	_, err := ResolveProcessorGate(ProcessorSpec{Name: "extract_metrics", OnUndetermined: "skip"}, []PipelineGate{unknown}, gateFacts("standard"), GateResolutionOptions{OnConflict: PipelineBindingOnConflictFallback})
	if err == nil {
		t.Fatal("expected indeterminate gate to hard-fail even in fallback mode")
	}
	var gateErr *PipelineGateResolutionError
	if !errors.As(err, &gateErr) || gateErr.Reason != "indeterminate_after_validated_pipeline" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveProcessorGateMandatoryExplicitAndRunOverridesWin(t *testing.T) {
	skip := gateFixture(1, 100, GateEffectSkip, "standard")
	tests := []struct {
		name       string
		spec       ProcessorSpec
		options    GateResolutionOptions
		wantSource string
		wantEffect string
	}{
		{name: "mandatory", spec: ProcessorSpec{Name: "static_analyzer", Class: "mandatory"}, wantSource: "mandatory", wantEffect: GateEffectRequire},
		{name: "mandatory gated", spec: ProcessorSpec{Name: "classify_document", Class: "mandatory_gated"}, wantSource: "mandatory", wantEffect: GateEffectRequire},
		{name: "explicit", spec: ProcessorSpec{Name: "extract_metrics"}, options: GateResolutionOptions{Explicit: true}, wantSource: "explicit_request", wantEffect: GateEffectRequire},
		{name: "run override", spec: ProcessorSpec{Name: "extract_metrics"}, options: GateResolutionOptions{RunOverride: GateEffectEnable}, wantSource: "run_override", wantEffect: GateEffectEnable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveProcessorGate(test.spec, []PipelineGate{skip}, gateFacts("standard"), test.options)
			if err != nil {
				t.Fatal(err)
			}
			if got.Source != test.wantSource || got.Effect != test.wantEffect {
				t.Fatalf("resolution=%+v", got)
			}
		})
	}
}

func TestBuildProcessorGateShadowPlanDoesNotChangeEffectiveProcessors(t *testing.T) {
	baseline := []string{"static_analyzer", "extract_metrics", "extract_provisions"}
	gates := []PipelineGate{
		gateFixtureForProcessor(1, "extract_metrics", GateEffectSkip),
	}
	shadow, err := BuildProcessorGateShadowPlan(baseline, productionProcessorSpecs, gates, gateFacts("standard"), GateShadowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shadow.EffectiveProcessors, baseline) || !reflect.DeepEqual(shadow.WouldSkip, []string{"extract_metrics"}) {
		t.Fatalf("shadow=%+v", shadow)
	}
	// extract_provisions has no gate row -> processor_default (Enable) ->
	// WouldRun, alongside the mandatory static_analyzer.
	if !reflect.DeepEqual(shadow.WouldRun, []string{"static_analyzer", "extract_provisions"}) {
		t.Fatalf("shadow.WouldRun=%+v", shadow.WouldRun)
	}
}

func TestPipelineGateSQLStoreLoadsCanonicalActiveGates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT r.id,r.name,r.priority,r.target_processor,r.effect,r.predicate::text,r.predicate_checksum,r.required_facets::text,r.active")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "priority", "target", "effect", "predicate", "checksum", "facets", "active"}).
			AddRow(int64(7), "metrics", 20, "extract_metrics", "skip", `{"version":1,"expression":{"kind":"all","items":[]}}`, "sha256:gate", `["document.doc_kind"]`, true))
	gates, err := (PipelineGateSQLStore{DB: db}).ListPipelineGates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(gates) != 1 || gates[0].ID != 7 || !reflect.DeepEqual(gates[0].RequiredFacets, []string{"document.doc_kind"}) {
		t.Fatalf("gates=%+v", gates)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProductionPlanPersistsCompleteP5ShadowSnapshot(t *testing.T) {
	// The fixture gate must resolve deterministically (ADR 2026081001 DR9:
	// an indeterminate gate is now a hard failure in every conflict mode,
	// with no fallback salvage), so it matches against document.input_doc_type
	// -- a baseline routing fact always known from RoutingFacets, not the
	// tier-3-only document.doc_kind gateFixtureForProcessor otherwise uses.
	oldGates := currentProductionPipelineGates()
	defer SetProductionPipelineGates(oldGates)
	gate := gateFixtureForProcessor(11, "extract_metrics", GateEffectSkip)
	gate.Predicate = semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.input_doc_type", Op: "eq", Value: "pdf"}}
	SetProductionPipelineGates([]PipelineGate{gate})
	plan, err := BuildProductionProcessorPlanFromFacts(ProductionPlanFacts{
		RequestedProcessors: []string{"extract_metrics"}, RoutingFacets: ProductionRoutingFacets{InputDocType: "pdf"},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := plan.RoutingSnapshot()
	if snapshot == nil || snapshot.PipelineName != DefaultProductionPipelineName {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if snapshot.SelectedPipelineChecksum == "" || snapshot.BaselinePipelineChecksum == "" || len(snapshot.Facts) == 0 || len(snapshot.GateShadow.Decisions) == 0 {
		t.Fatalf("incomplete snapshot=%+v", snapshot)
	}
	if !reflect.DeepEqual(snapshot.RuleChecksums, []string{"sha256:gate"}) {
		t.Fatalf("checksums=%v", snapshot.RuleChecksums)
	}
	// Accessors return deep copies so later registry mutations cannot rewrite
	// the already-frozen plan.
	SetProductionPipelineGates(nil)
	again := plan.RoutingSnapshot()
	if !reflect.DeepEqual(snapshot, again) {
		t.Fatalf("snapshot changed after registry mutation: before=%+v after=%+v", snapshot, again)
	}
}

func gateFixture(id int64, priority int, effect, docKind string) PipelineGate {
	return PipelineGate{ID: id, Name: "gate", Priority: priority, TargetProcessor: "extract_metrics", Effect: effect, PredicateChecksum: "sha256:gate", Active: true, Predicate: semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: docKind}}}
}

func gateFixtureForProcessor(id int64, processor, effect string) PipelineGate {
	gate := gateFixture(id, 10, effect, "standard")
	gate.TargetProcessor = processor
	return gate
}

func gateFacts(docKind string) semrules.FactSet {
	return semrules.FactSet{"document.doc_kind": {Path: "document.doc_kind", State: semrules.FactKnown, Value: docKind}}
}
