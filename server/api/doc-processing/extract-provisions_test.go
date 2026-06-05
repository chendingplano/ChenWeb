package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeProvisionsStore struct {
	exists       bool
	existsErr    error
	saveErr      error
	deleteErr    error
	deleteCalled int
	saveCalled   int
	existCalled  int
	lastSave     SaveProvisionsRequest
}

func (f *fakeProvisionsStore) ProvisionsExist(_ context.Context, _ int64, _ string) (bool, error) {
	f.existCalled++
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists, nil
}

func (f *fakeProvisionsStore) DeleteProvisionsByInputRecordID(_ context.Context, _ int64) (int64, error) {
	f.deleteCalled++
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	return 0, nil
}

func (f *fakeProvisionsStore) SaveProvisions(_ context.Context, req SaveProvisionsRequest) (int64, error) {
	f.saveCalled++
	f.lastSave = req
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	return int64(len(req.Provisions)), nil
}

func TestProvisionsProcessor_ExtractsFromBlockBufferAndWritesStatus(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	t.Setenv("ARTIFACT_DIR", tmp)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              4001,
		ParserName:      "opendata",
		ResultFilename:  "/tmp/std-4001.json",
		StagingFilename: "/tmp/std-4001.pdf",
		StatusRaw:       "[]",
	}}
	provisionsStore := &fakeProvisionsStore{}
	extractor := &fakeJSONExtractor{out: map[string]any{
		"language": "en",
		"provisions": []any{
			map[string]any{
				"name":              "inspection_requirement",
				"type":              "mandatory",
				"provision":         "The operator shall inspect pressure relief valves monthly.",
				"provision_en":      "The operator shall inspect pressure relief valves monthly.",
				"source_line_spans": []any{float64(10), float64(11)},
				"context":           "maintenance section",
				"subject":           "pressure relief valve inspection",
				"location_type":     "sentence",
				"keywords":          []any{"inspection", "pressure relief valve"},
				"confidence":        0.91,
				"is_explicit":       true,
				"need_verify":       false,
				"category_paths": []any{
					map[string]any{
						"category_path": []any{
							map[string]any{"name": "safety", "keywords": []any{"safety"}, "confidence": float64(0.90)},
							map[string]any{"name": "pressure_relief", "keywords": []any{"pressure", "valve"}, "confidence": float64(0.88)},
						},
						"path_keywords":   []any{"valve inspection"},
						"path_confidence": float64(0.88),
					},
				},
			},
		},
	}}

	ctx, h := withBlockBufferHolder(context.Background())
	h.mu.Lock()
	h.buffer = &BlockBuffer{Blocks: []Block{{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 10, PageNumber: 2, LineType: "paragraph", Content: "The operator shall inspect pressure relief valves monthly."},
			{Flag: "o", LineNumber: 11, PageNumber: 3, LineType: "paragraph", Content: "Overlap context."},
		},
	}}}
	h.mu.Unlock()

	p := NewProvisionsProcessor(inputStore, provisionsStore, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "gpt-test"
	p.ExtractProvisionsInput = "blocks"

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"4001","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if !strings.Contains(extractor.inputText, `"flag":"n"`) ||
		!strings.Contains(extractor.inputText, `"line_number":10`) ||
		!strings.Contains(extractor.inputText, `"page_number":2`) ||
		!strings.Contains(extractor.inputText, `"content":"The operator shall inspect pressure relief valves monthly."`) {
		t.Fatalf("block line not sent to LLM input: %q", extractor.inputText)
	}
	if provisionsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", provisionsStore.saveCalled)
	}
	if provisionsStore.lastSave.ExtractID == "" {
		t.Fatalf("ExtractID should be set")
	}
	if got := int(toFloat(provisionsStore.lastSave.Provisions[0]["prov_id"])); got != 1 {
		t.Fatalf("prov_id=%v, want 1", got)
	}
	if got := provisionsStore.lastSave.Provisions[0]["prov_name"]; got != "inspection_requirement" {
		t.Fatalf("prov_name=%v", got)
	}
	if got := provisionsStore.lastSave.Provisions[0]["provision_subject"]; got != "pressure relief valve inspection" {
		t.Fatalf("provision_subject=%v", got)
	}
	if got := provisionsStore.lastSave.Provisions[0]["source_line_spans"]; len(got.([]string)) != 1 || got.([]string)[0] != "10-11" {
		t.Fatalf("source_line_spans=%v", got)
	}
	if got := strings.TrimSpace(asString(provisionsStore.lastSave.Provisions[0]["source_text"])); got != "The operator shall inspect pressure relief valves monthly." {
		t.Fatalf("source_text=%q", got)
	}
	dbRow := buildProvisionDBRecord(provisionsStore.lastSave.Provisions[0], provisionsStore.lastSave.Language)
	if got := dbRow["provision_type"]; got != "mandatory" {
		t.Fatalf("db provision_type=%v", got)
	}
	if got := dbRow["source_text"]; got != "The operator shall inspect pressure relief valves monthly." {
		t.Fatalf("db source_text=%v", got)
	}
	if got := dbRow["provision"]; got != nil {
		t.Fatalf("db provision=%v, want nil for English", got)
	}
	if got := dbRow["provision_en"]; got != "The operator shall inspect pressure relief valves monthly." {
		t.Fatalf("db provision_en=%v", got)
	}
	if got := dbRow["provision_subject"]; got != "pressure relief valve inspection" {
		t.Fatalf("db provision_subject=%v", got)
	}
	if got := dbRow["provision_keywords"]; len(got.([]string)) != 2 {
		t.Fatalf("db provision_keywords=%v", got)
	}
	if got := toFloat(dbRow["confidence"]); got != 0.91 {
		t.Fatalf("db confidence=%v", got)
	}
	if got := toBool(dbRow["need_verify"]); got {
		t.Fatalf("db need_verify=%v, want false", got)
	}
	if _, ok := dbRow["prov_subject"]; ok {
		t.Fatalf("db row should not include duplicate prov_subject")
	}
	if _, ok := dbRow["prov_keywords"]; ok {
		t.Fatalf("db row should not include duplicate prov_keywords")
	}
	if got := int(toFloat(dbRow["num_blocks"])); got != 1 {
		t.Fatalf("db num_blocks=%v, want 1", got)
	}
	if got := int(toFloat(dbRow["num_provisions"])); got != 1 {
		t.Fatalf("db num_provisions=%v, want 1", got)
	}
	if _, ok := dbRow["time_per_provision"]; !ok {
		t.Fatalf("db row missing time_per_provision")
	}
	if got := strings.TrimSpace(asString(dbRow["model_name"])); got != "gpt-test" {
		t.Fatalf("db model_name=%q, want gpt-test", got)
	}
	if got := strings.TrimSpace(asString(dbRow["prompt_name"])); got != "prompt-test" {
		t.Fatalf("db prompt_name=%q, want prompt-test", got)
	}
	provisionsPath := filepath.Join(tmp, "safety", "pressure_relief", "provisions.txt")
	body, err := os.ReadFile(provisionsPath)
	if err != nil {
		t.Fatalf("read provisions.txt: %v", err)
	}
	if got := strings.TrimSpace(string(body)); got != "4001_1" {
		t.Fatalf("provisions.txt=%q, want 4001_1", got)
	}

	artifactPath := filepath.Join(tmp, "4", "4001", "std-4001_opendata.provisions")
	artifactBody, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read .provisions artifact: %v", err)
	}
	var artifactRecords []map[string]any
	if err := json.Unmarshal(artifactBody, &artifactRecords); err != nil {
		t.Fatalf("parse .provisions artifact: %v", err)
	}
	if len(artifactRecords) != 1 {
		t.Fatalf(".provisions record count=%d, want 1", len(artifactRecords))
	}
	if got := int(toFloat(artifactRecords[0]["prov_id"])); got != 1 {
		t.Fatalf(".provisions prov_id=%v, want 1", got)
	}
	if got := strings.TrimSpace(asString(artifactRecords[0]["prov_type"])); got != "mandatory" {
		t.Fatalf(".provisions prov_type=%v, want mandatory", got)
	}
	if got := strings.TrimSpace(asString(artifactRecords[0]["subject"])); got != "pressure relief valve inspection" {
		t.Fatalf(".provisions subject=%v", got)
	}

	var statusArr []map[string]any
	if err := json.Unmarshal([]byte(inputStore.updateReq.StatusRaw), &statusArr); err != nil {
		t.Fatalf("status json: %v", err)
	}
	last := statusArr[len(statusArr)-1]
	if strings.TrimSpace(asString(last["operation"])) != "extract_provisions" {
		t.Fatalf("operation=%v", last["operation"])
	}
	if strings.TrimSpace(asString(last["proc_status"])) != "success" {
		t.Fatalf("proc_status=%v", last["proc_status"])
	}
	if last["record_id"] != "4001" {
		t.Fatalf("record_id=%v, want 4001", last["record_id"])
	}
	if last["file_type"] != "pdf" {
		t.Fatalf("file_type=%v, want pdf", last["file_type"])
	}
	if strings.TrimSpace(asString(last["input_filename"])) == "" {
		t.Fatalf("input_filename should be set")
	}
	if _, ok := last["ms_used"]; !ok {
		t.Fatalf("ms_used should be present")
	}
	if _, ok := last["ms-used"]; ok {
		t.Fatalf("ms-used should not be present (renamed to ms_used)")
	}
	if _, ok := last["proc-status"]; ok {
		t.Fatalf("proc-status should not be present (use proc_status only)")
	}
}

