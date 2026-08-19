package keywords

import "unicode"

func autoPromotedLabelLanguage(label string) string {
	hasLatin := false
	for _, r := range label {
		if unicode.Is(unicode.Han, r) {
			return "zh"
		}
		if unicode.Is(unicode.Latin, r) {
			hasLatin = true
		}
	}
	if hasLatin {
		return "en"
	}
	return "und"
}
