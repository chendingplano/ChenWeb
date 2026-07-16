package kbhandler

import (
	"os"
	"path/filepath"
	"testing"
)

func withKnowledgeMenuLabelsDir(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	t.Setenv("KNOWLEDGE_MENU_LABELS_DIR", dir)
}

func TestLoadKnowledgeMenuLabelsMissingFileReturnsEmptyMap(t *testing.T) {
	withKnowledgeMenuLabelsDir(t, nil)

	got, err := LoadKnowledgeMenuLabels("en")
	if err != nil {
		t.Fatalf("LoadKnowledgeMenuLabels: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestLoadKnowledgeMenuLabelsReadsLabelsTable(t *testing.T) {
	withKnowledgeMenuLabelsDir(t, map[string]string{
		"labels-zh-cn.toml": `
[labels]
kb-metrics = "指标"
kb-doc-wiki = "知识百科"
`,
	})

	got, err := LoadKnowledgeMenuLabels("zh-cn")
	if err != nil {
		t.Fatalf("LoadKnowledgeMenuLabels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 labels, got %d (%v)", len(got), got)
	}
	if got["kb-metrics"] != "指标" {
		t.Fatalf("kb-metrics = %q", got["kb-metrics"])
	}
	if got["kb-doc-wiki"] != "知识百科" {
		t.Fatalf("kb-doc-wiki = %q", got["kb-doc-wiki"])
	}
}

func TestLoadKnowledgeMenuLabelsMalformedTomlReturnsError(t *testing.T) {
	withKnowledgeMenuLabelsDir(t, map[string]string{
		"labels-en.toml": `this is not valid toml [[[`,
	})

	_, err := LoadKnowledgeMenuLabels("en")
	if err == nil {
		t.Fatal("expected an error for malformed TOML, got nil")
	}
}

func TestLoadKnowledgeMenuLabelsRejectsUnrecognizedLangShape(t *testing.T) {
	withKnowledgeMenuLabelsDir(t, map[string]string{
		"labels-..%2f..%2fetc.toml": `[labels]
x = "y"`,
	})

	got, err := LoadKnowledgeMenuLabels("../../etc/passwd")
	if err != nil {
		t.Fatalf("LoadKnowledgeMenuLabels: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map for invalid lang shape, got %v", got)
	}
}
