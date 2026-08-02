package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func TestPipelineBindingLegacyAdapterCanonicalPathsOrderAndChecksum(t *testing.T) {
	doc, checksum, err := LegacyRulePredicateDocument(ProductionPipelineRule{
		MatchInputDocType:          " PDF ",
		MatchSourceLanguage:        " EN ",
		MatchKnowledgeStoreBinding: " Bound ",
	})
	if err != nil {
		t.Fatalf("LegacyRulePredicateDocument: %v", err)
	}
	if checksum == "" {
		t.Fatal("checksum is empty")
	}

	items := doc.Expression.Items
	if len(items) != 3 {
		t.Fatalf("items=%d want 3", len(items))
	}
	want := []struct {
		path  string
		value any
	}{
		{"document.input_doc_type", "pdf"},
		{"document.source_language", "en"},
		{"document.knowledge_store_binding_state", "bound"},
	}
	for i := range want {
		if items[i].Path != want[i].path || items[i].Value != want[i].value {
			t.Fatalf("item[%d]=%+v want path=%s value=%v", i, items[i], want[i].path, want[i].value)
		}
	}

	again, againChecksum, err := LegacyRulePredicateDocument(ProductionPipelineRule{
		MatchInputDocType:          "pdf",
		MatchSourceLanguage:        "en",
		MatchKnowledgeStoreBinding: "bound",
	})
	if err != nil {
		t.Fatalf("LegacyRulePredicateDocument again: %v", err)
	}
	if checksum != againChecksum {
		t.Fatalf("checksum drift: %s != %s", checksum, againChecksum)
	}
	if encodedA, encodedB := mustJSONForTest(t, doc), mustJSONForTest(t, again); string(encodedA) != string(encodedB) {
		t.Fatalf("canonical adapter drift:\n%s\n%s", encodedA, encodedB)
	}
}

func TestPipelineBindingLegacyAdapterWildcardMatchesOldFlatRules(t *testing.T) {
	doc, _, err := LegacyRulePredicateDocument(ProductionPipelineRule{MatchInputDocType: "pdf"})
	if err != nil {
		t.Fatalf("LegacyRulePredicateDocument: %v", err)
	}
	result := semrules.EvaluateDocument(doc, semrules.FactSet{
		"document.input_doc_type":  {Path: "document.input_doc_type", State: semrules.FactKnown, Value: "pdf"},
		"document.source_language": {Path: "document.source_language", State: semrules.FactKnown, Value: "zh"},
	})
	if result.Truth != semrules.TruthTrue {
		t.Fatalf("truth=%s want true", result.Truth)
	}
}

func TestPipelineBindingDR7DecisionTable(t *testing.T) {
	facts := semrules.FactSet{
		"document.input_doc_type":  {Path: "document.input_doc_type", State: semrules.FactKnown, Value: "pdf"},
		"document.source_language": {Path: "document.source_language", State: semrules.FactKnown, Value: "en"},
	}
	pdf := mustLegacyBinding(t, "pdf", "pipeline_a", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"})
	en := mustLegacyBinding(t, "en", "pipeline_b", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchSourceLanguage: "en"})
	missingSame := mustLegacyBinding(t, "missing-same", "pipeline_a", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchKnowledgeStoreBinding: "bound"})
	lower := mustLegacyBinding(t, "lower", "pipeline_b", 1, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{})
	storeDefault := PipelineBinding{Name: "store", BindingKind: PipelineBindingKindStoreDefault, PipelineName: "store_default", Active: true}

	tests := []struct {
		name         string
		bindings     []PipelineBinding
		onConflict   string
		wantPipeline string
		wantSource   string
		wantErr      string
	}{
		{
			name:         "true selects before store default",
			bindings:     []PipelineBinding{storeDefault, pdf},
			wantPipeline: "pipeline_a",
			wantSource:   "conditional_binding",
		},
		{
			name:         "same pipeline indeterminate agreement selects",
			bindings:     []PipelineBinding{pdf, missingSame, lower},
			wantPipeline: "pipeline_a",
			wantSource:   "conditional_binding",
		},
		{
			name:       "true conflict blocks",
			bindings:   []PipelineBinding{pdf, en},
			wantErr:    "conflicting conditional pipeline bindings",
			wantSource: "",
		},
		{
			name:         "fallback discards ambiguous rank and uses lower rank",
			bindings:     []PipelineBinding{pdf, en, lower},
			onConflict:   PipelineBindingOnConflictFallback,
			wantPipeline: "pipeline_b",
			wantSource:   "conditional_binding",
		},
		{
			name:       "higher rank indeterminate blocks lower rank",
			bindings:   []PipelineBinding{missingSame, lower},
			wantErr:    "indeterminate conditional pipeline bindings",
			wantSource: "",
		},
		{
			name:         "fallback indeterminate rank uses store default",
			bindings:     []PipelineBinding{missingSame, storeDefault},
			onConflict:   PipelineBindingOnConflictFallback,
			wantPipeline: "store_default",
			wantSource:   "store_default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePipelineBindings(tt.bindings, facts, tt.onConflict)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%v want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePipelineBindings: %v", err)
			}
			if got.SelectedPipeline != tt.wantPipeline || got.Source != tt.wantSource {
				t.Fatalf("got=%+v want pipeline=%s source=%s", got, tt.wantPipeline, tt.wantSource)
			}
		})
	}
}

