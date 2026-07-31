package docprocessing

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPipelineRuleSQLStoreListPipelineRulesReadsActiveRulesOrderedByPriority(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	query := regexp.QuoteMeta(`
SELECT r.id, r.name, r.priority,
       COALESCE(r.match_input_doc_type, ''),
       COALESCE(r.match_source_language, ''),
       COALESCE(r.match_knowledge_store_binding, ''),
       p.name
FROM kb.pipeline_rules r
JOIN kb.pipelines p ON p.id = r.pipeline_id
WHERE r.active
ORDER BY r.priority DESC, r.id`)
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "priority", "match_input_doc_type", "match_source_language", "match_knowledge_store_binding", "pipeline_name",
	}).AddRow(
		int64(2), "pdf-zh-specific", 10, "PDF", " ZH ", "", "regulated_reference",
	).AddRow(
		int64(1), "pdf-general", 1, "pdf", "", "", "narrative_default",
	))

	store := PipelineRuleSQLStore{DB: db}
	got, err := store.ListPipelineRules(context.Background())
	if err != nil {
		t.Fatalf("ListPipelineRules: %v", err)
	}
	want := []ProductionPipelineRule{
		{ID: 2, Name: "pdf-zh-specific", Priority: 10, MatchInputDocType: "pdf", MatchSourceLanguage: "zh", PipelineName: "regulated_reference"},
		{ID: 1, Name: "pdf-general", Priority: 1, MatchInputDocType: "pdf", PipelineName: "narrative_default"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rules, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rule[%d]=%#v want=%#v", i, got[i], want[i])
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPipelineRuleSQLStoreListPipelineRulesRejectsNilDB(t *testing.T) {
	store := PipelineRuleSQLStore{}
	if _, err := store.ListPipelineRules(context.Background()); err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestLoadProductionPipelineRulesInstallsRules(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRules(nil) })

	if err := LoadProductionPipelineRules(context.Background(), fakePipelineRuleStore{
		rules: []ProductionPipelineRule{{Name: "r1", Priority: 1, MatchInputDocType: "pdf", PipelineName: "narrative_default"}},
	}); err != nil {
		t.Fatalf("LoadProductionPipelineRules: %v", err)
	}

	_, pipeline, matched, err := resolveProductionPipelineRuleMatchName(ProductionRoutingFacets{InputDocType: "pdf"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !matched || pipeline != "narrative_default" {
		t.Fatalf("pipeline=%q matched=%v", pipeline, matched)
	}
}

func TestLoadProductionPipelineRulesEmptyResultIsNotAnError(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineRules(nil) })
	SetProductionPipelineRules([]ProductionPipelineRule{{Name: "stale", PipelineName: "narrative_default"}})

	if err := LoadProductionPipelineRules(context.Background(), fakePipelineRuleStore{}); err != nil {
		t.Fatalf("LoadProductionPipelineRules: %v", err)
	}

	// Unlike LoadProductionPipelineRegistry, an empty authored rule set is a
	// valid state and replaces whatever was previously installed.
	_, _, matched, err := resolveProductionPipelineRuleMatchName(ProductionRoutingFacets{})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if matched {
		t.Fatal("expected no rules installed after an empty load")
	}
}

func TestLoadProductionPipelineRulesRejectsNilStore(t *testing.T) {
	if err := LoadProductionPipelineRules(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil store")
	}
}

var errFakePipelineRuleStore = errors.New("pipeline rule store failure")

func TestLoadProductionPipelineRulesPropagatesStoreError(t *testing.T) {
	if err := LoadProductionPipelineRules(context.Background(), fakePipelineRuleStore{err: errFakePipelineRuleStore}); err == nil {
		t.Fatal("expected error from failing store")
	}
}

type fakePipelineRuleStore struct {
	rules []ProductionPipelineRule
	err   error
}

func (f fakePipelineRuleStore) ListPipelineRules(context.Context) ([]ProductionPipelineRule, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rules, nil
}
