package docprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func TestCompilePolicyValidatesPredicatesTargetsAndCanonicalChecksums(t *testing.T) {
	compiled, err := CompilePolicy(compilePolicyFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(compiled.Checksum, "sha256:") || len(compiled.BindingChecksums) != 1 || len(compiled.GateChecksums) != 1 {
		t.Fatalf("compiled=%+v", compiled)
	}
	tests := []struct {
		name   string
		mutate func(*PolicyDefinition)
		want   string
	}{
		{"invalid predicate", func(p *PolicyDefinition) { p.Gates[0].Predicate.Expression.Path = "unknown.path" }, "unknown fact path"},
		{"unknown pipeline", func(p *PolicyDefinition) { p.Bindings[0].PipelineName = "missing" }, "unknown pipeline"},
		{"unknown processor", func(p *PolicyDefinition) { p.Gates[0].TargetProcessor = "missing" }, "unknown processor"},
		{"invalid effect", func(p *PolicyDefinition) { p.Gates[0].Effect = "drop" }, "invalid effect"},
		{"stale binding checksum", func(p *PolicyDefinition) { p.Bindings[0].PredicateChecksum = "sha256:stale" }, "predicate checksum mismatch"},
		{"invalid clearance", func(p *PolicyDefinition) { p.ClearanceReferences[0].SubjectChecksum = "sha256:stale" }, "clearance"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := compilePolicyFixture()
			test.mutate(&definition)
			_, err := CompilePolicy(definition)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want %q", err, test.want)
			}
		})
	}
}

func TestCompilePolicyLegacyAdapterParity(t *testing.T) {
	legacy := ProductionPipelineRule{MatchInputDocType: " PDF ", MatchSourceLanguage: " EN ", PipelineName: "regulated"}
	doc, checksum, err := LegacyRulePredicateDocument(legacy)
	if err != nil {
		t.Fatal(err)
	}
	definition := compilePolicyFixture()
	definition.Bindings[0].Predicate, definition.Bindings[0].PredicateChecksum, definition.Bindings[0].LegacyRule = doc, checksum, &legacy
	if _, err := CompilePolicy(definition); err != nil {
		t.Fatal(err)
	}
	definition.Bindings[0].LegacyRule.MatchSourceLanguage = "fr"
	if _, err := CompilePolicy(definition); err == nil || !strings.Contains(err.Error(), "legacy adapter parity") {
		t.Fatalf("err=%v", err)
	}
}

func TestCompilePolicyBindingOverlapRulesAndRuntimeWarning(t *testing.T) {
	tests := []struct {
		name        string
		second      PipelineBinding
		wantErr     bool
		wantWarning bool
	}{
		{"overlap different pipelines", compileBinding(2, "other", "standard"), true, false},
		{"overlap agreeing pipeline", compileBinding(2, "regulated", "standard"), false, false},
		{"disjoint", compileBinding(2, "other", "invoice"), false, false},
		{"unanalyzable", PipelineBinding{ID: 2, Name: "complex", Priority: 10, Scope: PipelineBindingScopeSystem, BindingKind: PipelineBindingKindConditional, PipelineName: "other", Active: true, Predicate: semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "not", Items: []semrules.Predicate{{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "invoice"}}}}}, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := compilePolicyFixture()
			definition.ClearanceReferences = nil
			definition.Pipelines = append(definition.Pipelines, ProductionPipelineSpec{Name: "other"})
			definition.Bindings = append(definition.Bindings, test.second)
			compiled, err := CompilePolicy(definition)
			if (err != nil) != test.wantErr {
				t.Fatalf("err=%v compiled=%+v", err, compiled)
			}
			if test.wantWarning && !containsCompileWarning(compiled.Warnings, "runtime_conflict_check_required") {
				t.Fatalf("warnings=%v", compiled.Warnings)
			}
		})
	}
}

func TestCompilePolicyAllowsOverlappingGateEffectsWithDeterministicOrder(t *testing.T) {
	definition := compilePolicyFixture()
	definition.ClearanceReferences = nil
	second := definition.Gates[0]
	second.ID, second.Effect, second.PredicateChecksum = 12, GateEffectRequire, ""
	definition.Gates = append(definition.Gates, second)
	if _, err := CompilePolicy(definition); err != nil {
		t.Fatal(err)
	}
}

