package wpi

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/chendingplano/deepdoc/server/api/wpi/validators"
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
	fields, _ := checkInputs(page, doc)
	return fields
}

// validationErrors are validator failures by bound attribute path, such as
// "address.city" (OpenSpec wpi-primitive-validators, design D7).
type validationErrors map[string][]validators.Error

// inputErrors are the failures of a page's input rules and validators; a save
// with any is refused with 422.
type inputErrors struct {
	fields     fieldErrors
	validation validationErrors
}

func (e inputErrors) Error() string { return "input checks failed" }

// codes lists the validator error codes, for the log (§2.6.8).
func (e inputErrors) codes() []string {
	var out []string
	for _, key := range slices.Sorted(maps.Keys(e.validation)) {
		for _, v := range e.validation[key] {
			out = append(out, key+":"+v.Code)
		}
	}
	return out
}

// checkInputs checks the input doc against the page's elements bound to it:
// each input's own rules, and the validators of every input, select and
// value-editor element that is not read-only (design D8), which run here
// again whatever the browser checked.
func checkInputs(page map[string]any, doc map[string]any) (fieldErrors, validationErrors) {
	_, elements, _ := validatePage(str(page["descriptor_id"]), page)
	errs, vals := fieldErrors{}, validationErrors{}
	for _, el := range elements {
		module := str(el.copy["code_module_id"])
		list, _ := el.body["validators"].([]any)
		if module != "input" && (!acceptsValidators(module) || len(list) == 0) {
			continue
		}
		ref := str(el.body["data_binding"])
		if !strings.HasPrefix(ref, "$input_doc.") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(ref, "$input_doc."), ".")
		var value any = doc
		present := true
		for _, p := range parts {
			m, ok := value.(map[string]any)
			if !ok {
				value, present = nil, false
				break
			}
			value, present = m[p]
		}
		key := strings.Join(parts, ".")
		if module == "input" {
			if codes := checkInputValue(value, el.body); len(codes) > 0 {
				errs[key] = append(errs[key], codes...)
			}
		}
		if len(list) > 0 && el.body["binding_mode"] != "read-only" {
			checked := value
			if !present {
				checked = validators.Absent
			}
			if found := runValidators(list, checked, validatorPointer(parts)); len(found) > 0 {
				vals[key] = append(vals[key], found...)
			}
		}
	}
	return errs, vals
}
