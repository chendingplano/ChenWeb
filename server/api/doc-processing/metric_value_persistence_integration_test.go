package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/metricvalue"
	"github.com/lib/pq"
)

// All mutations use an isolated disposable database; PG* env variables select the local server.
func TestMetricValueWorkerSQLRoundTrip(t *testing.T) {
	if os.Getenv("METRIC_VALUE_TEST_DB") != "1" {
		t.Skip("METRIC_VALUE_TEST_DB not set")
	}
	admin, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("metric_value_test_%d", time.Now().UnixNano())
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
	store := MetricsSQLStore{DB: db}
	if err := store.ensureMetricsTable(ctx); err != nil {
		t.Fatal(err)
	}
	// Minimal surrounding schema required by worker SQL. The new column comes from its actual migration.
	_, err = db.Exec(`CREATE SCHEMA testbed;
 CREATE TABLE testbed.metrics(id BIGINT);
 CREATE TABLE kb.inputs(id BIGINT PRIMARY KEY, title TEXT, doc_no TEXT, doc_metadata JSONB);
 ALTER TABLE kb.metrics ADD COLUMN keyword_concept_id TEXT, ADD COLUMN metric_definition_term_id TEXT,
 ADD COLUMN subject_concept_id TEXT, ADD COLUMN source_table_rows JSONB, ADD COLUMN frame JSONB, ADD COLUMN ref_mention_id BIGINT;
 CREATE UNIQUE INDEX test_metric_identity ON kb.metrics(input_record_id,metric_id) WHERE metric_id IS NOT NULL;
 CREATE FUNCTION kb.connected_artifacts(BIGINT,TEXT,BIGINT) RETURNS JSONB LANGUAGE SQL AS 'SELECT ''[]''::jsonb';`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("../../../project_migrations", "20261018000003_add_metric_value_structure.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(strings.Split(string(raw), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	structure := map[string]any{"kind": "alternatives", "values": []string{"10", "20", "30"}}
	metric := map[string]any{"metric_id": "7_mtc_1", "metric_name": "Diameter", "metric_value": "10, 20 or 30", "unit": "mm", "value_structure": structure}
	req := SaveMetricsRequest{InputRecordID: 7, Language: "en", Metrics: []map[string]any{metric}}
	if n, err := store.SaveMetrics(ctx, req); err != nil || n != 1 {
		t.Fatalf("save = %d, %v", n, err)
	}
	check := func(want map[string]any) {
		t.Helper()
		loaded, err := store.GetMetricsByInputRecordID(ctx, 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded) != 1 || loaded[0]["metric_value"] != metric["metric_value"] || !metricvalue.StructureEqual(loaded[0]["value_structure"], want) {
			t.Fatalf("worker read lost semantics: %+v", loaded)
		}
		artifact, err := loadPersistedMetricArtifactRows(ctx, db, 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(artifact) != 1 || artifact[0]["metric_value"] != metric["metric_value"] || !metricvalue.StructureEqual(artifact[0]["value_structure"], want) {
			t.Fatalf("artifact lost semantics: %+v", artifact)
		}
	}
	check(structure)
	structure = map[string]any{"kind": "required_points", "values": []string{"10", "20", "30"}}
	metric["value_structure"] = structure
	metric["metric_value"] = "10, 20 and 30"
	if n, err := store.UpsertMetrics(ctx, req); err != nil || n != 1 {
		t.Fatalf("upsert = %d, %v", n, err)
	}
	check(structure)
	// Legacy/scalar NULL survives both readback paths and is not an unresolved object.
	metric["value_structure"] = nil
	metric["metric_value"] = "10"
	if n, err := store.UpsertMetrics(ctx, req); err != nil || n != 1 {
		t.Fatalf("scalar upsert = %d, %v", n, err)
	}
	check(nil)
	var isNull bool
	if err := db.QueryRow("SELECT value_structure IS NULL FROM kb.metrics WHERE metric_id='7_mtc_1'").Scan(&isNull); err != nil || !isNull {
		t.Fatalf("scalar not SQL NULL: %v, %v", isNull, err)
	}
	metric["value_structure"] = structure
	if _, err := store.UpsertMetrics(ctx, req); err == nil {
		t.Fatal("worker accepted collection members contradicting display")
	}
}
