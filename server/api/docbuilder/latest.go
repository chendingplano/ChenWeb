package docbuilder

import (
	"regexp"
	"strconv"
	"strings"
)

// versionRe finds a "-v1.2" style version in a path; group 1 is what precedes it and group 3 what
// follows (for example "-en.md"), so the two languages of one document are kept apart.
var versionRe = regexp.MustCompile(`^(.*?)[-_]v(\d+(?:\.\d+)*)((?:[-_.].*)?)$`)

// ParsePath splits a relative path into the logical key shared by all versions of one document and
// the version ("" when the name has none). name-v1.0-en.md and name-v1.1-en.md share a key;
// name-v1.0-zh.md has another.
func ParsePath(p string) (key, version string) {
	m := versionRe.FindStringSubmatch(p)
	if m == nil {
		return p, ""
	}
	return m[1] + "\x1f" + m[3], m[2]
}

// Newer reports whether version a is greater than b, comparing numeric components
// (1.10 > 1.9). An empty version is the lowest.
func Newer(a, b string) bool {
	if a == "" || b == "" {
		return a != "" && b == ""
	}
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}
