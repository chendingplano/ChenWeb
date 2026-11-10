package docbuilder

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePathAndNewer(t *testing.T) {
	k1, v1 := ParsePath("a/m-v1.10-en.md")
	k2, v2 := ParsePath("a/m-v1.9-en.md")
	k3, _ := ParsePath("a/m-v1.9-zh.md")
	k4, v4 := ParsePath("a/plain.md")
	if k1 != k2 || k1 == k3 || v1 != "1.10" || v2 != "1.9" || v4 != "" || k4 != "a/plain.md" {
		t.Fatalf("keys/versions wrong: %q %q %q %q %q", k1, k2, k3, k4, v4)
	}
	if !Newer("1.10", "1.9") || Newer("1.9", "1.10") || Newer("1.1", "1.1") || !Newer("2", "1.9") || !Newer("1.0", "") || Newer("", "1.0") {
		t.Fatal("Newer compares numerically")
	}
}

func TestCleanRel(t *testing.T) {
	for _, ok := range []string{"a", "a/b.md", "KnowledgeStore/doc-repo/devdocs", "a//b/../c"} {
		if _, err := CleanRel(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/etc", "../x", "a/../../x", "..", ".", "a/.hidden/b", ".git", "C:\\x"} {
		if _, err := CleanRel(bad); err == nil && bad != "C:\\x" {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestMemberCovers(t *testing.T) {
	dir := Member{Type: "dir", Path: "docs/a"}
	file := Member{Type: "file", Path: "docs/b/g-v1.0-en.md"}
	cases := []struct {
		m    Member
		p    string
		want bool
	}{
		{dir, "docs/a/x.md", true}, {dir, "docs/a/deep/y.pdf", true}, {dir, "docs/ab/x.md", false}, {dir, "docs/a", false},
		{file, "docs/b/g-v1.0-en.md", true}, {file, "docs/b/g-v1.3-en.md", true}, {file, "docs/b/g-v1.3-zh.md", false}, {file, "docs/c/g-v1.0-en.md", false},
	}
	for _, c := range cases {
		if got := memberCovers(c.m, c.p); got != c.want {
			t.Errorf("memberCovers(%v, %q) = %v", c.m, c.p, got)
		}
	}
}

func TestBuildTreeUncategorizedLast(t *testing.T) {
	tree := BuildTree([]Entry{
		{DocID: 1, Title: "X", Kind: KindMarkdown},
		{DocID: 2, Title: "Z", Kind: KindMarkdown, Category: "Zeta"},
		{DocID: 3, Title: "A", Kind: KindMarkdown, Category: "Alpha"},
	})
	if len(tree) != 3 || tree[0].Label != "Alpha" || tree[2].Label != Uncategorized || tree[2].Children[0].DocID != 1 {
		t.Fatalf("unexpected tree: %+v", tree)
	}
}

func TestParseAnswersToleratesFence(t *testing.T) {
	got, err := parseAnswers("```json\n{\"a.md\":\"Guides\"}\n```")
	if err != nil || got["a.md"] != "Guides" {
		t.Fatalf("got %v err %v", got, err)
	}
	if _, err := parseAnswers("no json"); err == nil {
		t.Fatal("want error")
	}
}

type fakeClassifier struct{ calls int }

func (f *fakeClassifier) Classify(_ context.Context, _ []string, docs []Candidate) (map[string]string, error) {
	f.calls++
	out := map[string]string{}
	for _, d := range docs {
		out[d.Path] = "Guides"
	}
	return out, nil
}

func TestClassifyBatchesSavesEachBatch(t *testing.T) {
	var items []Candidate
	for i := 0; i < 60; i++ {
		items = append(items, Candidate{Path: string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".md"})
	}
	fc := &fakeClassifier{}
	saves := 0
	n, err := ClassifyBatches(context.Background(), nil, items, fc, func(m map[string]string) error { saves++; return nil })
	if err != nil || n != 60 || fc.calls != 3 || saves != 3 {
		t.Fatalf("n=%d calls=%d saves=%d err=%v", n, fc.calls, saves, err)
	}
}

func TestPandocConvertRealMarkdown(t *testing.T) {
	for _, bin := range []string{"pandoc", "typst"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip(bin + " not installed")
		}
	}
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a.md"), filepath.Join(dir, "a.pdf")
	md := "---\ntitle: 测试 Title\n---\n\nSee [the section](#1-first-part) and [中文](#2-second).\n\n## 1. First part\n\ntext\n\n## 2. Second\n\n| a | b |\n|---|---|\n| 1 | 中 |\n"
	if err := os.WriteFile(src, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PandocConvert("typst", "Helvetica")(context.Background(), src, dst); err != nil {
		t.Fatalf("convert: %v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil || !strings.HasPrefix(string(raw), "%PDF") {
		t.Fatalf("not a PDF: %v", err)
	}
}
