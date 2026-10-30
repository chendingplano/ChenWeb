package wpi

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, e := n.Float64()
		return f, e == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
	case float64:
		return n, !math.IsInf(n, 0) && !math.IsNaN(n)
	}
	return 0, false
}

func checkInputValue(value any, d map[string]any) []string {
	if d["binding_mode"] == "read-only" {
		return nil
	}
	s := ""
	if value != nil {
		s = fmt.Sprint(value)
	}
	if s == "" {
		if d["required"] == true {
			return []string{"required"}
		}
		return nil
	}
	var out []string
	rules, _ := d["validation"].(map[string]any)
	length := int64(utf8.RuneCountInString(s))
	if n, ok := asInt(d["max_length"]); ok && length > n {
		out = append(out, "max_length")
	}
	if n, ok := asInt(rules["min_length"]); ok && length < n {
		out = append(out, "min_length")
	}
	if p, ok := rules["pattern"].(string); ok {
		re, err := regexp.Compile("^(?:" + p + ")$")
		if err != nil || !re.MatchString(s) {
			out = append(out, "pattern")
		}
	}
	switch d["input_type"] {
	case "number":
		n, ok := number(value)
		if !ok {
			n2, err := strconv.ParseFloat(s, 64)
			n = n2
			ok = err == nil && !math.IsInf(n, 0) && !math.IsNaN(n) && strings.TrimSpace(s) != ""
		}
		if !ok {
			out = append(out, "format")
			break
		}
		min, hasMin := number(rules["min"])
		max, hasMax := number(rules["max"])
		step, hasStep := number(rules["step"])
		if hasMin && n < min {
			out = append(out, "min")
		}
		if hasMax && n > max {
			out = append(out, "max")
		}
		if hasStep && step > 0 {
			base := 0.0
			if hasMin {
				base = min
			}
			q := (n - base) / step
			if math.Abs(q-math.Round(q)) > 1e-9 {
				out = append(out, "step")
			}
		}
	case "email":
		if !regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`).MatchString(s) {
			out = append(out, "format")
		}
	case "url":
		u, e := url.Parse(s)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			out = append(out, "format")
		}
	}
	return out
}

func checkPageInputs(page map[string]any, doc map[string]any) fieldErrors {
	_, elements, _ := validatePage(str(page["descriptor_id"]), page)
	errs := fieldErrors{}
	for _, el := range elements {
		if str(el.copy["code_module_id"]) != "input" {
			continue
		}
		ref := str(el.body["data_binding"])
		if !strings.HasPrefix(ref, "$input_doc.") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(ref, "$input_doc."), ".")
		var value any = doc
		for _, p := range parts {
			m, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = m[p]
		}
		if codes := checkInputValue(value, el.body); len(codes) > 0 {
			key := strings.Join(parts, ".")
			errs[key] = append(errs[key], codes...)
		}
	}
	return errs
}
