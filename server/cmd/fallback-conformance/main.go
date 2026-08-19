// Command fallback-conformance is the ADR 2026081801 Phase 4 task 7.2 and
// task 7.3 check. It:
//
//  1. registers a generic DR13 fallback adapter (semantic.FallbackAdapter)
//     for every family with a registered normalizer (assertions seam 5) that
//     has no compliant semantic-instance adapter of its own yet, then runs
//     the shared adapter conformance suite for each and records the result
//     in kb.semantic_adapter_compliance with writer mode "fallback" (task
//     7.2); and
//  2. for every family this command knows an artifact-source query for (see
//     artifactSourceSQL below), runs the same completeness projection
//     metric-writer-readiness runs, explicitly reporting how many of that
//     family's current artifacts have neither a compliant assertion path nor
//     an active unresolved occurrence -- i.e. are silently skipped today,
//     the same way metrics were before this ADR (task 7.3's "explicitly
//     report" half; its backfill half is deliberately deprioritized, see
//     "Note on backfills" in foundation-shadow-confirmation.md).
//
// This never enables LOSSLESS_SEMANTIC_FALLBACK_WRITES or any other writer
// gate, and it never calls semantic.OccurrenceStore -- no
// kb.unresolved_semantic_occurrences row is written and no kb.provisions (or
// other family) row is mutated. Recording a compliance result is a safe,
// additive write: it never activates a writer by itself
// (AuthorizeWriterActivation still requires the gate to be on).
//
// Usage:
//
//	PG_DB_NAME=miner fallback-conformance
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

func main() {
	log.SetFlags(0)
	db := connect()
	defer db.Close()
	ctx := context.Background()

	wired := assertions.EnsureGenericFallbackAdapters()
	if len(wired) == 0 {
		fmt.Println("no registered family needs the generic fallback adapter (every family already has a compliant adapter)")
	}

	allPassed := true
	for _, family := range wired {
		adapter, ok := semantic.LookupAdapter(family)
		if !ok {
			log.Fatalf("no adapter registered for family %q after wiring", family)
		}
		conformance, err := semantic.VerifyAndRecord(ctx, db, adapter, semantic.WriterFallback)
		if err != nil {
			log.Fatalf("run and record conformance suite for %q: %v", family, err)
		}
		fmt.Printf("%s: conformance suite %s: passed=%v\n", family, conformance.SuiteVersion, conformance.Passed)
		for _, failure := range conformance.Failures {
			fmt.Printf("  FAIL: %s\n", failure)
		}
		allPassed = allPassed && conformance.Passed
	}

	fmt.Println("\nLOSSLESS_SEMANTIC_FALLBACK_WRITES was not read or modified by this command.")

	fmt.Println("\n--- task 7.3: historical-skip coverage (report only, no data mutation) ---")
	for _, family := range wired {
		src, ok := artifactSourceSQL[family]
		if !ok {
			fmt.Printf("%s: no artifact-source query registered for this command yet; skipping coverage report\n", family)
			continue
		}
		adapter, _ := semantic.LookupAdapter(family)
		rep, err := semantic.CompletenessChecker{DB: db, ArtifactSourceSQL: src}.Run(ctx, adapter)
		if err != nil {
			log.Fatalf("run completeness projection for %q: %v", family, err)
		}
		fmt.Printf("%s: current artifacts=%d, with neither an assertion path nor an active unresolved occurrence=%d\n",
			family, rep.CurrentArtifacts, rep.ArtifactsWithNeitherPath)
	}

	if !allPassed {
		os.Exit(1)
	}
}

// artifactSourceSQL is, per family, the same kind of current-occurrence query
// semantic.MetricArtifactSourceSQL is for metrics: one row per current
// artifact, selecting (artifact_id, input_record_id). Only families this
// command knows how to query are listed; a newly-wired family with no entry
// here is reported as skipped rather than guessed at.
var artifactSourceSQL = map[string]string{
	"provision": `
SELECT p.prov_id AS artifact_id, p.input_record_id AS input_record_id
FROM kb.provisions p
WHERE p.prov_id IS NOT NULL AND btrim(p.prov_id) <> ''`,
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
