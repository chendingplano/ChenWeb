package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

	payload, err := p.extractMetricPayload(context.Background(), "input text", "prompt text", "metrics-model", structureModelConfig{})
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

// TestMetricsProcessor_ExtractsFromBlocksInContext verifies that HandleEvent uses
// a BlockBuffer injected via context (as BlockingProcessor would provide) and
// passes block-format lines to the LLM.
func TestMetricsProcessor_ExtractsFromBlocksInContext(t *testing.T) {
	ctx, holder := withBlockBufferHolder(context.Background())
	holder.mu.Lock()
	holder.buffer = &BlockBuffer{
		Blocks: []Block{{
			Index: 1,
			Lines: []BlockLine{
				{Flag: "o", LineNumber: 1, PageNumber: 1, LineType: "heading", Content: "Intro"},
				{Flag: "n", LineNumber: 2, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"},
			},
		}},
	}
	holder.mu.Unlock()

	tmp := t.TempDir()
	resultFilename := filepath.Join(tmp, "ocr_rslt_2005.json")
	stagingFilename := filepath.Join(tmp, "ocr_rslt_2005.pdf")
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              2005,
		ParserName:      "opendata",
		ResultFilename:  resultFilename,
		StagingFilename: stagingFilename,
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

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"2005","force":true}`)); err != nil {
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
// no BlockBuffer is in context, HandleEvent reads the canonical line file, builds
// blocks, and passes them to the LLM.
func TestMetricsProcessor_ExtractsFromLineFileWhenNoContextBuffer(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(3001)

	lineFileBody := strings.Join([]string{
		"1\t1\theading\tTestFont\t12\t[0,0,1,1]\tIntro",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLatency must be <= 200ms",
	}, "\n")
	lineFilePath := filepath.Join(tmp, "ocr_rslt_3001_opendata.txt")
	if err := osWriteFile(lineFilePath, []byte(lineFileBody)); err != nil {
		t.Fatalf("write line file: %v", err)
	}

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
	ctx, holder := withBlockBufferHolder(context.Background())
	holder.mu.Lock()
	holder.buffer = &BlockBuffer{
		Blocks: []Block{
			{
				Index: 1,
				Lines: []BlockLine{
					{Flag: "n", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"},
					{Flag: "n", LineNumber: 11, PageNumber: 1, LineType: "paragraph", Content: "The service response time is monitored daily."},
				},
			},
			{
				Index: 2,
				Lines: []BlockLine{
					{Flag: "o", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"},
					{Flag: "n", LineNumber: 12, PageNumber: 1, LineType: "paragraph", Content: "Latency violations trigger alerts."},
				},
			},
		},
	}
	holder.mu.Unlock()

	tmp := t.TempDir()
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
						"category_paths":      []any{},
						"category_paths_en":   []any{},
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

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"3101","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 3 {
		t.Fatalf("structuredCalledCount=%d, want 3", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if got, want := extractor.modelNames, []string{"mention-model", "mention-model", "relation-model"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("modelNames=%v, want %v", got, want)
	}
	if metricsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", metricsStore.saveCalled)
	}
	if len(metricsStore.lastSave.Metrics) != 1 {
		t.Fatalf("saved metrics=%d, want 1", len(metricsStore.lastSave.Metrics))
	}
	if got := strings.TrimSpace(asString(metricsStore.lastSave.Metrics[0]["metric_id"])); got != "3101_1" {
		t.Fatalf("metric_id=%q, want 3101_1", got)
	}
}

func TestMetricsProcessor_RepairsMissingEnglishCategoryPathsBeforeIndexing(t *testing.T) {
	ctx, holder := withBlockBufferHolder(context.Background())
	holder.mu.Lock()
	holder.buffer = &BlockBuffer{
		Blocks: []Block{{
			Index: 1,
			Lines: []BlockLine{
				{Flag: "n", LineNumber: 1, PageNumber: 1, LineType: "paragraph", Content: "本标准规定了时延要求。"},
			},
		}},
	}
	holder.mu.Unlock()

	tmp := t.TempDir()
	artifactWebDir := filepath.Join(tmp, "artifact_web")
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              3301,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_3301.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_3301.pdf"),
		StatusRaw:       "[]",
	}}
	metricsStore := &fakeMetricsStore{}
	extractor := &fakeJSONExtractor{
		outs: []map[string]any{
			{
				"language": "zh",
				"candidates": []any{
					map[string]any{
						"metric_name_hint":  "时延",
						"subject_hint":      "系统性能",
						"evidence_quote":    "本标准规定了时延要求。",
						"source_line_spans": []any{float64(1)},
						"unit_hint":         "ms",
						"value_hint":        "200",
						"confidence":        0.88,
					},
				},
			},
			{
				"language": "zh",
				"metrics": []any{
					map[string]any{
						"metric_name":       "时延",
						"metric_name_en":    "Latency",
						"source_line_spans": []any{float64(1)},
						"subject":           "系统性能",
						"subject_en":        "System Performance",
						"desc":              "最大允许时延",
						"desc_en":           "Maximum allowed latency",
						"keywords":          []any{"时延"},
						"keywords_en":       []any{"latency"},
						"location_type":     "sentence",
						"unit":              "毫秒",
						"unit_en":           "ms",
						"metric_value":      "200",
						"category_paths": []any{
							map[string]any{
								"category_path": []any{
									map[string]any{"name": "标准文件", "keywords": []any{"规范"}, "confidence": 0.9},
								},
								"path_keywords":   []any{"规范"},
								"path_confidence": 0.9,
							},
						},
						"category_paths_en": []any{},
					},
				},
				"uncertain_metrics": []any{},
			},
			{
				"category_paths_en": []any{
					map[string]any{
						"category_path": []any{
							map[string]any{"name": "Standards", "keywords": []any{"standards"}, "confidence": 0.95},
						},
						"path_keywords":   []any{"standards"},
						"path_confidence": 0.95,
					},
				},
			},
		},
	}

	p := NewMetricsProcessor(inputStore, metricsStore, extractor, nil)
	p.ChunkDir = tmp
	p.ArtifactWebDir = artifactWebDir
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
	p.TranslationEnabled = true
	p.TranslationModelName = "translation-model"
	p.TranslationModelCfg = structureModelConfig{ModelName: "translation-model"}

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"3301","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 3 {
		t.Fatalf("structuredCalledCount=%d, want 3", extractor.structuredCalledCount)
	}
	if metricsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", metricsStore.saveCalled)
	}
	gotPathsEn := parseCategoryPathsAny(metricsStore.lastSave.Metrics[0]["category_paths_en"])
	if len(gotPathsEn) != 1 || len(gotPathsEn[0].Nodes) != 1 || gotPathsEn[0].Nodes[0].Name != "Standards" {
		t.Fatalf("category_paths_en=%#v, want translated English path", metricsStore.lastSave.Metrics[0]["category_paths_en"])
	}
	leaf := filepath.Join(artifactWebDir, "standards", "metrics.txt")
	body, err := os.ReadFile(leaf)
	if err != nil {
		t.Fatalf("read indexed metrics: %v", err)
	}
	if strings.TrimSpace(string(body)) != "3301_1" {
		t.Fatalf("metrics.txt=%q, want 3301_1", strings.TrimSpace(string(body)))
	}
	if _, err := os.Stat(filepath.Join(artifactWebDir, "标准文件")); !os.IsNotExist(err) {
		t.Fatalf("unexpected non-English category dir created: err=%v", err)
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
			BlockLines:        []BlockLine{{Flag: "o", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"}},
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
		BlockLines:        []BlockLine{{Flag: "n", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"}},
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
	ctx, holder := withBlockBufferHolder(context.Background())
	holder.mu.Lock()
	holder.buffer = &BlockBuffer{
		Blocks: []Block{{
			Index: 1,
			Lines: []BlockLine{
				{Flag: "n", LineNumber: 2, PageNumber: 1, LineType: "paragraph", Content: "Latency must be <= 200ms"},
			},
		}},
	}
	holder.mu.Unlock()

	tmp := t.TempDir()
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
			fmt.Errorf("(MID_26050174) failed resolveScopedString, error:(MID_26050177) failed resolveScopedString, error:(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}"),
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

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"3201","force":true}`)); err != nil {
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
			ModelName:    "gpt-5.4-mini",
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

func osWriteFile(path string, body []byte) error {
	return os.WriteFile(path, body, 0o644)
}

// func osMkdirAll(path string) error {
// 	return os.MkdirAll(path, 0o755)
// }
