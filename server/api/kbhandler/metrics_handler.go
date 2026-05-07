package kbhandler

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type metricRecord struct {
	ID                  int64           `json:"id"`
	InputRecordID       int64           `json:"input_record_id"`
	EventID             *string         `json:"event_id,omitempty"`
	InputFilename       string          `json:"input_filename"`
	MetricName          *string         `json:"metric_name,omitempty"`
	SourceLineSpans     json.RawMessage `json:"source_line_spans,omitempty"`
	MetricSubject       *string         `json:"metric_subject,omitempty"`
	MetricDesc          *string         `json:"metric_desc,omitempty"`
	MetricContext       *string         `json:"metric_context,omitempty"`
	MetricKeywords      json.RawMessage `json:"metric_keywords,omitempty"`
	LocationType        *string         `json:"location_type,omitempty"`
	MetricUnit          *string         `json:"metric_unit,omitempty"`
	MetricValue         *string         `json:"metric_value,omitempty"`
	ValueDataType       *string         `json:"value_data_type,omitempty"`
	ValueRangeType      *string         `json:"value_range_type,omitempty"`
	ValueClass          *string         `json:"value_class,omitempty"`
	ValueClassEn        *string         `json:"value_class_en,omitempty"`
	FormulaOrDefinition *string         `json:"formula_or_definition,omitempty"`
	ThresholdOrTarget   *string         `json:"threshold_or_target,omitempty"`
	MeasurementFreq     *string         `json:"measurement_frequency,omitempty"`
	Confidence          *float64        `json:"confidence,omitempty"`
	IsExplicitMetric    *bool           `json:"is_explicit_metric,omitempty"`
	ReasoningTags       json.RawMessage `json:"reasoning_tags,omitempty"`
	CreatedAt           string          `json:"created_at,omitempty"`
}

type listMetricsResponse struct {
	Status  bool           `json:"status"`
	Results []metricRecord `json:"results"`
	Total   int            `json:"total"`
}

// ListMetrics handles GET /api/v1/kb/metrics?input_record_id=N
func ListMetrics(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_001")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	if idStr == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "input_record_id is required (CWB_KB_M_010)",
		})
	}
	inputID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || inputID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid input_record_id (CWB_KB_M_011)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	const query = `
SELECT
    m.id, m.input_record_id, m.event_id, COALESCE(i.staging_filename, '') AS input_filename,
    m.metric_name, m.source_line_spans, m.metric_subject, m.metric_desc,
    m.metric_context, m.metric_keywords, m.location_type, m.metric_unit,
    m.metric_value, m.value_data_type, m.value_range_type, m.value_class, m.value_class_en,
    m.formula_or_definition, m.threshold_or_target, m.measurement_frequency,
    m.confidence, m.is_explicit_metric, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.input_record_id = $1
ORDER BY m.id ASC
`
	rows, err := db.Query(query, inputID)
	if err != nil {
		logger.Error("query kb.metrics failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to retrieve kb metrics (CWB_KB_M_020)",
		})
	}
	defer rows.Close()

	out := make([]metricRecord, 0)
	for rows.Next() {
		var (
			r              metricRecord
			spansBytes     []byte
			keywordsBytes  []byte
			reasoningBytes []byte
			confidence     sql.NullFloat64
			isExplicit     sql.NullBool
		)
		if err := rows.Scan(
			&r.ID, &r.InputRecordID, &r.EventID, &r.InputFilename,
			&r.MetricName, &spansBytes, &r.MetricSubject, &r.MetricDesc,
			&r.MetricContext, &keywordsBytes, &r.LocationType, &r.MetricUnit,
			&r.MetricValue, &r.ValueDataType, &r.ValueRangeType, &r.ValueClass, &r.ValueClassEn,
			&r.FormulaOrDefinition, &r.ThresholdOrTarget, &r.MeasurementFreq,
			&confidence, &isExplicit, &reasoningBytes, &r.CreatedAt,
		); err != nil {
			logger.Error("scan kb.metrics row failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to scan kb metrics (CWB_KB_M_021)",
			})
		}
		if len(spansBytes) > 0 {
			r.SourceLineSpans = json.RawMessage(spansBytes)
		}
		if len(keywordsBytes) > 0 {
			r.MetricKeywords = json.RawMessage(keywordsBytes)
		}
		if len(reasoningBytes) > 0 {
			r.ReasoningTags = json.RawMessage(reasoningBytes)
		}
		if confidence.Valid {
			v := confidence.Float64
			r.Confidence = &v
		}
		if isExplicit.Valid {
			v := isExplicit.Bool
			r.IsExplicitMetric = &v
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate kb.metrics failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to iterate kb metrics (CWB_KB_M_022)",
		})
	}

	return c.JSON(http.StatusOK, listMetricsResponse{
		Status:  true,
		Results: out,
		Total:   len(out),
	})
}

