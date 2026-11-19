package docprocessing

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProductsV2DecodeKeepsUnlistedRelationCode(t *testing.T) {
	wire := strings.Replace(productV2Example, `"r":"use"`, `"r":"NEW_CODE"`, 1)
	rows := decodeProductV2Test(t, wire, productV2Lines)
	if rows[0]["relation_code"] != "new_code" || rows[0]["relation_type"] != "new_code" {
		t.Fatalf("unlisted code: code=%v type=%v, want new_code/new_code", rows[0]["relation_code"], rows[0]["relation_type"])
	}
}

type fakeRelationResolver struct {
	uses   []ProductRelationUse
	values map[string]string
}

func (f *fakeRelationResolver) ResolveProductRelations(_ context.Context, _ int64, uses []ProductRelationUse) (map[string]string, error) {
	f.uses = uses
	return f.values, nil
}

func TestResolveProductRelationsAppliesTableValues(t *testing.T) {
	fake := &fakeRelationResolver{values: map[string]string{"new_code": "new_code", "stor": "storage_requirement"}}
	p := &ProductsProcessor{Relations: fake}
	payload := map[string]any{"products": []any{
		map[string]any{"product_name": "a", "relation_code": "new_code", "relation_type": "new_code"},
		map[string]any{"product_name": "b", "relation_code": "stor", "relation_type": "x"},
		map[string]any{"product_name": "c", "relation_code": "new_code", "relation_type": "new_code"},
	}}
	if err := p.resolveProductRelations(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	rows := payload["products"].([]any)
	if rows[1].(map[string]any)["relation_type"] != "storage_requirement" {
		t.Fatalf("relation_type not resolved: %v", rows[1])
	}
	for _, r := range rows {
		if _, ok := r.(map[string]any)["relation_code"]; ok {
			t.Fatalf("relation_code left in row: %v", r)
		}
	}
	if len(fake.uses) != 2 || fake.uses[0].Code != "new_code" || fake.uses[0].Count != 2 {
		t.Fatalf("uses = %+v, want new_code x2 then stor", fake.uses)
	}
}

func TestResolveProductRelationsWithoutResolverKeepsDecodeValue(t *testing.T) {
	p := &ProductsProcessor{}
	payload := map[string]any{"products": []any{
		map[string]any{"relation_code": "stor", "relation_type": "storage_requirement"},
	}}
	if err := p.resolveProductRelations(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	row := payload["products"].([]any)[0].(map[string]any)
	if row["relation_type"] != "storage_requirement" || row["relation_code"] != nil {
		t.Fatalf("row = %v", row)
	}
}

func TestProductRelationStoreKnownCode(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("UPDATE kb.product_relation_types").WithArgs("stor", int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("storage_requirement"))
	log := &fakeLogger{}
	got, err := ProductRelationSQLStore{DB: db, Logger: log}.ResolveProductRelations(
		context.Background(), 377, []ProductRelationUse{{Code: "stor", Count: 3}})
	if err != nil || got["stor"] != "storage_requirement" {
		t.Fatalf("got %v err %v", got, err)
	}
	if len(log.warns) != 0 {
		t.Fatalf("known code warned: %v", log.warns)
	}
}

func TestProductRelationStoreProposesUnknownCodeOnceWithWarnings(t *testing.T) {
	t.Setenv("MAX_PRODUCT_RELATION_TYPES", "21")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("UPDATE kb.product_relation_types").WithArgs("new_code", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"value"}))
	mock.ExpectQuery("INSERT INTO kb.product_relation_types").
		WithArgs("new_code", int64(377), "机房空调", int64(2), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"value", "inserted"}).AddRow("new_code", true))
	mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(22))
	log := &fakeLogger{}
	got, err := ProductRelationSQLStore{DB: db, Logger: log}.ResolveProductRelations(
		context.Background(), 377, []ProductRelationUse{{Code: "new_code", Count: 2, ProductName: "机房空调"}})
	if err != nil || got["new_code"] != "new_code" {
		t.Fatalf("got %v err %v", got, err)
	}
	if len(log.warns) != 2 || !strings.Contains(log.warns[0].message, "new product relation type proposed") ||
		!strings.Contains(log.warns[1].message, "limit exceeded") {
		t.Fatalf("warns = %+v, want proposal then limit", log.warns)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProductRelationStoreExistingProposalDoesNotWarn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Lost the race: another run inserted the code between our UPDATE and INSERT.
	mock.ExpectQuery("UPDATE kb.product_relation_types").WillReturnRows(sqlmock.NewRows([]string{"value"}))
	mock.ExpectQuery("INSERT INTO kb.product_relation_types").
		WillReturnRows(sqlmock.NewRows([]string{"value", "inserted"}).AddRow("new_code", false))
	log := &fakeLogger{}
	if _, err := (ProductRelationSQLStore{DB: db, Logger: log}).ResolveProductRelations(
		context.Background(), 1, []ProductRelationUse{{Code: "new_code", Count: 1}}); err != nil {
		t.Fatal(err)
	}
	if len(log.warns) != 0 {
		t.Fatalf("warns = %+v", log.warns)
	}
}

func TestMaxProductRelationTypesDefault(t *testing.T) {
	t.Setenv("MAX_PRODUCT_RELATION_TYPES", "")
	if got := maxProductRelationTypes(); got != 50 {
		t.Fatalf("default = %d, want 50", got)
	}
	t.Setenv("MAX_PRODUCT_RELATION_TYPES", "7")
	if got := maxProductRelationTypes(); got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}
