package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// artifactCategoryItem is a lightweight record returned for any artifact type when
// browsing by category path. It carries just enough to display the artifact in the
// Artifact Wiki graph and to link to the source PDF.
type artifactCategoryItem struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Sublabel string `json:"sublabel,omitempty"`
	InputID  int64  `json:"inputId"`
	Page     int    `json:"page"`
}

type getArtifactCategoryResponse struct {
	Status       bool                   `json:"status"`
	CategoryPath string                 `json:"categoryPath"`
	Type         string                 `json:"type"`
	Items        []artifactCategoryItem `json:"items"`
}

// readArtifactIndexIDs reads one artifact ID per non-blank line from
// ARTIFACT_WEB_DIR/{categoryPath}/{filename}.
func readArtifactIndexIDs(artifactWebDir, categoryPath, filename string) ([]string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(categoryPath))
	if cleanPath == "." || cleanPath == "" {
		return nil, fmt.Errorf("invalid category path")
	}
	if strings.HasPrefix(cleanPath, "..") {
		return nil, fmt.Errorf("invalid category path")
	}
	indexPath := filepath.Join(artifactWebDir, filepath.FromSlash(cleanPath), filename)
	body, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	ids := make([]string, 0)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ids = append(ids, line)
	}
	sort.Strings(ids)
	return ids, nil
}

// buildPlaceholders returns "$1,$2,...,$n" for n items.
func buildPlaceholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("$%d", i+1)
	}
	return strings.Join(parts, ",")
}

// ---------- Metric Category ----------

// GetMetricCategory handles GET /api/v1/kb/metric-category?category_path=X
// It reads metrics.txt from ARTIFACT_WEB_DIR/{categoryPath}/, treats each line as
// a numeric kb.metrics.id, and returns lightweight metric records.
func GetMetricCategory(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MCAT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	artifactWebDir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if artifactWebDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing ARTIFACT_WEB_DIR (CWB_KB_MCAT_010)",
		})
	}

	categoryPath := strings.TrimSpace(c.QueryParam("category_path"))
	if categoryPath == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing category_path (CWB_KB_MCAT_011)",
		})
	}

	rawIDs, err := readArtifactIndexIDs(artifactWebDir, categoryPath, "metrics.txt")
	if err != nil {
		logger.Error("read metric category index failed", "category_path", categoryPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read metric category index (CWB_KB_MCAT_012)",
		})
	}
	if len(rawIDs) == 0 {
		return c.JSON(http.StatusOK, getArtifactCategoryResponse{
			Status:       true,
			CategoryPath: categoryPath,
			Type:         "metrics",
			Items:        []artifactCategoryItem{},
		})
	}

	items, err := fetchMetricCategoryItems(ApiTypes.ProjectDBHandle, rawIDs)
	if err != nil {
		logger.Error("fetch metric category items failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to fetch metrics (CWB_KB_MCAT_013)",
		})
	}

	return c.JSON(http.StatusOK, getArtifactCategoryResponse{
		Status:       true,
		CategoryPath: categoryPath,
		Type:         "metrics",
		Items:        items,
	})
}