func TestPipelineBindingSQLStoreListPipelineBindingsReadsActivePolicyRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	doc, checksum, err := LegacyRulePredicateDocument(ProductionPipelineRule{MatchInputDocType: "pdf"})
	if err != nil {
		t.Fatalf("LegacyRulePredicateDocument: %v", err)
	}
	predicateJSON := string(mustJSONForTest(t, doc))
	query := regexp.QuoteMeta(`
SELECT b.id, COALESCE(b.name, ''), b.priority, b.binding_kind,
       COALESCE(p.name, ''), COALESCE(b.predicate, '{}'::jsonb)::text,
       COALESCE(b.predicate_checksum, ''), b.active,
       CASE
         WHEN b.input_record_id IS NOT NULL THEN 'document'
         WHEN NULLIF(b.user_id, '') IS NOT NULL THEN 'user'
         WHEN b.ks_store_id IS NOT NULL THEN 'knowledge_store'
         WHEN NULLIF(b.tenant_id, '') IS NOT NULL AND b.tenant_id <> '-' THEN 'tenant'
         ELSE 'system'
       END AS binding_scope
FROM kb.pipeline_bindings b
LEFT JOIN kb.pipelines p ON p.id = b.pipeline_id
WHERE b.active AND b.policy_id = (SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1)
ORDER BY b.priority DESC,
         CASE
           WHEN b.input_record_id IS NOT NULL THEN 4
           WHEN NULLIF(b.user_id, '') IS NOT NULL THEN 3
           WHEN b.ks_store_id IS NOT NULL THEN 2
           WHEN NULLIF(b.tenant_id, '') IS NOT NULL AND b.tenant_id <> '-' THEN 1
           ELSE 0
         END DESC,
         b.id`)
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "priority", "binding_kind", "pipeline_name", "predicate", "predicate_checksum", "active", "binding_scope",
	}).AddRow(
		int64(7), "pdf-binding", 10, PipelineBindingKindConditional, "regulated_reference", predicateJSON, checksum, true, PipelineBindingScopeKnowledgeStore,
	).AddRow(
		int64(8), "store-default", 0, PipelineBindingKindStoreDefault, "store_default", `{}`, "", true, PipelineBindingScopeKnowledgeStore,
	))

	got, err := (PipelineBindingSQLStore{DB: db}).ListPipelineBindings(context.Background())
	if err != nil {
		t.Fatalf("ListPipelineBindings: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d bindings, want 2: %#v", len(got), got)
	}
	if got[0].Name != "pdf-binding" || got[0].PipelineName != "regulated_reference" || got[0].Scope != PipelineBindingScopeKnowledgeStore {
		t.Fatalf("binding[0]=%#v", got[0])
	}
	if got[0].Predicate.Expression.Items[0].Path != "document.input_doc_type" {
		t.Fatalf("predicate=%#v", got[0].Predicate)
	}
	if got[1].BindingKind != PipelineBindingKindStoreDefault || got[1].PipelineName != "store_default" {
		t.Fatalf("binding[1]=%#v", got[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPipelineBindingSQLStoreRejectsNilDB(t *testing.T) {
	if _, err := (PipelineBindingSQLStore{}).ListPipelineBindings(context.Background()); err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestLoadProductionPipelineBindingsInstallsBindings(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineBindings(nil) })
	binding := mustLegacyBinding(t, "pdf", "pipeline_a", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"})
	if err := LoadProductionPipelineBindings(context.Background(), fakePipelineBindingStore{bindings: []PipelineBinding{binding}}); err != nil {
		t.Fatalf("LoadProductionPipelineBindings: %v", err)
	}
	got := currentProductionPipelineBindings()
	if len(got) != 1 || got[0].Name != "pdf" {
		t.Fatalf("bindings=%#v", got)
	}
}

func TestLoadProductionPipelineBindingsClearsStaleBindings(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineBindings(nil) })
	SetProductionPipelineBindings([]PipelineBinding{{Name: "stale"}})
	if err := LoadProductionPipelineBindings(context.Background(), fakePipelineBindingStore{}); err != nil {
		t.Fatalf("LoadProductionPipelineBindings: %v", err)
	}
	if got := currentProductionPipelineBindings(); len(got) != 0 {
		t.Fatalf("bindings=%#v, want empty", got)
	}
}

func TestLoadProductionPipelineBindingsRejectsNilStore(t *testing.T) {
	if err := LoadProductionPipelineBindings(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil store")
	}
}

var errFakePipelineBindingStore = errors.New("pipeline binding store failure")

func TestLoadProductionPipelineBindingsPropagatesStoreError(t *testing.T) {
	if err := LoadProductionPipelineBindings(context.Background(), fakePipelineBindingStore{err: errFakePipelineBindingStore}); err == nil {
		t.Fatal("expected error from failing store")
	}
}

func mustLegacyBinding(t *testing.T, name, pipeline string, priority int, scope string, rule ProductionPipelineRule) PipelineBinding {
	t.Helper()
	doc, checksum, err := LegacyRulePredicateDocument(rule)
	if err != nil {
		t.Fatalf("LegacyRulePredicateDocument: %v", err)
	}
	return PipelineBinding{
		Name:              name,
		PipelineName:      pipeline,
		Priority:          priority,
		Scope:             scope,
		BindingKind:       PipelineBindingKindConditional,
		Predicate:         doc,
		PredicateChecksum: checksum,
		Active:            true,
	}
}

type fakePipelineBindingStore struct {
	bindings []PipelineBinding
	err      error
}

func (f fakePipelineBindingStore) ListPipelineBindings(context.Context) ([]PipelineBinding, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bindings, nil
}

func mustJSONForTest(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
