package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestMetricsProcessor_ExtractMetricPayloadUsesStructuredContractWhenAvailable(t *testing.T) {
	extractor := &fakeJSONExtractor{
		out: map[string]any{
			"language": "en",
			"metrics": []any{
				map[string]any{
					"metric_name": "Latency",
				},
			},
		},
	}

	p := NewMetricsProcessor(nil, nil, extractor, nil)
	p.PromptRef = "prompt-extract-metrics-v1.md"

	payload, err := p.extractMetricPayload(context.Background(), "input text", "prompt text", "metrics-model", structureModelConfig{}, "extract_metric_candidates", "MID-26052908")
	if err != nil {
		t.Fatalf("extractMetricPayload: %v", err)
	}
	if extractor.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if len(extractor.contractNames) != 1 || extractor.contractNames[0] != "chenweb_metrics_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_metrics_extraction]", extractor.contractNames)
	}
	metrics, ok := payload["metrics"].([]any)
	if !ok || len(metrics) != 1 {
		t.Fatalf("metrics=%#v", payload["metrics"])
	}
}

type fakeMetricsStore struct {
	exists            bool
	existsErr         error
	saveErr           error
	deleteErr         error
	deleteCalled      int
	saveCalled        int
	lastSave          SaveMetricsRequest
	metricsExistCalls int
	existingMetrics   []map[string]any
	getExistingErr    error
	upsertErr         error
	upsertCalled      int
	lastUpsert        SaveMetricsRequest
}

func (f *fakeMetricsStore) MetricsExist(_ context.Context, _ int64) (bool, error) {
	f.metricsExistCalls++
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists, nil
}

func (f *fakeMetricsStore) DeleteMetricsByInputRecordID(_ context.Context, _ int64) (int64, error) {
	f.deleteCalled++
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	return 0, nil
}

func (f *fakeMetricsStore) SaveMetrics(_ context.Context, req SaveMetricsRequest) (int64, error) {
	f.saveCalled++
	f.lastSave = req
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	return int64(len(req.Metrics)), nil
}

func (f *fakeMetricsStore) GetMetricsByInputRecordID(_ context.Context, _ int64) ([]map[string]any, error) {
	if f.getExistingErr != nil {
		return nil, f.getExistingErr
	}
	return f.existingMetrics, nil
}

func (f *fakeMetricsStore) UpsertMetrics(_ context.Context, req SaveMetricsRequest) (int64, error) {
	f.upsertCalled++
	f.lastUpsert = req
	if f.upsertErr != nil {
		return 0, f.upsertErr
	}
	return int64(len(req.Metrics)), nil
}

type fakeArtifactObjectsStore struct {
	called       int
	recordID     int64
	artifactType string
	objects      []ArtifactObject
	err          error
}

func (f *fakeArtifactObjectsStore) ReplaceObjectsForRecord(_ context.Context, recordID int64, artifactType string, objects []ArtifactObject) error {
	f.called++
	f.recordID = recordID
	f.artifactType = artifactType
	f.objects = append([]ArtifactObject(nil), objects...)
	return f.err
}

// TestMetricsProcessor_ExtractsFromChunksArtifact verifies that HandleEvent loads
// chunks from the persisted .chunks file and passes block-format lines to the LLM,
// preserving overlap flags.
func TestMetricsProcessor_ExtractsFromChunksArtifact(t *testing.T) {
	tmp := t.TempDir()
	stagingFilename := filepath.Join(tmp, "ocr_rslt_2005.pdf")
	resultFilename := filepath.Join(tmp, "ocr_rslt_2005.json")

	writeTestArtifacts(t, tmp, 2005, stagingFilename, "opendata",
		strings.Join([]string{
			"1\t1\theading\tTestFont\t12\t[0,0,1,1]\tIntro",
			"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLatency must be <= 200ms",
		}, "\n"),
		"overlap: [1]\nlines: [2]\n",
	)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              2005,
		ParserName:      "opendata",
		ResultFilename:  resultFilename,
		StagingFilename: stagingFilename,
		StatusRaw:       "[]",
	}}
	metricsStore := &fakeMetricsStore{}
	objectStore := &fakeArtifactObjectsStore{}
	extractor := &fakeJSONExtractor{outs: []map[string]any{
		{
			"language": "en",
			"candidates": []any{
				map[string]any{
					"metric_name_hint":  "Latency",
					"subject_hint":      "service latency",
					"evidence_quote":    "Latency must be <= 200ms",
					"source_line_spans": []any{float64(2)},
					"unit_hint":         "ms",
					"value_hint":        "200",
					"confidence":        0.9,
				},
			},
		},
		{
			"language": "en",
			"metrics": []any{
				map[string]any{
					"metric_name":         "Latency",
					"source_line_spans":   []any{float64(2)},
					"subject":             "service latency",
					"desc":                "max latency",
					"context":             "SLA",
					"keywords":            []any{"latency"},
					"location_type":       "sentence",
					"unit":                "ms",
					"threshold_or_target": "<=200",
					"objects": []any{map[string]any{
						"object_name":       "service gateway",
						"object_role":       "measured_object",
						"source_line_spans": []any{float64(2)},
					}},
				},
			},
			"uncertain_metrics": []any{},
		},
	}}

	p := NewMetricsProcessor(inputStore, metricsStore, extractor, nil)
	p.ChunkDir = tmp
	p.MentionPromptText = "extract metric candidates"
	p.MentionPromptRef = "prompt-extract-metric-candidates-v1.md"
	p.MentionPromptErr = nil
	p.MentionModelErr = nil
	p.MentionModelName = "gpt-test-mention"
	p.RelationPromptText = "enrich metrics"
	p.RelationPromptRef = "prompt-enrich-metrics-v1.md"
	p.RelationPromptErr = nil
	p.RelationModelErr = nil
	p.RelationModelName = "gpt-test"
	p.PromptText = "extract metrics"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "gpt-test"
	p.ObjectStore = objectStore
	p.ObjectReconciler = ObjectReconciler{Store: &memoryObjectNodeStore{}}

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"2005","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if metricsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", metricsStore.saveCalled)
	}
	if extractor.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2 (candidate + enrich)", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if objectStore.called != 1 {
		t.Fatalf("object store called=%d, want 1", objectStore.called)
	}
	if objectStore.recordID != 2005 || objectStore.artifactType != searchArtifactMetric {
		t.Fatalf("unexpected object store target: record=%d type=%q", objectStore.recordID, objectStore.artifactType)
	}
	if len(objectStore.objects) != 1 || objectStore.objects[0].ArtifactID != "2005_mtc_1" || objectStore.objects[0].ObjectName != "service gateway" {
		t.Fatalf("unexpected persisted objects: %+v", objectStore.objects)
	}
	if !strings.Contains(extractor.inputTexts[0], "Intro") {
		t.Fatalf("overlap line content missing from LLM input; input=%q", extractor.inputTexts[0])
	}
	if !strings.Contains(extractor.inputTexts[0], "Latency must be") {
		t.Fatalf("normal line content missing from LLM input; input=%q", extractor.inputTexts[0])
	}
	// Parsed line hints should include the overlap flag.
	if !strings.Contains(extractor.inputTexts[0], `"flag":"o"`) {
		t.Fatalf("overlap flag missing from LLM input hints; input=%q", extractor.inputTexts[0])
	}

	var statusArr []map[string]any
	if err := json.Unmarshal([]byte(inputStore.updateReq.StatusRaw), &statusArr); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(statusArr) == 0 {
		t.Fatalf("expected status entries")
	}
	last := statusArr[len(statusArr)-1]
	if strings.TrimSpace(asString(last["operation"])) != "extract_metrics" {
		t.Fatalf("operation=%v", last["operation"])
	}
	if strings.TrimSpace(asString(last["proc_status"])) != "success" {
		t.Fatalf("proc_status=%v", last["proc_status"])
	}
	if strings.TrimSpace(asString(last["record_id"])) != "2005" {
		t.Fatalf("record_id=%v", last["record_id"])
	}
	if strings.TrimSpace(asString(last["file_type"])) != "pdf" {
		t.Fatalf("file_type=%v", last["file_type"])
	}
	if strings.TrimSpace(asString(last["input_filename"])) != resultFilename {
		t.Fatalf("input_filename=%v, want %v", last["input_filename"], resultFilename)
	}
	if _, ok := last["ms_used"]; !ok {
		t.Fatalf("ms_used missing from status entry")
	}
}

