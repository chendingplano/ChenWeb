package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
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

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// freshAssertionsTestDB follows the same fresh-scratch-database pattern as
// server/api/ontology/semantic/integration_test.go's freshSemanticTestDB:
//
//	TEST_DATABASE_URL='host=127.0.0.1 user=cding dbname=postgres sslmode=disable' \
//	    go test ./server/api/ontology/assertions/ -run Integration -v
func freshAssertionsTestDB(t *testing.T) *sql.DB {
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

	name := fmt.Sprintf("assertions_metric_lossless_%d_%d", time.Now().UnixNano(), rand.Intn(1_000_000))
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

func seedObjectNode(t *testing.T, db *sql.DB, objectID string) {
	t.Helper()
	if _, err := db.Exec(`
INSERT INTO kb.object_nodes (object_id, canonical_name, object_type)
VALUES ($1, $1, 'other')
ON CONFLICT DO NOTHING`, objectID); err != nil {
		t.Fatalf("seed object node %s: %v", objectID, err)
	}
}

func seedGovernedTerm(t *testing.T, db *sql.DB, termID, termKind, moduleID string) {
	t.Helper()
	if _, err := db.Exec(`
INSERT INTO kb.ontology_terms (term_id, version, term_kind, module_id, status)
VALUES ($1, 1, $2, $3, 'included_in_release')
ON CONFLICT DO NOTHING`, termID, termKind, moduleID); err != nil {
		t.Fatalf("seed governed term %s: %v", termID, err)
	}
}

func proposeMetricCandidate(t *testing.T, db *sql.DB, logicalIdentityKey, metricID string, inputRecordID int64, p metricCandidatePayload) DecisionCandidate {
	t.Helper()
	payload, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	dc, err := (DecisionCandidateStore{DB: db}).Propose(context.Background(), DecisionCandidate{
		LogicalIdentityKey: logicalIdentityKey,
		CandidateKind:      "assertion",
		ProposedPayload:    payload,
		Method:             "explicit_structured",
		SourceArtifactType: "metric",
		SourceArtifactID:   metricID,
		InputRecordID:      &inputRecordID,
		Status:             StatusCandidate,
		CreateBy:           "test",
		ModifyBy:           "test",
	})
	if err != nil {
		t.Fatalf("propose metric candidate: %v", err)
	}
	dc, err = (DecisionCandidateStore{DB: db}).TransitionStatus(context.Background(), dc.ID, StatusInReview, "", "test")
	if err != nil {
		t.Fatalf("transition candidate to in_review: %v", err)
	}
	return dc
}

func TestIntegrationWriteMetricLosslessMaterializesRawPreservedRepresentedAssertion(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-1")

	numeric := 42.0
	p := metricCandidatePayload{
		MetricID:             "m-1",
		MetricName:           "Widget Temperature",
		SubjectObjectID:      "obj-1",
		RawText:              "42 C",
		ValueForm:            "single",
		NumericValue:         &numeric,
		Unit:                 "C",
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "proposed",
	}
	dc := proposeMetricCandidate(t, db, "metric:m-1", "m-1", 100, p)

	a := AssociateSemantics{DB: db}
	outcome, err := a.writeMetricLossless(ctx, dc, p, 100, "mea:measured_by")
	if err != nil {
		t.Fatalf("writeMetricLossless: %v", err)
	}
	if outcome != "represented" {
		t.Fatalf("outcome = %q, want represented", outcome)
	}

	assertion, err := (AssertionStore{DB: db}).GetLatest(ctx, "claim:"+mustFindClaimID(t, db))
	if err != nil {
		t.Fatalf("load assertion by claim identity: %v", err)
	}
	if assertion.Status != StatusRepresented {
		t.Fatalf("status = %q, want represented", assertion.Status)
	}
	if assertion.ValueStateTermID != semantic.ValuePresent {
		t.Fatalf("value_state = %q, want present (literal parsed even though mapping unresolved)", assertion.ValueStateTermID)
	}
	if assertion.MappingResolutionStateTermID != semantic.MappingUnresolved {
		t.Fatalf("mapping_state = %q, want unresolved", assertion.MappingResolutionStateTermID)
	}
	if assertion.ClassIdentityStateTermID != semantic.ClassProvisionalNew {
		t.Fatalf("class_identity_state = %q, want provisional_new", assertion.ClassIdentityStateTermID)
	}

	var supportCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.assertion_evidence
WHERE artifact_type='metric' AND artifact_id='m-1' AND input_record_id=100
  AND evidence_role='supports' AND NOT deleted`).Scan(&supportCount); err != nil {
		t.Fatalf("count evidence: %v", err)
	}
	if supportCount != 1 {
		t.Fatalf("active supporting evidence links = %d, want 1", supportCount)
	}

	var outcomeCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.semantic_processing_outcomes
WHERE artifact_type='metric' AND artifact_id='m-1' AND active`).Scan(&outcomeCount); err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	if outcomeCount != 3 {
		t.Fatalf("active outcome envelopes = %d, want 3 (one per required stage)", outcomeCount)
	}

	var findingCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.semantic_processing_findings f
JOIN kb.semantic_processing_outcomes o ON o.id = f.outcome_id
WHERE o.artifact_type='metric' AND o.artifact_id='m-1' AND f.active
  AND f.finding_term_id = $1`, semantic.FindingMappingUnresolved).Scan(&findingCount); err != nil {
		t.Fatalf("count mapping_unresolved findings: %v", err)
	}
	if findingCount != 1 {
		t.Fatalf("mapping_unresolved findings = %d, want 1", findingCount)
	}

	var classTermCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.ontology_term_headers WHERE term_id LIKE 'measurement:auto:defname_%'`).Scan(&classTermCount); err != nil {
		t.Fatalf("count provisional class terms: %v", err)
	}
	if classTermCount != 1 {
		t.Fatalf("provisional class terms created = %d, want 1", classTermCount)
	}

	// Idempotent replay: re-running the same attempt must not create a second
	// provisional class, a second supporting link, or duplicate outcomes.
	dc2 := mustReopenCandidateForReplay(t, db, dc.ID)
	if _, err := a.writeMetricLossless(ctx, dc2, p, 100, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless replay: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.ontology_term_headers WHERE term_id LIKE 'measurement:auto:defname_%'`).Scan(&classTermCount); err != nil {
		t.Fatalf("count provisional class terms after replay: %v", err)
	}
	if classTermCount != 1 {
		t.Fatalf("provisional class terms after replay = %d, want still 1 (idempotent)", classTermCount)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.assertion_evidence
WHERE artifact_type='metric' AND artifact_id='m-1' AND input_record_id=100
  AND evidence_role='supports' AND NOT deleted`).Scan(&supportCount); err != nil {
		t.Fatalf("count evidence after replay: %v", err)
	}
	if supportCount != 1 {
		t.Fatalf("active supporting evidence links after replay = %d, want still 1", supportCount)
	}
}

