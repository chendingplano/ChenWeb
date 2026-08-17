package semantic

import (
	"strings"
	"testing"
)

// stubAdapter is a minimal compliant adapter that individual tests break in
// one specific way, so each failure message maps to one DR13 requirement.
type stubAdapter struct {
	artifactType string
	name         string
	version      string
	scope        string
	rawFields    []string
	stages       []StageContract
	valueStates  []string
	conformance  []string
	depAxes      []string
	instances    bool
}

func (s stubAdapter) ArtifactType() string            { return s.artifactType }
func (s stubAdapter) AdapterName() string             { return s.name }
func (s stubAdapter) AdapterVersion() string          { return s.version }
func (s stubAdapter) OccurrenceScope() string         { return s.scope }
func (s stubAdapter) RawIdentityFields() []string     { return s.rawFields }
func (s stubAdapter) RequiredStages() []StageContract { return s.stages }
func (s stubAdapter) ValueStates() []string           { return s.valueStates }
func (s stubAdapter) ConformanceStates() []string     { return s.conformance }
func (s stubAdapter) DependencyAxes() []string        { return s.depAxes }
func (s stubAdapter) SupportsInstances() bool         { return s.instances }

func compliantStub() stubAdapter {
	return stubAdapter{
		artifactType: "widget",
		name:         "widget_adapter",
		version:      "1.0.0",
		scope:        "widget_occurrence:v1",
		rawFields:    []string{"widget_id", "input_record_id"},
		stages: []StageContract{{
			StageTermID:         StageNormalize,
			DecisionScopes:      []string{"value_literal"},
			Dimensions:          []string{DimensionValue},
			AllowedDispositions: []string{DispositionNormalized, DispositionRawPreserved},
		}},
		valueStates: []string{ValuePresent, ValueUnparsed},
		conformance: []string{Conforms, ConformanceNotEvaluated},
		depAxes:     []string{"parser_version"},
		instances:   true,
	}
}

func TestConformanceSuitePassesCompliantAdapter(t *testing.T) {
	res := RunConformanceSuite(compliantStub())
	if !res.Passed {
		t.Fatalf("compliant adapter failed: %v", res.Failures)
	}
	if res.SuiteVersion != ConformanceSuiteVersion {
		t.Errorf("suite version = %q, want %q", res.SuiteVersion, ConformanceSuiteVersion)
	}
}

// Task 4.8 / DR13: an adapter that omits a contract axis fails conformance.
// Each of these would silently break a different downstream guarantee, so each
// is checked independently rather than as one "is valid" boolean.
func TestConformanceSuiteRejectsIncompleteDeclarations(t *testing.T) {
	cases := map[string]func(stubAdapter) stubAdapter{
		"no required stages": func(a stubAdapter) stubAdapter { a.stages = nil; return a },
		"no decision scopes": func(a stubAdapter) stubAdapter {
			a.stages[0].DecisionScopes = nil
			return a
		},
		"no dispositions": func(a stubAdapter) stubAdapter {
			a.stages[0].AllowedDispositions = nil
			return a
		},
		"no value states":       func(a stubAdapter) stubAdapter { a.valueStates = nil; return a },
		"no conformance states": func(a stubAdapter) stubAdapter { a.conformance = nil; return a },
		"no dependency axes":    func(a stubAdapter) stubAdapter { a.depAxes = nil; return a },
		"no raw identity":       func(a stubAdapter) stubAdapter { a.rawFields = nil; return a },
		"no occurrence scope":   func(a stubAdapter) stubAdapter { a.scope = ""; return a },
		"no adapter version":    func(a stubAdapter) stubAdapter { a.version = ""; return a },
		"ungoverned value state": func(a stubAdapter) stubAdapter {
			a.valueStates = []string{"present"}
			return a
		},
		"ungoverned stage": func(a stubAdapter) stubAdapter {
			a.stages[0].StageTermID = "semantic:stage-normalize"
			return a
		},
	}
	for name, mutate := range cases {
		res := RunConformanceSuite(mutate(compliantStub()))
		if res.Passed {
			t.Errorf("%s: adapter passed conformance but should have failed", name)
		}
	}
}

func TestConformanceSuiteRejectsDuplicateStage(t *testing.T) {
	a := compliantStub()
	a.stages = append(a.stages, a.stages[0])
	res := RunConformanceSuite(a)
	if res.Passed {
		t.Fatal("a duplicated required stage must fail: outcome cardinality would be ambiguous")
	}
	var found bool
	for _, f := range res.Failures {
		if strings.Contains(f, "declared twice") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a duplicate-stage failure, got %v", res.Failures)
	}
}

// The real metric adapter must pass its own suite; otherwise the writer gate
// could never be authorized in Phase 3.
func TestMetricAdapterPassesConformanceSuite(t *testing.T) {
	res := RunConformanceSuite(MetricAdapter{})
	if !res.Passed {
		t.Fatalf("MetricAdapter failed conformance: %v", res.Failures)
	}
}

// DR1: a family modelled as ontology object instances must not fall back to an
// unresolved occurrence. The metric adapter declares that it does model
// instances, which is what makes option 2 mandatory for it.
func TestMetricAdapterDeclaresInstanceSupport(t *testing.T) {
	if !(MetricAdapter{}).SupportsInstances() {
		t.Fatal("metrics are modelled as ontology object instances (DR1); option 3 fallback is not available to them")
	}
}

func TestMetricAdapterDeclaresAllThreeRequiredStages(t *testing.T) {
	stages := MetricAdapter{}.RequiredStages()
	if len(stages) != 3 {
		t.Fatalf("required stage count = %d, want 3", len(stages))
	}
	want := map[string]bool{StageNormalize: true, StageClassResolution: true, StageAssociate: true}
	for _, st := range stages {
		if !want[st.StageTermID] {
			t.Errorf("unexpected required stage %q", st.StageTermID)
		}
		delete(want, st.StageTermID)
	}
	if len(want) != 0 {
		t.Errorf("missing required stages: %v", want)
	}
}

func TestRegisterAdapterRejectsDuplicateRegistration(t *testing.T) {
	resetAdaptersForTest()
	t.Cleanup(resetAdaptersForTest)
	RegisterAdapter(compliantStub())
	defer func() {
		if recover() == nil {
			t.Fatal("registering two adapters for one artifact type must panic: the completeness projection would be non-deterministic")
		}
	}()
	RegisterAdapter(compliantStub())
}

func TestLookupAdapterFindsRegistered(t *testing.T) {
	resetAdaptersForTest()
	t.Cleanup(resetAdaptersForTest)
	RegisterAdapter(MetricAdapter{})
	got, ok := LookupAdapter(MetricArtifactType)
	if !ok {
		t.Fatal("registered metric adapter not found")
	}
	if got.AdapterName() != MetricAdapterName {
		t.Errorf("adapter name = %q, want %q", got.AdapterName(), MetricAdapterName)
	}
	if types := RegisteredArtifactTypes(); len(types) != 1 || types[0] != MetricArtifactType {
		t.Errorf("registered types = %v, want [metric]", types)
	}
}
