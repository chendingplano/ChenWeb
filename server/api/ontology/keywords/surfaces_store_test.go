package keywords

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateSurface(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := SurfaceStore{DB: db}
	ctx := context.Background()

	sf := Surface{
		ConceptID:   "kw:test",
		Surface:     "hello world",
		NormKey:     "hello world",
		NormVersion: 1,
		LabelRole:   "pref",
		AliasType:   "synonym",
		Lang:        "en",
		Scope:       "_",
		Provenance:  "human:tester",
		Confidence:  1.0,
	}
	// Pre-compute the derived surface_id so the mock arg matches.
	sf.SurfaceID = deriveSurfaceID(sf.ConceptID, sf.Surface, sf.LabelRole)

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO kb.keyword_surfaces`)).
		WithArgs(sf.SurfaceID, sf.ConceptID, sf.Surface, sf.NormKey, sf.NormVersion,
			sf.LabelRole, sf.AliasType, sf.Lang, sf.Scope, sf.Confidence,
			sf.Provenance, sf.Locked, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow(sf.SurfaceID, sf.ConceptID, sf.Surface, sf.NormKey, sf.NormVersion,
			sf.LabelRole, sf.AliasType, "en", "_", 1.0,
			"human:tester", false, nil, testNow, testNow))

	result, err := store.CreateSurface(ctx, sf)
	if err != nil {
		t.Fatalf("CreateSurface: %v", err)
	}
	if result.SurfaceID != sf.SurfaceID {
		t.Errorf("expected surface_id %s, got %s", sf.SurfaceID, result.SurfaceID)
	}
	if result.Surface != "hello world" {
		t.Errorf("expected surface 'hello world', got %s", result.Surface)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCreateSurfaceValidation(t *testing.T) {
	store := SurfaceStore{DB: nil}
	ctx := context.Background()

	tests := []struct {
		name string
		sf   Surface
	}{
		{"missing surface", Surface{ConceptID: "kw:x", NormKey: "x", LabelRole: "pref", AliasType: "synonym", Provenance: "human:"}},
		{"missing concept_id", Surface{Surface: "x", NormKey: "x", LabelRole: "pref", AliasType: "synonym", Provenance: "human:"}},
		{"missing norm_key", Surface{Surface: "x", ConceptID: "kw:x", LabelRole: "pref", AliasType: "synonym", Provenance: "human:"}},
		{"invalid label_role", Surface{Surface: "x", ConceptID: "kw:x", NormKey: "x", LabelRole: "bogus", AliasType: "synonym", Provenance: "human:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.CreateSurface(ctx, tt.sf)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestGetSurface(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := SurfaceStore{DB: db}
	ctx := context.Background()

	sid := "kws_test12345678"
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + surfaceColumns + ` ` + surfaceFrom + ` WHERE surface_id = $1`)).
		WithArgs(sid).
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow(sid, "kw:test", "hello", "hello", 1, "pref", "synonym", "en", "_", 1.0, "human:tester", false, nil, testNow, testNow))

	sf, err := store.GetSurface(ctx, sid)
	if err != nil {
		t.Fatalf("GetSurface: %v", err)
	}
	if sf.SurfaceID != sid {
		t.Errorf("expected %s, got %s", sid, sf.SurfaceID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListSurfacesByConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := SurfaceStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + surfaceColumns + ` ` + surfaceFrom + ` WHERE concept_id = $1 ORDER BY label_role, surface`)).
		WithArgs("kw:test").
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow("kws_a", "kw:test", "hello", "hello", 1, "pref", "synonym", "en", "_", 1.0, "human:", false, nil, testNow, testNow).
			AddRow("kws_b", "kw:test", "hi", "hi", 1, "alt", "synonym", "en", "_", 0.9, "human:", false, nil, testNow, testNow))

	surfaces, err := store.ListSurfacesByConcept(ctx, "kw:test")
	if err != nil {
		t.Fatalf("ListSurfacesByConcept: %v", err)
	}
	if len(surfaces) != 2 {
		t.Fatalf("expected 2 surfaces, got %d", len(surfaces))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListSurfacesByNormKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := SurfaceStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT `+surfaceColumns+` `+surfaceFrom+` WHERE norm_key = $1 AND scope = $2 ORDER BY confidence DESC`)).
		WithArgs("hello world", "_").
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow("kws_x", "kw:test", "hello world", "hello world", 1, "pref", "synonym", "en", "_", 1.0, "human:", false, nil, testNow, testNow))

	surfaces, err := store.ListSurfacesByNormKey(ctx, "hello world", "_")
	if err != nil {
		t.Fatalf("ListSurfacesByNormKey: %v", err)
	}
	if len(surfaces) != 1 {
		t.Fatalf("expected 1 surface, got %d", len(surfaces))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpdateSurfaceLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := SurfaceStore{DB: db}
	ctx := context.Background()

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE kb.keyword_surfaces SET locked = $2, modify_time = NOW() WHERE surface_id = $1`)).
		WithArgs("kws_x", true).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.UpdateSurfaceLock(ctx, "kws_x", true); err != nil {
		t.Fatalf("UpdateSurfaceLock: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