func TestBuildProvisionDBRecord_NonEnglishPreservesOriginal(t *testing.T) {
	row := map[string]any{
		"provision_subject":  "fire fighter physical examination",
		"prov_context":       "Section 4.1.1",
		"provision_keywords": []string{"消防员", "体格检查"},
		"confidence":         0.95,
		"source_text":        "100 消防员体格检查应符合下列标准:",
		"source_line_spans":  []string{"100"},
		"num_blocks":         2,
		"num_provisions":     3,
		"time_per_provision": int64(12),
		"model_name":         "gpt-test",
		"prompt_name":        "prompt-test",
		"provision":          "消防员体格检查应符合下列标准:",
		"provision_en":       "Fire fighter physical examination shall meet the following standards:",
		"need_verify":        false,
		"public_info": map[string]any{
			"provision_type": "mandatory",
		},
	}

	dbRow := buildProvisionDBRecord(row, "zh")
	if got := dbRow["provision"]; got != "消防员体格检查应符合下列标准:" {
		t.Fatalf("provision=%v", got)
	}
	if got := dbRow["provision_en"]; got != "Fire fighter physical examination shall meet the following standards:" {
		t.Fatalf("provision_en=%v", got)
	}
}

func TestProvisionsProcessor_FallsBackToInputFileWhenBlockBufferMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	t.Setenv("ARTIFACT_DIR", tmp)
	lineFile := filepath.Join(tmp, "std_5001_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tArial\t11\t[0,0,1,1]\tThe device must log alarms.\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              5001,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "std_5001.json"),
		StagingFilename: filepath.Join(tmp, "std_5001.pdf"),
		StatusRaw:       "[]",
	}}
	provisionsStore := &fakeProvisionsStore{}
	extractor := &fakeJSONExtractor{out: map[string]any{
		"language":   "en",
		"provisions": []any{},
	}}

	p := NewProvisionsProcessor(inputStore, provisionsStore, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "gpt-test"
	p.ExtractProvisionsInput = "blocks"

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"5001","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if !strings.Contains(extractor.inputText, `"flag":"n"`) ||
		!strings.Contains(extractor.inputText, `"line_number":1`) ||
		!strings.Contains(extractor.inputText, `"page_number":1`) ||
		!strings.Contains(extractor.inputText, `"content":"The device must log alarms."`) {
		t.Fatalf("fallback block input not built from line file: %q", extractor.inputText)
	}
}