type inputDetailResponse struct {
	Status bool        `json:"status"`
	Record inputRecord `json:"record"`
}

func fetchInputRecordByID(db *sql.DB, inputTable string, id int64) (inputRecord, error) {
	nameColumnExpr, err := resolveNameColumnExpr(db, inputTable)
	if err != nil {
		return inputRecord{}, err
	}

	parserNameColumnExpr, err := resolveParserNameColumnExpr(db, inputTable)
	if err != nil {
		return inputRecord{}, err
	}

	query := fmt.Sprintf(`
SELECT
    i.id, %s AS name, %s AS parser_name, i.type, i.tenant_id, i.ks_store_id, i.title, i.doc_no, i.ks_desc, i.source,
    i.file_name, i.backup_filename, i.result_filename, i.publish_date,
    i.authors, i.owner, COALESCE(i.status, '[]'::jsonb) AS status,
    i.create_time, i.modify_time, i.public_info, i.private_info, i.doc_metadata::text,
    i.notes, i.error_msg
FROM %s i
WHERE i.id = $1
`, nameColumnExpr, parserNameColumnExpr, inputTable)

	row := db.QueryRow(query, id)
	var (
		record              inputRecord
		statusBytes         []byte
		publishDate         sql.NullTime
		publicInfoNullable  sql.NullString
		privateInfoNullable sql.NullString
		docMetadataNullable sql.NullString
	)
	if err := row.Scan(
		&record.ID, &record.Name, &record.ParserName, &record.Type, &record.TenantID, &record.KSStoreID, &record.Title, &record.DocNo, &record.KSDesc, &record.Source,
		&record.FileName, &record.BackupFileName, &record.ResultFileName, &publishDate,
		&record.Authors, &record.Owner, &statusBytes,
		&record.CreateTime, &record.ModifyTime, &publicInfoNullable, &privateInfoNullable, &docMetadataNullable,
		&record.Notes, &record.ErrorMsg,
	); err != nil {
		return inputRecord{}, err
	}
	if publishDate.Valid {
		ts := publishDate.Time
		record.PublishDate = &ts
	}
	record.Status = json.RawMessage(statusBytes)
	if publicInfoNullable.Valid && strings.TrimSpace(publicInfoNullable.String) != "" {
		record.PublicInfo = json.RawMessage([]byte(publicInfoNullable.String))
	}
	if privateInfoNullable.Valid && strings.TrimSpace(privateInfoNullable.String) != "" {
		record.PrivateInfo = json.RawMessage([]byte(privateInfoNullable.String))
	}
	if docMetadataNullable.Valid {
		docMetadataText := strings.TrimSpace(docMetadataNullable.String)
		if docMetadataText != "" && docMetadataText != "null" {
			record.DocMetadata = json.RawMessage([]byte(docMetadataText))
		}
	}
	return record, nil
}

// GetInput handles GET /api/v1/kb/inputs/:id
func GetInput(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_100")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid id (CWB_KB_M_110)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input table (CWB_KB_M_120)",
		})
	}
	record, err := fetchInputRecordByID(db, inputTable, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status:   false,
				ErrorMsg: "record not found (CWB_KB_M_130)",
			})
		}
		logger.Error("query kb input failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to retrieve kb input (CWB_KB_M_131)",
		})
	}

	return c.JSON(http.StatusOK, inputDetailResponse{Status: true, Record: record})
}

func decodeStringValue(raw json.RawMessage, trim bool) (*string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("missing value")
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("must be a string or null")
	}
	if trim {
		s = strings.TrimSpace(s)
	}
	return &s, nil
}

func compactJSONRaw(raw json.RawMessage) (string, error) {
	if !json.Valid(raw) {
		return "", fmt.Errorf("must be valid json")
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("must be valid json")
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("failed to encode json")
	}
	return string(encoded), nil
}

func decodeOwnerValue(raw json.RawMessage) (*int64, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var numeric int64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return &numeric, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("owner must be integer text")
		}
		return &v, nil
	}
	return nil, fmt.Errorf("owner must be integer or string")
}

func decodeInt64Value(raw json.RawMessage) (*int64, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var numeric int64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return &numeric, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("must be integer text")
		}
		return &v, nil
	}
	return nil, fmt.Errorf("must be integer or string")
}

