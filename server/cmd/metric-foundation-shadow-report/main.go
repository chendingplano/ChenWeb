// Command metric-foundation-shadow-report is the ADR 2026081801 Phase 3 task
// 6.1 confirmation tool. It runs semantic.MetricAdapter.RunShadow across
// every metric-bearing input record in the corpus, aggregates the result,
// and proves no consumer-visible write occurred by diffing row counts on
// every table the ADR 2026081701 foundations (class contracts, class
// resolution, observed profiles, claim identities) and the legacy semantic
// tables could have written to.
//
// Nothing before this command has ever run RunShadow against the real
// corpus -- it previously had zero production callers, only test callers.
//
// Usage:
//
//	PG_DB_NAME=miner metric-foundation-shadow-report
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// writeGuardedTables are every table the ADR 2026081701 foundations or the
// existing semantic-assertion path could write to. A shadow run must leave
// every one of these row counts unchanged.
var writeGuardedTables = []string{
	"kb.ontology_term_headers",
	"kb.ontology_term_revisions",
	"kb.ontology_class_resolution_decisions",
	"kb.ontology_class_resolution_alternatives",
	"kb.ontology_class_contract_revisions",
	"kb.ontology_class_contract_capabilities",
	"kb.ontology_class_capability_validation_results",
	"kb.ontology_observed_class_profiles",
	"kb.ontology_observed_class_attribute_observations",
	"kb.ontology_observed_class_attribute_distributions",
	"kb.ontology_observed_class_profile_examples",
	"kb.ontology_observed_class_profile_exceptions",
	"kb.semantic_canonical_key_versions",
	"kb.semantic_claim_identities",
	"kb.semantic_assertions",
	"kb.assertion_evidence",
}

func main() {
	log.SetFlags(0)

	db := connect()
	defer db.Close()
	ctx := context.Background()

	before, err := tableRowCounts(ctx, db)
	if err != nil {
		log.Fatalf("row counts before: %v", err)
	}

	inputRecordIDs, err := metricBearingInputRecordIDs(ctx, db)
	if err != nil {
		log.Fatalf("list input records: %v", err)
	}

	adapter := semantic.MetricAdapter{}
	var total semantic.ShadowComparison
	total.IntendedFindingsByTerm = map[string]int{}
	failures := 0
	for _, id := range inputRecordIDs {
		cmp, err := adapter.RunShadow(ctx, db, id)
		if err != nil {
			log.Printf("RunShadow input_record_id=%d: %v", id, err)
			failures++
			continue
		}
		total.MetricsExamined += cmp.MetricsExamined
		total.ExistingSupportedMetrics += cmp.ExistingSupportedMetrics
		total.ExistingUnreachableMetrics += cmp.ExistingUnreachableMetrics
		total.WouldBeNormalized += cmp.WouldBeNormalized
		total.WouldBecomeRawPreserved += cmp.WouldBecomeRawPreserved
		total.IntendedOutcomeEnvelopes += cmp.IntendedOutcomeEnvelopes
		total.DuplicateCurrentSupport += cmp.DuplicateCurrentSupport
		total.FoundationClassResolved += cmp.FoundationClassResolved
		total.FoundationClassUnavailable += cmp.FoundationClassUnavailable
		total.FoundationClaimCandidates += cmp.FoundationClaimCandidates
		total.FoundationProfileCandidates += cmp.FoundationProfileCandidates
		for term, count := range cmp.IntendedFindingsByTerm {
			total.IntendedFindingsByTerm[term] += count
		}
	}

	after, err := tableRowCounts(ctx, db)
	if err != nil {
		log.Fatalf("row counts after: %v", err)
	}

	fmt.Printf("input records examined: %d (failures: %d)\n", len(inputRecordIDs), failures)
	fmt.Printf("metrics examined: %d\n", total.MetricsExamined)
	fmt.Printf("existing supported (reachable today): %d\n", total.ExistingSupportedMetrics)
	fmt.Printf("existing unreachable today: %d\n", total.ExistingUnreachableMetrics)
	fmt.Printf("would be normalized: %d\n", total.WouldBeNormalized)
	fmt.Printf("would become raw-preserved: %d\n", total.WouldBecomeRawPreserved)
	fmt.Printf("intended outcome envelopes: %d\n", total.IntendedOutcomeEnvelopes)
	fmt.Printf("duplicate current support remaining: %d\n", total.DuplicateCurrentSupport)
	fmt.Printf("foundation class resolved (stable, existing): %d\n", total.FoundationClassResolved)
	fmt.Printf("foundation class unavailable: %d\n", total.FoundationClassUnavailable)
	fmt.Printf("foundation claim key candidates (not registered -- shadow only): %d\n", total.FoundationClaimCandidates)
	fmt.Printf("foundation profile observation candidates (not persisted -- shadow only): %d\n", total.FoundationProfileCandidates)
	fmt.Println("intended findings by term:")
	terms := make([]string, 0, len(total.IntendedFindingsByTerm))
	for term := range total.IntendedFindingsByTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	for _, term := range terms {
		fmt.Printf("  %s: %d\n", term, total.IntendedFindingsByTerm[term])
	}

	fmt.Println("\nwrite-guard (must be zero-diff on every row):")
	drift := false
	for _, table := range writeGuardedTables {
		diff := after[table] - before[table]
		status := "OK"
		if diff != 0 {
			status = "DRIFT"
			drift = true
		}
		fmt.Printf("  %-55s before=%-8d after=%-8d diff=%-6d %s\n", table, before[table], after[table], diff, status)
	}

	if failures > 0 || drift {
		os.Exit(1)
	}
}

func metricBearingInputRecordIDs(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `
SELECT DISTINCT input_record_id
FROM kb.metrics
WHERE input_record_id IS NOT NULL
ORDER BY input_record_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func tableRowCounts(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	counts := make(map[string]int64, len(writeGuardedTables))
	for _, table := range writeGuardedTables {
		var count int64
		if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = count
	}
	return counts, nil
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
