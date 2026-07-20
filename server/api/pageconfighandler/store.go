// Package pageconfighandler serves DB-backed, language-aware page content
// configuration (kb.page_def / kb.page_config). It resolves, for a page and a
// requesting user, the entries that are enabled and authorized, with content
// resolved for the requested locale and a fallback to [languages].default.
// See KnowledgeStore spec 2026072001 §9 and change db-backed-page-config.
package pageconfighandler

import (
	"context"
	"database/sql"
	"encoding/json"
)

// configRow is one kb.page_config row (one entry for one language).
type configRow struct {
	EntryKey   string
	Language   string
	Content    json.RawMessage
	AccessRole []string
	Accessible bool
	Enabled    bool
	EntryDesc  string
}

// pageExists reports whether a kb.page_def row exists for pageKey.
func pageExists(ctx context.Context, db *sql.DB, pageKey string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM kb.page_def WHERE page_key = $1)`, pageKey).Scan(&exists)
	return exists, err
}

// loadPageConfigRows loads every kb.page_config row for a page, across all
// languages. Callers group by entry_key to resolve per-entry access + content.
func loadPageConfigRows(ctx context.Context, db *sql.DB, pageKey string) ([]configRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT entry_key, language, content, access_role, accessible, enabled, COALESCE(entry_desc, '')
		FROM kb.page_config
		WHERE page_key = $1`, pageKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []configRow{}
	for rows.Next() {
		var (
			r          configRow
			content    []byte
			accessRole []byte
		)
		if err := rows.Scan(&r.EntryKey, &r.Language, &content, &accessRole, &r.Accessible, &r.Enabled, &r.EntryDesc); err != nil {
			return nil, err
		}
		if len(content) > 0 {
			r.Content = json.RawMessage(content)
		} else {
			r.Content = json.RawMessage("{}")
		}
		if len(accessRole) > 0 {
			// access_role is a JSON array of role tokens; ignore malformed values
			// (they resolve to an empty list, which suspends the entry — fail closed).
			_ = json.Unmarshal(accessRole, &r.AccessRole)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// entryRows holds the per-language rows for a single entry_key.
type entryRows struct {
	byLang map[string]configRow
}

// groupByEntry indexes rows by entry_key then language.
func groupByEntry(rows []configRow) map[string]entryRows {
	out := map[string]entryRows{}
	for _, r := range rows {
		e, ok := out[r.EntryKey]
		if !ok {
			e = entryRows{byLang: map[string]configRow{}}
			out[r.EntryKey] = e
		}
		e.byLang[r.Language] = r
	}
	return out
}
