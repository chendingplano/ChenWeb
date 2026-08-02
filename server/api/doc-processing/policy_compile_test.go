package docprocessing

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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

func TestPolicyCompilerSQLStoreLoadsLegacyAdapterForParity(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy := ProductionPipelineRule{ID: 41, MatchInputDocType: "pdf", MatchSourceLanguage: "en", PipelineName: "regulated"}
	doc, checksum, _ := LegacyRulePredicateDocument(legacy)
	raw, _ := json.Marshal(doc)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT b.id,COALESCE(b.name,''),b.priority,b.binding_kind,p.name")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "priority", "kind", "pipeline", "predicate", "checksum", "active", "scope", "legacy_id", "doc_type", "language", "binding",
	}).AddRow(int64(9), "legacy", 10, "conditional", "regulated", string(raw), "migration-md5", true, "system", int64(41), "pdf", "en", ""))
	bindings, err := (PolicyCompilerSQLStore{DB: db}).loadBindings(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].LegacyRule == nil || bindings[0].PredicateChecksum != checksum {
		t.Fatalf("bindings=%+v", bindings)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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
