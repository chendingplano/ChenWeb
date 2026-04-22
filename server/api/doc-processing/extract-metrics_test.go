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

func (f *fakeMetricsStore) MetricsExist(_ context.Context, _ int64, _ string) (bool, error) {
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

func TestReadRegularLinesFromChunk_SkipsOverlap(t *testing.T) {
	tmp := t.TempDir()
	chunk := filepath.Join(tmp, "chunk_0001")
	body := strings.Join([]string{
		"r 1	1	heading	TestFont	12	[0,0,1,1]	Intro",
		"o 2	1	paragraph	TestFont	12	[0,0,1,1]	Overlap",
		"r 3	1	paragraph	TestFont	12	[0,0,1,1]	Normal",
	}, "\n")
	if err := osWriteFile(chunk, []byte(body)); err != nil {
		t.Fatalf("write chunk: %v", err)
	}

	lines, err := readRegularLinesFromChunk(chunk)
	if err != nil {
		t.Fatalf("readRegularLinesFromChunk: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines=%v", lines)
	}
	if strings.Contains(strings.Join(lines, "\n"), "Overlap") {
		t.Fatalf("overlap lines should be excluded: %v", lines)
	}
}

func TestMetricsProcessor_ExtractsFromChunkFilesAndWritesStatus(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(2005)
	chunkDir := filepath.Join(tmp, "2", "2005")
	if err := osMkdirAll(chunkDir); err != nil {
		t.Fatalf("mkdir chunk dir: %v", err)
	}
	chunkFile := filepath.Join(chunkDir, "chunk_0001")
	body := strings.Join([]string{
		"r 1	1	heading	TestFont	12	[0,0,1,1]	Intro",
		"o 2	1	paragraph	TestFont	12	[0,0,1,1]	Ignore overlap",
		"r 3	1	paragraph	TestFont	12	[0,0,1,1]	Latency must be <= 200ms",
	}, "\n")
	if err := osWriteFile(chunkFile, []byte(body)); err != nil {
		t.Fatalf("write chunk: %v", err)
	}

	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_2005.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_2005.pdf"),
		StatusRaw:       "[]",
	}}
	metricsStore := &fakeMetricsStore{}
	extractor := &fakeJSONExtractor{out: map[string]any{
		"language": "en",
		"metrics": []any{
			map[string]any{
				"metric_name":         "Latency",
				"source_line_spans":   []any{float64(3)},
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

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"2005","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if metricsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d", metricsStore.saveCalled)
	}
	if strings.Contains(extractor.inputText, "Ignore overlap") {
		t.Fatalf("overlap lines must not be sent to LLM")
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
}

func osWriteFile(path string, body []byte) error {
	return os.WriteFile(path, body, 0o644)
}

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
