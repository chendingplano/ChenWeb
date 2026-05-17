package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	extractor := &fakeJSONExtractor{out: map[string]any{
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
	}}

	p := NewMetricsProcessor(inputStore, metricsStore, extractor, nil)
	p.ChunkDir = tmp
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
	if extractor.calledCount != 1 {
		t.Fatalf("LLM called %d times, want 1 (one block)", extractor.calledCount)
	}
	if !strings.Contains(extractor.inputText, "Intro") {
		t.Fatalf("overlap line content missing from LLM input; input=%q", extractor.inputText)
	}
	if !strings.Contains(extractor.inputText, "Latency must be") {
		t.Fatalf("normal line content missing from LLM input; input=%q", extractor.inputText)
	}
	// Parsed line hints should include the overlap flag.
	if !strings.Contains(extractor.inputText, `"flag":"o"`) {
		t.Fatalf("overlap flag missing from LLM input hints; input=%q", extractor.inputText)
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
	extractor := &fakeJSONExtractor{out: map[string]any{
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
	}}

	p := NewMetricsProcessor(inputStore, metricsStore, extractor, nil)
	p.ChunkDir = tmp
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
	if extractor.calledCount != 1 {
		t.Fatalf("LLM called %d times, want 1 (one block for single-page doc)", extractor.calledCount)
	}
	if !strings.Contains(extractor.inputText, "Intro") {
		t.Fatalf("line 1 content missing from LLM input; input=%q", extractor.inputText)
	}
	if !strings.Contains(extractor.inputText, "Latency must be") {
		t.Fatalf("line 2 content missing from LLM input; input=%q", extractor.inputText)
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

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
