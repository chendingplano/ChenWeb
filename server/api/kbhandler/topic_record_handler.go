package kbhandler

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type getRecordTopicsResponse struct {
	Status   bool                  `json:"status"`
	RecordID int64                 `json:"recordId"`
	Topics   []topicCategoryRecord `json:"topics"`
}

type parsedTopicEntry struct {
	topicID           string
	topicType         string
	topicText         string
	topicDescEn       string
	keywords          []string
	keywordsEn        []string
	lines             []string
	categoryPaths     []string
	categoryPathsEn   []string
}

func GetRecordTopics(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_TREC_001")
	defer rc.Close()
	logger := rc.GetLogger()

	recordID, err := parseOptionalPositiveInt64(c.QueryParam("record_id"))
	if err != nil || recordID == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing or invalid record_id (CWB_KB_TREC_010)",
		})
	}

	logger.Info("get record topics request", "record_id", *recordID)

	results, err := readRecordTopicCards(logger, *recordID)
	if err != nil {
		logger.Error("read record topics failed", "record_id", *recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read record topics (CWB_KB_TREC_011)",
		})
	}

	logger.Info("get record topics result", "record_id", *recordID, "count", len(results))

	return c.JSON(http.StatusOK, getRecordTopicsResponse{
		Status:   true,
		RecordID: *recordID,
		Topics:   results,
	})
}

func readRecordTopicCards(logger ApiTypes.JimoLogger, recordID int64) ([]topicCategoryRecord, error) {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return nil, fmt.Errorf("missing ARTIFACT_DIR")
	}

	recordDir, err := resolveRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return nil, err
	}

	entries, err := filepath.Glob(filepath.Join(recordDir, "*.topics"))
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return []topicCategoryRecord{}, nil
	}
	sort.Strings(entries)

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return nil, err
	}
	stagingExpr, err := resolveStagingOrNameExpr(db, inputTable)
	if err != nil {
		return nil, err
	}
	parserExpr, err := resolveParserNameExpr(db, inputTable)
	if err != nil {
		return nil, err
	}
	meta, err := fetchSummaryArtifactMeta(db, inputTable, stagingExpr, parserExpr, recordID)
	if err != nil {
		return nil, err
	}
	lineTargets, err := readLineTargetMapForRecord(artifactDir, meta)
	if err != nil {
		return nil, err
	}

	results := make([]topicCategoryRecord, 0)
	for _, path := range entries {
		topics, err := parseTopicsFile(path)
		if err != nil {
			if logger != nil {
				logger.Warn("failed to parse topics file", "path", path, "err", err)
			}
			continue
		}
		for _, topic := range topics {
			targets := expandSummaryTargets(topic.lines, lineTargets)
			page, coords := firstSummaryTarget(targets)
			topicID := topic.topicID
			if topicID == "" {
				topicID = fmt.Sprintf("%d_%s", recordID, filepath.Base(path))
			}
			results = append(results, topicCategoryRecord{
				ID:              topicID,
				PdfFileName:     filepath.Base(strings.TrimSpace(meta.fileName)),
				TopicType:       topic.topicType,
				TopicText:       topic.topicText,
				TopicDescEn:     topic.topicDescEn,
				Keywords:        append([]string(nil), topic.keywords...),
				KeywordsEn:      append([]string(nil), topic.keywordsEn...),
				CategoryPaths:   append([]string(nil), topic.categoryPaths...),
				CategoryPathsEn: append([]string(nil), topic.categoryPathsEn...),
				SourceLineSpecs: append([]string(nil), topic.lines...),
				InputID:         recordID,
				Page:            page,
				Coords:          coords,
				Targets:         targets,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}

// parseTopicsFile reads a .topics artifact file and returns all topic entries.
// Topic file format:
//
//	topic_id: ddd,
//	topic_type: "type"
//	lines: [ddd, ddd-ddd, ...]
//	topic_keywords: ["k1", ...]
//	topic: "text"
//
//	<next topic>
func parseTopicsFile(path string) ([]parsedTopicEntry, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entries []parsedTopicEntry
	var current *parsedTopicEntry

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 1024), 8*1024*1024)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			if current != nil {
				entries = append(entries, *current)
				current = nil
			}
			continue
		}

		if current == nil {
			current = &parsedTopicEntry{keywords: []string{}, lines: []string{}}
		}

		switch {
		case strings.HasPrefix(trimmed, "topic_id:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_id:"))
			raw = strings.TrimSuffix(raw, ",")
			current.topicID = strings.Trim(raw, `"`)
		case strings.HasPrefix(trimmed, "topic_type:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_type:"))
			current.topicType = strings.Trim(raw, `"`)
		case strings.HasPrefix(trimmed, "lines:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "lines:"))
			current.lines = parseQuotedStringArray(raw)
		case strings.HasPrefix(trimmed, "topic_keywords_en:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_keywords_en:"))
			current.keywordsEn = parseQuotedStringArray(raw)
		case strings.HasPrefix(trimmed, "topic_keywords:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_keywords:"))
			current.keywords = parseQuotedStringArray(raw)
		case strings.HasPrefix(trimmed, "category_paths_en:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths_en:"))
			current.categoryPathsEn = parseTopicCategoryPaths(raw)
		case strings.HasPrefix(trimmed, "category_paths:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths:"))
			current.categoryPaths = parseTopicCategoryPaths(raw)
		case strings.HasPrefix(trimmed, "topic_desc_en:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_desc_en:"))
			current.topicDescEn = strings.Trim(raw, `"`)
		case strings.HasPrefix(trimmed, "topic_desc:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_desc:"))
			current.topicText = strings.Trim(raw, `"`)
		case strings.HasPrefix(trimmed, "topic:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic:"))
			if current.topicText == "" {
				current.topicText = strings.Trim(raw, `"`)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		entries = append(entries, *current)
	}
	return entries, nil
}

