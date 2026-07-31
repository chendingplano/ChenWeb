package candidates

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
