package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const governedPropertyValueMapSelectSQL = "SELECT dimension, raw_value, term_id, status FROM kb.governed_property_value_map"

// TestGovernedPropertyResolverLookupApprovedHit locks in ADR 2026082203 DR3:
// an approved row resolves directly, mirroring ValueRangeTypeMapper's
// established approved-hit contract.
func TestGovernedPropertyResolverLookupApprovedHit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(governedPropertyValueMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "term_id", "status"}).
			AddRow("metric_subject", "ambient", "measurement:subj_ambient", "approved"))

	resolver := GovernedPropertyResolver{DB: db, cache: &governedPropertyValueMapCache{}}
	termID, status, err := resolver.Lookup(context.Background(), "metric_subject", "ambient", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if termID != "measurement:subj_ambient" || status != "approved" {
		t.Fatalf("Lookup(metric_subject, ambient) = (%q, %q), want (measurement:subj_ambient, approved)", termID, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGovernedPropertyResolverLookupProposedNeverReturnsTermID locks in DR3:
// a proposed row is never usable for signature resolution, even if a
// term_id happens to be set on it.
func TestGovernedPropertyResolverLookupProposedNeverReturnsTermID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(governedPropertyValueMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "term_id", "status"}).
			AddRow("metric_subject", "huanjing", nil, "proposed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.governed_property_value_map")).
		WithArgs("metric_subject", "huanjing", int64(416)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resolver := GovernedPropertyResolver{DB: db, cache: &governedPropertyValueMapCache{}}
	termID, status, err := resolver.Lookup(context.Background(), "metric_subject", "huanjing", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if termID != "" || status != "proposed" {
		t.Fatalf("Lookup(metric_subject, huanjing) = (%q, %q), want (\"\", proposed)", termID, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGovernedPropertyResolverLookupRejectedNeverReturnsTermID locks in DR3's
// fourth status (absent from ValueRangeTypeMapper's three-state original):
// rejected never resolves, same shape as ambiguous/proposed.
func TestGovernedPropertyResolverLookupRejectedNeverReturnsTermID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(governedPropertyValueMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "term_id", "status"}).
			AddRow("metric_value_class", "garbage", nil, "rejected"))

	resolver := GovernedPropertyResolver{DB: db, cache: &governedPropertyValueMapCache{}}
	termID, status, err := resolver.Lookup(context.Background(), "metric_value_class", "garbage", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if termID != "" || status != "rejected" {
		t.Fatalf("Lookup(metric_value_class, garbage) = (%q, %q), want (\"\", rejected)", termID, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGovernedPropertyResolverLookupMissInsertsProposed locks in DR3: an
// unseen (dimension, raw_value) pair auto-inserts a proposed row rather than
// discarding the raw value.
func TestGovernedPropertyResolverLookupMissInsertsProposed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(governedPropertyValueMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "term_id", "status"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.governed_property_value_map")).
		WithArgs("metric_object_name", "feilliao", int64(416)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	resolver := GovernedPropertyResolver{DB: db, cache: &governedPropertyValueMapCache{}}
	termID, status, err := resolver.Lookup(context.Background(), "metric_object_name", "feilliao", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if termID != "" || status != "proposed" {
		t.Fatalf("Lookup(metric_object_name, feilliao) = (%q, %q), want (\"\", proposed)", termID, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGovernedPropertyResolverLookupSameRawDifferentDimensionsDoNotCollide
// locks in the composite (dimension, raw_value) key: two dimensions sharing
// the same normalized raw string must resolve independently, unlike
// ValueRangeTypeMapper's single-dimension cache which only ever keyed on
// raw_value.
func TestGovernedPropertyResolverLookupSameRawDifferentDimensionsDoNotCollide(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(governedPropertyValueMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "term_id", "status"}).
			AddRow("metric_subject", "standard", "measurement:subj_standard", "approved").
			AddRow("metric_value_class", "standard", nil, "proposed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.governed_property_value_map")).
		WithArgs("metric_value_class", "standard", int64(416)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resolver := GovernedPropertyResolver{DB: db, cache: &governedPropertyValueMapCache{}}
	ctx := context.Background()

	termID, status, err := resolver.Lookup(ctx, "metric_subject", "standard", 416)
	if err != nil {
		t.Fatalf("Lookup(metric_subject) error: %v", err)
	}
	if termID != "measurement:subj_standard" || status != "approved" {
		t.Fatalf("Lookup(metric_subject, standard) = (%q, %q), want (measurement:subj_standard, approved)", termID, status)
	}

	termID, status, err = resolver.Lookup(ctx, "metric_value_class", "standard", 416)
	if err != nil {
		t.Fatalf("Lookup(metric_value_class) error: %v", err)
	}
	if termID != "" || status != "proposed" {
		t.Fatalf("Lookup(metric_value_class, standard) = (%q, %q), want (\"\", proposed)", termID, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGovernedPropertyResolverLookupRepeatMissUpdatesOccurrenceNoDuplicateInsert
// locks in DR3's occurrence bookkeeping: a second Lookup for a pair already
// seen this process increments occurrence_count via UPDATE, never a second
// INSERT.
func TestGovernedPropertyResolverLookupRepeatMissUpdatesOccurrenceNoDuplicateInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(governedPropertyValueMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "term_id", "status"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.governed_property_value_map")).
		WithArgs("metric_name", "wendu", int64(416)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta(governedPropertyValueMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "term_id", "status"}).
			AddRow("metric_name", "wendu", nil, "proposed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.governed_property_value_map")).
		WithArgs("metric_name", "wendu", int64(417)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resolver := GovernedPropertyResolver{DB: db, cache: &governedPropertyValueMapCache{}}
	ctx := context.Background()

	if _, status, err := resolver.Lookup(ctx, "metric_name", "wendu", 416); err != nil || status != "proposed" {
		t.Fatalf("first Lookup(metric_name, wendu) = (status=%q, err=%v), want (proposed, nil)", status, err)
	}
	if _, status, err := resolver.Lookup(ctx, "metric_name", "wendu", 417); err != nil || status != "proposed" {
		t.Fatalf("second Lookup(metric_name, wendu) = (status=%q, err=%v), want (proposed, nil)", status, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGovernedPropertyResolverLookupEmptyRawIsAbsent locks in DR3: an
// empty/whitespace-only raw value is "absent", not a mapping problem -- no
// DB access at all.
func TestGovernedPropertyResolverLookupEmptyRawIsAbsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	resolver := GovernedPropertyResolver{DB: db, cache: &governedPropertyValueMapCache{}}
	termID, status, err := resolver.Lookup(context.Background(), "metric_subject", "  ", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if termID != "" || status != "absent" {
		t.Fatalf("Lookup(metric_subject, \"  \") = (%q, %q), want (\"\", absent)", termID, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