func TestProvisionsProcessor_AcceptsSingleProvisionObject(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	t.Setenv("ARTIFACT_DIR", tmp)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              6000,
		ParserName:      "opendata",
		ResultFilename:  "/tmp/std-6000.json",
		StagingFilename: "/tmp/std-6000.pdf",
		StatusRaw:       "[]",
	}}
	provisionsStore := &fakeProvisionsStore{}
	extractor := &fakeJSONExtractor{out: map[string]any{
		"language": "zh",
		"provisions": map[string]any{
			"name":              "terminal_radiation_requirement",
			"type":              "mandatory",
			"provision":         "终端设备中密封源的质量应符合GB4075。",
			"provision_en":      "The quality of sealed sources in terminal equipment shall comply with GB4075.",
			"source_line_spans": []any{"181"},
			"subject":           "医疗健康监测终端设备辐射安全要求",
			"keywords":          []any{"终端辐射", "密封源", "GB4075"},
			"confidence":        0.95,
			"is_explicit":       true,
			"need_verify":       false,
			"category_paths":    []any{},
		},
	}}

	ctx, h := withBlockBufferHolder(context.Background())
	h.mu.Lock()
	h.buffer = &BlockBuffer{Blocks: []Block{{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 181, PageNumber: 8, LineType: "paragraph", Content: "终端设备中密封源的质量应符合GB4075。"},
		},
	}}}
	h.mu.Unlock()

	p := NewProvisionsProcessor(inputStore, provisionsStore, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "primary-model"
	p.ExtractProvisionsInput = "blocks"

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"6000","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if provisionsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", provisionsStore.saveCalled)
	}
	if len(provisionsStore.lastSave.Provisions) != 1 {
		t.Fatalf("saved provisions=%d, want 1", len(provisionsStore.lastSave.Provisions))
	}
	if got := provisionsStore.lastSave.Provisions[0]["prov_name"]; got != "terminal_radiation_requirement" {
		t.Fatalf("prov_name=%v", got)
	}
}

