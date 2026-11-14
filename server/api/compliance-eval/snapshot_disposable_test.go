package complianceeval

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
)

// This test owns an empty disposable database, never the application database.
func TestDisposableMetricSnapshot(t *testing.T) {
	if os.Getenv("COMPLIANCE_EVAL_DISPOSABLE_INTEGRATION") != "1" {
		t.Skip("requires an empty disposable PostgreSQL database through PG* variables")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE SCHEMA kb; CREATE SCHEMA testbed;
 CREATE TABLE kb.inputs(id bigint PRIMARY KEY, md5 text);
 CREATE TABLE kb.chunk_search_state(input_record_id bigint, generation bigint);
 CREATE TABLE kb.search_artifacts_chunk(input_record_id bigint);
 CREATE TABLE kb.search_artifacts_provision(input_record_id bigint);
 CREATE TABLE kb.metrics(id bigint PRIMARY KEY, metric_value text, provenance jsonb, updated_at timestamptz DEFAULT now());
 CREATE TABLE kb.search_artifacts_metric(artifact_id text, input_record_id bigint, source_row_id bigint, semantic_payload jsonb, embedding text, keywords text[], source_line_spans jsonb, updated_at timestamptz DEFAULT now());
 CREATE TABLE testbed.compliance_eval_snapshots(id bigserial PRIMARY KEY, name text UNIQUE, embedding_model text, notes text);
 CREATE TABLE testbed.compliance_eval_snapshot_records(snapshot_id bigint, input_record_id bigint, md5 text, generation bigint, chunk_count int, provision_count int);
 INSERT INTO kb.inputs VALUES (1,'a'), (2,'b');
 INSERT INTO kb.search_artifacts_chunk VALUES (1);
 INSERT INTO kb.metrics(id, metric_value, provenance) VALUES(20,'10','{"line":3}');
 INSERT INTO kb.search_artifacts_metric(artifact_id,input_record_id,source_row_id,semantic_payload,embedding,keywords,source_line_spans)
 VALUES('m2',2,20,'{"value":10}','[1,0]',ARRAY['temperature'],'[[3,3]]');
 INSERT INTO testbed.compliance_eval_snapshots VALUES(100,'legacy','synthetic-model','');
 INSERT INTO testbed.compliance_eval_snapshot_records VALUES(100,1,'a',0,1,0);`)
	migration, err := os.ReadFile("../../../project_migrations/20261114000005_compliance_metric_snapshots.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down, ok := strings.Cut(string(migration), "-- +goose Down")
	if !ok {
		t.Fatal("missing migration down")
	}
	exec(up)
	ctx := context.Background()
	id, n, err := Record(ctx, db, "metric-inclusive", "synthetic-model", "")
	if err != nil || n != 2 {
		t.Fatalf("record metric-only member: id=%d n=%d err=%v", id, n, err)
	}
	if err := RequireMetricSnapshot(ctx, db, 100); err == nil {
		t.Fatal("legacy snapshot accepted for metric pool")
	}
	if err := RequireMetricSnapshot(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	verify := func(changed bool) {
		t.Helper()
		diffs, _, err := Verify(ctx, db, id, "synthetic-model")
		if err != nil || (len(diffs) > 0) != changed {
			t.Fatalf("verify changed=%v: %v %v", changed, diffs, err)
		}
		if len(diffs) > 0 && !strings.Contains(diffs[0].Reason, "metric fingerprint") {
			t.Fatalf("unexpected diff %v", diffs)
		}
	}
	verify(false)
	for _, update := range []string{
		`UPDATE kb.metrics SET metric_value='11'`,
		`UPDATE kb.metrics SET provenance='{"line":4}'`,
		`UPDATE kb.search_artifacts_metric SET semantic_payload='{"value":11}'`,
		`UPDATE kb.search_artifacts_metric SET embedding='[0,1]'`,
		`UPDATE kb.search_artifacts_metric SET keywords=ARRAY['pressure']`,
		`UPDATE kb.search_artifacts_metric SET source_line_spans='[[4,4]]'`,
	} {
		t.Run(update, func(t *testing.T) {
			// Restore the original rows after checking each independent same-count edit.
			exec("BEGIN")
			exec(update)
			verify(true)
			exec("ROLLBACK")
		})
	}
	exec(`UPDATE kb.search_artifacts_metric SET updated_at=now()+interval '1 day'; UPDATE kb.metrics SET updated_at=now()+interval '1 day'`)
	verify(false)
	exec(`UPDATE kb.metrics SET metric_value='12'`)
	diffs, added, err := Verify(ctx, db, 100, "synthetic-model")
	if err != nil || len(diffs) != 0 || len(added) != 0 {
		t.Fatalf("legacy must ignore metric-only record and edits: %v %v %v", diffs, added, err)
	}
	exec(down)
	exec(up)
}
