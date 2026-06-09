package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type fakeDocMetadataStore struct {
	// mu guards all fields. The doc pipeline writes from a background goroutine
	// (HandleJetStreamEvent) while tests read concurrently via accessors, so the
	// store must be synchronized to stay race-free under -race.
	mu           sync.Mutex
	rec          DocMetadataInputRecord
	getErr       error
	updateReq    DocMetadataUpdate
	updateReqs   []DocMetadataUpdate
	updateCalled int
}

func (f *fakeDocMetadataStore) GetInputRecord(_ context.Context, id int64) (DocMetadataInputRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return DocMetadataInputRecord{}, f.getErr
	}
	if id != f.rec.ID {
		return DocMetadataInputRecord{}, sql.ErrNoRows
	}
	return f.rec, nil
}

func (f *fakeDocMetadataStore) UpdateInputMetadata(_ context.Context, id int64, req DocMetadataUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != f.rec.ID {
		return errors.New("wrong id")
	}
	f.updateCalled++
	f.updateReq = req
	f.updateReqs = append(f.updateReqs, req)
	if strings.TrimSpace(req.StatusRaw) != "" {
		f.rec.StatusRaw = req.StatusRaw
	}
	return nil
}

// statusUpdates returns a snapshot of the recorded update requests under the
// lock, for tests that poll while the pipeline goroutine is still writing.
func (f *fakeDocMetadataStore) statusUpdates() []DocMetadataUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]DocMetadataUpdate, len(f.updateReqs))
	copy(out, f.updateReqs)
	return out
}

type fakeJSONExtractor struct {
	out                   map[string]any
	outs                  []map[string]any
	err                   error
	errs                  []error
	inputText             string
	inputTexts            []string
	modelNames            []string
	calledCount           int
	structuredCalledCount int
	contractNames         []string
}

func (f *fakeJSONExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	f.calledCount++
	f.inputText = in.InputText
	f.inputTexts = append(f.inputTexts, in.InputText)
	f.modelNames = append(f.modelNames, in.ModelName)
	if len(f.outs) > 0 || len(f.errs) > 0 {
		var out map[string]any
		var err error
		if len(f.outs) > 0 {
			out = f.outs[0]
			f.outs = f.outs[1:]
		}
		if len(f.errs) > 0 {
			err = f.errs[0]
			f.errs = f.errs[1:]
		}
		return out, err
	}
	return f.out, f.err
}

func (f *fakeJSONExtractor) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, contract llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	f.structuredCalledCount++
	f.contractNames = append(f.contractNames, contract.Name)
	f.inputText = in.InputText
	f.inputTexts = append(f.inputTexts, in.InputText)
	f.modelNames = append(f.modelNames, in.ModelName)
	if len(f.outs) > 0 || len(f.errs) > 0 {
		var out map[string]any
		var err error
		if len(f.outs) > 0 {
			out = f.outs[0]
			f.outs = f.outs[1:]
		}
		if len(f.errs) > 0 {
			err = f.errs[0]
			f.errs = f.errs[1:]
		}
		if err != nil {
			return nil, err
		}
		return &llmclients.StructuredOutputResult{Parsed: out}, nil
	}
	if f.err != nil {
		return nil, f.err
	}
	return &llmclients.StructuredOutputResult{Parsed: f.out}, nil
}

