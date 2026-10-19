package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// inputDeletedTable records actual transaction deletions, including cascades.
type inputDeletedTable struct {
	Table string `json:"table"`
	Rows  int64  `json:"rows"`
}

// PostgreSQL's transaction-local counters include deletes performed by foreign-key cascades
// and triggers. Read them before commit; explicit DELETE RowsAffected does not include cascades.
const inputDeletedTablesQuery = `SELECT current_setting('track_counts')::boolean,
 COALESCE(jsonb_agg(jsonb_build_object('table', format('%I.%I', schemaname, relname),
 'rows', n_tup_del) ORDER BY schemaname, relname) FILTER (WHERE n_tup_del > 0), '[]'::jsonb)
 FROM pg_stat_xact_user_tables`

func readInputDeletedTables(tx *sql.Tx) ([]inputDeletedTable, error) {
	var enabled bool
	var encoded []byte
	if err := tx.QueryRow(inputDeletedTablesQuery).Scan(&enabled, &encoded); err != nil {
		return nil, err
	}
	if !enabled {
		return nil, fmt.Errorf("PostgreSQL track_counts must be enabled to audit cascading deletions")
	}
	var tables []inputDeletedTable
	if err := json.Unmarshal(encoded, &tables); err != nil {
		return nil, err
	}
	return tables, nil
}

func inputDeleteAuditInfo(deleteFiles bool, report inputDeleteReport, tables []inputDeletedTable, stats artifactWebCleanupStats, status string) map[string]any {
	return map[string]any{
		"operation": "delete_input", "delete_file": deleteFiles, "cleanup_status": status,
		"archived_file": report.ArchivedFile, "deleted_files": report.DeletedFiles,
		"missing_files": report.MissingFiles, "deleted_tables": tables, "artifact_web_cleanup": stats,
	}
}

// The pending audit commits atomically with the database deletion. If file cleanup or the
// final update fails, this row retains the archive, table counts and planned cleanup paths.
func startInputDeleteAudit(tx *sql.Tx, recordID int64, deleteFiles bool, plan inputDeletePlan, report inputDeleteReport, tables []inputDeletedTable) (int64, error) {
	info := inputDeleteAuditInfo(deleteFiles, report, tables, artifactWebCleanupStats{}, "pending")
	plannedFiles := append([]string{}, plan.Files...)
	for _, dir := range plan.Dirs {
		files, err := listRegularFiles(dir)
		if err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("inventory cleanup folder %s: %w", dir, err)
		}
		plannedFiles = append(plannedFiles, files...)
	}
	info["source_pdf"] = plan.PDFPath
	info["planned_files"] = plannedFiles
	info["planned_dirs"] = plan.Dirs
	info["artifact_web_roots"] = artifactWebCleanupRoots()
	encoded, err := json.Marshal(info)
	if err != nil {
		return 0, err
	}
	var auditID int64
	err = tx.QueryRow(`INSERT INTO kb.doc_proc_logs
 (call_reason, doc_proc_name, entry_type, record_id, extra_info, log_loc)
 VALUES ('delete_input', 'delete_input', 'delete_input', $1, $2::jsonb, $3) RETURNING id`,
		recordID, string(encoded), deleteInputDocProcLogLoc).Scan(&auditID)
	return auditID, err
}

func finishInputDeleteAudit(db *sql.DB, auditID int64, deleteFiles bool, report inputDeleteReport, tables []inputDeletedTable, stats artifactWebCleanupStats) error {
	status := "completed"
	var errorsText *string
	if len(report.Failures) > 0 {
		status = "failed"
		joined := strings.Join(report.Failures, "; ")
		errorsText = &joined
	}
	encoded, err := json.Marshal(inputDeleteAuditInfo(deleteFiles, report, tables, stats, status))
	if err != nil {
		return err
	}
	// Finish the audit even when the HTTP client disconnects, with a bounded database wait.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := db.ExecContext(ctx, `UPDATE kb.doc_proc_logs
 SET extra_info = extra_info || $1::jsonb, errors = $2 WHERE id = $3`, string(encoded), errorsText, auditID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("delete audit %d is missing", auditID)
	}
	return nil
}
