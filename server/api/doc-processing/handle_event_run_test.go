package docprocessing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// fakePolicyAuditEventWriter records every event handed to it so
// control.go's conflict/fallback/decision event wiring can be asserted
// end-to-end through handleEvent without a database.
type fakePolicyAuditEventWriter struct {
	mu     sync.Mutex
	events []policyaudit.Event
}

func (w *fakePolicyAuditEventWriter) WriteEvent(_ context.Context, event policyaudit.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event)
	return nil
}

func (w *fakePolicyAuditEventWriter) snapshot() []policyaudit.Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]policyaudit.Event(nil), w.events...)
}

type fakeRunStoreClose struct {
	runID  int64
	status string
	errMsg *string
}

type fakePlanStore struct {
	mu      sync.Mutex
	nextID  int64
	creates []DocProcessPlanRecord
}

func (f *fakePlanStore) CreateDocProcessPlan(_ context.Context, rec DocProcessPlanRecord) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.creates = append(f.creates, rec)
	return f.nextID, nil
}

func (f *fakePlanStore) GetLatestDocProcessPlan(_ context.Context, _ int64) (DocProcessPlanView, error) {
	return DocProcessPlanView{}, errors.New("not implemented")
}

func (f *fakePlanStore) ListDocProcessPlans(_ context.Context, _ int64, _ int, _ int, _ string, _ string, _ string) ([]DocProcessPlanView, int64, error) {
	return nil, 0, errors.New("not implemented")
}

// fakeRunStore is a test double for DocProcessRunStore that records every
// create/close call so handleEvent's run lifecycle wiring can be asserted
// without a real database. See ADR 2026071201.
type fakeRunStore struct {
	mu        sync.Mutex
	nextID    int64
	creates   []DocProcessRunRecord
	closes    []fakeRunStoreClose
	createErr error
}

func (f *fakeRunStore) CreateDocProcessRun(_ context.Context, rec DocProcessRunRecord) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.nextID++
	f.creates = append(f.creates, rec)
	return f.nextID, nil
}

func (f *fakeRunStore) CloseDocProcessRun(_ context.Context, runID int64, status string, errMsg *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes = append(f.closes, fakeRunStoreClose{runID: runID, status: status, errMsg: errMsg})
	return nil
}

// runIDCapturingProcessor records the run id (if any) visible on its ctx when
// handleEvent invokes it, so tests can assert the id created for this
// invocation actually reaches processor call sites.
type runIDCapturingProcessor struct {
	name string
	got  *int64
	ok   *bool
}

func (p runIDCapturingProcessor) Name() string { return p.name }

func (p runIDCapturingProcessor) HandleEvent(ctx context.Context, _ []byte) error {
	id, ok := runIDFromContext(ctx)
	*p.got = id
	*p.ok = ok
	return nil
}

