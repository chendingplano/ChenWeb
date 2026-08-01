package docprocessing

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/candidates"
)

type recordingOntologyCandidateSink struct{ got []candidates.Candidate }

type recordingStructuralSink struct {
	got []assertions.DecisionCandidate
}

func (s *recordingStructuralSink) Propose(_ context.Context, c assertions.DecisionCandidate) (assertions.DecisionCandidate, error) {
	s.got = append(s.got, c)
	return c, nil
}

func (s *recordingOntologyCandidateSink) CreateCandidate(_ context.Context, c candidates.Candidate) (candidates.Candidate, error) {
	s.got = append(s.got, c)
	return c, nil
}

func TestBuildMetricDefinitionCandidatePreservesProvenance(t *testing.T) {
	candidate, err := buildMetricDefinitionCandidate(42, metricDefinitionMention{
		CanonicalName: "Air flow rate",
		Aliases:       []string{"flow rate"},
		Definition:    "The volume of air delivered per unit time.",
		ValueType:     "number",
		RangeType:     "minimum",
		Confidence:    0.91,
		LineNumbers:   []int{17, 18},
	})
	if err != nil {
		t.Fatalf("buildMetricDefinitionCandidate: %v", err)
	}
	if candidate.CandidateKind != "term" || candidate.ProposedModuleID != "measurement" || candidate.SourceRef != "input_record:42" {
		t.Fatalf("candidate metadata = %#v", candidate)
	}
	if got := string(candidate.SourceLineSpans); got != `[17,18]` {
		t.Fatalf("source spans = %s", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(candidate.ProposedPayload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["term_id"] != "measurement:air_flow_rate" || payload["term_kind"] != "metric_definition" {
		t.Fatalf("payload identity = %#v", payload)
	}
	if got := payload["aliases"].([]any); len(got) != 1 || got[0] != "flow rate" {
		t.Fatalf("aliases = %#v", got)
	}
}

func TestHarvestMetricDefinitionsCreatesReviewCandidateForDefinedMetric(t *testing.T) {
	sink := &recordingOntologyCandidateSink{}
	err := harvestMetricDefinitions(context.Background(), 42, []map[string]any{{
		"metric_name":           "Air flow rate",
		"formula_or_definition": "The volume of air delivered per unit time.",
		"value_data_type":       "number",
		"value_range_type":      "minimum",
		"source_line_spans":     []string{"17", "18"},
	}}, sink)
	if err != nil {
		t.Fatalf("harvestMetricDefinitions: %v", err)
	}
	if len(sink.got) != 1 {
		t.Fatalf("created %d candidates, want 1", len(sink.got))
	}
	if got := string(sink.got[0].SourceLineSpans); got != `[17,18]` {
		t.Fatalf("source spans = %s", got)
	}
}

func TestMetricsProcessorHarvestsDefinitionCandidatesAfterMetricExtraction(t *testing.T) {
	sink := &recordingOntologyCandidateSink{}
	p := &MetricsProcessor{batchRecordID: 42, CandidateSink: sink}
	if err := p.harvestMetricDefinitionCandidates(context.Background(), []map[string]any{{
		"metric_name":           "Air flow rate",
		"formula_or_definition": "The volume of air delivered per unit time.",
		"source_line_spans":     []string{"17"},
	}}); err != nil {
		t.Fatalf("harvestMetricDefinitionCandidates: %v", err)
	}
	if len(sink.got) != 1 || sink.got[0].SourceRef != "input_record:42" {
		t.Fatalf("candidate sink = %#v", sink.got)
	}
}

func TestBuildTestMethodCandidatesProposesProcedureAndMetricLink(t *testing.T) {
	got, err := buildTestMethodCandidates(42, testMethodMention{
		ProcedureName: "Constant-flow test",
		Definition:    "Measure output at a controlled constant flow.",
		MetricNames:   []string{"Air flow rate"},
		LineNumbers:   []int{31},
	})
	if err != nil {
		t.Fatalf("buildTestMethodCandidates: %v", err)
	}
	if len(got) != 2 || got[0].CandidateKind != "term" || got[1].CandidateKind != "axiom" {
		t.Fatalf("candidates = %#v", got)
	}
	var link map[string]any
	if err := json.Unmarshal(got[1].ProposedPayload, &link); err != nil {
		t.Fatalf("unmarshal link: %v", err)
	}
	if link["predicate_term_id"] != "mea:measured_by" || link["subject_term_id"] != "measurement:air_flow_rate" {
		t.Fatalf("link = %#v", link)
	}
}

func TestParseTestMethodMentionsUsesReturnedLineSpans(t *testing.T) {
	got := parseTestMethodMentions(map[string]any{"procedures": []any{map[string]any{
		"procedure_name": "Constant-flow test", "definition": "Measure output.",
		"metric_names": []any{"Air flow rate"}, "source_line_spans": []any{"31", "32"},
	}}})
	if len(got) != 1 || got[0].ProcedureName != "Constant-flow test" || len(got[0].LineNumbers) != 2 {
		t.Fatalf("mentions = %#v", got)
	}
}

func TestBuildProductStructureCandidatePreservesExplicitRelationAndEvidence(t *testing.T) {
	candidate, err := buildProductStructureCandidate(42, productStructureMention{
		SubjectObjectID: "obj:child", ObjectObjectID: "obj:parent", Relation: "part_of", LineNumbers: []int{51},
	})
	if err != nil {
		t.Fatalf("buildProductStructureCandidate: %v", err)
	}
	if candidate.CandidateKind != "assertion" || candidate.Method != "structural_candidate" || candidate.LogicalIdentityKey != "product_structure:42:obj:child:part_of:obj:parent" {
		t.Fatalf("candidate=%#v", candidate)
	}
}

func TestHarvestProductStructureFromRelationsUsesReconciledEndpoints(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT r.relation_id")).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"relation_id", "predicate", "line_spans", "subject_object_id", "object_object_id"}).AddRow("42_rel_1", "part_of", []byte(`[51]`), "obj:child", "obj:parent"))
	sink := &recordingStructuralSink{}
	if err := HarvestProductStructureFromRelations(context.Background(), db, 42, sink); err != nil {
		t.Fatalf("HarvestProductStructureFromRelations: %v", err)
	}
	if len(sink.got) != 1 || sink.got[0].LogicalIdentityKey != "product_structure:42:obj:child:part_of:obj:parent" {
		t.Fatalf("candidates=%#v", sink.got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestMethodsProcessorPersistsValidatedChunkOutput(t *testing.T) {
	sink := &recordingOntologyCandidateSink{}
	p := &TestMethodsProcessor{Extractor: &fakeJSONExtractor{out: map[string]any{"procedures": []any{map[string]any{
		"procedure_name": "Constant-flow test", "metric_names": []any{"Air flow rate"}, "source_line_spans": []any{"61"},
	}}}}, CandidateSink: sink, ModelName: "test-model"}
	chunks := []Chunk{{SeqNo: 1, Lines: []MarkedLine{{Line: Line{LineNo: 61, Content: "The constant-flow test measures air flow rate."}}}}}
	if err := p.InitChunkBatch(context.Background(), 42, chunks, "doc"); err != nil {
		t.Fatalf("InitChunkBatch: %v", err)
	}
	if err := p.ProcessChunk(context.Background(), 0); err != nil {
		t.Fatalf("ProcessChunk: %v", err)
	}
	if err := p.FinalizeChunkBatch(context.Background()); err != nil {
		t.Fatalf("FinalizeChunkBatch: %v", err)
	}
	if len(sink.got) != 2 {
		t.Fatalf("persisted %d candidates, want procedure and link", len(sink.got))
	}
}

func TestMetricDefinitionsProcessorPersistsValidatedChunkOutput(t *testing.T) {
	sink := &recordingOntologyCandidateSink{}
	p := &MetricDefinitionsProcessor{Extractor: &fakeJSONExtractor{out: map[string]any{"metric_definitions": []any{map[string]any{"canonical_name": "Air flow rate", "definition": "Volume per time", "source_line_spans": []any{"71"}}}}}, CandidateSink: sink, ModelName: "test-model"}
	chunks := []Chunk{{SeqNo: 1, Lines: []MarkedLine{{Line: Line{LineNo: 71, Content: "Air flow rate is volume per time."}}}}}
	if err := p.InitChunkBatch(context.Background(), 42, chunks, "doc"); err != nil {
		t.Fatalf("InitChunkBatch: %v", err)
	}
	if err := p.ProcessChunk(context.Background(), 0); err != nil {
		t.Fatalf("ProcessChunk: %v", err)
	}
	if err := p.FinalizeChunkBatch(context.Background()); err != nil {
		t.Fatalf("FinalizeChunkBatch: %v", err)
	}
	if len(sink.got) != 1 || sink.got[0].CandidateKind != "term" {
		t.Fatalf("candidates=%#v", sink.got)
	}
}
