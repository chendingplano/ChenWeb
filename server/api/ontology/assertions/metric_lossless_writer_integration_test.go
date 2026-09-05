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

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
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

// signatureEntry builds one BuildConfiguredProperties-shaped
// {"raw":..., "resolved":...} ExtraProperties value (openspec change
// governed-property-normalization; was {"raw":..,"term_id":..} under ADR
// 2026082203 DR2). The second parameter name stays "resolved" in spirit --
// callers pass whatever value the field's method (strong/simple/system)
// produced, a keyword concept id or a canonical bucket string alike.
func signatureEntry(raw, resolved string) map[string]any {
	return map[string]any{"raw": raw, "resolved": resolved}
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
	// to a non-empty value, so it must never be considered a candidate.
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
			"non_identity_field": "plain value, not a {raw,resolved} map",
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

// TestIntegrationWriteMetricLosslessCreatesContractHeaderForNewClass locks in
// metric-class-contracts' EnsureHeader wiring: a class resolved by this write
// path gets exactly one identity_only contract revision, not just a
// kb.ontology_term_headers row (which the pre-existing
// kb_sync_ontology_term_revision_after_insert trigger already provides for
// free on the kb.ontology_terms insert -- the contract revision is the actual
// gap this change closes).
func TestIntegrationWriteMetricLosslessCreatesContractHeaderForNewClass(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-contract-1")

	numeric := 500.0
	p := metricCandidatePayload{
		MetricID:             "m-contract-1",
		MetricName:           "Contract Header Probe",
		SubjectObjectID:      "obj-contract-1",
		RawText:              "500",
		ValueForm:            "single",
		NumericValue:         &numeric,
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
	}
	dc := proposeMetricCandidate(t, db, "metric:m-contract-1", "m-contract-1", 601, p)

	a := AssociateSemantics{DB: db}
	if _, err := a.writeMetricLossless(ctx, dc, p, 601, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless: %v", err)
	}

	assertion := mustFindAssertionByObject(t, db, "obj-contract-1")
	var revisionCount int
	var definitionState string
	if err := db.QueryRowContext(ctx, `
SELECT count(*), max(r.definition_state)
FROM kb.ontology_class_contract_revisions r
WHERE r.term_id = $1`, assertion.InstanceOfTermID).Scan(&revisionCount, &definitionState); err != nil {
		t.Fatalf("count contract revisions: %v", err)
	}
	if revisionCount != 1 {
		t.Fatalf("contract revisions for %s = %d, want 1", assertion.InstanceOfTermID, revisionCount)
	}
	if definitionState != classfoundation.DefinitionIdentityOnly {
		t.Fatalf("definition_state = %q, want identity_only", definitionState)
	}
	var currentRevisionSet bool
	if err := db.QueryRowContext(ctx, `
SELECT current_contract_revision_id IS NOT NULL FROM kb.ontology_term_headers WHERE term_id = $1`,
		assertion.InstanceOfTermID).Scan(&currentRevisionSet); err != nil {
		t.Fatalf("check current_contract_revision_id: %v", err)
	}
	if !currentRevisionSet {
		t.Fatal("kb.ontology_term_headers.current_contract_revision_id was not set")
	}
}

// TestIntegrationWriteMetricLosslessReusedClassGetsOneContractRevision proves
// EnsureHeader's idempotency at the actual call site: two metrics resolving
// to the same class (same ConceptID, so the same synthesized/aligned term)
// must not accumulate a second identity_only revision on the second write.
// This payload's two writes also happen to agree on datatype/unit from two
// distinct documents, which legitimately promotes the contract to
// partially_defined on the second write (metric_capability_validators_test.go
// and contract_synthesis_integration_test.go cover that mechanism directly)
// -- so this test checks the identity_only-revision count specifically,
// not the total revision count, to isolate the property it's actually about.
func TestIntegrationWriteMetricLosslessReusedClassGetsOneContractRevision(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedGovernedTerm(t, db, "core:aligns_to_term", "property", "core")
	seedObjectNode(t, db, "obj-contract-2a")
	seedObjectNode(t, db, "obj-contract-2b")

	basePayload := metricCandidatePayload{
		MetricName:           "Reused Class Probe",
		ConceptID:            "kwc_reused_class_probe",
		Definition:           "A metric occurrence written twice under the same concept.",
		ValueForm:            "single",
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
	}
	a := AssociateSemantics{DB: db}

	first := basePayload
	first.MetricID = "m-contract-2a"
	first.SubjectObjectID = "obj-contract-2a"
	first.RawText = "10"
	v1 := 10.0
	first.NumericValue = &v1
	dc1 := proposeMetricCandidate(t, db, "metric:m-contract-2a", "m-contract-2a", 602, first)
	if _, err := a.writeMetricLossless(ctx, dc1, first, 602, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless (first): %v", err)
	}

	second := basePayload
	second.MetricID = "m-contract-2b"
	second.SubjectObjectID = "obj-contract-2b"
	second.RawText = "20"
	v2 := 20.0
	second.NumericValue = &v2
	dc2 := proposeMetricCandidate(t, db, "metric:m-contract-2b", "m-contract-2b", 603, second)
	if _, err := a.writeMetricLossless(ctx, dc2, second, 603, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless (second): %v", err)
	}

	assertionA := mustFindAssertionByObject(t, db, "obj-contract-2a")
	assertionB := mustFindAssertionByObject(t, db, "obj-contract-2b")
	if assertionA.InstanceOfTermID != assertionB.InstanceOfTermID {
		t.Fatalf("expected both occurrences to resolve to the same class, got %q and %q", assertionA.InstanceOfTermID, assertionB.InstanceOfTermID)
	}

	var identityOnlyCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.ontology_class_contract_revisions WHERE term_id = $1 AND definition_state = 'identity_only'`,
		assertionA.InstanceOfTermID).Scan(&identityOnlyCount); err != nil {
		t.Fatalf("count identity_only contract revisions: %v", err)
	}
	if identityOnlyCount != 1 {
		t.Fatalf("identity_only contract revisions after two writes to the same class = %d, want still 1 (no duplicate)", identityOnlyCount)
	}
}

// TestIntegrationWriteMetricLosslessBackfillsContractForPreExistingClass
// proves EnsureHeader also repairs a class term that predates this change --
// created directly via terms.TermStore (so the insert trigger already gave
// it a kb.ontology_term_headers row) but with no contract revision behind
// it, matching the live shape found in `miner` (4,639 headers, 0 with a
// contract) before this change.
func TestIntegrationWriteMetricLosslessBackfillsContractForPreExistingClass(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedObjectNode(t, db, "obj-contract-3")

	// Seed the class term the old way: a plain kb.ontology_terms insert (the
	// trigger mirrors it into kb.ontology_term_headers with no contract),
	// with no ConceptID and no signature-matchable properties, so
	// resolveOrCreateMetricClass's existing-term-reuse branch selects it by
	// its own deterministic hash-derived candidate term ID.
	preExistingPayload := metricCandidatePayload{MetricName: "Pre-Existing Class Probe"}
	preExistingTermID := provisionalMetricClassTermID(preExistingPayload)
	seedGovernedTerm(t, db, preExistingTermID, "metric_definition", "measurement")
	var headerExists, hasContract bool
	if err := db.QueryRowContext(ctx, `
SELECT true, current_contract_revision_id IS NOT NULL FROM kb.ontology_term_headers WHERE term_id = $1`,
		preExistingTermID).Scan(&headerExists, &hasContract); err != nil {
		t.Fatalf("precondition: header row must exist via the insert trigger: %v", err)
	}
	if !headerExists || hasContract {
		t.Fatalf("precondition failed: headerExists=%v hasContract=%v, want true/false", headerExists, hasContract)
	}

	numeric := 7.0
	p := metricCandidatePayload{
		MetricID:             "m-contract-3",
		MetricName:           "Pre-Existing Class Probe",
		SubjectObjectID:      "obj-contract-3",
		RawText:              "7",
		ValueForm:            "single",
		NumericValue:         &numeric,
		AssertionKind:        "observed_value",
		ValueRangeTypeLookup: "absent",
	}
	dc := proposeMetricCandidate(t, db, "metric:m-contract-3", "m-contract-3", 604, p)

	a := AssociateSemantics{DB: db}
	if _, err := a.writeMetricLossless(ctx, dc, p, 604, "mea:measured_by"); err != nil {
		t.Fatalf("writeMetricLossless: %v", err)
	}

	assertion := mustFindAssertionByObject(t, db, "obj-contract-3")
	if assertion.InstanceOfTermID != preExistingTermID {
		t.Fatalf("instance_of_term_id = %q, want the pre-existing class %q reused", assertion.InstanceOfTermID, preExistingTermID)
	}
	var revisionCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.ontology_class_contract_revisions WHERE term_id = $1`, preExistingTermID).Scan(&revisionCount); err != nil {
		t.Fatalf("count contract revisions: %v", err)
	}
	if revisionCount != 1 {
		t.Fatalf("contract revisions backfilled for pre-existing class = %d, want 1", revisionCount)
	}
}