func fetchMetricCategoryItems(db *sql.DB, ids []string) ([]artifactCategoryItem, error) {
	if len(ids) == 0 {
		return []artifactCategoryItem{}, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	query := fmt.Sprintf(`
SELECT m.metric_id, m.input_record_id,
       COALESCE(m.metric_name, ''), COALESCE(m.metric_name_en, '')
FROM kb.metrics m
WHERE m.metric_id IN (%s)
ORDER BY m.metric_id`, buildPlaceholders(len(ids)))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]artifactCategoryItem, 0, len(ids))
	for rows.Next() {
		var metricID string
		var inputID int64
		var name, nameEn string
		if scanErr := rows.Scan(&metricID, &inputID, &name, &nameEn); scanErr != nil {
			return nil, scanErr
		}
		label := name
		if label == "" {
			label = nameEn
		}
		if label == "" {
			label = metricID
		}
		sublabel := ""
		if nameEn != "" && nameEn != name {
			sublabel = nameEn
		}
		items = append(items, artifactCategoryItem{
			ID:       metricID,
			Label:    label,
			Sublabel: sublabel,
			InputID:  inputID,
			Page:     1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ---------- Scene Category ----------

// GetSceneCategory handles GET /api/v1/kb/scene-category?category_path=X
// It reads scenes.txt from ARTIFACT_WEB_DIR/{categoryPath}/, treats each line as an
// object_id in kb.scene_objects, and returns lightweight scene records.
func GetSceneCategory(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SCCAT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	artifactWebDir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if artifactWebDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing ARTIFACT_WEB_DIR (CWB_KB_SCCAT_010)",
		})
	}

	categoryPath := strings.TrimSpace(c.QueryParam("category_path"))
	if categoryPath == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing category_path (CWB_KB_SCCAT_011)",
		})
	}

	rawIDs, err := readArtifactIndexIDs(artifactWebDir, categoryPath, "scenes.txt")
	if err != nil {
		logger.Error("read scene category index failed", "category_path", categoryPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read scene category index (CWB_KB_SCCAT_012)",
		})
	}
	if len(rawIDs) == 0 {
		return c.JSON(http.StatusOK, getArtifactCategoryResponse{
			Status:       true,
			CategoryPath: categoryPath,
			Type:         "scenes",
			Items:        []artifactCategoryItem{},
		})
	}

	items, err := fetchSceneCategoryItems(ApiTypes.ProjectDBHandle, rawIDs)
	if err != nil {
		logger.Error("fetch scene category items failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to fetch scenes (CWB_KB_SCCAT_013)",
		})
	}

	return c.JSON(http.StatusOK, getArtifactCategoryResponse{
		Status:       true,
		CategoryPath: categoryPath,
		Type:         "scenes",
		Items:        items,
	})
}

func fetchSceneCategoryItems(db *sql.DB, objectIDs []string) ([]artifactCategoryItem, error) {
	if len(objectIDs) == 0 {
		return []artifactCategoryItem{}, nil
	}
	args := make([]any, len(objectIDs))
	for i, id := range objectIDs {
		args[i] = id
	}
	query := fmt.Sprintf(`
SELECT s.id, s.input_record_id, s.object_id,
       COALESCE(s.title, ''), COALESCE(s.scene_type, '')
FROM kb.scene_objects s
WHERE s.object_id IN (%s)
ORDER BY s.id`, buildPlaceholders(len(objectIDs)))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]artifactCategoryItem, 0, len(objectIDs))
	for rows.Next() {
		var id, inputID int64
		var objectID, title, sceneType string
		if scanErr := rows.Scan(&id, &inputID, &objectID, &title, &sceneType); scanErr != nil {
			return nil, scanErr
		}
		label := title
		if label == "" {
			label = objectID
		}
		items = append(items, artifactCategoryItem{
			ID:       objectID,
			Label:    label,
			Sublabel: sceneType,
			InputID:  inputID,
			Page:     1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ---------- Product Category ----------

// GetProductCategory handles GET /api/v1/kb/product-category?category_path=X
// It reads products.txt from ARTIFACT_WEB_DIR/{categoryPath}/, treats each line as a
// product_rel_id in kb.products, and returns lightweight product records.
func GetProductCategory(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PRCAT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	artifactWebDir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if artifactWebDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing ARTIFACT_WEB_DIR (CWB_KB_PRCAT_010)",
		})
	}

	categoryPath := strings.TrimSpace(c.QueryParam("category_path"))
	if categoryPath == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing category_path (CWB_KB_PRCAT_011)",
		})
	}

	rawIDs, err := readArtifactIndexIDs(artifactWebDir, categoryPath, "products.txt")
	if err != nil {
		logger.Error("read product category index failed", "category_path", categoryPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read product category index (CWB_KB_PRCAT_012)",
		})
	}
	if len(rawIDs) == 0 {
		return c.JSON(http.StatusOK, getArtifactCategoryResponse{
			Status:       true,
			CategoryPath: categoryPath,
			Type:         "products",
			Items:        []artifactCategoryItem{},
		})
	}

	items, err := fetchProductCategoryItems(ApiTypes.ProjectDBHandle, rawIDs)
	if err != nil {
		logger.Error("fetch product category items failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to fetch products (CWB_KB_PRCAT_013)",
		})
	}

	return c.JSON(http.StatusOK, getArtifactCategoryResponse{
		Status:       true,
		CategoryPath: categoryPath,
		Type:         "products",
		Items:        items,
	})
}

