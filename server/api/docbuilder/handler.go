package docbuilder

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// safeJoin joins a manifest-relative slash path onto dir.
func safeJoin(dir, rel string) string { return filepath.Join(dir, filepath.FromSlash(rel)) }

// Register adds the read-only routes to an authenticated group:
//
//	GET /docbuilder/:source/tree[?all=1]  → {title, tree, uncategorized}; only the latest version of each
//	                                         document unless all=1
//	GET /docbuilder/:source/file?path=... → the Markdown text or the PDF bytes
//
// Serving never calls an LLM: categories come from the saved manifest, and files not yet
// classified appear under Uncategorized. Only files the scan found can be fetched.
func Register(g *echo.Group, sources map[string]Source) {
	g.GET("/docbuilder/:source/tree", func(c echo.Context) error {
		src, ok := sources[c.Param("source")]
		if !ok {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "unknown source"})
		}
		m, err := loadFresh(src.Dir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "unable to read documents"})
		}
		entries := m.Entries
		if c.QueryParam("all") != "1" {
			entries = LatestVersions(entries)
		}
		pending := 0
		for _, e := range entries {
			if e.Category == "" {
				pending++
			}
		}
		return c.JSON(http.StatusOK, map[string]any{"title": src.Title, "tree": BuildTree(entries), "uncategorized": pending})
	})
	g.GET("/docbuilder/:source/file", func(c echo.Context) error {
		src, ok := sources[c.Param("source")]
		if !ok {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "unknown source"})
		}
		m, err := loadFresh(src.Dir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "unable to read documents"})
		}
		want := c.QueryParam("path")
		for _, e := range m.Entries {
			if e.Path != want {
				continue
			}
			raw, err := os.ReadFile(safeJoin(src.Dir, e.Path))
			if err != nil {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "file not found"})
			}
			ct := "text/markdown; charset=utf-8"
			if e.Kind == KindPDF {
				ct = "application/pdf"
			}
			c.Response().Header().Set("Content-Disposition", "inline")
			return c.Blob(http.StatusOK, ct, raw)
		}
		return c.JSON(http.StatusNotFound, map[string]string{"error": "file not found"})
	})
}

// loadFresh merges a new scan into the saved manifest without writing it.
func loadFresh(dir string) (*Manifest, error) {
	m, err := LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	if err := Refresh(dir, m); err != nil {
		return nil, err
	}
	return m, nil
}
