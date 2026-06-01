package docprocessing

import (
	"context"
	"regexp"
	"testing"

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
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, NOW()
)`)
	mock.ExpectExec(insertQuery).
		WithArgs(
			nil,
			"generate_summaries",
			"{}",
			nil,
			nil,
			nil,
			"doc_proc_summary",
			nil,
			nil,
			nil,
			nil,
			nil,
			&extra,
			&msUsed,
			"MID-26052803",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	logger := DocProcLogger{DB: db}
	if err := logger.LogSummary(context.Background(), "test_entry_type", DocProcLogRecord{
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
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, NOW()
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
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	updateQuery := regexp.QuoteMeta(`
UPDATE kb.inputs
SET status = (
    SELECT jsonb_agg(
        CASE
            WHEN elem->>'operation' = $2
            THEN jsonb_set(elem, '{progress}', to_jsonb($3::text))
            ELSE elem
        END
    )
    FROM jsonb_array_elements(COALESCE(status, '[]'::jsonb)) AS elem
),
modify_time = NOW()
WHERE id = $1`)
	mock.ExpectExec(updateQuery).
		WithArgs(recordID, "extract_metrics", progress).
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
		"ms_used", "log_loc", "coalesce",
	}).AddRow(
		int64(9), "summary call", "generate_topics", `{topic-model}`, "topic-prompt", int64(81), "66% (2/3)", "test_entry_type",
		nil, nil, nil, nil, nil, `{"topics_generated":7}`, int64(2400), "generate_phase_processors_test.go_123", "2026-05-27T12:00:00+00:00",
	)

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\)\s+FROM kb\.doc_proc_logs\s+WHERE entry_type = \$1\s+ORDER BY create_time DESC\s+LIMIT \$2 OFFSET \$3`
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
		"ms_used", "log_loc", "coalesce",
	}).AddRow(
		int64(10), "llm call", "extract_metrics", `{deepseek-v4-flash}`, "metric-prompt", int64(55), nil, "test_entry_type",
		2, "call-1", "enrich_metrics", nil, nil, `{"source":"test"}`, int64(9300), "doc_proc_log_store_test.go_234", "2026-05-27T12:00:00+00:00",
	)

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\)\s+FROM kb\.doc_proc_logs\s+ORDER BY ms_used ASC\s+LIMIT \$1 OFFSET \$2`
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
