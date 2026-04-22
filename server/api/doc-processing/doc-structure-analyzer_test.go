package docprocessing

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type fakeStructureExtractor struct {
	outs      []string
	err       error
	inputs    []string
	callCount int
}

func (f *fakeStructureExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	// Keep JSON path for backward compatibility fallback; tests for this file
	// use ExtractText.
	f.inputs = append(f.inputs, in.InputText)
	return map[string]any{}, nil
}

func (f *fakeStructureExtractor) ExtractText(_ context.Context, in llmclients.JSONExtractionInput) (string, error) {
	f.callCount++
	f.inputs = append(f.inputs, in.InputText)
	if f.err != nil {
		return "", f.err
	}
	if len(f.outs) == 0 {
		return "", nil
	}
	idx := f.callCount - 1
	if idx >= len(f.outs) {
		idx = len(f.outs) - 1
	}
	return f.outs[idx], nil
}

func TestStructureAnalyzer_SuccessWritesArtifactsAndStatus(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(7523)
	lineFile := filepath.Join(tmp, "ocr_rslt_7523_opendata.txt")
	body := strings.Join([]string{
		"109\t10\tparagraph\tHiddenHorzOCR\t11\t[160.76,484.42,238.69,499.932]\t3 基本规定",
		"110\t10\tparagraph\tHiddenHorzOCR\t11\t[160.76,520.42,238.69,539.932]\t3.1 范围",
	}, "\n")
	if err := os.WriteFile(lineFile, []byte(body), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}
	promptPath := filepath.Join(tmp, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("structure prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	modelsPath := filepath.Join(tmp, ".models.toml")
	if err := os.WriteFile(modelsPath, []byte(`
[structure-test]
host = "cloud"
model_name = "gpt-test"
api_key = "sk-test"
base_url = "https://api.openai.com"
timeout_sec = 120
`), 0o644); err != nil {
		t.Fatalf("write .models.toml: %v", err)
	}

	t.Setenv("STRUCTURE_DIR", tmp)
	t.Setenv("STRUCTURE_MODEL_NAME", "structure-test")
	t.Setenv("STRUCTURE_MODELS_FILE", modelsPath)
	t.Setenv("STRUCTURE_PROMPT", promptPath)
	t.Setenv("STRUCTURE_LLM_MAX_RETRIES", "2")

	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_7523.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_7523.pdf"),
		StatusRaw:       "[]",
	}}
	extractor := &fakeStructureExtractor{
		outs: []string{
			"cover_pages: [1]\n" +
				"109\tparagraph\theading-1\t0.94\tTop-level section marker.\n" +
				"110\tparagraph\theading-2\t0.91\tSubsection marker.",
		},
	}

	p := NewStructureAnalyzerProcessor(store, extractor, nil)
	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"7523"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if store.updateCalled != 1 {
		t.Fatalf("updateCalled=%d, want 1", store.updateCalled)
	}
	if store.updateReq.ErrorMsg != nil {
		t.Fatalf("unexpected error: %v", *store.updateReq.ErrorMsg)
	}
	var status []map[string]any
	if err := json.Unmarshal([]byte(store.updateReq.StatusRaw), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(status) != 1 {
		t.Fatalf("status len=%d", len(status))
	}
	if got := strings.TrimSpace(asString(status[0]["operation"])); got != "structure_analyzer" {
		t.Fatalf("operation=%q", got)
	}
	if got := strings.TrimSpace(asString(status[0]["proc_status"])); got != "success" {
		t.Fatalf("proc_status=%q", got)
	}

	runDir := filepath.Join(tmp, "7", "7523")
	labelPath := filepath.Join(runDir, "structure_labels.jsonl")
	summaryPath := filepath.Join(runDir, "structure_summary.json")

	if _, err := os.Stat(labelPath); err != nil {
		t.Fatalf("stat labels: %v", err)
	}
	if _, err := os.Stat(summaryPath); err != nil {
		t.Fatalf("stat summary: %v", err)
	}

	f, err := os.Open(labelPath)
	if err != nil {
		t.Fatalf("open labels: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	count := 0
	for sc.Scan() {
		count++
		var row map[string]any
		if err := json.Unmarshal([]byte(sc.Text()), &row); err != nil {
			t.Fatalf("labels row json: %v", err)
		}
		if strings.TrimSpace(asString(row["reason"])) == "" {
			t.Fatalf("reason should not be empty")
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan labels: %v", err)
	}
	if count != 2 {
		t.Fatalf("labels lines=%d, want 2", count)
	}
}

func TestStructureAnalyzer_RetryInvalidThenSuccess(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(1001)
	lineFile := filepath.Join(tmp, "ocr_rslt_1001_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\t3 基本规定\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}
	promptPath := filepath.Join(tmp, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("structure prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	modelsPath := filepath.Join(tmp, ".models.toml")
	if err := os.WriteFile(modelsPath, []byte(`
[structure-test]
host = "cloud"
model_name = "gpt-test"
api_key = "sk-test"
base_url = "https://api.openai.com"
timeout_sec = 120
`), 0o644); err != nil {
		t.Fatalf("write .models.toml: %v", err)
	}

	t.Setenv("STRUCTURE_DIR", tmp)
	t.Setenv("STRUCTURE_MODEL_NAME", "structure-test")
	t.Setenv("STRUCTURE_MODELS_FILE", modelsPath)
	t.Setenv("STRUCTURE_PROMPT", promptPath)
	t.Setenv("STRUCTURE_LLM_MAX_RETRIES", "2")

	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_1001.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_1001.pdf"),
		StatusRaw:       "[]",
	}}
	extractor := &fakeStructureExtractor{
		outs: []string{
			"cover_pages: [1]\n1\tparagraph\tparagraph\t0.8\tNo change.",
			"cover_pages: [1]\n1\tparagraph\theading-1\t0.8\tLooks like heading.",
		},
	}

	p := NewStructureAnalyzerProcessor(store, extractor, nil)
	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"1001"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.callCount != 2 {
		t.Fatalf("extractor calls=%d, want 2", extractor.callCount)
	}
	if len(extractor.inputs) < 2 || !strings.Contains(extractor.inputs[1], "schema issues") {
		t.Fatalf("retry prompt should include schema issues feedback")
	}
}

func TestStructureAnalyzer_UnchangedLinesAreNotRequiredFromLLMOutput(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(2222)
	lineFile := filepath.Join(tmp, "ocr_rslt_2222_opendata.txt")
	body := strings.Join([]string{
		"1\t1\tparagraph\tF\t12\t[1,1,2,2]\tAlpha",
		"2\t1\tparagraph\tF\t12\t[1,2,2,3]\tBeta",
	}, "\n")
	if err := os.WriteFile(lineFile, []byte(body), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}
	promptPath := filepath.Join(tmp, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("structure prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	modelsPath := filepath.Join(tmp, ".models.toml")
	if err := os.WriteFile(modelsPath, []byte(`
[structure-test]
host = "cloud"
model_name = "gpt-test"
api_key = "sk-test"
base_url = "https://api.openai.com"
timeout_sec = 120
`), 0o644); err != nil {
		t.Fatalf("write .models.toml: %v", err)
	}

	t.Setenv("STRUCTURE_DIR", tmp)
	t.Setenv("STRUCTURE_MODEL_NAME", "structure-test")
	t.Setenv("STRUCTURE_MODELS_FILE", modelsPath)
	t.Setenv("STRUCTURE_PROMPT", promptPath)

	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_2222.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_2222.pdf"),
		StatusRaw:       "[]",
	}}
	extractor := &fakeStructureExtractor{
		outs: []string{
			"cover_pages: []\n2\tparagraph\theading-1\t0.93\tSection-like heading.",
		},
	}

	p := NewStructureAnalyzerProcessor(store, extractor, nil)
	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"2222"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	runDir := filepath.Join(tmp, "2", "2222")
	labelPath := filepath.Join(runDir, "structure_labels.jsonl")
	bs, err := os.ReadFile(labelPath)
	if err != nil {
		t.Fatalf("read labels: %v", err)
	}
	rows := strings.Split(strings.TrimSpace(string(bs)), "\n")
	if len(rows) != 2 {
		t.Fatalf("labels rows=%d, want 2", len(rows))
	}
	var row1, row2 map[string]any
	if err := json.Unmarshal([]byte(rows[0]), &row1); err != nil {
		t.Fatalf("row1 json: %v", err)
	}
	if err := json.Unmarshal([]byte(rows[1]), &row2); err != nil {
		t.Fatalf("row2 json: %v", err)
	}
	if got := strings.TrimSpace(asString(row1["corrected_line_type"])); got != "paragraph" {
		t.Fatalf("line1 corrected_line_type=%q, want paragraph", got)
	}
	if got := strings.TrimSpace(asString(row1["reason"])); got != "unchanged" {
		t.Fatalf("line1 reason=%q, want unchanged", got)
	}
	if got := strings.TrimSpace(asString(row2["corrected_line_type"])); got != "heading-1" {
		t.Fatalf("line2 corrected_line_type=%q, want heading-1", got)
	}
}

func TestCanonicalOperationName_StructureAliases(t *testing.T) {
	got := canonicalOperationName("structure-analyzer")
	if got != "structure_analyzer" {
		t.Fatalf("canonical operation=%q", got)
	}
}

func TestStructureAnalyzer_UsesOverlappingPageBlocks(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(3333)
	lineFile := filepath.Join(tmp, "ocr_rslt_3333_opendata.txt")
	body := strings.Join([]string{
		"1\t1\tparagraph\tF\t12\t[1,1,2,2]\tPage one",
		"2\t2\tparagraph\tF\t12\t[1,2,2,3]\tPage two",
		"3\t3\tparagraph\tF\t12\t[1,3,2,4]\tPage three",
	}, "\n")
	if err := os.WriteFile(lineFile, []byte(body), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}
	promptPath := filepath.Join(tmp, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("structure prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	modelsPath := filepath.Join(tmp, ".models.toml")
	if err := os.WriteFile(modelsPath, []byte(`
[structure-test]
host = "cloud"
model_name = "gpt-test"
api_key = "sk-test"
base_url = "https://api.openai.com"
timeout_sec = 120
`), 0o644); err != nil {
		t.Fatalf("write .models.toml: %v", err)
	}

	t.Setenv("STRUCTURE_DIR", tmp)
	t.Setenv("STRUCTURE_MODEL_NAME", "structure-test")
	t.Setenv("STRUCTURE_MODELS_FILE", modelsPath)
	t.Setenv("STRUCTURE_PROMPT", promptPath)
	t.Setenv("INPUT_BLOCK_SIZE", "1")

	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_3333.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_3333.pdf"),
		StatusRaw:       "[]",
	}}
	extractor := &fakeStructureExtractor{
		outs: []string{
			"cover_pages: [1]\n1\tparagraph\theading-1\t0.95\tFirst page heading.",
			"cover_pages: [1]\n1\tparagraph\tlist-item\t0.60\tOverlap variant.\n2\tparagraph\theading-2\t0.90\tSecond page heading.",
			"cover_pages: []\n2\tparagraph\tlist-item\t0.60\tOverlap variant.\n3\tparagraph\theading-3\t0.92\tThird page heading.",
		},
	}

	p := NewStructureAnalyzerProcessor(store, extractor, nil)
	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"3333"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.callCount != 3 {
		t.Fatalf("extractor calls=%d, want 3", extractor.callCount)
	}

	runDir := filepath.Join(tmp, "3", "3333")
	labelPath := filepath.Join(runDir, "structure_labels.jsonl")
	bs, err := os.ReadFile(labelPath)
	if err != nil {
		t.Fatalf("read labels: %v", err)
	}
	rows := strings.Split(strings.TrimSpace(string(bs)), "\n")
	if len(rows) != 3 {
		t.Fatalf("labels rows=%d, want 3", len(rows))
	}

	gotTypes := make(map[float64]string, 3)
	for _, rowStr := range rows {
		var row map[string]any
		if err := json.Unmarshal([]byte(rowStr), &row); err != nil {
			t.Fatalf("labels row json: %v", err)
		}
		gotTypes[toFloat(row["line_number"])] = strings.TrimSpace(asString(row["corrected_line_type"]))
	}
	if got := gotTypes[1]; got != "heading-1" {
		t.Fatalf("line1 corrected_line_type=%q, want heading-1", got)
	}
	if got := gotTypes[2]; got != "heading-2" {
		t.Fatalf("line2 corrected_line_type=%q, want heading-2", got)
	}
	if got := gotTypes[3]; got != "heading-3" {
		t.Fatalf("line3 corrected_line_type=%q, want heading-3", got)
	}
}

func TestStructureAnalyzer_MissingModelFromModelsFile(t *testing.T) {
	tmp := t.TempDir()
	promptPath := filepath.Join(tmp, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("structure prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	modelsPath := filepath.Join(tmp, ".models.toml")
	if err := os.WriteFile(modelsPath, []byte(`
[other-model]
host = "cloud"
model_name = "gpt-other"
api_key = "sk-other"
base_url = "https://api.openai.com"
timeout_sec = 120
`), 0o644); err != nil {
		t.Fatalf("write .models.toml: %v", err)
	}

	t.Setenv("STRUCTURE_DIR", tmp)
	t.Setenv("STRUCTURE_MODEL_NAME", "missing-model")
	t.Setenv("STRUCTURE_MODELS_FILE", modelsPath)
	t.Setenv("STRUCTURE_PROMPT", promptPath)

	p := NewStructureAnalyzerProcessor(&fakeDocMetadataStore{}, &fakeStructureExtractor{}, nil)
	if err := p.validateRequiredEnv(); err != nil {
		t.Fatalf("validateRequiredEnv should pass when STRUCTURE_MODEL_NAME is set: %v", err)
	}
	if p.ModelErr == nil {
		t.Fatalf("expected ModelErr when STRUCTURE_MODEL_NAME is missing in .models.toml")
	}
	if !strings.Contains(p.ModelErr.Error(), "not found") {
		t.Fatalf("ModelErr=%v, want not found", p.ModelErr)
	}
}
