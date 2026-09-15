package datasync

import "encoding/json"

// Row is one transferred record, keyed by column name. Every value is valid
// JSON: JSONB columns are embedded verbatim (already valid JSON), every other
// column is either a JSON-encoded string (e.g. `"2026-09-15T00:00:00Z"`,
// `"12"`) or the literal `null`. Keeping every column as an opaque
// json.RawMessage avoids needing per-column Go types for values that only
// ever flow through as text on both the SQL and the wire side.
type Row map[string]json.RawMessage

var nullJSON = json.RawMessage("null")

func isNullJSON(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}
