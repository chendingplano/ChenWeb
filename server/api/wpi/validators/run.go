package validators

import (
	"maps"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Absent stands for a value that is not there at all, which is not the same as
// null: an input doc without the bound attribute.
var Absent any = absent{}

type absent struct{}

// Error is one failure, in the shape of JSON Schema's output (§2.6.3): a code,
// the JSON Pointer of the value, the params its text is made from, and the
// index of the validator in its element's list.
type Error struct {
	Code      string         `json:"code"`
	Path      string         `json:"path"`
	Params    map[string]any `json:"params"`
	Validator int            `json:"validator"`
}

// Result is the outcome of one validator. Value is the normalized value, when
// the value is valid and has one (§2.6.1); nothing writes it back yet.
type Result struct {
	Valid    bool
	Errors   []Error
	Value    any
	HasValue bool
}

// Run checks value with a validator code module and its constraints, which are
// overlaid on the module's code defaults. path is the JSON Pointer reported in
// errors.
func Run(module string, constraints map[string]any, value any, path string) Result {
	spec, ok := Modules[module]
	if !ok {
		return Result{Errors: []Error{{Code: "validator.unknown", Path: path, Params: map[string]any{"module": module}}}}
	}
	r := &run{module: module, c: spec.effective(constraints), path: path}
	var out any
	var has bool
	switch module {
	case "string":
		out, has = r.text(value)
	case "integer":
		out, has = r.integer(value)
	case "number":
		out, has = r.number(value)
	case "decimal":
		out, has = r.decimal(value)
	case "boolean":
		out, has = r.boolean(value)
	case "enum":
		out, has = r.enum(value)
	case "null":
		out, has = r.null(value)
	case "binary":
		out, has = r.binary(value)
	case "bigint":
		out, has = r.bigint(value)
	}
	res := Result{Valid: len(r.errs) == 0, Errors: r.errs}
	if res.Valid && has {
		res.Value, res.HasValue = out, true
	}
	return res
}

type run struct {
	module string
	c      map[string]any
	path   string
	errs   []Error
}

func (r *run) fail(code string, params map[string]any) {
	if params == nil {
		params = map[string]any{}
	}
	r.errs = append(r.errs, Error{Code: r.module + "." + code, Path: r.path, Params: params})
}

func (r *run) str(name string) string { s, _ := r.c[name].(string); return s }

func (r *run) flag(name string) bool { b, _ := r.c[name].(bool); return b }

func (r *run) count(name string) (int, bool) {
	f, ok := jsonNumber(r.c[name])
	return int(f), ok
}

func isEmpty(v any) bool { return v == Absent || v == nil || v == "" }

// empty applies the empty constraint (design D6) to an empty value: pass ends
// the checks, fail reports <module>.empty, check goes on with zero instead.
func (r *run) empty(v, zero any) (next any, done bool) {
	if !isEmpty(v) {
		return v, false
	}
	switch r.str("empty") {
	case "fail":
		r.fail("empty", nil)
		return nil, true
	case "check":
		return zero, false
	}
	return nil, true
}

func (r *run) text(v any) (any, bool) {
	v, done := r.empty(v, "")
	if done {
		return nil, false
	}
	s, ok := v.(string)
	if !ok {
		r.fail("type", nil)
		return nil, false
	}
	s = normalize(trimSpace(s, r.str("trim")), r.str("normalization"))
	unit := r.str("length_unit")
	n := textLength(s, unit)
	if min, ok := r.count("min_length"); ok && n < min {
		r.fail("min_length", map[string]any{"min_length": r.c["min_length"], "length_unit": unit, "actual": n})
	}
	if max, ok := r.count("max_length"); ok && n > max {
		r.fail("max_length", map[string]any{"max_length": r.c["max_length"], "length_unit": unit, "actual": n})
	}
	if r.flag("non_blank") && strings.TrimFunc(s, isSpace) == "" {
		r.fail("blank", nil)
	}
	if r.flag("single_line") && strings.ContainsFunc(s, isLineBreak) {
		r.fail("line_break", nil)
	}
	if p, ok := r.c["pattern"].(string); ok {
		if utf8.RuneCountInString(s) > PatternLimit {
			r.fail("pattern_limit", map[string]any{"limit": PatternLimit})
		} else if !matchesWhole(p, s) {
			r.fail("pattern", map[string]any{"pattern": p})
		}
	}
	if p, ok := r.c["prefix"].(string); ok && !strings.HasPrefix(s, p) {
		r.fail("prefix", map[string]any{"prefix": p})
	}
	if p, ok := r.c["suffix"].(string); ok && !strings.HasSuffix(s, p) {
		r.fail("suffix", map[string]any{"suffix": p})
	}
	if p, ok := r.c["contains"].(string); ok && !strings.Contains(s, p) {
		r.fail("contains", map[string]any{"contains": p})
	}
	return s, true
}

// wholeNumber reads a JSON number that must be a safe integer, reporting
// <module>.type or unsafeCode.
func (r *run) wholeNumber(v any, unsafeCode string) (dec, bool) {
	f, ok := jsonNumber(v)
	if !ok || f != math.Trunc(f) {
		r.fail("type", nil)
		return dec{}, false
	}
	if math.Abs(f) > maxSafe {
		r.fail(unsafeCode, nil)
		return dec{}, false
	}
	return fromFloat(f), true
}

func (r *run) integer(v any) (any, bool) {
	if _, done := r.empty(v, nil); done {
		return nil, false
	}
	var d dec
	if s, ok := v.(string); ok {
		t := trimSpace(s, "both")
		if !r.flag("accept_strings") || len(t) > maxNumberText || !integerSyntax.MatchString(t) {
			r.fail("type", nil)
			return nil, false
		}
		d, _ = parseDec(t)
		if d.cmp(safeMax) > 0 || d.cmp(safeMin) < 0 {
			r.fail("unsafe", nil)
			return nil, false
		}
	} else if d, ok = r.wholeNumber(v, "unsafe"); !ok {
		return nil, false
	}
	r.numberChecks(d, integerRanges)
	return d.float(), true
}

func (r *run) number(v any) (any, bool) {
	if _, done := r.empty(v, nil); done {
		return nil, false
	}
	var f float64
	if s, ok := v.(string); ok {
		t := trimSpace(s, "both")
		switch {
		case !r.flag("accept_strings"):
			r.fail("type", nil)
			return nil, false
		case t == "Infinity" || t == "+Infinity":
			f = math.Inf(1)
		case t == "-Infinity":
			f = math.Inf(-1)
		case t == "NaN":
			f = math.NaN()
		case len(t) > maxNumberText || !decimalSyntax.MatchString(t):
			r.fail("type", nil)
			return nil, false
		default:
			f, _ = strconv.ParseFloat(t, 64) // ±Inf on overflow, as in JavaScript
		}
	} else if f, ok = jsonNumber(v); !ok {
		r.fail("type", nil)
		return nil, false
	}
	if math.IsInf(f, 0) || math.IsNaN(f) {
		// Other constraints do not apply to a non-finite value.
		if !r.flag("allow_non_finite") {
			r.fail("finite", nil)
		}
		return nil, false
	}
	if r.flag("integer_only") && f != math.Trunc(f) {
		r.fail("integer", nil)
	}
	r.numberChecks(fromFloat(f), nil)
	if f == 0 {
		f = 0 // no minus zero
	}
	return f, true
}

func (r *run) decimal(v any) (any, bool) {
	if _, done := r.empty(v, nil); done {
		return nil, false
	}
	var d dec
	if s, ok := v.(string); ok {
		if d, ok = parseDec(trimSpace(s, "both")); !ok {
			r.fail("type", nil)
			return nil, false
		}
	} else {
		f, ok := jsonNumber(v)
		if !ok || !r.flag("accept_numbers") {
			r.fail("type", nil)
			return nil, false
		}
		d = fromFloat(f)
	}
	scale, hasScale := r.count("scale")
	if hasScale && d.fracDigits() > scale {
		if mode := r.str("rounding"); mode == "reject" || mode == "" {
			r.fail("scale", map[string]any{"scale": r.c["scale"], "actual": d.fracDigits()})
		} else {
			d = d.round(scale, mode)
		}
	}
	if precision, ok := r.count("precision"); ok {
		if hasScale {
			allowed := max(0, precision-scale)
			if d.intDigits() > allowed {
				r.fail("integer_digits", map[string]any{"integer_digits": allowed, "actual": d.intDigits()})
			}
		} else if total := d.intDigits() + d.fracDigits(); total > precision {
			r.fail("precision", map[string]any{"precision": r.c["precision"], "actual": total})
		}
	}
	r.numberChecks(d, nil)
	return d.String(), true
}

func (r *run) boolean(v any) (any, bool) {
	v, done := r.empty(v, false)
	if done {
		return nil, false
	}
	var b bool
	switch x := v.(type) {
	case bool:
		b = x
	case string:
		switch t := asciiLower(trimSpace(x, "both")); {
		case !r.flag("accept_strings") || (t != "true" && t != "false"):
			r.fail("type", nil)
			return nil, false
		default:
			b = t == "true"
		}
	default:
		f, ok := jsonNumber(v)
		if !ok || !r.flag("accept_numbers") || (f != 0 && f != 1) {
			r.fail("type", nil)
			return nil, false
		}
		b = f == 1
	}
	if want := r.str("must_be"); (want == "true" && !b) || (want == "false" && b) {
		r.fail("must_be", map[string]any{"must_be": want})
	}
	return b, true
}

func (r *run) enum(v any) (any, bool) {
	// check compares the empty value itself; absent is compared as null.
	if _, done := r.empty(v, nil); done {
		return nil, false
	}
	if v == Absent {
		v = nil
	}
	key := enumKey(v, r.flag("case_sensitive"), r.flag("type_sensitive"))
	in := func(list []any) bool {
		return slices.ContainsFunc(list, func(x any) bool {
			return enumKey(x, r.flag("case_sensitive"), r.flag("type_sensitive")) == key
		})
	}
	if allowed, ok := r.c["allowed_values"].([]any); ok && !in(allowed) {
		r.fail("not_allowed", map[string]any{"allowed_values": allowed})
	}
	if excluded, ok := r.c["excluded_values"].([]any); ok && in(excluded) {
		r.fail("excluded", map[string]any{"excluded_values": excluded})
	}
	return v, true
}

func (r *run) null(v any) (any, bool) {
	isNull := v == nil || (v == Absent && r.flag("absent_is_null")) || (v == "" && r.flag("empty_string_is_null"))
	switch r.str("mode") {
	case "must_not_be_null":
		if isNull {
			r.fail("must_not_be_null", nil)
		}
	default:
		if !isNull {
			r.fail("must_be_null", nil)
		}
	}
	if isNull {
		return nil, true
	}
	return v, v != Absent
}

func (r *run) binary(v any) (any, bool) {
	if _, done := r.empty(v, nil); done {
		return nil, false
	}
	s, ok := v.(string)
	if !ok {
		r.fail("type", nil)
		return nil, false
	}
	if r.flag("ignore_whitespace") {
		s = strings.Map(func(c rune) rune {
			if isSpace(c) {
				return -1
			}
			return c
		}, s)
	}
	encoding := r.str("encoding")
	var size int
	if encoding == "hex" {
		if len(s)%2 != 0 || strings.ContainsFunc(s, func(c rune) bool { return !strings.ContainsRune("0123456789abcdefABCDEF", c) }) {
			r.fail("encoding", map[string]any{"encoding": encoding})
			return nil, false
		}
		if want := r.str("hex_case"); (want == "lower" && strings.ContainsAny(s, "ABCDEF")) || (want == "upper" && strings.ContainsAny(s, "abcdef")) {
			r.fail("case", map[string]any{"hex_case": want})
		}
		size = len(s) / 2
	} else {
		alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
		if encoding == "base64url" {
			alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
		}
		body := strings.TrimRight(s, "=")
		pad := len(s) - len(body)
		rest := len(body) % 4
		if pad > 2 || rest == 1 || (pad > 0 && (len(body)+pad)%4 != 0) ||
			strings.ContainsFunc(body, func(c rune) bool { return !strings.ContainsRune(alphabet, c) }) {
			r.fail("encoding", map[string]any{"encoding": encoding})
			return nil, false
		}
		if want := r.str("padding"); (want == "required" && rest != 0 && pad == 0) || (want == "forbidden" && pad > 0) {
			r.fail("padding", map[string]any{"padding": want})
		}
		size = len(body) * 3 / 4
	}
	if min, ok := r.count("min_bytes"); ok && size < min {
		r.fail("min_bytes", map[string]any{"min_bytes": r.c["min_bytes"], "actual": size})
	}
	if max, ok := r.count("max_bytes"); ok && size > max {
		r.fail("max_bytes", map[string]any{"max_bytes": r.c["max_bytes"], "actual": size})
	}
	return s, true
}

func (r *run) bigint(v any) (any, bool) {
	if _, done := r.empty(v, nil); done {
		return nil, false
	}
	var d dec
	leading := false
	if s, ok := v.(string); ok {
		t := trimSpace(s, "both")
		if len(t) > maxNumberText || !integerSyntax.MatchString(t) {
			r.fail("type", nil)
			return nil, false
		}
		digits := strings.TrimLeft(t, "+-")
		leading = len(digits) > 1 && digits[0] == '0'
		d, _ = parseDec(t)
	} else {
		if _, isNumber := jsonNumber(v); !isNumber || !r.flag("accept_numbers") {
			r.fail("type", nil)
			return nil, false
		}
		if d, ok = r.wholeNumber(v, "unsafe_number"); !ok {
			return nil, false
		}
	}
	if leading && !r.flag("allow_leading_zeros") {
		r.fail("leading_zeros", nil)
	}
	n := d.digitCount()
	if min, ok := r.count("min_digits"); ok && n < min {
		r.fail("min_digits", map[string]any{"min_digits": r.c["min_digits"], "actual": n})
	}
	if max, ok := r.count("max_digits"); ok && n > max {
		r.fail("max_digits", map[string]any{"max_digits": r.c["max_digits"], "actual": n})
	}
	r.numberChecks(d, bigintRanges)
	return d.String(), true
}

// numberChecks applies the shared numeric constraints: the bounds, sign,
// multiple_of and, with ranges, representation.
func (r *run) numberChecks(d dec, ranges map[string][2]string) {
	for _, b := range []struct {
		name  string
		fails func(c int) bool
	}{
		{"minimum", func(c int) bool { return c < 0 }},
		{"maximum", func(c int) bool { return c > 0 }},
		{"exclusive_minimum", func(c int) bool { return c <= 0 }},
		{"exclusive_maximum", func(c int) bool { return c >= 0 }},
	} {
		if limit, ok := numberDec(r.c[b.name]); ok && b.fails(d.cmp(limit)) {
			r.fail(b.name, map[string]any{b.name: r.c[b.name]})
		}
	}
	s := d.sign()
	if want := r.str("sign"); (want == "positive" && s <= 0) || (want == "negative" && s >= 0) ||
		(want == "non_negative" && s < 0) || (want == "non_positive" && s > 0) || (want == "non_zero" && s == 0) {
		r.fail("sign", map[string]any{"sign": want})
	}
	if m, ok := numberDec(r.c["multiple_of"]); ok && m.sign() > 0 && !d.multipleOf(m) {
		r.fail("multiple_of", map[string]any{"multiple_of": r.c["multiple_of"]})
	}
	if bounds, ok := ranges[r.str("representation")]; ok {
		lo, _ := parseDec(bounds[0])
		hi, _ := parseDec(bounds[1])
		if d.cmp(lo) < 0 || d.cmp(hi) > 0 {
			r.fail("range", map[string]any{"representation": r.str("representation"), "lowest": bounds[0], "highest": bounds[1]})
		}
	}
}

// safeMin and safeMax bound the integers a JavaScript number holds exactly.
var (
	safeMin = dec{big.NewInt(-maxSafe), 0}
	safeMax = dec{big.NewInt(maxSafe), 0}
)

var integerRanges = map[string][2]string{
	"int8": {"-128", "127"}, "int16": {"-32768", "32767"}, "int32": {"-2147483648", "2147483647"},
	"uint8": {"0", "255"}, "uint16": {"0", "65535"}, "uint32": {"0", "4294967295"},
}

var bigintRanges = map[string][2]string{
	"int64":   {"-9223372036854775808", "9223372036854775807"},
	"uint64":  {"0", "18446744073709551615"},
	"int128":  {"-170141183460469231731687303715884105728", "170141183460469231731687303715884105727"},
	"uint128": {"0", "340282366920938463463374607431768211455"},
}

func asciiLower(s string) string {
	return strings.Map(func(c rune) rune {
		if c >= 'A' && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}, s)
}

// enumKey is a text that two JSON values share exactly when they are equal for
// enum: numbers by value, objects by content, strings length-prefixed. Without
// type sensitivity, numbers and booleans compare as their text; without case
// sensitivity, text compares lowercased.
func enumKey(v any, caseSensitive, typeSensitive bool) string {
	text := func(s string) string {
		if !caseSensitive {
			s = strings.ToLower(s)
		}
		return "s" + strconv.Itoa(utf8.RuneCountInString(s)) + ":" + s
	}
	switch x := v.(type) {
	case nil:
		return "z"
	case bool:
		if !typeSensitive {
			return text(map[bool]string{true: "true", false: "false"}[x])
		}
		return map[bool]string{true: "b1", false: "b0"}[x]
	case string:
		return text(x)
	case []any:
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = enumKey(item, caseSensitive, typeSensitive)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case map[string]any:
		keys := slices.Sorted(maps.Keys(x))
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = "s" + strconv.Itoa(utf8.RuneCountInString(k)) + ":" + k + "=" + enumKey(x[k], caseSensitive, typeSensitive)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	if f, ok := jsonNumber(v); ok {
		if !typeSensitive {
			return text(fromFloat(f).String())
		}
		return "n" + fromFloat(f).String()
	}
	return "?"
}