// TestIntegrationWriteMetricLosslessProvisionalClassIsCatalogVisible locks in
// metric-class-synthesis-seam's visibility fix (bug 2026082101 finding 5
// follow-up): before this change, a brand-new concept's provisional class
// was created only via classfoundation.CreateIdentityOnlyClass, which never
// inserted into kb.ontology_terms -- so it never appeared in
// kb.ontology_terms_current (the view every other governed-term reader
// uses), because kb.ontology_term_revisions.source_term_row_id is NOT NULL
// REFERENCES kb.ontology_terms(id) and nothing had populated that FK target.
// synthesizeClass now always creates the class through terms.TermStore,
// so the resulting term is a real, catalog-visible row.
func TestIntegrationWriteMetricLosslessProvisionalClassIsCatalogVisible(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	// This test's payload carries a ConceptID, which routes class synthesis
	// through AlignmentsStore.ensureAccepted's released-guard check
	// (alignment.go) -- it requires core:aligns_to_term itself to already be
	// a released property term, independent of the metric_definition term
	// being created.
	seedGovernedTerm(t, db, "core:aligns_to_term", "property", "core")
	seedObjectNode(t, db, "obj-3")

	numeric := 99.0
	p := metricCandidatePayload{
		MetricID:             "m-3",
		MetricName:           "Concept-Linked Metric",
		ConceptID:            "kwc_concept_linked_metric",
		Definition:           "A metric occurrence whose concept has never been promoted before.",
		SubjectObjectID:      "obj-3",
		RawText:              "99",
		ValueForm:            "single",
		NumericValue:         &numeric,
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
	}
	dc := proposeMetricCandidate(t, db, "metric:m-3", "m-3", 300, p)

	a := AssociateSemantics{DB: db}
	if _, err := a.writeMetricLossless(ctx, dc, p, 300, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless: %v", err)
	}

	const wantTermID = "measurement:kwc_concept_linked_metric"
	var definition string
	if err := db.QueryRowContext(ctx,
		`SELECT definition FROM kb.ontology_terms_current WHERE term_id = $1`, wantTermID,
	).Scan(&definition); err != nil {
		t.Fatalf("provisional class not visible via kb.ontology_terms_current: %v", err)
	}
	if definition != p.Definition {
		t.Fatalf("kb.ontology_terms_current.definition = %q, want %q", definition, p.Definition)
	}
}