// TestMetricsProcessor_ExtractsFromLineFileWhenNoContextBuffer verifies that when
// TestMetricsProcessor_ExtractsFromLineFileViaChunksArtifact verifies that HandleEvent
// reads the line file and .chunks artifact, then passes the resolved lines to the LLM.
func TestMetricsProcessor_ExtractsFromLineFileViaChunksArtifact(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(3001)

	writeTestArtifacts(t, tmp, recordID, filepath.Join(tmp, "ocr_rslt_3001.pdf"), "opendata",
		strings.Join([]string{
			"1\t1\theading\tTestFont\t12\t[0,0,1,1]\tIntro",
			"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLatency must be <= 200ms",
		}, "\n"),
		"lines: [1, 2]\n",
	)

	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_3001.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_3001.pdf"),
		StatusRaw:       "[]",
	}}
	metricsStore := &fakeMetricsStore{}
	extractor := &fakeJSONExtractor{outs: []map[string]any{
		{
			"language": "en",
			"candidates": []any{
				map[string]any{
					"metric_name_hint":  "Latency",
					"subject_hint":      "service latency",
					"evidence_quote":    "Latency must be <= 200ms",
					"source_line_spans": []any{float64(2)},
					"unit_hint":         "ms",
					"value_hint":        "200",
					"confidence":        0.9,
				},
			},
		},
		{
			"language": "en",
			"metrics": []any{
				map[string]any{
					"metric_name":         "Latency",
					"source_line_spans":   []any{float64(2)},
					"subject":             "service latency",
					"desc":                "max latency",
					"context":             "SLA",
					"keywords":            []any{"latency"},
					"location_type":       "sentence",
					"unit":                "ms",
					"threshold_or_target": "<=200",
				},
			},
			"uncertain_metrics": []any{},
		},
	}}

	p := NewMetricsProcessor(inputStore, metricsStore, extractor, nil)
	p.ChunkDir = tmp
	p.MentionPromptText = "extract metric candidates"
	p.MentionPromptRef = "prompt-extract-metric-candidates-v1.md"
	p.MentionPromptErr = nil
	p.MentionModelErr = nil
	p.MentionModelName = "gpt-test-mention"
	p.RelationPromptText = "enrich metrics"
	p.RelationPromptRef = "prompt-enrich-metrics-v1.md"
	p.RelationPromptErr = nil
	p.RelationModelErr = nil
	p.RelationModelName = "gpt-test"
	p.PromptText = "extract metrics"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "gpt-test"

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"3001","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if metricsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", metricsStore.saveCalled)
	}
	if extractor.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2 (candidate + enrich for one block)", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if !strings.Contains(extractor.inputTexts[0], "Intro") {
		t.Fatalf("line 1 content missing from LLM input; input=%q", extractor.inputTexts[0])
	}
	if !strings.Contains(extractor.inputTexts[0], "Latency must be") {
		t.Fatalf("line 2 content missing from LLM input; input=%q", extractor.inputTexts[0])
	}
}

