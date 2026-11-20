package docsync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"
)

// VocabularyChanged checks the source window using the write timestamps on
// each vocabulary table. The complete reconciliation digest also catches
// deletions and edits without a timestamp change.
func VocabularyChanged(ctx context.Context, tx *sql.Tx, after, until time.Time) (bool, error) {
	if after.IsZero() {
		return true, nil
	}
	var checks []string
	for _, name := range vocabularyTables {
		timestamp := "GREATEST(create_time, modify_time)"
		switch name {
		case "ontology_term_headers", "ontology_term_redirects", "ontology_class_contract_revisions":
			timestamp = "create_time"
		case "ontology_class_contract_capabilities":
			timestamp = "declared_time"
		case "ontology_module_releases":
			timestamp = "GREATEST(create_time, modify_time, released_at)"
		}
		checks = append(checks, fmt.Sprintf("EXISTS (SELECT 1 FROM kb.%s WHERE %s > $1 AND %s <= $2)", name, timestamp, timestamp))
	}
	var changed bool
	err := tx.QueryRowContext(ctx, "SELECT "+strings.Join(checks, " OR "), after, until).Scan(&changed)
	return changed, err
}

func validateGovernedTerms(ctx context.Context, tx *sql.Tx, bundle Bundle) error {
	keys := make(map[string]struct{})
	for table, columns := range governedTermFields {
		for _, row := range bundle.Rows[table] {
			for _, column := range strings.Fields(columns) {
				if term := stringField(row.Fields, column); term != "" {
					keys[term] = struct{}{}
				}
			}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	terms := slices.Collect(maps.Keys(keys))
	var missing string
	err := tx.QueryRowContext(ctx, `SELECT key FROM unnest($1::text[]) AS wanted(key)
		WHERE NOT EXISTS (SELECT 1 FROM kb.ontology_term_headers h WHERE h.term_id=wanted.key)
		ORDER BY key LIMIT 1`, pq.Array(terms)).Scan(&missing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("waiting for governed term %s", missing)
}
