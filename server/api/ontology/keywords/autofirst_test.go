package keywords

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// D11: the auto-created concept id is a content hash of (norm_key, scope) —
// deterministic so repeated targeted misses on the same name converge on one
// concept, and distinct names/scopes never collide.
func TestAutoConceptID(t *testing.T) {
	id := autoConceptID("luminance", "_")
	if id != autoConceptID("luminance", "_") {
		t.Error("autoConceptID must be deterministic")
	}
	if id == autoConceptID("luminance", "ventilator") {
		t.Error("different scopes must produce different concept ids")
	}
	if id == autoConceptID("brightness", "_") {
		t.Error("different norm keys must produce different concept ids")
	}
	if len(id) != len("kwc_")+12 {
		t.Errorf("expected kwc_ + 12 hex chars, got %q", id)
	}
}

// expectTotalMiss queues the read-only tier queries for a surface that
// matches nothing: tiers 0-3 all return empty row sets. Tier 4 is skipped
// when the norm key falls outside the initials rune gate, exactly as the
// production path does.
func expectTotalMiss(mock sqlmock.Sqlmock, ks semid.KeySet, scope string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT s.concept_id FROM kb.keyword_surfaces s WHERE s.surface =`)).
		WithArgs(ks.Exact, scope).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT s.concept_id, s.norm_key FROM kb.keyword_surfaces s WHERE s.norm_key =`)).
		WithArgs(ks.Norm, scope, semid.CurrentNormalizerVersion).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}))
	for _, pair := range []struct{ kind, value string }{
		{"alnum", ks.Alnum},
		{"sorted", ks.Sorted},
		{"singular", ks.Singular},
	} {
		if pair.value == "" {
			continue
		}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT s.concept_id, s.norm_key FROM kb.keyword_surface_keys sk`)).
			WithArgs(pair.kind, pair.value, scope, semid.CurrentNormalizerVersion).
			WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}))
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + rewriteRuleColumns + ` ` + rewriteRuleFrom + ` WHERE enabled = true AND scope =`)).
		WithArgs(scope).
		WillReturnRows(sqlmock.NewRows([]string{
			"rule_id", "pattern", "replacement", "scope", "enabled", "provenance", "create_time", "modify_time",
		}))
	if r := utf8.RuneCountInString(ks.Norm); r >= 2 && r <= 8 {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT s.concept_id, s.norm_key FROM kb.keyword_surface_keys sk`)).
			WithArgs("initials", ks.Norm, scope, semid.CurrentNormalizerVersion).
			WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}))
	}
}