func TestHandleEvent_CreatesAndClosesRunAndThreadsRunIDToProcessors(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:              4821,
			ParserName:      "opendata",
			ResultFilename:  "result.json",
			StagingFilename: inputPath,
			StatusRaw:       "[]",
			InputDocType:    "pdf",
			SourceLanguage:  "en",
			DocumentNumber:  "YY 9706.252-2021",
		},
	}
	runStore := &fakeRunStore{}
	planStore := &fakePlanStore{}
	var gotRunID int64
	var gotOK bool
	svc := &ControlService{
		InputStore: store,
		RunStore:   runStore,
		PlanStore:  planStore,
		Processors: []Processor{
			runIDCapturingProcessor{name: "extract_metrics", got: &gotRunID, ok: &gotOK},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`",
		"operation":["extract_metrics"]
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	if !gotOK {
		t.Fatal("expected extract_metrics to see a run id on its context")
	}

	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.creates) != 1 {
		t.Fatalf("creates=%d, want 1", len(runStore.creates))
	}
	create := runStore.creates[0]
	if create.RecordID != 4821 {
		t.Errorf("RecordID=%d, want 4821", create.RecordID)
	}
	if create.Mode != "auto" {
		t.Errorf("Mode=%q, want auto", create.Mode)
	}
	if len(create.Processors) != 1 || create.Processors[0] != "extract_metrics" {
		t.Errorf("Processors=%v, want [extract_metrics]", create.Processors)
	}
	if force, ok := create.Parameters["force"].(bool); !ok || !force {
		t.Errorf("Parameters[force]=%v, want true (event omitted force, defaults to true)", create.Parameters["force"])
	}
	rawFacts, ok := create.Parameters["processor_plan_facts"].(ProductionPlanFacts)
	if !ok {
		t.Fatalf("processor_plan_facts type=%T", create.Parameters["processor_plan_facts"])
	}
	if rawFacts.KnowledgeStoreID != 0 {
		t.Errorf("processor_plan_facts.KnowledgeStoreID=%d, want 0", rawFacts.KnowledgeStoreID)
	}
	if got, want := rawFacts.RequestedProcessors, []string{"extract_metrics"}; len(got) != len(want) || got[0] != want[0] {
		t.Errorf("processor_plan_facts.RequestedProcessors=%v, want %v", got, want)
	}
	if got, want := rawFacts.InputDocType, "pdf"; got != want {
		t.Errorf("processor_plan_facts.InputDocType=%q, want %q", got, want)
	}
	if got, want := rawFacts.SourceLanguage, "en"; got != want {
		t.Errorf("processor_plan_facts.SourceLanguage=%q, want %q", got, want)
	}
	if got, want := rawFacts.DocumentNumber, "YY 9706.252-2021"; got != want {
		t.Errorf("processor_plan_facts.DocumentNumber=%q, want %q", got, want)
	}
	if got, want := rawFacts.RoutingFacets, (ProductionRoutingFacets{
		KnowledgeStoreBinding: "absent",
		InputDocType:          "pdf",
		SourceLanguage:        "en",
		HasDocumentNumber:     true,
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("processor_plan_facts.RoutingFacets=%#v, want %#v", got, want)
	}
	rawSteps, ok := create.Parameters["processor_plan_steps"].([]ProcessorPlanStep)
	if !ok {
		t.Fatalf("processor_plan_steps type=%T", create.Parameters["processor_plan_steps"])
	}
	if got, want := len(rawSteps), 3; got != want {
		t.Fatalf("processor_plan_steps len=%d, want %d", got, want)
	}
	if got, want := rawSteps[2].Name, "extract_metrics"; got != want {
		t.Errorf("processor_plan_steps[2].Name=%q, want %q", got, want)
	}
	rawSelection, ok := create.Parameters["processor_pipeline_selection"].(ProductionPipelineSelection)
	if !ok {
		t.Fatalf("processor_pipeline_selection type=%T", create.Parameters["processor_pipeline_selection"])
	}
	if got, want := rawSelection, (ProductionPipelineSelection{
		PipelineName: "legacy_default",
		Reason:       "system_default",
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("processor_pipeline_selection=%#v, want %#v", got, want)
	}
	rawBinding, ok := create.Parameters["processor_pipeline_binding"].(ProductionPipelineBindingResolution)
	if !ok {
		t.Fatalf("processor_pipeline_binding type=%T", create.Parameters["processor_pipeline_binding"])
	}
	if got, want := rawBinding, (ProductionPipelineBindingResolution{
		Source:           "system_default",
		SelectedPipeline: "legacy_default",
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("processor_pipeline_binding=%#v, want %#v", got, want)
	}

	statusEntries := decodeDocMetaStatus(store.currentStatusRaw())
	var docProcessing map[string]any
	for _, entry := range statusEntries {
		if strings.TrimSpace(asString(entry["operation"])) == "doc_processing" {
			docProcessing = entry
			break
		}
	}
	if docProcessing == nil {
		t.Fatalf("missing doc_processing status entry in %s", store.currentStatusRaw())
	}
	if _, ok := docProcessing["processor_plan_facts"]; !ok {
		t.Fatalf("doc_processing status missing processor_plan_facts: %#v", docProcessing)
	}
	if _, ok := docProcessing["processor_plan_steps"]; !ok {
		t.Fatalf("doc_processing status missing processor_plan_steps: %#v", docProcessing)
	}
	if _, ok := docProcessing["processor_pipeline_selection"]; !ok {
		t.Fatalf("doc_processing status missing processor_pipeline_selection: %#v", docProcessing)
	}
	if _, ok := docProcessing["processor_pipeline_binding"]; !ok {
		t.Fatalf("doc_processing status missing processor_pipeline_binding: %#v", docProcessing)
	}

	if len(runStore.closes) != 1 {
		t.Fatalf("closes=%d, want 1", len(runStore.closes))
	}
	closeCall := runStore.closes[0]
	if closeCall.runID != gotRunID {
		t.Errorf("close runID=%d, want %d (the id created for this invocation)", closeCall.runID, gotRunID)
	}
	if closeCall.status != "success" {
		t.Errorf("close status=%q, want success", closeCall.status)
	}

	planStore.mu.Lock()
	defer planStore.mu.Unlock()
	if len(planStore.creates) != 1 {
		t.Fatalf("plan creates=%d, want 1", len(planStore.creates))
	}
	planCreate := planStore.creates[0]
	if planCreate.RunID != gotRunID {
		t.Errorf("plan run_id=%d, want %d", planCreate.RunID, gotRunID)
	}
	if planCreate.RecordID != 4821 {
		t.Errorf("plan record_id=%d, want 4821", planCreate.RecordID)
	}
	if got, want := planCreate.PlanFacts.RequestedProcessors, []string{"extract_metrics"}; len(got) != len(want) || got[0] != want[0] {
		t.Errorf("plan facts requested=%v, want %v", got, want)
	}
	if got, want := len(planCreate.PlanSteps), 3; got != want {
		t.Errorf("plan steps len=%d, want %d", got, want)
	}
	if got, want := planCreate.PipelineSelection, (ProductionPipelineSelection{
		PipelineName: "legacy_default",
		Reason:       "system_default",
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("plan pipeline selection=%#v, want %#v", got, want)
	}
	if got, want := planCreate.PipelineBinding, (ProductionPipelineBindingResolution{
		Source:           "system_default",
		SelectedPipeline: "legacy_default",
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("plan pipeline binding=%#v, want %#v", got, want)
	}
	if got, want := planCreate.PlanFacts.InputDocType, "pdf"; got != want {
		t.Errorf("plan facts InputDocType=%q, want %q", got, want)
	}
	if got, want := planCreate.PlanFacts.SourceLanguage, "en"; got != want {
		t.Errorf("plan facts SourceLanguage=%q, want %q", got, want)
	}
	if got, want := planCreate.PlanFacts.DocumentNumber, "YY 9706.252-2021"; got != want {
		t.Errorf("plan facts DocumentNumber=%q, want %q", got, want)
	}
	if got, want := planCreate.PlanFacts.RoutingFacets, (ProductionRoutingFacets{
		KnowledgeStoreBinding: "absent",
		InputDocType:          "pdf",
		SourceLanguage:        "en",
		HasDocumentNumber:     true,
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("plan facts RoutingFacets=%#v, want %#v", got, want)
	}
}

// TestHandleEvent_ExplicitOperationsBypassPolicyGates proves C1's run-scoped
// processor override precedence: operation/processor_override payload fields
// select processors directly for this run, even when enforced policy would
// normally exclude one of them. The no-explicit-operations test below still
// proves common ingestion is policy-enforced.
func TestHandleEvent_ExplicitOperationsBypassPolicyGates(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	t.Setenv("DOC_PIPELINE_PLAN_ONLY", "false")
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "legacy_default", LegacyEquivalent: true},
		{Name: "metrics_only", Processors: []string{"extract_metrics"}},
	})

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:                4821,
			ParserName:        "opendata",
			ResultFilename:    "result.json",
			StagingFilename:   inputPath,
			StatusRaw:         "[]",
			RequestedPipeline: "metrics_only",
			InputDocType:      "pdf",
			SourceLanguage:    "en",
		},
	}
	runStore := &fakeRunStore{}
	planStore := &fakePlanStore{}

	var calls []string
	svc := &ControlService{
		InputStore: store,
		RunStore:   runStore,
		PlanStore:  planStore,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &calls},
			fakeProcessor{name: "extract_provisions", calls: &calls},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`",
		"operation":["extract_metrics","extract_provisions"]
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	if got, want := calls, []string{"extract_metrics", "extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Errorf("processor calls=%v, want %v (explicit operations bypass policy gates)", got, want)
	}

	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.creates) != 1 {
		t.Fatalf("creates=%d, want 1", len(runStore.creates))
	}
	if got, want := runStore.creates[0].Processors, []string{"extract_metrics", "extract_provisions"}; !reflect.DeepEqual(got, want) {
		t.Errorf("persisted run Processors=%v, want %v", got, want)
	}
	if got := runStore.creates[0].Parameters["processor_override_bypasses_policy"]; got != true {
		t.Errorf("processor_override_bypasses_policy=%v, want true", got)
	}

	planStore.mu.Lock()
	defer planStore.mu.Unlock()
	if len(planStore.creates) != 1 {
		t.Fatalf("plan creates=%d, want 1", len(planStore.creates))
	}
	if got := planStore.creates[0].ExcludedByPolicy; len(got) != 0 {
		t.Errorf("plan ExcludedByPolicy=%v, want empty because explicit operations bypass policy gates", got)
	}
	if got, want := planStore.creates[0].PlanFacts.Mode, DocPipelineModePlanOnly; got != want {
		t.Errorf("plan mode=%q, want %q", got, want)
	}
}

