package kbhandler

import (
	"database/sql"
	"os"
	"testing"
)

// A dropped metric keeps a foreign key to the extract_metrics run that set it aside. Deleting the
// record must remove those rows before the runs themselves, or PostgreSQL rejects the delete with
// metrics_dropped_run_id_fkey. Same disposable-database contract as TestDeleteInputPostgres.
func TestDeleteInputRemovesMetricsDroppedBeforeRuns(t *testing.T) {
	dsn := os.Getenv("KB_INPUT_DELETE_TEST_DSN")
	if dsn == "" {
		t.Skip("set KB_INPUT_DELETE_TEST_DSN to an empty disposable PostgreSQL database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'kb')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("integration test requires a disposable database without a kb schema")
	}
	if _, err := db.Exec(`CREATE SCHEMA kb`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(`DROP SCHEMA kb CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	for _, stmt := range []string{
		`CREATE TABLE kb.doc_process_runs (id bigserial PRIMARY KEY, record_id bigint NOT NULL, event_id text)`,
		`CREATE TABLE kb.doc_proc_logs (id bigserial PRIMARY KEY, entry_type text, record_id bigint, run_id bigint REFERENCES kb.doc_process_runs (id))`,
		`CREATE TABLE kb.metrics_dropped (
			id bigserial PRIMARY KEY,
			input_record_id bigint NOT NULL,
			drop_id text NOT NULL UNIQUE,
			run_id bigint REFERENCES kb.doc_process_runs (id))`,
		`INSERT INTO kb.doc_process_runs (record_id, event_id) VALUES (430, 'e1')`,
		`INSERT INTO kb.doc_proc_logs (entry_type, record_id, run_id) VALUES ('extract_metrics', 430, 1)`,
		`INSERT INTO kb.metrics_dropped (input_record_id, drop_id, run_id) VALUES (430, '430_drp_1', 1), (430, '430_drp_2', NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := deleteInputRelatedRows(tx, 430); err != nil {
		t.Fatalf("deleteInputRelatedRows returned error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	for _, table := range []string{"kb.metrics_dropped", "kb.doc_process_runs", "kb.doc_proc_logs"} {
		var remaining int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if remaining != 0 {
			t.Fatalf("%s still has %d rows for the deleted record", table, remaining)
		}
	}
}
