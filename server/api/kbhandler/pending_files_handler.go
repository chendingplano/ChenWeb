package kbhandler

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// pendingFileSuffix marks a file in the staging directory as placed there
// directly (not through the upload API) and not yet claimed by an admin. The
// staging poller (server/cmd/service-pdf-parser) ignores any file with this
// suffix.
const pendingFileSuffix = ".pending"

// pendingTypeExtensions mirrors kb-import-view.svelte's typeExtensions so a
// claimed pending file's `type` is derived the same way a multi-file browser
// upload derives it from each file's extension.
var pendingTypeExtensions = map[string][]string{
	"pdf":      {".pdf"},
	"doc":      {".doc", ".docx"},
	"excel":    {".xls", ".xlsx"},
	"ppt":      {".ppt", ".pptx"},
	"text":     {".txt"},
	"json":     {".json"},
	"xml":      {".xml"},
	"markdown": {".md", ".markdown"},
	"typst":    {".typ"},
	"zip":      {".zip"},
}

func typeFromExtension(name string) string {
	lower := strings.ToLower(name)
	for docType, exts := range pendingTypeExtensions {
		for _, ext := range exts {
			if strings.HasSuffix(lower, ext) {
				return docType
			}
		}
	}
	return ""
}

func requirePendingFilesAdmin(c echo.Context, loc string) (ApiTypes.RequestContext, error) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		return rc, c.JSON(http.StatusUnauthorized, errorResponse{Status: false, ErrorMsg: "authentication required (" + loc + ")"})
	}
	admin := user.IsOwner || user.Admin
	if !admin {
		for _, role := range user.Roles {
			role = strings.ToLower(strings.TrimSpace(role))
			if role == "admin" || role == "root" {
				admin = true
				break
			}
		}
	}
	if !admin {
		return rc, c.JSON(http.StatusForbidden, errorResponse{Status: false, ErrorMsg: "admin access required (" + loc + ")"})
	}
	return rc, nil
}

func pendingFilesStagingDir() (string, error) {
	dir := strings.TrimSpace(os.Getenv("UPLOAD_FILE_STAGING_DIR"))
	if dir == "" {
		return "", fmt.Errorf("UPLOAD_FILE_STAGING_DIR is not configured")
	}
	return dir, nil
}

type pendingFileEntry struct {
	Name        string `json:"name"`
	PendingName string `json:"pending_name"`
	ModTime     string `json:"mod_time"`
}

type listPendingFilesResponse struct {
	Status bool               `json:"status"`
	Files  []pendingFileEntry `json:"files"`
}