func TestHandleEvent_PipelineOverrideOutranksCanonicalBindingAndStoreDefault(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	t.Setenv("DOC_PIPELINE_PLAN_ONLY", "false")
	t.Cleanup(func() {
		SetProductionPipelineRegistry(nil)
		SetProductionPipelineBindings(nil)
	})
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "legacy_default", LegacyEquivalent: true},
		{Name: "store_default"},
		{Name: "policy_selected"},
		{Name: "override_pipeline"},
	})
	SetProductionPipelineBindings([]PipelineBinding{
		mustLegacyBinding(t, "pdf-policy", "policy_selected", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"}),
	})

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:                 4821,
			ParserName:         "opendata",
			ResultFilename:     "result.json",
			StagingFilename:    inputPath,
			StatusRaw:          "[]",
			StoreBoundPipeline: "store_default",
			InputDocType:       "pdf",
			SourceLanguage:     "en",
		},
	}
	runStore := &fakeRunStore{}
	planStore := &fakePlanStore{}
	var calls []string
	svc := &ControlService{
		InputStore: store,
		RunStore:   runStore,
		PlanStore:  planStore,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &calls},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`",
		"pipeline_override":"override_pipeline"
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	planStore.mu.Lock()
	defer planStore.mu.Unlock()
	if len(planStore.creates) != 1 {
		t.Fatalf("plan creates=%d, want 1", len(planStore.creates))
	}
	binding := planStore.creates[0].PipelineBinding
	if binding.Source != "explicit_request" || binding.SelectedPipeline != "override_pipeline" || binding.RequestedPipeline != "override_pipeline" {
		t.Fatalf("binding=%#v, want explicit override_pipeline", binding)
	}

	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.creates) != 1 {
		t.Fatalf("run creates=%d, want 1", len(runStore.creates))
	}
	if got := runStore.creates[0].Parameters["pipeline_override"]; got != "override_pipeline" {
		t.Fatalf("pipeline_override parameter=%v, want override_pipeline", got)
	}
}

// TestHandleEvent_EnforcedModeExcludesProcessorsNotInPipeline_NoExplicitOperations
// reproduces the common ingestion path -- and what gold-run's payload
// actually looks like -- where the event carries no "operation" field at
// all, so resolveProductionPlanFacts must fall back to s.Processors as the
// RequestedProcessors baseline. Before that fallback existed,
// facts.RequestedProcessors stayed empty here, plan.ExcludedByPolicy() was
// therefore always empty, and applyPlanEnforcement was a silent no-op for
// exactly this (the most common) case.
func TestHandleEvent_EnforcedModeExcludesProcessorsNotInPipeline_NoExplicitOperations(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	t.Setenv("DOC_PIPELINE_PLAN_ONLY", "false")
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "legacy_default", LegacyEquivalent: true},
		{Name: "metrics_only", Processors: []string{"extract_metrics"}},
	})

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:                4821,
			ParserName:        "opendata",
			ResultFilename:    "result.json",
			StagingFilename:   inputPath,
			StatusRaw:         "[]",
			RequestedPipeline: "metrics_only",
			InputDocType:      "pdf",
			SourceLanguage:    "en",
		},
	}
	runStore := &fakeRunStore{}
	planStore := &fakePlanStore{}

	var calls []string
	svc := &ControlService{
		InputStore: store,
		RunStore:   runStore,
		PlanStore:  planStore,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &calls},
			fakeProcessor{name: "extract_provisions", calls: &calls},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`"
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	if got, want := calls, []string{"extract_metrics"}; len(got) != len(want) || got[0] != want[0] {
		t.Errorf("processor calls=%v, want %v (extract_provisions must be excluded even with no explicit operations)", got, want)
	}

	planStore.mu.Lock()
	defer planStore.mu.Unlock()
	if len(planStore.creates) != 1 {
		t.Fatalf("plan creates=%d, want 1", len(planStore.creates))
	}
	if got, want := planStore.creates[0].ExcludedByPolicy, []string{"extract_provisions"}; len(got) != len(want) || got[0] != want[0] {
		t.Errorf("plan ExcludedByPolicy=%v, want %v (persisted audit trail must match real exclusion)", got, want)
	}
}

