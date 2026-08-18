// Command semantic-baseline produces the ADR 2026081801 Phase 0 corpus
// baseline and capacity model.
//
// Phase 0 is a gate: the full-corpus baseline and capacity report must pass
// before Phase 1 additive schema work begins, and the same report is re-run
// before any writer cutover so the two can be compared directly. The command
// is read-only and safe to run against production.
//
// Usage:
//
//	PG_DB_NAME=miner semantic-baseline [--format markdown|json] [--out FILE]
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	log.SetFlags(0)
	format := flag.String("format", "markdown", "output format: markdown|json")
	out := flag.String("out", "", "write to FILE instead of stdout")
	sourceRoot := flag.String("source-root", ".", "Go source root for kb.ontology_terms reader audit")
	flag.Parse()

	db := connect()
	defer db.Close()

	baseline, err := Collect(context.Background(), db)
	if err != nil {
		log.Fatalf("collect baseline: %v", err)
	}
	baseline.SetFindingsLowEstimate()
	baseline.TermReaders, err = AuditTermReaders(*sourceRoot)
	if err != nil {
		log.Fatalf("audit term readers: %v", err)
	}

	var rendered string
	switch *format {
	case "json":
		buf, err := json.MarshalIndent(baseline, "", "  ")
		if err != nil {
			log.Fatalf("marshal baseline: %v", err)
		}
		rendered = string(buf) + "\n"
	case "markdown":
		rendered = RenderMarkdown(baseline, time.Now())
	default:
		log.Fatalf("unknown format %q", *format)
	}

	if *out == "" {
		fmt.Print(rendered)
		return
	}
	if err := os.WriteFile(*out, []byte(rendered), 0o644); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	log.Printf("wrote %s", *out)
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