func TestParseLineFileGeneratedEvent(t *testing.T) {
	t.Run("supports string record id", func(t *testing.T) {
		evt, err := ParseLineFileGeneratedEvent([]byte(`{"record_id":"42"}`))
		if err != nil {
			t.Fatalf("ParseLineFileGeneratedEvent: %v", err)
		}
		if evt.RecordID != 42 {
			t.Fatalf("RecordID=%d, want 42", evt.RecordID)
		}
		if !evt.Force {
			t.Fatalf("default force should be true")
		}
	})

	t.Run("parses operations list", func(t *testing.T) {
		evt, err := ParseLineFileGeneratedEvent([]byte(`{"record_id":"42","operation":["chunking","extract-doc-metadata","extract_metrics"]}`))
		if err != nil {
			t.Fatalf("ParseLineFileGeneratedEvent: %v", err)
		}
		want := []string{"chunking", "extract_doc_metadata", "extract_metrics"}
		if len(evt.Operations) != len(want) {
			t.Fatalf("operations len=%d, want %d (%v)", len(evt.Operations), len(want), evt.Operations)
		}
		for i := range want {
			if evt.Operations[i] != want[i] {
				t.Fatalf("operations[%d]=%q, want %q", i, evt.Operations[i], want[i])
			}
		}
	})

	t.Run("uses line_file_filename when present", func(t *testing.T) {
		evt, err := ParseLineFileGeneratedEvent([]byte(`{"record_id":"42","line_file_filename":"/tmp/out/actual_opendata.txt"}`))
		if err != nil {
			t.Fatalf("ParseLineFileGeneratedEvent: %v", err)
		}
		if evt.Filename != "/tmp/out/actual_opendata.txt" {
			t.Fatalf("Filename=%q, want /tmp/out/actual_opendata.txt", evt.Filename)
		}
	})

	t.Run("invalid record id returns error", func(t *testing.T) {
		_, err := ParseLineFileGeneratedEvent([]byte(`{"record_id":"abc"}`))
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestResolveInputFilePath(t *testing.T) {
	got, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: 1}, "/tmp/out/ocr_rslt_101.json", "opendata", "/tmp/staging/source.pdf")
	if err != nil {
		t.Fatalf("ResolveInputFilePath default: %v", err)
	}
	want := "/tmp/out/source_opendata.txt"
	if got != want {
		t.Fatalf("path=%q, want %q", got, want)
	}

	got2, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: 1, Filename: "custom.txt"}, "/tmp/out/ocr_rslt_101.json", "opendata", "/tmp/staging/source.pdf")
	if err != nil {
		t.Fatalf("ResolveInputFilePath relative: %v", err)
	}
	if got2 != "/tmp/out/custom.txt" {
		t.Fatalf("path=%q, want %q", got2, "/tmp/out/custom.txt")
	}

	got2b, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: 1, Filename: "custom.origin"}, "/tmp/out/ocr_rslt_101.json", "opendata", "/tmp/staging/source.pdf")
	if err != nil {
		t.Fatalf("ResolveInputFilePath relative origin: %v", err)
	}
	if got2b != "/tmp/out/custom.txt" {
		t.Fatalf("path=%q, want %q", got2b, "/tmp/out/custom.txt")
	}

	_, err = ResolveInputFilePath(LineFileGeneratedEvent{RecordID: 1, Filename: "sub/rel.txt"}, "/tmp/out/ocr_rslt_101.json", "opendata", "/tmp/staging/source.pdf")
	if err == nil {
		t.Fatalf("expected relative path with separator to fail")
	}

	gotAbsOrigin, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: 1, Filename: "/tmp/out/custom.origin"}, "/tmp/out/ocr_rslt_101.json", "opendata", "/tmp/staging/source.pdf")
	if err != nil {
		t.Fatalf("ResolveInputFilePath absolute origin: %v", err)
	}
	if gotAbsOrigin != "/tmp/out/custom.txt" {
		t.Fatalf("path=%q, want %q", gotAbsOrigin, "/tmp/out/custom.txt")
	}

	t.Setenv("DATA_HOME_DIR", "/tmp/repo")
	got3, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: 1}, "Artifacts/0/101/source_opendata.json", "opendata", "source.pdf")
	if err != nil {
		t.Fatalf("ResolveInputFilePath relative result: %v", err)
	}
	if got3 != "/tmp/repo/Artifacts/0/101/source_opendata.txt" {
		t.Fatalf("path=%q, want %q", got3, "/tmp/repo/Artifacts/0/101/source_opendata.txt")
	}
}

func TestExtractDocMetadata_MissingParserNameUpdatesFailure(t *testing.T) {
	st := &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 9, ParserName: "", ResultFilename: "/tmp/x.json", StagingFilename: "/tmp/in/source.pdf", StatusRaw: "[]"}}
	ex := &fakeJSONExtractor{}
	svc := NewExtractDocMetadataProcessor(st, ex, nil)
	svc.ModelErr = nil

	err := svc.HandleEvent(context.Background(), []byte(`{"record_id":9}`))
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if st.updateCalled != 1 {
		t.Fatalf("updateCalled=%d, want 1", st.updateCalled)
	}
	if st.updateReq.ErrorMsg == nil || !strings.Contains(*st.updateReq.ErrorMsg, "missing parser name") {
		t.Fatalf("expected missing parser name error, got %+v", st.updateReq)
	}
}