// ListPendingFiles handles GET /api/v1/kb/pending-files.
func ListPendingFiles(c echo.Context) error {
	rc, authErr := requirePendingFilesAdmin(c, "CWB_KB_PF_001")
	if authErr != nil {
		rc.Close()
		return authErr
	}
	defer rc.Close()
	logger := rc.GetLogger()

	stagingDir, err := pendingFilesStagingDir()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: err.Error() + " (CWB_KB_PF_002)"})
	}

	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		logger.Error("read staging dir failed", "staging_dir", stagingDir, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read staging directory (CWB_KB_PF_003)"})
	}

	type candidate struct {
		name string
		size int64
		mod  time.Time
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), pendingFileSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{name: entry.Name(), size: info.Size(), mod: info.ModTime()})
	}

	files := make([]pendingFileEntry, 0, len(candidates))
	if len(candidates) > 0 {
		// One fixed delay per request (not per file) to skip files still
		// mid-copy: a candidate whose size changed between the two reads is
		// still being written and is excluded from the list.
		time.Sleep(2 * time.Second)

		for _, cand := range candidates {
			info, err := os.Stat(filepath.Join(stagingDir, cand.name))
			if err != nil || info.Size() != cand.size {
				continue
			}
			files = append(files, pendingFileEntry{
				Name:        strings.TrimSuffix(cand.name, pendingFileSuffix),
				PendingName: cand.name,
				ModTime:     cand.mod.UTC().Format(time.RFC3339),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	return c.JSON(http.StatusOK, listPendingFilesResponse{Status: true, Files: files})
}

type claimPendingFilesRequest struct {
	Filenames         []string `json:"filenames"`
	ProcessingMode    string   `json:"processing_mode"`
	ParserName        string   `json:"parser_name"`
	KSStoreID         int64    `json:"ks_store_id"`
	TenantID          string   `json:"tenant_id"`
	RequestedPipeline string   `json:"requested_pipeline"`
}

type claimResult struct {
	Filename string `json:"filename"`
	Status   bool   `json:"status"`
	ID       int64  `json:"id,omitempty"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

type claimPendingFilesResponse struct {
	Status  bool          `json:"status"`
	Results []claimResult `json:"results"`
}

// ClaimPendingFiles handles POST /api/v1/kb/pending-files/claim.
func ClaimPendingFiles(c echo.Context) error {
	rc, authErr := requirePendingFilesAdmin(c, "CWB_KB_PF_010")
	if authErr != nil {
		rc.Close()
		return authErr
	}
	defer rc.Close()
	logger := rc.GetLogger()

	var req claimPendingFilesRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PF_011)"})
	}

	if len(req.Filenames) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "at least one filename is required (CWB_KB_PF_012)"})
	}
	if req.KSStoreID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "active knowledge store is required (CWB_KB_PF_013)"})
	}
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" || tenantID == "-" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "tenant_id is required (CWB_KB_PF_014)"})
	}
	processingMode := strings.ToLower(strings.TrimSpace(req.ProcessingMode))
	if processingMode == "" {
		processingMode = "auto"
	}
	if _, ok := allowedProcessingModes[processingMode]; !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid processing_mode (CWB_KB_PF_015)"})
	}
	parserName := strings.ToLower(strings.TrimSpace(req.ParserName))
	if _, ok := allowedUploadParsers[parserName]; !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid parser_name (CWB_KB_PF_016)"})
	}

	stagingDir, err := pendingFilesStagingDir()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: err.Error() + " (CWB_KB_PF_017)"})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve kb input table (CWB_KB_PF_018)"})
	}

	var ksDesc *string
	var activeStoreDesc sql.NullString
	if err := db.QueryRow(`SELECT ks_desc FROM kb.knowledge_store WHERE id = $1`, req.KSStoreID).
		Scan(&activeStoreDesc); err == nil && activeStoreDesc.Valid {
		ksDesc = normalizeOptionalString(activeStoreDesc.String)
	}
	requestedPipeline := normalizeOptionalString(req.RequestedPipeline)

	results := make([]claimResult, 0, len(req.Filenames))
	for _, rawName := range req.Filenames {
		name := filepath.Base(strings.TrimSpace(rawName))
		if name == "" || name == "." || !strings.HasSuffix(name, pendingFileSuffix) {
			results = append(results, claimResult{Filename: rawName, Status: false, ErrorMsg: "invalid pending filename"})
			continue
		}

		finalName := strings.TrimSuffix(name, pendingFileSuffix)
		docType := typeFromExtension(finalName)
		if docType == "" {
			results = append(results, claimResult{Filename: name, Status: false, ErrorMsg: "unrecognized file extension"})
			continue
		}

		pendingPath := filepath.Join(stagingDir, name)
		finalPath := filepath.Join(stagingDir, finalName)

		md5Hex, err := fileMD5Hex(pendingPath)
		if err != nil {
			results = append(results, claimResult{Filename: name, Status: false, ErrorMsg: "file no longer available"})
			continue
		}

		id, err := claimOnePendingFile(db, inputTable, uploadedInputInsert{
			TenantID:          tenantID,
			KSStoreID:         req.KSStoreID,
			Type:              docType,
			KSDesc:            ksDesc,
			RequestedPipeline: requestedPipeline,
			ProcessingMode:    processingMode,
			ParserName:        parserName,
			StagingName:       finalName,
			StagingAbsPath:    finalPath,
			MD5:               &md5Hex,
		}, pendingPath, finalPath)
		if err != nil {
			logger.Warn("claim pending file failed", "filename", name, "err", err)
			results = append(results, claimResult{Filename: name, Status: false, ErrorMsg: err.Error()})
			continue
		}

		results = append(results, claimResult{Filename: name, Status: true, ID: id})
	}

	return c.JSON(http.StatusOK, claimPendingFilesResponse{Status: true, Results: results})
}

// claimOnePendingFile inserts the kb.inputs row first (matching what
// UploadInputs would insert for the same target file), then renames the
// pending file to its final name. Renaming after the insert closes the race
// where service-pdf-parser's poller could see the renamed file before a
// matching row exists. If the rename fails (already claimed, or gone), the
// transaction is rolled back so no orphan row is left behind.
func claimOnePendingFile(db *sql.DB, inputTable string, req uploadedInputInsert, pendingPath, finalPath string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction failed: %w", err)
	}

	id, err := insertUploadedInputRecord(tx, inputTable, req)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("insert kb input record failed: %w", err)
	}

	if err := os.Rename(pendingPath, finalPath); err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("file no longer available")
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction failed: %w", err)
	}

	return id, nil
}

func fileMD5Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
