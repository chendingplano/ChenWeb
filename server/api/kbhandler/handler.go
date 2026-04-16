package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const (
	defaultPageSize = 50
	maxPageSize     = 500
)

const (
	kbInputTableSingular = "kb.input"
	kbInputTablePlural   = "kb.inputs"
)

type inputRecord struct {
	ID             int64           `json:"id"`
	Name           *string         `json:"name,omitempty"`
	Type           string          `json:"type"`
	Title          *string         `json:"title,omitempty"`
	DocNo          *string         `json:"doc_no,omitempty"`
	Source         *string         `json:"source,omitempty"`
	FileName       *string         `json:"file_name,omitempty"`
	BackupFileName *string         `json:"backup_filename,omitempty"`
	ResultFileName *string         `json:"result_filename,omitempty"`
	PublishDate    *time.Time      `json:"publish_date,omitempty"`
	Authors        *string         `json:"authors,omitempty"`
	Owner          *int64          `json:"owner,omitempty"`
	Status         json.RawMessage `json:"status"`
	CreateTime     time.Time       `json:"create_time"`
	ModifyTime     time.Time       `json:"modify_time"`
	PublicInfo     json.RawMessage `json:"public_info,omitempty"`
	PrivateInfo    json.RawMessage `json:"private_info,omitempty"`
	Notes          *string         `json:"notes,omitempty"`
	ErrorMsg       *string         `json:"error_msg,omitempty"`
}

type listInputsResponse struct {
	Status   bool          `json:"status"`
	Results  []inputRecord `json:"results"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int64         `json:"total"`
}

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

// ListInputs handles GET /api/v1/kb/inputs.
func ListInputs(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_001")
	defer rc.Close()
	logger := rc.GetLogger()

	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	startTime, err := parseTimeQuery(c.QueryParam("start_time"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid start_time: %v (CWB_KB_011)", err),
		})
	}
	endTime, err := parseTimeQuery(c.QueryParam("end_time"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid end_time: %v (CWB_KB_012)", err),
		})
	}

	whereSQL, args, err := buildWhereClause(
		c.QueryParam("doc_type"),
		c.QueryParam("parse_state"),
		c.QueryParam("file_name"),
		startTime,
		endTime,
	)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: err.Error(),
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input table (CWB_KB_020)",
		})
	}

	total, err := queryTotalCount(db, inputTable, whereSQL, args)
	if err != nil {
		logger.Error("count kb input failed", "table", inputTable, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to count kb inputs (CWB_KB_021)",
		})
	}

	offset := (page - 1) * pageSize
	nameColumnExpr, err := resolveNameColumnExpr(db, inputTable)
	if err != nil {
		logger.Error("resolve kb input name column failed", "table", inputTable, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input schema (CWB_KB_023)",
		})
	}

	results, err := queryInputs(db, inputTable, nameColumnExpr, whereSQL, args, pageSize, offset)
	if err != nil {
		logger.Error("query kb input failed", "table", inputTable, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to retrieve kb inputs (CWB_KB_022)",
		})
	}

	if results == nil {
		results = []inputRecord{}
	}

	return c.JSON(http.StatusOK, listInputsResponse{
		Status:   true,
		Results:  results,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

func queryTotalCount(db *sql.DB, inputTable, whereSQL string, args []any) (int64, error) {
	base := fmt.Sprintf("SELECT COUNT(1) FROM %s i", inputTable)
	query := base
	if whereSQL != "" {
		query += " WHERE " + whereSQL
	}

	var total int64
	if err := db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func queryInputs(db *sql.DB, inputTable, nameColumnExpr, whereSQL string, args []any, limit, offset int) ([]inputRecord, error) {
	query := fmt.Sprintf(`
SELECT
    i.id,
    %s AS name,
    i.type,
    i.title,
    i.doc_no,
    i.source,
    i.file_name,
    i.backup_filename,
    i.result_filename,
    i.publish_date,
    i.authors,
    i.owner,
    COALESCE(i.status, '[]'::jsonb) AS status,
    i.create_time,
    i.modify_time,
    i.public_info,
    i.private_info,
    i.notes,
    i.error_msg
FROM %s i
`, nameColumnExpr, inputTable)
	if whereSQL != "" {
		query += " WHERE " + whereSQL
	}
	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	query += fmt.Sprintf(" ORDER BY i.create_time DESC LIMIT $%d OFFSET $%d", limitPos, offsetPos)
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]inputRecord, 0)
	for rows.Next() {
		var (
			record              inputRecord
			statusBytes         []byte
			publicInfoBytes     []byte
			privateInfoBytes    []byte
			publishDate         sql.NullTime
			publicInfoNullable  sql.NullString
			privateInfoNullable sql.NullString
		)

		if err := rows.Scan(
			&record.ID,
			&record.Name,
			&record.Type,
			&record.Title,
			&record.DocNo,
			&record.Source,
			&record.FileName,
			&record.BackupFileName,
			&record.ResultFileName,
			&publishDate,
			&record.Authors,
			&record.Owner,
			&statusBytes,
			&record.CreateTime,
			&record.ModifyTime,
			&publicInfoNullable,
			&privateInfoNullable,
			&record.Notes,
			&record.ErrorMsg,
		); err != nil {
			return nil, err
		}

		if publishDate.Valid {
			ts := publishDate.Time
			record.PublishDate = &ts
		}

		record.Status = json.RawMessage(statusBytes)
		if publicInfoNullable.Valid && strings.TrimSpace(publicInfoNullable.String) != "" {
			publicInfoBytes = []byte(publicInfoNullable.String)
			record.PublicInfo = json.RawMessage(publicInfoBytes)
		}
		if privateInfoNullable.Valid && strings.TrimSpace(privateInfoNullable.String) != "" {
			privateInfoBytes = []byte(privateInfoNullable.String)
			record.PrivateInfo = json.RawMessage(privateInfoBytes)
		}

		out = append(out, record)
	}
	return out, rows.Err()
}

func resolveInputTable(db *sql.DB) (string, error) {
	const query = `
SELECT
	to_regclass($1)::text AS singular,
	to_regclass($2)::text AS plural
`

	var singular sql.NullString
	var plural sql.NullString
	if err := db.QueryRow(query, kbInputTableSingular, kbInputTablePlural).Scan(&singular, &plural); err != nil {
		return "", err
	}

	if singular.Valid && strings.TrimSpace(singular.String) != "" {
		return singular.String, nil
	}
	if plural.Valid && strings.TrimSpace(plural.String) != "" {
		return plural.String, nil
	}
	return "", fmt.Errorf("neither %s nor %s exists", kbInputTableSingular, kbInputTablePlural)
}

func resolveNameColumnExpr(db *sql.DB, inputTable string) (string, error) {
	schema, table, err := splitQualifiedTable(inputTable)
	if err != nil {
		return "", err
	}

	hasName, err := columnExists(db, schema, table, "name")
	if err != nil {
		return "", err
	}
	if hasName {
		return "i.name", nil
	}

	hasStagingFilename, err := columnExists(db, schema, table, "staging_filename")
	if err != nil {
		return "", err
	}
	if hasStagingFilename {
		return "i.staging_filename", nil
	}

	return "", fmt.Errorf("neither name nor staging_filename exists on %s", inputTable)
}

func splitQualifiedTable(inputTable string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(inputTable), ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid table reference: %s", inputTable)
	}
	schema := strings.Trim(parts[0], `"`)
	table := strings.Trim(parts[1], `"`)
	if schema == "" || table == "" {
		return "", "", fmt.Errorf("invalid table reference: %s", inputTable)
	}
	return schema, table, nil
}

