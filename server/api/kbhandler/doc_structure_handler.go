package kbhandler

import (
	"bufio"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type docStructureLine struct {
	LineNumber        int       `json:"line_number"`
	PageNumber        int       `json:"page_number"`
	LineType          string    `json:"line_type"`
	CorrectedLineType string    `json:"corrected_line_type"`
	Font              string    `json:"font"`
	FontSize          string    `json:"font_size"`
	Coords            []float64 `json:"coords"`
	Content           string    `json:"content"`
}

type docStructureResponse struct {
	Status        bool               `json:"status"`
	InputID       int64              `json:"input_id"`
	FileName      string             `json:"file_name,omitempty"`
	CorrectedFile string             `json:"corrected_file,omitempty"`
	Lines         []docStructureLine `json:"lines"`
	Pages         int                `json:"pages"`
	Total         int                `json:"total"`
}

// GetDocStructure handles GET /api/v1/kb/doc-structure?input_record_id=N
func GetDocStructure(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DS_001")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	inputID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || inputID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid input_record_id (CWB_KB_DS_010)",
		})
	}

	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "missing ARTIFACT_DIR (CWB_KB_DS_011)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve table (CWB_KB_DS_012)",
		})
	}

	stagingExpr, err := resolveStagingOrNameExpr(db, inputTable)
	if err != nil {
		logger.Error("resolve staging/name column failed", "table", inputTable, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve input filename column (CWB_KB_DS_013)",
		})
	}
	parserExpr, err := resolveParserNameExpr(db, inputTable)
	if err != nil {
		logger.Error("resolve parser_name column failed", "table", inputTable, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve parser column (CWB_KB_DS_014)",
		})
	}

	query := fmt.Sprintf(`SELECT %s AS staging_filename, %s AS parser_name, i.file_name FROM %s i WHERE i.id = $1`, stagingExpr, parserExpr, inputTable)
	var stagingFilename sql.NullString
	var parserName sql.NullString
	var fileName sql.NullString
	if err := db.QueryRow(query, inputID).Scan(&stagingFilename, &parserName, &fileName); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status: false, ErrorMsg: "record not found (CWB_KB_DS_020)",
			})
		}
		logger.Error("query kb input failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve record (CWB_KB_DS_021)",
		})
	}

	staging := strings.TrimSpace(stagingFilename.String)
	if staging == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "staging filename is empty (CWB_KB_DS_022)",
		})
	}
	parser := strings.TrimSpace(parserName.String)
	if parser == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "parser name is empty (CWB_KB_DS_023)",
		})
	}

	stagingBase := filepath.Base(staging)
	stagingRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if strings.TrimSpace(stagingRoot) == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "invalid staging filename (CWB_KB_DS_024)",
		})
	}

	txtPath := filepath.Join(
		artifactDir,
		strconv.FormatInt(inputID/1000, 10),
		strconv.FormatInt(inputID, 10),
		stagingRoot+"_"+parser+".txt",
	)

	lines, pages, err := readCorrectedLinesFile(txtPath)
	if err != nil {
		logger.Error("read txt structure file failed", "path", txtPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("structure file not found: %s (CWB_KB_DS_025)", filepath.Base(txtPath)),
		})
	}

	resp := docStructureResponse{
		Status:        true,
		InputID:       inputID,
		CorrectedFile: txtPath,
		Lines:         lines,
		Pages:         pages,
		Total:         len(lines),
	}
	if fileName.Valid {
		resp.FileName = fileName.String
	}
	return c.JSON(http.StatusOK, resp)
}

func resolveStagingOrNameExpr(db *sql.DB, inputTable string) (string, error) {
	schema, table, err := splitQualifiedTable(inputTable)
	if err != nil {
		return "", err
	}
	hasStaging, err := columnExists(db, schema, table, "staging_filename")
	if err != nil {
		return "", err
	}
	if hasStaging {
		return "COALESCE(i.staging_filename, '')", nil
	}
	hasName, err := columnExists(db, schema, table, "name")
	if err != nil {
		return "", err
	}
	if hasName {
		return "COALESCE(i.name, '')", nil
	}
	return "", fmt.Errorf("neither staging_filename nor name exists on %s", inputTable)
}

func resolveParserNameExpr(db *sql.DB, inputTable string) (string, error) {
	schema, table, err := splitQualifiedTable(inputTable)
	if err != nil {
		return "", err
	}
	hasParser, err := columnExists(db, schema, table, "parser_name")
	if err != nil {
		return "", err
	}
	if hasParser {
		return "COALESCE(i.parser_name, '')", nil
	}
	return "''", nil
}

