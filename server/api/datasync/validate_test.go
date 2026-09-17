package datasync

import "testing"

func validTestItem() TableSyncItem {
	return TableSyncItem{
		ID:         "test_item",
		Table:      "kb.some_table",
		CursorCol:  "update_time",
		NaturalKey: []string{"natural_col"},
		Columns:    []string{"natural_col", "update_time", "other_col"},
	}
}

func TestValidateItemAcceptsValidTableShape(t *testing.T) {
	if err := validateItem(validTestItem()); err != nil {
		t.Fatalf("validateItem() error = %v, want nil", err)
	}
}

func TestValidateItemRejectsBadTableName(t *testing.T) {
	item := validTestItem()
	item.Table = "kb.some_table; DROP TABLE x"
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error for a malformed table name")
	}
}

func TestValidateItemRejectsUnqualifiedTableName(t *testing.T) {
	item := validTestItem()
	item.Table = "some_table"
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error for a non-schema-qualified table name")
	}
}

func TestValidateItemRejectsBadColumnName(t *testing.T) {
	item := validTestItem()
	item.Columns = append(item.Columns, "bad col")
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error for an invalid column name")
	}
}

func TestValidateItemRejectsCursorColNotInColumns(t *testing.T) {
	item := validTestItem()
	item.CursorCol = "not_in_columns"
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error when cursor_col isn't in columns")
	}
}

func TestValidateItemRejectsNaturalKeyNotInColumns(t *testing.T) {
	item := validTestItem()
	item.NaturalKey = []string{"not_in_columns"}
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error when a natural-key column isn't in columns")
	}
}

func TestValidateItemRequiresFileColumnForTableWithFiles(t *testing.T) {
	item := validTestItem()
	item.Kind = KindTableWithFiles
	item.FileDirDefaultSubdir = "Videos"
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error when table_with_files has no file column")
	}
}

func TestValidateItemRequiresFileColumnInColumns(t *testing.T) {
	item := validTestItem()
	item.Kind = KindTableWithFiles
	item.FileColumn = "not_in_columns"
	item.FileDirDefaultSubdir = "Videos"
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error when file_column isn't in columns")
	}
}

func TestValidateItemRequiresAFileDirForTableWithFiles(t *testing.T) {
	item := validTestItem()
	item.Kind = KindTableWithFiles
	item.FileColumn = "natural_col"
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error when neither file dir env nor default subdir is set")
	}
}

func TestValidateItemAcceptsValidTableWithFilesShape(t *testing.T) {
	item := validTestItem()
	item.Kind = KindTableWithFiles
	item.FileColumn = "natural_col"
	item.FileDirEnv = "VIDEO_DIR"
	if err := validateItem(item); err != nil {
		t.Fatalf("validateItem() error = %v, want nil", err)
	}
}

func TestValidateItemRejectsUnknownKind(t *testing.T) {
	item := validTestItem()
	item.Kind = "bogus"
	if err := validateItem(item); err == nil {
		t.Fatalf("validateItem() error = nil, want a validation error for an unknown kind")
	}
}

func TestValidateIDRejectsMalformedIDs(t *testing.T) {
	for _, id := range []string{"Bad_ID", "1bad", "bad id", ""} {
		if err := validateID(id); err == nil {
			t.Fatalf("validateID(%q) error = nil, want a validation error", id)
		}
	}
}
