package docprocessing

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
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
			`{"RequestedPipeline":"","StoreBoundPipeline":"","RuleName":"","Source":"system_default","SelectedPipeline":"legacy_default","SelectedPipelineVersion":0}`,
			`{"Name":"legacy_default","DisplayName":"Legacy Default","Processors":null,"LegacyEquivalent":true,"Version":0}`,
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

func TestCreateDocProcessPlan_NilExcludedByPolicyDoesNotViolateNotNull(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.doc_process_plans (run_id, record_id, plan_facts, plan_steps, pipeline_selection, pipeline_binding, pipeline_spec, excluded_by_policy)
VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, $8)
RETURNING id`)

	// excluded_by_policy is NOT NULL in the schema. A nil []string (the
	// common case: ProductionProcessorPlan.ExcludedByPolicy() returns nil
	// when nothing was excluded) must be sent as an empty array literal,
	// not as SQL NULL: pq.StringArray(nil).Value() returns NULL while
	// pq.StringArray{}.Value() returns "{}".
	mock.ExpectQuery(insertQuery).
		WithArgs(
			int64(5),
			int64(4821),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"{}",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))

	store := SQLStore{DB: db}
	if _, err := store.CreateDocProcessPlan(context.Background(), DocProcessPlanRecord{
		RunID:            5,
		RecordID:         4821,
		ExcludedByPolicy: nil,
	}); err != nil {
		t.Fatalf("CreateDocProcessPlan: %v", err)
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

func TestPersistedP5PlanReloadIgnoresLaterActivation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wantSnapshot := &P5RoutingSnapshot{
		Facts:           []semrules.Fact{{Path: "document.doc_kind", State: semrules.FactKnown, Value: "standard"}},
		PipelineName:    "old", PipelineVersion: 3,
		SelectedPipelineChecksum: "sha256:selected", BaselinePipelineChecksum: "sha256:baseline",
		GateShadow:    ProcessorGateShadowPlan{EffectiveProcessors: []string{"extract_metrics"}, WouldSkip: []string{"extract_metrics"}},
		RuleChecksums: []string{"sha256:old-rule"},
	}
	factsRaw, _ := json.Marshal(ProductionPlanFacts{RequestedProcessors: []string{"extract_metrics"}, RoutingSnapshot: wantSnapshot})
	stepsRaw, _ := json.Marshal([]ProcessorPlanStep{{Name: "extract_metrics", Phase: "B", Reason: "explicit_request"}})
	selectionRaw, _ := json.Marshal(ProductionPipelineSelection{PipelineName: "old", Reason: "conditional"})
	bindingRaw, _ := json.Marshal(ProductionPipelineBindingResolution{SelectedPipeline: "old", Source: "conditional"})
	specRaw, _ := json.Marshal(ProductionPipelineSpec{Name: "old", Processors: []string{"extract_metrics"}})
	mock.ExpectQuery("SELECT p.run_id,").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{
		"run_id", "record_id", "facts", "steps", "selection", "binding", "spec", "excluded", "mode", "status", "processors", "parameters", "created",
	}).AddRow(int64(9), int64(42), string(factsRaw), string(stepsRaw), string(selectionRaw), string(bindingRaw), string(specRaw), "{}", "plan_only", "succeeded", `["extract_metrics"]`, `{}`, "2026-08-02T00:00:00+00"))

	SetProductionPipelineRegistry([]ProductionPipelineSpec{{Name: "new", Processors: []string{"extract_provisions"}}})
	SetProductionPipelineGates([]PipelineGate{gateFixtureForProcessor(99, "extract_metrics", GateEffectRequire)})
	defer SetProductionPipelineRegistry(nil)
	defer SetProductionPipelineGates(nil)
	view, err := (SQLStore{DB: db}).GetLatestDocProcessPlan(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view.PlanFacts.RoutingSnapshot, wantSnapshot) || view.PipelineSpec.Name != "old" {
		t.Fatalf("reloaded view=%+v", view)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
