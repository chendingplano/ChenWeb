package docprocessing

import (
	"context"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func registeredFactPathsForTest(t *testing.T) map[string]bool {
	t.Helper()
	out := make(map[string]bool)
	for path := range semrules.RegisteredFactPaths() {
		out[path] = true
	}
	return out
}

func facetValue(t *testing.T, obs []FacetObservation, path string) any {
	t.Helper()
	for _, o := range obs {
		if o.Path == path {
			return o.Value
		}
	}
	t.Fatalf("no facet observation for path %q in %+v", path, obs)
	return nil
}

func TestTier1FacetsFromLinesEmptyReturnsNothing(t *testing.T) {
	// No content to measure: report nothing rather than misleading zeros.
	// file_type is deliberately not tier 1's job -- document.input_doc_type
	// already exists and is already populated elsewhere.
	obs := tier1FacetsFromLines(1, nil)
	if len(obs) != 0 {
		t.Fatalf("expected no facets for an empty line set, got %+v", obs)
	}
}

func TestTier1FacetsFromLinesPageCountIsMaxPageNo(t *testing.T) {
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "text", Content: "hello"},
		{LineNo: 2, PageNo: 3, LineType: "text", Content: "world"},
		{LineNo: 3, PageNo: 2, LineType: "text", Content: "middle"},
	}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.page_count"); got != 3 {
		t.Errorf("page_count: got %v, want 3", got)
	}
}

func TestTier1FacetsFromLinesTOCAndHeadingCount(t *testing.T) {
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "toc", Content: "1. Scope ... 3"},
		{LineNo: 2, PageNo: 2, LineType: "heading", Content: "1 Scope"},
		{LineNo: 3, PageNo: 2, LineType: "heading", Content: "1.1 Purpose"},
		{LineNo: 4, PageNo: 2, LineType: "text", Content: "Regular paragraph text."},
	}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.toc_presence"); got != true {
		t.Errorf("toc_presence: got %v, want true", got)
	}
	if got := facetValue(t, obs, "document.heading_count"); got != 2 {
		t.Errorf("heading_count: got %v, want 2", got)
	}
}

func TestTier1FacetsFromLinesNoTOCIsFalse(t *testing.T) {
	lines := []Line{{LineNo: 1, PageNo: 1, LineType: "text", Content: "no toc here"}}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.toc_presence"); got != false {
		t.Errorf("toc_presence: got %v, want false", got)
	}
}

func TestTier1FacetsFromLinesTableLineRatio(t *testing.T) {
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "table_row", Content: "10 | 20 | 30"},
		{LineNo: 2, PageNo: 1, LineType: "table_row", Content: "40 | 50 | 60"},
		{LineNo: 3, PageNo: 1, LineType: "text", Content: "not a table"},
		{LineNo: 4, PageNo: 1, LineType: "text", Content: "also not a table"},
	}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.table_line_ratio"); got != 0.5 {
		t.Errorf("table_line_ratio: got %v, want 0.5", got)
	}
}

func TestTier1FacetsFromLinesNumericUnitDensity(t *testing.T) {
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "text", Content: "The gap shall be 5mm at 25°C."},
		{LineNo: 2, PageNo: 1, LineType: "text", Content: "Supply voltage is 12V nominal."},
		{LineNo: 3, PageNo: 1, LineType: "text", Content: "No numbers or units in this line."},
		{LineNo: 4, PageNo: 1, LineType: "text", Content: "Neither in this one."},
	}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.numeric_unit_density"); got != 0.5 {
		t.Errorf("numeric_unit_density: got %v, want 0.5", got)
	}
}

func TestTier1FacetsFromLinesModalVerbDensityEnglishAndChinese(t *testing.T) {
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "text", Content: "The device shall not exceed 40°C."},
		{LineNo: 2, PageNo: 1, LineType: "text", Content: "设备必须满足以下条件。"},
		{LineNo: 3, PageNo: 1, LineType: "text", Content: "This is purely descriptive text."},
		{LineNo: 4, PageNo: 1, LineType: "text", Content: "So is this line."},
	}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.modal_verb_density"); got != 0.5 {
		t.Errorf("modal_verb_density: got %v, want 0.5", got)
	}
}

func TestTier1FacetsFromLinesFigureDensityEnglishAndChinese(t *testing.T) {
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "text", Content: "Figure 1: System overview"},
		{LineNo: 2, PageNo: 1, LineType: "text", Content: "表 2 测试结果"},
		{LineNo: 3, PageNo: 1, LineType: "text", Content: "Ordinary body text."},
		{LineNo: 4, PageNo: 1, LineType: "text", Content: "More ordinary text."},
	}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.figure_density"); got != 0.5 {
		t.Errorf("figure_density: got %v, want 0.5", got)
	}
}

func TestTier1FacetsFromLinesLanguageMixAllCJK(t *testing.T) {
	lines := []Line{{LineNo: 1, PageNo: 1, LineType: "text", Content: "本标准规定了通用要求"}}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.language_mix"); got != 1.0 {
		t.Errorf("language_mix: got %v, want 1.0 (all-CJK)", got)
	}
}

