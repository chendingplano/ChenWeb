package semantic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// These tests exercise the ADR 2026081801 Phase 1 schema against a live
// PostgreSQL database. They follow the fresh-scratch-database pattern
// established by server/api/ontology/seed/seed_integration_test.go rather than
// reusing chenweb_test: the invariants under test are partial unique indexes
// and deferred constraint triggers, and a shared database carrying rows from
// other tests would make "exactly one active row" untestable.
//
//	TEST_DATABASE_URL='host=127.0.0.1 user=cding dbname=postgres sslmode=disable' \
//	    go test ./server/api/ontology/semantic/ -run Integration -v
//
// The connecting role needs CREATEDB.
func freshSemanticTestDB(t *testing.T) *sql.DB {
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

	name := fmt.Sprintf("semantic_phase1_%d_%d", time.Now().UnixNano(), rand.Intn(1_000_000))
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

func baseOutcome(artifactID string) Outcome {
	return Outcome{
		OutcomeKey:            OutcomeKey(1, "metric", artifactID, StageNormalize),
		ArtifactType:          "metric",
		ArtifactID:            artifactID,
		StageTermID:           StageNormalize,
		DispositionTermID:     DispositionNormalized,
		ExecutionStatus:       ExecutionCompleted,
		OutcomeCategory:       CategorySemanticSuccess,
		DependencyFingerprint: Dependencies{ParserVersion: "p-1"}.Fingerprint(),
		InputFingerprint:      "v1:input-a",
		ProcessorName:         "test",
	}
}

func mappingFinding(fp string) Finding {
	return Finding{
		FindingKey:            FindingKey("range_type_mapping", DimensionMapping),
		DimensionTermID:       DimensionMapping,
		FindingTermID:         FindingMappingUnresolved,
		SeverityTermID:        SeverityWarning,
		RetryStateTermID:      RetryPending,
		ErrorCode:             "MAPPING_UNRESOLVED",
		DependencyFingerprint: fp,
	}
}

// Task 4.1: an unchanged replay advances last_seen only. Without this, every
// re-run of a document would append a row per metric per stage and re-alert,
// which at 7,074 metrics is precisely the failure storm DR11 forbids.
func TestIntegrationUnchangedReplayReusesOutcome(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OutcomeStore{DB: db}
	ctx := context.Background()

	first, err := store.Record(ctx, baseOutcome("m-1"), nil)
	if err != nil {
		t.Fatalf("first record: %v", err)
	}
	if first.Reused {
		t.Fatal("first write must not report reuse")
	}

	time.Sleep(5 * time.Millisecond)
	second, err := store.Record(ctx, baseOutcome("m-1"), nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !second.Reused {
		t.Fatal("identical replay must reuse the existing envelope (DR4)")
	}
	if second.Outcome.ID != first.Outcome.ID {
		t.Fatalf("replay created a new row: %d != %d", second.Outcome.ID, first.Outcome.ID)
	}

	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE outcome_key = $1`,
		first.Outcome.OutcomeKey).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("replay appended rows: got %d, want 1", rows)
	}

	var lastSeen time.Time
	if err := db.QueryRow(`SELECT last_seen FROM kb.semantic_processing_outcomes WHERE id = $1`,
		first.Outcome.ID).Scan(&lastSeen); err != nil {
		t.Fatal(err)
	}
	if !lastSeen.After(first.Outcome.LastSeen) {
		t.Errorf("last_seen did not advance: %v vs %v", lastSeen, first.Outcome.LastSeen)
	}
}

// Task 4.1: a changed dependency supersedes transactionally and deactivates
// child findings in the same transaction (DR4).
func TestIntegrationChangedDependencySupersedesAndDeactivatesChildren(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OutcomeStore{DB: db}
	ctx := context.Background()

	oldFP := Dependencies{MappingRevision: "map-1"}.Fingerprint()
	first, err := store.Record(ctx, baseOutcome("m-2"), []Finding{mappingFinding(oldFP)})
	if err != nil {
		t.Fatalf("first record: %v", err)
	}
	if first.Outcome.FindingCount != 1 || first.Outcome.HighestSeverityTermID != SeverityWarning {
		t.Fatalf("summary not derived from children: count=%d severity=%q",
			first.Outcome.FindingCount, first.Outcome.HighestSeverityTermID)
	}

	newFP := Dependencies{MappingRevision: "map-2"}.Fingerprint()
	second, err := store.Record(ctx, baseOutcome("m-2"), []Finding{mappingFinding(newFP)})
	if err != nil {
		t.Fatalf("superseding record: %v", err)
	}
	if second.Reused {
		t.Fatal("a changed dependency must not be treated as a replay")
	}
	if second.Superseded == nil || *second.Superseded != first.Outcome.ID {
		t.Fatalf("supersession not recorded: %v", second.Superseded)
	}

	var activeOutcomes, activeFindings, totalOutcomes int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE outcome_key=$1 AND active`,
		first.Outcome.OutcomeKey).Scan(&activeOutcomes); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_findings WHERE outcome_id=$1 AND active`,
		first.Outcome.ID).Scan(&activeFindings); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE outcome_key=$1`,
		first.Outcome.OutcomeKey).Scan(&totalOutcomes); err != nil {
		t.Fatal(err)
	}
	if activeOutcomes != 1 {
		t.Errorf("active outcomes = %d, want exactly 1", activeOutcomes)
	}
	if activeFindings != 0 {
		t.Errorf("superseded outcome still has %d active findings", activeFindings)
	}
	// History is append-only: the superseded row remains.
	if totalOutcomes != 2 {
		t.Errorf("total outcome rows = %d, want 2 (append-only history)", totalOutcomes)
	}
}

