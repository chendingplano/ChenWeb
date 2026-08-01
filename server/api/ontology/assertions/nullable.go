package assertions

import "time"

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableBool(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// nullableJSON returns the JSON literal "null" for an empty payload rather
// than a Go nil (SQL NULL): database/sql's scan of a real SQL NULL into
// json.RawMessage errors in this environment, whereas the JSON literal
// scans cleanly and round-trips as an empty RawMessage on read (the same
// convention P2's candidate store uses for source_line_spans/candidate_matches).
func nullableJSON(raw []byte) any {
	if len(raw) == 0 {
		return "null"
	}
	return string(raw)
}
