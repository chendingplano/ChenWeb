package llmusage

import (
	"context"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
)

func TestSinkCaptureWritesArchivesAndPersistsUsageEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	startedAt := time.Date(2026, 6, 19, 13, 30, 0, 0, time.UTC)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	tmpDir := t.TempDir()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO llm_usage_event (
    id, account_id, profile_id, provider, model_name, prompt_name,
    request_started_at, request_finished_at, workspace_day,
    input_tokens, output_tokens, total_tokens, latency_ms, http_status,
    error_message, input_body_ref, output_body_ref, provider_request_id, metadata_json,
    record_id, call_reason, call_loc
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19::jsonb,
    $20, $21, $22
)`)).
		WithArgs(
			"evt-test-1",
			"acct_1",
			"prof_1",
			"openai_compatible",
			"deepseek-chat",
			"extract-products-v2",
			startedAt,
			finishedAt,
			time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
			int64(12),
			int64(34),
			int64(46),
			int64(1500),
			0,
			"",
			filepath.Join("2026", "2026-06", "2026-06-19", "account-acct_1", "bodies", "evt-test-1-input.json.gz"),
			filepath.Join("2026", "2026-06", "2026-06-19", "account-acct_1", "bodies", "evt-test-1-output.json.gz"),
			"req_123",
			`{"capture_source":"shared_llm"}`,
			int64(901),
			"extract_products",
			"MID-CWB-TEST-SINK",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	sink := &Sink{
		DB:            db,
		ArchiveRoot:   tmpDir,
		WorkspaceTZ:   time.UTC,
		NewID:         func() string { return "evt-test-1" },
		Now:           func() time.Time { return finishedAt },
		DefaultStatus: 0,
	}

	record := sharedllm.UsageCaptureRecord{
		AccountID:         "acct_1",
		ProfileID:         "prof_1",
		Provider:          sharedllm.ProviderOpenAICompatible,
		ModelName:         "deepseek-chat",
		PromptName:        "extract-products-v2",
		RequestStartedAt:  startedAt,
		RequestFinishedAt: finishedAt,
		InputTokens:       12,
		OutputTokens:      34,
		TotalTokens:       46,
		ProviderRequestID: "req_123",
		InputBody:         []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		OutputBody:        []byte(`{"content":"hello"}`),
		RecordID:          901,
		CallReason:        "extract_products",
		CallLoc:           "MID-CWB-TEST-SINK",
	}

	if err := sink.Capture(context.Background(), record); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	inputPath := filepath.Join(tmpDir, "2026", "2026-06", "2026-06-19", "account-acct_1", "bodies", "evt-test-1-input.json.gz")
	outputPath := filepath.Join(tmpDir, "2026", "2026-06", "2026-06-19", "account-acct_1", "bodies", "evt-test-1-output.json.gz")
	inputBody, err := sharedllm.ReadGzipFile(inputPath)
	if err != nil {
		t.Fatalf("ReadGzipFile(input) error = %v", err)
	}
	outputBody, err := sharedllm.ReadGzipFile(outputPath)
	if err != nil {
		t.Fatalf("ReadGzipFile(output) error = %v", err)
	}
	if string(inputBody) != `{"messages":[{"role":"user","content":"hi"}]}` {
		t.Fatalf("unexpected input archive = %q", string(inputBody))
	}
	if string(outputBody) != `{"content":"hello"}` {
		t.Fatalf("unexpected output archive = %q", string(outputBody))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSinkCaptureSkipsDatabaseInsertWhenAccountProfileMissing(t *testing.T) {
	sink := &Sink{
		ArchiveRoot: t.TempDir(),
		WorkspaceTZ: time.UTC,
		NewID:       func() string { return "evt-test-2" },
		Now:         func() time.Time { return time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC) },
	}

	record := sharedllm.UsageCaptureRecord{
		Provider:         sharedllm.ProviderOpenAI,
		ModelName:        "gpt-4o-mini",
		PromptName:       "no-account-yet",
		RequestStartedAt: time.Date(2026, 6, 19, 9, 59, 0, 0, time.UTC),
		InputBody:        []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		OutputBody:       []byte(`{"content":"hello"}`),
	}

	if err := sink.Capture(context.Background(), record); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	inputPath := filepath.Join(sink.ArchiveRoot, "2026", "2026-06", "2026-06-19", "account-unknown", "bodies", "evt-test-2-input.json.gz")
	if _, err := sharedllm.ReadGzipFile(inputPath); err != nil {
		t.Fatalf("expected archive for unknown account, got error %v", err)
	}
}

func TestSinkCaptureResolvesAccountProfileFromLookupHints(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	startedAt := time.Date(2026, 6, 19, 13, 30, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)
	tmpDir := t.TempDir()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT a.id, p.id
FROM llm_account a
JOIN llm_account_model_profile p ON p.account_id = a.id
WHERE LOWER(a.provider) = LOWER($1)
  AND LOWER(TRIM(TRAILING '/' FROM a.base_url)) = LOWER(TRIM(TRAILING '/' FROM $2))
  AND a.api_key_ref = $3
  AND LOWER(p.profile_name) = LOWER($4)
LIMIT 1`)).
		WithArgs("deepseek", "https://api.deepseek.com", "sk-live", "deepseek-prod").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "profile_id"}).AddRow("acct_22", "prof_33"))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO llm_usage_event (
    id, account_id, profile_id, provider, model_name, prompt_name,
    request_started_at, request_finished_at, workspace_day,
    input_tokens, output_tokens, total_tokens, latency_ms, http_status,
    error_message, input_body_ref, output_body_ref, provider_request_id, metadata_json,
    record_id, call_reason, call_loc
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19::jsonb,
    $20, $21, $22
)`)).
		WithArgs(
			"evt-test-3",
			"acct_22",
			"prof_33",
			"deepseek",
			"deepseek-chat",
			"extract-provisions-v1",
			startedAt,
			finishedAt,
			time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
			int64(9),
			int64(4),
			int64(13),
			int64(2000),
			0,
			"",
			filepath.Join("2026", "2026-06", "2026-06-19", "account-acct_22", "bodies", "evt-test-3-input.json.gz"),
			filepath.Join("2026", "2026-06", "2026-06-19", "account-acct_22", "bodies", "evt-test-3-output.json.gz"),
			"req_lookup_1",
			`{"capture_source":"shared_llm"}`,
			nil,
			"",
			"",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	sink := &Sink{
		DB:            db,
		ArchiveRoot:   tmpDir,
		WorkspaceTZ:   time.UTC,
		NewID:         func() string { return "evt-test-3" },
		Now:           func() time.Time { return finishedAt },
		DefaultStatus: 0,
	}

	record := sharedllm.UsageCaptureRecord{
		Provider:          sharedllm.ProviderID("deepseek"),
		BaseURL:           "https://api.deepseek.com",
		APIKey:            "sk-live",
		ProfileName:       "deepseek-prod",
		ModelName:         "deepseek-chat",
		PromptName:        "extract-provisions-v1",
		RequestStartedAt:  startedAt,
		RequestFinishedAt: finishedAt,
		InputTokens:       9,
		OutputTokens:      4,
		TotalTokens:       13,
		ProviderRequestID: "req_lookup_1",
		InputBody:         []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		OutputBody:        []byte(`{"content":"hello"}`),
	}

	if err := sink.Capture(context.Background(), record); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSinkCaptureResolvesAccountProfileWhenBaseURLTrailingSlashDiffers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	startedAt := time.Date(2026, 6, 19, 13, 30, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)
	tmpDir := t.TempDir()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT a.id, p.id
FROM llm_account a
JOIN llm_account_model_profile p ON p.account_id = a.id
WHERE LOWER(a.provider) = LOWER($1)
  AND LOWER(TRIM(TRAILING '/' FROM a.base_url)) = LOWER(TRIM(TRAILING '/' FROM $2))
  AND a.api_key_ref = $3
  AND LOWER(p.profile_name) = LOWER($4)
LIMIT 1`)).
		WithArgs("deepseek", "https://api.deepseek.com/", "sk-live", "deepseek-prod").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "profile_id"}).AddRow("acct_22", "prof_33"))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO llm_usage_event (
    id, account_id, profile_id, provider, model_name, prompt_name,
    request_started_at, request_finished_at, workspace_day,
    input_tokens, output_tokens, total_tokens, latency_ms, http_status,
    error_message, input_body_ref, output_body_ref, provider_request_id, metadata_json,
    record_id, call_reason, call_loc
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19::jsonb,
    $20, $21, $22
)`)).
		WithArgs(
			"evt-test-4",
			"acct_22",
			"prof_33",
			"deepseek",
			"deepseek-chat",
			"extract-provisions-v1",
			startedAt,
			finishedAt,
			time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
			int64(9),
			int64(4),
			int64(13),
			int64(2000),
			0,
			"",
			filepath.Join("2026", "2026-06", "2026-06-19", "account-acct_22", "bodies", "evt-test-4-input.json.gz"),
			filepath.Join("2026", "2026-06", "2026-06-19", "account-acct_22", "bodies", "evt-test-4-output.json.gz"),
			"req_lookup_2",
			`{"capture_source":"shared_llm"}`,
			nil,
			"",
			"",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	sink := &Sink{
		DB:            db,
		ArchiveRoot:   tmpDir,
		WorkspaceTZ:   time.UTC,
		NewID:         func() string { return "evt-test-4" },
		Now:           func() time.Time { return finishedAt },
		DefaultStatus: 0,
	}

	record := sharedllm.UsageCaptureRecord{
		Provider:          sharedllm.ProviderID("deepseek"),
		BaseURL:           "https://api.deepseek.com/",
		APIKey:            "sk-live",
		ProfileName:       "deepseek-prod",
		ModelName:         "deepseek-chat",
		PromptName:        "extract-provisions-v1",
		RequestStartedAt:  startedAt,
		RequestFinishedAt: finishedAt,
		InputTokens:       9,
		OutputTokens:      4,
		TotalTokens:       13,
		ProviderRequestID: "req_lookup_2",
		InputBody:         []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		OutputBody:        []byte(`{"content":"hello"}`),
	}

	if err := sink.Capture(context.Background(), record); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSinkCaptureResolvesAccountProfileByModelNameFallback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	startedAt := time.Date(2026, 6, 19, 13, 30, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)
	tmpDir := t.TempDir()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT a.id, p.id
FROM llm_account a
JOIN llm_account_model_profile p ON p.account_id = a.id
WHERE LOWER(a.provider) = LOWER($1)
  AND LOWER(TRIM(TRAILING '/' FROM a.base_url)) = LOWER(TRIM(TRAILING '/' FROM $2))
  AND a.api_key_ref = $3
  AND LOWER(p.profile_name) = LOWER($4)
LIMIT 1`)).
		WithArgs("deepseek", "https://api.deepseek.com", "sk-live", "missing-profile").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "profile_id"}))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT a.id, p.id
