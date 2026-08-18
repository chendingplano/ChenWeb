// Command metric-support-cleanup resolves duplicate current metric supporting
// evidence links before uq_assertion_evidence_current_metric_support is
// created (ADR 2026081801 Phase 3 task 6.2). It always reports every metric
// occurrence with more than one active supports link; with --apply it also
// retires the surplus links via classfoundation.MetricSupportCleanup, which
// soft-deletes and audits each retirement in kb.metric_support_cleanup_audit
// rather than deleting evidence.
//
// Usage:
//
//	PG_DB_NAME=miner metric-support-cleanup            # report only
//	PG_DB_NAME=miner metric-support-cleanup --apply     # retire duplicates
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

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
)

func main() {
	log.SetFlags(0)
	apply := flag.Bool("apply", false, "retire duplicate supporting links (default: report only)")
	flag.Parse()

	db := connect()
	defer db.Close()

	ctx := context.Background()
	cleanup := classfoundation.MetricSupportCleanup{DB: db}

	before, err := cleanup.ReportCurrentDuplicates(ctx)
	if err != nil {
		log.Fatalf("report duplicates: %v", err)
	}
	printDuplicates("before", before)

	if !*apply {
		if len(before) > 0 {
			fmt.Printf("\n%d occurrence(s) have duplicate current supporting links; re-run with --apply to retire the surplus.\n", len(before))
		} else {
			fmt.Println("\nno duplicates found; uq_assertion_evidence_current_metric_support is safe to apply.")
		}
		return
	}

	retired, err := cleanup.RetireCurrentDuplicates(ctx)
	if err != nil {
		log.Fatalf("retire duplicates: %v", err)
	}
	fmt.Printf("\nretired %d surplus supporting link(s)\n", retired)

	after, err := cleanup.ReportCurrentDuplicates(ctx)
	if err != nil {
		log.Fatalf("report duplicates after retirement: %v", err)
	}
	printDuplicates("after", after)
	if len(after) > 0 {
		log.Fatalf("%d occurrence(s) still have duplicate current supporting links after retirement", len(after))
	}
}

func printDuplicates(label string, duplicates []classfoundation.MetricSupportDuplicate) {
	fmt.Printf("%s: %d occurrence(s) with duplicate current supporting links\n", label, len(duplicates))
	for _, d := range duplicates {
		inputRecordID := "NULL"
		if d.InputRecordID != nil {
			inputRecordID = fmt.Sprintf("%d", *d.InputRecordID)
		}
		fmt.Printf("  artifact_type=%s artifact_id=%s input_record_id=%s support_links=%d\n",
			d.ArtifactType, d.ArtifactID, inputRecordID, d.SupportLinks)
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
