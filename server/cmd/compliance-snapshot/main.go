// Command compliance-snapshot records and verifies evaluation snapshots for the Compliance
// Review gold set (openspec compliance-reviewer-phase2). A snapshot is a version stamp of the whole
// indexed knowledge base: every indexed record's md5, chunk-index generation and index counts.
//
//	mise exec -- go run ./server/cmd/compliance-snapshot record -name kb-20261114 [-notes ...]
//	mise exec -- go run ./server/cmd/compliance-snapshot verify [-id 1]     # default: the latest
//	mise exec -- go run ./server/cmd/compliance-snapshot list
//
// verify exits 1 and names every snapshot record that changed or disappeared; records indexed since
// the snapshot are reported but allowed.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

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
	notes := fs.String("notes", "", "free-text notes (record)")
	id := fs.Int64("id", 0, "snapshot id (verify; default the latest)")
	fs.Parse(args)

	db := connect()
	defer db.Close()
	ctx := context.Background()
	model := os.Getenv("EMBEDDING_MODEL_NAME")
	switch cmd {
	case "record":
		snap, n, err := complianceeval.Record(ctx, db, *name, model, *notes)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("snapshot %d %q recorded: %d indexed records, embedding model %s\n", snap, *name, n, model)
	case "verify":
		if *id == 0 {
			if err := db.QueryRowContext(ctx, `SELECT COALESCE(max(id), 0) FROM testbed.compliance_eval_snapshots`).Scan(id); err != nil {
				fatal(err)
			}
			if *id == 0 {
				fatal(fmt.Errorf("no snapshot recorded yet"))
			}
		}
		diffs, added, err := complianceeval.Verify(ctx, db, *id, model)
		if err != nil {
			fatal(err)
		}
		if len(diffs) == 0 {
			fmt.Printf("snapshot %d still matches the knowledge base (%d record(s) indexed since, allowed)\n", *id, len(added))
			return
		}
		for _, d := range diffs {
			fmt.Println(d)
		}
		os.Exit(1)
	case "list":
		rows, err := db.QueryContext(ctx, `SELECT s.id, s.name, s.embedding_model, to_char(s.created_at, 'YYYY-MM-DD HH24:MI'),
			(SELECT count(*) FROM testbed.compliance_eval_snapshot_records r WHERE r.snapshot_id = s.id)
			FROM testbed.compliance_eval_snapshots s ORDER BY s.id`)
		if err != nil {
			fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var sid, n int64
			var sname, emb, created string
			if err := rows.Scan(&sid, &sname, &emb, &created, &n); err != nil {
				fatal(err)
			}
			fmt.Printf("%d\t%s\t%s\t%d records\t%s\n", sid, created, sname, n, emb)
		}
		if err := rows.Err(); err != nil {
			fatal(err)
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: compliance-snapshot record -name N [-notes T] | verify [-id N] | list")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
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
