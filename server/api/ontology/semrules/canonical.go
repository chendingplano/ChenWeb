package semrules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// Canonicalize validates a predicate document, serializes it deterministically,
// and returns both the canonical bytes and their SHA-256 checksum.
func Canonicalize(doc Document) ([]byte, string, error) {
	if err := Validate(doc); err != nil {
		return nil, "", err
	}
	normalized, err := normalizePredicate(doc.Expression)
	if err != nil {
		return nil, "", fmt.Errorf("expression: %w", err)
	}
	doc.Expression = normalized
	canonical, err := json.Marshal(doc)
	if err != nil {
		return nil, "", fmt.Errorf("marshal canonical predicate: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}

func normalizePredicate(predicate Predicate) (Predicate, error) {
	if predicate.Value != nil {
		normalized, err := normalizeJSONValue(predicate.Value)
		if err != nil {
			return Predicate{}, err
		}
		predicate.Value = normalized
	}
	// in/not_in operands are semantically a set: unlike all/any child order
	// (deliberately preserved -- see canonical_test.go), operand order and
	// duplicates carry no logical meaning and must not affect the checksum
	// (P5 review 2026080302 finding P5-24). Validate guarantees a
	// homogeneous scalar array by this point, so every element marshals to
	// a comparable JSON encoding.
	if predicate.Kind == "fact" && (predicate.Op == "in" || predicate.Op == "not_in") {
		sorted, err := sortAndDedupeArrayValue(predicate.Value)
		if err != nil {
			return Predicate{}, fmt.Errorf("value: %w", err)
		}
		predicate.Value = sorted
	}
	items := make([]Predicate, len(predicate.Items))
	for index, child := range predicate.Items {
		normalized, err := normalizePredicate(child)
		if err != nil {
			return Predicate{}, fmt.Errorf("items[%d]: %w", index, err)
		}
		items[index] = normalized
	}
	predicate.Items = items
	return predicate, nil
}

// sortAndDedupeArrayValue sorts an already-normalized array value by its
// JSON encoding and removes adjacent duplicates, giving in/not_in operand
// order and repetition no effect on the canonical checksum.
func sortAndDedupeArrayValue(value any) ([]any, error) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || (reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array) {
		return nil, fmt.Errorf("expected array, got %T", value)
	}
	type keyedValue struct {
		key   string
		value any
	}
	n := reflected.Len()
	pairs := make([]keyedValue, n)
	for i := 0; i < n; i++ {
		v := reflected.Index(i).Interface()
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		pairs[i] = keyedValue{key: string(encoded), value: v}
	}
	sort.Slice(pairs, func(a, b int) bool { return pairs[a].key < pairs[b].key })
	result := make([]any, 0, n)
	var lastKey string
	for i, p := range pairs {
		if i > 0 && p.key == lastKey {
			continue
		}
		result = append(result, p.value)
		lastKey = p.key
	}
	return result, nil
}

func normalizeJSONValue(value any) (any, error) {
	if isNumericValue(value) {
		canonical, err := canonicalNumber(value)
		if err != nil {
			return nil, err
		}
		return json.Number(canonical), nil
	}

	reflected := reflect.ValueOf(value)
	if reflected.IsValid() && (reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array) {
		result := make([]any, reflected.Len())
		for index := 0; index < reflected.Len(); index++ {
			normalized, err := normalizeJSONValue(reflected.Index(index).Interface())
			if err != nil {
				return nil, fmt.Errorf("value[%d]: %w", index, err)
			}
			result[index] = normalized
		}
		return result, nil
	}
	return value, nil
}

func canonicalNumber(value any) (string, error) {
	parsed, err := parseNumber(value)
	if err != nil {
		return "", err
	}
	return parsed.canonical, nil
}