// Task 4.1: one stage reports several independent findings under ONE envelope.
func TestIntegrationMultipleIndependentFindingsShareOneEnvelope(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OutcomeStore{DB: db}
	fp := Dependencies{ParserVersion: "p-1"}.Fingerprint()

	out := baseOutcome("m-3")
	out.DispositionTermID = DispositionRawPreserved
	out.OutcomeCategory = CategorySemanticFinding

	res, err := store.Record(context.Background(), out, []Finding{
		{FindingKey: FindingKey("range_type_mapping", DimensionMapping), DimensionTermID: DimensionMapping,
			FindingTermID: FindingMappingUnresolved, SeverityTermID: SeverityWarning, DependencyFingerprint: fp},
		{FindingKey: FindingKey("value_literal", DimensionValue), DimensionTermID: DimensionValue,
			FindingTermID: FindingDatatypeMismatch, SeverityTermID: SeverityError, DependencyFingerprint: fp},
		{FindingKey: FindingKey("assertion_conformance", DimensionConformance), DimensionTermID: DimensionConformance,
			FindingTermID: FindingContractViolation, SeverityTermID: SeverityError, DependencyFingerprint: fp},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if len(res.Findings) != 3 {
		t.Fatalf("wrote %d findings, want 3", len(res.Findings))
	}
	if res.Outcome.FindingCount != 3 {
		t.Errorf("finding_count = %d, want 3", res.Outcome.FindingCount)
	}
	if res.Outcome.HighestSeverityTermID != SeverityError {
		t.Errorf("highest severity = %q, want error", res.Outcome.HighestSeverityTermID)
	}
	var envelopes int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE artifact_id='m-3'`).
		Scan(&envelopes); err != nil {
		t.Fatal(err)
	}
	if envelopes != 1 {
		t.Errorf("three findings produced %d envelopes, want 1 (DR4)", envelopes)
	}
}

// Task 4.1: two artifacts never share an outcome envelope or a finding row,
// even when they exhibit the identical unresolved vocabulary value.
func TestIntegrationTwoArtifactsNeverShareAnEnvelope(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OutcomeStore{DB: db}
	fp := Dependencies{MappingRevision: "map-1"}.Fingerprint()
	ctx := context.Background()

	for _, id := range []string{"m-a", "m-b"} {
		out := baseOutcome(id)
		out.OutcomeCategory = CategorySemanticFinding
		out.DispositionTermID = DispositionRawPreserved
		if _, err := store.Record(ctx, out, []Finding{mappingFinding(fp)}); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}
	var envelopes, findings int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE active`).Scan(&envelopes); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_findings WHERE active`).Scan(&findings); err != nil {
		t.Fatal(err)
	}
	if envelopes != 2 || findings != 2 {
		t.Fatalf("envelopes=%d findings=%d, want 2 and 2: outcomes and findings are occurrence-specific (DR11)", envelopes, findings)
	}
}

// Task 4.2: a completed semantic-stage outcome must identify its artifact, and
// only a failed source_or_output_unrecoverable invocation may omit it.
func TestIntegrationCompletedOutcomeRequiresArtifact(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OutcomeStore{DB: db}
	ctx := context.Background()

	bad := baseOutcome("")
	if _, err := store.Record(ctx, bad, nil); err == nil {
		t.Fatal("a completed outcome with no artifact_id must be rejected (DR4)")
	}

	unrecoverable := baseOutcome("")
	unrecoverable.ExecutionStatus = ExecutionFailed
	unrecoverable.OutcomeCategory = CategorySourceOrOutputUnrecoverable
	unrecoverable.DispositionTermID = DispositionNoResult
	if _, err := store.Record(ctx, unrecoverable, nil); err != nil {
		t.Fatalf("failed unrecoverable invocation should be recordable without an artifact: %v", err)
	}

	// Task 4.10 / DR13: and it must NOT create an unresolved occurrence.
	var occurrences int
	if err := db.QueryRow(`SELECT count(*) FROM kb.unresolved_semantic_occurrences`).Scan(&occurrences); err != nil {
		t.Fatal(err)
	}
	if occurrences != 0 {
		t.Fatalf("an unidentified failed invocation created %d unresolved occurrence(s); DR13 forbids fabricating one", occurrences)
	}
}

func TestIntegrationCategoryAndExecutionStatusMustAgree(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OutcomeStore{DB: db}
	out := baseOutcome("m-4")
	out.OutcomeCategory = CategorySystemFailure // implies failed
	out.ExecutionStatus = ExecutionCompleted    // contradiction
	if _, err := store.Record(context.Background(), out, nil); err == nil {
		t.Fatal("a system_failure category with completed status must be rejected (DR3)")
	}
}

func TestIntegrationRecordRejectsUngovernedIdentifiers(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OutcomeStore{DB: db}
	ctx := context.Background()

	out := baseOutcome("m-5")
	out.DispositionTermID = "raw-preserved" // hyphenated display label
	if _, err := store.Record(ctx, out, nil); err == nil {
		t.Fatal("a hyphenated disposition alias must be rejected (DR9)")
	}

	ok := baseOutcome("m-5")
	ok.OutcomeCategory = CategorySemanticFinding
	bad := mappingFinding(Dependencies{}.Fingerprint())
	bad.SeverityTermID = "warning"
	if _, err := store.Record(ctx, ok, []Finding{bad}); err == nil {
		t.Fatal("a hyphen-free but ungoverned severity must still be rejected (DR9)")
	}
}

func TestIntegrationDuplicateFindingKeyInOneOutcomeRejected(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OutcomeStore{DB: db}
	fp := Dependencies{}.Fingerprint()
	out := baseOutcome("m-6")
	out.OutcomeCategory = CategorySemanticFinding
	if _, err := store.Record(context.Background(), out, []Finding{mappingFinding(fp), mappingFinding(fp)}); err == nil {
		t.Fatal("two findings with the same finding_key in one outcome must be rejected")
	}
}

// Task 4.4 / DR10: enqueue is idempotent, claims are exclusive, stale jobs
// write nothing, and expired leases are reclaimable.
func TestIntegrationRetryQueueIdempotencyAndLeases(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()
	store := OutcomeStore{DB: db}
	queue := RetryQueue{DB: db}

	fp := Dependencies{MappingRevision: "map-1"}.Fingerprint()
	out := baseOutcome("m-7")
	out.OutcomeCategory = CategorySemanticFinding
	out.DispositionTermID = DispositionRawPreserved
	res, err := store.Record(ctx, out, []Finding{mappingFinding(fp)})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	target := Dependencies{MappingRevision: "map-2"}.Fingerprint()
	job := RetryJob{
		OutcomeID:                   res.Outcome.ID,
		FindingID:                   &res.Findings[0].ID,
		TargetDependencyFingerprint: target,
		SourceInputFingerprint:      res.Outcome.InputFingerprint,
	}
	first, reused, err := queue.Enqueue(ctx, job)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if reused {
		t.Fatal("first enqueue must not report reuse")
	}
	_, reused, err = queue.Enqueue(ctx, job)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if !reused {
		t.Fatal("concurrent enqueue of the same target must be an idempotent conflict (DR10)")
	}

	// Whole-stage retry uses a null finding_id; NULLS NOT DISTINCT must make
	// two of those collide too.
	whole := RetryJob{OutcomeID: res.Outcome.ID, TargetDependencyFingerprint: target}
	if _, _, err := queue.Enqueue(ctx, whole); err != nil {
		t.Fatalf("whole-stage enqueue: %v", err)
	}
	if _, reused, err := queue.Enqueue(ctx, whole); err != nil || !reused {
		t.Fatalf("duplicate whole-stage enqueue: reused=%v err=%v; NULLS NOT DISTINCT should collide", reused, err)
	}

	claimed, err := queue.Claim(ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.LeaseToken != "worker-1" || claimed.Attempts != 1 {
		t.Fatalf("claim did not record a lease/attempt: %+v", claimed)
	}
	// A second worker must not get the same job while the lease holds.
	other, err := queue.Claim(ctx, "worker-2", time.Minute)
	if err == nil && other.ID == claimed.ID {
		t.Fatal("two workers claimed the same job: leases are not exclusive")
	}

	// Staleness on both axes.
	if !claimed.IsStale(StalenessCheck{
		CurrentInputFingerprint:      "v1:input-CHANGED",
		CurrentDependencyFingerprint: target,
	}) {
		t.Error("a changed source input must make the job stale (DR10)")
	}
	if !claimed.IsStale(StalenessCheck{
		CurrentInputFingerprint:      res.Outcome.InputFingerprint,
		CurrentDependencyFingerprint: Dependencies{MappingRevision: "map-3"}.Fingerprint(),
	}) {
		t.Error("a dependency that moved past the target must make the job stale")
	}
	if claimed.IsStale(StalenessCheck{
		CurrentInputFingerprint:      res.Outcome.InputFingerprint,
		CurrentDependencyFingerprint: target,
	}) {
		t.Error("a matching job must not be stale")
	}

	if err := queue.MarkStale(ctx, claimed.ID, "dependency moved"); err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM kb.semantic_retry_queue WHERE id=$1`, claimed.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != RetryJobStale {
		t.Fatalf("state = %q, want stale", state)
	}
	_ = first

	// Expired lease is reclaimable: this is what makes a crashed worker safe.
	if _, err := db.Exec(`UPDATE kb.semantic_retry_queue
		SET state='claimed', lease_token='dead-worker', lease_expires_at = NOW() - interval '1 hour'
		WHERE id=$1`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := queue.Claim(ctx, "worker-3", time.Minute)
	if err != nil {
		t.Fatalf("reclaim after lease expiry: %v", err)
	}
	if reclaimed.ID != claimed.ID {
		t.Fatalf("reclaimed job %d, want %d", reclaimed.ID, claimed.ID)
	}
}

// Task 4.4 / DR10: an unchanged dependency sweep must enqueue nothing.
func TestIntegrationScheduleForDependencyChangeTargetsOnlyAffected(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()
	store := OutcomeStore{DB: db}
	queue := RetryQueue{DB: db}

	currentFP := Dependencies{MappingRevision: "map-1"}.Fingerprint()
	for _, id := range []string{"m-8", "m-9"} {
		out := baseOutcome(id)
		out.OutcomeCategory = CategorySemanticFinding
		out.DispositionTermID = DispositionRawPreserved
		if _, err := store.Record(ctx, out, []Finding{mappingFinding(currentFP)}); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}

	// Unchanged sweep: the target equals what the findings already carry.
	n, err := queue.ScheduleForDependencyChange(ctx, FindingMappingUnresolved, currentFP)
	if err != nil {
		t.Fatalf("unchanged sweep: %v", err)
	}
	if n != 0 {
		t.Fatalf("unchanged dependency enqueued %d job(s); DR10 requires none", n)
	}

	// A genuine mapping approval changes the fingerprint and schedules both.
	newFP := Dependencies{MappingRevision: "map-2"}.Fingerprint()
	n, err = queue.ScheduleForDependencyChange(ctx, FindingMappingUnresolved, newFP)
	if err != nil {
		t.Fatalf("changed sweep: %v", err)
	}
	if n != 2 {
		t.Fatalf("changed dependency enqueued %d job(s), want 2", n)
	}
	// Re-running the same sweep must not duplicate work.
	n, err = queue.ScheduleForDependencyChange(ctx, FindingMappingUnresolved, newFP)
	if err != nil {
		t.Fatalf("repeat sweep: %v", err)
	}
	if n != 0 {
		t.Fatalf("repeating the sweep enqueued %d more job(s), want 0", n)
	}

	// A finding term nobody reported must schedule nothing.
	n, err = queue.ScheduleForDependencyChange(ctx, FindingSourceConflict, newFP)
	if err != nil {
		t.Fatalf("unrelated sweep: %v", err)
	}
	if n != 0 {
		t.Fatalf("unrelated finding term enqueued %d job(s), want 0", n)
	}
}

// Task 4.2 / DR13: the fallback applies only to an identified artifact.
func TestIntegrationOccurrenceRequiresIdentifiedArtifact(t *testing.T) {
	db := freshSemanticTestDB(t)
	store := OccurrenceStore{DB: db}
	occ := UnresolvedOccurrence{
		OccurrenceKey:         OccurrenceKey("widget:v1", 1, "widget", ""),
		ArtifactType:          "widget",
		ArtifactID:            "",
		InputFingerprint:      "v1:in",
		DependencyFingerprint: "v1:dep",
	}
	if _, _, err := store.Upsert(context.Background(), occ); err == nil {
		t.Fatal("an occurrence with no artifact_id must be refused (DR13)")
	}
}

// Task 4.4: occurrence replay, supersession, lease claim, and transactional
// materialization.
func TestIntegrationOccurrenceLifecycle(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()
	store := OccurrenceStore{DB: db}

	key := OccurrenceKey("widget:v1", 1, "widget", "w-1")
	recordID := int64(1)
	occ := UnresolvedOccurrence{
		OccurrenceKey:         key,
		InputRecordID:         &recordID,
		ArtifactType:          "widget",
		ArtifactID:            "w-1",
		RawPayload:            json.RawMessage(`{"raw":"3 to 5 mm"}`),
		InputFingerprint:      "v1:in-1",
		DependencyFingerprint: "v1:dep-1",
	}
	created, reused, err := store.Upsert(ctx, occ)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if reused {
		t.Fatal("first upsert must not report reuse")
	}

	if _, reused, err = store.Upsert(ctx, occ); err != nil || !reused {
		t.Fatalf("identical replay: reused=%v err=%v", reused, err)
	}

	changed := occ
	changed.InputFingerprint = "v1:in-2"
	superseding, reused, err := store.Upsert(ctx, changed)
	if err != nil {
		t.Fatalf("superseding upsert: %v", err)
	}
	if reused {
		t.Fatal("a changed input must not be a replay")
	}
	if superseding.SupersedesOccurrenceID == nil || *superseding.SupersedesOccurrenceID != created.ID {
		t.Fatalf("supersession not recorded: %v", superseding.SupersedesOccurrenceID)
	}
	var active int
	if err := db.QueryRow(`SELECT count(*) FROM kb.unresolved_semantic_occurrences WHERE occurrence_key=$1 AND active`,
		key).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active occurrences = %d, want exactly 1", active)
	}

	claimed, err := store.Claim(ctx, "widget", "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.LeaseToken != "worker-1" {
		t.Fatalf("lease token = %q", claimed.LeaseToken)
	}

	// A worker whose lease was taken over must not be able to commit.
	if _, err := store.Materialize(ctx, claimed, "stale-worker", func(context.Context, *sql.Tx, UnresolvedOccurrence) (int64, int64, error) {
		t.Fatal("materialize body must not run for a stale lease")
		return 0, 0, nil
	}); err == nil {
		t.Fatal("materializing with a stale lease token must fail")
	}

	assertionID := seedAssertion(t, db)
	outcomeID := seedOutcomeRow(t, db, "widget", "w-1")
	materialized, err := store.Materialize(ctx, claimed, "worker-1",
		func(context.Context, *sql.Tx, UnresolvedOccurrence) (int64, int64, error) {
			return assertionID, outcomeID, nil
		})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if materialized.MaterializationState != MaterializationMaterialized {
		t.Fatalf("state = %q, want materialized", materialized.MaterializationState)
	}
	// DR13: a materialized assertion may never be paired with an ACTIVE
	// unresolved occurrence. The chk_unresolved_occurrence_materialized_inactive
	// constraint plus this assertion together make that unreachable.
	if materialized.Active {
		t.Fatal("a materialized occurrence must not remain active (DR13)")
	}
	if materialized.ResultingAssertionID == nil || *materialized.ResultingAssertionID != assertionID {
		t.Fatal("materialization did not record its resulting assertion")
	}
}

// A rollback inside the materialization body must leave nothing behind: the
// occurrence stays active and claimable, and no partial writes survive.
func TestIntegrationMaterializationRollbackLeavesOccurrenceActive(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()
	store := OccurrenceStore{DB: db}

	recordID := int64(1)
	occ := UnresolvedOccurrence{
		OccurrenceKey:         OccurrenceKey("widget:v1", 1, "widget", "w-2"),
		InputRecordID:         &recordID,
		ArtifactType:          "widget",
		ArtifactID:            "w-2",
		InputFingerprint:      "v1:in",
		DependencyFingerprint: "v1:dep",
	}
	if _, _, err := store.Upsert(ctx, occ); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	claimed, err := store.Claim(ctx, "widget", "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	boom := errors.New("class resolution unavailable")
	if _, err := store.Materialize(ctx, claimed, "worker-1",
		func(ctx context.Context, tx *sql.Tx, o UnresolvedOccurrence) (int64, int64, error) {
			// A partial write that must not survive the rollback.
			if _, err := tx.Exec(`INSERT INTO kb.semantic_processing_outcomes
				(outcome_key, artifact_type, artifact_id, stage_term_id, dependency_fingerprint, input_fingerprint)
				VALUES ('partial','widget','w-2',$1,'v1:d','v1:i')`, StageNormalize); err != nil {
				return 0, 0, err
			}
			return 0, 0, boom
		}); !errors.Is(err, boom) {
		t.Fatalf("materialize error = %v, want the body's error", err)
	}

	var partial int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE outcome_key='partial'`).
		Scan(&partial); err != nil {
		t.Fatal(err)
	}
	if partial != 0 {
		t.Fatalf("rollback left %d partial outcome row(s)", partial)
	}
	current, err := store.ActiveOccurrence(ctx, occ.OccurrenceKey)
	if err != nil {
		t.Fatalf("occurrence should still be active after rollback: %v", err)
	}
	if current.MaterializationState == MaterializationMaterialized {
		t.Fatal("a rolled-back materialization must not mark the occurrence materialized")
	}
}

func seedAssertion(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, status, value_state_term_id)
VALUES ('test:claim:1','object_node','obj-1','mea:measured_by','literal','{"value":1}'::jsonb,'represented',$1)
RETURNING id`, ValuePresent).Scan(&id); err != nil {
		t.Fatalf("seed assertion: %v", err)
	}
	return id
}

func seedOutcomeRow(t *testing.T, db *sql.DB, artifactType, artifactID string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
INSERT INTO kb.semantic_processing_outcomes
  (outcome_key, artifact_type, artifact_id, stage_term_id, dependency_fingerprint, input_fingerprint)
VALUES ($1,$2,$3,$4,'v1:dep','v1:in') RETURNING id`,
		OutcomeKey(1, artifactType, artifactID, StageAssociate), artifactType, artifactID, StageAssociate).
		Scan(&id); err != nil {
		t.Fatalf("seed outcome: %v", err)
	}
	return id
}
