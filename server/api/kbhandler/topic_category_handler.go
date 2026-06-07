package kbhandler

import (
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

type topicCategoryRecord struct {
	ID                string                `json:"id"`
	TopicName         string                `json:"topicName,omitempty"`
	PdfFileName       string                `json:"pdfFileName"`
	TopicType         string                `json:"topicType"`
	TopicText         string                `json:"topicText"`
	TopicDescEn       string                `json:"topicDescEn,omitempty"`
	Keywords          []string              `json:"topicKeywords"`
	KeywordsEn        []string              `json:"topicKeywordsEn,omitempty"`
	CategoryPaths     []string              `json:"categoryPaths,omitempty"`
	CategoryPathsEn   []string              `json:"categoryPathsEn,omitempty"`
	SourceLineSpecs   []string              `json:"sourceLineSpecs,omitempty"`
	InputID           int64                 `json:"inputId"`
	Page              int                   `json:"page"`
	Coords            []float64             `json:"coords"`
	Targets           []summaryRecordTarget `json:"targets"`
}

type getTopicCategoryResponse struct {
	Status       bool                  `json:"status"`
	CategoryPath string                `json:"categoryPath"`
	Topics       []topicCategoryRecord `json:"topics"`
}

type topicCategoryReference struct {
	RecordID  int64
	TopicID   string
	TopicText string
	Lines     []string
	Ordinal   int
}

func GetTopicCategory(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_TCAT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	topicTreeDir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if topicTreeDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing ARTIFACT_WEB_DIR (CWB_KB_TCAT_010)",
		})
	}

	categoryPath := strings.TrimSpace(c.QueryParam("category_path"))
	if categoryPath == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing category_path (CWB_KB_TCAT_011)",
		})
	}

	results, err := readTopicCategoryRecords(topicTreeDir, categoryPath)
	if err != nil {
		logger.Error("read topic category failed", "category_path", categoryPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read topic category (CWB_KB_TCAT_012)",
		})
	}

	return c.JSON(http.StatusOK, getTopicCategoryResponse{
		Status:       true,
		CategoryPath: categoryPath,
		Topics:       results,
	})
}