func mustReopenCandidateForReplay(t *testing.T, db *sql.DB, id int64) DecisionCandidate {
	t.Helper()
	if _, err := db.Exec(`UPDATE kb.semantic_decision_candidates SET status='in_review' WHERE id=$1`, id); err != nil {
		t.Fatalf("reopen candidate for replay: %v", err)
	}
	dc, err := (DecisionCandidateStore{DB: db}).GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("reload candidate: %v", err)
	}
	return dc
}

func mustFindClaimID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var logicalKey string
	if err := db.QueryRow(`SELECT logical_identity_key FROM kb.semantic_assertions WHERE subject_ref_id = 'obj-1' LIMIT 1`).Scan(&logicalKey); err != nil {
		t.Fatalf("find assertion logical identity key: %v", err)
	}
	return logicalKey[len("claim:"):]
}

func TestIntegrationWriteMetricLosslessRollsBackOnLateFailure(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-2")

	numeric := 7.0
	p := metricCandidatePayload{
		MetricID:             "m-2",
		MetricName:           "Rollback Probe",
		SubjectObjectID:      "obj-2",
		RawText:              "7",
		NumericValue:         &numeric,
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
	}
	dc := proposeMetricCandidate(t, db, "metric:m-2", "m-2", 200, p)

	// Close the DB's write path after the transaction begins is impractical
	// with *sql.DB; instead corrupt the decision candidate row so the final
	// dcStoreTx.TransitionStatus write inside the transaction fails, and
	// confirm nothing from the earlier steps (class, claim, assertion,
	// evidence, outcomes) survived the rollback.
	if _, err := db.Exec(`DELETE FROM kb.semantic_decision_candidates WHERE id = $1`, dc.ID); err != nil {
		t.Fatalf("delete candidate to force a late failure: %v", err)
	}

	a := AssociateSemantics{DB: db}
	if _, err := a.writeMetricLossless(ctx, dc, p, 200, "mea:measured_by"); err == nil {
		t.Fatalf("expected writeMetricLossless to fail once the decision candidate row is gone")
	}

	var assertionCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.semantic_assertions WHERE subject_ref_id = 'obj-2'`).Scan(&assertionCount); err != nil {
		t.Fatalf("count assertions: %v", err)
	}
	if assertionCount != 0 {
		t.Fatalf("assertions after rollback = %d, want 0", assertionCount)
	}
	var evidenceCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.assertion_evidence WHERE artifact_id = 'm-2'`).Scan(&evidenceCount); err != nil {
		t.Fatalf("count evidence: %v", err)
	}
	if evidenceCount != 0 {
		t.Fatalf("evidence after rollback = %d, want 0", evidenceCount)
	}
	var outcomeCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.semantic_processing_outcomes WHERE artifact_id = 'm-2'`).Scan(&outcomeCount); err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	if outcomeCount != 0 {
		t.Fatalf("outcomes after rollback = %d, want 0", outcomeCount)
	}
	var classTermCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.ontology_term_headers WHERE term_id LIKE 'measurement:auto:defname_%'`).Scan(&classTermCount); err != nil {
		t.Fatalf("count provisional class terms: %v", err)
	}
	if classTermCount != 0 {
		t.Fatalf("provisional class terms after rollback = %d, want 0", classTermCount)
	}
	var claimCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.semantic_claim_identities`).Scan(&claimCount); err != nil {
		t.Fatalf("count claim identities: %v", err)
	}
	if claimCount != 0 {
		t.Fatalf("claim identities after rollback = %d, want 0", claimCount)
	}
	var decisionCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.ontology_class_resolution_decisions WHERE source_artifact_id = 'm-2'`).Scan(&decisionCount); err != nil {
		t.Fatalf("count class resolution decisions: %v", err)
	}
	if decisionCount != 0 {
		t.Fatalf("class resolution decisions after rollback = %d, want 0", decisionCount)
	}
}

