package releaseshandler

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func rel(v Version, kind, desc string, items ...Item) Release {
	r := Release{Project: ProjectChenWeb, V: v, Version: v.String(), Kind: kind, Status: StatusReleased,
		Description: desc, ReleaseDate: "2026-11-19", Items: items}
	if kind == KindBuild {
		r.BuildNumber = "20261119063000"
	}
	return r
}

// sampleHistory is newest first, like History.
func sampleHistory() []Release {
	return []Release{
		rel(Version{1, 3, 2}, KindBuild, "", Item{ItemType: "new feature", Component: "ChenWeb", Description: "Release picker"}),
		rel(Version{1, 3, 1}, KindBuild, ""),
		rel(Version{1, 3, 0}, KindMinor, "Release management"),
		rel(Version{1, 2, 6}, KindBuild, "",
			Item{ItemType: "bug fix", Component: "shared", Description: "DeepSeek span repair corrupted quotes", Issue: "26"},
			Item{ItemType: "bug fix", Component: "ChenWeb", Description: "Cited table evidence scrolled out of view", Issue: "142"},
			Item{ItemType: "improvement", Component: "shared", Description: "Faster *pool* start"}),
		rel(Version{1, 2, 5}, KindBuild, ""),
	}
}

func TestInRange(t *testing.T) {
	from := Version{1, 2, 5}
	got := InRange(sampleHistory(), &from, Version{1, 3, 2})
	var versions []string
	for _, r := range got {
		versions = append(versions, r.Version)
	}
	if strings.Join(versions, ",") != "01.03.02,01.03.01,01.03.00,01.02.06" {
		t.Fatalf("range = %v", versions)
	}
	if n := len(InRange(sampleHistory(), nil, Version{1, 2, 6})); n != 2 {
		t.Fatalf("from start = %d releases, want 2", n)
	}
	building := sampleHistory()
	building[0].Status = StatusBuilding
	if n := len(InRange(building, nil, Version{1, 3, 2})); n != 4 {
		t.Fatalf("building release must be left out, got %d", n)
	}
}

func TestRenderSingleRelease(t *testing.T) {
	from := Version{1, 2, 5}
	to := Version{1, 2, 6}
	n := Render(ProjectChenWeb, InRange(sampleHistory(), &from, to), &from, to, "en")
	want := `ChenWeb release notes
Release 01.02.06

01.02.06 (2026-11-19, build 20261119063000)
  Improvements
  - [shared] Faster *pool* start
  Bug fixes
  - DeepSeek span repair corrupted quotes (shared#26)
  - Cited table evidence scrolled out of view (#142)
`
	if n.Text != want {
		t.Fatalf("text =\n%s\nwant\n%s", n.Text, want)
	}
	for _, s := range []string{`title: "ChenWeb release notes"`, `### 01.02.06`, `**Bug fixes**`,
		`- DeepSeek span repair corrupted quotes (shared#26)`, `- [shared] Faster \*pool\* start`} {
		if !strings.Contains(n.Markdown, s) {
			t.Errorf("markdown lacks %q:\n%s", s, n.Markdown)
		}
	}
}

func TestRenderRangeAcrossMinor(t *testing.T) {
	from := Version{1, 2, 5}
	to := Version{1, 3, 2}
	n := Render(ProjectChenWeb, InRange(sampleHistory(), &from, to), &from, to, "en")
	i32 := strings.Index(n.Text, "01.03.02 (")
	i31 := strings.Index(n.Text, "01.03.01 (")
	head := strings.Index(n.Text, "== 01.03 — Release management ==")
	i26 := strings.Index(n.Text, "01.02.06 (")
	if !(i32 >= 0 && i32 < i31 && i31 < head && head < i26) {
		t.Fatalf("order wrong:\n%s", n.Text)
	}
	if !strings.Contains(n.Text, "Changes after 01.02.05 up to 01.03.02") {
		t.Fatalf("subtitle missing:\n%s", n.Text)
	}
	if !strings.Contains(n.Text, "01.03.01 (2026-11-19, build 20261119063000)\n  No changes were recorded.") {
		t.Fatalf("empty release line missing:\n%s", n.Text)
	}
}

func TestRenderChinese(t *testing.T) {
	to := Version{1, 2, 6}
	n := Render(ProjectChenWeb, InRange(sampleHistory(), nil, to), nil, to, "zh-cn")
	for _, s := range []string{"ChenWeb 发布说明", "截至 01.02.06 的变更", "缺陷修复", "改进", "构建 "} {
		if !strings.Contains(n.Text, s) {
			t.Errorf("text lacks %q:\n%s", s, n.Text)
		}
	}
}

func TestPDFRendererCachesByContent(t *testing.T) {
	calls := 0
	p := &PDFRenderer{CacheDir: t.TempDir(), Engine: "typst", Font: "Helvetica",
		Convert: func(_ context.Context, src, dst string) error {
			calls++
			md, _ := os.ReadFile(src)
			return os.WriteFile(dst, append([]byte("%PDF "), md...), 0o644)
		}}
	a, err := p.PDF(context.Background(), "# one")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := p.PDF(context.Background(), "# one")
	c, _ := p.PDF(context.Background(), "# two")
	if a != b || a == c || calls != 2 {
		t.Fatalf("paths %s %s %s, %d conversions", a, b, c, calls)
	}
}

func TestPDFRendererNamesMissingTool(t *testing.T) {
	p := &PDFRenderer{CacheDir: t.TempDir(), Engine: "typst", Font: "Helvetica",
		LookPath: func(name string) (string, error) {
			if name == "pandoc" {
				return "", errors.New("not found")
			}
			return "/bin/" + name, nil
		}}
	_, err := p.PDF(context.Background(), "# x")
	if !errors.Is(err, ErrPDFUnavailable) || !strings.Contains(err.Error(), "pandoc") {
		t.Fatalf("err = %v", err)
	}
}

// TestPDFRendererReal runs pandoc for real when RELEASES_TEST_REAL_PDF=1.
func TestPDFRendererReal(t *testing.T) {
	if os.Getenv("RELEASES_TEST_REAL_PDF") != "1" {
		t.Skip("RELEASES_TEST_REAL_PDF not set")
	}
	from, to := Version{1, 2, 5}, Version{1, 3, 2}
	p := NewPDFRenderer(t.TempDir())
	for _, lang := range Langs {
		n := Render(ProjectChenWeb, InRange(sampleHistory(), &from, to), &from, to, lang)
		path, err := p.PDF(context.Background(), n.Markdown)
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		data, _ := os.ReadFile(path)
		if !strings.HasPrefix(string(data), "%PDF") {
			t.Fatalf("%s: not a PDF", lang)
		}
		if out := os.Getenv("RELEASES_TEST_PDF_OUT"); out != "" {
			_ = os.WriteFile(out+"-"+lang+".pdf", data, 0o644)
		}
	}
}

func TestPreviousBuild(t *testing.T) {
	if p := PreviousBuild(sampleHistory(), Version{1, 3, 1}); p == nil || *p != (Version{1, 2, 6}) {
		t.Fatalf("PreviousBuild(01.03.01) = %v", p)
	}
	if p := PreviousBuild(sampleHistory(), Version{1, 2, 5}); p != nil {
		t.Fatalf("PreviousBuild(first) = %v", p)
	}
}