// TestIntegrationWriteMetricLosslessObservedProfileEvidenceAndConformance is
// the end-to-end proof for tasks 3.2/3.3/6.2/7.1-7.3 of the metric-class-
// contracts change: it drives four writes to the same class through the real
// writeMetricLossless path and checks the observed-profile counts, the
// synthesis promotion, and each write's own conformance state against the
// contract as it stood at that write -- not retroactively.
func TestIntegrationWriteMetricLosslessObservedProfileEvidenceAndConformance(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "mea:measured_by", "property", "measurement")
	seedGovernedTerm(t, db, "mea:observed_value", "property", "measurement")
	seedGovernedTerm(t, db, "core:aligns_to_term", "property", "core")
	seedGovernedTerm(t, db, "quantity:unit_SEC", "unit", "quantity")
	seedGovernedTerm(t, db, "quantity:unit_COUNT", "unit", "quantity")
	for _, obj := range []string{"obj-evid-1", "obj-evid-2", "obj-evid-3", "obj-evid-4"} {
		seedObjectNode(t, db, obj)
	}

	a := AssociateSemantics{DB: db}
	write := func(metricID, objectID string, inputRecordID int64, unit string, value float64) Assertion {
		t.Helper()
		p := metricCandidatePayload{
			MetricID:             metricID,
			MetricName:           "Evidence Probe",
			ConceptID:            "kwc_evidence_probe",
			Definition:           "A metric written multiple times to exercise contract synthesis.",
			SubjectObjectID:      objectID,
			RawText:              fmt.Sprintf("%v %s", value, unit),
			ValueForm:            "single",
			ValueDataType:        "number",
			Unit:                 unit,
			NumericValue:         &value,
			AssertionKind:        "observed_value",
			ValueRangeTypeLookup: "absent",
		}
		dc := proposeMetricCandidate(t, db, "metric:"+metricID, metricID, inputRecordID, p)
		if _, err := a.writeMetricLossless(ctx, dc, p, inputRecordID, "mea:measured_by"); err != nil {
			t.Fatalf("writeMetricLossless(%s): %v", metricID, err)
		}
		return mustFindAssertionByObject(t, db, objectID)
	}

	// First write: only one document's evidence exists yet, so the class
	// stays identity_only and this instance is honestly not_evaluated.
	first := write("m-evid-1", "obj-evid-1", 701, "s", 10)
	if first.ConformanceStateTermID != semantic.ConformanceNotEvaluated {
		t.Fatalf("first write conformance = %q, want not_evaluated", first.ConformanceStateTermID)
	}
	var profileCount, observedCount, documentCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.ontology_observed_class_profiles WHERE class_term_id = $1`, first.InstanceOfTermID).Scan(&profileCount); err != nil {
		t.Fatalf("count observed profiles: %v", err)
	}
	if profileCount != 1 {
		t.Fatalf("observed profiles for class = %d, want 1", profileCount)
	}
	if err := db.QueryRowContext(ctx, `
