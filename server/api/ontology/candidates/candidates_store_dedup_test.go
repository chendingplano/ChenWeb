package candidates

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// candidateMatchesArg matches a candidate_matches JSONB argument containing
// an entry that references wantCandidateID via identity_key -- used instead
// of an exact string match because entries carry a live detected_at
// timestamp.
type candidateMatchesArg struct {
	wantCandidateID int64
}

func (m candidateMatchesArg) Match(v driver.Value) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	var entries []candidateMatchEntry
	if err := json.Unmarshal([]byte(s), &entries); err != nil {
		return false
	}
	for _, e := range entries {
		if e.MatchType == "candidate" && e.CandidateID == m.wantCandidateID && e.MatchedOn == "identity_key" {
			return true
		}
	}
	return false
}

// TestCreateCandidateMatchesSwappedLabelAlias reproduces the bug doc's case
// (KnowledgeStore/doc-repo/bugs/202608/2026081101-...): two extraction
// passes over document 416 produced candidates whose label and sole alias
// swapped, so the fingerprint differed but both name the same metric.
func TestCreateCandidateMatchesSwappedLabelAlias(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payloadA := []byte(`{"term_kind":"metric_definition","label":"种子发芽指数","aliases":["发芽指数"]}`)
	payloadB := []byte(`{"term_kind":"metric_definition","label":"发芽指数","aliases":["种子发芽指数"]}`)
	fpA, err := Fingerprint(payloadA, "document", "input_record:416", "measurement")
	if err != nil {
		t.Fatalf("Fingerprint A: %v", err)
	}
	fpB, err := Fingerprint(payloadB, "document", "input_record:416", "measurement")
	if err != nil {
		t.Fatalf("Fingerprint B: %v", err)
	}
	if fpA == fpB {
		t.Fatal("expected different fingerprints for swapped label/alias (that's the bug this fix addresses)")
	}
	identityKey := IdentityKey("term", "measurement", payloadA)
	if identityKey == "" || identityKey != IdentityKey("term", "measurement", payloadB) {
		t.Fatalf("expected matching, non-empty identity keys, got %q vs %q", identityKey, IdentityKey("term", "measurement", payloadB))
	}
	now := time.Now()

	// Candidate A is created first; no other candidate shares its identity
	// key yet.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payloadA), "measurement", "document", "input_record:416", "null", nil, nil, fpA, "null",
			StatusDiscovered, nil, "tester", "tester", identityKey).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDiscovered)).
			AddRow(int64(1), "term", payloadA, "measurement", "document", "input_record:416", []byte("null"), nil, nil, fpA,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "tester", now, "tester", identityKey))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM kb.ontology_candidates")).
		WithArgs(identityKey, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	store := CandidateStore{DB: db}
	candA, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "term", ProposedPayload: payloadA, ProposedModuleID: "measurement",
		SourceType: "document", SourceRef: "input_record:416", CreateBy: "tester", ModifyBy: "tester",
	})
	if err != nil {
		t.Fatalf("create candidate A: %v", err)
	}
	if candA.ID != 1 || string(candA.CandidateMatches) != "null" {
		t.Fatalf("expected candidate A with no matches yet, got %#v", candA)
	}

	// Candidate B is created second, with the label/alias swapped -- its
	// identity key matches A's, so both sides get a soft match entry.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payloadB), "measurement", "document", "input_record:416", "null", nil, nil, fpB, "null",
			StatusDiscovered, nil, "tester", "tester", identityKey).
		WillReturnRows(sqlmock.NewRows(candidateRow(2, StatusDiscovered)).
			AddRow(int64(2), "term", payloadB, "measurement", "document", "input_record:416", []byte("null"), nil, nil, fpB,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "tester", now, "tester", identityKey))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM kb.ontology_candidates")).
		WithArgs(identityKey, int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectExec(regexp.QuoteMeta("SET candidate_matches = COALESCE")).
		WithArgs(int64(1), candidateMatchesArg{wantCandidateID: 2}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("SET candidate_matches = COALESCE")).
		WithArgs(int64(2), candidateMatchesArg{wantCandidateID: 1}).
		WillReturnResult(sqlmock.NewResult(0, 1))

	candB, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "term", ProposedPayload: payloadB, ProposedModuleID: "measurement",
		SourceType: "document", SourceRef: "input_record:416", CreateBy: "tester", ModifyBy: "tester",
	})
	if err != nil {
		t.Fatalf("create candidate B: %v", err)
	}
	if candB.ID != 2 {
		t.Fatalf("expected candidate B id=2, got %d", candB.ID)
	}
	var entries []candidateMatchEntry
	if err := json.Unmarshal(candB.CandidateMatches, &entries); err != nil {
		t.Fatalf("unmarshal candidate_matches: %v", err)
	}
	if len(entries) != 1 || entries[0].CandidateID != 1 || entries[0].MatchType != "candidate" || entries[0].MatchedOn != "identity_key" {
		t.Fatalf("expected candidate B to reference candidate A, got %#v", entries)
	}
	// mock.ExpectationsWereMet below also confirms the reciprocal UPDATE
	// against candidate A (id=1) actually ran.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestCreateCandidateDoesNotMatchAcrossTermKinds confirms two candidates
