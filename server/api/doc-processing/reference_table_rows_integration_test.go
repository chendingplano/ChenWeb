package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

// Opt in using REFERENCE_TABLE_ROWS_TEST_DB=1 and standard PG* connection variables.
// All writes occur in a scratch database, which is dropped after the test.
func referenceTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	if os.Getenv("REFERENCE_TABLE_ROWS_TEST_DB") != "1" {
		t.Skip("REFERENCE_TABLE_ROWS_TEST_DB not set")
	}
	admin, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := fmt.Sprintf("reference_rows_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE " + pq.QuoteIdentifier(name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE " + pq.QuoteIdentifier(name) + " WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	db, err := sql.Open("postgres", "dbname="+name+" sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`
CREATE SCHEMA kb; CREATE SCHEMA testbed; CREATE TABLE testbed.metrics(id BIGINT);
CREATE TABLE kb.inputs (id BIGINT PRIMARY KEY, user_id TEXT, ks_store_id BIGINT);
CREATE TABLE kb.agentic_knowledge_grants (user_id TEXT, active BOOLEAN, expires_at TIMESTAMPTZ,
                                         document_id BIGINT, knowledge_store_id BIGINT);
CREATE TABLE kb.input_proc_status (record_id BIGINT, processor TEXT, proc_status TEXT);
CREATE TABLE kb.metrics_dropped (id BIGSERIAL PRIMARY KEY, input_record_id BIGINT, drop_id TEXT, drop_reason TEXT, row_data JSONB)`); err != nil {
		t.Fatal(err)
	}
	// Minimal metric table with the columns used by insertResolvedMetricRow. Reference
	// tables and the provenance column come from their real goose migrations.
	columns := []string{"input_record_id BIGINT", "ref_mention_id BIGINT", "ref_input_record_id BIGINT", "is_explicit_metric BOOLEAN", "source_line_spans JSONB", "source_table_rows JSONB", "reasoning_tags JSONB", "ext_info JSONB"}
	for _, col := range strings.Fields(`metric_id metric_name metric_name_en metric_subject metric_subject_en metric_desc metric_desc_en metric_context location_type metric_keywords metric_keywords_en metric_unit metric_unit_en formula_or_definition threshold_or_target measurement_frequency confidence table_name_or_section metric_value value_data_type value_range_type value_class value_class_en category_paths category_paths_en metric_categories metric_categories_en value_min value_max condition keyword_concept_id metric_definition_term_id ref_metric_id ref_metric_key`) {
		columns = append(columns, col+" TEXT")
	}
	if _, err = db.Exec("CREATE TABLE kb.metrics (id BIGSERIAL PRIMARY KEY," + strings.Join(columns, ",") + ")"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, file := range []string{"20261011000001_create_kb_reference_tables.sql", "20261011000004_add_reference_source_table_rows.sql", "20261018000003_add_metric_value_structure.sql", "20261117000001_create_kb_attention_items.sql"} {
		body, err := os.ReadFile(filepath.Join("../../../project_migrations", file))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, file), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(db, dir); err != nil {
		t.Fatal(err)
	}
	return db, dir
}

func TestMetricReferenceAttentionLifecycle(t *testing.T) {
	db, _ := referenceTestDB(t)
	ctx := context.Background()
	store := ReferenceSQLStore{DB: db}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO kb.inputs VALUES (753, 'alice', 1), (999, 'alice', 1)`)
	mentions := assembleRefMentions(753, []RefMention{{CitedRaw: "ISO 9001", TargetContent: "criterion", MetricName: "Power", SourceLineSpans: []string{"116"}}})
	if err := store.ReplaceRecordReferences(ctx, 753, "test", mentions); err != nil {
		t.Fatal(err)
	}
	outgoing := func() {
		t.Helper()
		if _, err := ResolveOutgoing(ctx, db, 753, nil); err != nil {
			t.Fatal(err)
		}
	}
	incoming := func() {
		t.Helper()
		if _, err := ResolveIncoming(ctx, db, "metric", 999, nil); err != nil {
			t.Fatal(err)
		}
	}
	wantOpen := func(wantType, wantReason string, wantCount int) {
		t.Helper()
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM kb.attention_items WHERE status = 'open'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if wantType == "" && count == 0 {
			return
		}
		if wantType == "" || count != 1 {
			t.Fatalf("open attention items = %d, want one %q or none when empty", count, wantType)
		}
		var typ, reason string
		var occurrences int
		err := db.QueryRow(`SELECT item_type, details->>'reason', occurrence_count FROM kb.attention_items WHERE status = 'open'`).Scan(&typ, &reason, &occurrences)
		if err != nil || typ != wantType || reason != wantReason || occurrences != wantCount {
			t.Fatalf("open item = (%q, %q, %d), want (%q, %q, %d), err=%v", typ, reason, occurrences, wantType, wantReason, wantCount, err)
		}
	}

	if err := store.ResolveHoldings(ctx, []int64{753}, 0); err != nil {
		t.Fatal(err)
	}
	outgoing()
	wantOpen("missing-reference-document", "not_held", 1)
	outgoing()
	wantOpen("missing-reference-document", "not_held", 2)

	resourceID, err := store.AttachOwnDocument(ctx, 999, RefIdentifier{Kind: "standard", Value: "ISO 9001", Edition: "2015"}, "", "standard")
	if err != nil {
		t.Fatal(err)
	}
	p := ReferencesProcessor{DB: db}
	if _, err := p.resolveHoldingEvent(ctx, store, 999, resourceID); err != nil {
		t.Fatal(err)
	}
	wantOpen("", "", 0)
	incoming() // The target's extractor finished with zero metrics.
	wantOpen("reference-metric-no-match", "type_doubtful", 1)

	exec(`INSERT INTO kb.metrics (input_record_id, metric_id, metric_name, metric_value) VALUES (999, 'power', 'Power', '10')`)
	exec(`
INSERT INTO kb.attention_items (item_type, dedupe_key, input_record_id, details)
VALUES ('unresolved-reference', $1, 753, '{"family":"metric"}')`,
		"metric:753:"+mentions[0].MentionKey+":999")
	incoming()
	wantOpen("", "", 0)
	var value string
	if err := db.QueryRow(`SELECT metric_value FROM kb.metrics WHERE input_record_id=753 AND ref_mention_id IS NOT NULL`).Scan(&value); err != nil || value != "10" {
		t.Fatalf("resolved value = %q, err=%v", value, err)
	}

	exec(`UPDATE kb.metrics SET metric_name='Temperature' WHERE input_record_id=999`)
	exec(`ALTER TABLE kb.attention_items RENAME TO blocked_attention_items`)
	if _, err := ResolveIncoming(ctx, db, "metric", 999, nil); err == nil {
		t.Fatal("expected reconciliation to fail when attention items cannot be updated")
	}
	exec(`ALTER TABLE kb.blocked_attention_items RENAME TO attention_items`)
	var state string
	if err := db.QueryRow(`SELECT metric_state FROM kb.ref_mentions WHERE input_record_id=753`).Scan(&state); err != nil || state != metricRefResolved {
		t.Fatalf("metric state after failed attention write = %q, err=%v", state, err)
	}
	incoming()
	wantOpen("reference-metric-no-match", "type_doubtful", 2)
	if err := store.ReplaceRecordReferences(ctx, 753, "test-rerun", nil); err != nil {
		t.Fatal(err)
	}
	wantOpen("", "", 0)
}

func TestReferenceTableRowsSQLRoundTrip(t *testing.T) {
	db, dir := referenceTestDB(t)
	ctx := context.Background()
	var err error
	refs := []TableRowRef{{Line: 116, Rows: []string{"r1"}, RowHash: map[string]string{"r1": "systemhash12"}}}
	mentions := assembleRefMentions(753, []RefMention{{CitedRaw: "ISO 9001", SourceLineSpans: []string{"116"}, SourceTableRows: refs, Evidence: "[r1] Item: Fan | Standard: ISO 9001", TargetContent: "criterion", MetricName: "Power"}, {CitedRaw: "ISO 14001", SourceLineSpans: []string{"117"}, Evidence: "Prose reference"}})
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range mentions {
		if err = insertRefMention(ctx, tx, "test", &mentions[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRefMentions(ctx, db, "m.input_record_id = $1", 753)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || !reflect.DeepEqual(loaded[0].SourceTableRows, refs) || len(loaded[1].SourceTableRows) != 0 {
		t.Fatalf("round trip lost table/prose provenance: %+v", loaded)
	}
	var targetID int64
	if err = db.QueryRow(`INSERT INTO kb.metrics (input_record_id, metric_id, metric_value, value_structure) VALUES (999,'target_metric','10, 20 or 30','{"kind":"alternatives","values":["10","20","30"]}') RETURNING id`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = insertResolvedMetricRow(ctx, tx, loaded[0], ArtifactCandidate{RowID: targetID, ArtifactKey: "target_key"}, "753_rfm_1"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var stored, value, evidence, structure string
	if err = db.QueryRow(`SELECT source_table_rows::text,metric_value,metric_context,value_structure::text FROM kb.metrics WHERE metric_id='753_rfm_1'`).Scan(&stored, &value, &evidence, &structure); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parseTableRowRefs(stored), refs) || value != "10, 20 or 30" || evidence != loaded[0].Evidence {
		t.Fatalf("resolved metric lost source rows/value/evidence: %s %s %s", stored, value, evidence)
	}
	var collection struct {
		Kind   string   `json:"kind"`
		Values []string `json:"values"`
	}
	if err := json.Unmarshal([]byte(structure), &collection); err != nil {
		t.Fatal(err)
	}
	if collection.Kind != "alternatives" || !reflect.DeepEqual(collection.Values, []string{"10", "20", "30"}) {
		t.Fatalf("reference clone lost collection relationship: %s", structure)
	}
	if err = goose.DownTo(db, dir, 20261011000004); err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(db, dir); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM kb.ref_mentions`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("migration rollback/up lost mentions: count=%d err=%v", count, err)
	}
}
