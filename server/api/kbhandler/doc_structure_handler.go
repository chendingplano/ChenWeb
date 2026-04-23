package kbhandler

import (
	"bufio"
	"database/sql"
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

	correctedPath := filepath.Join(
		artifactDir,
		strconv.FormatInt(inputID/1000, 10),
		strconv.FormatInt(inputID, 10),
		stagingRoot+"_"+parser+".corrected",
	)
	lines, pages, err := readCorrectedLinesFile(correctedPath)
	if err != nil {
		logger.Error("read corrected file failed", "path", correctedPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("corrected file not found: %s (CWB_KB_DS_025)", filepath.Base(correctedPath)),
		})
	}

	resp := docStructureResponse{
		Status:        true,
		InputID:       inputID,
		CorrectedFile: correctedPath,
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
func parseCorrectedLine(s string) (docStructureLine, bool) {
	s = strings.TrimRight(s, "\r\n")
	if strings.TrimSpace(s) == "" {
		return docStructureLine{}, false
	}
	fields := strings.SplitN(s, "\t", 8)
	if len(fields) != 8 {
		return docStructureLine{}, false
	}

	lineNum, err1 := strconv.Atoi(strings.TrimSpace(fields[0]))
	pageNum, err2 := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err1 != nil || err2 != nil || lineNum <= 0 || pageNum <= 0 {
		return docStructureLine{}, false
	}

	lineType := strings.TrimSpace(fields[2])
	correctedLineType := strings.TrimSpace(fields[3])
	font := strings.TrimSpace(fields[4])
	fontSize := strings.TrimSpace(fields[5])
	coordsRaw := strings.TrimSpace(fields[6])
	content := strings.TrimSpace(fields[7])
	if lineType == "" || correctedLineType == "" || coordsRaw == "" {
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