func TestMetricsProcessor_UsesMultiPassAndMergesDuplicateCandidates(t *testing.T) {
	tmp := t.TempDir()

	writeTestArtifacts(t, tmp, 3101, filepath.Join(tmp, "ocr_rslt_3101.pdf"), "opendata",
		strings.Join([]string{
			"10\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLatency must be <= 200ms",
			"11\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tThe service response time is monitored daily.",
			"12\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLatency violations trigger alerts.",
		}, "\n"),
		"lines: [10, 11]\n\noverlap: [10]\nlines: [12]\n",
	)

	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              3101,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_3101.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_3101.pdf"),
		StatusRaw:       "[]",
	}}
	metricsStore := &fakeMetricsStore{}
	extractor := &fakeJSONExtractor{
		outs: []map[string]any{
			{
				"language": "en",
				"candidates": []any{
					map[string]any{
						"metric_name_hint":  "Latency",
						"subject_hint":      "service response time",
						"evidence_quote":    "Latency must be <= 200ms",
						"source_line_spans": []any{float64(10), float64(11)},
						"unit_hint":         "ms",
						"value_hint":        "200",
						"confidence":        0.82,
					},
				},
			},
			{
				"language": "en",
				"candidates": []any{
					map[string]any{
						"metric_name_hint":  "Latency",
						"subject_hint":      "service response time",
						"evidence_quote":    "Latency must be <= 200ms",
						"source_line_spans": []any{"10:12"},
						"unit_hint":         "ms",
						"value_hint":        "200",
						"confidence":        0.76,
					},
				},
			},
			{
				"language": "en",
				"metrics": []any{
					map[string]any{
						"metric_name":         "Latency",
						"source_line_spans":   []any{"10:12"},
						"subject":             "service response time",
						"desc":                "maximum latency threshold",
						"context":             "SLA monitoring",
						"keywords":            []any{"latency"},
						"location_type":       "sentence",
						"unit":                "ms",
						"metric_value":        "200",
						"value_data_type":     "numerical",
						"value_range_type":    "<=",
						"threshold_or_target": "<=200",
					},
				},
				"uncertain_metrics": []any{},
			},
			{
				"language":          "en",
				"metrics":           []any{},
				"uncertain_metrics": []any{},
			},
		},
	}

	p := NewMetricsProcessor(inputStore, metricsStore, extractor, nil)
	p.ChunkDir = tmp
	p.MentionPromptText = "extract metric candidates"
	p.MentionPromptRef = "prompt-extract-metric-candidates-v1.md"
	p.MentionPromptErr = nil
	p.MentionModelName = "mention-model"
	p.MentionModelErr = nil
	p.RelationPromptText = "enrich metrics"
	p.RelationPromptRef = "prompt-enrich-metrics-v1.md"
	p.RelationPromptErr = nil
	p.RelationModelName = "relation-model"
	p.RelationModelErr = nil
	p.PromptText = p.RelationPromptText
	p.PromptRef = p.RelationPromptRef
	p.ModelName = p.RelationModelName

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"3101","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 4 {
		t.Fatalf("structuredCalledCount=%d, want 4 (2 candidate passes + 2 enrich batches, one per block)", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if got, want := extractor.modelNames, []string{"mention-model", "mention-model", "relation-model", "relation-model"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("modelNames=%v, want %v", got, want)
	}
	if metricsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", metricsStore.saveCalled)
	}
	if len(metricsStore.lastSave.Metrics) != 1 {
		t.Fatalf("saved metrics=%d, want 1", len(metricsStore.lastSave.Metrics))
	}
	if got := strings.TrimSpace(asString(metricsStore.lastSave.Metrics[0]["metric_id"])); got != "3101_mtc_1" {
		t.Fatalf("metric_id=%q, want 3101_mtc_1", got)
	}
}

