// Command terminology-import registers governed local terminology sources,
// imports immutable staging snapshots, reports release diffs, and moves the
// audited deployment pointer. It never fetches live URLs; every artifact is
// a local file pinned by checksum in its manifest.
//
// Usage:
//
//	terminology-import import --manifest <manifest.json>
//	terminology-import diff --base <manifest.json> --candidate <manifest.json> [--format json|summary]
//	terminology-import activate --deployment-key <key> --source <source> --release <release> --changed-by <user>
//	terminology-import rollback --deployment-key <key> --changed-by <user>
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
)

type app interface {
	importManifest(ctx context.Context, manifestPath string) (terminology.ImportResult, error)
	diffManifests(ctx context.Context, basePath, candidatePath string) (terminology.ReleaseDiff, error)
	activate(ctx context.Context, deploymentKey, source, release, changedBy string) error
	rollback(ctx context.Context, deploymentKey, changedBy string) error
}

func main() {
	code := execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr, defaultApp{})
	os.Exit(code)
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer, application app) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}
	switch args[0] {
	case "import":
		return runImport(ctx, args[1:], stdout, stderr, application)
	case "diff":
		return runDiff(ctx, args[1:], stdout, stderr, application)
	case "activate":
		return runActivate(ctx, args[1:], stdout, stderr, application)
	case "rollback":
		return runRollback(ctx, args[1:], stdout, stderr, application)
	case "help", "-h", "--help":
		writeUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n", args[0])
		writeUsage(stderr)
		return 2
	}
}

func runImport(ctx context.Context, args []string, stdout, stderr io.Writer, application app) int {
	fs := flag.NewFlagSet("terminology-import import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	manifestPath := fs.String("manifest", "", "manifest JSON for one immutable source release")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *manifestPath == "" {
		fmt.Fprintln(stderr, "--manifest is required")
		return 2
	}
	result, err := application.importManifest(ctx, *manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "import: %v\n", err)
		return 3
	}
	return writeJSON(stdout, stderr, result)
}

func runDiff(ctx context.Context, args []string, stdout, stderr io.Writer, application app) int {
	fs := flag.NewFlagSet("terminology-import diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	basePath := fs.String("base", "", "base release manifest")
	candidatePath := fs.String("candidate", "", "candidate release manifest")
	format := fs.String("format", "json", "output format: json|summary")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *basePath == "" || *candidatePath == "" {
		fmt.Fprintln(stderr, "--base and --candidate are required")
		return 2
	}
	if *format != "json" && *format != "summary" {
		fmt.Fprintln(stderr, "--format must be json or summary")
		return 2
	}
	diff, err := application.diffManifests(ctx, *basePath, *candidatePath)
	if err != nil {
		fmt.Fprintf(stderr, "diff: %v\n", err)
		return 3
	}
	if *format == "json" {
		return writeJSON(stdout, stderr, diff)
	}
	writeDiffSummary(stdout, diff)
	return 0
}

func runActivate(ctx context.Context, args []string, stdout, stderr io.Writer, application app) int {
	fs := flag.NewFlagSet("terminology-import activate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	deploymentKey := fs.String("deployment-key", "", "deployment pointer key")
	source := fs.String("source", "", "source to activate")
	release := fs.String("release", "", "release to activate")
	changedBy := fs.String("changed-by", "", "operator identity for the audit row")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *deploymentKey == "" || *source == "" || *release == "" || *changedBy == "" {
		fmt.Fprintln(stderr, "--deployment-key, --source, --release, and --changed-by are required")
		return 2
	}
	if err := application.activate(ctx, *deploymentKey, *source, *release, *changedBy); err != nil {
		fmt.Fprintf(stderr, "activate: %v\n", err)
		return 3
	}
	return writeJSON(stdout, stderr, map[string]string{
		"deployment_key": *deploymentKey, "action": "activate", "source": *source, "release": *release, "changed_by": *changedBy,
	})
}

func runRollback(ctx context.Context, args []string, stdout, stderr io.Writer, application app) int {
	fs := flag.NewFlagSet("terminology-import rollback", flag.ContinueOnError)
	fs.SetOutput(stderr)
	deploymentKey := fs.String("deployment-key", "", "deployment pointer key")
	changedBy := fs.String("changed-by", "", "operator identity for the audit row")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *deploymentKey == "" || *changedBy == "" {
		fmt.Fprintln(stderr, "--deployment-key and --changed-by are required")
		return 2
	}
	if err := application.rollback(ctx, *deploymentKey, *changedBy); err != nil {
		fmt.Fprintf(stderr, "rollback: %v\n", err)
		return 3
	}
	return writeJSON(stdout, stderr, map[string]string{
		"deployment_key": *deploymentKey, "action": "rollback", "changed_by": *changedBy,
	})
}

func writeJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "write output: %v\n", err)
		return 3
	}
	return 0
}