// TestIntegrationWriteMetricLosslessConvergesOnSameClaimAcrossDifferingRawWording
// covers task 6.12's identity-value branch requirement: two occurrences of
// the same subject, class, and normalized value converge on the same claim
// (DR2) even though their raw wording, metric ID, and extraction provenance
// differ -- provenance is deliberately excluded from canonical claim
// identity so semantically equal occurrences converge.
func TestIntegrationWriteMetricLosslessConvergesOnSameClaimAcrossDifferingRawWording(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-3")

	numeric := 100.0
	base := metricCandidatePayload{
		MetricName:             "Converging Metric",
		MetricDefinitionTermID: "measurement:auto:kwc_converge_test",
		SubjectObjectID:        "obj-3",
		NumericValue:           &numeric,
		Unit:                   "C",
		AssertionKind:          "observed_value",
		ValueRangeTypeLookup:   "absent",
	}
	a := AssociateSemantics{DB: db}

	first := base
	first.MetricID = "m-3a"
	first.RawText = "one hundred degrees"
	dc1 := proposeMetricCandidate(t, db, "metric:m-3a", "m-3a", 300, first)
	if _, err := a.writeMetricLossless(ctx, dc1, first, 300, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless first occurrence: %v", err)
	}

	second := base
	second.MetricID = "m-3b"
	second.RawText = "100 C exactly"
	dc2 := proposeMetricCandidate(t, db, "metric:m-3b", "m-3b", 300, second)
	if _, err := a.writeMetricLossless(ctx, dc2, second, 300, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless second occurrence: %v", err)
	}

	var distinctKeys, maxRevision int
	if err := db.QueryRowContext(ctx, `
SELECT count(DISTINCT logical_identity_key), max(revision)
FROM kb.semantic_assertions WHERE subject_ref_id = 'obj-3'`).Scan(&distinctKeys, &maxRevision); err != nil {
		t.Fatalf("query converged assertions: %v", err)
	}
	if distinctKeys != 1 {
		t.Fatalf("distinct logical identity keys = %d, want 1 (same claim)", distinctKeys)
	}
	if maxRevision != 2 {
		t.Fatalf("max revision = %d, want 2 (second occurrence superseded the first)", maxRevision)
	}
	var claimCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.semantic_claim_identities`).Scan(&claimCount); err != nil {
		t.Fatalf("count claim identities: %v", err)
	}
	if claimCount != 1 {
		t.Fatalf("claim identities registered = %d, want 1 (converged, not two)", claimCount)
	}
}

// TestIntegrationWriteMetricLosslessDoesNotConvergeDistinctUnparsedRawValues
// covers the other half of task 6.12: two unparsed occurrences with distinct
// raw text must NOT converge merely because they share the unparsed state --
// their raw-value fingerprints differ, so they resolve to different claims.
func TestIntegrationWriteMetricLosslessDoesNotConvergeDistinctUnparsedRawValues(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-4")

	base := metricCandidatePayload{
		MetricName:             "Unparsed Divergence",
		MetricDefinitionTermID: "measurement:auto:kwc_diverge_test",
		SubjectObjectID:        "obj-4",
		AssertionKind:          "unparsed",
		ValueRangeTypeLookup:   "absent",
	}
	a := AssociateSemantics{DB: db}

	first := base
	first.MetricID = "m-4a"
	first.RawText = "excellent condition"
	dc1 := proposeMetricCandidate(t, db, "metric:m-4a", "m-4a", 400, first)
	if _, err := a.writeMetricLossless(ctx, dc1, first, 400, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless first occurrence: %v", err)
	}

	second := base
	second.MetricID = "m-4b"
	second.RawText = "poor condition"
	dc2 := proposeMetricCandidate(t, db, "metric:m-4b", "m-4b", 400, second)
	if _, err := a.writeMetricLossless(ctx, dc2, second, 400, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless second occurrence: %v", err)
	}

	var distinctKeys int
	if err := db.QueryRowContext(ctx, `
SELECT count(DISTINCT logical_identity_key)
FROM kb.semantic_assertions WHERE subject_ref_id = 'obj-4'`).Scan(&distinctKeys); err != nil {
		t.Fatalf("query assertions: %v", err)
	}
	if distinctKeys != 2 {
		t.Fatalf("distinct logical identity keys = %d, want 2 (distinct raw fingerprints must not converge)", distinctKeys)
	}
	var claimCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.semantic_claim_identities`).Scan(&claimCount); err != nil {
		t.Fatalf("count claim identities: %v", err)
	}
	if claimCount != 2 {
		t.Fatalf("claim identities registered = %d, want 2 (no false convergence)", claimCount)
	}
}

