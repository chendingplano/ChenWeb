// Command terminology-coverage measures a selected keyword corpus against a
// reviewed seed release. It is read-only and reports readiness; it does not
// record or grant operator approval.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
)

type coverageLoader func(context.Context, terminology.CoverageQuery) (terminology.CorpusData, error)

func main() {
	code := execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr, loadFromDatabase)
	os.Exit(code)
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer, load coverageLoader) int {
	fs := flag.NewFlagSet("terminology-coverage", flag.ContinueOnError)
	fs.SetOutput(stderr)
	acceptancePath := fs.String("acceptance", "", "operator-authored acceptance JSON")
	format := fs.String("format", "json", "output format: json|summary")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *acceptancePath == "" {
		fmt.Fprintln(stderr, "--acceptance is required")
		return 2
	}
	if *format != "json" && *format != "summary" {
		fmt.Fprintln(stderr, "--format must be json or summary")
		return 2
	}
	acceptance, err := readAcceptance(*acceptancePath)
	if err != nil {
		fmt.Fprintf(stderr, "acceptance: %v\n", err)
		return 2
	}
	if load == nil {
		fmt.Fprintln(stderr, "coverage reader is not configured")
		return 3
	}
	data, err := load(ctx, terminology.CoverageQuery{
		Scope: acceptance.Scope, Corpus: acceptance.Corpus, TargetSeedRelease: acceptance.TargetSeedRelease,
	})
	if err != nil {
		fmt.Fprintf(stderr, "load coverage data: %v\n", err)
		return 3
	}
	report, err := terminology.BuildCoverage(acceptance, data)
	if err != nil {
		fmt.Fprintf(stderr, "calculate coverage: %v\n", err)
		return 3
	}
	if *format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "write report: %v\n", err)
			return 3
		}
	} else {
		writeSummary(stdout, report)
	}
	if !report.Ready {
		return 1
	}
	return 0
}

func readAcceptance(path string) (terminology.Acceptance, error) {
	f, err := os.Open(path)
	if err != nil {
		return terminology.Acceptance{}, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	var acceptance terminology.Acceptance
	if err := decoder.Decode(&acceptance); err != nil {
		return terminology.Acceptance{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return terminology.Acceptance{}, fmt.Errorf("multiple JSON values")
		}
		return terminology.Acceptance{}, err
	}
	if err := acceptance.Validate(); err != nil {
		return terminology.Acceptance{}, err
	}
	return acceptance, nil
}

func writeSummary(w io.Writer, report terminology.CoverageReport) {
	status := "NOT READY"
	if report.Ready {
		status = "READY"
	}
	riskStatus := "MET"
	if !report.RiskTermsMet {
		riskStatus = "NOT MET"
	}
	fmt.Fprintf(w, "terminology coverage: %s\n", status)
	fmt.Fprintf(w, "scope: %s\n", report.Scope)
	fmt.Fprintf(w, "seed release: %s@%s\n", report.TargetSeedRelease.Source, report.TargetSeedRelease.Release)
	fmt.Fprintf(w, "coverage: %.2f%% (%d/%d eligible occurrences; target %.2f%%)\n", report.Coverage*100, report.CoveredFrequency, report.EligibleFrequency, report.TargetCoverage*100)
	fmt.Fprintf(w, "risk terms: %s", riskStatus)
	if len(report.UncoveredRiskTerms) > 0 {
		fmt.Fprintf(w, " (%s)", strings.Join(report.UncoveredRiskTerms, ", "))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "unresolved bilingual pairs: %d\n", len(report.UnresolvedBilingualPairs))
	fmt.Fprintf(w, "context-sensitive surfaces: %d\n", len(report.ContextSensitiveSurfaces))
	fmt.Fprintf(w, "high-frequency uncovered concepts: %d\n", len(report.HighFrequencyUncovered))
	fmt.Fprintf(w, "approval: %s (approver: %s)\n", report.Approval, report.Approver)
}

func loadFromDatabase(ctx context.Context, query terminology.CoverageQuery) (terminology.CorpusData, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"), envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return terminology.CorpusData{}, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return terminology.CorpusData{}, fmt.Errorf("ping database: %w", err)
	}
	return (terminology.SQLReader{DB: db}).Load(ctx, query)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