func TestMentionsAsCandidates_DropsOverlapOnlyCandidates(t *testing.T) {
	overlapOnly := []metricCandidateMention{
		{
			MetricNameHint:    "Latency",
			SubjectHint:       "service response time",
			EvidenceQuote:     "Latency must be <= 200ms",
			SourceLineSpans:   []string{"10"},
			UnitHint:          "ms",
			ValueHint:         "200",
			ChunkIndex:        1,
			ChunkLines:        []BlockLine{{Flag: "o", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"}},
			HasNormalEvidence: false,
		},
	}
	if got := mentionsAsCandidates(overlapOnly); len(got) != 0 {
		t.Fatalf("overlap-only candidates=%d, want 0", len(got))
	}

	withNormal := append([]metricCandidateMention(nil), overlapOnly...)
	withNormal = append(withNormal, metricCandidateMention{
		MetricNameHint:    "Latency",
		SubjectHint:       "service response time",
		EvidenceQuote:     "Latency must be <= 200ms",
		SourceLineSpans:   []string{"10"},
		UnitHint:          "ms",
		ValueHint:         "200",
		ChunkIndex:        2,
		ChunkLines:        []BlockLine{{Flag: "n", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"}},
		HasNormalEvidence: true,
	})
	merged := mentionsAsCandidates(withNormal)
	if len(merged) != 1 {
		t.Fatalf("merged candidates=%d, want 1", len(merged))
	}
	if got := merged[0].MetricNameHint; got != "Latency" {
		t.Fatalf("metric name=%q", got)
	}
}

func TestMetricsProcessor_RetriesCandidateFallbackOnEmptyJSON(t *testing.T) {
	tmp := t.TempDir()

	writeTestArtifacts(t, tmp, 3201, filepath.Join(tmp, "ocr_rslt_3201.pdf"), "opendata",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLatency must be <= 200ms\n",
		"lines: [2]\n",
	)

	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              3201,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_3201.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_3201.pdf"),
		StatusRaw:       "[]",
	}}
	metricsStore := &fakeMetricsStore{}
	extractor := &fakeJSONExtractor{
		errs: []error{
			fmt.Errorf("(MID_26050174) failed resolveScopedString, error:(MID_26052924) failed resolveScopedString, error:(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}"),
			nil,
			nil,
		},
		outs: []map[string]any{
			nil,
			{
				"language": "en",
				"candidates": []any{
					map[string]any{
						"metric_name_hint":  "Latency",
						"subject_hint":      "service latency",
						"evidence_quote":    "Latency must be <= 200ms",
						"source_line_spans": []any{float64(2)},
						"unit_hint":         "ms",
						"value_hint":        "200",
						"confidence":        0.9,
					},
				},
			},
			{
				"language": "en",
				"metrics": []any{
					map[string]any{
						"metric_name":         "Latency",
						"source_line_spans":   []any{float64(2)},
						"subject":             "service latency",
						"desc":                "max latency",
						"context":             "SLA",
						"keywords":            []any{"latency"},
						"location_type":       "sentence",
						"unit":                "ms",
						"threshold_or_target": "<=200",
					},
				},
				"uncertain_metrics": []any{},
			},
		},
	}

	p := NewMetricsProcessor(inputStore, metricsStore, extractor, nil)
	p.ChunkDir = tmp
	p.MentionPromptText = "extract metric candidates"
	p.MentionPromptRef = "prompt-extract-metric-candidates-v1.md"
	p.MentionPromptErr = nil
	p.MentionModelErr = nil
	p.MentionModelName = "primary-candidate-model"
	p.FallbackMentionModelName = "fallback-candidate-model"
	p.RelationPromptText = "enrich metrics"
	p.RelationPromptRef = "prompt-enrich-metrics-v1.md"
	p.RelationPromptErr = nil
	p.RelationModelErr = nil
	p.RelationModelName = "relation-model"
	p.PromptText = "enrich metrics"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "relation-model"

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"3201","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 3 {
		t.Fatalf("structuredCalledCount=%d, want 3", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if got, want := extractor.modelNames, []string{"primary-candidate-model", "fallback-candidate-model", "relation-model"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("modelNames=%v, want %v", got, want)
	}
	if metricsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", metricsStore.saveCalled)
	}
}

func TestMetricsProcessor_ForceDisablesThinkingForAllPasses(t *testing.T) {
	p := &MetricsProcessor{
		MentionModelCfg: structureModelConfig{
			ModelName:    "deepseek-v4-pro",
			ThinkingType: "enabled",
		},
		FallbackMentionModelCfg: structureModelConfig{
			ModelName:    "deepseek-v4-flash",
			ThinkingType: "enabled",
		},
		RelationModelCfg: structureModelConfig{
			ModelName:    "deepseek-v4-pro",
			ThinkingType: "enabled",
		},
	}

	p.forceDisableThinking()

	if got := p.MentionModelCfg.ThinkingType; got != "disabled" {
		t.Fatalf("MentionModelCfg.ThinkingType=%q, want disabled", got)
	}
	if got := p.FallbackMentionModelCfg.ThinkingType; got != "disabled" {
		t.Fatalf("FallbackMentionModelCfg.ThinkingType=%q, want disabled", got)
	}
	if got := p.RelationModelCfg.ThinkingType; got != "disabled" {
		t.Fatalf("RelationModelCfg.ThinkingType=%q, want disabled", got)
	}
}

func TestLoadMetricsPromptFromEnv_UsesPromptDir(t *testing.T) {
	tmp := t.TempDir()
	promptPath := filepath.Join(tmp, "prompt_extract_metrics_v1.txt")
	want := "extract metrics from lines"
	if err := os.WriteFile(promptPath, []byte(want), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	t.Setenv("PROMPT_DIR", tmp)
	t.Setenv("EXTRACT_METRICS_PROMPT", "prompt_extract_metrics_v1.txt")

	got, ref, gotPath, err := loadMetricsPromptFromEnv()
	if err != nil {
		t.Fatalf("loadMetricsPromptFromEnv: %v", err)
	}
	if got != want {
		t.Fatalf("promptText=%q, want %q", got, want)
	}
	if ref != "prompt_extract_metrics_v1.txt" {
		t.Fatalf("promptRef=%q", ref)
	}
	if gotPath != promptPath {
		t.Fatalf("promptPath=%q, want %q", gotPath, promptPath)
	}
}

// writeTestArtifacts creates the canonical line file and the .chunks artifact file
// under chunkDir so HandleEvent can load them without a live pipeline context.
// stagingFilename is the full path used as rec.StagingFilename (e.g. "{tmp}/foo.pdf").
// lineFileContent uses 7-field TSV: lineNo\tpageNo\tlineType\tfont\tsize\tcoord\tcontent
// chunksContent uses the .chunks format: "overlap: [...]\nlines: [...]"
func writeTestArtifacts(t *testing.T, chunkDir string, recordID int64, stagingFilename, parserName, lineFileContent, chunksContent string) {
	t.Helper()
	root := strings.TrimSuffix(filepath.Base(stagingFilename), filepath.Ext(stagingFilename))
	lineFilePath := filepath.Join(chunkDir, root+"_"+parserName+".txt")
	if err := os.WriteFile(lineFilePath, []byte(lineFileContent), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}
	artifactDir := filepath.Join(chunkDir, fmt.Sprintf("%d", recordID/1000), fmt.Sprintf("%d", recordID))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	chunksPath := filepath.Join(artifactDir, root+"_"+parserName+".chunks")
	if err := os.WriteFile(chunksPath, []byte(chunksContent), 0o644); err != nil {
		t.Fatalf("write chunks file: %v", err)
	}
}

// func osMkdirAll(path string) error {
// 	return os.MkdirAll(path, 0o755)
// }

// ── concurrency test helpers ──────────────────────────────────────────────────

// metricsBlockingExtractor implements LLMJSONExtractor.
// All calls block on gate; it tracks the maximum simultaneous in-flight count.
type metricsBlockingExtractor struct {
	mu          sync.Mutex
	inFlight    int
	MaxInFlight int
	ready       chan struct{}
	readyOnce   sync.Once
	readyAt     int // close ready when inFlight reaches this value
	gate        chan struct{}
	out         map[string]any
	err         error
}

func (e *metricsBlockingExtractor) ExtractJSON(ctx context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	e.mu.Lock()
	e.inFlight++
	if e.inFlight > e.MaxInFlight {
		e.MaxInFlight = e.inFlight
	}
	if e.inFlight >= e.readyAt {
		e.readyOnce.Do(func() { close(e.ready) })
	}
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.inFlight--
		e.mu.Unlock()
	}()

	select {
	case <-e.gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if e.err != nil {
		return nil, e.err
	}
	return e.out, nil
}

// metricsBiPhaseExtractor handles pass1 (non-blocking) and pass2 (blocking).
// The first totalPass1 calls return pass1Out immediately; subsequent calls block.
type metricsBiPhaseExtractor struct {
	mu          sync.Mutex
	calls       int
	totalPass1  int
	pass1Out    map[string]any
	inFlight    int
	MaxInFlight int
	ready       chan struct{}
	readyOnce   sync.Once
	readyAt     int
	gate        chan struct{}
	pass2Out    map[string]any
}

func (e *metricsBiPhaseExtractor) ExtractJSON(ctx context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	e.mu.Lock()
	n := e.calls
	e.calls++
	if n < e.totalPass1 {
		out := e.pass1Out
		e.mu.Unlock()
		return out, nil
	}
	e.inFlight++
	if e.inFlight > e.MaxInFlight {
		e.MaxInFlight = e.inFlight
	}
	if e.inFlight >= e.readyAt {
		e.readyOnce.Do(func() { close(e.ready) })
	}
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.inFlight--
		e.mu.Unlock()
	}()

	select {
	case <-e.gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return e.pass2Out, nil
}

// metricsSeqErrExtractor returns preset outputs then errors for selected calls.
type metricsSeqErrExtractor struct {
	mu   sync.Mutex
	outs []map[string]any
	errs []error
}

func (e *metricsSeqErrExtractor) ExtractJSON(_ context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out map[string]any
	var err error
	if len(e.outs) > 0 {
		out = e.outs[0]
		e.outs = e.outs[1:]
	}
	if len(e.errs) > 0 {
		err = e.errs[0]
		e.errs = e.errs[1:]
	}
	return out, err
}

func makeTestMetricsProcessor(ext LLMJSONExtractor, maxTasks int) *MetricsProcessor {
	return &MetricsProcessor{
		Logger:                loggerutil.CreateDefaultLogger("TEST_METRICS"),
		ProcLogger:            DocProcLogger{DB: nil},
		Now:                   time.Now,
		Extractor:             ext,
		MentionPromptText:     "candidates prompt",
		MentionPromptRef:      "test-candidates-prompt",
		MentionModelName:      "test-mention-model",
		RelationPromptText:    "enrich prompt",
		RelationPromptRef:     "test-enrich-prompt",
		RelationModelName:     "test-relation-model",
		MetricEnrichGroupSize: 1,
		MaxTasks:              maxTasks,
	}
}

// makeMetricsBlock creates a Block with one normal line at the given line number.
func makeMetricsBlock(idx, lineNum int) Block {
	return Block{
		Index: idx,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: lineNum, Content: fmt.Sprintf("metric text %d", lineNum)},
		},
	}
}

func metricsBlocksToChunks(blocks []Block) []Chunk {
	chunks := make([]Chunk, len(blocks))
	for i, b := range blocks {
		lines := make([]MarkedLine, len(b.Lines))
		for j, bl := range b.Lines {
			lines[j] = MarkedLine{
				Mark: bl.Flag,
				Line: Line{LineNo: bl.LineNumber, PageNo: bl.PageNumber, LineType: bl.LineType, Content: bl.Content},
			}
		}
		chunks[i] = Chunk{SeqNo: b.Index, Lines: lines}
	}
	return chunks
}

// pass1NoCandidates is the extractor output for a Pass 1 chunk with no candidates.
var pass1NoCandidates = map[string]any{
	"language":   "en",
	"candidates": []any{},
}

// pass1WithCandidate returns a Pass 1 output containing one candidate referencing lineNum.
func pass1WithCandidate(lineNum int) map[string]any {
	return map[string]any{
		"language": "en",
		"candidates": []any{
			map[string]any{
				"metric_name_hint":  "Metric",
				"subject_hint":      "system",
				"evidence_quote":    fmt.Sprintf("metric text %d", lineNum),
				"source_line_spans": []any{float64(lineNum)},
				"unit_hint":         "",
				"value_hint":        "",
				"confidence":        0.9,
			},
		},
	}
}

// pass2EnrichOut is a minimal Pass 2 enrichment output.
var pass2EnrichOut = map[string]any{
	"language": "en",
	"metrics": []any{
		map[string]any{
			"metric_name":       "Metric",
			"source_line_spans": []any{float64(1)},
		},
	},
	"uncertain_metrics": []any{},
}

func TestNormalizeMetricListPreservesMetricCategoriesAndCategoryPaths(t *testing.T) {
	categoryPaths := []any{
		map[string]any{
			"category_path": []any{
				map[string]any{"name": "Public Health", "keywords": []any{"health"}, "confidence": 0.9},
				map[string]any{"name": "Vaccination", "keywords": []any{"vaccine"}, "confidence": 0.8},
			},
			"path_keywords":   []any{"vaccination"},
			"path_confidence": 0.8,
		},
	}
	got := normalizeMetricList([]any{
		map[string]any{
			"metric_name":       "Coverage",
			"source_line_spans": []any{"22-45"},
			"metric_categories": " Public Health, Vaccination ",
			"category_paths":    categoryPaths,
		},
	})
	if len(got) != 1 {
		t.Fatalf("normalizeMetricList len=%d", len(got))
	}
	if !reflect.DeepEqual(got[0]["metric_categories"], []string{"public_health", "vaccination"}) {
		t.Fatalf("metric_categories=%#v", got[0]["metric_categories"])
	}
	if !reflect.DeepEqual(got[0]["source_line_spans"], []string{"22:45"}) {
		t.Fatalf("source_line_spans=%#v", got[0]["source_line_spans"])
	}
	if got[0]["category_paths"] == nil {
		t.Fatal("category_paths should be preserved")
	}
}

func TestNormalizeMetricListPreservesObjects(t *testing.T) {
	got := normalizeMetricList([]any{
		map[string]any{
			"metric_name":       "maximum pressure",
			"subject":           "tank",
			"source_line_spans": []any{"10"},
			"objects": []any{map[string]any{
				"object_name":       "液化气储罐",
				"object_name_en":    "LPG storage tank",
				"object_role":       "measured_object",
				"source_line_spans": []any{"10"},
			}},
		},
	})
	if len(got) != 1 {
		t.Fatalf("normalizeMetricList len=%d", len(got))
	}
	objects := objectItemsFromValue(got[0]["objects"])
	if len(objects) != 1 {
		t.Fatalf("objects=%#v, want one object", got[0]["objects"])
	}
	if objects[0]["object_name"] != "液化气储罐" || objects[0]["object_name_en"] != "LPG storage tank" {
		t.Fatalf("unexpected objects: %#v", objects)
	}
}

func TestBuildMetricRelationBatchPromptIncludesMetricCategorySchema(t *testing.T) {
	prompt := buildMetricRelationBatchPrompt([]metricCandidate{
		{
			CandidateID:    "metric_cand_1",
			MetricNameHint: "Latency",
			SupportLines: []BlockLine{
				{Flag: "n", LineNumber: 2, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"},
			},
		},
	})
	if !strings.Contains(prompt, `"metric_categories"`) {
		t.Fatalf("metric_categories missing from relation batch schema: %s", prompt)
	}
	if !strings.Contains(prompt, `"metric_categories_en"`) {
		t.Fatalf("metric_categories_en missing from relation batch schema: %s", prompt)
	}
	if !strings.Contains(prompt, `"objects"`) || !strings.Contains(prompt, `"object_name"`) {
		t.Fatalf("objects schema missing from relation batch schema: %s", prompt)
	}
}

func TestMetricCandidateTaskIncludesObjectHints(t *testing.T) {
	prompt := metricCandidateTask("candidate prompt")
	for _, want := range []string{`"object_hint"`, `"object_type_hint"`, `"object_role_hint"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("%s missing from candidate task schema: %s", want, prompt)
		}
	}
}

func TestDedupeFinalMetricRowsMergesMetricCategoriesEn(t *testing.T) {
	got := dedupeFinalMetricRows([]map[string]any{
		{
			"metric_name":          "Latency",
			"subject":              "API",
			"unit":                 "ms",
			"metric_value":         "200",
			"source_line_spans":    []any{"10"},
			"metric_categories":    []any{"performance"},
			"metric_categories_en": []any{"Performance"},
			"confidence":           0.7,
		},
		{
			"metric_name":          "Latency",
			"subject":              "API",
			"unit":                 "ms",
			"metric_value":         "200",
			"source_line_spans":    []any{"10"},
			"metric_categories":    []any{"quality"},
			"metric_categories_en": []any{"Quality"},
			"confidence":           0.9,
		},
	})
	if len(got) != 1 {
		t.Fatalf("deduped rows=%d, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0]["metric_categories"], []string{"performance", "quality"}) {
		t.Fatalf("metric_categories=%#v", got[0]["metric_categories"])
	}
	if !reflect.DeepEqual(got[0]["metric_categories_en"], []string{"performance", "quality"}) {
		t.Fatalf("metric_categories_en=%#v", got[0]["metric_categories_en"])
	}
	if gotConfidence := toFloat(got[0]["confidence"]); gotConfidence != 0.9 {
		t.Fatalf("confidence=%v, want 0.9", gotConfidence)
	}
}

func TestMetricsSQLStoreSaveMetricsPersistsMetricCategoriesEn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb;").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO kb.metrics").
		WithArgs(
			nil,
			int64(42),
			"42_1",
			"Latency",
			"Latency",
			`["10"]`,
			"API",
			"API",
			"Maximum latency",
			"Maximum latency",
			"SLA",
			"SLA",
			`["latency"]`,
			`["latency"]`,
			"relation-model",
			"prompt-enrich-metrics-v1.md",
			"sentence",
			"ms",
			"ms",
			"200",
			"number",
			"maximum",
			"performance",
			"performance",
			"",
			"<=200",
			"daily",
			0.91,
			true,
			"SLA",
			`["named_metric"]`,
			`["performance"]`,
			`["performance_en"]`,
			"null",
			"null",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	store := MetricsSQLStore{DB: db}
	inserted, err := store.SaveMetrics(context.Background(), SaveMetricsRequest{
		InputRecordID: int64(42),
		Language:      "zh",
		ModelName:     "relation-model",
		PromptName:    "prompt-enrich-metrics-v1.md",
		Metrics: []map[string]any{
			{
				"metric_id":             "42_1",
				"metric_name":           "Latency",
				"metric_name_en":        "Latency",
				"source_line_spans":     []string{"10"},
				"subject":               "API",
				"subject_en":            "API",
				"desc":                  "Maximum latency",
				"desc_en":               "Maximum latency",
				"context":               "SLA",
				"context_en":            "SLA",
				"keywords":              []string{"latency"},
				"keywords_en":           []string{"latency"},
				"location_type":         "sentence",
				"unit":                  "ms",
				"unit_en":               "ms",
				"metric_value":          "200",
				"value_data_type":       "number",
				"value_range_type":      "maximum",
				"value_class":           "performance",
				"value_class_en":        "performance",
				"threshold_or_target":   "<=200",
				"measurement_frequency": "daily",
				"confidence":            0.91,
				"is_explicit_metric":    true,
				"table_name_or_section": "SLA",
				"reasoning_tags":        []string{"named_metric"},
				"metric_categories":     []string{"performance"},
				"metric_categories_en":  []string{"performance_en"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveMetrics returned error: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted=%d, want 1", inserted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMetricsSQLStore_UpsertMetrics_OnConflictUpdatesOnlyGivenRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	store := MetricsSQLStore{DB: db}
	mock.ExpectExec(`CREATE SCHEMA IF NOT EXISTS kb`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO kb\.metrics`).
		WithArgs(sqlmock.AnyArg(), int64(173), "173_mtc_1", sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	n, err := store.UpsertMetrics(context.Background(), SaveMetricsRequest{
		InputRecordID: 173,
		Metrics: []map[string]any{
			{"metric_id": "173_mtc_1", "metric_name": "Latency", "source_line_spans": []any{float64(2)}},
		},
	})
	if err != nil {
		t.Fatalf("UpsertMetrics: %v", err)
	}
	if n != 1 {
		t.Fatalf("n=%d, want 1", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMetricsSQLStore_UpsertMetrics_NoMetricsIsNoop(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	store := MetricsSQLStore{DB: db}
	mock.ExpectExec(`CREATE SCHEMA IF NOT EXISTS kb`).WillReturnResult(sqlmock.NewResult(0, 0))

	n, err := store.UpsertMetrics(context.Background(), SaveMetricsRequest{InputRecordID: 173})
	if err != nil {
		t.Fatalf("UpsertMetrics: %v", err)
	}
	if n != 0 {
		t.Fatalf("n=%d, want 0", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMetricsSQLStore_GetMetricsByInputRecordID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	store := MetricsSQLStore{DB: db}
	mock.ExpectExec(`CREATE SCHEMA IF NOT EXISTS kb`).WillReturnResult(sqlmock.NewResult(0, 0))

	cols := []string{
		"id", "metric_id", "metric_name", "metric_name_en", "source_line_spans", "metric_subject",
		"metric_subject_en", "metric_desc", "metric_desc_en", "metric_context", "metric_context_en",
		"metric_keywords", "metric_keywords_en", "metric_unit", "metric_unit_en", "metric_value",
		"value_data_type", "value_range_type", "value_class", "value_class_en",
		"formula_or_definition", "threshold_or_target", "measurement_frequency",
		"metric_categories", "metric_categories_en", "ext_info",
	}
	rows := sqlmock.NewRows(cols).AddRow(
		int64(99), "173_mtc_1", "Latency", "Latency", `[2]`, "API",
		"API", "Max latency", "Max latency", "SLA", "SLA",
		`["latency"]`, `["latency"]`, "ms", "ms", "200",
		"number", "maximum", "performance", "performance",
		"", "<=200", "daily",
		`["performance"]`, `["performance"]`, `{"language":"zh","schema_version":"2"}`,
	)
	mock.ExpectQuery(`SELECT .* FROM kb\.metrics WHERE input_record_id = \$1`).
		WithArgs(int64(173)).
		WillReturnRows(rows)

	got, err := store.GetMetricsByInputRecordID(context.Background(), 173)
	if err != nil {
		t.Fatalf("GetMetricsByInputRecordID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	row := got[0]
	if id, ok := row["id"].(int64); !ok || id != 99 {
		t.Fatalf("id=%#v, want int64(99)", row["id"])
	}
	if row["metric_id"] != "173_mtc_1" {
		t.Fatalf("metric_id=%#v", row["metric_id"])
	}
	if row["metric_name"] != "Latency" {
		t.Fatalf("metric_name=%#v", row["metric_name"])
	}
	kwEn, ok := row["metric_keywords_en"].([]any)
	if !ok || len(kwEn) != 1 || kwEn[0] != "latency" {
		t.Fatalf("metric_keywords_en=%#v, want [\"latency\"]", row["metric_keywords_en"])
	}
	catsEn, ok := row["metric_categories_en"].([]any)
	if !ok || len(catsEn) != 1 || catsEn[0] != "performance" {
		t.Fatalf("metric_categories_en=%#v, want [\"performance\"]", row["metric_categories_en"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMetricCategoryKeysFromMetricFallsBackToCategoryPaths(t *testing.T) {
	metric := map[string]any{
		"category_paths": []any{
			map[string]any{
				"category_path": []any{
					map[string]any{"name": "System Safety"},
					map[string]any{"name": "Alarm Thresholds"},
				},
			},
		},
	}
	got := metricCategoryKeysFromMetric(metric)
	want := []string{"system_safety", "alarm_thresholds"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metricCategoryKeysFromMetric=%v, want %v", got, want)
	}
}

// ── concurrency bound tests ───────────────────────────────────────────────────

// TestMetricsProcessor_Pass1ConcurrencyBound verifies that at most MaxTasks
// goroutines call the extractor simultaneously during Pass 1.
func TestMetricsProcessor_Pass1ConcurrencyBound(t *testing.T) {
	const numChunks = 4
	const maxTasks = 2

	ext := &metricsBlockingExtractor{
		ready:   make(chan struct{}),
		readyAt: maxTasks,
		gate:    make(chan struct{}, numChunks),
		out:     pass1NoCandidates,
	}

	blocks := make([]Block, numChunks)
	for i := range blocks {
		blocks[i] = makeMetricsBlock(i+1, i+1)
	}

	p := makeTestMetricsProcessor(ext, maxTasks)

	done := make(chan error, 1)
	go func() {
		_, err := p.extractMetricsFromChunksWithLLM(context.Background(), 1, metricsBlocksToChunks(blocks), "")
		done <- err
	}()

	select {
	case <-ext.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for workers to reach maxTasks in-flight")
	}

	for range numChunks {
		ext.gate <- struct{}{}
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for extraction to complete")
	}

	ext.mu.Lock()
	got := ext.MaxInFlight
	ext.mu.Unlock()
	if got != maxTasks {
		t.Errorf("Pass1 MaxInFlight = %d, want %d", got, maxTasks)
	}
}

// TestMetricsProcessor_Pass2ConcurrencyBound verifies that at most MaxTasks
// goroutines call the extractor simultaneously during Pass 2 enrichment.
func TestMetricsProcessor_Pass2ConcurrencyBound(t *testing.T) {
	const numChunks = 3
	const maxTasks = 2

	// Pass 1: numChunks non-blocking calls each returning one candidate.
	// Pass 2: numChunks blocking calls (MetricEnrichGroupSize=1 → one batch per candidate).
	ext := &metricsBiPhaseExtractor{
		totalPass1: numChunks,
		pass1Out:   pass1WithCandidate(1),
		ready:      make(chan struct{}),
		readyAt:    maxTasks,
		gate:       make(chan struct{}, numChunks),
		pass2Out:   pass2EnrichOut,
	}

	blocks := make([]Block, numChunks)
	for i := range blocks {
		blocks[i] = makeMetricsBlock(i+1, 1)
	}

	p := makeTestMetricsProcessor(ext, maxTasks)

	done := make(chan error, 1)
	go func() {
		_, err := p.extractMetricsFromChunksWithLLM(context.Background(), 1, metricsBlocksToChunks(blocks), "")
		done <- err
	}()

	select {
	case <-ext.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Pass 2 workers to reach maxTasks in-flight")
	}

	for range numChunks {
		ext.gate <- struct{}{}
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for extraction to complete")
	}

	ext.mu.Lock()
	got := ext.MaxInFlight
	ext.mu.Unlock()
	if got != maxTasks {
		t.Errorf("Pass2 MaxInFlight = %d, want %d", got, maxTasks)
	}
}

// TestMetricsProcessor_Pass1DeterministicOrder verifies that metrics from
// sequential pass 1 and pass 2 execution are collected correctly.
func TestMetricsProcessor_Pass1DeterministicOrder(t *testing.T) {
	lineNums := []int{10, 20, 30}
	blocks := make([]Block, len(lineNums))
	for i, ln := range lineNums {
		blocks[i] = makeMetricsBlock(i+1, ln)
	}

	// 3 pass-1 responses + 3 pass-2 enrich responses (one batch per candidate).
	enrichOut := func(lineNum int) map[string]any {
		return map[string]any{
			"language": "en",
			"metrics": []any{
				map[string]any{
					"metric_name":       fmt.Sprintf("Metric%d", lineNum),
					"source_line_spans": []any{float64(lineNum)},
				},
			},
			"uncertain_metrics": []any{},
		}
	}
	ext := &metricsSeqErrExtractor{
		outs: []map[string]any{
			pass1WithCandidate(lineNums[0]),
			pass1WithCandidate(lineNums[1]),
			pass1WithCandidate(lineNums[2]),
			enrichOut(lineNums[0]),
			enrichOut(lineNums[1]),
			enrichOut(lineNums[2]),
		},
		errs: []error{nil, nil, nil, nil, nil, nil},
	}

	p := makeTestMetricsProcessor(ext, 1) // sequential — deterministic call order
	result, err := p.extractMetricsFromChunksWithLLM(context.Background(), 1, metricsBlocksToChunks(blocks), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After dedup, 3 distinct metrics (different metric_name values).
	if len(result.Metrics) != 3 {
		t.Fatalf("expected 3 metrics after dedup, got %d", len(result.Metrics))
	}
}

// TestMetricsProcessor_Pass1FailFast verifies that a Pass 1 LLM error causes
// extractMetricsFromChunksWithLLM to return that error.
func TestMetricsProcessor_Pass1FailFast(t *testing.T) {
	ext := &metricsSeqErrExtractor{
		outs: []map[string]any{pass1NoCandidates, nil, pass1NoCandidates},
		errs: []error{nil, errors.New("candidate extraction boom"), nil},
	}

	blocks := []Block{
		makeMetricsBlock(1, 1),
		makeMetricsBlock(2, 2),
		makeMetricsBlock(3, 3),
	}

	p := makeTestMetricsProcessor(ext, 1) // sequential so the error always fires
	_, err := p.extractMetricsFromChunksWithLLM(context.Background(), 1, metricsBlocksToChunks(blocks), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "candidate extraction boom") {
		t.Errorf("error does not contain expected message: %v", err)
	}
}

// TestMetricsProcessor_Pass2FailFast verifies that a Pass 2 LLM error causes
// extractMetricsFromChunksWithLLM to return that error.
func TestMetricsProcessor_Pass2FailFast(t *testing.T) {
	// Pass 1: 2 chunks, each with one candidate → 2 batches for pass 2.
	// Pass 2: first batch succeeds, second fails.
	ext := &metricsSeqErrExtractor{
		outs: []map[string]any{
			pass1WithCandidate(1),
			pass1WithCandidate(1),
			pass2EnrichOut,
			nil,
		},
		errs: []error{nil, nil, nil, errors.New("enrich batch boom")},
	}

	blocks := []Block{
		makeMetricsBlock(1, 1),
		makeMetricsBlock(2, 1),
	}

	p := makeTestMetricsProcessor(ext, 1) // sequential so the error always fires
	_, err := p.extractMetricsFromChunksWithLLM(context.Background(), 1, metricsBlocksToChunks(blocks), "")
	if err == nil {
		t.Fatal("expected error from Pass 2, got nil")
	}
	if !strings.Contains(err.Error(), "enrich batch boom") {
		t.Errorf("error does not contain expected message: %v", err)
	}
}
