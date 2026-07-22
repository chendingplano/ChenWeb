// Package videohandler implements the training-video management API backing the
// Resources page (Videos > Training). Video bytes are stored on the filesystem
// under VIDEO_DIR (mirroring the STAGING_DIR pattern in kbhandler); metadata
// lives in kb.videos. All routes are registered under the authenticated
// /api/v1 group.
package videohandler

import (
	"database/sql"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

// defaultMaxVideoBytes caps a single upload. Override with VIDEO_MAX_BYTES.
const defaultMaxVideoBytes int64 = 2 << 30 // 2 GiB

// allowedVideoTypes is the accepted set of client-declared content types.
var allowedVideoTypes = map[string]struct{}{
	"video/mp4":        {},
	"video/webm":       {},
	"video/ogg":        {},
	"video/quicktime":  {},
	"video/x-msvideo":  {},
	"video/x-matroska": {},
	"video/mpeg":       {},
}

// allowedVideoExts is a fallback when the browser omits/garbles the type.
var allowedVideoExts = map[string]struct{}{
	".mp4": {}, ".webm": {}, ".ogg": {}, ".ogv": {},
	".mov": {}, ".avi": {}, ".mkv": {}, ".mpeg": {}, ".mpg": {},
}

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error"`
}

type videoMeta struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentType string    `json:"content_type"`
	UploadedBy  string    `json:"uploaded_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// videoDir resolves the storage directory. VIDEO_DIR is the explicit override;
// when it is unset we fall back to <DATA_HOME_DIR>/Videos so uploads work out of
// the box on any deployment that already sets DATA_HOME_DIR. Only when neither is
// set do we fail closed (CWB_VID_010).
func videoDir() string {
	if v := strings.TrimSpace(os.Getenv("VIDEO_DIR")); v != "" {
		return v
	}
	if home := strings.TrimSpace(os.Getenv("DATA_HOME_DIR")); home != "" {
		return filepath.Join(home, "Videos")
	}
	return ""
}

func maxVideoBytes() int64 {
	if v := strings.TrimSpace(os.Getenv("VIDEO_MAX_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxVideoBytes
}

func currentUserEmail(rc ApiTypes.RequestContext) string {
	if u := rc.IsAuthenticated(); u != nil {
		return strings.TrimSpace(u.Email)
	}
	return ""
}

func isAllowedVideo(header *multipart.FileHeader) bool {
	ct := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if _, ok := allowedVideoTypes[ct]; ok {
		return true
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	_, ok := allowedVideoExts[ext]
	return ok
}

// UploadVideo handles POST /api/v1/videos.
func UploadVideo(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_VID_001")
	defer rc.Close()
	logger := rc.GetLogger()

	dir := videoDir()
	if dir == "" {
		logger.Error("VIDEO_DIR is not configured")
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "VIDEO_DIR is not configured (CWB_VID_010)"})
	}

	header, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "a video file is required (CWB_VID_011)"})
	}
	if !isAllowedVideo(header) {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "unsupported video type (CWB_VID_012)"})
	}
	if header.Size <= 0 || header.Size > maxVideoBytes() {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "video exceeds the maximum allowed size (CWB_VID_013)"})
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Error("create video dir failed", "video_dir", dir, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to prepare video directory (CWB_VID_014)"})
	}

	// Unique on-disk name; original filename is preserved in metadata.
	storedName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFilename(header.Filename))
	destPath := filepath.Join(dir, storedName)

	written, err := saveMultipartFile(header, destPath)
	if err != nil {
		_ = os.Remove(destPath)
		logger.Error("save video failed", "filename", header.Filename, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to save video (CWB_VID_015)"})
	}

	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	uploadedBy := currentUserEmail(rc)

	db := ApiTypes.ProjectDBHandle
	var meta videoMeta
	err = db.QueryRow(
		`INSERT INTO kb.videos (filename, stored_path, size_bytes, content_type, uploaded_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, filename, size_bytes, content_type, COALESCE(uploaded_by, ''), created_at`,
		header.Filename, destPath, written, contentType, nullableString(uploadedBy),
	).Scan(&meta.ID, &meta.Filename, &meta.SizeBytes, &meta.ContentType, &meta.UploadedBy, &meta.CreatedAt)
	if err != nil {
		_ = os.Remove(destPath)
		logger.Error("insert video metadata failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to record video (CWB_VID_016)"})
	}

	logger.Info("video uploaded", "id", meta.ID, "filename", meta.Filename, "size", meta.SizeBytes, "by", uploadedBy)
	return c.JSON(http.StatusOK, meta)
}

// ListVideos handles GET /api/v1/videos.
func ListVideos(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_VID_002")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	rows, err := db.Query(
		`SELECT id, filename, size_bytes, content_type, COALESCE(uploaded_by, ''), created_at
		   FROM kb.videos
		  ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		logger.Error("list videos failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to list videos (CWB_VID_020)"})
	}
	defer rows.Close()

	list := make([]videoMeta, 0)
	for rows.Next() {
		var m videoMeta
		if err := rows.Scan(&m.ID, &m.Filename, &m.SizeBytes, &m.ContentType, &m.UploadedBy, &m.CreatedAt); err != nil {
			logger.Error("scan video row failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to read videos (CWB_VID_021)"})
		}
		list = append(list, m)
	}
	return c.JSON(http.StatusOK, list)
}

