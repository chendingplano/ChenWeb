package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const valueBucketMapSelectSQL = "SELECT dimension, raw_value, canonical_bucket, status FROM kb.metric_value_bucket_map"

func TestValueBucketMapperLookupApprovedHit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueBucketMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "canonical_bucket", "status"}).
			AddRow("value_type", "numeric", "number", "approved"))

	mapper := ValueBucketMapper{DB: db, cache: &valueBucketMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "value_type", "numeric", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "number" || status != "approved" {
		t.Fatalf("Lookup(value_type, numeric) = (%q, %q), want (number, approved)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValueBucketMapperLookupProposedNeverReturnsBucket(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueBucketMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "canonical_bucket", "status"}).
			AddRow("value_type", "ratio", nil, "proposed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.metric_value_bucket_map")).
		WithArgs("value_type", "ratio", int64(416)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mapper := ValueBucketMapper{DB: db, cache: &valueBucketMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "value_type", "ratio", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "" || status != "proposed" {
		t.Fatalf("Lookup(value_type, ratio) = (%q, %q), want (\"\", proposed)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValueBucketMapperLookupAmbiguousNeverReturnsBucket(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueBucketMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "canonical_bucket", "status"}).
			AddRow("value_type", "garbage", nil, "ambiguous"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.metric_value_bucket_map")).
		WithArgs("value_type", "garbage", int64(416)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mapper := ValueBucketMapper{DB: db, cache: &valueBucketMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "value_type", "garbage", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "" || status != "ambiguous" {
		t.Fatalf("Lookup(value_type, garbage) = (%q, %q), want (\"\", ambiguous)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValueBucketMapperLookupMissInsertsProposedNoGuess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueBucketMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "canonical_bucket", "status"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.metric_value_bucket_map")).
		WithArgs("value_type", "percentage", int64(416)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mapper := ValueBucketMapper{DB: db, cache: &valueBucketMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "value_type", "percentage", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "" || status != "proposed" {
		t.Fatalf("Lookup(value_type, percentage) = (%q, %q), want (\"\", proposed)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValueBucketMapperLookupSameRawDifferentDimensionsDoNotCollide locks in
// the composite (dimension, raw_value) key -- two dimensions sharing the
// same normalized raw string must resolve independently.
func TestValueBucketMapperLookupSameRawDifferentDimensionsDoNotCollide(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueBucketMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "canonical_bucket", "status"}).
			AddRow("value_type", "standard", "text", "approved").
			AddRow("value_class", "standard", nil, "proposed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.metric_value_bucket_map")).
		WithArgs("value_class", "standard", int64(416)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mapper := ValueBucketMapper{DB: db, cache: &valueBucketMapCache{}}
	ctx := context.Background()

	bucket, status, err := mapper.Lookup(ctx, "value_type", "standard", 416)
	if err != nil {
		t.Fatalf("Lookup(value_type) error: %v", err)
	}
	if bucket != "text" || status != "approved" {
		t.Fatalf("Lookup(value_type, standard) = (%q, %q), want (text, approved)", bucket, status)
	}

	bucket, status, err = mapper.Lookup(ctx, "value_class", "standard", 416)
	if err != nil {
		t.Fatalf("Lookup(value_class) error: %v", err)
	}
	if bucket != "" || status != "proposed" {
		t.Fatalf("Lookup(value_class, standard) = (%q, %q), want (\"\", proposed)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValueBucketMapperLookupRepeatMissUpdatesOccurrenceNoDuplicateInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(valueBucketMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "canonical_bucket", "status"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.metric_value_bucket_map")).
		WithArgs("value_type", "integer", int64(416)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta(valueBucketMapSelectSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"dimension", "raw_value", "canonical_bucket", "status"}).
			AddRow("value_type", "integer", nil, "proposed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.metric_value_bucket_map")).
		WithArgs("value_type", "integer", int64(417)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mapper := ValueBucketMapper{DB: db, cache: &valueBucketMapCache{}}
	ctx := context.Background()

	if _, status, err := mapper.Lookup(ctx, "value_type", "integer", 416); err != nil || status != "proposed" {
		t.Fatalf("first Lookup(value_type, integer) = (status=%q, err=%v), want (proposed, nil)", status, err)
	}
	if _, status, err := mapper.Lookup(ctx, "value_type", "integer", 417); err != nil || status != "proposed" {
		t.Fatalf("second Lookup(value_type, integer) = (status=%q, err=%v), want (proposed, nil)", status, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValueBucketMapperLookupEmptyRawIsAbsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mapper := ValueBucketMapper{DB: db, cache: &valueBucketMapCache{}}
	bucket, status, err := mapper.Lookup(context.Background(), "value_type", "  ", 416)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if bucket != "" || status != "absent" {
		t.Fatalf("Lookup(value_type, \"  \") = (%q, %q), want (\"\", absent)", bucket, status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
