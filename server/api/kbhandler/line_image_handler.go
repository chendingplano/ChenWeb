package kbhandler

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// GetLineImage serves an image relative to the input's artifact directory.
func GetLineImage(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	name := strings.TrimSpace(c.QueryParam("path"))
	if err != nil || id <= 0 || !filepath.IsLocal(name) {
		return c.NoContent(http.StatusBadRequest)
	}
	var contentType string
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	default:
		return c.NoContent(http.StatusBadRequest)
	}
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return c.NoContent(http.StatusInternalServerError)
	}
	root, err := os.OpenRoot(filepath.Join(artifactDir, strconv.FormatInt(id/1000, 10), strconv.FormatInt(id, 10)))
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return c.NoContent(http.StatusNotFound)
	}
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Response(), c.Request(), info.Name(), info.ModTime(), file)
	return nil
}