func TestHandleEvent_PlanOnlyModeStillRunsEverythingRequested(t *testing.T) {
	// Same setup as the enforced-mode test above, but without
	// DOC_PIPELINE_PLAN_ONLY=false: proves applyPlanEnforcement is a true
	// no-op in the default (plan-only) mode, preserving legacy behavior.
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	t.Cleanup(func() { SetProductionPipelineRegistry(nil) })
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "legacy_default", LegacyEquivalent: true},
		{Name: "metrics_only", Processors: []string{"extract_metrics"}},
	})

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:                4821,
			ParserName:        "opendata",
			ResultFilename:    "result.json",
			StagingFilename:   inputPath,
			StatusRaw:         "[]",
			RequestedPipeline: "metrics_only",
		},
	}
	runStore := &fakeRunStore{}

	var calls []string
	svc := &ControlService{
		InputStore: store,
		RunStore:   runStore,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &calls},
			fakeProcessor{name: "extract_provisions", calls: &calls},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`",
		"operation":["extract_metrics","extract_provisions"]
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	if len(calls) != 2 {
		t.Errorf("plan-only mode must run everything requested regardless of pipeline: calls=%v", calls)
	}
}

// TestHandleEvent_BlockModeConflictFailsBeforeAnyProcessorRuns proves E3
// Finding 2: a decision-relevant DR7 conditional-binding conflict (block
// mode is hardcoded for binding resolution) must fail handleEvent before
// any processor is dispatched and before a run row is created, raising
// exactly one binding_conflict alarm/event -- not silently continue with
// every processor unfiltered, which was the bug this test guards against.
func TestHandleEvent_BlockModeConflictFailsBeforeAnyProcessorRuns(t *testing.T) {
	t.Cleanup(func() {
		SetProductionPipelineRegistry(nil)
		SetProductionPipelineBindings(nil)
	})
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "legacy_default", LegacyEquivalent: true},
		{Name: "pipeline_a"},
		{Name: "pipeline_b"},
	})
	// Two same-rank conditional bindings that both evaluate true for a
	// pdf/en document but select different pipelines -- DR7's "true
	// conflict blocks" case (see TestPipelineBindingDR7DecisionTable).
	SetProductionPipelineBindings([]PipelineBinding{
		mustLegacyBinding(t, "match-pdf", "pipeline_a", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"}),
		mustLegacyBinding(t, "match-en", "pipeline_b", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchSourceLanguage: "en"}),
	})

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:              4821,
			ParserName:      "opendata",
			ResultFilename:  "result.json",
			StagingFilename: inputPath,
			StatusRaw:       "[]",
			InputDocType:    "pdf",
			SourceLanguage:  "en",
		},
	}
	runStore := &fakeRunStore{}
	alarms := &fakeAlarmWriter{}
	audit := &fakePolicyAuditEventWriter{}

	var calls []string
	svc := &ControlService{
		InputStore:    store,
		RunStore:      runStore,
		RoutingAlarms: alarms,
		PolicyAudit:   audit,
		Processors:    []Processor{fakeProcessor{name: "extract_metrics", calls: &calls}},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`"
	}`))
	if err == nil {
		t.Fatal("handleEvent must return an error for a decision-relevant block-mode conflict")
	}
	if len(calls) != 0 {
		t.Fatalf("processor calls=%v, want zero -- block-mode conflict must fail before any processor runs", calls)
	}
	runStore.mu.Lock()
	created := len(runStore.creates)
	runStore.mu.Unlock()
	if created != 0 {
		t.Fatalf("run creates=%d, want zero -- no run row for a pre-dispatch conflict", created)
	}
	if len(alarms.written) != 1 || alarms.written[0].Kind != RoutingAlarmKindBindingConflict {
		t.Fatalf("alarms=%+v, want exactly one binding_conflict alarm", alarms.written)
	}
	events := audit.snapshot()
	if len(events) != 1 || events[0].Kind != policyaudit.EventBindingConflict {
		t.Fatalf("audit events=%+v, want exactly one binding_conflict event", events)
	}
}

// TestHandleEvent_FallbackModeGateConflictStillExecutes proves the
// fallback-mode counterpart to the block-mode test above: processor-gate
// resolution is always fallback mode today
// (BuildProductionProcessorPlanFromFacts hardcodes
// PipelineBindingOnConflictFallback for gates), so an indeterminate gate
// decision never produces a planErr and never blocks dispatch -- the
// affected processor keeps running (DR7 fallback, not a hard failure) while
// still raising a gate_conflict alarm/event so operators see the
// ambiguity.
func TestHandleEvent_FallbackModeGateConflictStillExecutes(t *testing.T) {
	t.Cleanup(func() { SetProductionPipelineGates(nil) })
	// document.doc_kind is never populated by BuildPipelineBindingFactSet,
	// so this predicate is indeterminate (missing fact) for every document.
	SetProductionPipelineGates([]PipelineGate{
		{
			ID: 1, Name: "needs-doc-kind", Priority: 10, TargetProcessor: "extract_metric_definitions",
			Effect: GateEffectSkip, PredicateChecksum: "sha256:test-gate", Active: true,
			Predicate: semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "narrative"}},
		},
	})

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:              4821,
			ParserName:      "opendata",
			ResultFilename:  "result.json",
			StagingFilename: inputPath,
			StatusRaw:       "[]",
		},
	}
	runStore := &fakeRunStore{}
	alarms := &fakeAlarmWriter{}
	audit := &fakePolicyAuditEventWriter{}

	var calls []string
	svc := &ControlService{
		InputStore:    store,
		RunStore:      runStore,
		RoutingAlarms: alarms,
		PolicyAudit:   audit,
		Processors:    []Processor{fakeProcessor{name: "extract_metric_definitions", calls: &calls}},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`"
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v (fallback-mode conflicts must not block processing)", err)
	}
	if len(calls) != 1 {
		t.Fatalf("processor calls=%v, want extract_metric_definitions to still run under fallback-mode indeterminacy", calls)
	}
	foundAlarm := false
	for _, alarm := range alarms.written {
		if alarm.Kind == RoutingAlarmKindGateConflict {
			foundAlarm = true
		}
	}
	if !foundAlarm {
		t.Fatalf("alarms=%+v, want a gate_conflict alarm", alarms.written)
	}
	foundEvent := false
	for _, event := range audit.snapshot() {
		if event.Kind == policyaudit.EventGateConflict {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Fatalf("audit events=%+v, want a gate_conflict event", audit.snapshot())
	}
}

