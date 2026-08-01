// Command ontology-seed authors the curated core 4a ontology modules as data
// (core, document-authority, measurement) and, unless --author-only, releases
// and activates them through the module compiler. It is the DB-native
// authoring surface for platform-owned vocabulary -- content lives in the
// database (2026-07-31 storage decision), not in a data repository.
//
// Usage:
//
//	ontology-seed --module core|document-authority|measurement|all [--author-only]
//
// Re-running is safe: existing modules, terms, labels, and releases are
// skipped rather than overwritten. The quantity module is imported separately
// (QUDT catalog) and is a dependency of measurement, so measurement is only
// released once its dependencies have a release to pin.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

type moduleContent struct {
	ModuleID  string
	Version   string
	Title     string
	Owner     string
	DependsOn []string
	Terms     []seedTerm
}

var curatedModules = map[string]moduleContent{
	"core":               coreModule,
	"document-authority": documentAuthorityModule,
	"measurement":        measurementModule,
}

func main() {
	log.SetFlags(0)
	moduleFlag := flag.String("module", "", "module to seed: core|document-authority|measurement|all")
	authorOnly := flag.Bool("author-only", false, "author content only; do not release or activate")
	flag.Parse()

	targets := []string{*moduleFlag}
	if *moduleFlag == "all" || *moduleFlag == "" {
		targets = []string{"core", "document-authority", "measurement"}
	}

	db := connect()
	defer db.Close()
	ctx := context.Background()

	for _, id := range targets {
		mc, ok := curatedModules[id]
		if !ok {
			log.Fatalf("unknown module %q", id)
		}
		fmt.Printf("== %s ==\n", id)
		authorModule(ctx, db, mc)
		if !*authorOnly {
			releaseAndActivate(ctx, db, mc)
		}
	}
}

func authorModule(ctx context.Context, db *sql.DB, mc moduleContent) {
	ms := modules.ModuleStore{DB: db}
	if _, err := ms.GetModule(ctx, mc.ModuleID); err != nil {
		_, err := ms.CreateModule(ctx, modules.Module{
			ModuleID: mc.ModuleID, Title: mc.Title, Owner: mc.Owner,
			DependsOn: mc.DependsOn, Status: "active", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
		})
		if err != nil {
			log.Fatalf("register module %s: %v", mc.ModuleID, err)
		}
		fmt.Printf("  module %s registered (depends_on=%v)\n", mc.ModuleID, mc.DependsOn)
	} else {
		fmt.Printf("  module %s already registered\n", mc.ModuleID)
	}

	ts := terms.TermStore{DB: db}
	ls := terms.LabelStore{DB: db}
	for _, t := range mc.Terms {
		if _, err := ts.GetTermLatest(ctx, t.ID); err == nil {
			continue // already authored
		}
		created, err := ts.CreateTerm(ctx, terms.Term{
			TermID: t.ID, TermKind: t.Kind, ModuleID: mc.ModuleID,
			Definition: t.Def, Status: "approved", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
		})
		if err != nil {
			log.Fatalf("author term %s: %v", t.ID, err)
		}
		_ = created
		for _, l := range t.Labels {
			if _, err := ls.CreateLabel(ctx, terms.TermLabel{
				TermID: t.ID, Label: l.Label, Lang: l.Lang, LabelRole: l.Role,
				Status: "approved", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
			}); err != nil {
				log.Fatalf("author label %s (%s): %v", t.ID, l.Label, err)
			}
		}
	}
	fmt.Printf("  terms authored: %d\n", len(mc.Terms))
}

func releaseAndActivate(ctx context.Context, db *sql.DB, mc moduleContent) {
	rs := modules.ReleaseStore{DB: db}
	if _, err := rs.GetRelease(ctx, mc.ModuleID, mc.Version); err == nil {
		fmt.Printf("  release %s@%s already exists; skipping\n", mc.ModuleID, mc.Version)
		return
	}
	rel, err := rs.CreateRelease(ctx, mc.ModuleID, mc.Version, "ontology-seed")
	if err != nil {
		fmt.Printf("  release %s@%s skipped: %v\n", mc.ModuleID, mc.Version, err)
		return
	}
	if _, err := rs.Activate(ctx, mc.ModuleID, rel.ID, "ontology-seed"); err != nil {
		log.Fatalf("activate %s@%s: %v", mc.ModuleID, mc.Version, err)
	}
	fmt.Printf("  released + activated %s@%s (release id=%d, checksum=%s)\n",
		mc.ModuleID, mc.Version, rel.ID, short(rel.ContentChecksum))
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"),
		envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
