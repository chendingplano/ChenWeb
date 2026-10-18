// Command metric-frame-backfill moves the legacy apparatus, medium, method and stage qualifiers of
// stored metric rows out of condition into the structured frame, with no LLM calls (openspec change
// metric-frame, task 5). Every changed row's before-image is written to --backup, and --revert
// restores a backup file.
//
// Usage:
//
//	PG_DB_NAME=miner metric-frame-backfill --dry-run
//	PG_DB_NAME=miner metric-frame-backfill --table kb.metrics --backup frame-backup.json
//	PG_DB_NAME=miner metric-frame-backfill --revert frame-backup.json
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	_ "github.com/lib/pq"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func main() {
	log.SetFlags(0)
	dryRun := flag.Bool("dry-run", false, "report changes without writing")
	table := flag.String("table", "all", "kb.metrics, testbed.metrics or all")
	recordID := flag.Int64("record-id", 0, "only this input_record_id (default: all records)")
	backup := flag.String("backup", "", "file to write the before-image of every changed row (required unless --dry-run)")
	revert := flag.String("revert", "", "restore the rows of this backup file and exit")
	sample := flag.Int("sample", 10, "number of before/after examples to print per table")
	flag.Parse()

	db := connect()
	defer db.Close()
	ctx := context.Background()

	if *revert != "" {
		b, err := os.ReadFile(*revert)
		if err != nil {
			log.Fatalf("read backup: %v", err)
		}
		var backups []docprocessing.MetricFrameBackup
		if err := json.Unmarshal(b, &backups); err != nil {
			log.Fatalf("parse backup: %v", err)
		}
		n, err := docprocessing.RevertMetricFrameBackfill(ctx, db, backups)
		if err != nil {
			log.Fatalf("revert: %v (restored %d rows before the error)", err, n)
		}
		fmt.Printf("restored %d rows\n", n)
		return
	}
	if !*dryRun && *backup == "" {
		log.Fatal("--backup is required unless --dry-run")
	}

	tables := []string{"kb.metrics", "testbed.metrics"}
	if *table != "all" {
		tables = []string{*table}
	}
	var all []docprocessing.MetricFrameBackup
	for _, t := range tables {
		printed := 0
		stats, backups, err := docprocessing.RunMetricFrameBackfill(ctx, db, t, *recordID, *dryRun,
			func(ch docprocessing.MetricFrameBackfillChange) {
				if printed >= *sample {
					return
				}
				printed++
				fmt.Printf("── %s %s\n  before: %s\n  after:  %s\n  frame:  %s\n\n", ch.Table, ch.MetricID, ch.Before, ch.After, ch.FrameJSON)
			})
		all = append(all, backups...)
		if err != nil {
			writeBackup(*backup, all)
			log.Fatalf("backfill %s: %v", t, err)
		}
		mode := "updated"
		if *dryRun {
			mode = "would update"
		}
		fmt.Printf("%s: rows with a condition %d, %s %d, unresolved 药物浓度 %d\n", t, stats.Rows, mode, stats.Changed, stats.UnresolvedSpecs)
		labels := make([]string, 0, len(stats.Moved))
		for l := range stats.Moved {
			labels = append(labels, l)
		}
		sort.Slice(labels, func(i, j int) bool { return stats.Moved[labels[i]] > stats.Moved[labels[j]] })
		for _, l := range labels {
			fmt.Printf("  %-10s %d\n", l, stats.Moved[l])
		}
	}
	if !*dryRun {
		writeBackup(*backup, all)
		fmt.Printf("before-images of %d rows written to %s\n", len(all), *backup)
	}
}

func writeBackup(path string, backups []docprocessing.MetricFrameBackup) {
	if path == "" {
		return
	}
	b, err := json.MarshalIndent(backups, "", "  ")
	if err != nil {
		log.Fatalf("encode backup: %v", err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		log.Fatalf("write backup: %v", err)
	}
}

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
