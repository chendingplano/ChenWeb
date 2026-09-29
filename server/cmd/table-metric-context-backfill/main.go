// Command table-metric-context-backfill rebuilds metric_context, source_table_rows
// and search_document for existing metrics extracted from tables, from the stored
// line files and without LLM calls (openspec change table-row-context, task 7).
// metric_context_en is never touched.
//
// Usage:
//
//	PG_DB_NAME=miner table-metric-context-backfill --dry-run --record-id 416
//	PG_DB_NAME=miner table-metric-context-backfill
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	_ "github.com/lib/pq"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func main() {
	log.SetFlags(0)
	dryRun := flag.Bool("dry-run", false, "report changes without writing")
	recordID := flag.Int64("record-id", 0, "only this kb.inputs id (default: all records)")
	sample := flag.Int("sample", 10, "number of before/after examples to print")
	flag.Parse()

	db := connect()
	defer db.Close()
	// LoadRecordLinesByID resolves line files through the input store, which reads the
	// project DB handle.
	ApiTypes.ProjectDBHandle = db

	printed := 0
	stats, err := docprocessing.RunTableContextBackfill(context.Background(), db, *recordID, *dryRun,
		func(ch docprocessing.TableContextBackfillChange) {
			if printed >= *sample {
				return
			}
			printed++
			fmt.Printf("── record %d  %s  (%s)\n  before: %s\n  after:  %s\n\n",
				ch.RecordID, ch.MetricID, ch.Outcome, oneLine(ch.Before), oneLine(ch.After))
		})
	if err != nil {
		log.Fatalf("backfill: %v", err)
	}

	mode := "updated"
	if *dryRun {
		mode = "would update"
	}
	fmt.Printf("records scanned: %d (line file unreadable: %d %v)\n", stats.Records, stats.RecordErrors, stats.ErrorRecords)
	fmt.Printf("table metrics:   %d, %s: %d\n", stats.TableMetrics, mode, stats.Changed)
	keys := make([]string, 0, len(stats.Outcomes))
	for k := range stats.Outcomes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-12s %d\n", k, stats.Outcomes[k])
	}
}

func oneLine(s string) string { return strings.ReplaceAll(s, "\n", " ⏎ ") }

func connect() *sql.DB {
	dbName := os.Getenv("PG_DB_NAME")
	if dbName == "" {
		log.Fatal("PG_DB_NAME must be set")
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), postgresUserName(), dbName)
	if pw := os.Getenv("PG_PASSWORD"); pw != "" {
		dsn += " password=" + pw
	}
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