SELECT a.observed_count, a.document_count FROM kb.ontology_observed_class_attribute_observations a
JOIN kb.ontology_observed_class_profiles p ON p.id = a.profile_id
WHERE p.class_term_id = $1 AND a.attribute_key = 'value'`, first.InstanceOfTermID).Scan(&observedCount, &documentCount); err != nil {
		t.Fatalf("load attribute observation: %v", err)
	}
	if observedCount != 1 || documentCount != 1 {
		t.Fatalf("attribute observation counts = (%d, %d), want (1, 1)", observedCount, documentCount)
	}

	// Second write: a second document agrees on datatype and unit. This
	// write's own conformance is still evaluated against the pre-promotion
	// (identity_only) contract, so it too is not_evaluated -- the promotion
	// this write causes applies going forward, not retroactively to itself.
	second := write("m-evid-2", "obj-evid-2", 702, "s", 20)
	if second.ConformanceStateTermID != semantic.ConformanceNotEvaluated {
		t.Fatalf("second write conformance = %q, want not_evaluated (promotion is not retroactive to the write that causes it)", second.ConformanceStateTermID)
	}
	if err := db.QueryRowContext(ctx, `
SELECT a.observed_count, a.document_count FROM kb.ontology_observed_class_attribute_observations a
JOIN kb.ontology_observed_class_profiles p ON p.id = a.profile_id
WHERE p.class_term_id = $1 AND a.attribute_key = 'value'`, first.InstanceOfTermID).Scan(&observedCount, &documentCount); err != nil {
		t.Fatalf("load attribute observation after second write: %v", err)
	}
	if observedCount != 2 || documentCount != 2 {
		t.Fatalf("attribute observation counts after second write = (%d, %d), want (2, 2) (incremented, not duplicated)", observedCount, documentCount)
	}
	var definitionState string
	if err := db.QueryRowContext(ctx, `
