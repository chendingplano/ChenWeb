package productreviews

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const scopeTOML = `
[scoring]
standards_boost = 0.25

[scoring.relation_type_weights]
scope = 1.0
storage_requirement = 0.6

[aspects.storage]
name_en = "Storage"
relation_types = ["storage_requirement"]
`

func scopeCfg(t *testing.T) *Config {
	t.Helper()
	c, err := ParseConfig([]byte(scopeTOML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	return c
}

func productRows(rows ...[]driver.Value) *sqlmock.Rows {
	out := sqlmock.NewRows([]string{"input_record_id", "relation_type", "canonical_name", "canonical_name_en"})
	for _, r := range rows {
		out.AddRow(r...)
	}
	return out
}

// Scenarios 1.2.2 B + 1.2.3 C: a standard and a manual match a node with equal
// text score; the standard ranks above the manual and the manual is NOT filtered.
func TestScopeStandardsBoost(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	s := DocumentScoper{DB: db, Config: scopeCfg(t)}
	nodes := []ScopeNode{sn(1, KindProduct, "Ventilator")}

	mock.ExpectQuery(rx("FROM kb.products")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(productRows(
			[]driver.Value{int64(42), "scope", "ventilator", ""},
			[]driver.Value{int64(43), "scope", "ventilator", ""},
		))
	mock.ExpectQuery(rx("lex AS (")).WillReturnRows(
		sqlmock.NewRows([]string{"artifact_type", "artifact_id", "input_record_id", "source_row_id", "score"}))
	mock.ExpectQuery(rx("FROM kb.doc_facet_values")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"record_id", "value"}).
			AddRow(int64(42), "da:doc_kind_standard").
			AddRow(int64(43), "da:doc_kind_manual"))

	docs, err := s.Scope(context.Background(), nodes)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2 (manual not filtered)", len(docs))
	}
	if docs[0].InputRecordID != 42 || docs[0].DocKind != "standard" {
		t.Fatalf("top doc = %+v, want record 42 / standard", docs[0])
	}
	if docs[1].InputRecordID != 43 {
		t.Fatalf("second doc = %+v, want the manual (record 43), ranked below", docs[1])
	}
	if !(docs[0].FusedScore > docs[1].FusedScore) {
		t.Fatalf("standard score %v not above manual score %v", docs[0].FusedScore, docs[1].FusedScore)
	}
}

// Scenarios 1.2.1 A + 1.2.4 D: a Path A product-record match attributes the
// document to the matched node — the root for a product-identity row, and a
// part node for a row whose identity is that part.
func TestScopePathAAttribution(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	s := DocumentScoper{DB: db, Config: scopeCfg(t)}
	nodes := []ScopeNode{sn(1, KindProduct, "Ventilator"), sn(2, KindPart, "Display")}

	mock.ExpectQuery(rx("FROM kb.products")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(productRows(
			[]driver.Value{int64(42), "scope", "ventilator", ""},
			[]driver.Value{int64(88), "scope", "display", ""},
		))
	mock.ExpectQuery(rx("lex AS (")).WillReturnRows(emptyRRFRows()) // node 1
	mock.ExpectQuery(rx("lex AS (")).WillReturnRows(emptyRRFRows()) // node 2
	mock.ExpectQuery(rx("FROM kb.doc_facet_values")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"record_id", "value"}))
	mock.ExpectQuery(rx("FROM kb.doc_facets")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"record_id", "input_doc_type"}))

	docs, err := s.Scope(context.Background(), nodes)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	byRec := map[int64]ScopedDoc{}
	for _, d := range docs {
		byRec[d.InputRecordID] = d
	}
	if d := byRec[42]; !containsInt(d.NodeIDs, 1) || len(d.Reasons) == 0 ||
		!contains(d.Reasons[0], "Ventilator") || !contains(d.Reasons[0], "scope") {
		t.Fatalf("doc 42 = %+v, want attributed to root node 1 with a Path A reason", d)
	}
	if d := byRec[88]; !containsInt(d.NodeIDs, 2) {
		t.Fatalf("doc 88 = %+v, want attributed to the Display part node (2)", d)
	}
}

func emptyRRFRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"artifact_type", "artifact_id", "input_record_id", "source_row_id", "score"})
}

// Scenario 1.2.5 E: a document with a storage_requirement product record for the
// root identity enters scope attributed to the `storage` aspect node.
func TestScopeAspectScoped(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	cfg := scopeCfg(t)
	s := DocumentScoper{DB: db, Config: cfg}
	nodes := []ScopeNode{
		sn(1, KindProduct, "Ventilator"),
		{ID: 9, Kind: KindAspect, Label: "Storage", AspectKey: "storage", RelationTypes: []string{"storage_requirement"}},
	}

	mock.ExpectQuery(rx("FROM kb.products")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(productRows([]driver.Value{int64(60), "storage_requirement", "ventilator", ""}))
	mock.ExpectQuery(rx("lex AS (")).WillReturnRows(
		sqlmock.NewRows([]string{"artifact_type", "artifact_id", "input_record_id", "source_row_id", "score"}))
	mock.ExpectQuery(rx("FROM kb.doc_facet_values")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"record_id", "value"}))
	mock.ExpectQuery(rx("FROM kb.doc_facets")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"record_id", "input_doc_type"}))

	docs, err := s.Scope(context.Background(), nodes)
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1", len(docs))
	}
	if !containsInt(docs[0].NodeIDs, 9) {
		t.Fatalf("doc NodeIDs = %v, want it to include the storage aspect node (9)", docs[0].NodeIDs)
	}
}

func containsInt(s []int64, v int64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