// with the same label but different term_kind (e.g. a metric_definition and
// an unrelated concept both happening to be called "load") get distinct
// identity keys and are never matched to each other.
func TestCreateCandidateDoesNotMatchAcrossTermKinds(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payloadMetric := []byte(`{"term_kind":"metric_definition","label":"load"}`)
	payloadConcept := []byte(`{"term_kind":"concept","label":"load"}`)
	keyMetric := IdentityKey("term", "measurement", payloadMetric)
	keyConcept := IdentityKey("term", "measurement", payloadConcept)
	if keyMetric == keyConcept {
		t.Fatal("expected different identity keys for different term_kind values")
	}
	fpMetric, _ := Fingerprint(payloadMetric, "document", "input_record:1", "measurement")
	fpConcept, _ := Fingerprint(payloadConcept, "document", "input_record:1", "measurement")
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payloadMetric), "measurement", "document", "input_record:1", "null", nil, nil, fpMetric, "null",
			StatusDiscovered, nil, "tester", "tester", keyMetric).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDiscovered)).
			AddRow(int64(1), "term", payloadMetric, "measurement", "document", "input_record:1", []byte("null"), nil, nil, fpMetric,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "tester", now, "tester", keyMetric))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM kb.ontology_candidates")).
		WithArgs(keyMetric, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	store := CandidateStore{DB: db}
	if _, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "term", ProposedPayload: payloadMetric, ProposedModuleID: "measurement",
		SourceType: "document", SourceRef: "input_record:1", CreateBy: "tester", ModifyBy: "tester",
	}); err != nil {
		t.Fatalf("create metric candidate: %v", err)
	}

	// Second candidate has a different identity key (different term_kind),
	// so its own lookup finds nothing -- even though a same-labeled row
	// exists -- and no UPDATE is issued.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payloadConcept), "measurement", "document", "input_record:1", "null", nil, nil, fpConcept, "null",
			StatusDiscovered, nil, "tester", "tester", keyConcept).
		WillReturnRows(sqlmock.NewRows(candidateRow(2, StatusDiscovered)).
			AddRow(int64(2), "term", payloadConcept, "measurement", "document", "input_record:1", []byte("null"), nil, nil, fpConcept,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "tester", now, "tester", keyConcept))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM kb.ontology_candidates")).
		WithArgs(keyConcept, int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	got, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "term", ProposedPayload: payloadConcept, ProposedModuleID: "measurement",
		SourceType: "document", SourceRef: "input_record:1", CreateBy: "tester", ModifyBy: "tester",
	})
	if err != nil {
		t.Fatalf("create concept candidate: %v", err)
	}
	if string(got.CandidateMatches) != "null" {
		t.Fatalf("expected no candidate_matches across different term_kind values, got %s", got.CandidateMatches)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestCreateCandidateIdentityMatchQueryExcludesTerminalStatuses locks in
// that the identity-key lookup's WHERE clause excludes rejected/superseded
// rows, and that an empty result (as Postgres would return when the only
// same-key row is terminal) produces no UPDATE.
func TestCreateCandidateIdentityMatchQueryExcludesTerminalStatuses(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payload := []byte(`{"term_kind":"metric_definition","label":"load"}`)
	key := IdentityKey("term", "measurement", payload)
	fp, _ := Fingerprint(payload, "document", "input_record:1", "measurement")
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payload), "measurement", "document", "input_record:1", "null", nil, nil, fp, "null",
			StatusDiscovered, nil, "tester", "tester", key).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDiscovered)).
			AddRow(int64(1), "term", payload, "measurement", "document", "input_record:1", []byte("null"), nil, nil, fp,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "tester", now, "tester", key))
	// The only other row sharing this identity key is rejected/superseded,
	// so Postgres's own WHERE clause (status NOT IN ('rejected',
	// 'superseded')) excludes it -- simulated here as an empty result set.
	mock.ExpectQuery(regexp.QuoteMeta("status NOT IN ('rejected', 'superseded')")).
		WithArgs(key, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	store := CandidateStore{DB: db}
	got, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "term", ProposedPayload: payload, ProposedModuleID: "measurement",
		SourceType: "document", SourceRef: "input_record:1", CreateBy: "tester", ModifyBy: "tester",
	})
	if err != nil {
		t.Fatalf("CreateCandidate: %v", err)
	}
	if string(got.CandidateMatches) != "null" {
		t.Fatalf("expected no candidate_matches when only match is terminal-status, got %s", got.CandidateMatches)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestCreateCandidateFingerprintReuseSkipsIdentityMatching confirms that
// when CreateCandidate takes the existing ON CONFLICT (fingerprint) reuse
// path, no identity-key lookup or candidate_matches update happens -- only
// the fingerprint INSERT-attempt and byFingerprint SELECT are expected;
// any extra unmocked call would surface as an error from CreateCandidate.
func TestCreateCandidateFingerprintReuseSkipsIdentityMatching(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payload := []byte(`{"term_kind":"metric_definition","label":"load"}`)
	key := IdentityKey("term", "measurement", payload)
	fp, _ := Fingerprint(payload, "document", "input_record:1", "measurement")
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payload), "measurement", "document", "input_record:1", "null", nil, nil, fp, "null",
			StatusDiscovered, nil, "tester", "tester", key).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("WHERE fingerprint = $1")).
		WithArgs(fp).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusApproved)).
			AddRow(int64(1), "term", payload, "measurement", "document", "input_record:1", []byte("null"), nil, nil, fp,
				[]byte("null"), StatusApproved, nil, nil, nil, now, "system", now, "system", key))

	store := CandidateStore{DB: db}
	got, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "term", ProposedPayload: payload, ProposedModuleID: "measurement",
		SourceType: "document", SourceRef: "input_record:1", CreateBy: "tester", ModifyBy: "tester",
	})
	if err != nil {
		t.Fatalf("CreateCandidate: %v", err)
	}
	if !got.Reused {
		t.Fatal("expected Reused=true on fingerprint hit")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet (no identity-key lookup should run on reuse): %v", err)
	}
}