// StreamVideo handles GET /api/v1/videos/:id/stream — inline playback with HTTP
// range support so the browser can seek.
func StreamVideo(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_VID_003")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := parseID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "invalid video id (CWB_VID_030)"})
	}

	path, filename, contentType, err := lookupVideo(id)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, errorResponse{false, "video not found (CWB_VID_031)"})
	}
	if err != nil {
		logger.Error("lookup video failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to load video (CWB_VID_032)"})
	}

	f, err := os.Open(path)
	if err != nil {
		logger.Error("open video file failed", "id", id, "path", path, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{false, "video file missing (CWB_VID_033)"})
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		logger.Error("stat video file failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to read video (CWB_VID_034)"})
	}

	c.Response().Header().Set("Content-Type", contentType)
	// http.ServeContent negotiates Range requests for seeking.
	http.ServeContent(c.Response(), c.Request(), filename, info.ModTime(), f)
	return nil
}

// DownloadVideo handles GET /api/v1/videos/:id/download — attachment download.
func DownloadVideo(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_VID_004")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := parseID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "invalid video id (CWB_VID_040)"})
	}

	path, filename, _, err := lookupVideo(id)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, errorResponse{false, "video not found (CWB_VID_041)"})
	}
	if err != nil {
		logger.Error("lookup video failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to load video (CWB_VID_042)"})
	}
	if _, err := os.Stat(path); err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{false, "video file missing (CWB_VID_043)"})
	}
	return c.Attachment(path, filename)
}

// DeleteVideo handles DELETE /api/v1/videos/:id — removes file then metadata.
func DeleteVideo(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_VID_005")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := parseID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "invalid video id (CWB_VID_050)"})
	}

	path, _, _, err := lookupVideo(id)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, errorResponse{false, "video not found (CWB_VID_051)"})
	}
	if err != nil {
		logger.Error("lookup video failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to load video (CWB_VID_052)"})
	}

	// A missing file is not an error — still drop the metadata row.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Error("remove video file failed", "id", id, "path", path, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to delete video file (CWB_VID_053)"})
	}

	db := ApiTypes.ProjectDBHandle
	if _, err := db.Exec(`DELETE FROM kb.videos WHERE id = $1`, id); err != nil {
		logger.Error("delete video metadata failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to delete video (CWB_VID_054)"})
	}

	logger.Info("video deleted", "id", id, "by", currentUserEmail(rc))
	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}

// --- helpers ---

func lookupVideo(id int64) (path, filename, contentType string, err error) {
	db := ApiTypes.ProjectDBHandle
	err = db.QueryRow(
		`SELECT stored_path, filename, content_type FROM kb.videos WHERE id = $1`, id,
	).Scan(&path, &filename, &contentType)
	return path, filename, contentType, err
}

func parseID(c echo.Context) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
}

func saveMultipartFile(header *multipart.FileHeader, destPath string) (int64, error) {
	src, err := header.Open()
	if err != nil {
		return 0, err
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	defer dst.Close()

	return io.Copy(dst, src)
}

// sanitizeFilename strips path separators and keeps a filesystem-safe basename.
func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.' || r == '-' || r == '_':
			return r
		default:
			return '_'
		}
	}, name)
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "video"
	}
	return cleaned
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
