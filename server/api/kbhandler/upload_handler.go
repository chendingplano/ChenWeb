package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

var allowedUploadParsers = map[string]struct{}{
	"paddleocr": {},
	"opendata":  {},
	"mineru":    {},
	"docling":   {},
}

var allowedUploadTypes = map[string]struct{}{
	"pdf":      {},
	"doc":      {},
	"excel":    {},
	"ppt":      {},
	"text":     {},
	"json":     {},
	"xml":      {},
	"markdown": {},
	"typst":    {},
}

type uploadInputsResponse struct {
	Status bool    `json:"status"`
	Count  int     `json:"count"`
	IDs    []int64 `json:"ids"`
}

// UploadInputs handles POST /api/v1/kb/inputs/upload.
func UploadInputs(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_U_001")
	defer rc.Close()
	logger := rc.GetLogger()

	stagingDir := strings.TrimSpace(os.Getenv("STAGING_DIR"))
	if stagingDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "STAGING_DIR is not configured (CWB_KB_U_010)",
		})
	}

	docType := strings.ToLower(strings.TrimSpace(c.FormValue("type")))
	if _, ok := allowedUploadTypes[docType]; !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid type (CWB_KB_U_011)",
		})
	}

	parserName := strings.ToLower(strings.TrimSpace(c.FormValue("parser_name")))
	if _, ok := allowedUploadParsers[parserName]; !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid parser_name (CWB_KB_U_012)",
		})
	}

	ksStoreID, err := strconv.ParseInt(strings.TrimSpace(c.FormValue("ks_store_id")), 10, 64)
	if err != nil || ksStoreID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "active knowledge store is required (CWB_KB_U_013)",
		})
	}

	tenantID := strings.TrimSpace(c.FormValue("tenant_id"))
	if tenantID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "tenant_id is required (CWB_KB_U_014)",
		})
	}

	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid multipart form (CWB_KB_U_015)",
		})
	}

	files := form.File["files"]
	if len(files) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "at least one file is required (CWB_KB_U_016)",
		})
	}

	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		logger.Error("create staging dir failed", "staging_dir", stagingDir, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to prepare staging directory (CWB_KB_U_017)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input table (CWB_KB_U_018)",
		})
	}

	title := normalizeOptionalString(c.FormValue("title"))
	docNo := normalizeOptionalString(c.FormValue("doc_no"))
	authors := normalizeOptionalString(c.FormValue("authors"))
	publicInfo, err := jsonStringOrNil(c.FormValue("public_info"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid public_info (CWB_KB_U_019)",
		})
	}
	privateInfo, err := jsonStringOrNil(c.FormValue("private_info"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid private_info (CWB_KB_U_020)",
		})
	}
	notes := normalizeOptionalString(c.FormValue("notes"))
	ksDesc := normalizeOptionalString(c.FormValue("ks_desc"))

	tx, err := db.Begin()
	if err != nil {
		logger.Error("begin upload transaction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to begin upload transaction (CWB_KB_U_021)",
		})
	}

	savedPaths := make([]string, 0, len(files))
	recordIDs := make([]int64, 0, len(files))
	rollback := func() {
		_ = tx.Rollback()
		for _, path := range savedPaths {
			_ = os.Remove(path)
		}
	}

	for _, fileHeader := range files {
		destPath, stageName, err := stageUploadFile(stagingDir, fileHeader)
		if err != nil {
			rollback()
			logger.Error("stage upload file failed", "filename", fileHeader.Filename, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to save uploaded file (CWB_KB_U_022)",
			})
		}
		savedPaths = append(savedPaths, destPath)

		recordID, err := insertUploadedInputRecord(
			tx,
			inputTable,
			uploadedInputInsert{
				TenantID:       tenantID,
				KSStoreID:      ksStoreID,
				Type:           docType,
				Title:          title,
				DocNo:          docNo,
				Authors:        authors,
				PublicInfo:     publicInfo,
				PrivateInfo:    privateInfo,
				Notes:          notes,
				KSDesc:         ksDesc,
				ParserName:     parserName,
				StagingName:    stageName,
				StagingAbsPath: destPath,
			},
		)
		if err != nil {
			rollback()
			logger.Error("insert uploaded kb input failed", "filename", fileHeader.Filename, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to create kb input record (CWB_KB_U_023)",
			})
		}
		recordIDs = append(recordIDs, recordID)
	}

	if err := tx.Commit(); err != nil {
		rollback()
		logger.Error("commit upload transaction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to finalize upload (CWB_KB_U_024)",
		})
	}

	return c.JSON(http.StatusOK, uploadInputsResponse{
		Status: true,
		Count:  len(recordIDs),
		IDs:    recordIDs,
	})
}

type uploadedInputInsert struct {
	TenantID       string
	KSStoreID      int64
	Type           string
	Title          *string
	DocNo          *string
	Authors        *string
	PublicInfo     *string
	PrivateInfo    *string
	Notes          *string
	KSDesc         *string
	ParserName     string
	StagingName    string
	StagingAbsPath string
}

func insertUploadedInputRecord(tx *sql.Tx, inputTable string, req uploadedInputInsert) (int64, error) {
	query := fmt.Sprintf(`
INSERT INTO %s (
    tenant_id,
    ks_store_id,
    type,
    title,
    doc_no,
    authors,
    public_info,
    private_info,
    notes,
    ks_desc,
    parser_name,
    staging_filename,
    file_name,
    status
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7::jsonb,
    $8::jsonb,
    $9,
    $10,
    $11,
    $12,
    $13,
    '[]'::jsonb
)
RETURNING id`, inputTable)

	var id int64
	if err := tx.QueryRow(
		query,
		req.TenantID,
		req.KSStoreID,
		req.Type,
		req.Title,
		req.DocNo,
		req.Authors,
		req.PublicInfo,
		req.PrivateInfo,
		req.Notes,
		req.KSDesc,
		req.ParserName,
		req.StagingName,
		req.StagingAbsPath,
	).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func stageUploadFile(stagingDir string, fileHeader *multipart.FileHeader) (destPath string, savedName string, err error) {
	baseName := filepath.Base(strings.TrimSpace(fileHeader.Filename))
	if baseName == "" || baseName == "." {
		return "", "", fmt.Errorf("invalid filename")
	}

	destPath, err = uniqueStagePath(stagingDir, baseName)
	if err != nil {
		return "", "", err
	}

	src, err := fileHeader.Open()
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return "", "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", "", err
	}

	return destPath, filepath.Base(destPath), nil
}

func uniqueStagePath(dir, fileName string) (string, error) {
	base := filepath.Base(strings.TrimSpace(fileName))
	if base == "" || base == "." {
		return "", fmt.Errorf("invalid filename")
	}

	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "file"
	}

	candidate := filepath.Join(dir, base)
	if _, err := os.Stat(candidate); err != nil {
		if os.IsNotExist(err) {
			return candidate, nil
		}
		return "", err
	}

	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s_%d%s", stem, i, ext))
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
}

func normalizeOptionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func jsonStringOrNil(value string) (*string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	encoded, err := json.Marshal(trimmed)
	if err != nil {
		return nil, err
	}
	s := string(encoded)
	return &s, nil
}
