package rendering

import "strings"

// typstEscaper neutralizes every Typst markup metacharacter so author text
// can never be interpreted as markup or a directive (spec §5.2, "Typst
// escaping"). The backslash replacement MUST run first, or escaping any
// later character would itself be escaped a second time.
var typstEscaper = strings.NewReplacer(
	`\`, `\\`,
	"#", `\#`,
	"$", `\$`,
	"*", `\*`,
	"_", `\_`,
	"`", "\\`",
	"@", `\@`,
	"<", `\<`,
	">", `\>`,
	"[", `\[`,
	"]", `\]`,
)

// escapeTypst escapes s for inclusion as literal Typst content text. It must
// not be applied to verbatim content such as a code block's Text, which is
// passed through inside a #raw call instead.
func escapeTypst(s string) string {
	return typstEscaper.Replace(s)
}
