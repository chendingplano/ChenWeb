package terms

import "encoding/json"

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// nullableJSON marshals a properties map to JSON for a jsonb column, or nil
// (SQL NULL) when empty -- matching how kind-specific properties are absent
// for the 7 term kinds that never populate them.
func nullableJSON(m map[string]any) any {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

// scanJSONMap unmarshals a jsonb column's raw bytes into a properties map;
// nil/empty input yields a nil map.
func scanJSONMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
