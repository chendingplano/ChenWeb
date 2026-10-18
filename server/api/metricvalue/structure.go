// Package metricvalue defines the stored structure of non-scalar metric values.
package metricvalue

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/width"
)

// NormalizeStructure validates the additive value_structure field. A null field
// denotes the legacy representation; it does not certify scalar parseability.
func NormalizeStructure(raw any) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	var data []byte
	switch v := raw.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	case json.RawMessage:
		data = v
	default:
		var err error
		data, err = json.Marshal(raw)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(string(data)) == "" || strings.TrimSpace(string(data)) == "null" {
		return nil, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("invalid metric value_structure: %w", err)
	}
	if obj == nil {
		return nil, fmt.Errorf("metric value_structure must be an object")
	}
	for k := range obj {
		if k != "kind" && k != "values" {
			return nil, fmt.Errorf("unknown value_structure field %q", k)
		}
	}
	kind, _ := obj["kind"].(string)
	switch kind {
	case "alternatives", "required_points", "composite", "expression", "unresolved":
	default:
		return nil, fmt.Errorf("unknown metric value_structure kind %q", kind)
	}
	out := map[string]any{"kind": kind}
	if v, exists := obj["values"]; exists {
		members, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("value_structure.values must be an array of strings")
		}
		values := make([]string, 0, len(members))
		seen := make(map[string]bool)
		for _, member := range members {
			s, ok := member.(string)
			s = strings.TrimSpace(s)
			if !ok || s == "" {
				return nil, fmt.Errorf("value_structure members must be nonempty strings")
			}
			if kind != "composite" && seen[s] {
				return nil, fmt.Errorf("duplicate value_structure member %q", s)
			}
			seen[s] = true
			values = append(values, s)
		}
		out["values"] = values
	}
	if kind == "alternatives" || kind == "required_points" || kind == "composite" {
		values, _ := out["values"].([]string)
		if len(values) < 2 {
			return nil, fmt.Errorf("%s requires at least two members", kind)
		}
	}
	return out, nil
}

// IsNonScalar is deliberately conservative: malformed structures must not
// accidentally opt into scalar normalization.
func IsNonScalar(raw any) bool {
	v, err := NormalizeStructure(raw)
	return err != nil || v != nil
}

// StructureEqual ignores member order only for unordered collections.
func StructureEqual(a, b any) bool {
	left, le := NormalizeStructure(a)
	right, re := NormalizeStructure(b)
	if le != nil || re != nil {
		return false
	}
	for _, v := range []map[string]any{left, right} {
		if v == nil {
			continue
		}
		if v["kind"] == "alternatives" || v["kind"] == "required_points" {
			values := append([]string(nil), v["values"].([]string)...)
			sort.Strings(values)
			v["values"] = values
		}
	}
	return reflect.DeepEqual(left, right)
}

// SQLValue returns a validated JSON parameter, preserving SQL NULL for legacy rows.
func SQLValue(raw any) (any, error) {
	v, err := NormalizeStructure(raw)
	if err != nil || v == nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	return string(b), err
}

var valueNumber = regexp.MustCompile(`[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?`)

// ValidateForValue prevents structured members from contradicting their display
// value. This is a numeric loss guard, not a substitute for source interpretation.
func ValidateForValue(raw any, display string) error {
	s, err := NormalizeStructure(raw)
	if err != nil || s == nil {
		return err
	}
	kind := s["kind"]
	if kind != "alternatives" && kind != "required_points" && kind != "composite" {
		return nil
	}
	values := s["values"].([]string)
	numbers := func(text string) []string {
		text = width.Fold.String(text)
		var result []string
		for _, token := range valueNumber.FindAllString(text, -1) {
			n, e := strconv.ParseFloat(token, 64)
			if e == nil {
				result = append(result, strconv.FormatFloat(n, 'g', -1, 64))
			}
		}
		return result
	}
	want, got := numbers(strings.Join(values, " ")), numbers(display)
	if len(want) == 0 {
		for _, value := range values {
			if !strings.Contains(display, value) {
				return fmt.Errorf("value_structure member %q missing from metric_value", value)
			}
		}
		return nil
	}
	if kind != "composite" {
		sort.Strings(want)
		sort.Strings(got)
	}
	if !reflect.DeepEqual(want, got) {
		return fmt.Errorf("value_structure members disagree with metric_value")
	}
	return nil
}
