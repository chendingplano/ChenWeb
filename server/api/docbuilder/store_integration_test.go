package docbuilder

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	_ "github.com/lib/pq"
)

// env is a Store on TEST_DATABASE_URL (chenweb_test, never miner) with a temporary document root.
type env struct {
	t     *testing.T
	s     *Store
	db    *sql.DB
	root  string
	ctx   context.Context
	convs int
}

var idCounter atomic.Int64

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	if strings.Contains(dsn, "/miner") {
		t.Fatal("refusing to run against miner")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, f := range []string{"20261110000001_document_groups.sql", "20261110000002_doc_groups_legacy_imported.sql"} {
		mig, err := os.ReadFile("../../../project_migrations/" + f)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.SplitN(strings.SplitN(string(mig), "-- +goose Down", 2)[0], "-- +goose Up", 2)[1]
		if _, err := db.Exec(up); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	e := &env{t: t, db: db, root: root, ctx: context.Background()}
	idCounter.CompareAndSwap(0, 9_000_000_000)
	e.s = &Store{DB: db, Root: root, ArchiveDir: filepath.Join(root, ".doc-archive"), PDFDir: filepath.Join(root, ".doc-pdf"),
		NextID: func(context.Context) (int64, error) { return idCounter.Add(1), nil },
		Convert: func(_ context.Context, src, dst string) error {
			e.convs++
			return os.WriteFile(dst, []byte("%PDF fake of "+filepath.Base(src)), 0o644)
		}}
	t.Cleanup(func() {
		// remove everything this test created: rows whose paths live in this temp root are unique by prefix
		db.Exec(`DELETE FROM kb.doc_groups WHERE group_key LIKE 'itest-%'`)
		db.Exec(`DELETE FROM kb.doc_pdf_exports WHERE file_id IN (SELECT file_id FROM kb.doc_files WHERE path LIKE 'itest/%')`)
		db.Exec(`DELETE FROM kb.doc_group_exclusions WHERE doc_id IN (SELECT doc_id FROM kb.doc_files WHERE path LIKE 'itest/%')`)
		db.Exec(`DELETE FROM kb.doc_group_categories WHERE doc_id IN (SELECT doc_id FROM kb.doc_files WHERE path LIKE 'itest/%')`)
		db.Exec(`DELETE FROM kb.doc_files WHERE path LIKE 'itest/%'`)
		db.Exec(`DELETE FROM kb.doc_documents WHERE doc_id >= 9000000000 AND doc_id NOT IN (SELECT doc_id FROM kb.doc_files)`)
	})
	return e
}

func (e *env) write(rel, body string) {
	e.t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) group(key string, members ...Member) *Group {
	e.t.Helper()
	g := &Group{Key: "itest-" + key, Label: map[string]string{"en": key}, Enabled: true, Order: 10, Roles: []string{"admin"}}
	if err := e.s.SaveGroup(e.ctx, g, "test"); err != nil {
		e.t.Fatal(err)
	}
	for _, m := range members {
		if _, err := e.s.AddMember(e.ctx, g.ID, m.Type, m.Path); err != nil {
			e.t.Fatal(err)
		}
	}
	return g
}

func (e *env) docs(g *Group) map[string]Entry {
	e.t.Helper()
	if err := e.s.SyncGroup(e.ctx, g.ID); err != nil {
		e.t.Fatal(err)
	}
	es, err := e.s.GroupDocs(e.ctx, g.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	out := map[string]Entry{}
	for _, d := range es {
		out[d.Path] = d
	}
	return out
}

func dir(p string) Member  { return Member{Type: "dir", Path: p} }
func file(p string) Member { return Member{Type: "file", Path: p} }

func TestLatestVersionIsOneDocumentInEveryGroup(t *testing.T) {
	e := newEnv(t)
	e.write("itest/a/guide-v1.0-en.md", "# Guide 1.0")
	e.write("itest/a/guide-v1.2-en.md", "# Guide 1.2")
	e.write("itest/a/guide-v1.10-en.md", "# Guide 1.10")
	e.write("itest/a/guide-v1.10-zh.md", "# 指南")
	e.write("itest/a/.hidden/x.md", "# hidden")
	g1 := e.group("latest1", dir("itest/a"))
	g2 := e.group("latest2", file("itest/a/guide-v1.0-en.md"))
	d1, d2 := e.docs(g1), e.docs(g2)
	if len(d1) != 2 || d1["itest/a/guide-v1.10-en.md"].Title != "Guide 1.10" || d1["itest/a/guide-v1.10-zh.md"].Kind != KindMarkdown {
		t.Fatalf("group 1: %+v", d1)
	}
	if len(d2) != 1 || d2["itest/a/guide-v1.10-en.md"].DocID != d1["itest/a/guide-v1.10-en.md"].DocID {
		t.Fatalf("a file member must bring the logical document: %+v", d2)
	}
	// A new version switches every group.
	e.write("itest/a/guide-v1.11-en.md", "# Guide 1.11")
	if _, ok := e.docs(g2)["itest/a/guide-v1.11-en.md"]; !ok {
		t.Fatal("group 2 must follow the new version")
	}
	if _, ok := e.docs(g1)["itest/a/guide-v1.11-en.md"]; !ok {
		t.Fatal("group 1 must follow the new version")
	}
}

func TestRenameKeepsIdentityAndCategory(t *testing.T) {
	e := newEnv(t)
	e.write("itest/b/one.md", "# One\nbody")
	e.write("itest/b/two.md", "# Two\nbody two")
	g := e.group("rename", dir("itest/b"))
	cl := &fakeClassifier{}
	if n, err := e.s.ClassifyGroup(e.ctx, g, cl, "m", false); err != nil || n != 2 {
		t.Fatalf("classify n=%d err=%v", n, err)
	}
	before := e.docs(g)
	// pure rename: same identity, same category, no classifier call
	if err := os.Rename(filepath.Join(e.root, "itest/b/one.md"), filepath.Join(e.root, "itest/b/one-renamed.md")); err != nil {
		t.Fatal(err)
	}
	// rename + edit: a new document
	if err := os.Remove(filepath.Join(e.root, "itest/b/two.md")); err != nil {
		t.Fatal(err)
	}
	e.write("itest/b/two-new.md", "# Two edited\ncompletely different")
	after := e.docs(g)
	if after["itest/b/one-renamed.md"].DocID != before["itest/b/one.md"].DocID || after["itest/b/one-renamed.md"].Category != "Guides" {
		t.Fatalf("rename lost identity or category: %+v vs %+v", after, before)
	}
	nd := after["itest/b/two-new.md"]
	if nd.DocID == before["itest/b/two.md"].DocID || nd.Category != "" || len(after) != 2 {
		t.Fatalf("rename+edit must be a new uncategorized document, old one hidden: %+v", after)
	}
	if n, err := e.s.ClassifyGroup(e.ctx, g, cl, "m", false); err != nil || n != 1 || cl.calls != 2 {
		t.Fatalf("only the new document is classified: n=%d calls=%d err=%v", n, cl.calls, err)
	}
}

func TestCategoriesArePerGroup(t *testing.T) {
	e := newEnv(t)
	e.write("itest/c/x.md", "# X")
	g1, g2 := e.group("cat1", dir("itest/c")), e.group("cat2", dir("itest/c"))
	if _, err := e.s.ClassifyGroup(e.ctx, g1, &fakeClassifier{}, "m", false); err != nil {
		t.Fatal(err)
	}
	if e.docs(g1)["itest/c/x.md"].Category == "" || e.docs(g2)["itest/c/x.md"].Category != "" {
		t.Fatal("classifying group 1 must not categorise group 2")
	}
}

func TestExcludeAndDeleteRules(t *testing.T) {
	e := newEnv(t)
	e.write("itest/d/keep.md", "# Keep")
	e.write("itest/d/shared.md", "# Shared")
	e.write("itest/d/solo.md", "# Solo")
	g1 := e.group("del1", dir("itest/d"))
	g2 := e.group("del2", file("itest/d/shared.md"))
	docs := e.docs(g1)
	shared, solo := docs["itest/d/shared.md"].DocID, docs["itest/d/solo.md"].DocID

	// shared file: delete is refused, nothing changes
	blocked, err := e.s.RemoveDocument(e.ctx, g1.ID, shared, true)
	if !errors.Is(err, ErrInUse) || len(blocked) != 1 || blocked[0] != g2.Key {
		t.Fatalf("want ErrInUse naming %s, got %v %v", g2.Key, blocked, err)
	}
	if _, err := os.Stat(filepath.Join(e.root, "itest/d/shared.md")); err != nil || len(e.docs(g1)) != 3 {
		t.Fatal("refused delete must change nothing")
	}
	// membership-only removal: excluded from g1, file stays, still in g2
	if _, err := e.s.RemoveDocument(e.ctx, g1.ID, shared, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.docs(g1)["itest/d/shared.md"]; ok {
		t.Fatal("shared must be excluded from g1")
	}
	if _, ok := e.docs(g2)["itest/d/shared.md"]; !ok {
		t.Fatal("shared must stay in g2")
	}
	// solo: delete archives, not unlinks
	if _, err := e.s.RemoveDocument(e.ctx, g1.ID, solo, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.root, "itest/d/solo.md")); !os.IsNotExist(err) {
		t.Fatal("solo must be gone from its place")
	}
	var archived int
	_ = filepath.Walk(e.s.ArchiveDir, func(p string, i os.FileInfo, _ error) error {
		if i != nil && !i.IsDir() && strings.HasSuffix(p, "itest/d/solo.md") {
			archived++
		}
		return nil
	})
	if archived != 1 || len(e.docs(g1)) != 1 {
		t.Fatalf("solo must be in the archive and out of the tree: archived=%d docs=%v", archived, e.docs(g1))
	}
	// re-adding the excluded file as a member clears the exclusion
	if _, err := e.s.AddMember(e.ctx, g1.ID, "file", "itest/d/shared.md"); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.docs(g1)["itest/d/shared.md"]; !ok {
		t.Fatal("adding an excluded file back must restore it")
	}
}

func TestPDFExportCachedAndDroppedOnChange(t *testing.T) {
	e := newEnv(t)
	e.write("itest/p/a.md", "# A")
	e.write("itest/p/b.pdf", "%PDF original")
	g := e.group("pdf", dir("itest/p"))
	docs := e.docs(g)
	a, b := docs["itest/p/a.md"].DocID, docs["itest/p/b.pdf"].DocID
	p1, err := e.s.ExportPDF(e.ctx, g.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.ExportPDF(e.ctx, g.ID, a); err != nil || e.convs != 1 {
		t.Fatalf("second export must reuse the first: convs=%d err=%v", e.convs, err)
	}
	if pb, err := e.s.ExportPDF(e.ctx, g.ID, b); err != nil || !strings.HasSuffix(pb, "itest/p/b.pdf") {
		t.Fatalf("a PDF document is returned as is: %q %v", pb, err)
	}
	e.write("itest/p/a.md", "# A edited, longer")
	e.docs(g) // sync sees the change
	if _, err := os.Stat(p1); !os.IsNotExist(err) {
		t.Fatal("editing the source must delete its PDF")
	}
	if _, err := e.s.ExportPDF(e.ctx, g.ID, a); err != nil || e.convs != 2 {
		t.Fatalf("export after edit must convert again: convs=%d err=%v", e.convs, err)
	}
}

func TestMemberPathsAreValidated(t *testing.T) {
	e := newEnv(t)
	g := e.group("paths")
	for _, c := range []struct{ typ, p string }{{"dir", "../.."}, {"dir", "/etc"}, {"dir", "itest/none"}, {"file", "itest"}, {"dir", ".git"}} {
		if _, err := e.s.AddMember(e.ctx, g.ID, c.typ, c.p); err == nil {
			t.Errorf("%s %q must be refused", c.typ, c.p)
		}
	}
}

func TestLegacyManifestCategoriesAreImportedOnce(t *testing.T) {
	e := newEnv(t)
	e.write("itest/l/a.md", "# A")
	e.write("itest/l/.docbuilder/manifest.json", `{"entries":[{"path":"a.md","category":"Old Category"}]}`)
	g := e.group("legacy", dir("itest/l"))
	if got := e.docs(g)["itest/l/a.md"].Category; got != "Old Category" {
		t.Fatalf("legacy category not imported: %q", got)
	}
	if _, err := e.s.ClassifyGroup(e.ctx, g, &fakeClassifier{}, "m", true); err != nil {
		t.Fatal(err)
	}
	if got := e.docs(g)["itest/l/a.md"].Category; got != "Guides" {
		t.Fatalf("force must not re-import the legacy category: %q", got)
	}
}

func TestMenuOrderAndRoles(t *testing.T) {
	e := newEnv(t)
	api := &API{Store: e.s, ValidRoles: func() []string { return []string{"admin", "dev", "guest"} }}
	mk := func(key string, order int, roles []string, enabled bool) *Group {
		g := &Group{Key: "itest-" + key, Label: map[string]string{"en": "EN " + key, "zh-cn": "ZH " + key}, Parent: "itest-menu", Order: order, Roles: roles, Enabled: enabled}
		if err := e.s.SaveGroup(e.ctx, g, "test"); err != nil {
			t.Fatal(err)
		}
		return g
	}
	dev := mk("dev", 10, []string{"dev"}, true)
	man := mk("man", 20, []string{"dev", "guest"}, true)
	mk("off", 5, []string{"dev"}, false)
	mk("none", 1, []string{}, true)
	keys := func(roles []string, lang string) (out []string) {
		items, err := api.menuFor(e.ctx, roles, lang)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range items {
			if i.Parent == "itest-menu" {
				out = append(out, i.Key+"="+i.Label)
			}
		}
		return
	}
	if got := strings.Join(keys([]string{"dev"}, "en"), ","); got != "itest-dev=EN dev,itest-man=EN man" {
		t.Fatalf("order by position, disabled and empty-role groups hidden: %s", got)
	}
	if got := strings.Join(keys([]string{"guest"}, "zh-cn"), ","); got != "itest-man=ZH man" {
		t.Fatalf("a role sees only its groups, with Chinese labels: %s", got)
	}
	if got := keys([]string{"trial"}, "en"); len(got) != 0 {
		t.Fatalf("no matching role sees nothing: %v", got)
	}
	// Swapping the position numbers swaps the order.
	dev.Order, man.Order = 20, 10
	for _, g := range []*Group{dev, man} {
		if err := e.s.SaveGroup(e.ctx, g, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(keys([]string{"dev"}, "en"), ","); got != "itest-man=EN man,itest-dev=EN dev" {
		t.Fatalf("swapped positions must swap the menu order: %s", got)
	}
}

func TestWrongRootDoesNotHideDocumentsAndFilesComeBack(t *testing.T) {
	e := newEnv(t)
	e.write("itest/r/a.md", "# A")
	e.write("itest/r/b.md", "# B")
	g := e.group("root", dir("itest/r"))
	before := e.docs(g)
	if len(before) != 2 {
		t.Fatalf("want 2 documents, got %v", before)
	}
	// Point the store at a root where the member directory does not exist (wrong DOCUMENT_ROOT_DIR):
	// nothing is known about the files, so none may be marked missing.
	realRoot := e.s.Root
	e.s.Root = t.TempDir()
	if err := e.s.SyncGroup(e.ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	e.s.Root = realRoot
	if after := e.docs(g); len(after) != 2 {
		t.Fatalf("an unreachable member must not hide its documents: %v", after)
	}
	// A file that is really gone is hidden, and comes back with the same ID when it returns.
	idA := before["itest/r/a.md"].DocID
	if err := os.Rename(filepath.Join(e.root, "itest/r/a.md"), filepath.Join(e.root, "a.md.away")); err != nil {
		t.Fatal(err)
	}
	if got := e.docs(g); len(got) != 1 {
		t.Fatalf("a deleted file must disappear: %v", got)
	}
	if err := os.Rename(filepath.Join(e.root, "a.md.away"), filepath.Join(e.root, "itest/r/a.md")); err != nil {
		t.Fatal(err)
	}
	if got := e.docs(g)["itest/r/a.md"]; got.DocID != idA {
		t.Fatalf("a returning file keeps its document ID: %+v", got)
	}
}