SELECT r.definition_state FROM kb.ontology_term_headers h
JOIN kb.ontology_class_contract_revisions r ON r.id = h.current_contract_revision_id
WHERE h.term_id = $1`, first.InstanceOfTermID).Scan(&definitionState); err != nil {
		t.Fatalf("load current definition_state: %v", err)
	}
	if definitionState != classfoundation.DefinitionPartiallyDefined {
		t.Fatalf("definition_state after two agreeing documents = %q, want partially_defined", definitionState)
	}
	var canValidateResult string
	if err := db.QueryRowContext(ctx, `
SELECT validation_result FROM kb.ontology_class_capability_validation_results r
JOIN kb.ontology_term_headers h ON h.current_contract_revision_id = r.contract_revision_id
WHERE h.term_id = $1 AND r.capability_term_id = $2`,
		first.InstanceOfTermID, classfoundation.CapabilityCanValidateValue).Scan(&canValidateResult); err != nil {
		t.Fatalf("load can_validate_value result: %v", err)
	}
	if canValidateResult != classfoundation.ValidationPass {
		t.Fatalf("can_validate_value result = %q, want pass", canValidateResult)
	}

	// Third write: the contract is now partially_defined declaring unit_SEC;
	// this instance's matching unit conforms.
	third := write("m-evid-3", "obj-evid-3", 703, "s", 30)
	if third.ConformanceStateTermID != semantic.Conforms {
		t.Fatalf("third write conformance = %q, want conforms", third.ConformanceStateTermID)
	}
	if third.Status != StatusRepresented {
		t.Fatalf("third write status = %q, want represented even though it conforms", third.Status)
	}

	// Fourth write: a different unit against the same partially_defined
	// contract violates it, and the contract itself is not reverted.
	fourth := write("m-evid-4", "obj-evid-4", 704, "count", 5)
	if fourth.ConformanceStateTermID != semantic.ConformanceContractViolation {
		t.Fatalf("fourth write conformance = %q, want conformance_contract_violation", fourth.ConformanceStateTermID)
	}
	if fourth.Status != StatusRepresented {
		t.Fatalf("fourth write status = %q, want represented even though it violates the contract", fourth.Status)
	}
	if err := db.QueryRowContext(ctx, `
SELECT r.definition_state FROM kb.ontology_term_headers h
JOIN kb.ontology_class_contract_revisions r ON r.id = h.current_contract_revision_id
WHERE h.term_id = $1`, first.InstanceOfTermID).Scan(&definitionState); err != nil {
		t.Fatalf("load definition_state after violation: %v", err)
	}
	if definitionState != classfoundation.DefinitionPartiallyDefined {
		t.Fatalf("definition_state after a contradicting write = %q, want unchanged partially_defined", definitionState)
	}
}
