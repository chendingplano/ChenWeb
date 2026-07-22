// Package imagehandler implements the minimal image library backing video cover
// selection (Pick an Image) and AI cover generation (Auto-Generate). Image bytes
// are stored on the filesystem under IMAGE_DIR (env override + DATA_HOME_DIR/Images
// fallback, mirroring videohandler); metadata lives in kb.images. All routes are
// registered under the authenticated /api/v1 group.
package imagehandler

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

const defaultMaxImageBytes int64 = 25 << 20 // 25 MiB

var allowedImageTypes = map[string]struct{}{
	"image/png":     {},
	"image/jpeg":    {},
	"image/webp":    {},
	"image/gif":     {},
	"image/svg+xml": {},
}

var allowedImageExts = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".webp": {}, ".gif": {}, ".svg": {},
}

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error"`
}

type imageMeta struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentType string    `json:"content_type"`
	Origin      string    `json:"origin"`
	ContentURL  string    `json:"content_url"`
	CreatedAt   time.Time `json:"created_at"`
}

// imageDir resolves the storage directory: IMAGE_DIR override, else
// <DATA_HOME_DIR>/Images, else "" (fail closed).
func imageDir() string {
	if v := strings.TrimSpace(os.Getenv("IMAGE_DIR")); v != "" {
		return v
	}
	if home := strings.TrimSpace(os.Getenv("DATA_HOME_DIR")); home != "" {
		return filepath.Join(home, "Images")
	}
	return ""
}

func contentURL(id int64) string {
	return fmt.Sprintf("/api/v1/images/%d/content", id)
}

func currentUserEmail(rc ApiTypes.RequestContext) string {
	if u := rc.IsAuthenticated(); u != nil {
		return strings.TrimSpace(u.Email)
	}
	return ""
}

func isAllowedImage(header *multipart.FileHeader) bool {
	ct := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if _, ok := allowedImageTypes[ct]; ok {
		return true
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	_, ok := allowedImageExts[ext]
	return ok
}

// UploadImage handles POST /api/v1/images.
func UploadImage(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_IMG_001")
	defer rc.Close()
	logger := rc.GetLogger()

	dir := imageDir()
	if dir == "" {
		logger.Error("IMAGE_DIR is not configured")
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "IMAGE_DIR is not configured (CWB_IMG_010)"})
	}

	header, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "an image file is required (CWB_IMG_011)"})
	}
	if !isAllowedImage(header) {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "unsupported image type (CWB_IMG_012)"})
	}
	if header.Size <= 0 || header.Size > defaultMaxImageBytes {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "image exceeds the maximum allowed size (CWB_IMG_013)"})
	}

	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	src, err := header.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to read image (CWB_IMG_014)"})
	}
	defer src.Close()

	meta, err := storeImage(dir, header.Filename, contentType, "upload", "", currentUserEmail(rc), src, logger)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to save image (CWB_IMG_015)"})
	}
	return c.JSON(http.StatusOK, meta)
}

// ListImages handles GET /api/v1/images.
func ListImages(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_IMG_002")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	rows, err := db.Query(
		`SELECT id, filename, size_bytes, content_type, origin, created_at
		   FROM kb.images
		  ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		logger.Error("list images failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to list images (CWB_IMG_020)"})
	}
	defer rows.Close()

	list := make([]imageMeta, 0)
	for rows.Next() {
		var m imageMeta
		if err := rows.Scan(&m.ID, &m.Filename, &m.SizeBytes, &m.ContentType, &m.Origin, &m.CreatedAt); err != nil {
			logger.Error("scan image row failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to read images (CWB_IMG_021)"})
		}
		m.ContentURL = contentURL(m.ID)
		list = append(list, m)
	}
	return c.JSON(http.StatusOK, list)
}

// ServeImageContent handles GET /api/v1/images/:id/content.
func ServeImageContent(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_IMG_003")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := parseID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "invalid image id (CWB_IMG_030)"})
	}

	var path, contentType string
	err = ApiTypes.ProjectDBHandle.QueryRow(
		`SELECT stored_path, content_type FROM kb.images WHERE id = $1`, id,
	).Scan(&path, &contentType)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, errorResponse{false, "image not found (CWB_IMG_031)"})
	}
	if err != nil {
		logger.Error("lookup image failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to load image (CWB_IMG_032)"})
	}

	f, err := os.Open(path)
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{false, "image file missing (CWB_IMG_033)"})
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to read image (CWB_IMG_034)"})
	}

	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(c.Response(), c.Request(), filepath.Base(path), info.ModTime(), f)
	return nil
}

// DeleteImage handles DELETE /api/v1/images/:id.
func DeleteImage(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_IMG_004")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := parseID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "invalid image id (CWB_IMG_040)"})
	}

	var path string
	err = ApiTypes.ProjectDBHandle.QueryRow(`SELECT stored_path FROM kb.images WHERE id = $1`, id).Scan(&path)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, errorResponse{false, "image not found (CWB_IMG_041)"})
	}
	if err != nil {
		logger.Error("lookup image failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to load image (CWB_IMG_042)"})
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Error("remove image file failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to delete image file (CWB_IMG_043)"})
	}
	if _, err := ApiTypes.ProjectDBHandle.Exec(`DELETE FROM kb.images WHERE id = $1`, id); err != nil {
		logger.Error("delete image metadata failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "failed to delete image (CWB_IMG_044)"})
	}
	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}

// --- helpers ---

// storeImage writes bytes to dir and inserts a kb.images row, returning metadata.
// On DB failure the written file is removed.
func storeImage(
	dir, filename, contentType, origin, prompt, uploadedBy string,
	src io.Reader,
	logger ApiTypes.JimoLogger,
) (imageMeta, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Error("create image dir failed", "dir", dir, "err", err)
		return imageMeta{}, err
	}
	if filename == "" {
		filename = "image"
	}
	storedName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFilename(filename))
	destPath := filepath.Join(dir, storedName)

	dst, err := os.Create(destPath)
	if err != nil {
		logger.Error("create image file failed", "err", err)
		return imageMeta{}, err
	}
	written, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(destPath)
		logger.Error("write image file failed", "copyErr", copyErr, "closeErr", closeErr)
		if copyErr != nil {
			return imageMeta{}, copyErr
		}
		return imageMeta{}, closeErr
	}

	var m imageMeta
	err = ApiTypes.ProjectDBHandle.QueryRow(
		`INSERT INTO kb.images (filename, stored_path, size_bytes, content_type, origin, prompt, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, filename, size_bytes, content_type, origin, created_at`,
		filename, destPath, written, contentType, origin, nullableString(prompt), nullableString(uploadedBy),
	).Scan(&m.ID, &m.Filename, &m.SizeBytes, &m.ContentType, &m.Origin, &m.CreatedAt)
	if err != nil {
		_ = os.Remove(destPath)
		logger.Error("insert image metadata failed", "err", err)
		return imageMeta{}, err
	}
	m.ContentURL = contentURL(m.ID)
	logger.Info("image stored", "id", m.ID, "origin", origin, "size", m.SizeBytes)
	return m, nil
}

func parseID(c echo.Context) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
}

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
		return "image"
	}
	return cleaned
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
