package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestResolveUnitTermsKnownUnitResolvesToUnitAndQuantityKind locks in design
// D4's term-id lookup: a raw unit that canonicalizes to a QUDT local name and
// exists in the catalog resolves to the unit term and its quantity-kind term.
func TestResolveUnitTermsKnownUnitResolvesToUnitAndQuantityKind(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
		WithArgs("quantity:unit_MilliSEC").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	a := AssociateSemantics{DB: db}
	unitID, qkID := a.resolveUnitTerms(context.Background(), "ms")
	if unitID != "quantity:unit_MilliSEC" {
		t.Fatalf("expected quantity:unit_MilliSEC, got %q", unitID)
	}
	if qkID != "quantity:qk_Time" {
		t.Fatalf("expected quantity:qk_Time, got %q", qkID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestResolveUnitTermsUnknownUnitLeavesIDsEmpty locks in the never-a-gate
// guarantee: a unit the catalog does not contain (cd/m² -- no candela unit was
// imported) leaves both term IDs empty without error, so the assertion is
// still accepted. The nil DB also proves the early return happens before any
// database access.
func TestResolveUnitTermsUnknownUnitLeavesIDsEmpty(t *testing.T) {
	a := AssociateSemantics{}
	unitID, qkID := a.resolveUnitTerms(context.Background(), "cd/m²")
	if unitID != "" || qkID != "" {
		t.Fatalf("expected empty term IDs for unimported unit, got unit=%q qk=%q", unitID, qkID)
	}
}

// TestResolveUnitTermsUnmappedCanonicalSkipsDB locks in the guard: a unit
// canonicalUnitForm does not recognize (e.g. a plain-text unit like "sensor")
// returns early without touching the database, so no sqlmock expectation is
// consumed.
func TestResolveUnitTermsUnmappedCanonicalSkipsDB(t *testing.T) {
	a := AssociateSemantics{}
	unitID, qkID := a.resolveUnitTerms(context.Background(), "sensor")
	if unitID != "" || qkID != "" {
		t.Fatalf("expected empty term IDs for unmapped unit, got unit=%q qk=%q", unitID, qkID)
	}
}

// TestResolveUnitTermsUnitNotInCatalogSkipsQuantityKind locks in the term-id
// confirmation: when the canonicalized term is not actually in the catalog,
// both IDs are empty (no quantity-kind without a confirmed unit).
func TestResolveUnitTermsUnitNotInCatalogSkipsQuantityKind(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
		WithArgs("quantity:unit_SEC").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	a := AssociateSemantics{DB: db}
	unitID, qkID := a.resolveUnitTerms(context.Background(), "s")
	if unitID != "" || qkID != "" {
		t.Fatalf("expected empty term IDs for unit absent from catalog, got unit=%q qk=%q", unitID, qkID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestCanonicalUnitFormPilotUnits(t *testing.T) {
	cases := map[string]string{
		"ms": "MilliSEC", "毫秒": "MilliSEC",
		"s": "SEC", "second": "SEC", "秒": "SEC",
		"ratio": "ONE", "1": "ONE", "无量纲": "ONE",
		"degree": "DEG", "deg": "DEG", "°": "DEG",
		"count": "COUNT", "个": "COUNT",
	}
	for raw, want := range cases {
		if got := canonicalUnitForm(raw); got != want {
			t.Errorf("canonicalUnitForm(%q) = %q, want %q", raw, got, want)
		}
	}
	if got := canonicalUnitForm("cd/m²"); got != "" {
		t.Errorf("canonicalUnitForm(cd/m²) = %q, want \"\" (no candela unit imported)", got)
	}
	if got := canonicalUnitForm("sensor"); got != "" {
		t.Errorf("canonicalUnitForm(sensor) = %q, want \"\"", got)
	}
}

func TestUnitQuantityKindMapCoversPilotUnits(t *testing.T) {
	for unit, want := range map[string]string{
		"MilliSEC": "quantity:qk_Time",
		"SEC":      "quantity:qk_Time",
		"ONE":      "quantity:qk_Dimensionless",
		"DEG":      "quantity:qk_Angle",
		"COUNT":    "quantity:qk_Count",
	} {
		if got := unitQuantityKindMap[unit]; got != want {
			t.Errorf("unitQuantityKindMap[%q] = %q, want %q", unit, got, want)
		}
	}
}
