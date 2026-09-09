package productreviews

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const twoAspectTOML = `
[aspects.storage]
name_en = "Storage"
name_zh_cn = "贮存"
relation_types = ["storage_requirement"]

[aspects.emc]
name_en = "EMC"
match_mode = "lexical"
`

func aspectInsertArgs(kind, aspectKey string, relTypes []byte, matchMode string) []driver.Value {
	a := make([]driver.Value, 20)
	for i := range a {
		a[i] = sqlmock.AnyArg()
	}
	a[2] = kind       // node_kind
	a[16] = aspectKey // aspect_key
	a[17] = relTypes  // relation_types (JSONB bytes)
	a[18] = matchMode // match_mode
	return a
}

// Scenario: Configured aspects are attached + Aspect carries its relation-type
// mapping (spec: Aspect nodes from configured vocabulary).
func TestAttachAspects(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	cfg, err := ParseConfig([]byte(twoAspectTOML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator"}))
	mock.ExpectBegin()
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(aspectInsertArgs(KindAspect, "storage", []byte(`["storage_requirement"]`), MatchModeJoin)...).
		WillReturnRows(idRow(30))
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(aspectInsertArgs(KindAspect, "emc", []byte(`[]`), MatchModeLexical)...).
		WillReturnRows(idRow(31))
	mock.ExpectExec(rx("SET version = version + 1")).WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	n, err := store.AttachAspects(context.Background(), 1, cfg)
	if err != nil {
		t.Fatalf("AttachAspects: %v", err)
	}
	if n != 2 {
		t.Fatalf("inserted %d aspect nodes, want 2", n)
	}
}

// An aspect already on the profile is not re-added.
func TestAttachAspectsIdempotent(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	cfg, _ := ParseConfig([]byte(twoAspectTOML))

	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1,
			nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator"},
			nodeRow{ID: 2, Kind: KindAspect, Label: "Storage", AspectKey: "storage", Status: StatusAccepted, Origin: OriginUserAdded},
		))
	mock.ExpectBegin()
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(aspectInsertArgs(KindAspect, "emc", []byte(`[]`), MatchModeLexical)...).
		WillReturnRows(idRow(31))
	mock.ExpectExec(rx("SET version = version + 1")).WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	n, err := store.AttachAspects(context.Background(), 1, cfg)
	if err != nil {
		t.Fatalf("AttachAspects: %v", err)
	}
	if n != 1 {
		t.Fatalf("inserted %d, want 1 (storage already present)", n)
	}
}
