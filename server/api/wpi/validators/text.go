package validators

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// isSpace is the one whitespace set both languages use (design D12): the
// Unicode White_Space characters and U+FEFF. strings.TrimSpace and JavaScript's
// trim differ on U+0085 and U+FEFF, so neither is used.
func isSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

func isLineBreak(r rune) bool { return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029 }

// trimSpace removes whitespace at the start, end or both ("none" keeps it).
func trimSpace(s, mode string) string {
	switch mode {
	case "both":
		return strings.TrimFunc(s, isSpace)
	case "start":
		return strings.TrimLeftFunc(s, isSpace)
	case "end":
		return strings.TrimRightFunc(s, isSpace)
	}
	return s
}

func normalize(s, form string) string {
	switch form {
	case "nfc":
		return norm.NFC.String(s)
	case "nfd":
		return norm.NFD.String(s)
	case "nfkc":
		return norm.NFKC.String(s)
	case "nfkd":
		return norm.NFKD.String(s)
	}
	return s
}

// textLength counts code points (the default, §2.6.7), UTF-16 code units or
// UTF-8 bytes.
func textLength(s, unit string) int {
	switch unit {
	case "utf8_bytes":
		return len(s)
	case "utf16_units":
		n := 0
		for _, r := range s {
			n++
			if r >= 0x10000 {
				n++
			}
		}
		return n
	}
	return utf8.RuneCountInString(s)
}

// PatternLimit is the longest value, in code points, a pattern is matched
// against; a longer one fails, in the browser and here alike (§2.6.6).
const PatternLimit = 10000

// nonPortable finds syntax that RE2 and ECMAScript read differently or only one
// accepts: inline flags and named groups, \A \z \Q \E and \p \P. It is the rule
// the input element's validation.pattern already follows (validate.go).
var nonPortable = regexp.MustCompile(`\(\?[a-zA-Z<]|\\[AzQEpP]`)

var errNonPortable = errors.New("uses syntax that Go and JavaScript read differently: inline flags, named groups, \\A, \\z, \\Q, \\E, \\p or \\P")

// PortablePattern reports why a pattern cannot be used, or nil.
func PortablePattern(p string) error {
	if _, err := regexp.Compile(`\A(?:` + p + `)\z`); err != nil {
		return err
	}
	if nonPortable.MatchString(p) {
		return errNonPortable
	}
	return nil
}

var compiled sync.Map // pattern → *regexp.Regexp, or nil when it does not compile

// matchesWhole reports whether the whole of s matches p; a pattern that does
// not compile matches nothing.
func matchesWhole(p, s string) bool {
	re, ok := compiled.Load(p)
	if !ok {
		c, err := regexp.Compile(`\A(?:` + p + `)\z`)
		if err != nil {
			c = nil
		}
		re, _ = compiled.LoadOrStore(p, c)
	}
	c, _ := re.(*regexp.Regexp)
	return c != nil && c.MatchString(s)
}