func TestCompilePolicyChecksumIsStableAcrossStorageOrder(t *testing.T) {
	definition := compilePolicyFixture()
	definition.ClearanceReferences = nil
	definition.Pipelines = append(definition.Pipelines, ProductionPipelineSpec{Name: "legacy_default"})
	definition.Bindings = append(definition.Bindings, compileBinding(2, "regulated", "invoice"))
	first, err := CompilePolicy(definition)
	if err != nil {
		t.Fatal(err)
	}
	definition.Bindings[0], definition.Bindings[1] = definition.Bindings[1], definition.Bindings[0]
	definition.Pipelines[0], definition.Pipelines[1] = definition.Pipelines[1], definition.Pipelines[0]
	second, err := CompilePolicy(definition)
	if err != nil {
		t.Fatal(err)
	}
	if first.Checksum != second.Checksum {
		t.Fatalf("checksum drift: %s != %s", first.Checksum, second.Checksum)
	}
}

func compilePolicyFixture() PolicyDefinition {
	binding := compileBinding(1, "regulated", "standard")
	gate := PipelineGate{ID: 11, Name: "skip metrics", Priority: 10, TargetProcessor: "extract_metrics", Effect: GateEffectSkip, Active: true, Predicate: predicateDoc("document.doc_kind", "standard")}
	_, gate.PredicateChecksum, _ = semrules.Canonicalize(gate.Predicate)
	return PolicyDefinition{ID: 7, Version: 3, Pipelines: []ProductionPipelineSpec{{Name: "regulated", Processors: []string{"extract_metrics"}}}, KnownProcessors: []string{"extract_metrics"}, Bindings: []PipelineBinding{binding}, Gates: []PipelineGate{gate}, ClearanceReferences: []PolicyClearanceReference{{SubjectKind: "processor_rule", SubjectID: gate.ID, SubjectChecksum: ProcessorGateSubjectChecksum(3, gate)}}}
}

func compileBinding(id int64, pipeline, kind string) PipelineBinding {
	binding := PipelineBinding{ID: id, Name: "binding", Priority: 10, Scope: PipelineBindingScopeSystem, BindingKind: PipelineBindingKindConditional, PipelineName: pipeline, Active: true, Predicate: predicateDoc("document.doc_kind", kind)}
	_, binding.PredicateChecksum, _ = semrules.Canonicalize(binding.Predicate)
	return binding
}

func predicateDoc(path, value string) semrules.Document {
	return semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: path, Op: "eq", Value: value}}
}

func containsCompileWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

// TestCompilePolicyAcceptsCanonicalPromotedBinding proves the create → approve
// → promote → compile chain closes for a proposal whose predicate + checksum
// are canonical -- exactly what modules.ProposalStore.CreateProposal now
// stores after the P5-3 fix (P5 review 2026080302 finding P5-3). The promoted
// binding materialized by EnsureDraftFromModuleRelease carries those bytes
// verbatim, so compilePredicate's canonical-checksum check must accept them.
// The negative case pins the old scheme (prefixed hash of raw client bytes)
// as the failure this fix removes.
func TestCompilePolicyAcceptsCanonicalPromotedBinding(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"standard"}}`)
	var doc semrules.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	canonical, checksum, err := semrules.Canonicalize(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(checksum, "sha256:") {
		t.Fatal("canonical checksum must be bare hex, not sha256:-prefixed")
	}

	// Positive: the promoted canonical binding compiles.
	definition := compilePolicyFixture()
	definition.Bindings[0].Predicate = doc
	definition.Bindings[0].PredicateChecksum = checksum
	compiled, err := CompilePolicy(definition)
	if err != nil {
		t.Fatalf("promoted canonical binding failed to compile: %v", err)
	}
	if len(compiled.BindingChecksums) != 1 || compiled.BindingChecksums[0] != checksum {
		t.Fatalf("binding checksums = %v, want [%s]", compiled.BindingChecksums, checksum)
	}

	// The stored predicate bytes are themselves canonical, so re-marshalling
	// them and canonicalising again is stable.
	var stored semrules.Document
	if err := json.Unmarshal(canonical, &stored); err != nil {
		t.Fatal(err)
	}
	_, roundTrip, err := semrules.Canonicalize(stored)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip != checksum {
		t.Fatalf("round-trip checksum %q != %q", roundTrip, checksum)
	}

	// Negative: the old prefixed raw-bytes hash never matches.
	sum := sha256.Sum256(raw)
	legacyHash := "sha256:" + hex.EncodeToString(sum[:])
	definition = compilePolicyFixture()
	definition.Bindings[0].PredicateChecksum = legacyHash
	_, err = CompilePolicy(definition)
	if err == nil || !strings.Contains(err.Error(), "predicate checksum mismatch") {
		t.Fatalf("expected the old prefixed raw-bytes checksum to fail compilation, err=%v", err)
	}
}
