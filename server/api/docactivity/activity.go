// Package docactivity records and reads reviewer correction actions performed on
// a Document Review Report (LLM Auto Fix, Edit Tool, Delete, and Document
// Structure edits). It is a leaf package so both the doc-reviews controller and
// the kbhandler doc-structure handlers can log activities without an import
// cycle. Persistence targets kb.doc_review_activities (see the goose migration
// 20260624000001_create_doc_review_activities.sql).
package docactivity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

var logger = loggerutil.CreateDefaultLogger("DR17")

// Activity types. These are the canonical activity_type values stored in
// kb.doc_review_activities and consumed by the Document Review Correction Report.
const (
	TypeAutoFix         = "auto_fix"         // LLM Auto Fix applied to a finding
	TypeEditTool        = "edit_tool"        // manual Edit Tool save of a finding's lines
	TypeFindingDelete   = "finding_delete"   // finding removed via the Delete button
	TypeStructureModify = "structure_modify" // Document Structure line edited
	TypeStructureSplit  = "structure_split"  // Document Structure line split
	TypeStructureDelete = "structure_delete" // Document Structure line deleted
)

// Activity is one reviewer correction action. Fields that do not apply to a given
// activity type are left at their zero value (and stored as SQL NULL where the
// column is nullable).
type Activity struct {
	ID            int64          `json:"id"`
	ActivityType  string         `json:"activity_type"`
	InputRecordID int64          `json:"input_record_id"`
	RunID         int64          `json:"run_id"`
	ReportID      int64          `json:"report_id"`
	FindingID     int64          `json:"finding_id"`
	PageNumber    int            `json:"page_number"`
	LineNumber    int            `json:"line_number"`
	Location      string         `json:"location"`
	OldContent    string         `json:"old_content"`
	NewContent    string         `json:"new_content"`
	Detail        map[string]any `json:"detail,omitempty"`
	Actor         string         `json:"actor"`
	CreateTime    string         `json:"create_time"`
}

// Log inserts one activity row. It is best-effort: failures are logged and
// swallowed so a logging problem never breaks the user's correction action.
func Log(ctx context.Context, db *sql.DB, a Activity) {
	if db == nil {
		logger.Warn("skip activity log: nil db handle", "activity_type", a.ActivityType)
		return
	}
	var detailJSON any
	if len(a.Detail) > 0 {
		b, err := json.Marshal(a.Detail)
		if err != nil {
			logger.Warn("marshal activity detail failed", "activity_type", a.ActivityType, "error", err)
		} else {
			detailJSON = b
		}
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO kb.doc_review_activities
			(activity_type, input_record_id, run_id, report_id, finding_id,
			 page_number, line_number, location, old_content, new_content, detail, actor)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ActivityType, a.InputRecordID, nullInt(a.RunID), nullInt(a.ReportID),
		nullInt(a.FindingID), nullIntI(a.PageNumber), nullIntI(a.LineNumber),
		nullStr(a.Location), a.OldContent, a.NewContent, detailJSON, nullStr(a.Actor),
	)
	if err != nil {
		logger.Warn("insert doc review activity failed", "activity_type", a.ActivityType,
			"input_record_id", a.InputRecordID, "error", err)
		return
	}
	logger.Info("doc review activity logged", "activity_type", a.ActivityType,
		"input_record_id", a.InputRecordID, "run_id", a.RunID,
		"report_id", a.ReportID, "finding_id", a.FindingID)
}

// List returns the activities for a document record, newest first. When
// runID is non-zero, results are restricted to that run plus any run-agnostic
// activities (Document Structure edits carry no run id). This is the source
// for the Document Review Correction Report.
func List(ctx context.Context, db *sql.DB, inputRecordID int64, runID int64) ([]Activity, error) {
	if db == nil {
		return nil, fmt.Errorf("no database handle")
	}
	query := `
		SELECT id, activity_type, input_record_id, COALESCE(run_id,0),
		       COALESCE(report_id,0), COALESCE(finding_id,0),
		       COALESCE(page_number,0), COALESCE(line_number,0),
		       COALESCE(location,''), COALESCE(old_content,''), COALESCE(new_content,''),
		       COALESCE(detail::text,''), COALESCE(actor,''), create_time::text
		FROM kb.doc_review_activities
		WHERE input_record_id = $1`
	args := []any{inputRecordID}
	if runID > 0 {
		query += ` AND (run_id = $2 OR run_id IS NULL)`
		args = append(args, runID)
	}
	query += ` ORDER BY create_time, id`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	var out []Activity
	for rows.Next() {
		var a Activity
		var detailRaw string
		if err := rows.Scan(&a.ID, &a.ActivityType, &a.InputRecordID, &a.RunID,
			&a.ReportID, &a.FindingID, &a.PageNumber, &a.LineNumber,
			&a.Location, &a.OldContent, &a.NewContent, &detailRaw, &a.Actor, &a.CreateTime); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		if detailRaw != "" {
			_ = json.Unmarshal([]byte(detailRaw), &a.Detail)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullIntI(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
