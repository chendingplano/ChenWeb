package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type updateRecordTopicRequest struct {
	RecordID        int64    `json:"record_id"`
	TopicID         string   `json:"topic_id"`
	TopicType       string   `json:"topic_type"`
	TopicText       string   `json:"topic_text"`
	TopicDescEn     string   `json:"topic_desc_en"`
	TopicKeywords   []string `json:"topic_keywords"`
	TopicKeywordsEn []string `json:"topic_keywords_en"`
	CategoryPaths   []string `json:"category_paths"`
	CategoryPathsEn []string `json:"category_paths_en"`
}

type updateRecordTopicResponse struct {
	Status bool `json:"status"`
}

func UpdateRecordTopic(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_TUPD_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req updateRecordTopicRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid request body (CWB_KB_TUPD_010)",
		})
	}
	if req.RecordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing or invalid record_id (CWB_KB_TUPD_011)",
		})
	}
	if strings.TrimSpace(req.TopicID) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing topic_id (CWB_KB_TUPD_012)",
		})
	}
	if req.TopicKeywords == nil {
		req.TopicKeywords = []string{}
	}
	if req.TopicKeywordsEn == nil {
		req.TopicKeywordsEn = []string{}
	}
	if req.CategoryPaths == nil {
		req.CategoryPaths = []string{}
	}
	if req.CategoryPathsEn == nil {
		req.CategoryPathsEn = []string{}
	}

	logger.Info("update record topic request", "record_id", req.RecordID, "topic_id", req.TopicID)

	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing ARTIFACT_DIR (CWB_KB_TUPD_020)",
		})
	}

	recordDir, err := resolveRecordArtifactDir(artifactDir, req.RecordID)
	if err != nil {
		logger.Error("resolve record artifact dir failed", "record_id", req.RecordID, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: "record artifact directory not found (CWB_KB_TUPD_021)",
		})
	}

	oldCategoryPaths, topicLines, err := updateTopicsArtifactFile(recordDir, req)
	if err != nil {
		logger.Error("update topics file failed", "record_id", req.RecordID, "topic_id", req.TopicID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to update topics file (CWB_KB_TUPD_030)",
		})
	}

	topicTreeDir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if topicTreeDir != "" {
		if err := updateTopicCategoryIndex(logger, topicTreeDir, req, oldCategoryPaths, topicLines); err != nil {
			logger.Warn("update topic category index failed", "err", err)
		}
	}
	if err := docprocessing.ReplaceTopicArtifactsFromArtifactFiles(context.Background(), req.RecordID, logger); err != nil {
		logger.Warn("refresh topic table from artifact files failed", "record_id", req.RecordID, "err", err)
	} else if err := docprocessing.ReindexTopicSearchForRecord(context.Background(), req.RecordID, logger); err != nil {
		logger.Warn("reindex topic search after manual update failed", "record_id", req.RecordID, "err", err)
	}

	logger.Info("update record topic success", "record_id", req.RecordID, "topic_id", req.TopicID)
	return c.JSON(http.StatusOK, updateRecordTopicResponse{Status: true})
}

// updateTopicsArtifactFile finds the .topics file containing the topic, updates its editable
// fields, and returns the old category paths and line specs for category index adjustment.
func updateTopicsArtifactFile(recordDir string, req updateRecordTopicRequest) (oldCategoryPaths []string, topicLines []string, err error) {
	entries, err := filepath.Glob(filepath.Join(recordDir, "*.topics"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(entries)

	for _, path := range entries {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		old, found, updated, err := rewriteTopicInContent(string(body), req)
		if err != nil || !found {
			continue
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return nil, nil, fmt.Errorf("write topics file: %w", err)
		}
		return old.categoryPaths, old.lines, nil
	}

	return nil, nil, fmt.Errorf("topic %s not found in record %d", req.TopicID, req.RecordID)
}

// rewriteTopicInContent splits the file content into blocks, updates the matching block, and
// returns the old parsed entry, whether it was found, and the new file content.
func rewriteTopicInContent(content string, req updateRecordTopicRequest) (parsedTopicEntry, bool, string, error) {
	rawBlocks := splitIntoTopicBlocks(content)
	var oldEntry parsedTopicEntry
	found := false

	updatedBlocks := make([]string, 0, len(rawBlocks))
	for _, block := range rawBlocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		parsed := parseSingleTopicBlock(block)
		if strings.TrimSpace(parsed.topicID) == strings.TrimSpace(req.TopicID) {
			oldEntry = parsed
			updatedBlocks = append(updatedBlocks, rewriteTopicBlock(block, req))
			found = true
		} else {
			updatedBlocks = append(updatedBlocks, strings.TrimSpace(block))
		}
	}

	if !found {
		return parsedTopicEntry{}, false, "", nil
	}
	return oldEntry, true, strings.Join(updatedBlocks, "\n\n") + "\n", nil
}

func splitIntoTopicBlocks(content string) []string {
	var blocks []string
	var current strings.Builder
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			if current.Len() > 0 {
				blocks = append(blocks, current.String())
				current.Reset()
			}
		} else {
			current.WriteString(line)
			current.WriteRune('\n')
		}
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks
}

