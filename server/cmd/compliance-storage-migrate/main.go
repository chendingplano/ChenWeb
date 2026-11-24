// Command compliance-storage-migrate moves legacy Compliance Review findings to normalized
// storage (openspec compliance-review-storage; KnowledgeStore design 2026112301).
//
//	mise exec -- go run ./server/cmd/compliance-storage-migrate -runs 116 -dry-run
//	mise exec -- go run ./server/cmd/compliance-storage-migrate -runs 115,116
//	mise exec -- go run ./server/cmd/compliance-storage-migrate -all
//	mise exec -- go run ./server/cmd/compliance-storage-migrate -strip -runs 116 -recovery-dir ~/Apps/PGBackup/compliance-storage
//	mise exec -- go run ./server/cmd/compliance-storage-migrate -restore ~/Apps/PGBackup/compliance-storage/<file>.jsonl.gz
//
// Migration writes each finding's records and switches it to storage version 2 in one
// transaction, after validating them against its legacy payload; the payload stays. -strip
// is the separate last step: it writes a recovery copy of every migrated finding's row, then
// removes the duplicated legacy payload; -restore writes such a copy back. Exit codes: 0 success, 1 a finding failed or an
// error occurred, 2 usage error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	complianceeval "github.com/chendingplano/deepdoc/server/api/compliance-eval"
	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
)

func main() {
	runs := flag.String("runs", "", "comma-separated doc-review run ids (kb.doc_review_runs.id)")
	all := flag.Bool("all", false, "every run that still has legacy compliance findings")
	batch := flag.Int("batch", 50, "findings loaded per query")
	dryRun := flag.Bool("dry-run", false, "validate and write inside transactions that are rolled back")
	strip := flag.Bool("strip", false, "remove the legacy payload of migrated findings after writing a recovery copy")
	recoveryDir := flag.String("recovery-dir", "", "directory for the -strip recovery copy (required with -strip)")
	restore := flag.String("restore", "", "recovery file whose legacy payload to write back (undoes -strip)")
	flag.Parse()
	if *restore != "" {
		if *runs != "" || *all || *strip || *dryRun {
			usage("-restore takes no other option")
		}
		if err := restoreFile(*restore); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	ids, err := runIDs(*runs)
	switch {
	case err != nil:
		usage(err.Error())
	case len(ids) == 0 && !*all:
		usage("give -runs or -all")
	case len(ids) > 0 && *all:
		usage("give -runs or -all, not both")
	case *strip && strings.TrimSpace(*recoveryDir) == "":
		usage("-strip needs -recovery-dir")
	case *strip && *dryRun:
		usage("-strip has no dry run; run the migration with -dry-run instead")
	}
	if err := run(ids, *batch, *dryRun, *strip, *recoveryDir); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage(msg string) {
	fmt.Fprintln(os.Stderr, "usage error:", msg)
	flag.Usage()
	os.Exit(2)
}

func runIDs(s string) ([]int64, error) {
	var out []int64
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("bad run id %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func restoreFile(path string) error {
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	n, err := docreviews.RestoreStrippedComplianceFindings(context.Background(), db, path)
	fmt.Printf("restored %d finding(s)\n", n)
	return err
}

func run(ids []int64, batch int, dryRun, strip bool, recoveryDir string) error {
	ctx := context.Background()
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	if strip {
		path, n, err := docreviews.StripMigratedComplianceFindings(ctx, db, ids, recoveryDir)
		if path != "" {
			fmt.Printf("recovery copy: %s\n", path)
		}
		fmt.Printf("stripped %d finding(s)\n", n)
		return err
	}
	rep, err := docreviews.MigrateComplianceFindings(ctx, db, docreviews.ComplianceMigrationOptions{RunIDs: ids, Batch: batch, DryRun: dryRun,
		Logf: func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }})
	if rep != nil {
		verb := "migrated"
		if dryRun {
			verb = "validated (dry run, nothing written)"
		}
		fmt.Printf("%s %d finding(s); %d left as legacy (unsupported shape); %d failed\n", verb, len(rep.Migrated), len(rep.Unsupported), len(rep.Failed))
		var runs []int64
		for r := range rep.RunFlags {
			runs = append(runs, r)
		}
		sort.Slice(runs, func(i, j int) bool { return runs[i] < runs[j] })
		for _, r := range runs {
			fmt.Printf("run %d: %d review flag(s)\n", r, len(rep.RunFlags[r]))
			for _, f := range rep.RunFlags[r] {
				fmt.Printf("  - %s\n", f)
			}
		}
		if len(rep.Failed) > 0 {
			b, _ := json.MarshalIndent(rep.Failed, "", "  ")
			fmt.Printf("failed findings:\n%s\n", b)
		}
	}
	if err != nil {
		return err
	}
	if len(rep.Failed) > 0 {
		return fmt.Errorf("%d finding(s) failed validation; their legacy payload is unchanged", len(rep.Failed))
	}
	return nil
}
