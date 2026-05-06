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
	ID          string                `json:"id"`
	TopicName   string                `json:"topicName,omitempty"`
	PdfFileName string                `json:"pdfFileName"`
	TopicType   string                `json:"topicType"`
	TopicText   string                `json:"topicText"`
	Keywords    []string              `json:"topicKeywords"`
	InputID     int64                 `json:"inputId"`
	Page        int                   `json:"page"`
	Coords      []float64             `json:"coords"`
	Targets     []summaryRecordTarget `json:"targets"`
}

type getTopicCategoryResponse struct {
	Status       bool                  `json:"status"`
	CategoryPath string                `json:"categoryPath"`
	Topics       []topicCategoryRecord `json:"topics"`
}

func GetTopicCategory(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_TCAT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	topicTreeDir := strings.TrimSpace(os.Getenv("TOPIC_TREE_ROOT_DIR"))
	if topicTreeDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing TOPIC_TREE_ROOT_DIR (CWB_KB_TCAT_010)",
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

	topicIDs, err := readTopicIDsForCategory(topicTreeDir, categoryPath)
	if err != nil {
		return nil, err
	}
	if len(topicIDs) == 0 {
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
	results := make([]topicCategoryRecord, 0, len(topicIDs))

	for _, topicID := range topicIDs {
		recordID, topicSeqNo, ok := parseTopicIDParts(topicID)
		if !ok {
			continue
		}

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

		parsed, err := readTopicFromArtifact(artifactDir, recordID, meta, topicSeqNo)
		if err != nil {
			continue
		}

		targets := expandSummaryTargets(parsed.lines, lineTargets)
		page, coords := firstSummaryTarget(targets)

		results = append(results, topicCategoryRecord{
			ID:          topicID,
			PdfFileName: filepath.Base(strings.TrimSpace(meta.fileName)),
			TopicType:   parsed.topicType,
			TopicText:   parsed.topicText,
			Keywords:    append([]string(nil), parsed.keywords...),
			InputID:     recordID,
			Page:        page,
			Coords:      coords,
			Targets:     targets,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}

func readTopicIDsForCategory(topicTreeDir string, categoryPath string) ([]string, error) {
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
			return []string{}, nil
		}
		return nil, err
	}
	rows := make([]string, 0)
	for _, row := range strings.Split(string(body), "\n") {
		row = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(row), ","))
		if row == "" {
			continue
		}
		rows = append(rows, row)
	}
	sort.Strings(rows)
	return rows, nil
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
