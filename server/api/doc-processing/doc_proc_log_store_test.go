package docprocessing

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDocProcLoggerLogSummary_InsertsMSUsed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	extra := `{}`
	msUsed := int64(1500)
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
    run_id,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, $16, $17, $18, NOW()
)`)
	mock.ExpectExec(insertQuery).
		WithArgs(
			nil,
			"generate_summaries",
			"{}",
			nil,
			nil,
			nil,
			EntryTypeGenerateTopics,
			nil,
			nil,
			nil,
			nil,
			nil,
			&extra,
			&msUsed,
			"MID-26052803",
			nil,
			nil,
			nil,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	logger := DocProcLogger{DB: db}
	if err := logger.LogSummary(context.Background(), EntryTypeGenerateTopics, DocProcLogRecord{
		DocProcName:   "generate_summaries",
		ExtraInfoJSON: &extra,
		MSUsed:        &msUsed,
	}, "MID-26052803"); err != nil {
		t.Fatalf("LogSummary: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDocProcLoggerLogExtractMetrics_IncludesCallReasonRecordIDAndProgress(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	extra := `{"percent":"50%"}`
	recordID := int64(170)
	progress := "50%"
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
    run_id,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, $16, $17, $18, NOW()
)`)
	mock.ExpectExec(insertQuery).
		WithArgs(
			strPtrValue("extract_metrics"),
			"extract_metrics",
			"{}",
			nil,
			int64PtrValue(recordID),
			strPtrValue(progress),
			"extract_metrics",
			nil,
			nil,
			nil,
			nil,
			nil,
			&extra,
			nil,
			"MID-26052811",
			nil,
			nil,
			nil,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	updateQuery := regexp.QuoteMeta(`
UPDATE kb.inputs
SET status = (
    SELECT jsonb_agg(
        CASE
            WHEN replace(lower(trim(coalesce(elem->>'operation', ''))), '-', '_') = ANY($2::text[])
            THEN jsonb_set(
                jsonb_set(elem, '{progress}', to_jsonb($3::text), true),
                '{proc_status}',
                to_jsonb('active'::text),
                true
            )
            ELSE elem
        END
    )
    FROM jsonb_array_elements(COALESCE(status, '[]'::jsonb)) AS elem
),
modify_time = NOW()
WHERE id = $1
  AND status IS NOT NULL
  AND jsonb_typeof(status) = 'array'`)
	mock.ExpectExec(updateQuery).
		WithArgs(recordID, sqlmock.AnyArg(), progress).
		WillReturnResult(sqlmock.NewResult(0, 1))

	logger := DocProcLogger{DB: db}
	if err := logger.LogExtractMetrics(context.Background(), DocProcLogRecord{
		CallReason:    "extract_metrics",
		DocProcName:   "extract_metrics",
		RecordID:      &recordID,
		ProcProgress:  &progress,
		ExtraInfoJSON: &extra,
	}, "MID-26052811"); err != nil {
		t.Fatalf("LogExtractMetrics: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDocProcLoggerLogExtractDocMetadata_ProgressUsesCanonicalAliases(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	recordID := int64(432)
	progress := "25%"
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
    run_id,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, $16, $17, $18, NOW()
)`)
	mock.ExpectExec(insertQuery).
		WithArgs(
			nil,
			"extract_doc_metadata",
			"{}",
			nil,
			&recordID,
			&progress,
			EntryTypeExtractDocMetadata,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			"MID-TEST-ALIAS",
			nil,
			nil,
			nil,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	updateQuery := regexp.QuoteMeta(`
UPDATE kb.inputs
SET status = (
    SELECT jsonb_agg(
        CASE
            WHEN replace(lower(trim(coalesce(elem->>'operation', ''))), '-', '_') = ANY($2::text[])
            THEN jsonb_set(
                jsonb_set(elem, '{progress}', to_jsonb($3::text), true),
                '{proc_status}',
                to_jsonb('active'::text),
                true
            )
            ELSE elem
        END
    )
    FROM jsonb_array_elements(COALESCE(status, '[]'::jsonb)) AS elem
),
modify_time = NOW()
WHERE id = $1
  AND status IS NOT NULL
  AND jsonb_typeof(status) = 'array'`)
	mock.ExpectExec(updateQuery).
		WithArgs(recordID, sqlmock.AnyArg(), progress).
		WillReturnResult(sqlmock.NewResult(0, 1))

	logger := DocProcLogger{DB: db}
	if err := logger.LogExtractDocMetadata(context.Background(), DocProcLogRecord{
		DocProcName:  "extract_doc_metadata",
		RecordID:     &recordID,
		ProcProgress: &progress,
	}, "MID-TEST-ALIAS"); err != nil {
		t.Fatalf("LogExtractDocMetadata: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDocProcLoggerLogSummary_IncludesRunIDFromContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	extra := `{}`
	msUsed := int64(1500)
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
    run_id,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, $16, $17, $18, NOW()
)`)
	runID := int64(77)
	mock.ExpectExec(insertQuery).
		WithArgs(
			nil,
			"generate_summaries",
			"{}",
			nil,
			nil,
			nil,
			EntryTypeGenerateSummary,
			nil,
			nil,
			nil,
			nil,
			nil,
			&extra,
			&msUsed,
			"MID-26071201",
			nil,
			nil,
			&runID,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	logger := DocProcLogger{DB: db}
	ctx := withRunID(context.Background(), runID)
	if err := logger.LogSummary(ctx, EntryTypeGenerateSummary, DocProcLogRecord{
		DocProcName:   "generate_summaries",
		ExtraInfoJSON: &extra,
		MSUsed:        &msUsed,
	}, "MID-26071201"); err != nil {
		t.Fatalf("LogSummary: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLStoreListDocProcLogs_ReturnsMSUsed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs WHERE entry_type = $1")).
		WithArgs("test_entry_type").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"id", "coalesce", "doc_proc_name", "model_names", "coalesce", "record_id", "proc_progress", "entry_type",
		"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info",
		"ms_used", "log_loc", "coalesce", "prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "run_id",
	}).AddRow(
		int64(9), "summary call", "generate_topics", `{topic-model}`, "topic-prompt", int64(81), "66% (2/3)", "test_entry_type",
		nil, nil, nil, nil, nil, `{"topics_generated":7}`, int64(2400), "generate_phase_processors_test.go_123", "2026-05-27T12:00:00+00:00",
		nil, nil, nil,
	)

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\),\s+prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id\s+FROM kb\.doc_proc_logs\s+WHERE entry_type = \$1\s+ORDER BY create_time DESC\s+LIMIT \$2 OFFSET \$3`
	mock.ExpectQuery(listQuery).
		WithArgs("test_entry_type", 50, 0).
		WillReturnRows(rows)

	store := SQLStore{DB: db}
	got, total, err := store.ListDocProcLogs(context.Background(), DocProcLogFilter{EntryType: "test_entry_type"})
	if err != nil {
		t.Fatalf("ListDocProcLogs: %v", err)
	}
	if total != 1 {
		t.Fatalf("total=%d, want 1", total)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	if got[0].MSUsed == nil || *got[0].MSUsed != 2400 {
		t.Fatalf("MSUsed=%v, want 2400", got[0].MSUsed)
	}
	if got[0].RecordID == nil || *got[0].RecordID != 81 {
		t.Fatalf("RecordID=%v, want 81", got[0].RecordID)
	}
	if got[0].ProcProgress == nil || *got[0].ProcProgress != "66% (2/3)" {
		t.Fatalf("ProcProgress=%v, want 66%% (2/3)", got[0].ProcProgress)
	}
	if got[0].CreateTime != "2026-05-27T12:00:00+00:00" {
		t.Fatalf("CreateTime=%q", got[0].CreateTime)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLStoreListDocProcLogs_FiltersByRunIDAndReturnsRunID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	runID := int64(77)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs WHERE run_id = $1")).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"id", "coalesce", "doc_proc_name", "model_names", "coalesce", "record_id", "proc_progress", "entry_type",
		"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info",
		"ms_used", "log_loc", "coalesce", "prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "run_id",
	}).AddRow(
		int64(9), "summary call", "generate_topics", `{topic-model}`, "topic-prompt", int64(81), "66% (2/3)", "test_entry_type",
		nil, nil, nil, nil, nil, `{"topics_generated":7}`, int64(2400), "generate_phase_processors_test.go_123", "2026-05-27T12:00:00+00:00",
		nil, nil, runID,
	)

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\),\s+prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id\s+FROM kb\.doc_proc_logs\s+WHERE run_id = \$1\s+ORDER BY create_time DESC\s+LIMIT \$2 OFFSET \$3`
	mock.ExpectQuery(listQuery).
		WithArgs(runID, 50, 0).
		WillReturnRows(rows)

	store := SQLStore{DB: db}
	got, total, err := store.ListDocProcLogs(context.Background(), DocProcLogFilter{RunID: &runID})
	if err != nil {
		t.Fatalf("ListDocProcLogs: %v", err)
	}
	if total != 1 {
		t.Fatalf("total=%d, want 1", total)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	if got[0].RunID == nil || *got[0].RunID != runID {
		t.Fatalf("RunID=%v, want %d", got[0].RunID, runID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLStoreListDocProcLogs_FiltersByActivityAndCreateTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	start := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 12, 23, 59, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs WHERE activity_name = $1 AND create_time >= $2 AND create_time <= $3")).
		WithArgs("extract_metrics_finish", start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"id", "coalesce", "doc_proc_name", "model_names", "coalesce", "record_id", "proc_progress", "entry_type",
		"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info",
		"ms_used", "log_loc", "coalesce", "prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "run_id",
	}).AddRow(
		int64(12), "summary call", "extract_metrics", `{deepseek-v4-flash}`, "metric-prompt", int64(55), "100%", "extract_metrics_finish",
		nil, nil, "extract_metrics_finish", nil, nil, `{"total_metrics":57}`, int64(9300), "doc_proc_log_store_test.go_234", "2026-07-12T12:00:00+00:00",
		nil, nil, int64(44),
	)

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\),\s+prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id\s+FROM kb\.doc_proc_logs\s+WHERE activity_name = \$1 AND create_time >= \$2 AND create_time <= \$3\s+ORDER BY create_time DESC\s+LIMIT \$4 OFFSET \$5`
	mock.ExpectQuery(listQuery).
		WithArgs("extract_metrics_finish", start, end, 50, 0).
		WillReturnRows(rows)

	store := SQLStore{DB: db}
	got, total, err := store.ListDocProcLogs(context.Background(), DocProcLogFilter{
		ActivityName:    "extract_metrics_finish",
		CreateTimeStart: &start,
		CreateTimeEnd:   &end,
	})
	if err != nil {
		t.Fatalf("ListDocProcLogs: %v", err)
	}
	if total != 1 {
		t.Fatalf("total=%d, want 1", total)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	if got[0].ActivityName == nil || *got[0].ActivityName != "extract_metrics_finish" {
		t.Fatalf("ActivityName=%v, want extract_metrics_finish", got[0].ActivityName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLStoreListDocProcLogs_OrdersByAllowedField(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs ")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"id", "coalesce", "doc_proc_name", "model_names", "coalesce", "record_id", "proc_progress", "entry_type",
		"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info",
		"ms_used", "log_loc", "coalesce", "prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "run_id",
	}).AddRow(
		int64(10), "llm call", "extract_metrics", `{deepseek-v4-flash}`, "metric-prompt", int64(55), nil, "test_entry_type",
		2, "call-1", "enrich_metrics", nil, nil, `{"source":"test"}`, int64(9300), "doc_proc_log_store_test.go_234", "2026-05-27T12:00:00+00:00",
		nil, nil, nil,
	)

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\),\s+prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id\s+FROM kb\.doc_proc_logs\s+ORDER BY ms_used ASC\s+LIMIT \$1 OFFSET \$2`
	mock.ExpectQuery(listQuery).
		WithArgs(50, 0).
		WillReturnRows(rows)

	store := SQLStore{DB: db}
	_, _, err = store.ListDocProcLogs(context.Background(), DocProcLogFilter{
		OrderBy:  "ms_used",
		OrderDir: "asc",
	})
	if err != nil {
		t.Fatalf("ListDocProcLogs: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
