package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const EntryTypeLLMCall = "llm_call"
const EntryTypeSummary = "doc_proc_summary"

// DocProcLogRecord is a single log entry inserted into kb.doc_proc_logs.
type DocProcLogRecord struct {
	CallReason    string
	DocProcName   string
	ModelNames    []string
	PromptName    string
	EntryType     string // EntryTypeLLMCall | EntryTypeSummary
	Pass          *int
	LLMCallID     *string
	ActivityName  *string
	ArtifactJSON  *string // serialized JSON, nil → NULL
	Errors        *string
	ExtraInfoJSON *string // serialized JSON, nil → NULL
	MSUsed        *int64
}

// DocProcLogFilter specifies optional server-side filters for ListDocProcLogs.
type DocProcLogFilter struct {
	EntryType   string // empty = all
	DocProcName string // empty = all
	LLMCallID   string // empty = all
	Page        int    // 1-based; 0 treated as 1
	PageSize    int    // 0 → 50
	OrderBy     string // empty → create_time
	OrderDir    string // empty → desc
}

// DocProcLogRow is a row returned from ListDocProcLogs.
type DocProcLogRow struct {
	ID            int64
	CallReason    string
	DocProcName   string
	ModelNames    []string
	PromptName    string
	EntryType     string
	Pass          *int
	LLMCallID     *string
	ActivityName  *string
	ArtifactJSON  *string
	Errors        *string
	ExtraInfoJSON *string
	MSUsed        *int64
	CreateTime    string
}

// DocProcLogger wraps a db connection and exposes logging helpers for processors.
type DocProcLogger struct {
	DB *sql.DB
}

// LogLLMCall inserts an llm_call entry. Callers marshal the artifact to JSON before passing it.
func (l DocProcLogger) LogLLMCall(ctx context.Context, rec DocProcLogRecord) error {
	rec.EntryType = EntryTypeLLMCall
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec)
}

// LogSummary inserts a doc_proc_summary entry.
func (l DocProcLogger) LogSummary(ctx context.Context, rec DocProcLogRecord) error {
	rec.EntryType = EntryTypeSummary
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec)
}

func insertDocProcLog(ctx context.Context, db *sql.DB, rec DocProcLogRecord) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if strings.TrimSpace(rec.DocProcName) == "" {
		return errors.New("doc_proc_name is required")
	}
	if rec.EntryType != EntryTypeLLMCall && rec.EntryType != EntryTypeSummary {
		return errors.New("entry_type must be 'llm_call' or 'doc_proc_summary'")
	}

	modelNamesArr := "{}"
	if len(rec.ModelNames) > 0 {
		modelNamesArr = "{" + strings.Join(rec.ModelNames, ",") + "}"
	}

	const stmt = `
INSERT INTO kb.doc_proc_logs (
    call_reason,
    doc_proc_name,
    model_names,
    prompt_name,
    entry_type,
    pass,
    llm_call_id,
    activity_name,
    artifact,
    errors,
    extra_info,
    ms_used,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8,
    $9::jsonb, $10, $11::jsonb,
    $12, NOW()
)`
	_, err := db.ExecContext(ctx, stmt,
		nullableString(rec.CallReason),
		rec.DocProcName,
		modelNamesArr,
		nullableString(rec.PromptName),
		rec.EntryType,
		rec.Pass,
		rec.LLMCallID,
		rec.ActivityName,
		rec.ArtifactJSON,
		rec.Errors,
		rec.ExtraInfoJSON,
		rec.MSUsed,
	)
	return err
}

// InsertDocProcLog inserts a log record from an SQLStore (satisfies existing pattern).
func (s SQLStore) InsertDocProcLog(ctx context.Context, rec DocProcLogRecord) error {
	return insertDocProcLog(ctx, resolveDocProcLogDB(s.DB), rec)
}

