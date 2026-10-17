package docprocessing

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrichMetricCandidatesRecoversOmittedCandidates(t *testing.T) {
	row := func(id, name string) map[string]any {
		return map[string]any{"candidate_id": id, "metric_name": name, "metric_value": "10", "source_line_spans": []any{"2"}}
	}
	ext := &fakeJSONExtractor{outs: []map[string]any{
		{"metrics": []any{row("7_56", "Range")}},
		{"metrics": []any{row("7_57", "Resolution")}},
		{"metrics": []any{}, "dropped_candidates": []any{map[string]any{"candidate_id": "7_58", "reason": "duplicate_of:7_57"}}},
	}}
	p := makeTestMetricsProcessor(ext, 1)
	p.MetricEnrichGroupSize = 12
	candidates := []metricCandidate{{CandidateID: "7_56", ChunkIndex: 7}, {CandidateID: "7_57", ChunkIndex: 7}, {CandidateID: "7_58", ChunkIndex: 7}}
	metrics, _, _, err := p.enrichMetricCandidates(context.Background(), 371, candidates, metricsBlocksToChunks([]Block{makeMetricsBlock(7, 2)}), "")
	if err != nil || len(metrics) != 2 || ext.structuredCalledCount != 3 {
		t.Fatalf("metrics=%v err=%v calls=%d, want two metrics recovered in three calls", metrics, err, ext.structuredCalledCount)
	}
}

func TestMetricCoverageRecoveryKeepsSiblingEvidenceTogether(t *testing.T) {
	ext := &fakeJSONExtractor{outs: []map[string]any{
		{"metrics": []any{}},
		{"metrics": []any{}, "dropped_candidates": []any{
			map[string]any{"candidate_id": "7_57", "reason": "not_a_metric"},
			map[string]any{"candidate_id": "7_58", "reason": "not_a_metric"},
		}},
	}}
	p := makeTestMetricsProcessor(ext, 1)
	mentions := []map[string]any{{"evidence_quote": "same table row", "source_line_spans": []string{"2"}}}
	candidates := []metricCandidate{
		{CandidateID: "7_57", ChunkIndex: 7, SupportingMentions: mentions},
		{CandidateID: "7_58", ChunkIndex: 7, SupportingMentions: mentions},
	}
	_, _, _, err := p.enrichMetricCandidates(context.Background(), 371, candidates, metricsBlocksToChunks([]Block{makeMetricsBlock(7, 2)}), "")
	if err != nil || ext.structuredCalledCount != 2 {
		t.Fatalf("err=%v calls=%d, want one recovery call for both siblings", err, ext.structuredCalledCount)
	}
}

func TestFinalizeChunkBatchDoesNotReplaceMetricsWithUnresolvedCoverage(t *testing.T) {
	ext := &fakeJSONExtractor{out: map[string]any{"metrics": []any{map[string]any{
		"candidate_id": "7_56", "metric_name": "Range", "metric_value": "10", "source_line_spans": []any{"2"},
	}}}}
	p := makeTestMetricsProcessor(ext, 1)
	p.MetricEnrichGroupSize = 12
	store := &fakeMetricsStore{}
	p.Store = store
	p.batchRecordID, p.batchForceClear = 371, true
	p.batchChunks = metricsBlocksToChunks([]Block{makeMetricsBlock(7, 2)})
	for _, id := range []string{"7_56", "7_57"} {
		p.batchMentions = append(p.batchMentions, metricCandidateMention{
			CandidateID: id, ChunkIndex: 7, HasNormalEvidence: true,
		})
	}
	err := p.FinalizeChunkBatch(context.Background())
	if !errors.Is(err, errMetricCandidateCoverage) {
		t.Fatalf("err=%v, want coverage failure", err)
	}
	if store.deleteCalled != 0 || store.saveCalled != 0 || store.upsertCalled != 0 {
		t.Fatalf("incomplete extraction wrote to store: %+v", store)
	}
}

func TestMetricCoverageRecoveryPreservesRetryError(t *testing.T) {
	ext := &metricsSeqErrExtractor{
		outs: []map[string]any{{"metrics": []any{}}, nil},
		errs: []error{nil, errors.New("recovery timed out")},
	}
	p := makeTestMetricsProcessor(ext, 1)
	_, _, _, err := p.enrichMetricCandidates(context.Background(), 371,
		[]metricCandidate{{CandidateID: "7_57", ChunkIndex: 7}},
		metricsBlocksToChunks([]Block{makeMetricsBlock(7, 2)}), "")
	if !errors.Is(err, errMetricCandidateCoverage) || !strings.Contains(err.Error(), "recovery timed out") {
		t.Fatalf("err=%v, want missing candidate and retry error", err)
	}
}

