package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type fakeDocMetadataStore struct {
	rec          DocMetadataInputRecord
	getErr       error
	updateReq    DocMetadataUpdate
	updateCalled int
}

func (f *fakeDocMetadataStore) GetInputRecord(_ context.Context, id int64) (DocMetadataInputRecord, error) {
	if f.getErr != nil {
		return DocMetadataInputRecord{}, f.getErr
	}
	if id != f.rec.ID {
		return DocMetadataInputRecord{}, sql.ErrNoRows
	}
	return f.rec, nil
}

func (f *fakeDocMetadataStore) UpdateInputMetadata(_ context.Context, id int64, req DocMetadataUpdate) error {
	if id != f.rec.ID {
		return errors.New("wrong id")
	}
	f.updateCalled++
	f.updateReq = req
	return nil
}

type fakeJSONExtractor struct {
	out         map[string]any
	err         error
	inputText   string
	calledCount int
}

func (f *fakeJSONExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	f.calledCount++
	f.inputText = in.InputText
	return f.out, f.err
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

	_, err = ResolveInputFilePath(LineFileGeneratedEvent{RecordID: 1, Filename: "sub/rel.txt"}, "/tmp/out/ocr_rslt_101.json", "opendata", "/tmp/staging/source.pdf")
	if err == nil {
		t.Fatalf("expected relative path with separator to fail")
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
	if ex.calledCount != 1 {
		t.Fatalf("extract called=%d, want 1", ex.calledCount)
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

	t.Setenv("EXTRACT_DOCMETA_LLM_NAME", "docmeta-test")
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
