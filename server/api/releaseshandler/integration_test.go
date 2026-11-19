package releaseshandler

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// These tests run only against scratch databases that hold the release tables (never
// miner): RELEASES_TEST_DSN_A is the build machine, RELEASES_TEST_DSN_B the deploy machine.
// Credentials come from the PG* environment variables, not the DSN.
func scratchDB(t *testing.T, env string) *sql.DB {
	t.Helper()
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skip(env + " not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`TRUNCATE kb.release_items, kb.releases, kb.release_current RESTART IDENTITY`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestIntegrationBuildCycle(t *testing.T) {
	a := scratchDB(t, "RELEASES_TEST_DSN_A")
	b := scratchDB(t, "RELEASES_TEST_DSN_B")
	ctx := context.Background()
	alive := func(int) bool { return true }
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	if _, err := CreateLine(ctx, a, ProjectChenWeb, KindMajor, "First major", "admin"); err != nil {
		t.Fatal(err)
	}
	fix, err := AddItem(ctx, a, ProjectChenWeb, Item{ItemType: "bug fix", Component: "ChenWeb", Description: "Fix one", Issue: "142"}, "admin")
	must(err)
	_, err = AddItem(ctx, a, ProjectChenWeb, Item{ItemType: "improvement", Component: "shared", Description: "Shared tweak"}, "agent")
	must(err)

	// Build 01.00.01 takes both items; an item added during the build stays pending.
	r, err := Reserve(ctx, a, ProjectChenWeb, "mac", 100, alive, "dev")
	must(err)
	if r.Version != "01.00.01" {
		t.Fatalf("reserved %s", r.Version)
	}
	late, err := AddItem(ctx, a, ProjectChenWeb, Item{ItemType: "new feature", Component: "ChenWeb", Description: "Late"}, "admin")
	must(err)
	if _, err := UpdateItem(ctx, a, fix.ID, Item{ItemType: "bug fix", Component: "ChenWeb", Description: "Edited"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("editing a bound item: %v", err)
	}
	if _, err := CreateLine(ctx, a, ProjectChenWeb, KindMinor, "", "admin"); !errors.Is(err, ErrConflict) {
		t.Fatalf("new minor during a build: %v", err)
	}
	must(SetBuildInfo(ctx, a, ProjectChenWeb, r.V, BuildInfo{BuildNumber: "20261119063000", GitRev: "abc", SharedGitRev: "def", SharedGitDirty: true}))
	f6, err := BuildFile(ctx, a, ProjectChenWeb, r.V, time.Now())
	must(err)
	if len(f6.History) != 2 || len(f6.History[0].Items) != 2 {
		t.Fatalf("file history = %+v", f6.History)
	}
	must(Commit(ctx, a, ProjectChenWeb, r.V))
	cur, build, err := Current(ctx, a, ProjectChenWeb)
	must(err)
	if cur.String() != "01.00.01" || build != "20261119063000" {
		t.Fatalf("current = %s %s", cur, build)
	}
	pending, err := Pending(ctx, a, ProjectChenWeb)
	must(err)
	if len(pending) != 1 || pending[0].ID != late.ID {
		t.Fatalf("pending = %+v", pending)
	}

	// A failed build gives 01.00.02 back; a killed one is reverted by the next build.
	r2, err := Reserve(ctx, a, ProjectChenWeb, "mac", 200, alive, "dev")
	must(err)
	must(Abort(ctx, a, ProjectChenWeb, r2.V))
	if p, _ := Pending(ctx, a, ProjectChenWeb); len(p) != 1 {
		t.Fatalf("abort did not return the item: %+v", p)
	}
	_, err = Reserve(ctx, a, ProjectChenWeb, "mac", 300, alive, "dev")
	must(err)
	r4, err := Reserve(ctx, a, ProjectChenWeb, "mac", 400, func(pid int) bool { return pid != 300 }, "dev")
	must(err)
	if r4.Version != "01.00.02" {
		t.Fatalf("after a killed build reserved %s", r4.Version)
	}
	must(SetBuildInfo(ctx, a, ProjectChenWeb, r4.V, BuildInfo{BuildNumber: "20261119073000"}))
	f7, err := BuildFile(ctx, a, ProjectChenWeb, r4.V, time.Now())
	must(err)
	must(Commit(ctx, a, ProjectChenWeb, r4.V))

	// Deploy machine: import the newer file, then the older one; nothing is lost or doubled.
	for _, f := range []File{f7, f6, f7} {
		if _, err := Import(ctx, b, f); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := History(ctx, b, ProjectChenWeb)
	must(err)
	if len(hist) != 3 || hist[0].Version != "01.00.02" || len(hist[1].Items) != 2 || len(hist[0].Items) != 1 {
		t.Fatalf("imported history = %+v", hist)
	}
	must(SetCurrent(ctx, b, ProjectChenWeb, r.V, "20261119063000"))
	if cur, _, _ := Current(ctx, b, ProjectChenWeb); cur.String() != "01.00.01" {
		t.Fatalf("deploy current = %s", cur)
	}
	if v, err := VersionForBuild(ctx, b, ProjectChenWeb, "20261119073000"); err != nil || v != "01.00.02" {
		t.Fatalf("VersionForBuild = %q, %v", v, err)
	}
	if v, _ := VersionForBuild(ctx, b, ProjectChenWeb, "20261116224055"); v != "" {
		t.Fatalf("unknown build = %q", v)
	}
	must(ClearCurrent(ctx, b, ProjectChenWeb))
	if cur, build, _ := Current(ctx, b, ProjectChenWeb); cur != (Version{}) || build != "" {
		t.Fatalf("after clear = %s %s", cur, build)
	}
}
