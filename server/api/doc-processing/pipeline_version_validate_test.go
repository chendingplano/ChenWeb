package docprocessing

import (
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func alwaysTrueDocumentForValidateTest() semrules.Document {
	return semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.input_doc_type", Op: "exists"}}
}

func TestValidatePipelineVersionRejectsMissingProducer(t *testing.T) {
	// A synthetic processor requiring an artifact kind nothing produces is
	// used here because every built-in processor's Requires is now either
	// satisfied by a baseline processor (static_analyzer/chunking) or, for
	// the DR8 Phase D stages, no longer declared at all -- see
	// TestValidatePipelineVersionAcceptsDataDrivenPhaseDStagesStandalone.
	RegisterProcessor(ProcessorSpec{Name: "test_only_missing_producer", Requires: []string{"nonexistent_artifact_kind"}})
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"test_only_missing_producer"},
	})
	if err == nil {
		t.Fatal("expected processor closure rejection")
	}
	if !strings.Contains(err.Error(), "test_only_missing_producer") || !strings.Contains(err.Error(), "nonexistent_artifact_kind") {
		t.Fatalf("error should name the processor and missing artifact kind: %v", err)
	}
}

// TestValidatePipelineVersionAcceptsDataDrivenPhaseDStagesStandalone is the
// regression test for the bug fixed 2026-08-19: normalize_assertions,
// associate_semantics, and project_semantics each read/write purely by
// input_record_id against already-persisted DB state (see phase_d.go), so
// none of them requires its logical predecessor to be selected in the same
// pipeline version -- only to have run at some point, possibly under an
// earlier version, for the records this version will process.
func TestValidatePipelineVersionAcceptsDataDrivenPhaseDStagesStandalone(t *testing.T) {
	for _, name := range []string{"normalize_assertions", "associate_semantics", "project_semantics"} {
		if err := ValidatePipelineVersion(PipelineVersionDraft{Processors: []string{name}}); err != nil {
			t.Fatalf("expected %q to be selectable standalone (data-driven, no same-run producer required), got: %v", name, err)
		}
	}
}

func TestValidatePipelineVersionRejectsEmptyProcessorSet(t *testing.T) {
	err := ValidatePipelineVersion(PipelineVersionDraft{})
	if err == nil {
		t.Fatal("expected empty processor set rejection")
	}
	if !strings.Contains(err.Error(), "at least one doc processor") {
		t.Fatalf("error should state that at least one processor is required: %v", err)
	}
}

func TestValidatePipelineVersionAcceptsSatisfiedClosure(t *testing.T) {
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"extract_metrics", "extract_provisions", "normalize_assertions"},
	})
	if err != nil {
		t.Fatalf("expected satisfied closure to pass, got: %v", err)
	}
}

func TestValidatePipelineVersionClosureTreatsBaselineAsAlwaysPresent(t *testing.T) {
	// chunking (baseline) is not in Processors, but its Produces("chunks")
	// must still count -- it always runs first.
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"extract_metrics"},
	})
	if err != nil {
		t.Fatalf("expected baseline processors to satisfy closure implicitly, got: %v", err)
	}
}

func TestValidatePipelineVersionRejectsCyclicDependency(t *testing.T) {
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"extract_metrics", "extract_provisions"},
		Rules: []PipelineGate{
			{Name: "gate-a", TargetProcessor: "extract_metrics", Effect: GateEffectRequire, Predicate: alwaysTrueDocumentForValidateTest(), DependsOnProcessors: []string{"extract_provisions"}},
			{Name: "gate-b", TargetProcessor: "extract_provisions", Effect: GateEffectRequire, Predicate: alwaysTrueDocumentForValidateTest(), DependsOnProcessors: []string{"extract_metrics"}},
		},
	})
	if err == nil {
		t.Fatal("expected cycle rejection")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error should mention the cycle: %v", err)
	}
}

func TestValidatePipelineVersionRejectsDanglingDependency(t *testing.T) {
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"extract_metrics"},
		Rules: []PipelineGate{
			{Name: "gate-a", TargetProcessor: "extract_metrics", Effect: GateEffectRequire, Predicate: alwaysTrueDocumentForValidateTest(), DependsOnProcessors: []string{"extract_provisions"}},
		},
	})
	if err == nil {
		t.Fatal("expected dangling-dependency rejection")
	}
	if !strings.Contains(err.Error(), "extract_provisions") {
		t.Fatalf("error should name the dangling reference: %v", err)
	}
}

func docKindPredicate() semrules.Document {
	return semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "invoice"}}
}

func TestValidatePipelineVersionRejectsUnavailableGateFact(t *testing.T) {
	// document.doc_kind is not a baseline fact; extract_metrics has no
	// upstream facet producer in its DAG, so the gate can never resolve.
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"extract_metrics"},
		Rules: []PipelineGate{
			{Name: "gate-a", TargetProcessor: "extract_metrics", Effect: GateEffectRequire, Predicate: docKindPredicate()},
		},
	})
	if err == nil {
		t.Fatal("expected gate-fact-availability rejection")
	}
	if !strings.Contains(err.Error(), "gate-a") || !strings.Contains(err.Error(), "document.doc_kind") {
		t.Fatalf("error should name the gate and missing fact: %v", err)
	}
}

func TestValidatePipelineVersionAcceptsAvailableGateFact(t *testing.T) {
	// facet_tier2 (Produces "facets") is a declared upstream dependency of
	// extract_metrics, so document.doc_kind is guaranteed available.
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"extract_metrics", "facet_tier2", "extract_doc_metadata"},
		Rules: []PipelineGate{
			{Name: "gate-a", TargetProcessor: "extract_metrics", Effect: GateEffectRequire, Predicate: docKindPredicate(), DependsOnProcessors: []string{"facet_tier2"}},
		},
	})
	if err != nil {
		t.Fatalf("expected gate-fact-availability to pass, got: %v", err)
	}
}

func TestValidatePipelineVersionRejectsDeferEffect(t *testing.T) {
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"extract_metrics"},
		Rules: []PipelineGate{
			{Name: "gate-a", TargetProcessor: "extract_metrics", Effect: GateEffectDefer, Predicate: docKindPredicate()},
		},
	})
	if err == nil {
		t.Fatal("expected effect=defer rejection")
	}
	if !strings.Contains(err.Error(), "gate-a") || !strings.Contains(err.Error(), "defer") {
		t.Fatalf("error should name the gate and mention defer: %v", err)
	}
}

func TestValidatePipelineVersionAcceptsVacuousPredicateWithoutUpstream(t *testing.T) {
	// The always-true baseline predicate references only document.input_doc_type,
	// a baseline routing fact available before any processor runs.
	err := ValidatePipelineVersion(PipelineVersionDraft{
		Processors: []string{"extract_metrics"},
		Rules: []PipelineGate{
			{Name: "gate-a", TargetProcessor: "extract_metrics", Effect: GateEffectRequire, Predicate: alwaysTrueDocumentForValidateTest()},
		},
	})
	if err != nil {
		t.Fatalf("expected baseline-fact-only predicate to pass, got: %v", err)
	}
}