func TestExtractDocMetadata_SuccessPersistsMetadata(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_8_opendata.txt")
	content := strings.Join([]string{
		"1	1	heading	TestFont	12	[0,0,1,1]	Document Title",
		"2	1	paragraph	TestFont	12	[0,0,1,1]	Some line",
		"3	2	paragraph	TestFont	12	[0,0,1,1]	Another page",
	}, "\n")
	if err := os.WriteFile(lineFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	st := &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 8, ParserName: "opendata", ResultFilename: filepath.Join(tmp, "ocr_rslt_8.json"), StagingFilename: filepath.Join(tmp, "ocr_rslt_8.pdf"), StatusRaw: "[]"}}
	ex := &fakeJSONExtractor{out: map[string]any{
		"title":        "Sample Standard",
		"doc_no":       "GB/T 123",
		"publish_date": "2024-01-01",
		"authors":      []any{"Org A", "Org B"},
		"language":     "zh-CN",
	}}
	svc := NewExtractDocMetadataProcessor(st, ex, nil)
	svc.ModelErr = nil
	svc.InitialPages = 1
	svc.PromptText = "extract metadata"
	svc.ModelName = "gpt-test"

	err := svc.HandleEvent(context.Background(), []byte(`{"record_id":8}`))
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if st.updateCalled != 1 {
		t.Fatalf("updateCalled=%d, want 1", st.updateCalled)
	}
	if st.updateReq.Title != "Sample Standard" {
		t.Fatalf("title=%q", st.updateReq.Title)
	}
	if len(st.updateReq.Authors) != 2 || st.updateReq.Authors[0] != "Org A" || st.updateReq.Authors[1] != "Org B" {
		t.Fatalf("authors=%v", st.updateReq.Authors)
	}
	if st.updateReq.ErrorMsg != nil {
		t.Fatalf("unexpected error msg: %v", *st.updateReq.ErrorMsg)
	}
	if got := asString(st.updateReq.DocMetadata["language"]); got != "zh" {
		t.Fatalf("doc metadata language=%q, want zh", got)
	}
	if ex.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", ex.structuredCalledCount)
	}
	if ex.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", ex.calledCount)
	}
	if !strings.Contains(ex.inputText, "1	1	heading	TestFont	12	[0,0,1,1]	Document Title") {
		t.Fatalf("extract input should include first page line")
	}
	if strings.Contains(ex.inputText, "3	2	paragraph	TestFont	12	[0,0,1,1]	Another page") {
		t.Fatalf("extract input should not include page 2 when InitialPages=1")
	}

	var statusArr []map[string]any
	if err := json.Unmarshal([]byte(st.updateReq.StatusRaw), &statusArr); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(statusArr) == 0 {
		t.Fatalf("expected status entries")
	}
	if op := strings.TrimSpace(asString(statusArr[0]["operation"])); op != "extract_metadata" {
		t.Fatalf("status operation=%q, want extract_metadata", op)
	}
}

