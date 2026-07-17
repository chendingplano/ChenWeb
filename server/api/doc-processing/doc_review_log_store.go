package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DocReviewLogFilter specifies optional server-side filters for review log listing.
type DocReviewLogFilter struct {
	InputRecordID, RunID           *int64
	Pass, Aspect, UnitType         string
	UnitKey, Outcome               string
	CreateTimeStart, CreateTimeEnd *time.Time
	Page, PageSize                 int
}

// DocReviewLogRow is one row returned from kb.doc_review_logs.
type DocReviewLogRow struct {
	ID, InputRecordID, RunID             int64
	Pass, Aspect, UnitType, UnitKey      string
	UnitLocation, MatchedUnits, Findings json.RawMessage
	Outcome                              string
	Detail                               json.RawMessage
	CreateTime                           string
}

// DocReviewLogSQLStore provides read-only access to reviewer audit logs.
type DocReviewLogSQLStore struct{ DB *sql.DB }

func (s DocReviewLogSQLStore) ListDocReviewLogs(ctx context.Context, f DocReviewLogFilter) ([]DocReviewLogRow, int64, error) {
	if s.DB == nil {
		return nil, 0, fmt.Errorf("db is nil")
	}
	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	where, args, index := make([]string, 0, 9), make([]any, 0, 9), 1
	add := func(column, operator string, value any) {
		where = append(where, column+" "+operator+" $"+strconv.Itoa(index))
		args = append(args, value)
		index++
	}
	if f.InputRecordID != nil {
		add("input_record_id", "=", *f.InputRecordID)
	}
	if f.RunID != nil {
		add("run_id", "=", *f.RunID)
	}
	if f.Pass != "" {
		add("pass", "=", f.Pass)
	}
	if f.Aspect != "" {
		add("aspect", "=", f.Aspect)
	}
	if f.UnitType != "" {
		add("unit_type", "=", f.UnitType)
	}
	if f.UnitKey != "" {
		where = append(where, "unit_key ILIKE $"+strconv.Itoa(index)+" ESCAPE '\\'")
		args = append(args, "%"+escapeDocReviewLogLike(f.UnitKey)+"%")
		index++
	}
	if f.Outcome != "" {
		add("outcome", "=", f.Outcome)
	}
	if f.CreateTimeStart != nil {
		add("create_time", ">=", *f.CreateTimeStart)
	}
	if f.CreateTimeEnd != nil {
		add("create_time", "<=", *f.CreateTimeEnd)
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM kb.doc_review_logs "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	stmt := `
SELECT id, input_record_id, run_id, pass, aspect, unit_type, unit_key,
       unit_location::text, matched_units::text, findings::text, outcome,
       detail::text, COALESCE(to_char(create_time, 'YYYY-MM-DD"T"HH24:MI:SSTZH:TZM'), '')
FROM kb.doc_review_logs
` + whereClause + `
ORDER BY create_time DESC, id DESC
LIMIT $` + strconv.Itoa(index) + ` OFFSET $` + strconv.Itoa(index+1)
	rows, err := s.DB.QueryContext(ctx, stmt, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]DocReviewLogRow, 0)
	for rows.Next() {
		var row DocReviewLogRow
		var unitLocation, matchedUnits, findings, detail sql.NullString
		if err := rows.Scan(&row.ID, &row.InputRecordID, &row.RunID, &row.Pass, &row.Aspect, &row.UnitType, &row.UnitKey, &unitLocation, &matchedUnits, &findings, &row.Outcome, &detail, &row.CreateTime); err != nil {
			return nil, 0, err
		}
		row.UnitLocation, row.MatchedUnits, row.Findings, row.Detail = rawDocReviewLogJSON(unitLocation), rawDocReviewLogJSON(matchedUnits), rawDocReviewLogJSON(findings), rawDocReviewLogJSON(detail)
		result = append(result, row)
	}
	return result, total, rows.Err()
}

func escapeDocReviewLogLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}

func rawDocReviewLogJSON(value sql.NullString) json.RawMessage {
	if !value.Valid {
		return nil
	}
	return json.RawMessage(value.String)
}
