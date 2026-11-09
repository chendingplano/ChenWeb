package docprocessing

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Run with OBJECT_TYPES_TEST_DSN pointing at an isolated PostgreSQL database.
// The test refuses a database with an existing kb schema, creates its own minimal
// fixtures, and drops only the schema it created. Never point this at production.
func TestObjectTypesPostgresMigration(t *testing.T) {
	dsn := os.Getenv("OBJECT_TYPES_TEST_DSN")
	if dsn == "" {
		t.Skip("set OBJECT_TYPES_TEST_DSN to an isolated PostgreSQL database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("open isolated PostgreSQL failed")
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT FROM pg_namespace WHERE nspname = 'kb')`).Scan(&exists); err != nil {
		t.Fatal("cannot inspect isolated PostgreSQL schema")
	}
	if exists {
		t.Fatal("refusing to use a database with an existing kb schema")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("isolated PostgreSQL statement failed: %v", err)
		}
	}
	exec(`CREATE SCHEMA kb`)
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, `DROP SCHEMA kb CASCADE`); err != nil {
			t.Errorf("clean up isolated schema: %v", err)
		}
	})
	exec(`CREATE TABLE kb.artifact_objects (
		id BIGSERIAL PRIMARY KEY, object_type TEXT, ext_info JSONB DEFAULT '{}'::jsonb,
		source_record_id BIGINT, input_record_id BIGINT, artifact_type TEXT,
		source_line_spans JSONB DEFAULT '[]'::jsonb
	);
	CREATE TABLE kb.object_nodes (id BIGSERIAL PRIMARY KEY, object_type TEXT, ext_info JSONB DEFAULT '{}'::jsonb)`)
	exec(`INSERT INTO kb.artifact_objects (object_type,source_record_id,input_record_id,artifact_type,source_line_spans)
	VALUES ('eq',224,224,'provision','["42"]'),('Novel Device',224,224,'metric','["43"]');
	INSERT INTO kb.object_nodes (object_type) VALUES ('Node Class')`)
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "project_migrations", "20261108000301_object_type_registry.sql"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(migration), "-- +goose Down")
	if len(parts) != 2 {
		t.Fatal("migration must contain one Up and one Down")
	}
	up, down := parts[0], parts[1]
	exec(up)
	var approved int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.object_types WHERE status='approved'`).Scan(&approved); err != nil {
		t.Fatal(err)
	}
	if approved != 10 {
		t.Fatalf("approved seeds=%d, want 10", approved)
	}
	var canonical, raw, status string
	if err := db.QueryRowContext(ctx, `SELECT object_type,ext_info->>'object_type_raw' FROM kb.artifact_objects WHERE id=1`).Scan(&canonical, &raw); err != nil {
		t.Fatal(err)
	}
	if canonical != "equipment" || raw != "eq" {
		t.Fatalf("legacy code canonical=%q raw=%q", canonical, raw)
	}
	for _, value := range []string{" New Gauge ", "New-Gauge", "NEW_GAUGE", "new/gauge", "new.gauge", "new\t gauge"} {
		if err := db.QueryRowContext(ctx, `SELECT kb.normalize_object_type($1)`, value).Scan(&canonical); err != nil {
			t.Fatal(err)
		}
		if canonical != normalizeObjectToken(value) {
			t.Fatalf("SQL/Go normalization differs for %q: SQL=%q Go=%q", value, canonical, normalizeObjectToken(value))
		}
	}
	for _, name := range []string{"novel_device", "node_class"} {
		if err := db.QueryRowContext(ctx, `SELECT status FROM kb.object_types WHERE name=$1`, name).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "proposed" {
			t.Fatalf("backfilled %q status=%q", name, status)
		}
	}
	var backfillRecord int64
	var backfillSpans string
	if err := db.QueryRowContext(ctx, `SELECT raw_value,source_record_id,source_spans::text FROM kb.object_types WHERE name='novel_device'`).Scan(&raw, &backfillRecord, &backfillSpans); err != nil {
		t.Fatal(err)
	}
	if raw != "Novel Device" || backfillRecord != 224 || backfillSpans != `["43"]` {
		t.Fatalf("backfill provenance raw=%q record=%d spans=%s", raw, backfillRecord, backfillSpans)
	}
	// An unfamiliar artifact type keeps first-observed evidence for review.
	exec(`INSERT INTO kb.artifact_objects (object_type,source_record_id,input_record_id,artifact_type,source_line_spans)
	VALUES ('New Gauge',225,225,'provision','["91-92"]')`)
	var recordID int64
	var processor, spans string
	if err := db.QueryRowContext(ctx, `SELECT status,raw_value,source_record_id,source_processor,source_spans::text FROM kb.object_types WHERE name='new_gauge'`).Scan(&status, &raw, &recordID, &processor, &spans); err != nil {
		t.Fatal(err)
	}
	if status != "proposed" || raw != "New Gauge" || recordID != 225 || processor != "provision" || spans != `["91-92"]` {
		t.Fatalf("new type did not preserve provenance: status=%q raw=%q record=%d processor=%q spans=%s", status, raw, recordID, processor, spans)
	}
	// The object-node trigger must independently discover and canonicalize.
	exec(`INSERT INTO kb.object_nodes (object_type) VALUES ('Fresh Node Type'),('eq')`)
	if err := db.QueryRowContext(ctx, `SELECT status FROM kb.object_types WHERE name='fresh_node_type'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "proposed" {
		t.Fatalf("new node status=%q", status)
	}
	if err := db.QueryRowContext(ctx, `SELECT object_type FROM kb.object_nodes WHERE id=3`).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	if canonical != "equipment" {
		t.Fatalf("node alias canonical=%q", canonical)
	}
	// Changing a stored type must not attribute it to the previous type's raw value.
	exec(`UPDATE kb.object_nodes SET object_type='Updated Node Type' WHERE id=3`)
	if err := db.QueryRowContext(ctx, `SELECT raw_value FROM kb.object_types WHERE name='updated_node_type'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "Updated Node Type" {
		t.Fatalf("updated type retained stale raw provenance=%q", raw)
	}
	exec(`UPDATE kb.object_nodes SET object_type='Explicit Type',ext_info=jsonb_build_object('object_type_raw','EXPLICIT TYPE FROM MODEL') WHERE id=3`)
	if err := db.QueryRowContext(ctx, `SELECT raw_value FROM kb.object_types WHERE name='explicit_type'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "EXPLICIT TYPE FROM MODEL" {
		t.Fatalf("explicit raw provenance lost=%q", raw)
	}
	// Canonical nodes are created before artifact rows by generic processors.
	// Their first observation must retain the originating record and evidence.
	exec(`INSERT INTO kb.object_nodes (object_type,ext_info) VALUES ('sensing_device',
	'{"object_type_raw":"Sensing Device","source_record_id":228,"source_run_id":174,"source_processor":"metric","source_line_spans":["64-65"]}'::jsonb)`)
	var runID int64
	if err := db.QueryRowContext(ctx, `SELECT raw_value,source_record_id,source_run_id,source_processor,source_spans::text FROM kb.object_types WHERE name='sensing_device'`).Scan(&raw, &recordID, &runID, &processor, &spans); err != nil {
		t.Fatal(err)
	}
	if raw != "Sensing Device" || recordID != 228 || runID != 174 || processor != "metric" || spans != `["64-65"]` {
		t.Fatalf("node-origin proposal lost provenance: raw=%q record=%d run=%d processor=%q spans=%s", raw, recordID, runID, processor, spans)
	}
	exec(`INSERT INTO kb.artifact_objects (object_type,source_record_id,input_record_id,artifact_type) VALUES ('Fallback Type',0,229,'metric')`)
	if err := db.QueryRowContext(ctx, `SELECT source_record_id FROM kb.object_types WHERE name='fallback_type'`).Scan(&recordID); err != nil {
		t.Fatal(err)
	}
	if recordID != 229 {
		t.Fatalf("zero source record did not fall back to input record: %d", recordID)
	}
	if err := db.QueryRowContext(ctx, `SELECT source_line_spans::text FROM kb.artifact_objects WHERE id=1`).Scan(&spans); err != nil {
		t.Fatal(err)
	}
	if spans != `["42"]` {
		t.Fatalf("canonicalization changed evidence spans=%s", spans)
	}
	// Repeated discovery must preserve approval and first reviewer attribution.
	store := ObjectTypeSQLStore{DB: db}
	entry, err := store.Resolve(ctx, "New Gauge", ObjectTypeSource{})
	if err != nil {
		t.Fatal(err)
	}
	approvedEntry, err := store.Approve(ctx, entry.ID, "reviewer-one")
	if err != nil {
		t.Fatal(err)
	}
	if approvedEntry.Status != "approved" || approvedEntry.ApprovedBy != "reviewer-one" || approvedEntry.ApprovedAt == nil {
		t.Fatalf("approval=%+v", approvedEntry)
	}
	repeated, err := store.Approve(ctx, entry.ID, "reviewer-two")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ApprovedBy != "reviewer-one" || !repeated.ApprovedAt.Equal(*approvedEntry.ApprovedAt) {
		t.Fatalf("repeat approval changed audit=%+v", repeated)
	}
	exec(`INSERT INTO kb.artifact_objects (object_type,source_record_id,artifact_type) VALUES ('New Gauge',226,'inventory_item')`)
	entry, err = store.Resolve(ctx, "new_gauge", ObjectTypeSource{RecordID: 227})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Status != "approved" || entry.SourceRecordID != 225 || entry.ApprovedBy != "reviewer-one" {
		t.Fatalf("rediscovery changed governance/provenance=%+v", entry)
	}
	// Independent concurrent writers converge on one proposal, including triggers.
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, err := store.Resolve(ctx, "Concurrent Gauge", ObjectTypeSource{RecordID: int64(300 + i)})
				errs <- err
			} else {
				_, err := db.ExecContext(ctx, `INSERT INTO kb.artifact_objects (object_type,source_record_id,artifact_type) VALUES ('Concurrent Gauge',$1,'metric')`, 300+i)
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.object_types WHERE name='concurrent_gauge' AND status='proposed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent proposals=%d, want one", count)
	}
	// The registry and trigger migration can roll back and apply again with data.
	exec(down)
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('kb.object_types') IS NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("Down left object_types behind")
	}
	exec(up)
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.object_types WHERE status='approved'`).Scan(&approved); err != nil {
		t.Fatal(err)
	}
	if approved != 10 {
		t.Fatalf("reapplied seeds=%d, want ten", approved)
	}
	exec(`INSERT INTO kb.object_nodes (object_type) VALUES ('After Rollback')`)
	if err := db.QueryRowContext(ctx, `SELECT status FROM kb.object_types WHERE name='after_rollback'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "proposed" {
		t.Fatalf("reapplied trigger status=%q", status)
	}
}
