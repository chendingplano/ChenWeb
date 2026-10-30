package wpi

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Shared with the text editor's value validation; no remote icon/font loading.
//
//go:embed text-values.json
var textValuesJSON []byte
var textValues struct {
	Icons  []string `json:"icons"`
	Colors []string `json:"colors"`
}

func init() {
	if err := json.Unmarshal(textValuesJSON, &textValues); err != nil {
		panic(err)
	}
}

var textAttributes = []string{"text", "data_binding", "label", "help_tip", "icon", "label_placement", "label_horizontal_gap", "label_vertical_gap", "label_horizontal_align", "label_vertical_align", "label_max_width", "label_font_weight", "icon_placement", "icon_size", "icon_gap", "icon_color", "font_family", "font_size", "font_weight", "font_style", "line_height", "color", "background_color", "text_align", "white_space", "padding"}
var textPixel = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?px$`)
var textFontSize = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(px|rem|em)$`)
var textPadding = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(px|rem)$`)
var textHex = regexp.MustCompile(`^#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
var textRGB = regexp.MustCompile(`^(rgb|rgba)\(\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*(,\s*([0-9]+(\.[0-9]+)?)\s*)?\)$`)
var textFamily = regexp.MustCompile(`^("[^"\\\r\n]+"|'[^'\\\r\n]+'|[\p{L}\p{N}_ -]+)(\s*,\s*("[^"\\\r\n]+"|'[^'\\\r\n]+'|[\p{L}\p{N}_ -]+))*$`)

func validTextColor(s string, inherit bool) bool {
	if inherit && s == "inherit" {
		return true
	}
	if slices.Contains(textValues.Colors, strings.ToLower(s)) || textHex.MatchString(s) {
		return true
	}
	m := textRGB.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	if (m[1] == "rgba") != (m[6] != "") {
		return false
	}
	for _, ch := range m[2:5] {
		n, err := strconv.Atoi(ch)
		if err != nil || n > 255 {
			return false
		}
	}
	if m[6] != "" {
		a, err := strconv.ParseFloat(m[6], 64)
		if err != nil || a < 0 || a > 1 {
			return false
		}
	}
	return true
}
func validTextFamily(s string) bool {
	if s == "inherit" {
		return true
	}
	if strings.TrimSpace(s) == "" || strings.ContainsAny(s, ";(){}<>\\\r\n") || !textFamily.MatchString(s) {
		return false
	}
	for _, part := range strings.Split(s, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "inherit", "initial", "unset", "revert", "revert-layer":
			return false
		}
	}
	return true
}
func (v *validator) textAttributes(e map[string]any, path string) {
	// A literal empty string is allowed only for text and runtime help.
	for _, k := range []string{"text", "help_tip"} {
		if x, ok := e[k]; ok {
			if s, ok := x.(string); ok && s == "" {
				continue
			}
			v.text(e, k, path)
		}
	}
	v.text(e, "label", path)
	v.binding(e, path)
	if x, present := e["icon"]; present {
		s, ok := x.(string)
		if !ok || !slices.Contains(textValues.Icons, s) {
			v.errf(path+"/icon", "must be an allowed local icon identifier or empty string")
		}
	}
	for key, choices := range map[string][]string{
		"label_placement":        {"top", "left", "right", "bottom"},
		"label_horizontal_align": {"start", "center", "end"}, "label_vertical_align": {"top", "center", "bottom"},
		"icon_placement": {"start", "end"}, "font_style": {"normal", "italic", "oblique"},
		"text_align": {"start", "center", "end", "justify"}, "white_space": {"normal", "pre-wrap"},
	} {
		v.oneOf(e, key, path, choices)
	}
	for _, k := range []string{"label_horizontal_gap", "label_vertical_gap", "label_max_width", "icon_size", "icon_gap", "font_size"} {
		if x, ok := e[k]; ok {
			s, ok := x.(string)
			pattern := textPixel
			if k == "font_size" {
				pattern = textFontSize
			}
			valid := ok && pattern.MatchString(s)
			if valid && (k == "label_max_width" || k == "icon_size" || k == "font_size") {
				n, _ := strconv.ParseFloat(regexp.MustCompile(`[a-z]+$`).ReplaceAllString(s, ""), 64)
				valid = n > 0
			}
			if !valid {
				v.errf(path+"/"+k, "must be a valid %s length (positive for font/icon size and caption maximum)", map[bool]string{true: "px/rem/em", false: "pixel"}[k == "font_size"])
			}
		}
	}
	for _, k := range []string{"font_weight", "label_font_weight"} {
		if x, ok := e[k]; ok {
			s, isString := x.(string)
			valid := isString && slices.Contains([]string{"normal", "bold", "inherit"}, s)
			if n, ok := asInt(x); ok && n >= 100 && n <= 900 && n%100 == 0 {
				valid = true
			}
			if !valid {
				v.errf(path+"/"+k, "must be inherit, normal, bold or an integer weight from 100 to 900 in steps of 100")
			}
		}
	}
	if x, ok := e["line_height"]; ok {
		n, valid := number(x)
		if s, ok := x.(string); !(ok && s == "normal") && (!valid || n <= 0) {
			v.errf(path+"/line_height", "must be normal or a positive number")
		}
	}
	for _, k := range []string{"color", "background_color", "icon_color"} {
		if x, ok := e[k]; ok {
			s, ok := x.(string)
			if !ok || !validTextColor(s, k != "background_color") {
				v.errf(path+"/"+k, "must be a supported text color")
			}
		}
	}
	if x, ok := e["font_family"]; ok {
		s, ok := x.(string)
		if !ok || !validTextFamily(s) {
			v.errf(path+"/font_family", "must be inherit or a font family list")
		}
	}
	if x, ok := e["padding"]; ok {
		s, ok := x.(string)
		parts := strings.Fields(s)
		valid := ok && len(parts) >= 1 && len(parts) <= 4
		for _, p := range parts {
			if !textPadding.MatchString(p) {
				valid = false
			}
		}
		if !valid {
			v.errf(path+"/padding", "must be one to four nonnegative px/rem lengths")
		}
	}
}
