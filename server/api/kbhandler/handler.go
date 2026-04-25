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
	ParserName     *string         `json:"parser_name,omitempty"`
	Type           string          `json:"type"`
	TenantID       *string         `json:"tenant_id,omitempty"`
	KSStoreID      *int64          `json:"ks_store_id,omitempty"`
	Title          *string         `json:"title,omitempty"`
	DocNo          *string         `json:"doc_no,omitempty"`
	KSDesc         *string         `json:"ks_desc,omitempty"`
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
	DocMetadata    json.RawMessage `json:"doc_metadata,omitempty"`
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

type listInputsFilters struct {
	RecordID        *int64
	DocType         string
	ParseState      string
	Name            string
	Title           string
	DocNo           string
	FileName        string
	ParserName      string
	Operation       string
	ProcStatus      string
	CreateTimeStart *time.Time
	CreateTimeEnd   *time.Time
	ModifyTimeStart *time.Time
	ModifyTimeEnd   *time.Time
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

	recordID, err := parseOptionalPositiveInt64(c.QueryParam("record_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: err.Error(),
		})
	}

	createStartTime, err := parseTimeQuery(firstNonEmpty(c.QueryParam("create_start_time"), c.QueryParam("start_time")))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid start_time: %v (CWB_KB_011)", err),
		})
	}
	createEndTime, err := parseTimeQuery(firstNonEmpty(c.QueryParam("create_end_time"), c.QueryParam("end_time")))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid end_time: %v (CWB_KB_012)", err),
		})
	}
	modifyStartTime, err := parseTimeQuery(c.QueryParam("modify_start_time"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid modify_start_time: %v (CWB_KB_013)", err),
		})
	}
	modifyEndTime, err := parseTimeQuery(c.QueryParam("modify_end_time"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid modify_end_time: %v (CWB_KB_014)", err),
		})
	}

	filters := listInputsFilters{
		RecordID:        recordID,
		DocType:         c.QueryParam("doc_type"),
		ParseState:      c.QueryParam("parse_state"),
		Name:            c.QueryParam("name"),
		Title:           c.QueryParam("title"),
		DocNo:           c.QueryParam("doc_no"),
		FileName:        c.QueryParam("file_name"),
		ParserName:      c.QueryParam("parser_name"),
		Operation:       c.QueryParam("operation"),
		ProcStatus:      c.QueryParam("proc_status"),
		CreateTimeStart: createStartTime,
		CreateTimeEnd:   createEndTime,
		ModifyTimeStart: modifyStartTime,
		ModifyTimeEnd:   modifyEndTime,
	}

	if !isValidParseState(filters.ParseState) {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid parse_state: %q (CWB_KB_031)", strings.TrimSpace(strings.ToLower(filters.ParseState))),
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

	var nameWhereExpr string
	if strings.TrimSpace(filters.Name) != "" {
		nameWhereExpr, err = resolveNameColumnExpr(db, inputTable)
		if err != nil {
			logger.Error("resolve kb input name column failed", "table", inputTable, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to resolve kb input schema (CWB_KB_023)",
			})
		}
	}

	var parserWhereExpr string
	if strings.TrimSpace(filters.ParserName) != "" {
		parserWhereExpr, err = resolveParserNameColumnExpr(db, inputTable)
		if err != nil {
			logger.Error("resolve kb input parser_name column failed", "table", inputTable, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to resolve kb input schema (CWB_KB_024)",
			})
		}
	}

	whereSQL, args, err := buildWhereClause(filters, nameWhereExpr, parserWhereExpr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: err.Error(),
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
	parserNameColumnExpr, err := resolveParserNameColumnExpr(db, inputTable)
	if err != nil {
		logger.Error("resolve kb input parser_name column failed", "table", inputTable, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input schema (CWB_KB_024)",
		})
	}

	results, err := queryInputs(db, inputTable, nameColumnExpr, parserNameColumnExpr, whereSQL, args, pageSize, offset)
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

func queryInputs(db *sql.DB, inputTable, nameColumnExpr, parserNameColumnExpr, whereSQL string, args []any, limit, offset int) ([]inputRecord, error) {
	query := fmt.Sprintf(`
SELECT
    i.id,
    %s AS name,
    %s AS parser_name,
    i.type,
    i.tenant_id,
    i.ks_store_id,
    i.title,
    i.doc_no,
    i.ks_desc,
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
    i.doc_metadata::text,
    i.notes,
    i.error_msg
FROM %s i
`, nameColumnExpr, parserNameColumnExpr, inputTable)
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
			docMetadataNullable sql.NullString
		)

		if err := rows.Scan(
			&record.ID,
			&record.Name,
			&record.ParserName,
			&record.Type,
			&record.TenantID,
			&record.KSStoreID,
			&record.Title,
			&record.DocNo,
			&record.KSDesc,
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
			&docMetadataNullable,
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
		if docMetadataNullable.Valid {
			docMetadataText := strings.TrimSpace(docMetadataNullable.String)
			if docMetadataText != "" && docMetadataText != "null" {
				record.DocMetadata = json.RawMessage([]byte(docMetadataText))
			}
		}

		out = append(out, record)
	}
	return out, rows.Err()
}

