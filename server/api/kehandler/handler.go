package kehandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var logger = loggerutil.CreateDefaultLogger("CWB_KE_001")

// reSafeName allows letters, digits, hyphens, and underscores only — no path separators.
var reSafeName = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

// ResearchTopicBean mirrors the RTB ontology defined in research-topics.md.
type ResearchTopicBean struct {
	BeanName         string   `json:"bean_name"`
	BeanDesc         string   `json:"bean_desc,omitempty"`
	BeanKeywords     []string `json:"bean_keywords,omitempty"`
	BeanType         string   `json:"bean_type"`
	RelatedTopics    []string `json:"related_topics,omitempty"`
	ResearchTitle    string   `json:"research_title"`
	ResearchSubtitle string   `json:"research_subtitle"`
	FileType         string   `json:"file_type"`
	BeanCategory     string   `json:"bean_category"`
	Authors          []string `json:"authors,omitempty"`
	Content          string   `json:"content,omitempty"`
	CreatedAt        string   `json:"created_at,omitempty"`
}

var fileTypeExtensions = map[string]string{
	"Markdown": "md",
	"Typst":    "typ",
	"Text":     "text",
	"HTML":     "html",
}

var validBeanTypes = map[string]bool{
	"Thoughts": true, "Design": true, "Spec": true,
	"Implementation": true, "Reading": true,
}

func topicsBaseDir() (string, error) {
	dir := os.Getenv("ARTIFACT_DIR")
	if dir == "" {
		return "", fmt.Errorf("ARTIFACT_DIR environment variable is not set")
	}
	return filepath.Join(dir, "Topics"), nil
}

func metaFilePath(base string, bean ResearchTopicBean) string {
	return filepath.Join(base, bean.BeanCategory, bean.BeanName+".json")
}

func contentFilePath(base string, bean ResearchTopicBean) (string, error) {
	ext, ok := fileTypeExtensions[bean.FileType]
	if !ok {
		return "", fmt.Errorf("unknown file_type: %s", bean.FileType)
	}
	return filepath.Join(base, bean.BeanCategory, bean.BeanName+"."+ext), nil
}

func validateBean(bean ResearchTopicBean) error {
	if bean.BeanName == "" {
		return fmt.Errorf("bean_name is required")
	}
	if !reSafeName.MatchString(bean.BeanName) {
		return fmt.Errorf("bean_name must contain only letters, digits, hyphens, and underscores")
	}
	if !validBeanTypes[bean.BeanType] {
		return fmt.Errorf("invalid bean_type: %s", bean.BeanType)
	}
	if bean.ResearchTitle == "" {
		return fmt.Errorf("research_title is required")
	}
	if bean.ResearchSubtitle == "" {
		return fmt.Errorf("research_subtitle is required")
	}
	if _, ok := fileTypeExtensions[bean.FileType]; !ok {
		return fmt.Errorf("invalid file_type: %s", bean.FileType)
	}
	if bean.BeanCategory == "" {
		return fmt.Errorf("bean_category is required")
	}
	// Prevent path traversal in category
	clean := filepath.Clean(bean.BeanCategory)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return fmt.Errorf("invalid bean_category")
	}
	return nil
}

// List scans ARTIFACT_DIR/Topics/**/*.json and returns all beans.
//
// GET /api/v1/ke/research-topics
func List(c echo.Context) error {
	base, err := topicsBaseDir()
	if err != nil {
		logger.Error("[ke] missing ARTIFACT_DIR", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	beans := []ResearchTopicBean{}
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("[ke] failed to read bean file", "path", path, "error", err)
			return nil
		}
		var b ResearchTopicBean
		if err := json.Unmarshal(raw, &b); err != nil {
			logger.Warn("[ke] failed to parse bean file", "path", path, "error", err)
			return nil
		}
		beans = append(beans, b)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		logger.Error("[ke] failed to walk topics dir", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	logger.Info("[ke] listed research topic beans", "count", len(beans))
	return c.JSON(http.StatusOK, beans)
}

// Create writes a new RTB: a metadata JSON sidecar and the content artifact file.
// File paths:
//
//	ARTIFACT_DIR/Topics/<bean_category>/<bean_name>.json      — metadata
//	ARTIFACT_DIR/Topics/<bean_category>/<bean_name>.<ext>     — content artifact
//
// POST /api/v1/ke/research-topics
func Create(c echo.Context) error {
	base, err := topicsBaseDir()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var body ResearchTopicBean
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := validateBean(body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	body.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if body.BeanKeywords == nil {
		body.BeanKeywords = []string{}
	}
	if body.RelatedTopics == nil {
		body.RelatedTopics = []string{}
	}
	if body.Authors == nil {
		body.Authors = []string{}
	}

	metaPath := metaFilePath(base, body)
	contentPath, err := contentFilePath(base, body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if _, statErr := os.Stat(metaPath); statErr == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "a bean with this name already exists in this category"})
	}

	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		logger.Error("[ke] failed to create directory", "path", metaPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	raw, _ := json.MarshalIndent(body, "", "  ")
	if err := os.WriteFile(metaPath, raw, 0o644); err != nil {
		logger.Error("[ke] failed to write bean metadata", "path", metaPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if err := os.WriteFile(contentPath, []byte(body.Content), 0o644); err != nil {
		logger.Error("[ke] failed to write bean content", "path", contentPath, "error", err)
		_ = os.Remove(metaPath) // roll back metadata on content write failure
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	logger.Info("[ke] created research topic bean",
		"name", body.BeanName,
		"category", body.BeanCategory,
		"meta", metaPath,
		"content", contentPath,
	)
	return c.JSON(http.StatusCreated, body)
}
