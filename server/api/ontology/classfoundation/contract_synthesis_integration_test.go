package classfoundation

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

// freshClassfoundationTestDB follows the same fresh-scratch-database pattern
// as server/api/ontology/assertions/metric_lossless_writer_integration_test.go's
// freshAssertionsTestDB (and semantic/integration_test.go's freshSemanticTestDB
// before it):
//
//	TEST_DATABASE_URL='host=127.0.0.1 user=cding dbname=postgres sslmode=disable' \
//	    go test ./server/api/ontology/classfoundation/ -run Synthesis -v
func freshClassfoundationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	template := os.Getenv("TEST_DATABASE_URL")
	if template == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	dbnamePattern := regexp.MustCompile(`dbname=\S+`)
	if !dbnamePattern.MatchString(template) {
		t.Fatalf("TEST_DATABASE_URL must be a key=value libpq string with dbname=..., got %q", template)
	}
	admin, err := sql.Open("postgres", dbnamePattern.ReplaceAllString(template, "dbname=postgres"))
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatalf("ping admin connection: %v", err)
	}

	name := fmt.Sprintf("classfoundation_synthesis_%d_%d", time.Now().UnixNano(), rand.Intn(1_000_000))
	if _, err := admin.Exec(`CREATE DATABASE ` + pq.QuoteIdentifier(name)); err != nil {
		t.Fatalf("create scratch database %s: %v", name, err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + pq.QuoteIdentifier(name) + ` WITH (FORCE)`)
	})

	db, err := sql.Open("postgres", dbnamePattern.ReplaceAllString(template, "dbname="+name))
	if err != nil {
		t.Fatalf("open scratch database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS kb`); err != nil {
		t.Fatalf("create kb schema: %v", err)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations")
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	if err := goose.Up(db, migrations); err != nil {
		t.Fatalf("run project migrations: %v", err)
	}
	return db
}

func seedPresentObservation(t *testing.T, db *sql.DB, classTermID, documentKey, logicalDatatype, unitTermID string) {
	t.Helper()
	if err := (ObservedProfileStore{DB: db}).Record(context.Background(), ObservedProfileObservation{
		ClassTermID:       classTermID,
		AttributeKey:      "value",
		LogicalDatatype:   logicalDatatype,
		ValueForm:         "single",
		UnitTermID:        unitTermID,
		ObservationState:  "present",
		AggregationMethod: "test",
		MethodVersion:     "v1",
		DocumentKey:       documentKey,
	}); err != nil {
		t.Fatalf("seed present observation: %v", err)
	}
}

func TestSynthesizeContractPromotesOnTwoDocumentAgreement(t *testing.T) {
	db := freshClassfoundationTestDB(t)
	ctx := context.Background()
	const classTermID = "measurement:synth-agree"
	if _, err := (ContractStore{DB: db}).EnsureHeader(ctx, ClassIdentity{TermID: classTermID, ModuleID: "measurement", By: "test"}); err != nil {
		t.Fatalf("EnsureHeader: %v", err)
	}
	seedPresentObservation(t, db, classTermID, "doc-1", "number", "quantity:unit_CD-PER-M2")
	seedPresentObservation(t, db, classTermID, "doc-2", "number", "quantity:unit_CD-PER-M2")

	revision, promoted, err := SynthesizeContractFromObservations(ctx, db, classTermID)
	if err != nil {
		t.Fatalf("SynthesizeContractFromObservations: %v", err)
	}
	if !promoted {
		t.Fatal("expected promotion on two-document agreement")
	}
	if revision.DefinitionState != DefinitionPartiallyDefined {
		t.Fatalf("definition_state = %q, want partially_defined", revision.DefinitionState)
	}
	wantPayload := `{"permitted_unit_term_ids":["quantity:unit_CD-PER-M2"],"value_type":"number"}`
	if revision.ContractPayload != wantPayload {
		t.Fatalf("contract_payload = %s, want %s", revision.ContractPayload, wantPayload)
	}
}

func TestSynthesizeContractDoesNotPromoteOnSingleDocument(t *testing.T) {
	db := freshClassfoundationTestDB(t)
	ctx := context.Background()
	const classTermID = "measurement:synth-single-doc"
	if _, err := (ContractStore{DB: db}).EnsureHeader(ctx, ClassIdentity{TermID: classTermID, ModuleID: "measurement", By: "test"}); err != nil {
		t.Fatalf("EnsureHeader: %v", err)
	}
	seedPresentObservation(t, db, classTermID, "doc-1", "number", "quantity:unit_CD-PER-M2")

	_, promoted, err := SynthesizeContractFromObservations(ctx, db, classTermID)
	if err != nil {
		t.Fatalf("SynthesizeContractFromObservations: %v", err)
	}
	if promoted {
		t.Fatal("expected no promotion from a single document's evidence")
	}
	current, ok, err := (ContractStore{DB: db}).Current(ctx, classTermID)
	if err != nil || !ok {
		t.Fatalf("Current: ok=%v err=%v", ok, err)
	}
	if current.DefinitionState != DefinitionIdentityOnly {
		t.Fatalf("definition_state = %q, want identity_only (unchanged)", current.DefinitionState)
	}
}

func TestSynthesizeContractDoesNotPromoteOnConflictingUnits(t *testing.T) {
	db := freshClassfoundationTestDB(t)
	ctx := context.Background()
	const classTermID = "measurement:synth-conflict"
	if _, err := (ContractStore{DB: db}).EnsureHeader(ctx, ClassIdentity{TermID: classTermID, ModuleID: "measurement", By: "test"}); err != nil {
		t.Fatalf("EnsureHeader: %v", err)
	}
	seedPresentObservation(t, db, classTermID, "doc-1", "number", "quantity:unit_CD-PER-M2")
	seedPresentObservation(t, db, classTermID, "doc-2", "number", "quantity:unit_LUX")

	_, promoted, err := SynthesizeContractFromObservations(ctx, db, classTermID)
	if err != nil {
		t.Fatalf("SynthesizeContractFromObservations: %v", err)
	}
	if promoted {
		t.Fatal("expected no promotion when documents disagree on unit")
	}
}

func TestSynthesizeContractExcludesNonPresentObservations(t *testing.T) {
	db := freshClassfoundationTestDB(t)
	ctx := context.Background()
	const classTermID = "measurement:synth-exclude-unparsed"
	if _, err := (ContractStore{DB: db}).EnsureHeader(ctx, ClassIdentity{TermID: classTermID, ModuleID: "measurement", By: "test"}); err != nil {
		t.Fatalf("EnsureHeader: %v", err)
	}
	seedPresentObservation(t, db, classTermID, "doc-1", "number", "quantity:unit_CD-PER-M2")
	if err := (ObservedProfileStore{DB: db}).Record(ctx, ObservedProfileObservation{
		ClassTermID: classTermID, AttributeKey: "value", LogicalDatatype: "text", ValueForm: "single",
		ObservationState: "unparsed", AggregationMethod: "test", MethodVersion: "v1", DocumentKey: "doc-2",
	}); err != nil {
		t.Fatalf("seed unparsed observation: %v", err)
	}

	_, promoted, err := SynthesizeContractFromObservations(ctx, db, classTermID)
	if err != nil {
		t.Fatalf("SynthesizeContractFromObservations: %v", err)
	}
	if promoted {
		t.Fatal("expected no promotion: only one present-state document, the other is unparsed and must not count")
	}
}

func TestSynthesizeContractLeavesAlreadyPromotedContractUnchanged(t *testing.T) {
	db := freshClassfoundationTestDB(t)
	ctx := context.Background()
	const classTermID = "measurement:synth-already-promoted"
	if _, err := (ContractStore{DB: db}).EnsureHeader(ctx, ClassIdentity{TermID: classTermID, ModuleID: "measurement", By: "test"}); err != nil {
		t.Fatalf("EnsureHeader: %v", err)
	}
	seedPresentObservation(t, db, classTermID, "doc-1", "number", "quantity:unit_CD-PER-M2")
	seedPresentObservation(t, db, classTermID, "doc-2", "number", "quantity:unit_CD-PER-M2")
	first, promoted, err := SynthesizeContractFromObservations(ctx, db, classTermID)
	if err != nil || !promoted {
		t.Fatalf("first synthesis: promoted=%v err=%v", promoted, err)
	}

	// Later, contradicting evidence arrives. It must not revert or replace
	// the already-promoted contract.
	seedPresentObservation(t, db, classTermID, "doc-3", "number", "quantity:unit_LUX")
	second, promotedAgain, err := SynthesizeContractFromObservations(ctx, db, classTermID)
	if err != nil {
		t.Fatalf("second synthesis: %v", err)
	}
	if promotedAgain {
		t.Fatal("expected no further promotion once a contract has left identity_only")
	}
	if second.ID != first.ID || second.DefinitionState != DefinitionPartiallyDefined {
		t.Fatalf("expected the original promoted revision to remain current, got %#v", second)
	}
}
