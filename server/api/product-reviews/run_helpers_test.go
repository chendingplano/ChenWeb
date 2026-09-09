package productreviews

import (
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func requestReturnRow(id, profileID int64, version int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "tenant_id", "profile_id", "profile_version", "artifact_types",
		"filters", "notes", "requester", "created_at", "updated_at",
	}).AddRow(id, "-", profileID, version, []byte(`["metric"]`), []byte(`{}`), "", "", time.Now(), time.Now())
}

func runReturnRow(id, requestID int64, runNumber int, status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "request_id", "run_number", "status", "result_count", "attributed_count",
		"document_scope_count", "scoped_document_count", "truncated_count",
		"report_json", "report_md", "error_message", "created_at", "updated_at",
	}).AddRow(id, requestID, runNumber, status, 0, 0, 0, 0, 0, []byte(`{}`), "", "", time.Now(), time.Now())
}

func runGetRow(id, requestID int64, runNumber int, status string, reportJSON string) *sqlmock.Rows {
	if reportJSON == "" {
		reportJSON = "{}"
	}
	return sqlmock.NewRows([]string{
		"id", "request_id", "run_number", "status", "started_at", "finished_at",
		"result_count", "attributed_count", "document_scope_count", "scoped_document_count",
		"truncated_count", "report_json", "report_md", "error_message", "created_at", "updated_at",
	}).AddRow(id, requestID, runNumber, status, time.Now(), time.Now(),
		0, 0, 0, 0, 0, []byte(reportJSON), "", "", time.Now(), time.Now())
}
