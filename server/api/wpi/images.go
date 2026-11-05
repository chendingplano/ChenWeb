package wpi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/labstack/echo/v4"
)

// Page images are PNGs of a page, made by the editor when it saves the page
// (OpenSpec wpi-page-images). They live in <DATA_HOME_DIR>/page-images and a
// page names its image in the root attribute page_image.

const (
	pageImageDir = "page-images"
	// maxPageImageBytes bounds an uploaded image.
	maxPageImageBytes = 4 << 20
)

// pageImageName is the form of a stored image's file name.
var pageImageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,160}\.png$`)

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

func pageImagesPath() string {
	home := pathutil.DataHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, pageImageDir)
}

// PutPageImage stores an uploaded PNG and returns the name it was stored under.
// Endpoint: PUT /api/v1/wpi/page-images/:pageId (admins only)
func (h *Handler) PutPageImage(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	if !isAdmin(user) {
		return errorJSON(c, http.StatusForbidden, "Admin access required")
	}
	pageID := c.Param("pageId")
	if !idPattern.MatchString(pageID) {
		return errorJSON(c, http.StatusBadRequest, "Invalid page ID")
	}
	dir := pageImagesPath()
	if dir == "" {
		return errorJSON(c, http.StatusServiceUnavailable, "DATA_HOME_DIR is not set")
	}
	data, err := io.ReadAll(io.LimitReader(c.Request().Body, maxPageImageBytes+1))
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Unable to read request body")
	}
	if len(data) > maxPageImageBytes {
		return errorJSON(c, http.StatusRequestEntityTooLarge, "Image is larger than 4 MiB")
	}
	if !bytes.HasPrefix(data, pngSignature) {
		return errorJSON(c, http.StatusBadRequest, "Body must be a PNG image")
	}
	var suffix [2]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "Unable to store image")
	}
	name := pageID + "-" + time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString(suffix[:]) + ".png"
	if !pageImageName.MatchString(name) {
		return errorJSON(c, http.StatusBadRequest, "Invalid page ID")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		handlerLog.Error("WPI page image directory failed", "dir", dir, "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to store image")
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		handlerLog.Error("WPI page image write failed", "name", name, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to store image")
	}
	handlerLog.Info("WPI page image stored", "name", name, "bytes", len(data), "user", actorName(user))
	return c.JSON(http.StatusOK, map[string]any{"page_image": name})
}

// GetPageImage serves a stored image.
// Endpoint: GET /api/v1/wpi/page-images/:name
func (h *Handler) GetPageImage(c echo.Context) error {
	if h.User(c) == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	name := c.Param("name")
	dir := pageImagesPath()
	if dir == "" || !pageImageName.MatchString(name) {
		return errorJSON(c, http.StatusNotFound, "Image not found")
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return errorJSON(c, http.StatusNotFound, "Image not found")
	}
	if err != nil {
		handlerLog.Error("WPI page image read failed", "name", name, "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to read image")
	}
	c.Response().Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	return c.Blob(http.StatusOK, "image/png", data)
}
