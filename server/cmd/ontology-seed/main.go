// Command ontology-seed authors the curated core 4a ontology modules as data
// (core, document-authority, measurement) and, unless --author-only, releases
// and activates them through the module compiler. It is the DB-native
// authoring surface for platform-owned vocabulary -- content lives in the
// database (2026-07-31 storage decision), not in a data repository.
//
// Usage:
//
//	ontology-seed --module core|document-authority|semantic-processing|measurement|all [--author-only]
//
// Re-running is safe and idempotent: module metadata is reconciled from the
// compiled-in content, curated terms and labels that changed since their last
// release are versioned as approved, and a new content-derived release is cut
// whenever the compiled-in content has not already been released. The quantity
// module is imported separately (QUDT catalog) and is a dependency of
// measurement, so measurement is only released once its dependencies have a
// release to pin.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/seed"
)

func main() {
	log.SetFlags(0)
	moduleFlag := flag.String("module", "", "module to seed: core|document-authority|semantic-processing|measurement|all")
	authorOnly := flag.Bool("author-only", false, "author content only; do not release or activate")
	flag.Parse()

	targets := []string{*moduleFlag}
	if *moduleFlag == "all" || *moduleFlag == "" {
		targets = []string{"core", "document-authority", "semantic-processing", "measurement"}
	}

	db := connect()
	defer db.Close()
	ctx := context.Background()

	warnings, err := seed.SeedCuratedModules(ctx, db, targets, *authorOnly)
	if err != nil {
		log.Fatal(err)
	}
	for _, warning := range warnings {
		log.Printf("warning: %s", warning)
	}
}

func connect() *sql.DB {
	dbName := os.Getenv("PG_DB_NAME")
	if dbName == "" {
		log.Fatal("PG_DB_NAME must be set")
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), postgresUserName(), dbName)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func postgresUserName() string {
	for _, key := range []string{"PG_USER_NAME", "PG_USER"} {
		if user := strings.TrimSpace(os.Getenv(key)); user != "" {
			return user
		}
	}
	return "cding"
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