func readCorrectedLinesFile(path string) ([]docStructureLine, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	lines := make([]docStructureLine, 0, 1024)
	maxPage := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		ln, ok := parseCorrectedLine(scanner.Text())
		if !ok {
			continue
		}
		if ln.PageNumber > maxPage {
			maxPage = ln.PageNumber
		}
		lines = append(lines, ln)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	return lines, maxPage, nil
}

// parseCorrectedLine parses corrected line records:
// "<line>\t<page>\t<line_type>\t<corrected_line_type>\t<font>\t<font_size>\t[x1,y1,x2,y2]\t<content...>"
type updateDocStructureLineRequest struct {
	InputRecordID     int64   `json:"input_record_id"`
	PageNumber        int     `json:"page_number"`
	LineNumber        int     `json:"line_number"`
	CorrectedLineType *string `json:"corrected_line_type"`
	Content           *string `json:"content"`
}

// UpdateDocStructureLine handles PATCH /api/v1/kb/doc-structure
func UpdateDocStructureLine(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DSU_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req updateDocStructureLineRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid request body (CWB_KB_DSU_010)",
		})
	}
	if req.InputRecordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid input_record_id (CWB_KB_DSU_011)",
		})
	}
	if req.PageNumber <= 0 || req.LineNumber <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid page_number or line_number (CWB_KB_DSU_012)",
		})
	}
	if req.CorrectedLineType == nil && req.Content == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "nothing to update (CWB_KB_DSU_013)",
		})
	}
	if req.CorrectedLineType != nil && strings.TrimSpace(*req.CorrectedLineType) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "corrected_line_type cannot be empty (CWB_KB_DSU_014)",
		})
	}

	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "missing ARTIFACT_DIR (CWB_KB_DSU_020)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve table (CWB_KB_DSU_021)",
		})
	}

	stagingExpr, err := resolveStagingOrNameExpr(db, inputTable)
	if err != nil {
		logger.Error("resolve staging/name column failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve filename column (CWB_KB_DSU_022)",
		})
	}
	parserExpr, err := resolveParserNameExpr(db, inputTable)
	if err != nil {
		logger.Error("resolve parser column failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve parser column (CWB_KB_DSU_023)",
		})
	}

	query := fmt.Sprintf(`SELECT %s AS staging_filename, %s AS parser_name, i.file_name FROM %s i WHERE i.id = $1`, stagingExpr, parserExpr, inputTable)
	var stagingFilename sql.NullString
	var parserName sql.NullString
	var fileName sql.NullString
	if err := db.QueryRow(query, req.InputRecordID).Scan(&stagingFilename, &parserName, &fileName); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status: false, ErrorMsg: "record not found (CWB_KB_DSU_030)",
			})
		}
		logger.Error("query kb input failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve record (CWB_KB_DSU_031)",
		})
	}

	staging := strings.TrimSpace(stagingFilename.String)
	if staging == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "staging filename is empty (CWB_KB_DSU_032)",
		})
	}
	parser := strings.TrimSpace(parserName.String)
	if parser == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "parser name is empty (CWB_KB_DSU_033)",
		})
	}

	stagingBase := filepath.Base(staging)
	stagingRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if strings.TrimSpace(stagingRoot) == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "invalid staging filename (CWB_KB_DSU_034)",
		})
	}

	artifactBase := filepath.Join(
		artifactDir,
		strconv.FormatInt(req.InputRecordID/1000, 10),
		strconv.FormatInt(req.InputRecordID, 10),
	)
	txtPath := filepath.Join(artifactBase, stagingRoot+"_"+parser+".txt")
	manualPath := filepath.Join(artifactBase, stagingRoot+"_"+parser+".manual")

	lines, pages, err := readCorrectedLinesFile(txtPath)
	if err != nil {
		logger.Error("read txt structure file failed", "path", txtPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("structure file not found: %s (CWB_KB_DSU_040)", filepath.Base(txtPath)),
		})
	}

	found := false
	var updatedLine docStructureLine
	for i := range lines {
		if lines[i].PageNumber == req.PageNumber && lines[i].LineNumber == req.LineNumber {
			if req.CorrectedLineType != nil {
				lines[i].LineType = strings.TrimSpace(*req.CorrectedLineType)
			}
			if req.Content != nil {
				lines[i].Content = strings.ReplaceAll(*req.Content, "\t", " ")
			}
			updatedLine = lines[i]
			found = true
			break
		}
	}
	if !found {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "line not found (CWB_KB_DSU_041)",
		})
	}

	if err := writeTxtLinesFile(txtPath, lines); err != nil {
		logger.Error("write txt file failed", "path", txtPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to write txt file (CWB_KB_DSU_042)",
		})
	}

	if err := upsertManualFile(manualPath, updatedLine); err != nil {
		logger.Error("upsert manual file failed", "path", manualPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to write manual file (CWB_KB_DSU_043)",
		})
	}

	logger.Info("updated doc structure line", "input_id", req.InputRecordID, "page", req.PageNumber, "line", req.LineNumber)
	resp := docStructureResponse{
		Status:        true,
		InputID:       req.InputRecordID,
		CorrectedFile: txtPath,
		Lines:         lines,
		Pages:         pages,
		Total:         len(lines),
	}
	if fileName.Valid {
		resp.FileName = fileName.String
	}
	return c.JSON(http.StatusOK, resp)
}