func columnExists(db *sql.DB, schema, table, column string) (bool, error) {
	const query = `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.columns
	WHERE table_schema = $1
	  AND table_name = $2
	  AND column_name = $3
)
`
	var exists bool
	if err := db.QueryRow(query, schema, table, column).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func buildWhereClause(docType, parseState, fileName string, startTime, endTime *time.Time) (string, []any, error) {
	whereParts := make([]string, 0)
	args := make([]any, 0)
	nextArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	docType = strings.TrimSpace(strings.ToLower(docType))
	if docType != "" && docType != "all" {
		whereParts = append(whereParts, fmt.Sprintf("LOWER(i.type) = LOWER(%s)", nextArg(docType)))
	}

	parseState = normalizeParseState(parseState)
	switch parseState {
	case "all":
	case "pending":
		whereParts = append(whereParts, `NOT EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(i.status, '[]'::jsonb)) AS st
			WHERE LOWER(COALESCE(st->>'operation', '')) IN ('parsing', 'parse')
		)`)
	case "parsed_success":
		whereParts = append(whereParts, `EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(i.status, '[]'::jsonb)) AS st
			WHERE LOWER(COALESCE(st->>'operation', '')) IN ('parsing', 'parse')
			  AND LOWER(COALESCE(st->>'status', '')) = 'success'
		)`)
	case "parsed_failed":
		whereParts = append(whereParts, `EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(i.status, '[]'::jsonb)) AS st
			WHERE LOWER(COALESCE(st->>'operation', '')) IN ('parsing', 'parse')
			  AND LOWER(COALESCE(st->>'status', '')) <> 'success'
		)`)
	default:
		return "", nil, fmt.Errorf("invalid parse_state: %q (CWB_KB_031)", parseState)
	}

	fileName = strings.TrimSpace(fileName)
	if fileName != "" {
		whereParts = append(whereParts, fmt.Sprintf("COALESCE(i.file_name, '') ILIKE %s", nextArg("%"+fileName+"%")))
	}

	if startTime != nil {
		whereParts = append(whereParts, fmt.Sprintf("i.create_time >= %s", nextArg(*startTime)))
	}
	if endTime != nil {
		whereParts = append(whereParts, fmt.Sprintf("i.create_time <= %s", nextArg(*endTime)))
	}

	return strings.Join(whereParts, " AND "), args, nil
}

func normalizeParseState(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "all":
		return "all"
	case "pending":
		return "pending"
	case "parsed_success", "success":
		return "parsed_success"
	case "parsed_failed", "failed":
		return "parsed_failed"
	default:
		return strings.TrimSpace(strings.ToLower(raw))
	}
}

func parsePositiveInt(raw string, defaultValue int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultValue
	}
	return n
}

func parseTimeQuery(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	// Inputs with timezone/offset should preserve that explicit timezone.
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}

	// Inputs without timezone (e.g. datetime-local) should be interpreted
	// in server local time so UI-selected local windows filter as expected.
	localLayouts := []string{
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("unsupported time format")
}