func TestTier1FacetsFromLinesLanguageMixAllLatin(t *testing.T) {
	lines := []Line{{LineNo: 1, PageNo: 1, LineType: "text", Content: "This document specifies general requirements"}}
	obs := tier1FacetsFromLines(1, lines)
	if got := facetValue(t, obs, "document.language_mix"); got != 0.0 {
		t.Errorf("language_mix: got %v, want 0.0 (all-Latin)", got)
	}
}

func TestTier1FacetsFromLinesEveryObservationCarriesExplicitConfidence(t *testing.T) {
	// P5 review 2026080302 finding P5-16: a nil Confidence goes spuriously
	// indeterminate against any min_confidence predicate, so every
	// deterministic observation must set one explicitly.
	lines := []Line{{LineNo: 1, PageNo: 1, LineType: "heading", Content: "1 Scope"}}
	obs := tier1FacetsFromLines(1, lines)
	if len(obs) == 0 {
		t.Fatal("expected at least one observation")
	}
	for _, o := range obs {
		if o.Confidence == nil {
			t.Errorf("observation %q has nil Confidence", o.Path)
		}
		if o.Method != FacetMethodDeterministic {
			t.Errorf("observation %q method = %q, want %q", o.Path, o.Method, FacetMethodDeterministic)
		}
		if o.RecordID != 1 {
			t.Errorf("observation %q record_id = %d, want 1", o.Path, o.RecordID)
		}
	}
}

func TestTier1FacetsFromLinesEveryPathIsRegisteredInSemrules(t *testing.T) {
	// Every path this producer emits must actually be recognized by the
	// predicate-evaluation system (semrules.RegisteredFactPaths), or the
	// observation is written but functionally invisible to routing/gate
	// rules -- exactly the mismatch this producer's design note warns about.
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "heading", Content: "Figure 1 shall be 5mm at 25°C 应"},
	}
	obs := tier1FacetsFromLines(1, lines)
	registered := registeredFactPathsForTest(t)
	for _, o := range obs {
		if !registered[o.Path] {
			t.Errorf("path %q is not registered in semrules.RegisteredFactPaths", o.Path)
		}
	}
}

func TestTier1WiringSetsRequiredFieldsInsertFacetObservationValidates(t *testing.T) {
	// InsertFacetObservation (doc_facet_store.go) rejects a call with an
	// empty DecisionAttemptID/InvocationID before ever touching the DB.
	// control.go's tier-1 wiring originally left both unset -- every real
	// insert would have failed in production even though every package
	// test passed, because stubFacetStore (classify-document_test.go)
	// doesn't enforce this validation. A nil-DB SQLStore isolates the
	// check: a validation failure returns its own specific error; "db is
	// nil" only happens once validation has already passed, so it proves
	// the fields were actually set, the same shape control.go's wiring
	// produces (DecisionAttemptID/InvocationID stamped after
	// ComputeTier1Facets returns, before InsertFacetObservation).
	store := SQLStore{}
	lines := []Line{{LineNo: 1, PageNo: 1, LineType: "heading", Content: "1 Scope"}}
	obs := tier1FacetsFromLines(1, lines)
	if len(obs) == 0 {
		t.Fatal("expected at least one observation")
	}
	for i := range obs {
		obs[i].DecisionAttemptID = "tier1-test.txt"
		obs[i].InvocationID = "tier1-1-test.txt"
		_, err := store.InsertFacetObservation(context.Background(), obs[i])
		if err == nil || err.Error() != "db is nil" {
			t.Errorf("observation %q: expected to clear field validation and fail only on nil DB, got %v", obs[i].Path, err)
		}
	}
}

// TestFacetTier1GatedOffDefaultsToRun proves the 2026-08-10 gate change (spec
// ADR 2026072901 S3.5) preserves facet_tier1's always-on default: with no
// kb.pipeline_rules row targeting it, facetTier1GatedOff must return false
// (not skipped), matching classify_document's classifyDocumentGatedOff
// contract for a processor no rule targets yet.
func TestFacetTier1GatedOffDefaultsToRun(t *testing.T) {
	oldGates := currentProductionPipelineGates()
	defer SetProductionPipelineGates(oldGates)
	SetProductionPipelineGates(nil)

	skip, err := facetTier1GatedOff(semrules.FactSet{})
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatal("facet_tier1 must not be skipped when no gate row targets it")
	}
}

// TestFacetTier1GatedOffHonorsAuthoredSkipRule proves an authored
// target_processor="facet_tier1" skip rule actually takes effect -- the
// lever this gate change exists to provide for debugging, testing, and bug
// fixing.
func TestFacetTier1GatedOffHonorsAuthoredSkipRule(t *testing.T) {
	oldGates := currentProductionPipelineGates()
	defer SetProductionPipelineGates(oldGates)
	SetProductionPipelineGates([]PipelineGate{gateFixtureForProcessor(101, FacetTier1Name, GateEffectSkip)})

	skip, err := facetTier1GatedOff(gateFacts("standard"))
	if err != nil {
		t.Fatal(err)
	}
	if !skip {
		t.Fatal("facet_tier1 must be skipped once an authored gate row resolves to skip")
	}
}
