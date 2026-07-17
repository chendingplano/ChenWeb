package sitehandler

import (
	"os"
	"path/filepath"
	"testing"
)

func withWorkspaceContentLabelsDir(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	t.Setenv("WORKSPACE_CONTENT_LABELS_DIR", dir)
}

func TestLoadWorkspaceContentOverridesMissingFileReturnsEmptyMaps(t *testing.T) {
	withWorkspaceContentLabelsDir(t, nil)

	labels, descriptions, err := LoadWorkspaceContentOverrides("en")
	if err != nil {
		t.Fatalf("LoadWorkspaceContentOverrides: %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("expected empty labels map, got %v", labels)
	}
	if len(descriptions) != 0 {
		t.Fatalf("expected empty descriptions map, got %v", descriptions)
	}
}

func TestLoadWorkspaceContentOverridesReadsLabelsAndDescriptionsTables(t *testing.T) {
	withWorkspaceContentLabelsDir(t, map[string]string{
		"labels-zh-cn.toml": `
[labels]
ws-kicker = "工作台"
knowledge_base = "知识库"

[descriptions]
knowledge_base = "浏览和管理文档与知识工件。"
`,
	})

	labels, descriptions, err := LoadWorkspaceContentOverrides("zh-cn")
	if err != nil {
		t.Fatalf("LoadWorkspaceContentOverrides: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d (%v)", len(labels), labels)
	}
	if labels["ws-kicker"] != "工作台" {
		t.Fatalf("ws-kicker = %q", labels["ws-kicker"])
	}
	if labels["knowledge_base"] != "知识库" {
		t.Fatalf("knowledge_base = %q", labels["knowledge_base"])
	}
	if len(descriptions) != 1 {
		t.Fatalf("expected 1 description, got %d (%v)", len(descriptions), descriptions)
	}
	if descriptions["knowledge_base"] != "浏览和管理文档与知识工件。" {
		t.Fatalf("knowledge_base description = %q", descriptions["knowledge_base"])
	}
}

func TestLoadWorkspaceContentOverridesMalformedTomlReturnsError(t *testing.T) {
	withWorkspaceContentLabelsDir(t, map[string]string{
		"labels-en.toml": `this is not valid toml [[[`,
	})

	_, _, err := LoadWorkspaceContentOverrides("en")
	if err == nil {
		t.Fatal("expected an error for malformed TOML, got nil")
	}
}

func TestLoadWorkspaceContentOverridesRejectsUnrecognizedLangShape(t *testing.T) {
	withWorkspaceContentLabelsDir(t, map[string]string{
		"labels-..%2f..%2fetc.toml": `[labels]
x = "y"`,
	})

	labels, descriptions, err := LoadWorkspaceContentOverrides("../../etc/passwd")
	if err != nil {
		t.Fatalf("LoadWorkspaceContentOverrides: %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("expected empty labels map for invalid lang shape, got %v", labels)
	}
	if len(descriptions) != 0 {
		t.Fatalf("expected empty descriptions map for invalid lang shape, got %v", descriptions)
	}
}