func parseSingleTopicBlock(block string) parsedTopicEntry {
	entry := parsedTopicEntry{keywords: []string{}, lines: []string{}}
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "topic_id:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_id:"))
			raw = strings.TrimSuffix(raw, ",")
			entry.topicID = strings.Trim(raw, `"`)
		case strings.HasPrefix(trimmed, "category_paths_en:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths_en:"))
			entry.categoryPathsEn = parseTopicCategoryPaths(raw)
		case strings.HasPrefix(trimmed, "category_paths:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths:"))
			entry.categoryPaths = parseTopicCategoryPaths(raw)
		case strings.HasPrefix(trimmed, "lines:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "lines:"))
			entry.lines = parseQuotedStringArray(raw)
		case strings.HasPrefix(trimmed, "topic_desc_en:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_desc_en:"))
			entry.topicDescEn = strings.Trim(raw, `"`)
		case strings.HasPrefix(trimmed, "topic_desc:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_desc:"))
			entry.topicText = strings.Trim(raw, `"`)
		case strings.HasPrefix(trimmed, "topic:") && !strings.HasPrefix(trimmed, "topic_"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic:"))
			if entry.topicText == "" {
				entry.topicText = strings.Trim(raw, `"`)
			}
		}
	}
	return entry
}

func rewriteTopicBlock(block string, req updateRecordTopicRequest) string {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	var out []string
	written := map[string]bool{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "topic_type:"):
			out = append(out, "topic_type: "+topicJSONString(req.TopicType))
			written["topic_type"] = true
		case strings.HasPrefix(trimmed, "topic_desc_en:"):
			out = append(out, "topic_desc_en: "+topicJSONString(req.TopicDescEn))
			written["topic_desc_en"] = true
		case strings.HasPrefix(trimmed, "topic_desc:"):
			out = append(out, "topic_desc: "+topicJSONString(req.TopicText))
			written["topic_desc"] = true
		case strings.HasPrefix(trimmed, "topic:") && !strings.HasPrefix(trimmed, "topic_"):
			// replace legacy "topic:" with "topic_desc:" and skip the old line
			if !written["topic_desc"] {
				out = append(out, "topic_desc: "+topicJSONString(req.TopicText))
				written["topic_desc"] = true
			}
		case strings.HasPrefix(trimmed, "topic_keywords_en:"):
			out = append(out, "topic_keywords_en: "+topicJSONStringArray(req.TopicKeywordsEn))
			written["topic_keywords_en"] = true
		case strings.HasPrefix(trimmed, "topic_keywords:"):
			out = append(out, "topic_keywords: "+topicJSONStringArray(req.TopicKeywords))
			written["topic_keywords"] = true
		case strings.HasPrefix(trimmed, "category_paths_en:"):
			out = append(out, "category_paths_en: "+topicJSONStringArray(req.CategoryPathsEn))
			written["category_paths_en"] = true
		case strings.HasPrefix(trimmed, "category_paths:"):
			out = append(out, "category_paths: "+topicJSONStringArray(req.CategoryPaths))
			written["category_paths"] = true
		default:
			out = append(out, line)
		}
	}

	if !written["topic_type"] {
		out = append(out, "topic_type: "+topicJSONString(req.TopicType))
	}
	if !written["topic_desc"] {
		out = append(out, "topic_desc: "+topicJSONString(req.TopicText))
	}
	if req.TopicDescEn != "" && !written["topic_desc_en"] {
		out = append(out, "topic_desc_en: "+topicJSONString(req.TopicDescEn))
	}
	if !written["topic_keywords"] {
		out = append(out, "topic_keywords: "+topicJSONStringArray(req.TopicKeywords))
	}
	if len(req.TopicKeywordsEn) > 0 && !written["topic_keywords_en"] {
		out = append(out, "topic_keywords_en: "+topicJSONStringArray(req.TopicKeywordsEn))
	}
	if !written["category_paths"] {
		out = append(out, "category_paths: "+topicJSONStringArray(req.CategoryPaths))
	}
	if len(req.CategoryPathsEn) > 0 && !written["category_paths_en"] {
		out = append(out, "category_paths_en: "+topicJSONStringArray(req.CategoryPathsEn))
	}

	return strings.Join(out, "\n")
}

func topicJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func topicJSONStringArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(items)
	return string(b)
}

// updateTopicCategoryIndex adjusts the topics.txt index files in ARTIFACT_WEB_DIR when
// category paths change.
func updateTopicCategoryIndex(logger ApiTypes.JimoLogger, topicTreeDir string, req updateRecordTopicRequest, oldPaths []string, topicLines []string) error {
	oldSet := stringSliceToSet(oldPaths)
	newSet := stringSliceToSet(req.CategoryPaths)

	for _, path := range oldPaths {
		if !newSet[path] {
			if err := removeTopicFromCategoryIndex(topicTreeDir, path, req.RecordID, req.TopicID); err != nil {
				logger.Warn("remove topic from category index failed", "category", path, "err", err)
			}
		}
	}

	for _, path := range req.CategoryPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !oldSet[path] {
			if err := addTopicToCategoryIndex(topicTreeDir, path, req.RecordID, req.TopicID, topicLines, req.TopicText); err != nil {
				logger.Warn("add topic to category index failed", "category", path, "err", err)
			}
		} else {
			if err := updateTopicTextInCategoryIndex(topicTreeDir, path, req.RecordID, req.TopicID, req.TopicText); err != nil {
				logger.Warn("update topic text in category index failed", "category", path, "err", err)
			}
		}
	}

	return nil
}

func stringSliceToSet(s []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range s {
		v = strings.TrimSpace(v)
		if v != "" {
			m[v] = true
		}
	}
	return m
}

func categoryTopicsFilePath(topicTreeDir, categoryPath string) string {
	return filepath.Join(topicTreeDir, filepath.FromSlash(strings.TrimSpace(categoryPath)), "topics.txt")
}

func removeTopicFromCategoryIndex(topicTreeDir, categoryPath string, recordID int64, topicID string) error {
	path := categoryTopicsFilePath(topicTreeDir, categoryPath)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	refs := parseTopicCategoryRefs(body)
	filtered := make([]topicCategoryReference, 0, len(refs))
	for _, ref := range refs {
		if ref.RecordID == recordID && strings.TrimSpace(ref.TopicID) == strings.TrimSpace(topicID) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return writeTopicCategoryRefsFile(path, filtered)
}

func addTopicToCategoryIndex(topicTreeDir, categoryPath string, recordID int64, topicID string, lines []string, topicText string) error {
	dir := filepath.Join(topicTreeDir, filepath.FromSlash(strings.TrimSpace(categoryPath)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(dir, "topics.txt")
	var existing []topicCategoryReference
	if body, err := os.ReadFile(path); err == nil {
		existing = parseTopicCategoryRefs(body)
	}

	// Avoid duplicate entries
	for _, ref := range existing {
		if ref.RecordID == recordID && strings.TrimSpace(ref.TopicID) == strings.TrimSpace(topicID) {
			return nil
		}
	}

	existing = append(existing, topicCategoryReference{
		RecordID:  recordID,
		TopicID:   strings.TrimSpace(topicID),
		TopicText: strings.TrimSpace(topicText),
		Lines:     append([]string(nil), lines...),
	})
	return writeTopicCategoryRefsFile(path, existing)
}

func updateTopicTextInCategoryIndex(topicTreeDir, categoryPath string, recordID int64, topicID string, topicText string) error {
	path := categoryTopicsFilePath(topicTreeDir, categoryPath)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	refs := parseTopicCategoryRefs(body)
	changed := false
	for i, ref := range refs {
		if ref.RecordID == recordID && strings.TrimSpace(ref.TopicID) == strings.TrimSpace(topicID) {
			refs[i].TopicText = strings.TrimSpace(topicText)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeTopicCategoryRefsFile(path, refs)
}

func writeTopicCategoryRefsFile(path string, refs []topicCategoryReference) error {
	var sb strings.Builder
	for i, ref := range refs {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "record_id: %d,\n", ref.RecordID)
		fmt.Fprintf(&sb, "topic_id: %s,\n", topicJSONString(ref.TopicID))
		if len(ref.Lines) > 0 {
			fmt.Fprintf(&sb, "lines: %s,\n", topicJSONStringArray(ref.Lines))
		}
		if ref.TopicText != "" {
			fmt.Fprintf(&sb, "topic: %s\n", topicJSONString(ref.TopicText))
		}
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
