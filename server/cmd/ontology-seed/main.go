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

	"github.com/chendingplano/deepdoc/server/api/ontology/seed"
)

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

	if err := seed.SeedCuratedModules(ctx, db, targets, *authorOnly); err != nil {
		log.Fatal(err)
	}
}

func connect() *sql.DB {
	dbName := os.Getenv("PG_DB_NAME")
	if dbName == "" {
		log.Fatal("PG_DB_NAME must be set")
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envFirst("PG_USER_NAME", "PG_USER", "cding"), dbName)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
