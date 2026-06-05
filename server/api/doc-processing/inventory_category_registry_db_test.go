package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInventoryCategoryRegistrySeedApprovedCategoriesUsesScopedConflictTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("CREATE SCHEMA IF NOT EXISTS kb;")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.artifact_categories") + `[\s\S]*` + regexp.QuoteMeta("ON CONFLICT (category_type, category_key) DO UPDATE SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	reg := InventoryCategoryRegistry{DB: db}
	dict := inventoryDictionary{
		Categories: map[string]inventoryCategorySchema{
			"Furniture": {
				RequiredAttrs: []string{"material"},
				Specs: map[string]InventorySpecSchema{
					"size": {},
				},
			},
		},
		PlausibleRanges: map[string]map[string]InventoryPlausibleRange{
			"furniture": {
				"weight": {},
			},
		},
	}

	if err := reg.SeedApprovedCategories(context.Background(), dict); err != nil {
		t.Fatalf("SeedApprovedCategories error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestInventoryCategoryRegistryMintCategoryUsesScopedConflictTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.artifact_categories") + `[\s\S]*` + regexp.QuoteMeta("ON CONFLICT (category_type, category_key) DO UPDATE SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	reg := InventoryCategoryRegistry{DB: db}
	if err := reg.mintCategory(context.Background(), "furniture", "Furniture", []float64{0.1, 0.2}); err != nil {
		t.Fatalf("mintCategory error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
