// Command compliance-snapshot records and verifies evaluation snapshots for the Compliance
// Review gold set (openspec compliance-reviewer-phase2).
//
//	mise exec -- go run ./server/cmd/compliance-snapshot record -name gold-1 -ids 416,371 [-notes ...]
//	mise exec -- go run ./server/cmd/compliance-snapshot verify -id 1
//
// verify exits 1 and names every record that differs from the manifest.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	_ "github.com/lib/pq"

	complianceeval "github.com/chendingplano/deepdoc/server/api/compliance-eval"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	name := fs.String("name", "", "snapshot name (record)")
	ids := fs.String("ids", "", "comma-separated record ids (record)")
	notes := fs.String("notes", "", "free-text notes (record)")
	id := fs.Int64("id", 0, "snapshot id (verify)")
	fs.Parse(args)

	db := connect()
	defer db.Close()
	ctx := context.Background()
	model := os.Getenv("EMBEDDING_MODEL_NAME")
	switch cmd {
	case "record":
		recs, err := parseIDs(*ids)
		if err != nil {
			fatal(err)
		}
		snap, err := complianceeval.Record(ctx, db, *name, model, *notes, recs)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("snapshot %d %q recorded: %d records, embedding model %s\n", snap, *name, len(recs), model)
	case "verify":
		diffs, err := complianceeval.Verify(ctx, db, *id, model)
		if err != nil {
			fatal(err)
		}
		if len(diffs) == 0 {
			fmt.Printf("snapshot %d still matches the corpus\n", *id)
			return
		}
		for _, d := range diffs {
			fmt.Println(d)
		}
		os.Exit(1)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: compliance-snapshot record -name N -ids 1,2 [-notes T] | verify -id N")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func parseIDs(s string) ([]int64, error) {
	var out []int64
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad record id %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func connect() *sql.DB {
	user := os.Getenv("PG_USER_NAME")
	if user == "" {
		user = "cding"
	}
	host := os.Getenv("PG_HOST")
	if host == "" {
		host = "/tmp"
	}
	port := os.Getenv("PG_PORT")
	if port == "" {
		port = "5432"
	}
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable", host, port, user, os.Getenv("PG_DB_NAME")))
	if err != nil || db.Ping() != nil {
		fatal(fmt.Errorf("cannot connect to the database (PG_DB_NAME=%q)", os.Getenv("PG_DB_NAME")))
	}
	return db
}