func TestHandleEventPreservesExistingMetricsOnUnresolvedCoverage(t *testing.T) {
	tmp := t.TempDir()
	writeTestArtifacts(t, tmp, 371, filepath.Join(tmp, "ocr_rslt_371.pdf"), "opendata",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tRange 10; resolution 1", "lines: [2]\n")
	partial := map[string]any{"metrics": []any{map[string]any{
		"candidate_id": "c1", "metric_name": "Range", "metric_value": "10", "source_line_spans": []any{"2"},
	}}}
	ext := &fakeJSONExtractor{outs: []map[string]any{
		{"candidates": []any{
			map[string]any{"candidate_id": "c1", "metric_name_hint": "Range", "source_line_spans": []any{"2"}},
			map[string]any{"candidate_id": "c2", "metric_name_hint": "Resolution", "source_line_spans": []any{"2"}},
		}}, partial, partial,
	}}
	p := makeTestMetricsProcessor(ext, 1)
	p.MetricEnrichGroupSize, p.ChunkDir = 12, tmp
	store := &fakeMetricsStore{}
	p.Store = store
	p.InputStore = &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID: 371, ParserName: "opendata", ResultFilename: filepath.Join(tmp, "ocr_rslt_371.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_371.pdf"), StatusRaw: "[]",
	}}
	err := p.HandleEvent(context.Background(), []byte(`{"record_id":"371","force":true}`))
	if !errors.Is(err, errMetricCandidateCoverage) {
		t.Fatalf("err=%v, want coverage failure", err)
	}
	if store.deleteCalled != 0 || store.saveCalled != 0 {
		t.Fatalf("failed forced extraction replaced metrics: deletes=%d saves=%d", store.deleteCalled, store.saveCalled)
	}
}

func TestEnrichMetricCandidatesReportsUnresolvedCoverageWithoutDiscardingRows(t *testing.T) {
	ext := &fakeJSONExtractor{out: map[string]any{"metrics": []any{map[string]any{
		"candidate_id": "7_56", "metric_name": "Range", "metric_value": "10", "source_line_spans": []any{"2"},
	}}}}
	p := makeTestMetricsProcessor(ext, 1)
	p.MetricEnrichGroupSize = 12
	candidates := []metricCandidate{{CandidateID: "7_56", ChunkIndex: 7}, {CandidateID: "7_57", ChunkIndex: 7}}
	metrics, _, _, err := p.enrichMetricCandidates(context.Background(), 371, candidates, metricsBlocksToChunks([]Block{makeMetricsBlock(7, 2)}), "")
	if err == nil || !strings.Contains(err.Error(), "7_57") || len(metrics) != 1 || ext.structuredCalledCount != 2 {
		t.Fatalf("metrics=%v err=%v calls=%d, want bounded recovery and explicit missing ID", metrics, err, ext.structuredCalledCount)
	}
}

// Record 364 run 233: one enrich batch returned every row without metric_value although each
// row's range type said it had a number, and all nine true metrics were set aside as no_value.
func TestEnrichMetricCandidatesRetriesRowsThatLostTheirValue(t *testing.T) {
	row := func(value any) map[string]any {
		m := map[string]any{"candidate_id": "12_4", "metric_name": "腺苷剂量", "unit": "mg", "value_range_type": "exact",
			"subject": "腺苷", "threshold_or_target": "6mg静脉快速推注", "source_line_spans": []any{"2"}}
		if value != nil {
			m["metric_value"] = value
		}
		return m
	}
	ext := &fakeJSONExtractor{outs: []map[string]any{
		{"metrics": []any{row(nil)}},
		{"metrics": []any{row("6")}},
	}}
	p := makeTestMetricsProcessor(ext, 1)
	p.MetricEnrichGroupSize = 12
	metrics, _, dropped, err := p.enrichMetricCandidates(context.Background(), 364,
		[]metricCandidate{{CandidateID: "12_4", ChunkIndex: 7}}, metricsBlocksToChunks([]Block{makeMetricsBlock(7, 2)}), "")
	if err != nil || ext.structuredCalledCount != 2 || len(metrics) != 1 || asString(metrics[0]["metric_value"]) != "6" {
		t.Fatalf("metrics=%v dropped=%v err=%v calls=%d, want the value recovered by one retry", metrics, dropped, err, ext.structuredCalledCount)
	}
}

func TestEnrichMetricCandidatesKeepsValuelessRowWhenRetryFails(t *testing.T) {
	row := map[string]any{"candidate_id": "12_4", "metric_name": "腺苷剂量", "value_range_type": "exact", "source_line_spans": []any{"2"}}
	ext := &fakeJSONExtractor{outs: []map[string]any{{"metrics": []any{row}}, {"metrics": []any{row}}}}
	p := makeTestMetricsProcessor(ext, 1)
	p.MetricEnrichGroupSize = 12
	_, _, dropped, err := p.enrichMetricCandidates(context.Background(), 364,
		[]metricCandidate{{CandidateID: "12_4", ChunkIndex: 7}}, metricsBlocksToChunks([]Block{makeMetricsBlock(7, 2)}), "")
	if err != nil || len(dropped) != 1 || dropped[0].Stage != metricDropStageNoValue {
		t.Fatalf("dropped=%v err=%v, want the row set aside as no_value, not a coverage error", dropped, err)
	}
}
