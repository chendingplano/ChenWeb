package validators

import (
	"fmt"
	"maps"
	"math"
	"slices"
)

// Problem is a constraint value that cannot be used; Constraint is empty for
// the module itself.
type Problem struct {
	Constraint string
	Message    string
}

// CheckValue reports why v cannot be the value of a constraint of this spec,
// or "" (design D10). null is never a value: in a class row it means "every
// instance must set it" and is handled by the caller.
func CheckValue(spec ConstraintSpec, v any) string {
	switch spec.Type {
	case "boolean":
		if _, ok := v.(bool); !ok {
			return "must be true or false"
		}
	case "integer", "non_negative_integer", "positive_integer":
		f, ok := jsonNumber(v)
		if !ok || f != math.Trunc(f) || math.Abs(f) > maxSafe {
			return "must be a whole number"
		}
		if spec.Type == "non_negative_integer" && f < 0 {
			return "must not be negative"
		}
		if spec.Type == "positive_integer" && f < 1 {
			return "must be at least 1"
		}
	case "number", "positive_number":
		f, ok := jsonNumber(v)
		if !ok {
			return "must be a number"
		}
		if spec.Type == "positive_number" && f <= 0 {
			return "must be greater than 0"
		}
	case "decimal", "positive_decimal", "bigint", "positive_bigint":
		d, ok := numberDec(v)
		whole := spec.Type == "bigint" || spec.Type == "positive_bigint"
		if s, isText := v.(string); isText && whole && !integerSyntax.MatchString(s) {
			ok = false
		}
		if !ok || (whole && !d.isInteger()) {
			if whole {
				return "must be a whole number, written as digits or a JSON number"
			}
			return "must be a decimal number, written as text or a JSON number"
		}
		if f, isNumber := jsonNumber(v); isNumber && whole && math.Abs(f) > maxSafe {
			return "must be written as text, because it is beyond the numbers JSON holds exactly"
		}
		if (spec.Type == "positive_decimal" || spec.Type == "positive_bigint") && d.sign() <= 0 {
			return "must be greater than 0"
		}
	case "string":
		if _, ok := v.(string); !ok {
			return "must be text"
		}
	case "pattern":
		p, ok := v.(string)
		if !ok {
			return "must be a regular expression"
		}
		if err := PortablePattern(p); err != nil {
			return "must be a regular expression both Go and JavaScript read the same way: " + err.Error()
		}
	case "choice":
		s, _ := v.(string)
		if !slices.Contains(spec.Choices, s) {
			return fmt.Sprintf("must be one of %v", spec.Choices)
		}
	case "json_array":
		if _, ok := v.([]any); !ok {
			return "must be an array"
		}
	default:
		return fmt.Sprintf("has unknown type %q", spec.Type)
	}
	return ""
}

// orderedPairs are constraints whose first value may not exceed the second.
var orderedPairs = [][2]string{
	{"min_length", "max_length"},
	{"minimum", "maximum"},
	{"exclusive_minimum", "exclusive_maximum"},
	{"min_bytes", "max_bytes"},
	{"min_digits", "max_digits"},
	{"scale", "precision"},
}

// CheckConstraints checks constraint values a class or an instance gives a
// module, and the rules between them on the effective values.
func CheckConstraints(module string, given map[string]any) []Problem {
	spec, ok := Modules[module]
	if !ok {
		return []Problem{{"", fmt.Sprintf("validator code module %q has no code in this interpreter", module)}}
	}
	var out []Problem
	valid := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(given)) {
		c, known := spec.Constraint(name)
		if !known {
			out = append(out, Problem{name, fmt.Sprintf("validator code %q has no constraint %q", module, name)})
			continue
		}
		if msg := CheckValue(c, given[name]); msg != "" {
			out = append(out, Problem{name, msg})
			continue
		}
		valid[name] = given[name]
	}
	eff := spec.effective(valid)
	for _, p := range orderedPairs {
		lo, okLo := numberDec(eff[p[0]])
		hi, okHi := numberDec(eff[p[1]])
		if okLo && okHi && lo.cmp(hi) > 0 {
			if p[0] == "scale" {
				out = append(out, Problem{p[0], "must not exceed precision"})
			} else {
				out = append(out, Problem{p[1], fmt.Sprintf("must not be less than %s", p[0])})
			}
		}
	}
	return out
}