// seedGovernedTermWithProperties is seedGovernedTerm plus a properties JSONB
// payload -- used to seed an existing metric_definition class carrying a
// class-identity signature (ADR 2026082203 DR2) for matchClassBySignature to
// find.
func seedGovernedTermWithProperties(t *testing.T, db *sql.DB, termID, termKind, moduleID, status string, properties map[string]any) {
	t.Helper()
	propsJSON, err := json.Marshal(properties)
	if err != nil {
		t.Fatalf("marshal properties for %s: %v", termID, err)
	}
	if _, err := db.Exec(`
INSERT INTO kb.ontology_terms (term_id, version, term_kind, module_id, status, properties)
VALUES ($1, 1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING`, termID, termKind, moduleID, status, propsJSON); err != nil {
		t.Fatalf("seed governed term with properties %s: %v", termID, err)
	}
}

// signatureEntry builds one BuildSignatureProperties-shaped
// {"raw":..., "term_id":...} ExtraProperties value (ADR 2026082203 DR2).
func signatureEntry(raw, termID string) map[string]any {
	return map[string]any{"raw": raw, "term_id": termID}
}

func mustFindAssertionByObject(t *testing.T, db *sql.DB, objectID string) Assertion {
	t.Helper()
	var logicalKey string
	if err := db.QueryRow(`SELECT logical_identity_key FROM kb.semantic_assertions WHERE subject_ref_id = $1 LIMIT 1`, objectID).Scan(&logicalKey); err != nil {
		t.Fatalf("find assertion logical identity key for %s: %v", objectID, err)
	}
	assertion, err := (AssertionStore{DB: db}).GetLatest(context.Background(), logicalKey)
	if err != nil {
		t.Fatalf("load assertion by logical identity key %s: %v", logicalKey, err)
	}
	return assertion
}

