package docprocessing

import (
	"context"
	"database/sql"
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
func TestReferenceTableRowsSQLRoundTrip(t *testing.T) {
	if os.Getenv("REFERENCE_TABLE_ROWS_TEST_DB") != "1" {
		t.Skip("REFERENCE_TABLE_ROWS_TEST_DB not set")
	}
	admin, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("reference_rows_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE " + pq.QuoteIdentifier(name)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP DATABASE " + pq.QuoteIdentifier(name) + " WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	}()
	db, err := sql.Open("postgres", "dbname="+name+" sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err = db.Exec(`CREATE SCHEMA kb`); err != nil {
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
	for _, file := range []string{"20261011000001_create_kb_reference_tables.sql", "20261011000004_add_reference_source_table_rows.sql"} {
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
	if err = db.QueryRow(`INSERT INTO kb.metrics (input_record_id, metric_id, metric_value) VALUES (999,'target_metric','10') RETURNING id`).Scan(&targetID); err != nil {
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
	var stored, value, evidence string
	if err = db.QueryRow(`SELECT source_table_rows::text,metric_value,metric_context FROM kb.metrics WHERE metric_id='753_rfm_1'`).Scan(&stored, &value, &evidence); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parseTableRowRefs(stored), refs) || value != "10" || evidence != loaded[0].Evidence {
		t.Fatalf("resolved metric lost source rows/value/evidence: %s %s %s", stored, value, evidence)
	}
	if err = goose.Down(db, dir); err != nil {
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