func decodeAuthorsValue(raw json.RawMessage) (*string, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}

	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		clean := make([]string, 0, len(arr))
		for _, entry := range arr {
			v := strings.TrimSpace(entry)
			if v == "" {
				continue
			}
			clean = append(clean, v)
		}
		encoded, err := json.Marshal(clean)
		if err != nil {
			return nil, fmt.Errorf("failed to encode authors")
		}
		s := string(encoded)
		return &s, nil
	}

	var asText string
	if err := json.Unmarshal(raw, &asText); err == nil {
		asText = strings.TrimSpace(asText)
		if asText == "" {
			return nil, nil
		}
		return &asText, nil
	}

	return nil, fmt.Errorf("authors must be string[] or string")
}

// UpdateInput handles PUT /api/v1/kb/inputs/:id
func UpdateInput(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_140")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid id (CWB_KB_M_141)",
		})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid request body (CWB_KB_M_142)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input table (CWB_KB_M_143)",
		})
	}

	sets := make([]string, 0, len(payload)+1)
	args := make([]any, 0, len(payload)+1)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	sets = append(sets, "modify_time = NOW()")

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "title", "doc_no", "source", "tenant_id":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_144)", field, err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "ks_desc":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_145A)", field, err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "notes", "error_msg":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_145)", field, err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "publish_date":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid publish_date: %v (CWB_KB_M_146)", err),
				})
			}
			if value == nil || *value == "" {
				addSet(field, nil)
				break
			}
			ts, err := parseTimeQuery(*value)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid publish_date format: %v (CWB_KB_M_147)", err),
				})
			}
			if ts == nil {
				addSet(field, nil)
			} else {
				addSet(field, *ts)
			}

		case "authors":
			value, err := decodeAuthorsValue(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid authors: %v (CWB_KB_M_148)", err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "owner":
			value, err := decodeOwnerValue(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid owner: %v (CWB_KB_M_149)", err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "ks_store_id":
			value, err := decodeInt64Value(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid ks_store_id: %v (CWB_KB_M_149A)", err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "public_info", "private_info", "doc_metadata":
			if strings.TrimSpace(string(raw)) == "null" {
				addSet(field, nil)
				break
			}
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_150)", field, err),
				})
			}
			addSet(field, compact)
		}
	}

	if len(sets) <= 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "no editable fields in request (CWB_KB_M_151)",
		})
	}

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d", inputTable, strings.Join(sets, ", "), len(args)+1)
	args = append(args, id)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update kb input failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to update kb input (CWB_KB_M_152)",
		})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected kb input failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to verify kb input update (CWB_KB_M_153)",
		})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: "record not found (CWB_KB_M_154)",
		})
	}

	record, err := fetchInputRecordByID(db, inputTable, id)
	if err != nil {
		logger.Error("reload kb input after update failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to reload kb input (CWB_KB_M_155)",
		})
	}

	return c.JSON(http.StatusOK, inputDetailResponse{Status: true, Record: record})
}

type rawLine struct {
	LineNumber int       `json:"line_number"`
	PageNumber int       `json:"page_number"`
	LineType   string    `json:"line_type"`
	Content    string    `json:"content"`
	Coords     []float64 `json:"coords"`
}

type rawLinesResponse struct {
	Status   bool      `json:"status"`
	InputID  int64     `json:"input_id"`
	FileName string    `json:"file_name,omitempty"`
	Lines    []rawLine `json:"lines"`
	Pages    int       `json:"pages"`
}

// GetRawLines handles GET /api/v1/kb/raw-lines?input_record_id=N
func GetRawLines(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid input_record_id (CWB_KB_M_210)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve table (CWB_KB_M_220)",
		})
	}
	q := fmt.Sprintf(`SELECT i.result_filename, i.file_name FROM %s i WHERE i.id = $1`, inputTable)
	var resultFile sql.NullString
	var fileName sql.NullString
	if err := db.QueryRow(q, id).Scan(&resultFile, &fileName); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status: false, ErrorMsg: "record not found (CWB_KB_M_230)",
			})
		}
		logger.Error("query result_filename failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve record (CWB_KB_M_231)",
		})
	}
	if !resultFile.Valid || strings.TrimSpace(resultFile.String) == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "result_filename is empty (CWB_KB_M_232)",
		})
	}

	rawPath := rawLinePathFor(resultFile.String)
	f, err := os.Open(rawPath)
	if err != nil {
		logger.Error("open raw_line file failed", "path", rawPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("raw_line file not found: %s (CWB_KB_M_240)", filepath.Base(rawPath)),
		})
	}
	defer f.Close()

	lines := make([]rawLine, 0, 256)
	pages := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		ln, ok := parseRawLine(scanner.Text())
		if !ok {
			continue
		}
		if ln.PageNumber > pages {
			pages = ln.PageNumber
		}
		lines = append(lines, ln)
	}
	if err := scanner.Err(); err != nil {
		logger.Error("scan raw_line file failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to read raw_line file (CWB_KB_M_241)",
		})
	}

	resp := rawLinesResponse{Status: true, InputID: id, Lines: lines, Pages: pages}
	if fileName.Valid {
		resp.FileName = fileName.String
	}
	return c.JSON(http.StatusOK, resp)
}

