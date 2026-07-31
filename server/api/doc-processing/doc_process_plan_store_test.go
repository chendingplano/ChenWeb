package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateDocProcessPlan_InsertsAndReturnsID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.doc_process_plans (run_id, record_id, plan_facts, plan_steps, pipeline_selection, pipeline_binding, pipeline_spec, excluded_by_policy)
VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, $8)
RETURNING id`)

	facts := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics", "extract_provisions"},
		KnowledgeStoreID:    42,
		InputDocType:        "pdf",
		SourceLanguage:      "en",
		DocumentNumber:      "YY 9706.252-2021",
		ParserName:          "mineru",
		DocumentTitle:       "Ventilator display module",
		RoutingFacets: ProductionRoutingFacets{
			KnowledgeStoreBinding: "bound",
			InputDocType:          "pdf",
			SourceLanguage:        "en",
			HasDocumentNumber:     true,
		},
	}
	steps := []ProcessorPlanStep{
		{Name: "static_analyzer", Phase: "A", DependsOn: []string{}, Reason: "mandatory_baseline"},
		{Name: "chunking", Phase: "A", DependsOn: []string{"static_analyzer"}, Reason: "implicit_dependency"},
		{Name: "generate_topics", Phase: "B", DependsOn: []string{"chunking"}, Reason: "explicit_request"},
		{Name: "extract_provisions", Phase: "B", DependsOn: []string{"chunking"}, Reason: "explicit_request"},
	}
	selection := ProductionPipelineSelection{
		PipelineName: "legacy_default",
		Reason:       "system_default",
	}
	binding := ProductionPipelineBindingResolution{
		Source:           "system_default",
		SelectedPipeline: "legacy_default",
	}
	spec := ProductionPipelineSpec{
		Name:             "legacy_default",
		DisplayName:      "Legacy Default",
		LegacyEquivalent: true,
	}

	mock.ExpectQuery(insertQuery).
		WithArgs(
			int64(5),
			int64(4821),
			`{"RequestedProcessors":["generate_topics","extract_provisions"],"RequestedPipeline":"","StoreBoundPipeline":"","KnowledgeStoreID":42,"KnowledgeStoreType":"","InputDocType":"pdf","SourceLanguage":"en","DocumentNumber":"YY 9706.252-2021","ParserName":"mineru","DocumentTitle":"Ventilator display module","RoutingFacets":{"KnowledgeStoreBinding":"bound","InputDocType":"pdf","SourceLanguage":"en","HasDocumentNumber":true},"Mode":""}`,
			`[{"Name":"static_analyzer","Phase":"A","DependsOn":[],"Reason":"mandatory_baseline"},{"Name":"chunking","Phase":"A","DependsOn":["static_analyzer"],"Reason":"implicit_dependency"},{"Name":"generate_topics","Phase":"B","DependsOn":["chunking"],"Reason":"explicit_request"},{"Name":"extract_provisions","Phase":"B","DependsOn":["chunking"],"Reason":"explicit_request"}]`,
			`{"PipelineName":"legacy_default","Reason":"system_default"}`,
			`{"RequestedPipeline":"","StoreBoundPipeline":"","Source":"system_default","SelectedPipeline":"legacy_default"}`,
			`{"Name":"legacy_default","DisplayName":"Legacy Default","Processors":null,"LegacyEquivalent":true}`,
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))

	store := SQLStore{DB: db}
	id, err := store.CreateDocProcessPlan(context.Background(), DocProcessPlanRecord{
		RunID:             5,
		RecordID:          4821,
		PlanFacts:         facts,
		PlanSteps:         steps,
		PipelineSelection: selection,
		PipelineBinding:   binding,
		PipelineSpec:      spec,
		ExcludedByPolicy:  []string{"extract_provisions"},
	})
	if err != nil {
		t.Fatalf("CreateDocProcessPlan: %v", err)
	}
	if got, want := id, int64(9); got != want {
		t.Fatalf("id=%d want=%d", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCreateDocProcessPlan_RequiresRunIDAndRecordID(t *testing.T) {
	store := SQLStore{DB: nil}
	if _, err := store.CreateDocProcessPlan(context.Background(), DocProcessPlanRecord{RunID: 1}); err == nil {
		t.Fatal("expected error for db nil / missing record id")
	}

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()
	store = SQLStore{DB: db}
	if _, err := store.CreateDocProcessPlan(context.Background(), DocProcessPlanRecord{RecordID: 1}); err == nil {
		t.Fatal("expected error for missing run id")
	}
	if _, err := store.CreateDocProcessPlan(context.Background(), DocProcessPlanRecord{RunID: 1}); err == nil {
		t.Fatal("expected error for missing record id")
	}
}
