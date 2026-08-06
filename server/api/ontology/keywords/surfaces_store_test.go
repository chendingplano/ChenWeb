package keywords

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

func TestCreateSurface(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := SurfaceStore{DB: db}
	ctx := context.Background()

	// No norm_key/norm_version supplied: the store derives them (D6).
	sf := Surface{
		ConceptID:  "kw:test",
		Surface:    "hello world",
		LabelRole:  "pref",
		AliasType:  "synonym",
		Lang:       "en",
		Scope:      "_",
		Provenance: "human:tester",
		Confidence: 1.0,
	}
	wantNorm := "hello world"
	wantVersion := semid.CurrentNormalizerVersion
	// Pre-compute the derived surface_id so the mock arg matches.
	sf.SurfaceID = deriveSurfaceID(sf.ConceptID, sf.Surface, sf.LabelRole)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO kb.keyword_surfaces`)).
		WithArgs(sf.SurfaceID, sf.ConceptID, sf.Surface, wantNorm, wantVersion,
			sf.LabelRole, sf.AliasType, sf.Lang, sf.Scope, sf.Confidence,
			sf.Provenance, sf.Locked, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow(sf.SurfaceID, sf.ConceptID, sf.Surface, wantNorm, wantVersion,
			sf.LabelRole, sf.AliasType, "en", "_", 1.0,
			"human:tester", false, nil, testNow, testNow))

	// K1: the derived keys are written with the surface, in the same
	// transaction. "sorted" equals the norm key and is omitted; "phonetic"
	// is not persisted (no tier reads it, §20.2); "singular" is written
	// even when it equals the norm key (plural bridge).
	mock.ExpectExec(regexp.QuoteMeta(
		`DELETE FROM kb.keyword_surface_keys WHERE surface_id = $1`)).
		WithArgs(sf.SurfaceID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, k := range []struct{ kind, value string }{
		{"alnum", "helloworld"},
		{"initials", "hw"},
		{"singular", "hello world"},
	} {
		mock.ExpectExec(regexp.QuoteMeta(
			`INSERT INTO kb.keyword_surface_keys (surface_id, key_kind, key_value, norm_version)`)).
			WithArgs(sf.SurfaceID, k.kind, k.value, wantVersion).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

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
	if result.NormKey != wantNorm || result.NormVersion != wantVersion {
		t.Errorf("expected derived key %q v%d, got %q v%d", wantNorm, wantVersion, result.NormKey, result.NormVersion)
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
		{"missing surface", Surface{ConceptID: "kw:x", LabelRole: "pref", AliasType: "synonym", Provenance: "human:"}},
		{"missing concept_id", Surface{Surface: "x", LabelRole: "pref", AliasType: "synonym", Provenance: "human:"}},
		// K3: a caller-authored norm_key that disagrees with the derived
		// key is rejected.
		{"mismatched norm_key rejected", Surface{Surface: "x", ConceptID: "kw:x", NormKey: "wrong", LabelRole: "pref", AliasType: "synonym", Provenance: "human:"}},
		{"invalid label_role", Surface{Surface: "x", ConceptID: "kw:x", LabelRole: "bogus", AliasType: "synonym", Provenance: "human:"}},
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

// The surface id hashes concept|surface|role — so the role default must be
// applied before the id is derived (§20.2). An empty LabelRole becomes
// "pref" in both the row and the id.
func TestCreateSurfaceDefaultsRoleBeforeDerivingID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := SurfaceStore{DB: db}
	ctx := context.Background()

	sf := Surface{
		ConceptID:  "kwc_role",
		Surface:    "luminance",
		AliasType:  "pref",
		Provenance: "human:tester",
	}
	wantID := deriveSurfaceID(sf.ConceptID, sf.Surface, "pref")
	ks := (semid.Normalizer{Version: semid.CurrentNormalizerVersion}).Normalize(sf.Surface)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_surfaces`)).
		WithArgs(wantID, sf.ConceptID, sf.Surface, ks.Norm, semid.CurrentNormalizerVersion,
			"pref", sf.AliasType, "en", "_", 1.0, sf.Provenance, false, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow(wantID, sf.ConceptID, sf.Surface, ks.Norm, semid.CurrentNormalizerVersion,
			"pref", sf.AliasType, "en", "_", 1.0, sf.Provenance, false, nil, testNow, testNow))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.keyword_surface_keys WHERE surface_id =`)).
		WithArgs(wantID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, k := range derivedSurfaceKeys(ks, wantID, semid.CurrentNormalizerVersion) {
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.keyword_surface_keys`)).
			WithArgs(wantID, k.KeyKind, k.KeyValue, semid.CurrentNormalizerVersion).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	created, err := store.CreateSurface(ctx, sf)
	if err != nil {
		t.Fatalf("CreateSurface: %v", err)
	}
	if created.LabelRole != "pref" {
		t.Errorf("LabelRole: got %q, want pref", created.LabelRole)
	}
	if created.SurfaceID != wantID {
		t.Errorf("SurfaceID: got %q, want %q (derived from the defaulted role)", created.SurfaceID, wantID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
