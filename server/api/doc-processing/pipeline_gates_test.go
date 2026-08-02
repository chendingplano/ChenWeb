package docprocessing

import (
	"context"
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
		gateFixture(3, 20, "defer", "standard"),
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
		wantEffect          string
		wantErr             bool
	}{
		{name: "require beats defer", trueEffect: "require", indeterminateEffect: "defer", wantEffect: "require"},
		{name: "skip cannot beat possible defer", trueEffect: "skip", indeterminateEffect: "defer", wantErr: true},
		{name: "skip beats possible enable", trueEffect: "skip", indeterminateEffect: "enable", wantEffect: "skip"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			known := gateFixture(1, 10, test.trueEffect, "standard")
			unknown := gateFixture(2, 10, test.indeterminateEffect, "missing")
			unknown.Predicate = semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.domain", Op: "eq", Value: "medical"}}
			got, err := ResolveProcessorGate(ProcessorSpec{Name: "extract_metrics", OnUndetermined: "skip"}, []PipelineGate{known, unknown}, gateFacts("standard"), GateResolutionOptions{OnConflict: PipelineBindingOnConflictBlock})
			if (err != nil) != test.wantErr {
				t.Fatalf("err=%v resolution=%+v", err, got)
			}
			if !test.wantErr && got.Effect != test.wantEffect {
				t.Fatalf("effect=%q want=%q", got.Effect, test.wantEffect)
			}
		})
	}
}

func TestResolveProcessorGateFallbackDefaultsAndDeferFingerprint(t *testing.T) {
	unknown := gateFixture(8, 10, GateEffectDefer, "missing")
	unknown.RequiredFacets = []string{"document.domain", "document.doc_kind"}
	unknown.Predicate = semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.domain", Op: "eq", Value: "medical"}}
	got, err := ResolveProcessorGate(ProcessorSpec{Name: "extract_metrics", OnUndetermined: "skip"}, []PipelineGate{unknown}, gateFacts("standard"), GateResolutionOptions{OnConflict: PipelineBindingOnConflictFallback})
	if err != nil {
		t.Fatal(err)
	}
	if got.Effect != GateEffectSkip || got.Source != "indeterminate_fallback" {
		t.Fatalf("resolution=%+v", got)
	}

	deferGate := gateFixture(9, 10, GateEffectDefer, "standard")
	deferGate.RequiredFacets = []string{"document.domain", "document.jurisdiction"}
	got, err = ResolveProcessorGate(ProcessorSpec{Name: "extract_metrics"}, []PipelineGate{deferGate}, gateFacts("standard"), GateResolutionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Effect != GateEffectDefer || got.DeferFingerprint == "" || !reflect.DeepEqual(got.DeferredPaths, []string{"document.doc_kind", "document.domain", "document.jurisdiction"}) {
		t.Fatalf("resolution=%+v", got)
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
		gateFixtureForProcessor(2, "extract_provisions", GateEffectDefer),
	}
	gates[1].RequiredFacets = []string{"document.domain"}
	shadow, err := BuildProcessorGateShadowPlan(baseline, productionProcessorSpecs, gates, gateFacts("standard"), GateShadowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shadow.EffectiveProcessors, baseline) || !reflect.DeepEqual(shadow.WouldSkip, []string{"extract_metrics"}) || !reflect.DeepEqual(shadow.WouldDefer, []string{"extract_provisions"}) {
		t.Fatalf("shadow=%+v", shadow)
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
	oldGates := currentProductionPipelineGates()
	defer SetProductionPipelineGates(oldGates)
	gate := gateFixtureForProcessor(11, "extract_metrics", GateEffectSkip)
	SetProductionPipelineGates([]PipelineGate{gate})
	plan, err := BuildProductionProcessorPlanFromFacts(ProductionPlanFacts{
		RequestedProcessors: []string{"extract_metrics"}, ActivePolicyID: 5, ActivePolicyVersion: 2,
		ActivePolicyChecksum: "sha256:policy", RoutingFacets: ProductionRoutingFacets{InputDocType: "pdf"},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := plan.RoutingSnapshot()
	if snapshot == nil || snapshot.PolicyID != 5 || snapshot.PolicyVersion != 2 || snapshot.PolicyChecksum != "sha256:policy" {
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