func TestProvisionsProcessor_AcceptsBareProvisionObject(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	t.Setenv("ARTIFACT_DIR", tmp)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              6004,
		ParserName:      "opendata",
		ResultFilename:  "/tmp/std-6004.json",
		StagingFilename: "/tmp/std-6004.pdf",
		StatusRaw:       "[]",
	}}
	provisionsStore := &fakeProvisionsStore{}
	extractor := &fakeJSONExtractor{out: map[string]any{
		"name":              "terminal_radiation_requirement",
		"type":              "mandatory",
		"provision":         "终端设备中密封源的质量应符合GB4075。",
		"provision_en":      "The quality of sealed sources in terminal equipment shall comply with GB4075.",
		"source_line_spans": []any{"181"},
		"subject":           "医疗健康监测终端设备辐射安全要求",
		"keywords":          []any{"终端辐射", "密封源", "GB4075"},
		"confidence":        0.95,
		"is_explicit":       true,
		"need_verify":       false,
		"category_paths":    []any{},
	}}

	ctx, h := withBlockBufferHolder(context.Background())
	h.mu.Lock()
	h.buffer = &BlockBuffer{Blocks: []Block{{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 181, PageNumber: 8, LineType: "paragraph", Content: "终端设备中密封源的质量应符合GB4075。"},
		},
	}}}
	h.mu.Unlock()

	p := NewProvisionsProcessor(inputStore, provisionsStore, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "primary-model"
	p.ExtractProvisionsInput = "blocks"

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"6004","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if provisionsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", provisionsStore.saveCalled)
	}
	if len(provisionsStore.lastSave.Provisions) != 1 {
		t.Fatalf("saved provisions=%d, want 1", len(provisionsStore.lastSave.Provisions))
	}
	if got := provisionsStore.lastSave.Provisions[0]["prov_name"]; got != "terminal_radiation_requirement" {
		t.Fatalf("prov_name=%v", got)
	}
}