func fetchProductCategoryItems(db *sql.DB, productRelIDs []string) ([]artifactCategoryItem, error) {
	if len(productRelIDs) == 0 {
		return []artifactCategoryItem{}, nil
	}
	args := make([]any, len(productRelIDs))
	for i, id := range productRelIDs {
		args[i] = id
	}
	query := fmt.Sprintf(`
SELECT p.id, p.input_record_id, p.product_rel_id,
       COALESCE(p.product_name, ''), COALESCE(p.product_name_en, '')
FROM kb.products p
WHERE p.product_rel_id IN (%s)
ORDER BY p.id`, buildPlaceholders(len(productRelIDs)))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]artifactCategoryItem, 0, len(productRelIDs))
	for rows.Next() {
		var id, inputID int64
		var relID, name, nameEn string
		if scanErr := rows.Scan(&id, &inputID, &relID, &name, &nameEn); scanErr != nil {
			return nil, scanErr
		}
		label := name
		if label == "" {
			label = nameEn
		}
		if label == "" {
			label = relID
		}
		sublabel := ""
		if nameEn != "" && nameEn != name {
			sublabel = nameEn
		}
		items = append(items, artifactCategoryItem{
			ID:       relID,
			Label:    label,
			Sublabel: sublabel,
			InputID:  inputID,
			Page:     1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ---------- Artifact graph node count helper ----------

// GetArtifactCategoryCounts handles GET /api/v1/kb/artifact-category-counts?category_path=X
// It checks which artifact index files exist under a category path and returns their line counts.
// This is used by the Artifact Graph tab to show which groups are populated.
func GetArtifactCategoryCounts(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ACAT_001")
	defer rc.Close()

	artifactWebDir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if artifactWebDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing ARTIFACT_WEB_DIR (CWB_KB_ACAT_010)",
		})
	}

	categoryPath := strings.TrimSpace(c.QueryParam("category_path"))
	if categoryPath == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing category_path (CWB_KB_ACAT_011)",
		})
	}

	type countEntry struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	}

	indexFiles := []struct {
		artType  string
		filename string
	}{
		{"summaries", "summaries.txt"},
		{"topics", "topics.txt"},
		{"metrics", "metrics.txt"},
		{"scenes", "scenes.txt"},
		{"provisions", "provisions.txt"},
		{"products", "products.txt"},
	}

	counts := make([]countEntry, 0, len(indexFiles))
	for _, entry := range indexFiles {
		ids, err := readArtifactIndexIDs(artifactWebDir, categoryPath, entry.filename)
		n := 0
		if err == nil {
			n = len(ids)
		}
		counts = append(counts, countEntry{Type: entry.artType, Count: n})
	}

	return c.JSON(http.StatusOK, struct {
		Status       bool         `json:"status"`
		CategoryPath string       `json:"categoryPath"`
		Counts       []countEntry `json:"counts"`
	}{
		Status:       true,
		CategoryPath: categoryPath,
		Counts:       counts,
	})
}

// jsonStringPtr safely marshals a *string field stored as JSON for query scan.
// Not used currently, kept for potential future extension.
var _ = json.Marshal