type deleteDocStructureLineRequest struct {
	InputRecordID int64 `json:"input_record_id" query:"input_record_id"`
	PageNumber    int   `json:"page_number"    query:"page_number"`
	LineNumber    int   `json:"line_number"    query:"line_number"`
}

// DeleteDocStructureLine handles DELETE /api/v1/kb/doc-structure
func DeleteDocStructureLine(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DSD_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req deleteDocStructureLineRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid request body (CWB_KB_DSD_010)",
		})
	}
	if req.InputRecordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid input_record_id (CWB_KB_DSD_011)",
		})
	}
	if req.PageNumber <= 0 || req.LineNumber <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid page_number or line_number (CWB_KB_DSD_012)",
		})
	}

	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "missing ARTIFACT_DIR (CWB_KB_DSD_020)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve table (CWB_KB_DSD_021)",
		})
	}

	stagingExpr, err := resolveStagingOrNameExpr(db, inputTable)
	if err != nil {
		logger.Error("resolve staging/name column failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve filename column (CWB_KB_DSD_022)",
		})
	}
	parserExpr, err := resolveParserNameExpr(db, inputTable)
	if err != nil {
		logger.Error("resolve parser column failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve parser column (CWB_KB_DSD_023)",
		})
	}

	dbQuery := fmt.Sprintf(`SELECT %s AS staging_filename, %s AS parser_name, i.file_name FROM %s i WHERE i.id = $1`, stagingExpr, parserExpr, inputTable)
	var stagingFilename, parserName, fileName sql.NullString
	if err := db.QueryRow(dbQuery, req.InputRecordID).Scan(&stagingFilename, &parserName, &fileName); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status: false, ErrorMsg: "record not found (CWB_KB_DSD_030)",
			})
		}
		logger.Error("query kb input failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve record (CWB_KB_DSD_031)",
		})
	}

	staging := strings.TrimSpace(stagingFilename.String)
	if staging == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "staging filename is empty (CWB_KB_DSD_032)",
		})
	}
	parser := strings.TrimSpace(parserName.String)
	if parser == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "parser name is empty (CWB_KB_DSD_033)",
		})
	}

	stagingBase := filepath.Base(staging)
	stagingRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if strings.TrimSpace(stagingRoot) == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "invalid staging filename (CWB_KB_DSD_034)",
		})
	}

	artifactBase := filepath.Join(
		artifactDir,
		strconv.FormatInt(req.InputRecordID/1000, 10),
		strconv.FormatInt(req.InputRecordID, 10),
	)
	txtPath := filepath.Join(artifactBase, stagingRoot+"_"+parser+".txt")
	manualPath := filepath.Join(artifactBase, stagingRoot+"_"+parser+".manual")

	lines, pages, err := readCorrectedLinesFile(txtPath)
	if err != nil {
		logger.Error("read txt structure file failed", "path", txtPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("structure file not found: %s (CWB_KB_DSD_040)", filepath.Base(txtPath)),
		})
	}

	var deletedLine docStructureLine
	found := false
	remaining := make([]docStructureLine, 0, len(lines))
	for _, ln := range lines {
		if ln.PageNumber == req.PageNumber && ln.LineNumber == req.LineNumber {
			deletedLine = ln
			found = true
		} else {
			remaining = append(remaining, ln)
		}
	}
	if !found {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "line not found (CWB_KB_DSD_041)",
		})
	}

	if err := writeTxtLinesFile(txtPath, remaining); err != nil {
		logger.Error("write txt file failed", "path", txtPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to write txt file (CWB_KB_DSD_042)",
		})
	}

	deletedLine.LineType = "deleted"
	if err := upsertManualFile(manualPath, deletedLine); err != nil {
		logger.Error("upsert manual file failed", "path", manualPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to write manual file (CWB_KB_DSD_043)",
		})
	}

	logger.Info("deleted doc structure line", "input_id", req.InputRecordID, "page", req.PageNumber, "line", req.LineNumber)
	resp := docStructureResponse{
		Status:        true,
		InputID:       req.InputRecordID,
		CorrectedFile: txtPath,
		Lines:         remaining,
		Pages:         pages,
		Total:         len(remaining),
	}
	if fileName.Valid {
		resp.FileName = fileName.String
	}
	return c.JSON(http.StatusOK, resp)
}

