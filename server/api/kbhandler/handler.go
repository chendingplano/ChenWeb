package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
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
	KsStoreID       *int64
	DocType         string
	ParseState      string
	Name            string
	Title           string
	DocNo           string
	FileName        string
	ParserName      string
	Operation       string
	ProcStatus      string
	PipelineFilter  string
	ExcludeDocType  string
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

	ksStoreID, err := parseOptionalPositiveInt64(c.QueryParam("ks_store_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid ks_store_id: %v (CWB_KB_015)", err),
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
		KsStoreID:       ksStoreID,
		DocType:         c.QueryParam("doc_type"),
		ParseState:      c.QueryParam("parse_state"),
		Name:            c.QueryParam("name"),
		Title:           c.QueryParam("title"),
		DocNo:           c.QueryParam("doc_no"),
		FileName:        c.QueryParam("file_name"),
		ParserName:      c.QueryParam("parser_name"),
		Operation:       c.QueryParam("operation"),
		ProcStatus:      c.QueryParam("proc_status"),
		PipelineFilter:  c.QueryParam("pipeline_filter"),
		ExcludeDocType:  c.QueryParam("exclude_doc_type"),
		CreateTimeStart: createStartTime,
		CreateTimeEnd:   createEndTime,
		ModifyTimeStart: modifyStartTime,
		ModifyTimeEnd:   modifyEndTime,
	}

	/*
	logger.Info("list kb inputs request",
		"page", page,
		"page_size", pageSize,
		"record_id", optionalInt64Value(recordID),
		"ks_store_id", optionalInt64Value(ksStoreID),
		"doc_type", strings.TrimSpace(filters.DocType),
		"parse_state", strings.TrimSpace(filters.ParseState),
		"name", strings.TrimSpace(filters.Name),
		"title", strings.TrimSpace(filters.Title),
		"doc_no", strings.TrimSpace(filters.DocNo),
		"file_name", strings.TrimSpace(filters.FileName),
		"parser_name", strings.TrimSpace(filters.ParserName),
		"operation", strings.TrimSpace(filters.Operation),
		"proc_status", strings.TrimSpace(filters.ProcStatus),
		"pipeline_filter", strings.TrimSpace(filters.PipelineFilter),
		"exclude_doc_type", strings.TrimSpace(filters.ExcludeDocType),
		"create_start_time", formatOptionalTime(filters.CreateTimeStart),
		"create_end_time", formatOptionalTime(filters.CreateTimeEnd),
		"modify_start_time", formatOptionalTime(filters.ModifyTimeStart),
		"modify_end_time", formatOptionalTime(filters.ModifyTimeEnd),
	)
	*/

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

	results, err := queryInputs(db, inputTable, nameColumnExpr, parserNameColumnExpr, whereSQL, args, pageSize, offset, c.QueryParam("order_by"), c.QueryParam("order_dir"))
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

	// Auto-heal stuck pipelines: doc_processing is "running" with a named
	// processor, but all direct processor entries are final — the pipeline
	// coordinator deferred finalization never ran (typically a Phase C crash).
	if strings.EqualFold(strings.TrimSpace(filters.Operation), "doc_processing") &&
		strings.EqualFold(strings.TrimSpace(filters.ProcStatus), "running") {
		store := docprocessing.StopRequestSQLStore{DB: db}
		kept := make([]inputRecord, 0, len(results))
		fixedCount := 0
		for _, rec := range results {
			stuck, fixErr := store.FixStuckPipeline(c.Request().Context(), rec.ID)
			if fixErr != nil {
				logger.Warn("stuck pipeline check failed", "record_id", rec.ID, "error", fixErr)
				kept = append(kept, rec)
				continue
			}
			if stuck {
				logger.Info("auto-fixed stuck pipeline", "record_id", rec.ID)
				fixedCount++
				continue
			}
			kept = append(kept, rec)
		}
		results = kept
		total -= int64(fixedCount)
		if total < 0 {
			total = 0
		}
	}

	// logger.Info("list kb inputs result",
	// 	"record_id", optionalInt64Value(recordID),
	// 	"ks_store_id", optionalInt64Value(ksStoreID),
	// 	"total", total,
	// 	"returned", len(results),
	// 	"sample_ids", sampleInputIDs(results, 8),
	// )

	return c.JSON(http.StatusOK, listInputsResponse{
		Status:   true,
		Results:  results,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

func optionalInt64Value(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func formatOptionalTime(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.Format(time.RFC3339)
}

func sampleInputIDs(records []inputRecord, limit int) []int64 {
	if limit <= 0 || len(records) == 0 {
		return []int64{}
	}
	if len(records) < limit {
		limit = len(records)
	}
	out := make([]int64, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, records[i].ID)
	}
	return out
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

// resolveOrderClause maps a client-supplied order_by/order_dir pair to a safe
// SQL ORDER BY clause. Only whitelisted columns are sortable; anything else
// falls back to the default create_time ordering. A secondary sort on i.id
// keeps results deterministic across pages.
func resolveOrderClause(orderBy, orderDir, parserNameColumnExpr string) string {
	columns := map[string]string{
		"id":          "i.id",
		"title":       "i.title",
		"doc_no":      "i.doc_no",
		"type":        "i.type",
		"file_name":   "i.file_name",
		"parser_name": parserNameColumnExpr,
		"create_time": "i.create_time",
		"modify_time": "i.modify_time",
	}

	expr, ok := columns[strings.ToLower(strings.TrimSpace(orderBy))]
	if !ok {
		expr = "i.create_time"
		orderDir = "desc"
	}

	dir := "DESC"
	if strings.EqualFold(strings.TrimSpace(orderDir), "asc") {
		dir = "ASC"
	}

	return fmt.Sprintf("ORDER BY %s %s NULLS LAST, i.id %s", expr, dir, dir)
}

func queryInputs(db *sql.DB, inputTable, nameColumnExpr, parserNameColumnExpr, whereSQL string, args []any, limit, offset int, orderBy, orderDir string) ([]inputRecord, error) {
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
	orderClause := resolveOrderClause(orderBy, orderDir, parserNameColumnExpr)
	query += fmt.Sprintf(" %s LIMIT $%d OFFSET $%d", orderClause, limitPos, offsetPos)
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

// mandatoryProcessorAliases maps each mandatory processor stage ID to all known
// operation names a status entry can carry. Mirrors the PIPELINE_STAGES mapping
// in the frontend state module. The first entry is always the canonical stage ID.
var mandatoryProcessorAliases = map[string][]string{
	"static_analyzer":      {"static_analyzer", "static_analzyer", "structure_analyzer"},
	"chunking":             {"chunking", "chunked"},
	"extract_doc_metadata": {"extract_doc_metadata", "extract_metadata"},
}

var (
	allProcessorAliasList [][]string
	loadProcessorsOnce    sync.Once
)

// getAllProcessorAliases returns one slice of aliases per expected processor
// (mandatory + configured). Each slice starts with the canonical stage ID and
// includes every known operation-name variant that might appear in
// kb.input_proc_status.processor. The result is cached after the first call.
func getAllProcessorAliases() [][]string {
	loadProcessorsOnce.Do(func() {
		cfg, err := LoadKbFrontendConfig()
		allProcs := mandatoryProcessorIDs
		if err == nil && len(cfg.RequiredProcessors) > 0 {
			allProcs = append(allProcs, cfg.RequiredProcessors...)
		}
		allProcessorAliasList = make([][]string, 0, len(allProcs))
		for _, id := range allProcs {
			if aliases, ok := mandatoryProcessorAliases[id]; ok {
				allProcessorAliasList = append(allProcessorAliasList, aliases)
			} else {
				allProcessorAliasList = append(allProcessorAliasList, []string{id})
			}
		}
	})
	return allProcessorAliasList
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
	if filters.KsStoreID != nil {
		whereParts = append(whereParts, fmt.Sprintf("i.ks_store_id = %s", nextArg(*filters.KsStoreID)))
	}

	docType := strings.TrimSpace(strings.ToLower(filters.DocType))
	if docType != "" && docType != "all" {
		whereParts = append(whereParts, fmt.Sprintf("LOWER(i.type) = LOWER(%s)", nextArg(docType)))
	}
	if excludeDocType := strings.TrimSpace(strings.ToLower(filters.ExcludeDocType)); excludeDocType != "" {
		whereParts = append(whereParts, fmt.Sprintf("LOWER(i.type) <> LOWER(%s)", nextArg(excludeDocType)))
	}

	// parse_state, pipeline_state and has_failed_proc are denormalized rollup
	// columns on kb.inputs, kept in sync with the status JSON by DB triggers (see
	// project_migrations/20260609000002_add_kb_inputs_status_rollups.sql). They
	// replace the per-row jsonb_array_elements scans these filters used to do.
	parseState := normalizeParseState(filters.ParseState)
	switch parseState {
	case "all":
	case "pending":
		whereParts = append(whereParts, "i.parse_state = 'pending'")
	case "parsing":
		whereParts = append(whereParts, "i.parse_state = 'parsing'")
	case "parsed_success":
		whereParts = append(whereParts, "i.parse_state = 'parsed_success'")
	case "parsed_failed":
		whereParts = append(whereParts, "i.parse_state = 'parsed_failed'")
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
	pipelineFilter := strings.TrimSpace(strings.ToLower(filters.PipelineFilter))
	switch pipelineFilter {
	case "", "all":
		operation := strings.TrimSpace(filters.Operation)
		procStatus := strings.TrimSpace(filters.ProcStatus)
		if operation != "" || procStatus != "" {
			// The aggregate "doc_processing" control entry lives in the
			// pipeline_state rollup, not the per-processor child table; map it
			// directly. Every other operation is a per-processor row.
			if canonicalOp(operation) == "doc_processing" {
				if procStatus != "" {
					whereParts = append(whereParts, fmt.Sprintf("i.pipeline_state = LOWER(%s)", nextArg(procStatus)))
				} else {
					whereParts = append(whereParts, "i.pipeline_state <> 'pending'")
				}
			} else {
				statusParts := []string{"ps.record_id = i.id"}
				if operation != "" {
					statusParts = append(statusParts, fmt.Sprintf("ps.processor = kb.canonical_op(%s)", nextArg(operation)))
				}
				if procStatus != "" {
					statusParts = append(statusParts, fmt.Sprintf("ps.proc_status = LOWER(%s)", nextArg(procStatus)))
				}
				existsClause := fmt.Sprintf(`EXISTS (
				SELECT 1 FROM kb.input_proc_status ps
				WHERE %s
			)`, strings.Join(statusParts, " AND "))
				if operation == "" && procStatus != "" {
					// A bare proc_status filter historically also matched the
					// aggregate "doc_processing" entry (which lives in
					// pipeline_state, not the child table); keep that behavior.
					whereParts = append(whereParts, fmt.Sprintf("(%s OR i.pipeline_state = LOWER(%s))", existsClause, nextArg(procStatus)))
				} else {
					whereParts = append(whereParts, existsClause)
				}
			}
		}
	case "parsed_success":
		whereParts = append(whereParts, "i.parse_state = 'parsed_success'")
	case "parse_failed":
		whereParts = append(whereParts, "i.parse_state = 'parsed_failed'")
	case "parsed_not_started":
		whereParts = append(whereParts, "i.parse_state = 'parsed_success' AND i.pipeline_state = 'pending'")
	case "all_processors_success":
		whereParts = append(whereParts, "i.pipeline_state = 'success'")
	case "failed_processors":
		whereParts = append(whereParts, "i.pipeline_state = 'failed'")
	case "with_not_started_procs":
		aliasList := getAllProcessorAliases()
		if len(aliasList) == 0 {
			whereParts = append(whereParts, "1=0")
			break
		}
		valueClauses := make([]string, len(aliasList))
		for i, aliases := range aliasList {
			aliasExprs := make([]string, len(aliases))
			for j, alias := range aliases {
				aliasExprs[j] = nextArg(alias)
			}
			valueClauses[i] = fmt.Sprintf("(ARRAY[%s]::text[])", strings.Join(aliasExprs, ", "))
		}
		valuesList := strings.Join(valueClauses, ", ")
		whereParts = append(whereParts, fmt.Sprintf(
			`i.parse_state = 'parsed_success' AND EXISTS (
				SELECT 1 FROM (VALUES %s) AS t(aliases)
				WHERE NOT EXISTS (
					SELECT 1 FROM kb.input_proc_status ps
					WHERE ps.record_id = i.id AND ps.processor = ANY(t.aliases)
				)
			)`,
			valuesList,
		))
	case "with_unfinished_procs":
		aliasList := getAllProcessorAliases()
		if len(aliasList) == 0 {
			whereParts = append(whereParts, "1=0")
			break
		}
		valueClauses := make([]string, len(aliasList))
		for i, aliases := range aliasList {
			aliasExprs := make([]string, len(aliases))
			for j, alias := range aliases {
				aliasExprs[j] = nextArg(alias)
			}
			valueClauses[i] = fmt.Sprintf("(ARRAY[%s]::text[])", strings.Join(aliasExprs, ", "))
		}
		valuesList := strings.Join(valueClauses, ", ")
		whereParts = append(whereParts, fmt.Sprintf(
			`i.parse_state = 'parsed_success' AND EXISTS (
				SELECT 1 FROM (VALUES %s) AS t(aliases)
				WHERE NOT EXISTS (
					SELECT 1 FROM kb.input_proc_status ps
					WHERE ps.record_id = i.id AND ps.processor = ANY(t.aliases) AND ps.proc_status = 'success'
				)
			)`,
			valuesList,
		))
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

// canonicalOp mirrors canonicalOperationName in the docprocessing package and
// the kb.canonical_op SQL function: it lowercases, maps '-' to '_', and folds
// the two legacy operation aliases. Kept local to avoid a package dependency.
func canonicalOp(raw string) string {
	op := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), "-", "_")
	switch op {
	case "chunked":
		return "chunking"
	case "extract_metadata":
		return "extract_doc_metadata"
	default:
		return op
	}
}

func normalizeParseState(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "all":
		return "all"
	case "pending":
		return "pending"
	case "parsing", "active":
		return "parsing"
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
	case "all", "pending", "parsing", "parsed_success", "parsed_failed":
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