// GetInputFile handles GET /api/v1/kb/inputs/:id/file
// Streams the original document referenced by kb.inputs.file_name. Falls back
// to backup_filename, then result_filename, if earlier columns are empty.
func GetInputFile(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_300")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid id (CWB_KB_M_310)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve table (CWB_KB_M_320)",
		})
	}

	q := fmt.Sprintf(
		`SELECT i.file_name, i.backup_filename, i.result_filename, i.type FROM %s i WHERE i.id = $1`,
		inputTable,
	)
	var fileName, backupName, resultName, docType sql.NullString
	if err := db.QueryRow(q, id).Scan(&fileName, &backupName, &resultName, &docType); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status: false, ErrorMsg: "record not found (CWB_KB_M_330)",
			})
		}
		logger.Error("query kb input file failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve record (CWB_KB_M_331)",
		})
	}

	pickFirstNonEmpty := func(vals ...sql.NullString) string {
		if len(vals) > 0 && vals[0].Valid && strings.TrimSpace(vals[0].String) != "" {
			return pathutil.ResolveDataHomePath(vals[0].String)
		}
		if len(vals) > 1 && vals[1].Valid && strings.TrimSpace(vals[1].String) != "" {
			return pathutil.ResolveBackupPath(vals[1].String)
		}
		if len(vals) > 2 && vals[2].Valid && strings.TrimSpace(vals[2].String) != "" {
			return pathutil.ResolveDataHomePath(vals[2].String)
		}
		return ""
	}
	path := pickFirstNonEmpty(fileName, backupName, resultName)
	if path == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "no file path on record (CWB_KB_M_340)",
		})
	}

	if _, err := os.Stat(path); err != nil {
		logger.Error("stat input file failed", "path", path, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("file not found: %s (CWB_KB_M_341)", filepath.Base(path)),
		})
	}

	contentType := contentTypeFor(docType.String, path)
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filepath.Base(path)))
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	return c.File(path)
}

func contentTypeFor(docType, path string) string {
	switch strings.ToLower(strings.TrimSpace(docType)) {
	case "pdf":
		return "application/pdf"
	case "json":
		return "application/json; charset=utf-8"
	case "xml":
		return "application/xml; charset=utf-8"
	case "markdown", "md":
		return "text/markdown; charset=utf-8"
	case "text", "txt", "typst":
		return "text/plain; charset=utf-8"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json; charset=utf-8"
	case ".xml":
		return "application/xml; charset=utf-8"
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt", ".typ":
		return "text/plain; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	}
	return "application/octet-stream"
}

func rawLinePathFor(resultFilename string) string {
	resolved := pathutil.ResolveDataHomePath(resultFilename)
	dir := filepath.Dir(resolved)
	base := filepath.Base(resolved)
	ext := filepath.Ext(base)
	root := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, root+".txt")
}

// parseRawLine parses canonical line file records:
// "<line>\t<page>\t<type>\t<font>\t<font_size>\t[x1,y1,x2,y2]\t<content...>"
func parseRawLine(s string) (rawLine, bool) {
	s = strings.TrimRight(s, "\r\n")
	if strings.TrimSpace(s) == "" {
		return rawLine{}, false
	}
	fields := strings.Split(s, "\t")
	if len(fields) != 7 {
		return rawLine{}, false
	}
	lineNum, err1 := strconv.Atoi(strings.TrimSpace(fields[0]))
	pageNum, err2 := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err1 != nil || err2 != nil {
		return rawLine{}, false
	}
	lineType := strings.TrimSpace(fields[2])
	coordRaw := strings.TrimSpace(fields[5])
	content := strings.TrimSpace(fields[6])
	if lineNum < 1 || pageNum < 1 || lineType == "" || coordRaw == "" {
		return rawLine{}, false
	}
	if !strings.HasPrefix(coordRaw, "[") || !strings.HasSuffix(coordRaw, "]") {
		return rawLine{}, false
	}
	coordRaw = strings.TrimSpace(coordRaw[1 : len(coordRaw)-1])

	coords := make([]float64, 0, 4)
	for _, tok := range strings.Split(coordRaw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			continue
		}
		coords = append(coords, v)
	}

	return rawLine{
		LineNumber: lineNum,
		PageNumber: pageNum,
		LineType:   lineType,
		Content:    content,
		Coords:     coords,
	}, true
}
