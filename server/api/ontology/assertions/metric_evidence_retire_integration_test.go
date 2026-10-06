package assertions

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

// writeTestMetricAssertion materializes one metric occurrence through the
// lossless writer, leaving one active 'supports' evidence row for it.
func writeTestMetricAssertion(t *testing.T, db *sql.DB, metricID, name string, value float64, recordID int64) {
	t.Helper()
	p := metricCandidatePayload{
		MetricID:             metricID,
		MetricName:           name,
		SubjectObjectID:      "obj-1",
		RawText:              name,
		ValueForm:            "single",
		NumericValue:         &value,
		Unit:                 "C",
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "proposed",
	}
	dc := proposeMetricCandidate(t, db, "metric:"+metricID, metricID, recordID, p)
	if _, err := (AssociateSemantics{DB: db}).writeMetricLossless(context.Background(), dc, p, recordID, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless %s: %v", metricID, err)
	}
}

func activeMetricSupport(t *testing.T, db *sql.DB, metricID string, recordID int64) (assertionID int64, found bool) {
	t.Helper()
	err := db.QueryRow(`
SELECT assertion_id FROM kb.assertion_evidence
WHERE artifact_type = 'metric' AND artifact_id = $1 AND input_record_id = $2
  AND evidence_role = 'supports' AND NOT deleted`, metricID, recordID).Scan(&assertionID)
	if err == sql.ErrNoRows {
		return 0, false
	}
	if err != nil {
		t.Fatalf("query support for %s: %v", metricID, err)
	}
	return assertionID, true
}

func lastMetricSupport(t *testing.T, db *sql.DB, metricID string, recordID int64) (assertionID int64) {
	t.Helper()
	if err := db.QueryRow(`
SELECT assertion_id FROM kb.assertion_evidence
WHERE artifact_type = 'metric' AND artifact_id = $1 AND input_record_id = $2
ORDER BY id DESC LIMIT 1`, metricID, recordID).Scan(&assertionID); err != nil {
		t.Fatalf("query last support for %s: %v", metricID, err)
	}
	return assertionID
}

func TestIntegrationRetireMetricEvidenceForRecord(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-1")

	writeTestMetricAssertion(t, db, "100_mtc_1", "Widget Temperature", 42, 100)
	writeTestMetricAssertion(t, db, "100_mtc_2", "Widget Pressure", 7, 100)
	writeTestMetricAssertion(t, db, "200_mtc_1", "Gadget Temperature", 11, 200)

	// A non-metric supporting link on 100_mtc_2's assertion must survive and
	// keep that assertion supported.
	keptAssertion, ok := activeMetricSupport(t, db, "100_mtc_2", 100)
	if !ok {
		t.Fatal("expected support for 100_mtc_2")
	}
	recordID := int64(100)
	evStore := EvidenceStore{DB: db, Assertions: AssertionStore{DB: db}}
	if _, err := evStore.AddEvidence(ctx, Evidence{
		AssertionID:   keptAssertion,
		InputRecordID: &recordID,
		ArtifactType:  "provision",
		ArtifactID:    "100_prv_1",
		EvidenceRole:  "supports",
		ActorKind:     "processor",
		CreateBy:      "test",
	}); err != nil {
		t.Fatalf("add provision evidence: %v", err)
	}
	retiredAssertion, _ := activeMetricSupport(t, db, "100_mtc_1", 100)

	const reason = "metric rows deleted for re-extraction"
	n, err := evStore.RetireMetricEvidenceForRecord(ctx, 100, "test", reason)
	if err != nil {
		t.Fatalf("RetireMetricEvidenceForRecord: %v", err)
	}
	if n != 2 {
		t.Fatalf("retired = %d, want 2", n)
	}

	for _, id := range []string{"100_mtc_1", "100_mtc_2"} {
		if _, found := activeMetricSupport(t, db, id, 100); found {
			t.Fatalf("%s still has active metric evidence", id)
		}
	}
	var deletedReason string
	if err := db.QueryRow(`SELECT deleted_reason FROM kb.assertion_evidence
WHERE artifact_id = '100_mtc_1' AND deleted`).Scan(&deletedReason); err != nil {
		t.Fatalf("read deleted reason: %v", err)
	}
	if deletedReason != reason {
		t.Fatalf("deleted_reason = %q, want %q", deletedReason, reason)
	}
	if _, found := activeMetricSupport(t, db, "200_mtc_1", 200); !found {
		t.Fatal("other record's metric evidence was retired")
	}
	var provisionActive bool
	if err := db.QueryRow(`SELECT NOT deleted FROM kb.assertion_evidence
WHERE artifact_type = 'provision' AND artifact_id = '100_prv_1'`).Scan(&provisionActive); err != nil {
		t.Fatalf("read provision evidence: %v", err)
	}
	if !provisionActive {
		t.Fatal("non-metric evidence was retired")
	}

	retired, err := evStore.Assertions.GetByID(ctx, retiredAssertion)
	if err != nil {
		t.Fatalf("load retired assertion: %v", err)
	}
	if retired.Status != StatusUnsupported {
		t.Fatalf("assertion losing last support: status = %q, want unsupported", retired.Status)
	}
	if !strings.Contains(retired.DecisionReason, reason) {
		t.Fatalf("decision_reason = %q, want it to contain %q", retired.DecisionReason, reason)
	}
	kept, err := evStore.Assertions.GetByID(ctx, keptAssertion)
	if err != nil {
		t.Fatalf("load kept assertion: %v", err)
	}
	if kept.Status == StatusUnsupported {
		t.Fatal("assertion with remaining provision support became unsupported")
	}

	if n, err := evStore.RetireMetricEvidenceForRecord(ctx, 100, "test", reason); err != nil || n != 0 {
		t.Fatalf("second retire = (%d, %v), want (0, nil)", n, err)
	}
}

