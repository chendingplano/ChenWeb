package modules

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
