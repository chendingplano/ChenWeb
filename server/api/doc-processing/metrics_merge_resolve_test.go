package docprocessing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
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
	if extractor.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2 (primary + fallback)", extractor.structuredCalledCount)
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

func TestMergeResolvePreservesStructuredValuePayload(t *testing.T) {
	structure := structuredPoints("alternatives")
	for _, omit := range []bool{false, true} {
		winner := map[string]any{"metric_id": "173_mtc_1", "metric_value": "10、20、30", "absorbed_metric_ids": []any{"173_mtc_9"}}
		if !omit {
			winner["value_structure"] = structure
		}
		extractor := &fakeJSONExtractor{out: map[string]any{"winning_metrics": []any{winner}}}
		p := newTestMergeResolveProcessor(t, extractor)
		group := []map[string]any{
			{"metric_id": "173_mtc_1", "metric_value": "10、20、30", "value_structure": structure},
			{"metric_id": "173_mtc_9", "metric_value": "10、20、30", "value_structure": structure},
		}
		winners, sent, _, err := p.resolveMergeAmbiguities(context.Background(), 173, group)
		if err != nil {
			t.Fatal(err)
		}
		if len(winners) != 1 || winners[0]["value_structure"] == nil {
			t.Fatalf("winner lost structure, omitted=%v: %v", omit, winners)
		}
		if len(sent) != 2 || sent[0]["value_structure"] == nil || sent[1]["value_structure"] == nil {
			t.Fatalf("LLM input lost structure: %v", sent)
		}
	}
}

func TestMergeResolveRejectsValueRelationshipCorruption(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		winnerStructure, absorbedStructure any
		display                            string
	}{
		{"explicit_null_erasure", nil, structuredPoints("alternatives"), "10、20、30"},
		{"changed_kind", structuredPoints("required_points"), structuredPoints("alternatives"), "10、20、30"},
		{"absorbed_kind_differs", structuredPoints("alternatives"), structuredPoints("required_points"), "10、20、30"},
		{"absorbed_scalar", structuredPoints("alternatives"), nil, "10、20、30"},
		{"first_member_display", structuredPoints("alternatives"), structuredPoints("alternatives"), "10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			winner := map[string]any{"metric_id": "173_mtc_1", "metric_value": tc.display, "value_structure": tc.winnerStructure, "absorbed_metric_ids": []any{"173_mtc_9"}}
			extractor := &fakeJSONExtractor{out: map[string]any{"winning_metrics": []any{winner}}}
			p := newTestMergeResolveProcessor(t, extractor)
			group := []map[string]any{
				{"metric_id": "173_mtc_1", "metric_value": "10、20、30", "value_structure": structuredPoints("alternatives")},
				{"metric_id": "173_mtc_9", "metric_value": "10、20、30", "value_structure": tc.absorbedStructure},
			}
			if winners, _, _, err := p.resolveMergeAmbiguities(context.Background(), 173, group); err == nil {
				t.Fatalf("corrupted merge accepted: %v", winners)
			}
		})
	}
}

func TestMergeResolvePreservesCompleteExpressions(t *testing.T) {
	for _, kind := range []string{"expression", "unresolved"} {
		for _, tc := range []struct {
			name, winnerValue, absorbedValue string
			wantError                        bool
		}{
			{"same", "70+age×2", "70+age×2", false},
			{"changed_winner", "70", "70+age×2", true},
			{"different_absorbed", "70+age×2", "90+age×2", true},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				structure := map[string]any{"kind": kind}
				winner := map[string]any{"metric_id": "173_mtc_1", "metric_value": tc.winnerValue, "value_structure": structure, "absorbed_metric_ids": []any{"173_mtc_9"}}
				extractor := &fakeJSONExtractor{out: map[string]any{"winning_metrics": []any{winner}}}
				p := newTestMergeResolveProcessor(t, extractor)
				group := []map[string]any{
					{"metric_id": "173_mtc_1", "metric_value": "70+age×2", "value_structure": structure},
					{"metric_id": "173_mtc_9", "metric_value": tc.absorbedValue, "value_structure": structure},
				}
				_, _, _, err := p.resolveMergeAmbiguities(context.Background(), 173, group)
				if (err != nil) != tc.wantError {
					t.Fatalf("err=%v, wantError=%v", err, tc.wantError)
				}
			})
		}
	}
}

// A partition that misses an input metric used to go straight to the fallback
// model. With a structured client the primary is retried first, with the
// partition error in the retry input (ChenWeb#17).
func TestResolveMergeAmbiguities_RetriesPrimaryOnInvalidPartition(t *testing.T) {
	bad := `{"winning_metrics":[{"metric_id":"173_mtc_1","absorbed_metric_ids":[],"metric_name":"Latency"}]}`
	good := `{"winning_metrics":[{"metric_id":"173_mtc_1","absorbed_metric_ids":["173_mtc_9"],"metric_name":"Latency"}]}`
	var models []string
	var retryInput string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &req)
		models = append(models, req.Model)
		content := good
		if len(models) == 1 {
			content = bad
		} else {
			retryInput = string(body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer srv.Close()
	p := &MetricsProcessor{
		Extractor:                     &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", HTTPClient: srv.Client()},
		Logger:                        loggerutil.CreateDefaultLogger("TEST_MERGE_RESOLVE"),
		MergeResolvePromptText:        "resolve merge ambiguities",
		MergeResolvePromptRef:         "prompt-merge-resolve-metrics-v1.md",
		MergeResolveModelName:         "test-merge-model",
		FallbackMergeResolveModelName: "fallback-model",
	}
	group := []map[string]any{
		{"metric_id": "173_mtc_1", "_merge_source": "existing", "metric_name": "Latency"},
		{"metric_id": "173_mtc_9", "_merge_source": "new", "metric_name": "Latency"},
	}
	winners, _, modelUsed, err := p.resolveMergeAmbiguities(context.Background(), 173, group)
	if err != nil {
		t.Fatalf("resolveMergeAmbiguities: %v", err)
	}
	if modelUsed != "test-merge-model" || !reflect.DeepEqual(models, []string{"test-merge-model", "test-merge-model"}) {
		t.Fatalf("model=%q requests=%v; want the primary retried and no fallback call", modelUsed, models)
	}
	if !strings.Contains(retryInput, "173_mtc_9") {
		t.Fatalf("retry input does not name the missing metric: %s", retryInput)
	}
	if len(winners) != 1 {
		t.Fatalf("winners=%+v", winners)
	}
}