// TestIntegrationWriteMetricLosslessSignatureMatchesExistingClassBySubject
// locks in ADR 2026082203 DR5's primary case: an occurrence whose resolved
// "subject" dimension agrees with an existing class's stored signature
// reuses that class (ClassResolvedExisting) instead of hashing metric_name
// into a brand-new provisional class -- even though this occurrence's name
// has never been seen before and would hash to a different candidate term.
func TestIntegrationWriteMetricLosslessSignatureMatchesExistingClassBySubject(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-sig-1")
	seedGovernedTermWithProperties(t, db, "measurement:seeded_ambient_temp", "metric_definition", "measurement", "included_in_release",
		map[string]any{"subject": signatureEntry("ambient", "measurement:subj_ambient")})

	numeric := 21.0
	p := metricCandidatePayload{
		MetricID:             "m-sig-1",
		MetricName:           "Never-Before-Seen Metric Name",
		SubjectObjectID:      "obj-sig-1",
		RawText:              "21",
		ValueForm:            "single",
		NumericValue:         &numeric,
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
		ExtraProperties: map[string]any{
			"subject": signatureEntry("ambient", "measurement:subj_ambient"),
		},
	}
	dc := proposeMetricCandidate(t, db, "metric:m-sig-1", "m-sig-1", 500, p)

	a := AssociateSemantics{DB: db}
	if _, err := a.writeMetricLossless(ctx, dc, p, 500, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless: %v", err)
	}

	assertion := mustFindAssertionByObject(t, db, "obj-sig-1")
	if assertion.InstanceOfTermID != "measurement:seeded_ambient_temp" {
		t.Fatalf("instance_of_term_id = %q, want the seeded class reused via signature match", assertion.InstanceOfTermID)
	}
	if assertion.ClassIdentityStateTermID != semantic.ClassResolvedExisting {
		t.Fatalf("class_identity_state = %q, want resolved_existing", assertion.ClassIdentityStateTermID)
	}

	var newClassCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.ontology_term_headers WHERE term_id LIKE 'measurement:auto:defname_%'`).Scan(&newClassCount); err != nil {
		t.Fatalf("count name-hash provisional classes: %v", err)
	}
	if newClassCount != 0 {
		t.Fatalf("name-hash provisional classes created = %d, want 0 (signature match should have short-circuited the fallback entirely)", newClassCount)
	}
}

// TestIntegrationWriteMetricLosslessSignatureDisagreementExcludesCandidate
// locks in design.md D3's "no contradiction" rule: a class that disagrees
// with the occurrence on one shared resolved dimension is excluded
// entirely, even though it also carries an unrelated dimension the
// occurrence didn't resolve -- and even though a *different* class with
// weaker (single-dimension) agreement is available and correctly wins
// instead.
func TestIntegrationWriteMetricLosslessSignatureDisagreementExcludesCandidate(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-sig-2")

	// Disagreeing candidate: agrees on subject, but its value_class
	// contradicts the occurrence's resolved value_class.
	seedGovernedTermWithProperties(t, db, "measurement:seeded_disagreeing_class", "metric_definition", "measurement", "included_in_release",
		map[string]any{
			"subject":     signatureEntry("soil", "measurement:subj_soil"),
			"value_class": signatureEntry("threshold", "measurement:vc_threshold"),
		})
	// Weaker but non-contradicting candidate: agrees on subject only
	// (value_class is absent from its signature, a don't-care, not a
	// mismatch).
	seedGovernedTermWithProperties(t, db, "measurement:seeded_agreeing_class", "metric_definition", "measurement", "included_in_release",
		map[string]any{"subject": signatureEntry("soil", "measurement:subj_soil")})

	numeric := 5.0
	p := metricCandidatePayload{
		MetricID:             "m-sig-2",
		MetricName:           "Disagreement Probe",
		SubjectObjectID:      "obj-sig-2",
		RawText:              "5",
		ValueForm:            "single",
		NumericValue:         &numeric,
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
		ExtraProperties: map[string]any{
			"subject":     signatureEntry("soil", "measurement:subj_soil"),
			"value_class": signatureEntry("requirement", "measurement:vc_requirement"),
		},
	}
	dc := proposeMetricCandidate(t, db, "metric:m-sig-2", "m-sig-2", 501, p)

	a := AssociateSemantics{DB: db}
	if _, err := a.writeMetricLossless(ctx, dc, p, 501, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless: %v", err)
	}

	assertion := mustFindAssertionByObject(t, db, "obj-sig-2")
	if assertion.InstanceOfTermID != "measurement:seeded_agreeing_class" {
		t.Fatalf("instance_of_term_id = %q, want the non-contradicting class, not the disagreeing one", assertion.InstanceOfTermID)
	}
	if assertion.ClassIdentityStateTermID != semantic.ClassResolvedExisting {
		t.Fatalf("class_identity_state = %q, want resolved_existing", assertion.ClassIdentityStateTermID)
	}
}

// TestIntegrationWriteMetricLosslessSignatureTieCreatesAmbiguousProvisional
// locks in design.md D3's second gap-fix: two existing classes tied at the
// same non-zero shared-agreeing-dimension count are never guessed between.
// The occurrence gets a brand-new provisional class instead, flagged
// ClassAmbiguousCandidates, with both tied candidates recorded as
// alternatives on the class-resolution decision (class-resolution-decisions
// spec's "signature ties never silently pick a winner" requirement).
func TestIntegrationWriteMetricLosslessSignatureTieCreatesAmbiguousProvisional(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-sig-3")

	seedGovernedTermWithProperties(t, db, "measurement:seeded_tie_a", "metric_definition", "measurement", "included_in_release",
		map[string]any{"subject": signatureEntry("water", "measurement:subj_water")})
	seedGovernedTermWithProperties(t, db, "measurement:seeded_tie_b", "metric_definition", "measurement", "included_in_release",
		map[string]any{"value_class": signatureEntry("requirement", "measurement:vc_requirement")})

	numeric := 3.0
	p := metricCandidatePayload{
		MetricID:             "m-sig-3",
		MetricName:           "Tie Probe",
		SubjectObjectID:      "obj-sig-3",
		RawText:              "3",
		ValueForm:            "single",
		NumericValue:         &numeric,
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
		ExtraProperties: map[string]any{
			"subject":     signatureEntry("water", "measurement:subj_water"),
			"value_class": signatureEntry("requirement", "measurement:vc_requirement"),
		},
	}
	dc := proposeMetricCandidate(t, db, "metric:m-sig-3", "m-sig-3", 502, p)

	a := AssociateSemantics{DB: db}
	if _, err := a.writeMetricLossless(ctx, dc, p, 502, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless: %v", err)
	}

	assertion := mustFindAssertionByObject(t, db, "obj-sig-3")
	if assertion.InstanceOfTermID == "measurement:seeded_tie_a" || assertion.InstanceOfTermID == "measurement:seeded_tie_b" {
		t.Fatalf("instance_of_term_id = %q, want a brand-new provisional class, not either tied candidate", assertion.InstanceOfTermID)
	}
	if assertion.ClassIdentityStateTermID != semantic.ClassAmbiguousCandidates {
		t.Fatalf("class_identity_state = %q, want ambiguous_candidates", assertion.ClassIdentityStateTermID)
	}

	var altCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.ontology_class_resolution_alternatives alt
JOIN kb.ontology_class_resolution_decisions d ON d.id = alt.decision_id
WHERE d.source_artifact_id = 'm-sig-3'`).Scan(&altCount); err != nil {
		t.Fatalf("count recorded alternatives: %v", err)
	}
	if altCount != 2 {
		t.Fatalf("recorded alternatives = %d, want 2 (both tied candidates)", altCount)
	}
}

