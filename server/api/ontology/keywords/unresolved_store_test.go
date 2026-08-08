package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMergeTiedCandidatesDedupesByConceptIDLatestWins(t *testing.T) {
	existing := []TiedCandidate{
		{ConceptID: "kwc_a", Score: 0.5, Method: "tier5_fuzzy"},
		{ConceptID: "kwc_b", Score: 0.5, Method: "tier5_fuzzy"},
	}
	incoming := []TiedCandidate{
		{ConceptID: "kwc_b", Score: 0.6, Method: "tier5_fuzzy"}, // score shifted, same concept
		{ConceptID: "kwc_c", Score: 0.5, Method: "tier5_fuzzy"}, // new concept
	}
	got := mergeTiedCandidates(existing, incoming)
	want := []TiedCandidate{
		{ConceptID: "kwc_a", Score: 0.5, Method: "tier5_fuzzy"},
		{ConceptID: "kwc_b", Score: 0.6, Method: "tier5_fuzzy"}, // updated, not duplicated
		{ConceptID: "kwc_c", Score: 0.5, Method: "tier5_fuzzy"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge: got %+v, want %+v", got, want)
	}
}

func TestMergeTiedCandidatesCapsAtMax(t *testing.T) {
	var existing []TiedCandidate
	for i := 0; i < maxUnresolvedCandidates; i++ {
		existing = append(existing, TiedCandidate{ConceptID: string(rune('a' + i)), Score: 0.5})
	}
	incoming := []TiedCandidate{{ConceptID: "overflow", Score: 0.5}}
	got := mergeTiedCandidates(existing, incoming)
	if len(got) != maxUnresolvedCandidates {
		t.Fatalf("expected cap at %d, got %d", maxUnresolvedCandidates, len(got))
	}
	if got[len(got)-1].ConceptID != "overflow" {
		t.Errorf("expected the newest candidate to survive the cap, got %+v", got)
	}
	if got[0].ConceptID == "a" {
		t.Errorf("expected the oldest candidate to be evicted, still present: %+v", got)
	}
}

func TestMergeTiedCandidatesNoIncomingReturnsExisting(t *testing.T) {
	existing := []TiedCandidate{{ConceptID: "kwc_a", Score: 0.5}}
	if got := mergeTiedCandidates(existing, nil); !reflect.DeepEqual(got, existing) {
		t.Errorf("nil incoming: got %+v, want unchanged %+v", got, existing)
	}
}

func TestNullableTiedCandidatesJSON(t *testing.T) {
	if got := nullableTiedCandidatesJSON(nil); got != nil {
		t.Errorf("empty set: got %v, want nil (SQL NULL)", got)
	}
	candidates := []TiedCandidate{{ConceptID: "kwc_a", Score: 0.5, Method: "tier5_fuzzy"}}
	got := nullableTiedCandidatesJSON(candidates)
	raw, ok := got.([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", got)
	}
	var roundTrip []TiedCandidate
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, candidates) {
		t.Errorf("round trip: got %+v, want %+v", roundTrip, candidates)
	}
}

func TestUpsertUnresolvedPersistsCandidatesForAmbiguous(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := UnresolvedStore{DB: db}
	ctx := context.Background()

	tied := []TiedCandidate{
		{ConceptID: "kwc_a", Score: 0.8, Method: "tier1_norm"},
		{ConceptID: "kwc_b", Score: 0.8, Method: "tier1_norm"},
	}
	tiedJSON, _ := json.Marshal(tied)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT surfaces, contexts, candidates, hits FROM kb.keyword_unresolved`)).
		WithArgs("ml", "_").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.keyword_unresolved`)).
		WithArgs("ml", "_", []byte(`["ML"]`), []byte(`null`), tiedJSON).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.UpsertUnresolved(ctx, "ml", "_", "ML", "", tied); err != nil {
		t.Fatalf("UpsertUnresolved: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListAmbiguousFiltersOnCandidatesNotNull(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := UnresolvedStore{DB: db}
	ctx := context.Background()

	tied := []TiedCandidate{{ConceptID: "kwc_a", Score: 0.8, Method: "tier1_norm"}}
	tiedJSON, _ := json.Marshal(tied)

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE scope = $1 AND status = 'pending' AND candidates IS NOT NULL`)).
		WithArgs("_", 200).
		WillReturnRows(sqlmock.NewRows([]string{"norm_key", "scope", "surfaces", "contexts", "candidates", "hits",
			"status", "attempts", "last_attempt", "priority", "first_seen", "last_seen"}).
			AddRow("ml", "_", []byte(`["ML"]`), []byte(`[]`), tiedJSON, 2,
				"pending", 0, nil, 0.0, time.Now(), time.Now()))

	rows, err := store.ListAmbiguous(ctx, "_", 0)
	if err != nil {
		t.Fatalf("ListAmbiguous: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Candidates) != 1 || rows[0].Candidates[0].ConceptID != "kwc_a" {
		t.Errorf("expected one row carrying the tied candidate, got %+v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
