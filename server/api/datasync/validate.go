package datasync

import (
	"fmt"
	"regexp"
)

// Validates admin-submitted sync item shapes before they're persisted and
// string-interpolated into generated SQL (fetchChangesPage, upsertRows, the
// per-file lookup). A compiled TableSyncItem literal is written and reviewed
// like any other Go code; a kb.data_sync_items row comes from an HTML form,
// so it gets the same treatment identifiers already implicitly get by being
// valid Go source. This is fat-finger protection for an already-privileged
// sysadmin session, not a trust-boundary fix -- see design.md Decision 5.

var (
	idPattern         = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	tablePattern      = regexp.MustCompile(`^[a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*$`)
)

func validateID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("id %q must start with a lowercase letter and contain only lowercase letters, digits, '_' or '-'", id)
	}
	return nil
}

func validateTable(table string) error {
	if !tablePattern.MatchString(table) {
		return fmt.Errorf("table %q must be a schema-qualified identifier like kb.some_table", table)
	}
	return nil
}

func validateIdentifier(field, name string) error {
	if !identifierPattern.MatchString(name) {
		return fmt.Errorf("%s %q is not a valid SQL identifier", field, name)
	}
	return nil
}

func containsString(vals []string, target string) bool {
	for _, v := range vals {
		if v == target {
			return true
		}
	}
	return false
}

// validateItem checks a submitted TableSyncItem shape ahead of persisting it
// (create) or replacing an existing row's shape (edit). Does not check id
// uniqueness -- that's insertItem's job, since it needs a DB round-trip.
func validateItem(item TableSyncItem) error {
	if err := validateID(item.ID); err != nil {
		return err
	}
	if err := validateTable(item.Table); err != nil {
		return err
	}
	if err := validateIdentifier("cursor column", item.CursorCol); err != nil {
		return err
	}
	if len(item.Columns) == 0 {
		return fmt.Errorf("at least one column is required")
	}
	for _, col := range item.Columns {
		if err := validateIdentifier("column", col); err != nil {
			return err
		}
	}
	if !containsString(item.Columns, item.CursorCol) {
		return fmt.Errorf("cursor column %q must be included in columns", item.CursorCol)
	}
	if len(item.NaturalKey) == 0 {
		return fmt.Errorf("at least one natural-key column is required")
	}
	for _, col := range item.NaturalKey {
		if err := validateIdentifier("natural-key column", col); err != nil {
			return err
		}
		if !containsString(item.Columns, col) {
			return fmt.Errorf("natural-key column %q must be included in columns", col)
		}
	}
	for _, col := range item.JSONColumns {
		if err := validateIdentifier("JSON column", col); err != nil {
			return err
		}
		if !containsString(item.Columns, col) {
			return fmt.Errorf("JSON column %q must be included in columns", col)
		}
	}

	switch item.Kind {
	case "", KindTable:
		// nothing further to check
	case KindTableWithFiles:
		if item.FileColumn == "" {
			return fmt.Errorf("file column is required for kind %q", KindTableWithFiles)
		}
		if err := validateIdentifier("file column", item.FileColumn); err != nil {
			return err
		}
		if !containsString(item.Columns, item.FileColumn) {
			return fmt.Errorf("file column %q must be included in columns", item.FileColumn)
		}
		if item.FileDirEnv == "" && item.FileDirDefaultSubdir == "" {
			return fmt.Errorf("at least one of file dir env var or file dir default subdirectory is required for kind %q", KindTableWithFiles)
		}
	default:
		return fmt.Errorf("unknown kind %q", item.Kind)
	}
	return nil
}
