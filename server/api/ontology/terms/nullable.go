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

// nullableStringArray marshals a string slice to JSON for a jsonb column,
// or nil (SQL NULL) when empty.
func nullableStringArray(list []string) any {
	if len(list) == 0 {
		return nil
	}
	b, err := json.Marshal(list)
	if err != nil {
		return nil
	}
	return b
}

// scanStringArray unmarshals a jsonb column's raw bytes into a string slice;
// nil/empty input yields a nil slice.
func scanStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