func TestProvisionsProcessor_RetriesWithCallbackModelOnEmptyJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	t.Setenv("ARTIFACT_DIR", tmp)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              6001,
		ParserName:      "opendata",
		ResultFilename:  "/tmp/std-6001.json",
		StagingFilename: "/tmp/std-6001.pdf",
		StatusRaw:       "[]",
	}}
	provisionsStore := &fakeProvisionsStore{}
	extractor := &fakeJSONExtractor{
		outs: []map[string]any{
			{},
			{
				"language": "en",
				"provisions": []any{
					map[string]any{
						"name":              "logging_requirement",
						"type":              "mandatory",
						"provision":         "The device shall log all alarms.",
						"provision_en":      "The device shall log all alarms.",
						"source_line_spans": []any{"10"},
						"confidence":        0.87,
					},
				},
			},
		},
	}

	ctx, h := withBlockBufferHolder(context.Background())
	h.mu.Lock()
	h.buffer = &BlockBuffer{Blocks: []Block{{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 10, PageNumber: 2, LineType: "paragraph", Content: "The device shall log all alarms."},
		},
	}}}
	h.mu.Unlock()

	p := NewProvisionsProcessor(inputStore, provisionsStore, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "primary-model"
	p.FallbackModelName = "fallback-model"
	p.ExtractProvisionsInput = "blocks"

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"6001","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if len(extractor.modelNames) != 2 {
		t.Fatalf("modelNames=%v", extractor.modelNames)
	}
	if extractor.modelNames[0] != "primary-model" {
		t.Fatalf("first model=%q, want primary-model", extractor.modelNames[0])
	}
	if extractor.modelNames[1] != "fallback-model" {
		t.Fatalf("second model=%q, want fallback-model", extractor.modelNames[1])
	}
	if provisionsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", provisionsStore.saveCalled)
	}
	if got := provisionsStore.lastSave.Provisions[0]["prov_name"]; got != "logging_requirement" {
		t.Fatalf("prov_name=%v", got)
	}
}

func TestProvisionsProcessor_FailsOnEmptyJSONWithoutCallbackModel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	t.Setenv("ARTIFACT_DIR", tmp)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              6002,
		ParserName:      "opendata",
		ResultFilename:  "/tmp/std-6002.json",
		StagingFilename: "/tmp/std-6002.pdf",
		StatusRaw:       "[]",
	}}
	provisionsStore := &fakeProvisionsStore{}
	extractor := &fakeJSONExtractor{out: map[string]any{}}

	ctx, h := withBlockBufferHolder(context.Background())
	h.mu.Lock()
	h.buffer = &BlockBuffer{Blocks: []Block{{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 10, PageNumber: 2, LineType: "paragraph", Content: "The device shall log all alarms."},
		},
	}}}
	h.mu.Unlock()

	p := NewProvisionsProcessor(inputStore, provisionsStore, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "primary-model"
	p.ExtractProvisionsInput = "blocks"

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"6002","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if provisionsStore.saveCalled != 0 {
		t.Fatalf("saveCalled=%d, want 0", provisionsStore.saveCalled)
	}
	if inputStore.updateReq.ErrorMsg == nil || !strings.Contains(*inputStore.updateReq.ErrorMsg, "empty llm json object") {
		t.Fatalf("ErrorMsg=%v, want empty-json message", inputStore.updateReq.ErrorMsg)
	}
}

