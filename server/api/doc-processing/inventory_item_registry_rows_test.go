package docprocessing

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBuildInventoryItemRegistryRowsUsesInventoryItemCategoryStatuses(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT category_key, status FROM kb\\.artifact_categories WHERE category_type = \\$1").
		WithArgs("inventory_item").
		WillReturnRows(sqlmock.NewRows([]string{"category_key", "status"}).
			AddRow("furniture", InventoryCategoryStatusApproved))

	mock.ExpectQuery("SELECT id, inventory_item_id, item_name, canonical_name, item_categories, manufacturer, brand,").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "inventory_item_id", "item_name", "canonical_name", "item_categories", "manufacturer", "brand",
			"model_number", "part_number", "normalized_specs", "standards", "aliases", "source_line_spans",
			"validation_flags", "missing_required_attrs", "dedupe_key", "confidence", "confidence_reason",
			"search_document",
		}).AddRow(
			int64(1), "42_i_1", "Chair", "Chair", []byte(`["Furniture"]`), "", "", "", "",
			[]byte(`[]`), []byte(`[]`), []byte(`[]`), []byte(`["12"]`),
			[]byte(`[]`), []byte(`[]`), "furniture|chair", 0.95, "explicit", "Chair furniture",
		))

	rows, err := buildInventoryItemRegistryRows(context.Background(), db, 42, "source.pdf")
	if err != nil {
		t.Fatalf("buildInventoryItemRegistryRows error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len=%d, want 1", len(rows))
	}
	var payload map[string]any
	if err := json.Unmarshal(rows[0].SemanticPayload, &payload); err != nil {
		t.Fatalf("semantic payload unmarshal: %v", err)
	}
	if got := payload["category_status"]; got != InventoryCategoryStatusApproved {
		t.Fatalf("category_status=%v, want approved", got)
	}
	if got := payload["validation_status"]; got != "valid" {
		t.Fatalf("validation_status=%v, want valid", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