// D11 auto-first: a targeted miss terminates in a decision — a provisional
// concept is minted (gloss_source "auto:d11"), the name becomes its pref
// surface (provenance "auto:resolver", keys derived server-side), the
// occurrence is assigned to the new concept with status unresolved
// (provisional is not governed, §9.5), the decision log captures the final
// resolution, and nothing queues in the backlog.
func TestObserveSurfaceTargetedMissAutoCreates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	ctx := context.Background()

	const surface = "Luminance"
	const scope = "_"
	ks := kf.normalizer().Normalize(surface)
	wantID := autoConceptID(ks.Norm, scope)

	expectTotalMiss(mock, ks, scope)

	// Auto-created provisional concept, marked for set-based sampling.
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_concepts`)).
		WithArgs(wantID, surface, nil, scope, "provisional", "auto:d11").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow(wantID, surface, nil, scope, "provisional", nil, "auto:d11", testNow, testNow))
	mock.ExpectCommit()

	// Its pref surface + derived keys in one transaction.
	sid := deriveSurfaceID(wantID, surface, "pref")
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_surfaces`)).
		WithArgs(sid, wantID, surface, ks.Norm, semid.CurrentNormalizerVersion,
			"pref", "pref", "und", scope, 1.0, "auto:resolver", false, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence", "provenance",
			"locked", "evidence", "create_time", "modify_time",
		}).AddRow(sid, wantID, surface, ks.Norm, semid.CurrentNormalizerVersion,
			"pref", "pref", "und", scope, 1.0, "auto:resolver", false, nil, testNow, testNow))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.keyword_surface_keys WHERE surface_id =`)).
		WithArgs(sid).
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, k := range derivedSurfaceKeys(ks, sid, semid.CurrentNormalizerVersion) {
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.keyword_surface_keys`)).
			WithArgs(sid, k.KeyKind, k.KeyValue, semid.CurrentNormalizerVersion).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	// Decision log captures the final resolution (verdict deferred + the
	// auto-created node in the serialized output).
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WithArgs("keyword", scope, sqlmock.AnyArg(), sqlmock.AnyArg(), "deferred",
			nil, nil, "keyword_family", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	// Occurrence: assigned to the auto-created concept, status unresolved,
	// linked to decision 42. No backlog row follows.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_occurrences`)).
		WithArgs("document", "art:1", "", surface, ks.Norm, scope,
			"the luminance of the display", nil, wantID, nil, "unresolved", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"occurrence_id"}).AddRow(7))

	res, err := kf.ObserveSurface(ctx, surface, scope, "document", "art:1", "the luminance of the display", true)
	if err != nil {
		t.Fatalf("ObserveSurface: %v", err)
	}
	if res.Verdict != semid.VerdictDeferred {
		t.Errorf("verdict: got %q, want deferred", res.Verdict)
	}
	if res.ResolvedNodeID != wantID {
		t.Errorf("ResolvedNodeID: got %q, want %q", res.ResolvedNodeID, wantID)
	}
	if res.Method != "auto_created" {
		t.Errorf("Method: got %q, want auto_created", res.Method)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// D11 scope limit: the mention collector path (targeted=false) keeps
// backlog-only behaviour — a miss queues in kb.keyword_unresolved and mints
// no concept, because the collector tokenizes all prose.
func TestObserveSurfaceCollectorMissQueuesBacklog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	ctx := context.Background()

	const surface = "Luminance"
	const scope = "_"
	ks := kf.normalizer().Normalize(surface)

	expectTotalMiss(mock, ks, scope)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WithArgs("keyword", scope, sqlmock.AnyArg(), sqlmock.AnyArg(), "deferred",
			nil, nil, "keyword_family", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))

	// Occurrence with no concept link — nothing was assigned.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_occurrences`)).
		WithArgs("document", "art:1", "", surface, ks.Norm, scope,
			"ctx", nil, nil, nil, "unresolved", int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"occurrence_id"}).AddRow(8))

	// Backlog upsert: no existing row → insert keyed on the norm key (K5).
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT surfaces, contexts, hits FROM kb.keyword_unresolved`)).
		WithArgs(ks.Norm, scope).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.keyword_unresolved`)).
		WithArgs(ks.Norm, scope, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	res, err := kf.ObserveSurface(ctx, surface, scope, "document", "art:1", "ctx", false)
	if err != nil {
		t.Fatalf("ObserveSurface: %v", err)
	}
	if res.ResolvedNodeID != "" {
		t.Errorf("collector miss must not assign a concept, got %q", res.ResolvedNodeID)
	}
	if res.Method != "" {
		t.Errorf("collector miss must carry no method, got %q", res.Method)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A concurrent targeted miss on the same name mints the same content-hashed
// concept id; the loser of the race converges on the winner's live concept
// instead of failing the call.
func TestAutoCreateConceptConvergesOnExisting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	kf.ensureDefaults()
	ctx := context.Background()

	const surface = "Luminance"
	const scope = "_"
	ks := kf.normalizer().Normalize(surface)
	wantID := autoConceptID(ks.Norm, scope)

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_concepts`)).
		WithArgs(wantID, surface, nil, scope, "provisional", "auto:d11").
		WillReturnError(errors.New("duplicate key value violates unique constraint"))
	mock.ExpectRollback()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id =`)).
		WithArgs(wantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow(wantID, surface, nil, scope, "provisional", nil, "auto:d11", testNow, testNow))

	got, err := kf.autoCreateConcept(ctx, surface, ks, scope)
	if err != nil {
		t.Fatalf("autoCreateConcept: %v", err)
	}
	if got != wantID {
		t.Errorf("got %q, want %q", got, wantID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// F11: if the loser of the auto-create race observes the winner's concept
// after reconciliation already merged it into a survivor, autoCreateConcept
// must chase the tombstone (FollowMerge) rather than hard-erroring on a
// "merged" status the caller has no way to act on.
func TestAutoCreateConceptFollowsMergeOnConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	kf.ensureDefaults()
	ctx := context.Background()

	const surface = "Luminance"
	const scope = "_"
	ks := kf.normalizer().Normalize(surface)
	wantID := autoConceptID(ks.Norm, scope)
	const survivorID = "kwc_survivor"

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_concepts`)).
		WithArgs(wantID, surface, nil, scope, "provisional", "auto:d11").
		WillReturnError(errors.New("duplicate key value violates unique constraint"))
	mock.ExpectRollback()
	// autoCreateConcept's own guard read: the id it just tried to mint is
	// already merged into survivorID.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id =`)).
		WithArgs(wantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow(wantID, surface, nil, scope, "merged", survivorID, "auto:d11", testNow, testNow))
	// FollowMerge re-reads wantID (cycle-guarded chase, D7 no-transitive-
	// closure — it always re-derives from the stored chain, never trusts a
	// cached hop), then reads the survivor, which is live.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id =`)).
		WithArgs(wantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow(wantID, surface, nil, scope, "merged", survivorID, "auto:d11", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id =`)).
		WithArgs(survivorID).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow(survivorID, "Luminance", nil, scope, "active", nil, "human:tester", testNow, testNow))

	got, err := kf.autoCreateConcept(ctx, surface, ks, scope)
	if err != nil {
		t.Fatalf("autoCreateConcept: %v", err)
	}
	if got != survivorID {
		t.Errorf("got %q, want the survivor %q", got, survivorID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
