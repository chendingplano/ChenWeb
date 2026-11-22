package docreviews

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// complianceLooseText keeps only letters and digits, so a model's copy that changes spacing,
// punctuation, full-width forms or markdown escapes (90%\~105% quoted as 90%~105%) still matches.
func complianceLooseText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, norm.NFKC.String(s))
}

// complianceQuoteInLines checks that quote is text of lines start..end, which must be lines of the
// chunk. With loose, spacing and punctuation differences are accepted (letters and digits must
// match); without it the quote must be exact.
func complianceQuoteInLines(lines []complianceInputLine, start, end int, quote string, loose bool) error {
	quote = strings.TrimSpace(quote)
	if start <= 0 || end < start {
		return fmt.Errorf("has invalid lines %d-%d", start, end)
	}
	if quote == "" {
		return fmt.Errorf("is empty")
	}
	var parts []string
	hasStart, hasEnd := false, false
	for _, l := range lines {
		if l.LineNumber == start {
			hasStart = true
		}
		if l.LineNumber == end {
			hasEnd = true
		}
		if l.LineNumber >= start && l.LineNumber <= end {
			parts = append(parts, l.Content)
		}
	}
	if !hasStart || !hasEnd {
		return fmt.Errorf("cites lines %d-%d, which are not lines of this chunk", start, end)
	}
	text := strings.Join(parts, "\n")
	if strings.Contains(text, quote) {
		return nil
	}
	if loose {
		if q := complianceLooseText(quote); q != "" && strings.Contains(complianceLooseText(text), q) {
			return nil
		}
	}
	return fmt.Errorf("does not match lines %d-%d", start, end)
}
