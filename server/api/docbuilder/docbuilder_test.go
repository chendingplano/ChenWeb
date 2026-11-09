package docbuilder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
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

func fixture(t *testing.T) string {
	dir := t.TempDir()
	write(t, dir, "a.md", "# Alpha Guide\n\ntext")
	write(t, dir, "sub/b.md", "---\ntitle: Beta Notes\n---\nbody")
	write(t, dir, "sub/deep/c.pdf", "%PDF-1.4 fake")
	write(t, dir, "ignored.txt", "x")
	write(t, dir, ".hidden/d.md", "# no")
	return dir
}

func TestScanFindsMarkdownAndPDFRecursively(t *testing.T) {
	es, err := Scan(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Entry{}
	for _, e := range es {
		got[e.Path] = e
	}
	if len(es) != 3 {
		t.Fatalf("want 3 entries, got %d: %+v", len(es), es)
	}
	if got["a.md"].Title != "Alpha Guide" || got["sub/b.md"].Title != "Beta Notes" || got["sub/deep/c.pdf"].Kind != KindPDF {
		t.Fatalf("titles/kinds wrong: %+v", got)
	}
}

func TestClassifyIsOneTime(t *testing.T) {
	dir := fixture(t)
	m, _ := LoadManifest(dir)
	if err := Refresh(dir, m); err != nil {
		t.Fatal(err)
	}
	fc := &fakeClassifier{}
	n, err := ClassifyNew(context.Background(), dir, m, fc, "test", false)
	if err != nil || n != 3 || fc.calls != 1 {
		t.Fatalf("first run: n=%d calls=%d err=%v", n, fc.calls, err)
	}
	// Reload from disk: nothing is pending, so no model call; a new file is the only one classified.
	m2, _ := LoadManifest(dir)
	write(t, dir, "new.md", "# New")
	if err := Refresh(dir, m2); err != nil {
		t.Fatal(err)
	}
	n, err = ClassifyNew(context.Background(), dir, m2, fc, "test", false)
	if err != nil || n != 1 || fc.calls != 2 {
		t.Fatalf("second run: n=%d calls=%d err=%v", n, fc.calls, err)
	}
	n, _ = ClassifyNew(context.Background(), dir, m2, fc, "test", false)
	if n != 0 || fc.calls != 2 {
		t.Fatalf("third run must not call the model: n=%d calls=%d", n, fc.calls)
	}
}

func TestBuildTreeUncategorizedLast(t *testing.T) {
	tree := BuildTree([]Entry{
		{Path: "x.md", Title: "X", Kind: KindMarkdown},
		{Path: "z.md", Title: "Z", Kind: KindMarkdown, Category: "Zeta"},
		{Path: "a.md", Title: "A", Kind: KindMarkdown, Category: "Alpha"},
	})
	if len(tree) != 3 || tree[0].Label != "Alpha" || tree[2].Label != Uncategorized || tree[2].Children[0].Path != "x.md" {
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

func TestFileEndpointOnlyServesScannedFiles(t *testing.T) {
	dir := fixture(t)
	e := echo.New()
	Register(e.Group("/api/v1"), map[string]Source{"s": {Key: "s", Dir: dir, Title: "S"}})
	get := func(url string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		return rec
	}
	if r := get("/api/v1/docbuilder/s/file?path=a.md"); r.Code != 200 || r.Header().Get("Content-Type") == "" {
		t.Fatalf("a.md: %d", r.Code)
	}
	if r := get("/api/v1/docbuilder/s/file?path=sub/deep/c.pdf"); r.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("pdf type %q", r.Header().Get("Content-Type"))
	}
	for _, bad := range []string{"../../etc/passwd", "ignored.txt", ".hidden/d.md", "/etc/passwd"} {
		if r := get("/api/v1/docbuilder/s/file?path=" + bad); r.Code != 404 {
			t.Fatalf("%s must be 404, got %d", bad, r.Code)
		}
	}
	if r := get("/api/v1/docbuilder/nope/tree"); r.Code != 404 {
		t.Fatalf("unknown source: %d", r.Code)
	}
	if r := get("/api/v1/docbuilder/s/tree"); r.Code != 200 {
		t.Fatalf("tree: %d", r.Code)
	}
}
