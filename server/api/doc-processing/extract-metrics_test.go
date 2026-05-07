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

	lines, err := readRegularLinesFromChunk(chunk, "")
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

func TestReadRegularLinesFromChunk_SummaryFormatUsesSourceLineFile(t *testing.T) {
	tmp := t.TempDir()
	chunk := filepath.Join(tmp, "std_20039_opendata.chunks")
	lineFile := filepath.Join(tmp, "std_20039_opendata.txt")
	chunkBody := strings.Join([]string{
		"overlap: [2]",
		"lines: [1, 3-4]",
	}, "\n")
	lineBody := strings.Join([]string{
		"1\t1\theading\tTestFont\t12\t[0,0,1,1]\tIntro",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tOverlap",
		"3\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tBody",
		"4\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tTail",
	}, "\n")
	if err := osWriteFile(chunk, []byte(chunkBody)); err != nil {
		t.Fatalf("write chunk: %v", err)
	}
	if err := osWriteFile(lineFile, []byte(lineBody)); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	lines, err := readRegularLinesFromChunk(chunk, lineFile)
	if err != nil {
		t.Fatalf("readRegularLinesFromChunk: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("lines=%v", lines)
	}
	got := strings.Join(lines, "\n")
	if strings.Contains(got, "Overlap") {
		t.Fatalf("overlap lines should be excluded: %v", lines)
	}
	for _, want := range []string{"Intro", "Body", "Tail"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
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
	lineFile := filepath.Join(tmp, "ocr_rslt_2005_opendata.txt")
	body := strings.Join([]string{
		"overlap: [2]",
		"lines: [1, 3]",
	}, "\n")
	if err := osWriteFile(chunkFile, []byte(body)); err != nil {
		t.Fatalf("write chunk: %v", err)
	}
	lineBody := strings.Join([]string{
		"1\t1\theading\tTestFont\t12\t[0,0,1,1]\tIntro",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tIgnore overlap",
		"3\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLatency must be <= 200ms",
	}, "\n")
	if err := osWriteFile(lineFile, []byte(lineBody)); err != nil {
		t.Fatalf("write line file: %v", err)
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

func TestMetricsProcessor_DetectChunkingInputs_PrefersSpecArtifacts(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(2005)
	chunkDir := filepath.Join(tmp, "2", "2005")
	if err := osMkdirAll(chunkDir); err != nil {
		t.Fatalf("mkdir chunk dir: %v", err)
	}
	if err := osWriteFile(filepath.Join(chunkDir, "std_20039_opendata.topics"), []byte("1\tgeneral\t[1]\t[x]\tTopic\n")); err != nil {
		t.Fatalf("write .topics file: %v", err)
	}
	if err := osWriteFile(filepath.Join(chunkDir, "std_20039_opendata.chunks"), []byte("overlap: []\nlines: [1]\n")); err != nil {
		t.Fatalf("write .chunks file: %v", err)
	}
	if err := osWriteFile(filepath.Join(chunkDir, "topics.txt"), []byte("1\tgeneral\t[9]\t[legacy]\tLegacy Topic\n")); err != nil {
		t.Fatalf("write legacy topics.txt: %v", err)
	}
	if err := osWriteFile(filepath.Join(chunkDir, "chunk_0001"), []byte("r 9\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLegacy line\n")); err != nil {
		t.Fatalf("write legacy chunk file: %v", err)
	}

	p := NewMetricsProcessor(&fakeDocMetadataStore{}, &fakeMetricsStore{}, &fakeJSONExtractor{}, nil)
	p.ChunkDir = tmp

	topicsPath, chunkFiles, err := p.detectChunkingInputs(recordID)
	if err != nil {
		t.Fatalf("detectChunkingInputs: %v", err)
	}
	if topicsPath != filepath.Join(chunkDir, "std_20039_opendata.topics") {
		t.Fatalf("topicsPath=%q", topicsPath)
	}
	if len(chunkFiles) != 0 {
		t.Fatalf("chunkFiles=%v, want none when .topics is present", chunkFiles)
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

func TestParseTopicLineRanges(t *testing.T) {
	cases := []struct {
		input string
		want  []int
	}{
		{"[1-3, 5, 7-9]", []int{1, 2, 3, 5, 7, 8, 9}},
		{"[10]", []int{10}},
		{"[]", nil},
		{"", nil},
		{"[42-42]", []int{42}},
	}
	for _, tc := range cases {
		got, err := parseTopicLineRanges(tc.input)
		if err != nil {
			t.Errorf("parseTopicLineRanges(%q): %v", tc.input, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseTopicLineRanges(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i, v := range got {
			if v != tc.want[i] {
				t.Errorf("parseTopicLineRanges(%q)[%d] = %d, want %d", tc.input, i, v, tc.want[i])
			}
		}
	}
}

func TestMetricsProcessor_ExtractsFromTopicsFile(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(3001)
	chunkDir := filepath.Join(tmp, "3", "3001")
	if err := osMkdirAll(chunkDir); err != nil {
		t.Fatalf("mkdir chunk dir: %v", err)
	}

	// Write topics.txt referencing lines 1 and 3
	topicsBody := "1\tgeneral\t[1, 3]\t[latency]\tLatency topic\n"
	if err := osWriteFile(filepath.Join(chunkDir, "topics.txt"), []byte(topicsBody)); err != nil {
		t.Fatalf("write topics.txt: %v", err)
	}

	// Write the original line file at the path ResolveInputFilePath will compute:
	// resultFilename=tmp/ocr_rslt_3001.json, parser=opendata, staging=tmp/ocr_rslt_3001.pdf
	// => tmp/ocr_rslt_3001_opendata.txt
	lineFileBody := strings.Join([]string{
		"1\t1\theading\tTestFont\t12\t[0,0,1,1]\tIntro",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tIgnored line",
		"3\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLatency must be <= 200ms",
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

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"3001","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if metricsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", metricsStore.saveCalled)
	}
	// Only lines 1 and 3 should be sent (line 2 not referenced in topics.txt)
	if strings.Contains(extractor.inputText, "Ignored line") {
		t.Fatalf("unreferenced line 2 must not be sent to LLM; input=%q", extractor.inputText)
	}
	if !strings.Contains(extractor.inputText, "Intro") {
		t.Fatalf("line 1 (Intro) must be sent to LLM; input=%q", extractor.inputText)
	}
	if !strings.Contains(extractor.inputText, "Latency must be") {
		t.Fatalf("line 3 must be sent to LLM; input=%q", extractor.inputText)
	}
}

func osWriteFile(path string, body []byte) error {
	return os.WriteFile(path, body, 0o644)
}

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