func TestExtractDocMetadata_NormalizesNestedMetadataLanguage(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_18_opendata.txt")
	content := "1\t1\theading\tTestFont\t12\t[0,0,1,1]\t中文标准\n"
	if err := os.WriteFile(lineFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	st := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              18,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_18.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_18.pdf"),
		StatusRaw:       "[]",
	}}
	ex := &fakeJSONExtractor{out: map[string]any{
		"title": "中文标准",
		"metadata": map[string]any{
			"language": "中文",
		},
	}}
	svc := NewExtractDocMetadataProcessor(st, ex, nil)
	svc.ModelErr = nil
	svc.InitialPages = 1
	svc.PromptText = "extract metadata"

	if err := svc.HandleEvent(context.Background(), []byte(`{"record_id":18}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	metadata, ok := st.updateReq.DocMetadata["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("nested metadata=%T, want map[string]any", st.updateReq.DocMetadata["metadata"])
	}
	if got := asString(metadata["language"]); got != "zh" {
		t.Fatalf("nested metadata language=%q, want zh", got)
	}
}

func TestExtractDocMetadata_ExtractMetadataWithModelUsesStructuredContractWhenAvailable(t *testing.T) {
	ex := &fakeJSONExtractor{out: map[string]any{
		"title":        "Sample Standard",
		"doc_no":       "GB/T 123",
		"publish_date": "2024-01-01",
		"authors":      []any{"Org A"},
	}}
	svc := NewExtractDocMetadataProcessor(&fakeDocMetadataStore{}, ex, nil)
	svc.PromptText = "extract metadata"

	got, err := svc.extractMetadataWithModel(context.Background(), "input text", "docmeta-model", structureModelConfig{})
	if err != nil {
		t.Fatalf("extractMetadataWithModel: %v", err)
	}
	if ex.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", ex.structuredCalledCount)
	}
	if ex.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", ex.calledCount)
	}
	if len(ex.contractNames) != 1 || ex.contractNames[0] != "chenweb_doc_metadata_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_doc_metadata_extraction]", ex.contractNames)
	}
	if strings.TrimSpace(asString(got["title"])) != "Sample Standard" {
		t.Fatalf("title=%v, want Sample Standard", got["title"])
	}
}

func TestExtractDocMetadata_FallbackAuthorsFromMainDraftingPersons(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_9_opendata.txt")
	content := "1	1	heading	TestFont	12	[0,0,1,1]	Doc\n"
	if err := os.WriteFile(lineFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	st := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              9,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_9.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_9.pdf"),
		StatusRaw:       "[]",
	}}
	ex := &fakeJSONExtractor{out: map[string]any{
		"title":                 "Sample Standard",
		"doc_no":                "GB/T 456",
		"publish_date":          "2024-02-01",
		"authors":               []any{},
		"main_drafting_persons": []any{"Alice", "Bob"},
		"drafting_persons":      []any{"Carol"},
	}}
	svc := NewExtractDocMetadataProcessor(st, ex, nil)
	svc.ModelErr = nil
	svc.PromptText = "extract metadata"

	if err := svc.HandleEvent(context.Background(), []byte(`{"record_id":9}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(st.updateReq.Authors) != 2 || st.updateReq.Authors[0] != "Alice" || st.updateReq.Authors[1] != "Bob" {
		t.Fatalf("fallback authors=%v", st.updateReq.Authors)
	}
}

func TestExtractDocMetadata_PartialPublishDateFallsBackToMetadata(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_12_opendata.txt")
	content := "1\t1\theading\tTestFont\t12\t[0,0,1,1]\tDocument Title\n"
	if err := os.WriteFile(lineFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	st := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              12,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_12.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_12.pdf"),
		StatusRaw:       "[]",
	}}
	ex := &fakeJSONExtractor{out: map[string]any{
		"title":        "Year Only Document",
		"publish_date": "2009",
		"metadata": map[string]any{
			"language": "en",
		},
	}}
	svc := NewExtractDocMetadataProcessor(st, ex, nil)
	svc.ModelErr = nil
	svc.PromptText = "extract metadata"

	if err := svc.HandleEvent(context.Background(), []byte(`{"record_id":12}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if st.updateReq.PublishDate != "" {
		t.Fatalf("publishDate=%q, want empty for partial date", st.updateReq.PublishDate)
	}
	if got := asString(st.updateReq.DocMetadata["publish_date"]); got != "2009" {
		t.Fatalf("doc metadata publish_date=%q, want 2009", got)
	}
}

func TestNewExtractDocMetadataProcessor_UsesConfiguredLLMName(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[docmeta-test]
model_name = "gpt-5.4-mini"
base_url = "https://api.openai.com"
api_key = "test-key"
timeout_sec = 30
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EXTRACT_DOCMETA_MODEL_NAME", "docmeta-test")
	t.Setenv("EXTRACT_DOCMETA_MODELS_FILE", modelsPath)
	t.Setenv("EXTRACT_DOCMETA_PROMPT", "")

	svc := NewExtractDocMetadataProcessor(&fakeDocMetadataStore{}, &fakeJSONExtractor{}, nil)
	if svc.ModelErr != nil {
		t.Fatalf("ModelErr=%v", svc.ModelErr)
	}
	if svc.ModelRef != "docmeta-test" {
		t.Fatalf("ModelRef=%q, want docmeta-test", svc.ModelRef)
	}
	if svc.ModelName != "gpt-5.4-mini" {
		t.Fatalf("ModelName=%q, want gpt-5.4-mini", svc.ModelName)
	}
}

func TestNewExtractDocMetadataProcessor_LoadsFallbackModel(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[docmeta-primary]
model_name = "gemma4:26b"
base_url = "http://127.0.0.1:11434"
api_key = "ollama"
timeout_sec = 30

[docmeta-fallback]
model_name = "gpt-5.4-mini"
base_url = "https://api.openai.com"
api_key = "test-key"
timeout_sec = 60
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EXTRACT_DOCMETA_MODEL_NAME", "docmeta-primary")
	t.Setenv("EXTRACT_DOCMETA_MODEL_FALLBACK", "docmeta-fallback")
	t.Setenv("EXTRACT_DOCMETA_MODELS_FILE", modelsPath)
	t.Setenv("EXTRACT_DOCMETA_PROMPT", "")

	svc := NewExtractDocMetadataProcessor(&fakeDocMetadataStore{}, &fakeJSONExtractor{}, nil)
	if svc.ModelErr != nil {
		t.Fatalf("ModelErr=%v", svc.ModelErr)
	}
	if svc.FallbackModelErr != nil {
		t.Fatalf("FallbackModelErr=%v", svc.FallbackModelErr)
	}
	if svc.FallbackModelRef != "docmeta-fallback" {
		t.Fatalf("FallbackModelRef=%q, want docmeta-fallback", svc.FallbackModelRef)
	}
	if svc.FallbackModelName != "gpt-5.4-mini" {
		t.Fatalf("FallbackModelName=%q, want gpt-5.4-mini", svc.FallbackModelName)
	}
}

func TestExtractDocMetadata_PrimaryFailureRetriesFallbackModel(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_10_opendata.txt")
	content := "1\t1\theading\tTestFont\t12\t[0,0,1,1]\tDocument Title\n"
	if err := os.WriteFile(lineFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	st := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              10,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_10.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_10.pdf"),
		StatusRaw:       "[]",
	}}
	ex := &fakeJSONExtractor{
		errs: []error{
			errors.New("primary timeout"),
			nil,
		},
		outs: []map[string]any{
			nil,
			{
				"title":        "Fallback Title",
				"doc_no":       "GB/T 789",
				"publish_date": "2024-03-01",
				"authors":      []any{"Fallback Org"},
			},
		},
	}
	svc := NewExtractDocMetadataProcessor(st, ex, nil)
	svc.ModelErr = nil
	svc.ModelName = "gemma4:26b"
	svc.FallbackModelName = "gpt-5.4-mini"
	svc.PromptText = "extract metadata"

	if err := svc.HandleEvent(context.Background(), []byte(`{"record_id":10}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if ex.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2", ex.structuredCalledCount)
	}
	if ex.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", ex.calledCount)
	}
	if len(ex.modelNames) != 2 {
		t.Fatalf("modelNames=%v", ex.modelNames)
	}
	if ex.modelNames[0] != "gemma4:26b" {
		t.Fatalf("primary model=%q", ex.modelNames[0])
	}
	if ex.modelNames[1] != "gpt-5.4-mini" {
		t.Fatalf("fallback model=%q", ex.modelNames[1])
	}
	if st.updateReq.Title != "Fallback Title" {
		t.Fatalf("title=%q", st.updateReq.Title)
	}
	if st.updateReq.ErrorMsg != nil {
		t.Fatalf("unexpected error msg: %v", *st.updateReq.ErrorMsg)
	}
}

