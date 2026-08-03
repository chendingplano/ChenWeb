// Package keywords provides stores for the keyword lexicon (P3 Track B),
// the second semid kernel instantiation (DR16 spec 2026080101).
// Keywords are ungoverned — concepts have a linear lifecycle with no
// draft/review stage — and shipped behind KEYWORD_RESOLVER_MODE=observe.
package keywords

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableFloat64(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableBool(v bool) any {
	if !v {
		return nil
	}
	return v
}
