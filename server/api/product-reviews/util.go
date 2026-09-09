package productreviews

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// asString coerces an arbitrary JSON value to a trimmed string.
func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

// asFloat coerces a JSON number (or numeric string) to a float64.
func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	default:
		return 0
	}
}

// asStringSlice coerces a JSON array of strings, dropping blanks.
func asStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s := asString(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

var namePunctRE = regexp.MustCompile(`[._\-/]+`)

// normalizeName lower-cases, folds punctuation to spaces, and collapses
// whitespace — the same shape kb.object_nodes.normalized_names is written in
// (see docprocessing.normalizeObjectName).
func normalizeName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return unicode.ToLower(r)
	}, raw)
	raw = namePunctRE.ReplaceAllString(raw, " ")
	return strings.Join(strings.Fields(raw), " ")
}

// dedupeStrings returns s with blanks and case-insensitive duplicates removed,
// preserving first-seen order.
func dedupeStrings(s []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(s))
	for _, v := range s {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		k := strings.ToLower(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

// formatVector renders a float slice as a pgvector literal: "[1,2,3]".
func formatVector(vec []float64) string {
	parts := make([]string, len(vec))
	for i, f := range vec {
		parts[i] = strconv.FormatFloat(f, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
