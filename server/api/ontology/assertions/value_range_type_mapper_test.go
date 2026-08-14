package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const valueRangeTypeMapSelectSQL = "SELECT raw_value, canonical_bucket, status FROM kb.metric_value_range_type_map"

// TestValueRangeTypeMapperLookupApprovedHit locks in ADR 2026081401 DR1: a
// raw value with an 'approved' row classifies directly, with no write back
// to the table (an approved mapping is already authoritative -- nothing to
// track).
func TestValueRangeTypeMapperLookupApprovedHit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueRangeTypeMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"}).
			AddRow("min", "lower_bound", "approved"))

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "min", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "lower_bound" || status != "approved" {
		t.Fatalf("Lookup(min) = (%q, %q), want (lower_bound, approved)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValueRangeTypeMapperLookupAmbiguousHit locks in DR1: an 'ambiguous' row
// never returns a usable bucket, but still records the occurrence (DR1: "a
// repeat hit on an existing 'proposed' or 'ambiguous' row increments
// occurrence_count and updates last_seen_record_id").
func TestValueRangeTypeMapperLookupAmbiguousHit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueRangeTypeMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"}).
			AddRow("threshold", nil, "ambiguous"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.metric_value_range_type_map")).
		WithArgs("threshold", int64(416)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "threshold", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "" || status != "ambiguous" {
		t.Fatalf("Lookup(threshold) = (%q, %q), want (\"\", ambiguous)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValueRangeTypeMapperLookupMissInsertsProposed locks in DR1: a raw
// value never seen before auto-inserts a 'proposed' row (occurrence_count=1,
// first/last_seen_record_id set) and the miss is never usable for
// classification even when a best-effort bucket guess was stored (DR6's
// guess is a suggestion for human review, never authoritative).
func TestValueRangeTypeMapperLookupMissInsertsProposed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueRangeTypeMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.metric_value_range_type_map")).
		WithArgs("not_less_than_spec", "lower_bound", int64(416)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "not less than spec", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "" || status != "proposed" {
		t.Fatalf("Lookup(not less than spec) = (%q, %q), want (\"\", proposed) -- a guess must never classify", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValueRangeTypeMapperLookupMissNoCueGuessesEmpty locks in DR6: when no
// direction cue applies, canonical_bucket stays NULL on insert rather than a
// fabricated guess.
func TestValueRangeTypeMapperLookupMissNoCueGuessesEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueRangeTypeMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.metric_value_range_type_map")).
		WithArgs("some_new_vocabulary", nil, int64(416)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "some new vocabulary", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "" || status != "proposed" {
		t.Fatalf("Lookup(some new vocabulary) = (%q, %q), want (\"\", proposed)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValueRangeTypeMapperLookupRepeatMissUpdatesOccurrenceNoDuplicateInsert
// locks in DR1: a second Lookup for a raw_value already seen this process
// (now present in the table, invalidated-and-reloaded cache) increments
// occurrence_count via UPDATE, never a second INSERT -- raw_value's PK is
// the dedup key, but the cache must also route to UPDATE, not re-attempt
// INSERT, once it knows the row exists.
func TestValueRangeTypeMapperLookupRepeatMissUpdatesOccurrenceNoDuplicateInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// First Lookup: cache starts empty, raw_value is a genuine miss.
	mock.ExpectQuery(regexp.QuoteMeta(valueRangeTypeMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.metric_value_range_type_map")).
		WithArgs("single", nil, int64(416)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Cache invalidated after the insert -- second Lookup reloads and now
	// finds the row it just created (status still 'proposed').
	mock.ExpectQuery(regexp.QuoteMeta(valueRangeTypeMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"}).
			AddRow("single", nil, "proposed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.metric_value_range_type_map")).
		WithArgs("single", int64(417)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	ctx := context.Background()

	if _, status, err := mapper.Lookup(ctx, "single", 416); err != nil || status != "proposed" {
		t.Fatalf("first Lookup(single) = (status=%q, err=%v), want (proposed, nil)", status, err)
	}
	if _, status, err := mapper.Lookup(ctx, "single", 417); err != nil || status != "proposed" {
		t.Fatalf("second Lookup(single) = (status=%q, err=%v), want (proposed, nil)", status, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValueRangeTypeMapperLookupEmptyRawIsAbsent locks in DR1/§3.3: an
// empty/unset raw value is "absent", not a mapping problem -- no DB access
// at all, since there is nothing to look up or track.
func TestValueRangeTypeMapperLookupEmptyRawIsAbsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "  ", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "" || status != "absent" {
		t.Fatalf("Lookup(\"  \") = (%q, %q), want (\"\", absent)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValueRangeTypeMapperLookupReusesCacheWithinTTL locks in design D1: the
// full-table cache serves repeated distinct-raw-value lookups from memory
// within its TTL -- only one SELECT for the whole test, not one per lookup.
func TestValueRangeTypeMapperLookupReusesCacheWithinTTL(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueRangeTypeMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"}).
			AddRow("min", "lower_bound", "approved").
			AddRow("max", "upper_bound", "approved"))

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	ctx := context.Background()
	if bucket, _, err := mapper.Lookup(ctx, "min", 416); err != nil || bucket != "lower_bound" {
		t.Fatalf("Lookup(min) = (%q, err=%v)", bucket, err)
	}
	if bucket, _, err := mapper.Lookup(ctx, "max", 416); err != nil || bucket != "upper_bound" {
		t.Fatalf("Lookup(max) = (%q, err=%v)", bucket, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