func resolveParserNameColumnExpr(db *sql.DB, inputTable string) (string, error) {
	schema, table, err := splitQualifiedTable(inputTable)
	if err != nil {
		return "", err
	}

	hasParserName, err := columnExists(db, schema, table, "parser_name")
	if err != nil {
		return "", err
	}
	if hasParserName {
		return "COALESCE(i.parser_name, '')", nil
	}
	return "''", nil
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

func buildWhereClause(filters listInputsFilters, nameColumnExprs ...string) (string, []any, error) {
	whereParts := make([]string, 0)
	args := make([]any, 0)
	nextArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	nameColumnExpr := "COALESCE(i.name, i.staging_filename, '')"
	if len(nameColumnExprs) > 0 && strings.TrimSpace(nameColumnExprs[0]) != "" {
		nameColumnExpr = fmt.Sprintf("COALESCE(%s, '')", strings.TrimSpace(nameColumnExprs[0]))
	}
	parserNameExpr := "COALESCE(i.parser_name, '')"
	if len(nameColumnExprs) > 1 && strings.TrimSpace(nameColumnExprs[1]) != "" {
		parserNameExpr = strings.TrimSpace(nameColumnExprs[1])
	}

	if filters.RecordID != nil {
		whereParts = append(whereParts, fmt.Sprintf("i.id = %s", nextArg(*filters.RecordID)))
	}

	docType := strings.TrimSpace(strings.ToLower(filters.DocType))
	if docType != "" && docType != "all" {
		whereParts = append(whereParts, fmt.Sprintf("LOWER(i.type) = LOWER(%s)", nextArg(docType)))
	}

	parseState := normalizeParseState(filters.ParseState)
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

	if title := strings.TrimSpace(filters.Title); title != "" {
		whereParts = append(whereParts, fmt.Sprintf("COALESCE(i.title, '') ILIKE %s", nextArg("%"+title+"%")))
	}
	if docNo := strings.TrimSpace(filters.DocNo); docNo != "" {
		whereParts = append(whereParts, fmt.Sprintf("COALESCE(i.doc_no, '') ILIKE %s", nextArg("%"+docNo+"%")))
	}
	if name := strings.TrimSpace(filters.Name); name != "" {
		whereParts = append(whereParts, fmt.Sprintf("%s ILIKE %s", nameColumnExpr, nextArg("%"+name+"%")))
	}
	fileName := strings.TrimSpace(filters.FileName)
	if fileName != "" {
		whereParts = append(whereParts, fmt.Sprintf("COALESCE(i.file_name, '') ILIKE %s", nextArg("%"+fileName+"%")))
	}
	if parserName := strings.TrimSpace(filters.ParserName); parserName != "" {
		whereParts = append(whereParts, fmt.Sprintf("%s ILIKE %s", parserNameExpr, nextArg("%"+parserName+"%")))
	}
	operation := strings.TrimSpace(filters.Operation)
	procStatus := strings.TrimSpace(filters.ProcStatus)
	if operation != "" || procStatus != "" {
		statusParts := make([]string, 0, 2)
		if operation != "" {
			statusParts = append(statusParts, fmt.Sprintf("LOWER(COALESCE(st->>'operation', '')) = LOWER(%s)", nextArg(operation)))
		}
		if procStatus != "" {
			statusParts = append(statusParts, fmt.Sprintf("LOWER(COALESCE(NULLIF(st->>'proc_status', ''), NULLIF(st->>'proc-status', ''), st->>'status', '')) = LOWER(%s)", nextArg(procStatus)))
		}
		whereParts = append(whereParts, fmt.Sprintf(`EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(i.status, '[]'::jsonb)) AS st
			WHERE %s
		)`, strings.Join(statusParts, " AND ")))
	}

	if filters.CreateTimeStart != nil {
		whereParts = append(whereParts, fmt.Sprintf("i.create_time >= %s", nextArg(*filters.CreateTimeStart)))
	}
	if filters.CreateTimeEnd != nil {
		whereParts = append(whereParts, fmt.Sprintf("i.create_time <= %s", nextArg(*filters.CreateTimeEnd)))
	}
	if filters.ModifyTimeStart != nil {
		whereParts = append(whereParts, fmt.Sprintf("i.modify_time >= %s", nextArg(*filters.ModifyTimeStart)))
	}
	if filters.ModifyTimeEnd != nil {
		whereParts = append(whereParts, fmt.Sprintf("i.modify_time <= %s", nextArg(*filters.ModifyTimeEnd)))
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

func isValidParseState(raw string) bool {
	switch normalizeParseState(raw) {
	case "all", "pending", "parsed_success", "parsed_failed":
		return true
	default:
		return false
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

func parseOptionalPositiveInt64(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("invalid record_id: %q (CWB_KB_015)", raw)
	}
	return &n, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