// readTopicFromArtifact finds and reads a specific topic by sequence number from the record's .topics files.
func readTopicFromArtifact(artifactDir string, recordID int64, meta summaryArtifactMeta, topicSeqNo int) (parsedTopicEntry, error) {
	recordDir, err := resolveRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return parsedTopicEntry{}, err
	}

	candidates := listTopicArtifactCandidates(recordDir, meta)

	for _, path := range candidates {
		topics, err := parseTopicsFile(path)
		if err != nil {
			continue
		}
		for _, topic := range topics {
			seqNo, err := strconv.Atoi(topic.topicID)
			if err != nil {
				continue
			}
			if seqNo == topicSeqNo {
				return topic, nil
			}
		}
		// Also try 1-based index position as fallback
		if topicSeqNo >= 1 && topicSeqNo <= len(topics) {
			return topics[topicSeqNo-1], nil
		}
	}
	return parsedTopicEntry{}, fmt.Errorf("topic %d not found for record %d", topicSeqNo, recordID)
}

func parseTopicCategoryPaths(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []string{}
	}
	if strings.HasPrefix(raw, "[(") {
		raw = strings.TrimPrefix(raw, "[")
		raw = strings.TrimSuffix(raw, "]")
		if strings.TrimSpace(raw) == "" {
			return []string{}
		}

		entries := splitTopLevelDelimited(raw, ',')
		out := make([]string, 0, len(entries))
		for _, entry := range entries {
			names := parseStructuredCategoryPathEntry(entry)
			if len(names) == 0 {
				continue
			}
			out = append(out, strings.Join(names, "/"))
		}
		return out
	}

	if parsed := parseQuotedStringArray(raw); len(parsed) > 0 {
		return parsed
	}
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	return []string{}
}

func parseStructuredCategoryPathEntry(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "(")
	raw = strings.TrimSuffix(raw, ")")
	parts := splitTopLevelDelimited(raw, ',')
	if len(parts) < 3 {
		return nil
	}
	nodeList := strings.TrimSpace(parts[2])
	nodeList = strings.TrimPrefix(nodeList, "[")
	nodeList = strings.TrimSuffix(nodeList, "]")
	if strings.TrimSpace(nodeList) == "" {
		return nil
	}

	nodes := splitTopLevelDelimited(nodeList, ',')
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		node = strings.TrimSpace(node)
		node = strings.TrimPrefix(node, "(")
		node = strings.TrimSuffix(node, ")")
		nodeParts := splitTopLevelDelimited(node, ',')
		if len(nodeParts) == 0 {
			continue
		}
		name := strings.TrimSpace(strings.Trim(nodeParts[0], `"`))
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func splitTopLevelDelimited(raw string, separator rune) []string {
	parts := make([]string, 0)
	var current strings.Builder
	bracketDepth := 0
	parenDepth := 0
	inString := false
	escaped := false

	flush := func() {
		part := strings.TrimSpace(current.String())
		if part != "" {
			parts = append(parts, part)
		}
		current.Reset()
	}

	for _, r := range raw {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && inString:
			current.WriteRune(r)
			escaped = true
		case r == '"':
			current.WriteRune(r)
			inString = !inString
		case !inString && r == '[':
			bracketDepth++
			current.WriteRune(r)
		case !inString && r == ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
			current.WriteRune(r)
		case !inString && r == '(':
			parenDepth++
			current.WriteRune(r)
		case !inString && r == ')':
			if parenDepth > 0 {
				parenDepth--
			}
			current.WriteRune(r)
		case !inString && bracketDepth == 0 && parenDepth == 0 && r == separator:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return parts
}

func listTopicArtifactCandidates(recordDir string, meta summaryArtifactMeta) []string {
	stagingBase := filepath.Base(meta.staging)
	stagingRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	candidates := make([]string, 0)
	if stagingRoot != "" && meta.parser != "" {
		named := filepath.Join(recordDir, stagingRoot+"_"+meta.parser+".topics")
		if _, err := os.Stat(named); err == nil {
			candidates = append(candidates, named)
		}
	}
	glob, err := filepath.Glob(filepath.Join(recordDir, "*.topics"))
	if err == nil {
		for _, g := range glob {
			alreadyAdded := false
			for _, c := range candidates {
				if c == g {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				candidates = append(candidates, g)
			}
		}
	}
	return candidates
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