func writeDiffSummary(w io.Writer, diff terminology.ReleaseDiff) {
	fmt.Fprintf(w, "release diff: %s %s -> %s\n", diff.Source, diff.BaseRelease, diff.CandidateRelease)
	fmt.Fprintf(w, "entries: +%d -%d ~%d\n", len(diff.AddedEntries), len(diff.RetiredEntries), len(diff.ReplacedEntries))
	fmt.Fprintf(w, "labels: +%d -%d ~%d\n", len(diff.AddedLabels), len(diff.RetiredLabels), len(diff.ChangedLabels))
	fmt.Fprintf(w, "relations: +%d -%d ~%d\n", len(diff.AddedRelations), len(diff.RetiredRelations), len(diff.ChangedRelations))
	fmt.Fprintf(w, "negative decisions: +%d -%d ~%d\n", len(diff.AddedNegativeDecisions), len(diff.RetiredNegativeDecisions), len(diff.ChangedNegativeDecisions))
	fmt.Fprintf(w, "ucum codes: +%d -%d ~%d\n", len(diff.AddedUCUMCodes), len(diff.RetiredUCUMCodes), len(diff.ChangedUCUMCodes))
	fmt.Fprintf(w, "artifacts: +%d -%d ~%d\n", len(diff.AddedArtifacts), len(diff.RetiredArtifacts), len(diff.ChangedArtifacts))
	fmt.Fprintf(w, "policy changes: %d\n", len(diff.PolicyChanges))
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: terminology-import <import|diff|activate|rollback> [flags]")
	fmt.Fprintln(w, "  import --manifest <path>")
	fmt.Fprintln(w, "  diff --base <path> --candidate <path> [--format json|summary]")
	fmt.Fprintln(w, "  activate --deployment-key <key> --source <source> --release <release> --changed-by <user>")
	fmt.Fprintln(w, "  rollback --deployment-key <key> --changed-by <user>")
}

type defaultApp struct{}

func (defaultApp) importManifest(ctx context.Context, manifestPath string) (terminology.ImportResult, error) {
	db := connect()
	defer db.Close()
	return (terminology.Runner{DB: db}).Import(ctx, manifestPath)
}

func (defaultApp) diffManifests(ctx context.Context, basePath, candidatePath string) (terminology.ReleaseDiff, error) {
	baseManifest, baseArtifacts, err := terminology.ParseAndVerifyManifest(basePath)
	if err != nil {
		return terminology.ReleaseDiff{}, fmt.Errorf("base manifest: %w", err)
	}
	candidateManifest, candidateArtifacts, err := terminology.ParseAndVerifyManifest(candidatePath)
	if err != nil {
		return terminology.ReleaseDiff{}, fmt.Errorf("candidate manifest: %w", err)
	}
	baseAdapter, ok := terminology.LookupAdapter(baseManifest.Adapter)
	if !ok {
		return terminology.ReleaseDiff{}, fmt.Errorf("base manifest: unknown adapter %q", baseManifest.Adapter)
	}
	candidateAdapter, ok := terminology.LookupAdapter(candidateManifest.Adapter)
	if !ok {
		return terminology.ReleaseDiff{}, fmt.Errorf("candidate manifest: unknown adapter %q", candidateManifest.Adapter)
	}
	basePolicy := baseManifest.Policy.SourcePolicy()
	candidatePolicy := candidateManifest.Policy.SourcePolicy()
	baseSnapshot, err := baseAdapter.Convert(ctx, basePolicy, baseArtifacts)
	if err != nil {
		return terminology.ReleaseDiff{}, fmt.Errorf("base adapter %s: %w", baseManifest.Adapter, err)
	}
	candidateSnapshot, err := candidateAdapter.Convert(ctx, candidatePolicy, candidateArtifacts)
	if err != nil {
		return terminology.ReleaseDiff{}, fmt.Errorf("candidate adapter %s: %w", candidateManifest.Adapter, err)
	}
	return terminology.DiffReleases(
		terminology.ReleaseContent{Policy: basePolicy, Artifacts: baseArtifacts, Snapshot: baseSnapshot},
		terminology.ReleaseContent{Policy: candidatePolicy, Artifacts: candidateArtifacts, Snapshot: candidateSnapshot},
	), nil
}

func (defaultApp) activate(ctx context.Context, deploymentKey, source, release, changedBy string) error {
	db := connect()
	defer db.Close()
	return (terminology.DeploymentStore{DB: db}).Activate(ctx, deploymentKey, source, release, changedBy)
}

func (defaultApp) rollback(ctx context.Context, deploymentKey, changedBy string) error {
	db := connect()
	defer db.Close()
	return (terminology.DeploymentStore{DB: db}).Rollback(ctx, deploymentKey, changedBy)
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"), envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(3)
	}
	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "ping db: %v\n", err)
		os.Exit(3)
	}
	return db
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
