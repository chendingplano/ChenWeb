package docprocessing

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type stubLLMJSONExtractor struct {
	results []stubLLMJSONResult
	calls   int
	inputs  []llmclients.JSONExtractionInput
}

type stubLLMJSONResult struct {
	payload map[string]any
	err     error
}

func (s *stubLLMJSONExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	s.inputs = append(s.inputs, in)
	if s.calls >= len(s.results) {
		return nil, errors.New("unexpected ExtractJSON call")
	}
	res := s.results[s.calls]
	s.calls++
	return res.payload, res.err
}

type stubCategoryLogger struct {
	infoMessages []string
	infoArgs     [][]any
}

func (l *stubCategoryLogger) Info(message string, args ...any) {
	l.infoMessages = append(l.infoMessages, message)
	l.infoArgs = append(l.infoArgs, args)
}

func (l *stubCategoryLogger) Debug(string, ...any) {}
func (l *stubCategoryLogger) Line(string, ...any)  {}
func (l *stubCategoryLogger) Warn(string, ...any)  {}
func (l *stubCategoryLogger) Error(string, ...any) {}
func (l *stubCategoryLogger) Trace(string)         {}
func (l *stubCategoryLogger) Close()               {}

func TestLLMCategoryCreatorCreateCategoryLogsAndPrintsEachCall(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.doc_proc_logs (
    call_reason,
    doc_proc_name,
    model_names,
    prompt_name,
    record_id,
    proc_progress,
    entry_type,
    pass,
    llm_call_id,
    activity_name,
    artifact,
    errors,
    extra_info,
    ms_used,
    log_loc,
    prompt_cache_hit_tokens,
    prompt_cache_miss_tokens,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, $16, $17, NOW()
)`)
	mock.ExpectExec(insertQuery).
		WithArgs(
			strPtrValue("create_artifact_category"),
			"create_artifact_category",
			"{category-model}",
			strPtrValue("CREATE_ARTIFACT_CATEGORY_PROMPT"),
			nil,
			nil,
			"llm_call",
			nil,
			strPtrValue("call-1"),
			strPtrValue("category_metric"),
			sqlmock.AnyArg(),
			nil,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"MID-26060501",
			nil,
			nil,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	extractor := &stubLLMJSONExtractor{results: []stubLLMJSONResult{
		{payload: map[string]any{
			"category_key":     "throughput",
			"category_type":    "metric",
			"display_names":    []any{"Throughput"},
			"aliases":          []any{},
			"acronyms":         []any{},
			"description":      "Requests handled per interval.",
			"keywords":         []any{"requests", "rate"},
			"required_attrs":   []any{},
			"specs":            map[string]any{},
			"plausible_ranges": map[string]any{},
			"search_document":  "throughput requests rate requests handled per interval",
		}},
	}}
	logger := &stubCategoryLogger{}
	creator := &llmCategoryCreator{
		extractor:    extractor,
		promptText:   "prompt",
		modelName:    "category-model",
		procLogger:   DocProcLogger{DB: db},
		logger:       logger,
		newLLMCallID: func() string { return "call-1" },
	}

	got, err := creator.CreateCategory(context.Background(), "Throughput", "metric", map[string]any{"unit": "req/s"})
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	if got.CategoryKey != "throughput" {
		t.Fatalf("CategoryKey = %q, want throughput", got.CategoryKey)
	}
	if extractor.calls != 1 {
		t.Fatalf("ExtractJSON calls = %d, want 1", extractor.calls)
	}
	if len(extractor.inputs) != 1 || extractor.inputs[0].PromptName != "CREATE_ARTIFACT_CATEGORY_PROMPT" {
		t.Fatalf("PromptName = %q, want CREATE_ARTIFACT_CATEGORY_PROMPT", firstPromptName(extractor.inputs))
	}
	if len(logger.infoMessages) != 2 || logger.infoMessages[0] != "Create Category start" || logger.infoMessages[1] != "Create Category end  " {
		t.Fatalf("infoMessages = %#v, want start/end logs", logger.infoMessages)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestDeterministicCategoryCreatorLogsAndNormalizes(t *testing.T) {
	logger := &stubCategoryLogger{}
	creator := &deterministicCategoryCreator{logger: logger}

	got, err := creator.CreateCategory(context.Background(), "  Response_Time  ", "metric", map[string]any{"artifact_kind": "metric"})
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	if got.CategoryKey != "response time" {
		t.Fatalf("CategoryKey = %q, want response time", got.CategoryKey)
	}
	if len(got.DisplayNames) != 1 || got.DisplayNames[0] != "Response_Time" {
		t.Fatalf("DisplayNames = %#v, want [Response_Time]", got.DisplayNames)
	}
	if len(logger.infoMessages) != 2 || logger.infoMessages[0] != "Create Category start" || logger.infoMessages[1] != "Create Category end  " {
		t.Fatalf("infoMessages = %#v, want start/end logs", logger.infoMessages)
	}
}

func TestLLMCategoryCreatorCreateCategoryLogsPrimaryAndFallbackCalls(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.doc_proc_logs (
    call_reason,
    doc_proc_name,
    model_names,
    prompt_name,
    record_id,
    proc_progress,
    entry_type,
    pass,
    llm_call_id,
    activity_name,
    artifact,
    errors,
    extra_info,
    ms_used,
    log_loc,
    prompt_cache_hit_tokens,
    prompt_cache_miss_tokens,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, $16, $17, NOW()
)`)
	mock.ExpectExec(insertQuery).
		WithArgs(
			strPtrValue("create_artifact_category"),
			"create_artifact_category",
			"{primary-model}",
			strPtrValue("CREATE_ARTIFACT_CATEGORY_PROMPT"),
			nil,
			nil,
			"llm_call",
			nil,
			strPtrValue("call-1"),
			strPtrValue("category_metric"),
			nil,
			strPtrValue("primary failed"),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"MID-26060501",
			nil,
			nil,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(insertQuery).
		WithArgs(
			strPtrValue("create_artifact_category"),
			"create_artifact_category",
			"{fallback-model}",
			strPtrValue("CREATE_ARTIFACT_CATEGORY_PROMPT"),
			nil,
			nil,
			"llm_call",
			nil,
			strPtrValue("call-2"),
			strPtrValue("category_metric"),
			sqlmock.AnyArg(),
			nil,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"MID-26060501",
			nil,
			nil,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	extractor := &stubLLMJSONExtractor{results: []stubLLMJSONResult{
		{err: errors.New("primary failed")},
		{payload: map[string]any{
			"category_key":     "throughput",
			"category_type":    "metric",
			"display_names":    []any{"Throughput"},
			"aliases":          []any{},
			"acronyms":         []any{},
			"description":      "Requests handled per interval.",
			"keywords":         []any{"requests", "rate"},
			"required_attrs":   []any{},
			"specs":            map[string]any{},
			"plausible_ranges": map[string]any{},
			"search_document":  "throughput requests rate requests handled per interval",
		}},
	}}
	logger := &stubCategoryLogger{}
	callIDs := []string{"call-1", "call-2"}
	creator := &llmCategoryCreator{
		extractor:     extractor,
		promptText:    "prompt",
		modelName:     "primary-model",
		fallbackModel: "fallback-model",
		procLogger:    DocProcLogger{DB: db},
		logger:        logger,
		newLLMCallID: func() string {
			id := callIDs[0]
			callIDs = callIDs[1:]
			return id
		},
	}

	got, err := creator.CreateCategory(context.Background(), "Throughput", "metric", nil)
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	if got.CategoryKey != "throughput" {
		t.Fatalf("CategoryKey = %q, want throughput", got.CategoryKey)
	}
	if extractor.calls != 2 {
		t.Fatalf("ExtractJSON calls = %d, want 2", extractor.calls)
	}
	if len(extractor.inputs) != 2 {
		t.Fatalf("len(inputs) = %d, want 2", len(extractor.inputs))
	}
	for i, in := range extractor.inputs {
		if in.PromptName != "CREATE_ARTIFACT_CATEGORY_PROMPT" {
			t.Fatalf("inputs[%d].PromptName = %q, want CREATE_ARTIFACT_CATEGORY_PROMPT", i, in.PromptName)
		}
	}
	if len(logger.infoMessages) != 4 {
		t.Fatalf("infoMessages = %#v, want start/end logs for primary and fallback", logger.infoMessages)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func firstPromptName(inputs []llmclients.JSONExtractionInput) string {
	if len(inputs) == 0 {
		return ""
	}
	return inputs[0].PromptName
}
