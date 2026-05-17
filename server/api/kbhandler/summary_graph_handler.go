package kbhandler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type summaryGraphMetadata struct {
	Desc         string   `json:"desc"`
	CategoryType string   `json:"category_type"`
	Confidence   float64  `json:"confidence"`
	Keywords     []string `json:"keywords"`
	CreateTime   string   `json:"create_time"`
}

type summaryGraphNode struct {
	ID               string               `json:"id"`
	CategoryPath     string               `json:"categoryPath"`
	Label            string               `json:"label"`
	Metadata         summaryGraphMetadata `json:"metadata"`
	ChildIDs         []string             `json:"childIds"`
	SummaryIDs       []string             `json:"summaryIds"`
	HasSummariesFile bool                 `json:"hasSummariesFile"`
	Expanded         bool                 `json:"expanded"`
}

type listSummaryGraphResponse struct {
	Status  bool               `json:"status"`
	Results []summaryGraphNode `json:"results"`
}

func ListSummaryGraph(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SG_001")
	defer rc.Close()
	logger := rc.GetLogger()

	summaryTreeDir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if summaryTreeDir == "" {
		logger.Error("missing ARTIFACT_WEB_DIR")
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing ARTIFACT_WEB_DIR (CWB_KB_SG_010)",
		})
	}

	results, err := readSummaryGraphNodes(summaryTreeDir)
	if err != nil {
		logger.Error("read summary graph failed", "dir", summaryTreeDir, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read summary graph (CWB_KB_SG_011)",
		})
	}

	return c.JSON(http.StatusOK, listSummaryGraphResponse{
		Status:  true,
		Results: results,
	})
}

func readSummaryGraphNodes(summaryTreeDir string) ([]summaryGraphNode, error) {
	summaryTreeDir = strings.TrimSpace(summaryTreeDir)
	if summaryTreeDir == "" {
		return nil, os.ErrNotExist
	}

	nodes := make([]summaryGraphNode, 0)
	err := filepath.WalkDir(summaryTreeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || path == summaryTreeDir {
			return nil
		}

		rel, err := filepath.Rel(summaryTreeDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		summaryIDs, hasSummariesFile := readSummaryGraphSummaryIDs(path)
		node := summaryGraphNode{
			ID:               rel,
			CategoryPath:     rel,
			Label:            filepath.Base(path),
			Metadata:         readSummaryGraphMetadata(path),
			ChildIDs:         readSummaryGraphChildIDs(summaryTreeDir, path),
			SummaryIDs:       summaryIDs,
			HasSummariesFile: hasSummariesFile,
			Expanded:         false,
		}
		nodes = append(nodes, node)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].CategoryPath < nodes[j].CategoryPath
	})
	return nodes, nil
}

func readSummaryGraphChildIDs(summaryTreeDir string, dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	childIDs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		childPath := filepath.Join(dir, entry.Name())
		rel, err := filepath.Rel(summaryTreeDir, childPath)
		if err != nil {
			continue
		}
		childIDs = append(childIDs, filepath.ToSlash(rel))
	}
	sort.Strings(childIDs)
	return childIDs
}

func readSummaryGraphSummaryIDs(dir string) ([]string, bool) {
	path := filepath.Join(dir, "summaries.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		return []string{}, false
	}
	rows := make([]string, 0)
	for _, row := range strings.Split(string(body), "\n") {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		rows = append(rows, row)
	}
	sort.Strings(rows)
	return rows, true
}

func readSummaryGraphMetadata(dir string) summaryGraphMetadata {
	for _, name := range []string{"desc.txt", "metadata.txt"} {
		path := filepath.Join(dir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, ok := parseSummaryGraphMetadata(body)
		if ok {
			return parsed
		}
	}
	return summaryGraphMetadata{Keywords: []string{}}
}

func parseSummaryGraphMetadata(body []byte) (summaryGraphMetadata, bool) {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return summaryGraphMetadata{}, false
	}

	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimSuffix(line, ",")
		cleaned = append(cleaned, line)
	}
	if len(cleaned) == 0 {
		return summaryGraphMetadata{}, false
	}

	wrapped := "{" + strings.Join(cleaned, ",") + "}"
	var meta summaryGraphMetadata
	if err := json.Unmarshal([]byte(wrapped), &meta); err != nil {
		return summaryGraphMetadata{}, false
	}
	if meta.Keywords == nil {
		meta.Keywords = []string{}
	} else {
		meta.Keywords = append([]string(nil), meta.Keywords...)
	}
	return meta, true
}

func parseSummaryIDParts(summaryID string) (recordID int64, level int, seqNo int, ok bool) {
	parts := strings.Split(strings.TrimSpace(summaryID), "_")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	recordID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || recordID <= 0 {
		return 0, 0, 0, false
	}
	level, err = strconv.Atoi(parts[1])
	if err != nil || level < 0 {
		return 0, 0, 0, false
	}
	seqNo, err = strconv.Atoi(parts[2])
	if err != nil || seqNo <= 0 {
		return 0, 0, 0, false
	}
	return recordID, level, seqNo, true
}
