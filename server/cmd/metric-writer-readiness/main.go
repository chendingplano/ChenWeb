// Command metric-writer-readiness is the ADR 2026081801 Phase 3 task 6.9
// precondition check for enabling LOSSLESS_SEMANTIC_WRITES_METRIC. It:
//
//  1. runs the shared adapter conformance suite for the metric adapter and
//     records the result in kb.semantic_adapter_compliance (DR13) -- this is
//     a real, safe write: recording a compliance result never activates a
//     writer by itself;
//  2. runs the completeness projection against the real corpus; and
//  3. calls AuthorizeWriterActivation as if the gate were enabled, to report
//     whether activation WOULD be authorized.
//
// It never sets LOSSLESS_SEMANTIC_WRITES_METRIC itself. Flipping that gate is
// a separate, deliberate operational decision this command only informs.
//
// Usage:
//
//	PG_DB_NAME=miner metric-writer-readiness
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

func main() {
	log.SetFlags(0)
	db := connect()
	defer db.Close()
	ctx := context.Background()

	adapter, ok := semantic.LookupAdapter(semantic.MetricArtifactType)
	if !ok {
		log.Fatal("metric adapter is not registered")
	}

	conformance, err := semantic.VerifyAndRecord(ctx, db, adapter, semantic.WriterShadow)
	if err != nil {
		log.Fatalf("run and record conformance suite: %v", err)
	}
	fmt.Printf("conformance suite %s: passed=%v\n", conformance.SuiteVersion, conformance.Passed)
	for _, failure := range conformance.Failures {
		fmt.Printf("  FAIL: %s\n", failure)
	}

	completeness, err := semantic.CompletenessChecker{DB: db, ArtifactSourceSQL: semantic.MetricArtifactSourceSQL}.Run(ctx, adapter)
	if err != nil {
		log.Fatalf("run completeness projection: %v", err)
	}
	fmt.Printf("\ncompleteness projection: complete=%v\n", completeness.Complete())
	fmt.Printf("  current artifacts:            %d\n", completeness.CurrentArtifacts)
	fmt.Printf("  missing stage outcomes:        %d\n", completeness.MissingStageOutcomes)
	fmt.Printf("  artifacts missing any stage:   %d\n", completeness.ArtifactsMissingAnyStage)
	fmt.Printf("  artifacts with neither path:   %d\n", completeness.ArtifactsWithNeitherPath)
	fmt.Printf("  summary drift:                 %d\n", completeness.SummaryDrift)
	fmt.Printf("  orphan active findings:        %d\n", completeness.OrphanActiveFindings)
	fmt.Printf("  assertions missing value state: %d\n", completeness.AssertionsMissingValueState)
	fmt.Printf("  artifacts with missing value:  %d\n", completeness.ArtifactsWithMissingValue)
	for _, reason := range completeness.BlockingReasons() {
		fmt.Printf("  BLOCKING: %s\n", reason)
	}

	// AuthorizeWriterActivation as if the gate were on -- reports what WOULD
	// happen without setting anything.
	authErr := semantic.AuthorizeWriterActivation(ctx, db, semantic.MetricArtifactType, true)
	fmt.Println()
	if authErr == nil {
		fmt.Println("writer activation: WOULD BE AUTHORIZED (conformance + compliance registry checks pass)")
	} else {
		fmt.Printf("writer activation: WOULD BE REFUSED: %v\n", authErr)
	}

	fmt.Println("\nLOSSLESS_SEMANTIC_WRITES_METRIC was not read or modified by this command.")

	if !conformance.Passed || authErr != nil {
		os.Exit(1)
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
