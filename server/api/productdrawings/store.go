package productdrawings

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

const maxPageSize = 50

func List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if pageSize < 1 || pageSize > maxPageSize {
		pageSize = 12
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
	}
	ctx := c.Request().Context()
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.product_drawings`).Scan(&total); err != nil {
		return c.JSON(500, map[string]string{"error": "failed to count drawings"})
	}
	rows, err := db.QueryContext(ctx, `SELECT id,name,description,prompt,keywords,notes,filename,model,model_name,created_at,updated_at FROM kb.product_drawings ORDER BY created_at DESC,id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "failed to load drawings"})
	}
	defer rows.Close()
	out := []ProductDrawing{}
	for rows.Next() {
		var d ProductDrawing
		var created, updated time.Time
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.Prompt, &d.Keywords, &d.Notes, &d.Filename, &d.Model, &d.ModelName, &created, &updated); err != nil {
			return c.JSON(500, map[string]string{"error": "failed to read drawings"})
		}
		d.CreatedAt = created.UTC().Format(time.RFC3339)
		d.UpdatedAt = updated.UTC().Format(time.RFC3339)
		d.ImageURL = fmt.Sprintf("/api/v1/product-drawings/%d/content", d.ID)
		out = append(out, d)
	}
	return c.JSON(http.StatusOK, ProductDrawingPage{Drawings: out, Total: total, Page: page, PageSize: pageSize})
}

func Update(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(400, map[string]string{"error": "invalid drawing id"})
	}
	var req Request
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request"})
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Prompt) == "" {
		return c.JSON(400, map[string]string{"error": "name and prompt are required"})
	}
	selection := normalizeSelection(req.Model)
	_, err = ApiTypes.ProjectDBHandle.ExecContext(c.Request().Context(), `UPDATE kb.product_drawings SET name=$1,description=$2,prompt=$3,keywords=$4,notes=$5,model=$6,model_name=$7,updated_at=NOW() WHERE id=$8`, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), strings.TrimSpace(req.Prompt), strings.TrimSpace(req.Keywords), strings.TrimSpace(req.Notes), selection, modelNameForSelection(selection), id)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "failed to update drawing"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

func Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(400, map[string]string{"error": "invalid drawing id"})
	}
	var stored string
	err = ApiTypes.ProjectDBHandle.QueryRowContext(c.Request().Context(), `DELETE FROM kb.product_drawings WHERE id=$1 RETURNING stored_path`, id).Scan(&stored)
	if err == sql.ErrNoRows {
		return c.JSON(404, map[string]string{"error": "drawing not found"})
	}
	if err != nil {
		return c.JSON(500, map[string]string{"error": "failed to delete drawing"})
	}
	if !safeStoredPath(stored) {
		return c.JSON(500, map[string]string{"error": "drawing path is invalid"})
	}
	if err := os.Remove(stored); err != nil && !os.IsNotExist(err) {
		return c.JSON(500, map[string]string{"error": "drawing metadata deleted but file removal failed"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

func Content(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(400, map[string]string{"error": "invalid drawing id"})
	}
	var stored string
	if err := ApiTypes.ProjectDBHandle.QueryRowContext(c.Request().Context(), `SELECT stored_path FROM kb.product_drawings WHERE id=$1`, id).Scan(&stored); err != nil {
		return c.JSON(404, map[string]string{"error": "drawing not found"})
	}
	if !safeStoredPath(stored) {
		return c.JSON(500, map[string]string{"error": "drawing path is invalid"})
	}
	return c.File(stored)
}

func safeStoredPath(path string) bool {
	dir := filepath.Clean(envOr("PRODUCT_DRAWINGS_DIR", "/Users/cding/Workspace/KnowledgeStore/doc-repo/resources/product-drawings"))
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(dir, clean)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
