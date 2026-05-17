package kbhandler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type topicGraphMetadata struct {
	Desc       string   `json:"desc"`
	Confidence float64  `json:"confidence"`
	Keywords   []string `json:"keywords"`
	CreateTime string   `json:"create_time"`
}

type topicGraphNode struct {
	ID            string             `json:"id"`
	CategoryPath  string             `json:"categoryPath"`
	Label         string             `json:"label"`
	Metadata      topicGraphMetadata `json:"metadata"`
	ChildIDs      []string           `json:"childIds"`
	TopicIDs      []string           `json:"topicIds"`
	HasTopicsFile bool               `json:"hasTopicsFile"`
	Expanded      bool               `json:"expanded"`
}

type listTopicGraphResponse struct {
	Status  bool             `json:"status"`
	Results []topicGraphNode `json:"results"`
}

func ListTopicGraph(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_TG_001")
	defer rc.Close()
	logger := rc.GetLogger()

	topicTreeDir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if topicTreeDir == "" {
		logger.Error("missing ARTIFACT_WEB_DIR")
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing ARTIFACT_WEB_DIR (CWB_KB_TG_010)",
		})
	}

	results, err := readTopicGraphNodes(topicTreeDir)
	if err != nil {
		logger.Error("read topic graph failed", "dir", topicTreeDir, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read topic graph (CWB_KB_TG_011)",
		})
	}

	return c.JSON(http.StatusOK, listTopicGraphResponse{
		Status:  true,
		Results: results,
	})
}

func readTopicGraphNodes(topicTreeDir string) ([]topicGraphNode, error) {
	topicTreeDir = strings.TrimSpace(topicTreeDir)
	if topicTreeDir == "" {
		return nil, os.ErrNotExist
	}

	nodes := make([]topicGraphNode, 0)
	err := filepath.WalkDir(topicTreeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || path == topicTreeDir {
			return nil
		}

		rel, err := filepath.Rel(topicTreeDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		topicIDs, hasTopicsFile := readTopicGraphTopicIDs(path)
		node := topicGraphNode{
			ID:            rel,
			CategoryPath:  rel,
			Label:         filepath.Base(path),
			Metadata:      readTopicGraphMetadata(path),
			ChildIDs:      readTopicGraphChildIDs(topicTreeDir, path),
			TopicIDs:      topicIDs,
			HasTopicsFile: hasTopicsFile,
			Expanded:      false,
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

func readTopicGraphChildIDs(topicTreeDir string, dir string) []string {
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
		rel, err := filepath.Rel(topicTreeDir, childPath)
		if err != nil {
			continue
		}
		childIDs = append(childIDs, filepath.ToSlash(rel))
	}
	sort.Strings(childIDs)
	return childIDs
}

func readTopicGraphTopicIDs(dir string) ([]string, bool) {
	path := filepath.Join(dir, "topics.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		return []string{}, false
	}
	return parseTopicCategoryIDs(body), true
}

func readTopicGraphMetadata(dir string) topicGraphMetadata {
	path := filepath.Join(dir, "metadata.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		return topicGraphMetadata{Keywords: []string{}}
	}
	parsed, ok := parseTopicGraphMetadata(body)
	if ok {
		return parsed
	}
	return topicGraphMetadata{Keywords: []string{}}
}

func parseTopicGraphMetadata(body []byte) (topicGraphMetadata, bool) {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return topicGraphMetadata{}, false
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
		return topicGraphMetadata{}, false
	}

	wrapped := "{" + strings.Join(cleaned, ",") + "}"
	var meta topicGraphMetadata
	if err := json.Unmarshal([]byte(wrapped), &meta); err != nil {
		return topicGraphMetadata{}, false
	}
	if meta.Keywords == nil {
		meta.Keywords = []string{}
	} else {
		meta.Keywords = append([]string(nil), meta.Keywords...)
	}
	return meta, true
}
