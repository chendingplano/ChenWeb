// Command ontology-compiler is the DB-native module compiler (ADR DR2,
// storage-revised 2026-07-31): it validates a module's approved content in
// the database, builds an immutable checksummed release, and manages the
// activation pointer. There is no file-based module source; content is
// authored and versioned in the database.
//
// Subcommands:
//
//	validate  --module <id>              run the release gate without writing
//	release   --module <id> --version <v> build and store an immutable release
//	activate  --module <id> --release-id <n>  make a release active
//	rollback  --module <id> --version <v>     re-point activation at an older release
//	modules                                list registered modules
//	active   [--module <id>]               show the active release pointer(s)
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
)

func main() {
	log.SetFlags(0)
	sub := "modules"
	if len(os.Args) > 1 {
		sub = os.Args[1]
	}

	switch sub {
	case "validate":
		runValidate(os.Args[2:])
	case "release":
		runRelease(os.Args[2:])
	case "activate":
		runActivate(os.Args[2:])
	case "rollback":
		runRollback(os.Args[2:])
	case "modules":
		runModules(os.Args[2:])
	case "active":
		runActive(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage())
	default:
		log.Fatalf("unknown subcommand %q\n\n%s", sub, usage())
	}
}

func usage() string {
	return `ontology-compiler — DB-native SemOS ontology module compiler

  validate  --module <id>
  release   --module <id> --version <v> [--by <actor>]
  activate  --module <id> --release-id <n> [--by <actor>]
  rollback  --module <id> --version <v> [--by <actor>]
  modules
  active    [--module <id>]

Database via PG_HOST/PG_PORT/PG_USER/PG_DB_NAME (defaults local socket /tmp, cding, chenweb_test).
`
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"),
		envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func runValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	moduleID := fs.String("module", "", "module id")
	fs.Parse(args)

	db := connect()
	defer db.Close()
	rs := modules.ReleaseStore{DB: db}
	if err := rs.Validate(context.Background(), *moduleID); err != nil {
		log.Fatalf("validate %s: %v", *moduleID, err)
	}
	fmt.Printf("validate %s: OK\n", *moduleID)
}

func runRelease(args []string) {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	moduleID := fs.String("module", "", "module id")
	version := fs.String("version", "", "release version")
	by := fs.String("by", "ontology-compiler", "actor")
	fs.Parse(args)

	db := connect()
	defer db.Close()
	rs := modules.ReleaseStore{DB: db}
	rel, err := rs.CreateRelease(context.Background(), *moduleID, *version, *by)
	if err != nil {
		log.Fatalf("release %s@%s: %v", *moduleID, *version, err)
	}
	fmt.Printf("release %s@%s created (id=%d) checksum=%s\n", rel.ModuleID, rel.Version, rel.ID, rel.ContentChecksum)

	// P5 spec section 8: promote approved proposals at release time.
	promoter := docprocessing.PolicyPromotionStore{DB: db, Audit: policyaudit.SQLStore{DB: db}}
	lister := proposalListAdapter{store: modules.ProposalStore{DB: db}}
	if policyID, err := docprocessing.PromoteModuleReleaseProposals(context.Background(), promoter, lister, rel.ID, rel.ContentChecksum); err != nil {
		log.Printf("warning: proposal promotion failed for release %d: %v", rel.ID, err)
	} else if policyID > 0 {
		fmt.Printf("promoted approved proposals into draft policy %d\n", policyID)
	}
}

func runActivate(args []string) {
	fs := flag.NewFlagSet("activate", flag.ExitOnError)
	moduleID := fs.String("module", "", "module id")
	releaseID := fs.Int64("release-id", 0, "release id")
	by := fs.String("by", "ontology-compiler", "actor")
	fs.Parse(args)

	db := connect()
	defer db.Close()
	rs := modules.ReleaseStore{DB: db}
	active, err := rs.Activate(context.Background(), *moduleID, *releaseID, *by)
	if err != nil {
		log.Fatalf("activate %s release %d: %v", *moduleID, *releaseID, err)
	}
	fmt.Printf("active %s -> release %d (version %s)\n", active.ModuleID, active.ReleaseID, active.ReleaseVersion)
}

func runRollback(args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	moduleID := fs.String("module", "", "module id")
	version := fs.String("version", "", "release version to roll back to")
	by := fs.String("by", "ontology-compiler", "actor")
	fs.Parse(args)

	db := connect()
	defer db.Close()
	rs := modules.ReleaseStore{DB: db}
	active, err := rs.Rollback(context.Background(), *moduleID, *version, *by)
	if err != nil {
		log.Fatalf("rollback %s to %s: %v", *moduleID, *version, err)
	}
	fmt.Printf("active %s -> release %d (version %s)\n", active.ModuleID, active.ReleaseID, active.ReleaseVersion)
}

func runModules(args []string) {
	flag.NewFlagSet("modules", flag.ExitOnError).Parse(args)
	db := connect()
	defer db.Close()
	ms := modules.ModuleStore{DB: db}
	list, err := ms.ListModules(context.Background())
	if err != nil {
		log.Fatalf("list modules: %v", err)
	}
	for _, m := range list {
		fmt.Printf("%s\tdepends_on=%v\tstatus=%s\n", m.ModuleID, m.DependsOn, m.Status)
	}
}

func runActive(args []string) {
	fs := flag.NewFlagSet("active", flag.ExitOnError)
	moduleID := fs.String("module", "", "module id (empty = all)")
	fs.Parse(args)

	db := connect()
	defer db.Close()
	rs := modules.ReleaseStore{DB: db}
	if *moduleID != "" {
		active, err := rs.GetActiveRelease(context.Background(), *moduleID)
		if err != nil {
			log.Fatalf("active %s: %v", *moduleID, err)
		}
		fmt.Printf("%s -> release %d (version %s), activated %s by %s\n",
			active.ModuleID, active.ReleaseID, active.ReleaseVersion, active.ActivatedAt.Format("2006-01-02 15:04:05"), active.ActivatedBy)
		return
	}
	for _, id := range modules.ActiveModuleIDs() {
		active, err := rs.GetActiveRelease(context.Background(), id)
		if err != nil {
			continue
		}
		fmt.Printf("%s -> release %d (version %s)\n", active.ModuleID, active.ReleaseID, active.ReleaseVersion)
	}
}

// proposalListAdapter wraps modules.ProposalStore to satisfy
// docprocessing.ApprovedProposalLister, converting ApplicabilityProposal
// values to PromotedProposal. This avoids the import cycle
// (modules -> profiles -> docprocessing -> modules).
type proposalListAdapter struct {
	store modules.ProposalStore
}

func (a proposalListAdapter) ListApprovedProposals(ctx context.Context, releaseID int64) ([]docprocessing.PromotedProposal, error) {
	proposals, err := a.store.ListApprovedProposals(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	out := make([]docprocessing.PromotedProposal, len(proposals))
	for i, p := range proposals {
		out[i] = docprocessing.PromotedProposal{
			ProposalID:        p.ID,
			Predicate:         p.Predicate,
			PredicateChecksum: p.PredicateChecksum,
		}
	}
	return out, nil
}