func writeTxtLinesFile(path string, lines []docStructureLine) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, ln := range lines {
		if _, err := fmt.Fprintln(w, formatTxtLine(ln)); err != nil {
			return err
		}
	}
	return w.Flush()
}

func formatTxtLine(ln docStructureLine) string {
	parts := make([]string, len(ln.Coords))
	for i, c := range ln.Coords {
		parts[i] = strconv.FormatFloat(c, 'f', -1, 64)
	}
	coordsStr := "[" + strings.Join(parts, ",") + "]"
	return strings.Join([]string{
		strconv.Itoa(ln.LineNumber),
		strconv.Itoa(ln.PageNumber),
		ln.LineType,
		ln.Font,
		ln.FontSize,
		coordsStr,
		ln.Content,
	}, "\t")
}

func upsertManualFile(path string, line docStructureLine) error {
	lines := make([]docStructureLine, 0)
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
		for scanner.Scan() {
			ln, ok := parseCorrectedLine(scanner.Text())
			if !ok {
				continue
			}
			if ln.PageNumber != line.PageNumber || ln.LineNumber != line.LineNumber {
				lines = append(lines, ln)
			}
		}
		f.Close()
	}
	lines = append(lines, line)
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].LineNumber != lines[j].LineNumber {
			return lines[i].LineNumber < lines[j].LineNumber
		}
		return lines[i].PageNumber < lines[j].PageNumber
	})
	return writeTxtLinesFile(path, lines)
}

func writeCorrectedLinesFile(path string, lines []docStructureLine) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, ln := range lines {
		if _, err := fmt.Fprintln(w, formatCorrectedLine(ln)); err != nil {
			return err
		}
	}
	return w.Flush()
}

func formatCorrectedLine(ln docStructureLine) string {
	parts := make([]string, len(ln.Coords))
	for i, c := range ln.Coords {
		parts[i] = strconv.FormatFloat(c, 'f', -1, 64)
	}
	coordsStr := "[" + strings.Join(parts, ",") + "]"
	return strings.Join([]string{
		strconv.Itoa(ln.LineNumber),
		strconv.Itoa(ln.PageNumber),
		ln.LineType,
		ln.CorrectedLineType,
		ln.Font,
		ln.FontSize,
		coordsStr,
		ln.Content,
	}, "\t")
}

func parseCorrectedLine(s string) (docStructureLine, bool) {
	s = strings.TrimRight(s, "\r\n")
	if strings.TrimSpace(s) == "" {
		return docStructureLine{}, false
	}
	// Accept 8-field .corrected format: line\tpage\tline_type\tcorrected_line_type\tfont\tfont_size\t[coords]\tcontent
	// Accept 7-field .txt format:       line\tpage\tline_type\tfont\tfont_size\t[coords]\tcontent
	fields := strings.SplitN(s, "\t", 8)
	if len(fields) != 8 && len(fields) != 7 {
		return docStructureLine{}, false
	}

	lineNum, err1 := strconv.Atoi(strings.TrimSpace(fields[0]))
	pageNum, err2 := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err1 != nil || err2 != nil || lineNum <= 0 || pageNum <= 0 {
		return docStructureLine{}, false
	}

	var lineType, correctedLineType, font, fontSize, coordsRaw, content string
	if len(fields) == 8 {
		lineType = strings.TrimSpace(fields[2])
		correctedLineType = strings.TrimSpace(fields[3])
		font = strings.TrimSpace(fields[4])
		fontSize = strings.TrimSpace(fields[5])
		coordsRaw = strings.TrimSpace(fields[6])
		content = strings.TrimSpace(fields[7])
	} else {
		lineType = strings.TrimSpace(fields[2])
		correctedLineType = ""
		font = strings.TrimSpace(fields[3])
		fontSize = strings.TrimSpace(fields[4])
		coordsRaw = strings.TrimSpace(fields[5])
		content = strings.TrimSpace(fields[6])
	}
	if lineType == "" || coordsRaw == "" {
		return docStructureLine{}, false
	}
	if !strings.HasPrefix(coordsRaw, "[") || !strings.HasSuffix(coordsRaw, "]") {
		return docStructureLine{}, false
	}

	coordsRaw = strings.TrimSpace(coordsRaw[1 : len(coordsRaw)-1])
	coords := make([]float64, 0, 4)
	for _, tok := range strings.Split(coordsRaw, ",") {
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

	return docStructureLine{
		LineNumber:        lineNum,
		PageNumber:        pageNum,
		LineType:          lineType,
		CorrectedLineType: correctedLineType,
		Font:              font,
		FontSize:          fontSize,
		Coords:            coords,
		Content:           content,
	}, true
}