// ListDocProcLogs returns paginated log rows with optional filtering.
func (s SQLStore) ListDocProcLogs(ctx context.Context, f DocProcLogFilter) ([]DocProcLogRow, int64, error) {
	db := resolveDocProcLogDB(s.DB)
	if db == nil {
		return nil, 0, errors.New("db is nil")
	}
	pageSize := f.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	var where []string
	var args []any
	argIdx := 1

	if f.EntryType != "" {
		where = append(where, "entry_type = $"+itoa(argIdx))
		args = append(args, f.EntryType)
		argIdx++
	}
	if f.DocProcName != "" {
		where = append(where, "doc_proc_name = $"+itoa(argIdx))
		args = append(args, f.DocProcName)
		argIdx++
	}
	if f.LLMCallID != "" {
		where = append(where, "llm_call_id = $"+itoa(argIdx))
		args = append(args, f.LLMCallID)
		argIdx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	countStmt := "SELECT COUNT(*) FROM kb.doc_proc_logs " + whereClause
	var total int64
	if err := db.QueryRowContext(ctx, countStmt, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := docProcLogOrderBy(f.OrderBy)
	orderDir := docProcLogOrderDir(f.OrderDir)
	listArgs := append(args, pageSize, offset)
	listStmt := `
SELECT id, COALESCE(call_reason,''), doc_proc_name,
       COALESCE(model_names, ARRAY[]::text[]),
       COALESCE(prompt_name,''), entry_type,
       pass, llm_call_id, activity_name,
       artifact::text, errors, extra_info::text,
       ms_used, COALESCE(to_char(create_time, 'YYYY-MM-DD\"T\"HH24:MI:SSOF'), '')
FROM kb.doc_proc_logs
` + whereClause + `
ORDER BY ` + orderBy + ` ` + orderDir + `
LIMIT $` + itoa(argIdx) + ` OFFSET $` + itoa(argIdx+1)

	rows, err := db.QueryContext(ctx, listStmt, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []DocProcLogRow
	for rows.Next() {
		var r DocProcLogRow
		var modelArr []string
		var artifact, extraInfo sql.NullString
		var msUsed sql.NullInt64
		if err := rows.Scan(
			&r.ID,
			&r.CallReason,
			&r.DocProcName,
			pq_text_array(&modelArr),
			&r.PromptName,
			&r.EntryType,
			&r.Pass,
			&r.LLMCallID,
			&r.ActivityName,
			&artifact,
			&r.Errors,
			&extraInfo,
			&msUsed,
			&r.CreateTime,
		); err != nil {
			return nil, 0, err
		}
		r.ModelNames = modelArr
		if artifact.Valid {
			r.ArtifactJSON = &artifact.String
		}
		if extraInfo.Valid {
			r.ExtraInfoJSON = &extraInfo.String
		}
		if msUsed.Valid {
			r.MSUsed = &msUsed.Int64
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}

func docProcLogOrderBy(orderBy string) string {
	switch strings.TrimSpace(orderBy) {
	case "entry_type":
		return "entry_type"
	case "doc_proc_name":
		return "doc_proc_name"
	case "activity_name":
		return "activity_name"
	case "model_names":
		return "model_names"
	case "pass":
		return "pass"
	case "ms_used":
		return "ms_used"
	case "errors":
		return "errors"
	default:
		return "create_time"
	}
}

func docProcLogOrderDir(orderDir string) string {
	if strings.EqualFold(strings.TrimSpace(orderDir), "asc") {
		return "ASC"
	}
	return "DESC"
}

// DeleteOldDocProcLogs removes log entries older than retentionDays days.
// Returns the number of rows deleted.
func (s SQLStore) DeleteOldDocProcLogs(ctx context.Context, retentionDays int) (int64, error) {
	db := resolveDocProcLogDB(s.DB)
	if db == nil {
		return 0, errors.New("db is nil")
	}
	if retentionDays < 1 {
		return 0, errors.New("retentionDays must be >= 1")
	}
	const stmt = `
DELETE FROM kb.doc_proc_logs
WHERE create_time < NOW() - ($1 || ' days')::interval`
	res, err := db.ExecContext(ctx, stmt, itoa(retentionDays))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func resolveDocProcLogDB(db *sql.DB) *sql.DB {
	if db != nil {
		return db
	}
	return ApiTypes.ProjectDBHandle
}

func int64Ptr(v int64) *int64 {
	return &v
}

// nullableString converts an empty string to nil so the DB stores NULL.
func nullableString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// itoa is a compact int-to-string helper used for query building only.
func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// pq_text_array returns a scan destination that decodes a PostgreSQL text[] column into []string.
// We avoid importing lib/pq directly; instead we use the TextArray wrapper below.
func pq_text_array(dest *[]string) interface{ Scan(src any) error } {
	return &pgTextArray{dest: dest}
}

type pgTextArray struct{ dest *[]string }

func (a *pgTextArray) Scan(src any) error {
	if src == nil {
		*a.dest = nil
		return nil
	}
	var raw string
	switch v := src.(type) {
	case []byte:
		raw = string(v)
	case string:
		raw = v
	default:
		*a.dest = nil
		return nil
	}
	// PostgreSQL text[] format: {elem1,elem2,...} or {"elem with space","elem2"}
	raw = strings.TrimSpace(raw)
	if raw == "{}" || raw == "" {
		*a.dest = []string{}
		return nil
	}
	raw = strings.TrimPrefix(raw, "{")
	raw = strings.TrimSuffix(raw, "}")
	if raw == "" {
		*a.dest = []string{}
		return nil
	}
	// Simple CSV split — elements containing commas must be quoted.
	// For model names this is sufficient.
	parts := splitPGArray(raw)
	*a.dest = parts
	return nil
}

func splitPGArray(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case c == ',' && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
