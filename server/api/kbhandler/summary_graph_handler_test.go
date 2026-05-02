package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestReadSummaryGraphNodes(t *testing.T) {
	root := t.TempDir()

	mustWriteFile(t, filepath.Join(root, "finance", "desc.txt"), "\"desc\":\"Finance root\",\n\"category_type\":\"domain\",\n\"confidence\":0.92,\n\"keywords\":[\"finance\",\"tax\"],\n\"create_time\":\"20260501-120000\",\n")
	mustWriteFile(t, filepath.Join(root, "finance", "summaries.txt"), "101_1_0001\n101_2_0001\n")
	mustWriteFile(t, filepath.Join(root, "finance", "tax", "metadata.txt"), "\"desc\":\"Tax summaries\",\n\"category_type\":\"topic\",\n\"confidence\":0.88,\n\"keywords\":[\"tax\",\"filing\"],\n")
	mustWriteFile(t, filepath.Join(root, "finance", "tax", "summaries.txt"), "101_2_0001\n")
	mustWriteFile(t, filepath.Join(root, "research", "summaries.txt"), "205_1_0001\n")

	nodes, err := readSummaryGraphNodes(root)
	if err != nil {
		t.Fatalf("readSummaryGraphNodes: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	byPath := make(map[string]summaryGraphNode)
	for _, node := range nodes {
		byPath[node.CategoryPath] = node
	}

	finance := byPath["finance"]
	if finance.Label != "finance" {
		t.Fatalf("unexpected finance label: %+v", finance)
	}
	if finance.Metadata.Desc != "Finance root" {
		t.Fatalf("unexpected finance metadata: %+v", finance.Metadata)
	}
	if len(finance.ChildIDs) != 1 || finance.ChildIDs[0] != "finance/tax" {
		t.Fatalf("unexpected finance child ids: %+v", finance.ChildIDs)
	}
	if len(finance.SummaryIDs) != 2 {
		t.Fatalf("unexpected finance summary ids: %+v", finance.SummaryIDs)
	}

	tax := byPath["finance/tax"]
	if tax.Metadata.CategoryType != "topic" {
		t.Fatalf("unexpected tax metadata: %+v", tax.Metadata)
	}
	if len(tax.SummaryIDs) != 1 || tax.SummaryIDs[0] != "101_2_0001" {
		t.Fatalf("unexpected tax summaries: %+v", tax.SummaryIDs)
	}

	research := byPath["research"]
	if research.Metadata.Desc != "" {
		t.Fatalf("expected empty metadata desc for research, got %+v", research.Metadata)
	}
	if research.Metadata.Keywords == nil {
		t.Fatalf("expected research keywords to be an empty slice, got nil")
	}
}

func TestListSummaryGraphSuccess(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "finance", "desc.txt"), "\"desc\":\"Finance root\",\n\"category_type\":\"domain\",\n\"confidence\":0.92,\n")
	mustWriteFile(t, filepath.Join(root, "finance", "summaries.txt"), "101_1_0001\n")
	mustWriteFile(t, filepath.Join(root, "finance", "tax", "summaries.txt"), "101_2_0001\n")

	t.Setenv("SUMMARY_TREE_DIR", root)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/summary-graph", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListSummaryGraph(c); err != nil {
		t.Fatalf("ListSummaryGraph returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status  bool               `json:"status"`
		Results []summaryGraphNode `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true, got %+v", payload)
	}
	if len(payload.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(payload.Results))
	}
}

func mustWriteFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
