// Package terminologyresourcehandler implements the External Terminology
// Resources admin API backing System Admin > Resources. Downloads run on the
// server and write local artifacts + unapproved draft manifests under
// TERMINOLOGY_DIR (or <DATA_HOME_DIR>/terminology); the same directory feeds
// terminology-import after operator approval.
package terminologyresourcehandler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
	"github.com/labstack/echo/v4"
)

const downloadTimeout = 10 * time.Minute

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error"`
}

// resourceView is the JSON shape served to the admin page: catalog entry
// merged with the persisted download status.
type resourceView struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	URL                string     `json:"url"`
	Release            string     `json:"release"`
	License            string     `json:"license"`
	LicenseURL         string     `json:"license_url"`
	CanDownload        bool       `json:"can_download"`
	PermissionRequired bool       `json:"permission_required"`
	Notes              string     `json:"notes"`
	Downloaded         bool       `json:"downloaded"`
	DownloadedAt       *time.Time `json:"downloaded_at"`
	SHA256             string     `json:"sha256"`
	SizeBytes          int64      `json:"size_bytes"`
	Artifact           string     `json:"artifact"`
	SourceURL          string     `json:"source_url"`
	ManifestDraft      string     `json:"manifest_draft"`
	Error              string     `json:"error"`
}

// terminologyDir resolves the fetch storage directory. TERMINOLOGY_DIR is the
// explicit override; DATA_HOME_DIR is the fallback so downloads work on any
// deployment that already sets it (mirrors videohandler).
func terminologyDir() string {
	if v := strings.TrimSpace(os.Getenv("TERMINOLOGY_DIR")); v != "" {
		return v
	}
	if home := strings.TrimSpace(os.Getenv("DATA_HOME_DIR")); home != "" {
		return filepath.Join(home, "terminology")
	}
	return ""
}

func viewFor(res terminology.Resource, st terminology.FetchStatus) resourceView {
	v := resourceView{
		ID: string(res.ID), Name: res.Name, Description: res.Description,
		URL: res.URL, Release: res.Release, License: res.License,
		LicenseURL: res.LicenseURL, CanDownload: res.Downloadable,
		PermissionRequired: res.PermissionRequired, Notes: res.Notes,
		Downloaded: st.Downloaded, SHA256: st.SHA256, SizeBytes: st.SizeBytes,
		Artifact: st.Artifact, SourceURL: st.SourceURL, ManifestDraft: st.ManifestDraft,
		Error: st.Error,
	}
	if !st.DownloadedAt.IsZero() {
		t := st.DownloadedAt
		v.DownloadedAt = &t
	}
	if st.Release != "" {
		v.Release = st.Release
	}
	return v
}

// ListResources returns every portfolio resource with its persisted status.
func ListResources(c echo.Context) error {
	dir := terminologyDir()
	if dir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "TERMINOLOGY_DIR or DATA_HOME_DIR is not set"})
	}
	views := make([]resourceView, 0, len(terminology.Resources()))
	for _, res := range terminology.Resources() {
		st, err := terminology.ReadStatus(dir, res.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorResponse{false, err.Error()})
		}
		views = append(views, viewFor(res, st))
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "resources": views})
}

// DownloadResource fetches one resource and returns its updated status.
func DownloadResource(c echo.Context) error {
	dir := terminologyDir()
	if dir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "TERMINOLOGY_DIR or DATA_HOME_DIR is not set"})
	}
	id := terminology.ResourceID(c.Param("source"))
	if _, ok := terminology.ResourceByID(id); !ok {
		return c.JSON(http.StatusNotFound, errorResponse{false, "unknown resource: " + string(id)})
	}
	opts := []terminology.FetchOption{terminology.WithClient(&http.Client{Timeout: downloadTimeout})}
	if titles := c.QueryParam("titles"); titles != "" {
		opts = append(opts, terminology.WithWikidataTitles(strings.Split(titles, "|")))
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), downloadTimeout)
	defer cancel()

	st, err := terminology.Fetch(ctx, id, dir, opts...)
	res, _ := terminology.ResourceByID(id)
	if err != nil {
		// Permission-gated resources are a client error; transport failures are server-side.
		code := http.StatusInternalServerError
		if !res.Downloadable {
			code = http.StatusForbidden
		}
		return c.JSON(code, errorResponse{false, err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "resource": viewFor(res, st)})
}
