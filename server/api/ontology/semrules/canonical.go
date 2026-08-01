package semrules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"
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

func normalizeJSONValue(value any) (any, error) {
	if _, err := numericValue(value); err == nil {
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
	rational, err := numericValue(value)
	if err != nil {
		return "", err
	}
	denominator := new(big.Int).Set(rational.Denom())
	two := 0
	five := 0
	for new(big.Int).Mod(denominator, big.NewInt(2)).Sign() == 0 {
		denominator.Div(denominator, big.NewInt(2))
		two++
	}
	for new(big.Int).Mod(denominator, big.NewInt(5)).Sign() == 0 {
		denominator.Div(denominator, big.NewInt(5))
		five++
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return "", fmt.Errorf("number %v has no finite decimal representation", value)
	}
	precision := two
	if five > precision {
		precision = five
	}
	result := rational.FloatString(precision)
	if strings.Contains(result, ".") {
		result = strings.TrimRight(strings.TrimRight(result, "0"), ".")
	}
	if result == "-0" {
		return "0", nil
	}
	return result, nil
}