func TestProvisionsProcessor_RetriesWithCallbackModelOnExtractorError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	t.Setenv("ARTIFACT_DIR", tmp)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              6003,
		ParserName:      "opendata",
		ResultFilename:  "/tmp/std-6003.json",
		StagingFilename: "/tmp/std-6003.pdf",
		StatusRaw:       "[]",
	}}
	provisionsStore := &fakeProvisionsStore{}
	extractor := &fakeJSONExtractor{
		errs: []error{
			errors.New("primary model failed"),
			nil,
		},
		outs: []map[string]any{
			nil,
			{
				"language":   "en",
				"provisions": []any{},
			},
		},
	}

	ctx, h := withBlockBufferHolder(context.Background())
	h.mu.Lock()
	h.buffer = &BlockBuffer{Blocks: []Block{{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 10, PageNumber: 2, LineType: "paragraph", Content: "The device shall log all alarms."},
		},
	}}}
	h.mu.Unlock()

	p := NewProvisionsProcessor(inputStore, provisionsStore, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "primary-model"
	p.FallbackModelName = "fallback-model"
	p.ExtractProvisionsInput = "blocks"

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"6003","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if extractor.modelNames[1] != "fallback-model" {
		t.Fatalf("second model=%q, want fallback-model", extractor.modelNames[1])
	}
	if provisionsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", provisionsStore.saveCalled)
	}
}

func TestProvisionsProcessor_TreatsEmptyFallbackJSONAsSuccess(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	t.Setenv("ARTIFACT_DIR", tmp)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              6004,
		ParserName:      "opendata",
		ResultFilename:  "/tmp/std-6004.json",
		StagingFilename: "/tmp/std-6004.pdf",
		StatusRaw:       "[]",
	}}
	provisionsStore := &fakeProvisionsStore{}
	extractor := &fakeJSONExtractor{
		errs: []error{
			errors.New("primary model failed"),
			errors.New("(MID_26050841) failed extracting provisions, error:(MID_26050174) failed resolveScopedString, error:(MID_26052920) failed resolveScopedString, error:(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}"),
		},
	}

	ctx, h := withBlockBufferHolder(context.Background())
	h.mu.Lock()
	h.buffer = &BlockBuffer{Blocks: []Block{{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 10, PageNumber: 2, LineType: "paragraph", Content: "The device shall log all alarms."},
		},
	}}}
	h.mu.Unlock()

	p := NewProvisionsProcessor(inputStore, provisionsStore, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "primary-model"
	p.FallbackModelName = "fallback-model"
	p.ExtractProvisionsInput = "blocks"

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"6004","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if provisionsStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", provisionsStore.saveCalled)
	}
	if len(provisionsStore.lastSave.Provisions) != 0 {
		t.Fatalf("saved provisions=%d, want 0", len(provisionsStore.lastSave.Provisions))
	}
	if got := strings.TrimSpace(provisionsStore.lastSave.Language); got != "unknown" {
		t.Fatalf("language=%q, want unknown", got)
	}

	var statusArr []map[string]any
	if err := json.Unmarshal([]byte(inputStore.updateReq.StatusRaw), &statusArr); err != nil {
		t.Fatalf("status json: %v", err)
	}
	last := statusArr[len(statusArr)-1]
	if strings.TrimSpace(asString(last["proc_status"])) != "success" {
		t.Fatalf("proc_status=%v, want success", last["proc_status"])
	}
}

func TestExtractProvisionPayloadWithFallback_EmptyPrimaryResponseRetriesFallback(t *testing.T) {
	extractor := &fakeJSONExtractor{
		errs: []error{
			errors.New("(MID_26050841) failed extracting provisions, error:(MID_26050174) failed resolveScopedString, error:(MID_26052921) failed resolveScopedString, error:(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}"),
		},
		outs: []map[string]any{
			nil,
			{
				"language": "en",
				"provisions": []any{
					map[string]any{"provision": "The device shall log all alarms."},
				},
			},
		},
	}

	p := NewProvisionsProcessor(nil, nil, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.ModelName = "deepseek-v4-flash"
	p.FallbackModelName = "gpt-5.4-mini"

	payload, modelName, err := p.extractProvisionPayloadWithFallback(context.Background(), buildProvisionUserPrompt(Block{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "The device shall log all alarms."},
		},
	}))
	if err != nil {
		t.Fatalf("extractProvisionPayloadWithFallback: %v", err)
	}
	if extractor.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if modelName != "gpt-5.4-mini" {
		t.Fatalf("modelName=%q, want gpt-5.4-mini", modelName)
	}
	if got := strings.TrimSpace(asString(payload["language"])); got != "en" {
		t.Fatalf("language=%q, want en", got)
	}
	if got := payload["provisions"]; got == nil {
		t.Fatalf("provisions missing from payload: %#v", payload)
	}
}

