package docprocessing

import (
	"context"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func TestTier2FacetsFromSourceEmptyReturnsNothing(t *testing.T) {
	obs := tier2FacetsFromSource(1, "", "")
	if len(obs) != 0 {
		t.Fatalf("expected no facets when doc_no and publish_date are both empty, got %+v", obs)
	}
}

func TestTier2FacetsFromSourcePublishDatePassthrough(t *testing.T) {
	obs := tier2FacetsFromSource(1, "", "2024-03-15")
	if got := facetValue(t, obs, "document.publish_date"); got != "2024-03-15" {
		t.Errorf("publish_date: got %v, want 2024-03-15", got)
	}
}

func TestAuthorityHintFromDocNo(t *testing.T) {
	cases := []struct {
		docNo string
		want  string
	}{
		{"GB/T 1234-2020", "gb"},
		{"GB50325-2010", "gb"},
		{"gb 5749-2022", "gb"},
		{"ISO 9001:2015", "iso"},
		{"ISO/IEC 17025:2017", "iso"}, // checked before IEC in pattern order
		{"IEC 60950-1", "iec"},
		{"ANSI Z21.1", "ansi"},
		{"ASTM D638", "astm"},
		{"XYZ-999-2020", "unknown"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := authorityHintFromDocNo(c.docNo); got != c.want {
			t.Errorf("authorityHintFromDocNo(%q) = %q, want %q", c.docNo, got, c.want)
		}
	}
}

func TestTier2FacetsFromSourceAuthorityHintObservation(t *testing.T) {
	obs := tier2FacetsFromSource(1, "IEC 60601-1", "")
	if got := facetValue(t, obs, "document.authority_hint"); got != "iec" {
		t.Errorf("authority_hint: got %v, want iec", got)
	}
}

func TestTier2FacetsFromSourceBothFacets(t *testing.T) {
	obs := tier2FacetsFromSource(1, "GB/T 191-2008", "2008-06-01")
	if len(obs) != 2 {
		t.Fatalf("expected 2 facets, got %+v", obs)
	}
	if got := facetValue(t, obs, "document.publish_date"); got != "2008-06-01" {
		t.Errorf("publish_date: got %v, want 2008-06-01", got)
	}
	if got := facetValue(t, obs, "document.authority_hint"); got != "gb" {
		t.Errorf("authority_hint: got %v, want gb", got)
	}
}

func TestTier2FacetsFromSourceMethodAndConfidence(t *testing.T) {
	obs := tier2FacetsFromSource(7, "ISO 13485", "2016-01-01")
	if len(obs) == 0 {
		t.Fatal("expected at least one observation")
	}
	for _, o := range obs {
		if o.Method != FacetMethodMetadata {
			t.Errorf("observation %q method = %q, want %q", o.Path, o.Method, FacetMethodMetadata)
		}
		if o.Confidence == nil {
			t.Errorf("observation %q has nil Confidence", o.Path)
		}
		if o.RecordID != 7 {
			t.Errorf("observation %q record_id = %d, want 7", o.Path, o.RecordID)
		}
	}
}

func TestTier2FacetsFromSourceEveryPathIsRegisteredInSemrules(t *testing.T) {
	obs := tier2FacetsFromSource(1, "GB/T 191-2008", "2008-06-01")
	registered := registeredFactPathsForTest(t)
	for _, o := range obs {
		if !registered[o.Path] {
			t.Errorf("path %q is not registered in semrules.RegisteredFactPaths", o.Path)
		}
	}
}

func TestTier2WiringSetsRequiredFieldsInsertFacetObservationValidates(t *testing.T) {
	// Same check as TestTier1WiringSetsRequiredFieldsInsertFacetObservation
	// Validates: extract-doc-metadata.go's tier-2 wiring must stamp
	// DecisionAttemptID/InvocationID before calling InsertFacetObservation,
	// or every real insert fails validation before touching the DB.
	store := SQLStore{}
	obs := tier2FacetsFromSource(1, "GB/T 191-2008", "2008-06-01")
	if len(obs) == 0 {
		t.Fatal("expected at least one observation")
	}
	for i := range obs {
		obs[i].DecisionAttemptID = "tier2-test-event"
		obs[i].InvocationID = "tier2-1-test-event"
		_, err := store.InsertFacetObservation(context.Background(), obs[i])
		if err == nil || err.Error() != "db is nil" {
			t.Errorf("observation %q: expected to clear field validation and fail only on nil DB, got %v", obs[i].Path, err)
		}
	}
}

// TestFacetTier2GatedOffDefaultsToRun mirrors
// TestFacetTier1GatedOffDefaultsToRun (facet_tier1_test.go): with no
// kb.pipeline_rules row targeting facet_tier2, facetTier2GatedOff must
// return false (not skipped).
func TestFacetTier2GatedOffDefaultsToRun(t *testing.T) {
	oldGates := currentProductionPipelineGates()
	defer SetProductionPipelineGates(oldGates)
	SetProductionPipelineGates(nil)

	skip, err := facetTier2GatedOff(semrules.FactSet{})
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatal("facet_tier2 must not be skipped when no gate row targets it")
	}
}

// TestFacetTier2GatedOffHonorsAuthoredSkipRule mirrors
// TestFacetTier1GatedOffHonorsAuthoredSkipRule (facet_tier1_test.go): an
// authored target_processor="facet_tier2" skip rule must take effect.
func TestFacetTier2GatedOffHonorsAuthoredSkipRule(t *testing.T) {
	oldGates := currentProductionPipelineGates()
	defer SetProductionPipelineGates(oldGates)
	SetProductionPipelineGates([]PipelineGate{gateFixtureForProcessor(102, FacetTier2Name, GateEffectSkip)})

	skip, err := facetTier2GatedOff(gateFacts("standard"))
	if err != nil {
		t.Fatal(err)
	}
	if !skip {
		t.Fatal("facet_tier2 must be skipped once an authored gate row resolves to skip")
	}
}