// TestCreateCandidateNonTermCandidateKindNeverMatched confirms axiom (and
// by extension label/mapping/profile/...) candidates never get an
// identity_key and are never matched, even with identical-looking payloads.
func TestCreateCandidateNonTermCandidateKindNeverMatched(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payload := []byte(`{"axiom_id":"measurement:load_measured_by_x","axiom_kind":"object_property_assertion"}`)
	fp, _ := Fingerprint(payload, "document", "input_record:1", "measurement")
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("axiom", string(payload), "measurement", "document", "input_record:1", "null", nil, nil, fp, "null",
			StatusDiscovered, nil, "tester", "tester", nil).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDiscovered)).
			AddRow(int64(1), "axiom", payload, "measurement", "document", "input_record:1", []byte("null"), nil, nil, fp,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "tester", now, "tester", nil))

	store := CandidateStore{DB: db}
	got, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "axiom", ProposedPayload: payload, ProposedModuleID: "measurement",
		SourceType: "document", SourceRef: "input_record:1", CreateBy: "tester", ModifyBy: "tester",
	})
	if err != nil {
		t.Fatalf("CreateCandidate: %v", err)
	}
	if got.IdentityKey != "" {
		t.Fatalf("expected no identity_key for axiom candidate, got %q", got.IdentityKey)
	}
	// Only the INSERT is mocked; if the code incorrectly ran an
	// identity-key lookup, this unmocked query would surface as an error.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