func TestExtractDocMetadata_FallbackEmptyJSONIsWarning(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_11_opendata.txt")
	content := "1\t1\theading\tTestFont\t12\t[0,0,1,1]\tDocument Title\n"
	if err := os.WriteFile(lineFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	st := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              11,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_11.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_11.pdf"),
		StatusRaw:       "[]",
	}}
	ex := &fakeJSONExtractor{
		errs: []error{
			errors.New("primary timeout"),
			errors.New("(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}, model name:deepseek-v4-pro, prompt file:prompt_extract_doc_metadata_v1.txt"),
		},
	}
	svc := NewExtractDocMetadataProcessor(st, ex, nil)
	svc.ModelErr = nil
	svc.ModelName = "gemma4:26b"
	svc.FallbackModelName = "deepseek-v4-pro"
	svc.PromptText = "extract metadata"

	if err := svc.HandleEvent(context.Background(), []byte(`{"record_id":11}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if ex.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2", ex.structuredCalledCount)
	}
	if ex.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", ex.calledCount)
	}
	if st.updateCalled != 1 {
		t.Fatalf("updateCalled=%d, want 1", st.updateCalled)
	}
	if st.updateReq.ErrorMsg != nil {
		t.Fatalf("unexpected error msg: %v", *st.updateReq.ErrorMsg)
	}
	if st.updateReq.Title != "" {
		t.Fatalf("title=%q, want empty", st.updateReq.Title)
	}
	if st.updateReq.DocNo != "" {
		t.Fatalf("docNo=%q, want empty", st.updateReq.DocNo)
	}
	if len(st.updateReq.Authors) != 0 {
		t.Fatalf("authors=%v, want empty", st.updateReq.Authors)
	}
	if len(st.updateReq.DocMetadata) != 0 {
		t.Fatalf("docMetadata=%v, want empty", st.updateReq.DocMetadata)
	}
}
