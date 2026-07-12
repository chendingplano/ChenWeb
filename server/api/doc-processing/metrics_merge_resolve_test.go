package docprocessing

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestResolveMergeAmbiguities_WellFormedResponse(t *testing.T) {
	extractor := &fakeJSONExtractor{outs: []map[string]any{
		{
			"winning_metrics": []any{
				map[string]any{
					"metric_id":           "173_mtc_1",
					"absorbed_metric_ids": []any{"173_mtc_9"},
					"metric_name":         "Latency", "metric_subject": "gw", "metric_unit": "ms",
					"metric_value": "250", "source_line_spans": []any{"2"},
				},
			},
		},
	}}
	p := newTestMergeResolveProcessor(t, extractor)
	group := []map[string]any{
		{"metric_id": "173_mtc_1", "_merge_source": "existing", "metric_name": "Latency"},
		{"metric_id": "173_mtc_9", "_merge_source": "new", "metric_name": "Latency"},
	}
	winners, _, _, err := p.resolveMergeAmbiguities(context.Background(), 173, group)
	if err != nil {
		t.Fatalf("resolveMergeAmbiguities: %v", err)
	}
	if len(winners) != 1 || winners[0]["metric_id"] != "173_mtc_1" {
		t.Fatalf("winners=%+v", winners)
	}
}

func TestResolveMergeAmbiguities_MissingMetricIDFailsAndUsesFallback(t *testing.T) {
	extractor := &fakeJSONExtractor{outs: []map[string]any{
		// Primary call: missing "173_mtc_9" from the output entirely -> invalid.
		{"winning_metrics": []any{
			map[string]any{"metric_id": "173_mtc_1", "absorbed_metric_ids": []any{}, "metric_name": "Latency"},
		}},
		// Fallback call: valid.
		{"winning_metrics": []any{
			map[string]any{"metric_id": "173_mtc_1", "absorbed_metric_ids": []any{"173_mtc_9"}, "metric_name": "Latency"},
		}},
	}}
	p := newTestMergeResolveProcessor(t, extractor)
	p.FallbackMergeResolveModelName = "fallback-model"
	group := []map[string]any{
		{"metric_id": "173_mtc_1", "_merge_source": "existing", "metric_name": "Latency"},
		{"metric_id": "173_mtc_9", "_merge_source": "new", "metric_name": "Latency"},
	}
	winners, _, modelUsed, err := p.resolveMergeAmbiguities(context.Background(), 173, group)
	if err != nil {
		t.Fatalf("resolveMergeAmbiguities: %v", err)
	}
	if len(winners) != 1 {
		t.Fatalf("winners=%+v, want 1 (from fallback)", winners)
	}
	if modelUsed != "fallback-model" {
		t.Fatalf("modelUsed=%q, want fallback-model", modelUsed)
	}
	if extractor.calledCount != 2 {
		t.Fatalf("calledCount=%d, want 2 (primary + fallback)", extractor.calledCount)
	}
	if len(extractor.modelNames) != 2 || extractor.modelNames[0] != "test-merge-model" || extractor.modelNames[1] != "fallback-model" {
		t.Fatalf("modelNames=%v, want [test-merge-model fallback-model]", extractor.modelNames)
	}
}

// newTestMergeResolveProcessor builds a minimal *MetricsProcessor with just
// enough wiring for resolveMergeAmbiguities, reusing the package's existing
// fakeJSONExtractor test double.
func newTestMergeResolveProcessor(t *testing.T, extractor *fakeJSONExtractor) *MetricsProcessor {
	t.Helper()
	return &MetricsProcessor{
		Extractor:              extractor,
		Logger:                 loggerutil.CreateDefaultLogger("TEST_MERGE_RESOLVE"),
		MergeResolvePromptText: "resolve merge ambiguities",
		MergeResolvePromptRef:  "prompt-merge-resolve-metrics-v1.md",
		MergeResolveModelName:  "test-merge-model",
	}
}