func readTopicCategoryRecords(topicTreeDir string, categoryPath string) ([]topicCategoryRecord, error) {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return nil, fmt.Errorf("missing ARTIFACT_DIR")
	}

	topicRefs, err := readTopicRefsForCategory(topicTreeDir, categoryPath)
	if err != nil {
		return nil, err
	}
	if len(topicRefs) == 0 {
		return []topicCategoryRecord{}, nil
	}

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

	metaCache := map[int64]summaryArtifactMeta{}
	lineTargetCache := map[int64]map[int]summaryLineTarget{}
	results := make([]topicCategoryRecord, 0, len(topicRefs))

	for _, topicRef := range topicRefs {
		if topicRef.RecordID <= 0 {
			continue
		}

		recordID := topicRef.RecordID
		meta, ok := metaCache[recordID]
		if !ok {
			var err error
			meta, err = fetchSummaryArtifactMeta(db, inputTable, stagingExpr, parserExpr, recordID)
			if err != nil {
				return nil, err
			}
			metaCache[recordID] = meta
		}

		lineTargets, ok := lineTargetCache[recordID]
		if !ok {
			var err error
			lineTargets, err = readLineTargetMapForRecord(artifactDir, meta)
			if err != nil {
				return nil, err
			}
			lineTargetCache[recordID] = lineTargets
		}

		parsed, err := resolveTopicCategoryReference(artifactDir, recordID, meta, topicRef)
		if err != nil {
			continue
		}

		targets := expandSummaryTargets(parsed.lines, lineTargets)
		page, coords := firstSummaryTarget(targets)
		resultTopicID := buildTopicCategoryRecordID(recordID, parsed.topicID, topicRef.Ordinal)

		results = append(results, topicCategoryRecord{
			ID:              resultTopicID,
			PdfFileName:     filepath.Base(strings.TrimSpace(meta.fileName)),
			TopicType:       parsed.topicType,
			TopicText:       parsed.topicText,
			Keywords:        append([]string(nil), parsed.keywords...),
			KeywordsEn:      append([]string(nil), parsed.keywordsEn...),
			CategoryPaths:   append([]string(nil), parsed.categoryPaths...),
			CategoryPathsEn: append([]string(nil), parsed.categoryPathsEn...),
			SourceLineSpecs: append([]string(nil), parsed.lines...),
			InputID:         recordID,
			Page:            page,
			Coords:          coords,
			Targets:         targets,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}

func resolveTopicCategoryReference(
	artifactDir string,
	recordID int64,
	meta summaryArtifactMeta,
	topicRef topicCategoryReference,
) (parsedTopicEntry, error) {
	if topicRef.TopicID != "" {
		if topicSeqNo, err := parseInt(topicRef.TopicID); err == nil && topicSeqNo > 0 {
			return readTopicFromArtifact(artifactDir, recordID, meta, topicSeqNo)
		}
	}

	recordDir, err := resolveRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return parsedTopicEntry{}, err
	}

	candidates := listTopicArtifactCandidates(recordDir, meta)
	normalizedText := strings.TrimSpace(topicRef.TopicText)
	for _, path := range candidates {
		topics, err := parseTopicsFile(path)
		if err != nil {
			continue
		}

		if match, ok := findBestMatchingTopic(topics, normalizedText, topicRef.Lines); ok {
			return match, nil
		}

		if topicRef.Ordinal >= 1 && topicRef.Ordinal <= len(topics) {
			return topics[topicRef.Ordinal-1], nil
		}
	}

	return parsedTopicEntry{}, fmt.Errorf("topic reference not found for record %d", recordID)
}

func findBestMatchingTopic(topics []parsedTopicEntry, topicText string, lines []string) (parsedTopicEntry, bool) {
	bestScore := 0
	var best parsedTopicEntry
	for _, topic := range topics {
		score := 0
		if topicText != "" && strings.TrimSpace(topic.topicText) == topicText {
			score += 3
		}
		if sameTopicLines(topic.lines, lines) {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			best = topic
		}
	}
	return best, bestScore > 0
}

func sameTopicLines(left []string, right []string) bool {
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return false
	}
	for index := range left {
		if strings.TrimSpace(left[index]) != strings.TrimSpace(right[index]) {
			return false
		}
	}
	return true
}

func buildTopicCategoryRecordID(recordID int64, topicID string, ordinal int) string {
	topicID = strings.TrimSpace(topicID)
	if topicID != "" {
		return topicID
	}
	return fmt.Sprintf("%d_tpc_%d", recordID, ordinal)
}

func readTopicIDsForCategory(topicTreeDir string, categoryPath string) ([]string, error) {
	refs, err := readTopicRefsForCategory(topicTreeDir, categoryPath)
	if err != nil {
		return nil, err
	}
	rows := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.RecordID <= 0 {
			continue
		}
		topicID := strings.TrimSpace(ref.TopicID)
		if topicID == "" {
			topicID = fmt.Sprintf("%d", ref.Ordinal)
		}
		rows = append(rows, fmt.Sprintf("%d_%s", ref.RecordID, topicID))
	}
	sort.Strings(rows)
	return rows, nil
}

func readTopicRefsForCategory(topicTreeDir string, categoryPath string) ([]topicCategoryReference, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(categoryPath))
	if cleanPath == "." || cleanPath == "" {
		return nil, fmt.Errorf("invalid category path")
	}
	if strings.HasPrefix(cleanPath, "..") {
		return nil, fmt.Errorf("invalid category path")
	}
	path := filepath.Join(topicTreeDir, filepath.FromSlash(cleanPath), "topics.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []topicCategoryReference{}, nil
		}
		return nil, err
	}
	return parseTopicCategoryRefs(body), nil
}