// TestIntegrationReusedMetricIDStartsWithoutEvidence reproduces the stale-link
// defect: after re-extraction renumbers metric_id, the reused ID must not carry
// the old occurrence's evidence until the writer links the new row.
func TestIntegrationReusedMetricIDStartsWithoutEvidence(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-1")

	writeTestMetricAssertion(t, db, "300_mtc_1", "Old Temperature", 42, 300)
	oldAssertion, _ := activeMetricSupport(t, db, "300_mtc_1", 300)

	evStore := EvidenceStore{DB: db, Assertions: AssertionStore{DB: db}}
	if _, err := evStore.RetireMetricEvidenceForRecord(ctx, 300, "test", "metric rows deleted for re-extraction"); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, found := activeMetricSupport(t, db, "300_mtc_1", 300); found {
		t.Fatal("reused metric_id still carries the old occurrence's evidence")
	}

	// The writer then links the new occurrence that reuses the ID.
	writeTestMetricAssertion(t, db, "300_mtc_1", "New Pressure", 9, 300)
	newAssertion, found := activeMetricSupport(t, db, "300_mtc_1", 300)
	if !found {
		t.Fatal("new occurrence was not linked")
	}
	if newAssertion == oldAssertion {
		t.Fatal("new occurrence linked to the old assertion")
	}
	if got := lastMetricSupport(t, db, "300_mtc_1", 300); got != newAssertion {
		t.Fatalf("latest evidence assertion = %d, want %d", got, newAssertion)
	}
}

func TestIntegrationMetricProvisionLink(t *testing.T) {
	db := freshAssertionsTestDB(t)

	var inputID int64
	if err := db.QueryRow(`INSERT INTO kb.inputs (type) VALUES ('doc') RETURNING id`).Scan(&inputID); err != nil {
		t.Fatalf("seed input: %v", err)
	}
	var provisionID int64
	if err := db.QueryRow(`
INSERT INTO kb.provisions (input_record_id, extract_id, input_filename, prov_id)
VALUES ($1, 'e1', 'f.txt', 'p1') RETURNING id`, inputID).Scan(&provisionID); err != nil {
		t.Fatalf("seed provision: %v", err)
	}

	var unlinked sql.NullInt64
	if err := db.QueryRow(`
INSERT INTO kb.metrics (input_record_id, metric_id) VALUES ($1, 'm-unlinked')
RETURNING provision_id`, inputID).Scan(&unlinked); err != nil {
		t.Fatalf("insert unlinked metric: %v", err)
	}
	if unlinked.Valid {
		t.Fatalf("default provision_id = %d, want NULL", unlinked.Int64)
	}

	if _, err := db.Exec(`INSERT INTO kb.metrics (input_record_id, metric_id, provision_id)
VALUES ($1, 'm-bad', $2)`, inputID, provisionID+1000); err == nil {
		t.Fatal("insert with unknown provision_id succeeded, want foreign-key violation")
	}

	if _, err := db.Exec(`INSERT INTO kb.metrics (input_record_id, metric_id, provision_id)
VALUES ($1, 'm-linked', $2)`, inputID, provisionID); err != nil {
		t.Fatalf("insert linked metric: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM kb.provisions WHERE input_record_id = $1`, inputID); err != nil {
		t.Fatalf("delete provisions (re-extraction): %v", err)
	}
	var after sql.NullInt64
	if err := db.QueryRow(`SELECT provision_id FROM kb.metrics WHERE metric_id = 'm-linked'`).Scan(&after); err != nil {
		t.Fatalf("metric row missing after provision delete: %v", err)
	}
	if after.Valid {
		t.Fatalf("provision_id after delete = %d, want NULL", after.Int64)
	}

	// Rollback and re-apply of migration 20261006000005.
	_, thisFile, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations")
	if err := goose.DownTo(db, migrations, 20261006000004); err != nil {
		t.Fatalf("goose down: %v", err)
	}
	if hasProvisionIDColumn(t, db) {
		t.Fatal("provision_id still present after rollback")
	}
	if err := goose.Up(db, migrations); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	if !hasProvisionIDColumn(t, db) {
		t.Fatal("provision_id missing after re-apply")
	}
}

func hasProvisionIDColumn(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.columns
WHERE table_schema = 'kb' AND table_name = 'metrics' AND column_name = 'provision_id')`).Scan(&exists); err != nil {
		t.Fatalf("check column: %v", err)
	}
	return exists
}
