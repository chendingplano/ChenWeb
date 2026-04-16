package kbhandler

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type metricRecord struct {
	ID                  int64           `json:"id"`
	InputRecordID       int64           `json:"input_record_id"`
	ExtractID           string          `json:"extract_id"`
	InputFilename       string          `json:"input_filename"`
	MetricName          *string         `json:"metric_name,omitempty"`
	SourceLineSpans     json.RawMessage `json:"source_line_spans,omitempty"`
	MetricSubject       *string         `json:"metric_subject,omitempty"`
	MetricDesc          *string         `json:"metric_desc,omitempty"`
	MetricContext       *string         `json:"metric_context,omitempty"`
	MetricKeywords      json.RawMessage `json:"metric_keywords,omitempty"`
	LocationType        *string         `json:"location_type,omitempty"`
	MetricUnit          *string         `json:"metric_unit,omitempty"`
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
    id, input_record_id, extract_id, input_filename,
    metric_name, source_line_spans, metric_subject, metric_desc,
    metric_context, metric_keywords, location_type, metric_unit,
    formula_or_definition, threshold_or_target, measurement_frequency,
    confidence, is_explicit_metric, reasoning_tags,
    COALESCE(to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at
FROM kb.metrics
WHERE input_record_id = $1
ORDER BY id ASC
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
			&r.ID, &r.InputRecordID, &r.ExtractID, &r.InputFilename,
			&r.MetricName, &spansBytes, &r.MetricSubject, &r.MetricDesc,
			&r.MetricContext, &keywordsBytes, &r.LocationType, &r.MetricUnit,
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
	nameColumnExpr, err := resolveNameColumnExpr(db, inputTable)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input schema (CWB_KB_M_121)",
		})
	}

	query := fmt.Sprintf(`
SELECT
    i.id, %s AS name, i.type, i.title, i.doc_no, i.source,
    i.file_name, i.backup_filename, i.result_filename, i.publish_date,
    i.authors, i.owner, COALESCE(i.status, '[]'::jsonb) AS status,
    i.create_time, i.modify_time, i.public_info, i.private_info,
    i.notes, i.error_msg
FROM %s i
WHERE i.id = $1
`, nameColumnExpr, inputTable)

	row := db.QueryRow(query, id)
	var (
		record              inputRecord
		statusBytes         []byte
		publishDate         sql.NullTime
		publicInfoNullable  sql.NullString
		privateInfoNullable sql.NullString
	)
	if err := row.Scan(
		&record.ID, &record.Name, &record.Type, &record.Title, &record.DocNo, &record.Source,
		&record.FileName, &record.BackupFileName, &record.ResultFileName, &publishDate,
		&record.Authors, &record.Owner, &statusBytes,
		&record.CreateTime, &record.ModifyTime, &publicInfoNullable, &privateInfoNullable,
		&record.Notes, &record.ErrorMsg,
	); err != nil {
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
		for _, v := range vals {
			if v.Valid && strings.TrimSpace(v.String) != "" {
				return v.String
			}
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
	dir := filepath.Dir(resultFilename)
	base := filepath.Base(resultFilename)
	ext := filepath.Ext(base)
	root := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, root+".txt")
}

// parseRawLine parses: "<line> <page> <type> <content...> [x1,y1,x2,y2]"
func parseRawLine(s string) (rawLine, bool) {
	s = strings.TrimRight(s, "\r\n")
	if strings.TrimSpace(s) == "" {
		return rawLine{}, false
	}
	open := strings.LastIndex(s, "[")
	closeIdx := strings.LastIndex(s, "]")
	if open < 0 || closeIdx < 0 || closeIdx < open {
		return rawLine{}, false
	}
	coordRaw := s[open+1 : closeIdx]
	leading := strings.TrimSpace(s[:open])

	parts := strings.SplitN(leading, " ", 4)
	if len(parts) < 4 {
		return rawLine{}, false
	}
	lineNum, err1 := strconv.Atoi(parts[0])
	pageNum, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return rawLine{}, false
	}
	lineType := parts[2]
	content := parts[3]

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