FROM llm_account a
JOIN llm_account_model_profile p ON p.account_id = a.id
WHERE LOWER(a.provider) = LOWER($1)
  AND LOWER(TRIM(TRAILING '/' FROM a.base_url)) = LOWER(TRIM(TRAILING '/' FROM $2))
  AND a.api_key_ref = $3
  AND LOWER(p.model_name) = LOWER($4)
LIMIT 1`)).
		WithArgs("deepseek", "https://api.deepseek.com", "sk-live", "deepseek-v4-flash").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "profile_id"}).AddRow("acct_44", "prof_55"))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO llm_usage_event (
    id, account_id, profile_id, provider, model_name, prompt_name,
    request_started_at, request_finished_at, workspace_day,
    input_tokens, output_tokens, total_tokens, latency_ms, http_status,
    error_message, input_body_ref, output_body_ref, provider_request_id, metadata_json,
    record_id, call_reason, call_loc
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19::jsonb,
    $20, $21, $22
)`)).
		WithArgs(
			"evt-test-5",
			"acct_44",
			"prof_55",
			"deepseek",
			"deepseek-v4-flash",
			"extract-products-v2",
			startedAt,
			finishedAt,
			time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
			int64(6),
			int64(7),
			int64(13),
			int64(2000),
			0,
			"",
			filepath.Join("2026", "2026-06", "2026-06-19", "account-acct_44", "bodies", "evt-test-5-input.json.gz"),
			filepath.Join("2026", "2026-06", "2026-06-19", "account-acct_44", "bodies", "evt-test-5-output.json.gz"),
			"req_lookup_3",
			`{"capture_source":"shared_llm"}`,
			nil,
			"",
			"",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	sink := &Sink{
		DB:            db,
		ArchiveRoot:   tmpDir,
		WorkspaceTZ:   time.UTC,
		NewID:         func() string { return "evt-test-5" },
		Now:           func() time.Time { return finishedAt },
		DefaultStatus: 0,
	}

	record := sharedllm.UsageCaptureRecord{
		Provider:          sharedllm.ProviderID("deepseek"),
		BaseURL:           "https://api.deepseek.com",
		APIKey:            "sk-live",
		ProfileName:       "missing-profile",
		ModelName:         "deepseek-v4-flash",
		PromptName:        "extract-products-v2",
		RequestStartedAt:  startedAt,
		RequestFinishedAt: finishedAt,
		InputTokens:       6,
		OutputTokens:      7,
		TotalTokens:       13,
		ProviderRequestID: "req_lookup_3",
		InputBody:         []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		OutputBody:        []byte(`{"content":"hello"}`),
	}

	if err := sink.Capture(context.Background(), record); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