// parseTopicIDParts parses "<record_id>_<topic_seq_no>" from a topic ID string.
func parseTopicIDParts(topicID string) (recordID int64, topicSeqNo int, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(topicID), "_", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	var err error
	recordID, err = parseInt64(parts[0])
	if err != nil || recordID <= 0 {
		return 0, 0, false
	}
	topicSeqNo, err = parseInt(parts[1])
	if err != nil || topicSeqNo <= 0 {
		return 0, 0, false
	}
	return recordID, topicSeqNo, true
}

func parseTopicCategoryIDs(body []byte) []string {
	refs := parseTopicCategoryRefs(body)
	rows := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.RecordID <= 0 {
			continue
		}
		topicID := strings.TrimSpace(ref.TopicID)
		if topicID == "" {
			topicID = fmt.Sprintf("%d", ref.Ordinal)
		}
		rows = append(rows, fmt.Sprintf("%d_%s", ref.RecordID, topicID))
	}
	sort.Strings(rows)
	return rows
}

func parseTopicCategoryRefs(body []byte) []topicCategoryReference {
	type topicBlock struct {
		recordID  int64
		topicID   string
		topicText string
		lines     []string
	}

	flush := func(
		block *topicBlock,
		out *[]topicCategoryReference,
		seqByRecord map[int64]int,
	) {
		if block == nil || block.recordID <= 0 {
			return
		}
		seqNo := seqByRecord[block.recordID]
		if strings.TrimSpace(block.topicID) == "" {
			seqByRecord[block.recordID]++
			seqNo = seqByRecord[block.recordID]
		} else if parsedTopicID, err := parseInt(block.topicID); err == nil && parsedTopicID > seqByRecord[block.recordID] {
			seqByRecord[block.recordID] = parsedTopicID
			seqNo = seqByRecord[block.recordID]
		} else if seqNo == 0 {
			seqByRecord[block.recordID]++
			seqNo = seqByRecord[block.recordID]
		}
		*out = append(*out, topicCategoryReference{
			RecordID:  block.recordID,
			TopicID:   strings.TrimSpace(block.topicID),
			TopicText: strings.TrimSpace(block.topicText),
			Lines:     append([]string(nil), block.lines...),
			Ordinal:   seqNo,
		})
	}

	rows := make([]topicCategoryReference, 0)
	seqByRecord := map[int64]int{}
	var current *topicBlock

	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(strings.TrimRight(rawLine, "\r"))
		if line == "" {
			flush(current, &rows, seqByRecord)
			current = nil
			continue
		}

		if !strings.Contains(line, ":") {
			flush(current, &rows, seqByRecord)
			current = nil
			legacyID := strings.TrimSuffix(strings.TrimSpace(line), ",")
			recordID, topicSeqNo, ok := parseTopicIDParts(legacyID)
			if !ok {
				continue
			}
			rows = append(rows, topicCategoryReference{
				RecordID: recordID,
				TopicID:  fmt.Sprintf("%d", topicSeqNo),
				Ordinal:  topicSeqNo,
			})
			continue
		}

		if current == nil {
			current = &topicBlock{}
		}

		switch {
		case strings.HasPrefix(line, "record_id:"):
			raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "record_id:"), ","))
			if recordID, err := parseInt64(raw); err == nil {
				current.recordID = recordID
			}
		case strings.HasPrefix(line, "topic_id:"):
			raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "topic_id:"), ","))
			current.topicID = strings.Trim(raw, `"`)
		case strings.HasPrefix(line, "lines:"):
			raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "lines:"), ","))
			current.lines = parseQuotedStringArray(raw)
		case strings.HasPrefix(line, "topic_desc:"):
			raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "topic_desc:"), ","))
			current.topicText = strings.Trim(raw, `"`)
		case strings.HasPrefix(line, "topic:"):
			raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "topic:"), ","))
			current.topicText = strings.Trim(raw, `"`)
		}
	}

	flush(current, &rows, seqByRecord)
	return rows
}
