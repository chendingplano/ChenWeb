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
	topicID       string
	topicType     string
	topicText     string
	keywords      []string
	lines         []string
	categoryPaths []string
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
				Keywords:        append([]string(nil), topic.keywords...),
				CategoryPaths:   append([]string(nil), topic.categoryPaths...),
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
		case strings.HasPrefix(trimmed, "topic_keywords:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_keywords:"))
			current.keywords = parseQuotedStringArray(raw)
		case strings.HasPrefix(trimmed, "category_paths:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths:"))
			current.categoryPaths = parseQuotedStringArray(raw)
		case strings.HasPrefix(trimmed, "topic:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "topic:"))
			current.topicText = strings.Trim(raw, `"`)
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

	// Try to find a matching topics file; prefer the one with a staging+parser name
	stagingBase := filepath.Base(meta.staging)
	stagingRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	var candidates []string
	if stagingRoot != "" && meta.parser != "" {
		named := filepath.Join(recordDir, stagingRoot+"_"+meta.parser+".topics")
		if _, err := os.Stat(named); err == nil {
			candidates = append(candidates, named)
		}
	}
	// Fall back to any .topics file
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

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