// TestHandleEvent_UnclearedSuppressiveBindingRaisesFallbackEvent proves E3
// Finding 1's "fallback" event category end-to-end: an enforced-mode
// conditional binding that would suppress a processor, with no D2 clearance
// configured, falls back to the baseline pipeline (nothing is suppressed)
// and raises exactly one fallback_warning alarm plus one EventFallbackApplied
// policyaudit event -- complementing the pure-decision proof already covered
// by TestFinalizeRoutingPlan_ConditionalBindingFallsBackToBaselineWhenUncleared.
func TestHandleEvent_UnclearedSuppressiveBindingRaisesFallbackEvent(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	t.Setenv("DOC_PIPELINE_PLAN_ONLY", "false")
	t.Cleanup(func() {
		SetProductionPipelineRegistry(nil)
		SetProductionPipelineBindings(nil)
	})
	SetProductionPipelineRegistry([]ProductionPipelineSpec{
		{Name: "legacy_default", LegacyEquivalent: true},
		{Name: "narrow", Processors: []string{"extract_metrics"}},
	})
	SetProductionPipelineBindings([]PipelineBinding{
		mustLegacyBinding(t, "pdf-narrow", "narrow", 10, PipelineBindingScopeKnowledgeStore, ProductionPipelineRule{MatchInputDocType: "pdf"}),
	})

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:              4821,
			ParserName:      "opendata",
			ResultFilename:  "result.json",
			StagingFilename: inputPath,
			StatusRaw:       "[]",
			InputDocType:    "pdf",
		},
	}
	runStore := &fakeRunStore{}
	alarms := &fakeAlarmWriter{}
	audit := &fakePolicyAuditEventWriter{}

	var calls []string
	svc := &ControlService{
		InputStore:    store,
		RunStore:      runStore,
		RoutingAlarms: alarms,
		PolicyAudit:   audit,
		// RoutingClearances is intentionally nil: no D2 store means every
		// suppressive decision is uncleared and must fall back to shadow.
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &calls},
			fakeProcessor{name: "extract_provisions", calls: &calls},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`"
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("processor calls=%v, want both processors to run (uncleared suppression falls back to the unrestricted baseline pipeline)", calls)
	}
	foundAlarm := false
	for _, alarm := range alarms.written {
		if alarm.Kind == RoutingAlarmKindFallbackWarning {
			foundAlarm = true
		}
	}
	if !foundAlarm {
		t.Fatalf("alarms=%+v, want a fallback_warning alarm", alarms.written)
	}
	foundEvent := false
	for _, event := range audit.snapshot() {
		if event.Kind == policyaudit.EventFallbackApplied {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Fatalf("audit events=%+v, want a fallback_applied event", audit.snapshot())
	}
}

func TestHandleEvent_ClosesRunAsFailedWhenProcessorFails(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:              4821,
			ParserName:      "opendata",
			ResultFilename:  "result.json",
			StagingFilename: inputPath,
			StatusRaw:       "[]",
		},
	}
	runStore := &fakeRunStore{}
	svc := &ControlService{
		InputStore: store,
		RunStore:   runStore,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", retErr: errors.New("boom")},
		},
	}

	_ = svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`",
		"operation":["extract_metrics"]
	}`))

	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.closes) != 1 {
		t.Fatalf("closes=%d, want 1", len(runStore.closes))
	}
	if runStore.closes[0].status != "failed" {
		t.Errorf("close status=%q, want failed", runStore.closes[0].status)
	}
}

func TestHandleEvent_SkippedEventCreatesNoRun(t *testing.T) {
	runStore := &fakeRunStore{}
	svc := &ControlService{
		RunStore: runStore,
	}

	// Empty payload / missing record_id triggers an early parse failure,
	// which returns before any run is created.
	_ = svc.handleEvent(context.Background(), []byte(`{}`))

	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.creates) != 0 {
		t.Fatalf("creates=%d, want 0 for a skipped/failed-to-parse event", len(runStore.creates))
	}
}