func TestProvisionsProcessor_ExtractProvisionPayloadUsesStructuredContractWhenAvailable(t *testing.T) {
	extractor := &fakeJSONExtractor{out: map[string]any{
		"language": "en",
		"provisions": []any{
			map[string]any{
				"provision": "The device shall log all alarms.",
			},
		},
	}}

	p := NewProvisionsProcessor(nil, nil, extractor, nil)
	p.PromptText = "extract provisions"
	p.PromptRef = "prompt-test"
	p.ModelName = "gpt-test"

	payload, err := p.extractProvisionPayloadFromText(context.Background(), buildProvisionUserPrompt(Block{
		Index: 1,
		Lines: []BlockLine{
			{Flag: "n", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "The device shall log all alarms."},
		},
	}), "gpt-test", extractor)
	if err != nil {
		t.Fatalf("extractProvisionPayloadFromText: %v", err)
	}
	if extractor.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if len(extractor.contractNames) != 1 || extractor.contractNames[0] != "chenweb_provision_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_provision_extraction]", extractor.contractNames)
	}
	items, ok := payload["provisions"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("provisions=%#v", payload["provisions"])
	}
}

func TestNormalizeProvisionSourceLineSpans_UsesCanonicalLineOnlyRanges(t *testing.T) {
	got := normalizeProvisionSourceLineSpans([]any{
		float64(15),
		map[string]any{"line_number": float64(16), "page_number": float64(4)},
		"17-19",
	}, nil)
	want := []string{"15-19"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeProvisionSourceLineSpans=%v, want %v", got, want)
	}
}

func TestNormalizeProvisionSourceLineSpans_AcceptsStringSlices(t *testing.T) {
	got := normalizeProvisionSourceLineSpans([]string{"48", "49", "50-51"}, nil)
	want := []string{"48-51"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeProvisionSourceLineSpans string slice=%v, want %v", got, want)
	}
}

func TestSourceTextFromChunkLineOnlySpans(t *testing.T) {
	text := sourceTextFromSpans([]string{"12", "16-17"}, map[string]string{
		"12": "Alpha",
		"16": "Beta",
		"17": "Gamma",
	})
	if text != "Alpha\nBeta\nGamma" {
		t.Fatalf("sourceTextFromSpans=%q", text)
	}
}

func TestControlService_ChunkingOperationDoesNotSelectProvisions(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "static_analyzer", calls: &got},
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "extract_provisions", calls: &got},
		},
	}

	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1","operation":"chunking"}`))

	want := []string{"static_analyzer", "chunking"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadProvisionsPromptFromEnv_UsesPromptDir(t *testing.T) {
	tmp := t.TempDir()
	promptPath := filepath.Join(tmp, "prompt-extract-provisions.md")
	want := "extract normative provisions"
	if err := os.WriteFile(promptPath, []byte(want), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	t.Setenv("PROMPT_DIR", tmp)
	t.Setenv("EXTRACT_PROVISIONS_PROMPT", "prompt-extract-provisions.md")

	got, ref, gotPath, err := loadProvisionsPromptFromEnv()
	if err != nil {
		t.Fatalf("loadProvisionsPromptFromEnv: %v", err)
	}
	if got != want {
		t.Fatalf("promptText=%q, want %q", got, want)
	}
	if ref != "prompt-extract-provisions.md" {
		t.Fatalf("promptRef=%q", ref)
	}
	if gotPath != promptPath {
		t.Fatalf("promptPath=%q, want %q", gotPath, promptPath)
	}
}
