// Command metric-contract-backfill re-evaluates metric assertion conformance
// for occurrences written before their class's contract left identity_only
// (metric-class-contracts change, task 8). It always reports every affected
// assertion; with --apply it also updates conformance_state_term_id in
// place via classfoundation.MetricContractBackfill, never touching any
// other field.
//
// Usage:
//
//	PG_DB_NAME=miner metric-contract-backfill            # report only
//	PG_DB_NAME=miner metric-contract-backfill --apply     # re-evaluate
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
	apply := flag.Bool("apply", false, "re-evaluate stale conformance states (default: report only)")
	flag.Parse()

	db := connect()
	defer db.Close()

	ctx := context.Background()
	backfill := classfoundation.MetricContractBackfill{DB: db}

	stale, err := backfill.Report(ctx)
	if err != nil {
		log.Fatalf("report stale conformance: %v", err)
	}
	printStale(stale)

	if !*apply {
		if len(stale) > 0 {
			fmt.Printf("\n%d assertion(s) have a stale conformance state; re-run with --apply to re-evaluate.\n", len(stale))
		} else {
			fmt.Println("\nno stale conformance states found.")
		}
		return
	}

	updated, err := backfill.Reevaluate(ctx)
	if err != nil {
		log.Fatalf("reevaluate: %v", err)
	}
	fmt.Printf("\nre-evaluated %d assertion(s)\n", updated)

	after, err := backfill.Report(ctx)
	if err != nil {
		log.Fatalf("report stale conformance after reevaluation: %v", err)
	}
	if len(after) > 0 {
		log.Fatalf("%d assertion(s) still stale after reevaluation", len(after))
	}
}

func printStale(stale []classfoundation.StaleConformanceAssertion) {
	fmt.Printf("%d assertion(s) with a stale conformance state\n", len(stale))
	for _, s := range stale {
		fmt.Printf("  assertion_id=%d class_term_id=%s unit_term_id=%s value_present=%v\n",
			s.AssertionID, s.ClassTermID, s.UnitTermID, s.ValueIsPresent)
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