// TestIntegrationWriteMetricLosslessZeroResolvedDimensionsSignatureIsNoop
// locks in design.md D3 gap-fix 1 explicitly for this ADR: an occurrence
// with no identity-shaped ExtraProperties entries (only plain, non-identity
// values, as a misconfigured or non-identity field would produce) never
// attempts a signature match and falls straight through to today's
// unchanged name-hash fallback -- proving matchClassBySignature is a true
// no-op addition, not a behavior change, when nothing resolved.
func TestIntegrationWriteMetricLosslessZeroResolvedDimensionsSignatureIsNoop(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-sig-4")
	// A class exists in the catalog, but nothing on the occurrence resolved
	// to a non-null term_id, so it must never be considered a candidate.
	seedGovernedTermWithProperties(t, db, "measurement:seeded_unrelated", "metric_definition", "measurement", "included_in_release",
		map[string]any{"subject": signatureEntry("ambient", "measurement:subj_ambient")})

	numeric := 1.0
	p := metricCandidatePayload{
		MetricID:             "m-sig-4",
		MetricName:           "No Signal Probe",
		SubjectObjectID:      "obj-sig-4",
		RawText:              "1",
		ValueForm:            "single",
		NumericValue:         &numeric,
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
		ExtraProperties: map[string]any{
			"non_identity_field": "plain value, not a {raw,term_id} map",
		},
	}
	dc := proposeMetricCandidate(t, db, "metric:m-sig-4", "m-sig-4", 503, p)

	a := AssociateSemantics{DB: db}
	if _, err := a.writeMetricLossless(ctx, dc, p, 503, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless: %v", err)
	}

	assertion := mustFindAssertionByObject(t, db, "obj-sig-4")
	if assertion.InstanceOfTermID == "measurement:seeded_unrelated" {
		t.Fatalf("instance_of_term_id = %q, want a fresh name-hash class, not the unrelated seeded one", assertion.InstanceOfTermID)
	}
	if assertion.ClassIdentityStateTermID != semantic.ClassProvisionalNew {
		t.Fatalf("class_identity_state = %q, want provisional_new (unchanged fallback behavior)", assertion.ClassIdentityStateTermID)
	}
}
