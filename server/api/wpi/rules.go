package wpi

import (
	"encoding/json"
	"net/mail"
	"regexp"
	"strconv"
	"unicode/utf8"
)

// Error codes returned in field_errors; the browser translates them.
const (
	codeRequired  = "required"
	codeMaxLength = "max_length"
	codePattern   = "pattern"
	codeFormat    = "format"
)

// checkFields applies each field's required flag and rules to the input doc
// and returns the failing attributes with their error codes.
func checkFields(fields []FieldMap, doc map[string]any) map[string][]string {
	errs := map[string][]string{}
	for _, f := range fields {
		v := doc[f.Attribute]
		if isEmpty(v) {
			if f.Required {
				errs[f.Attribute] = append(errs[f.Attribute], codeRequired)
			}
			continue
		}
		if f.Rules == nil {
			continue
		}
		s, isText := textOf(v)
		if !isText {
			continue // length, pattern and format apply to text and numbers only
		}
		if f.Rules.MaxLength != nil && utf8.RuneCountInString(s) > *f.Rules.MaxLength {
			errs[f.Attribute] = append(errs[f.Attribute], codeMaxLength)
		}
		if f.Rules.Pattern != "" {
			re, err := regexp.Compile(f.Rules.Pattern)
			if err != nil || !re.MatchString(s) {
				errs[f.Attribute] = append(errs[f.Attribute], codePattern)
			}
		}
		if f.Rules.Format == "email" {
			if addr, err := mail.ParseAddress(s); err != nil || addr.Address != s {
				errs[f.Attribute] = append(errs[f.Attribute], codeFormat)
			}
		}
	}
	return errs
}

func textOf(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	}
	return "", false
}
