package docbuilder

import (
	"regexp"
	"strconv"
	"strings"
)

// versionRe finds a "-v1.2" style version in a path; group 1 is what precedes it and group 3 what
// follows (for example "-en.md"), so the two languages of one document are kept apart.
var versionRe = regexp.MustCompile(`^(.*?)[-_]v(\d+(?:\.\d+)*)((?:[-_.].*)?)$`)

// LatestVersions keeps, for every document that exists in several versions
// (name-v1.0-en.md, name-v1.1-en.md, ...), only the highest version. Versions are compared
// number by number, so 1.10 is newer than 1.9. Documents without a version are kept as they are,
// and so is each language of a document (the part after the version is part of the identity).
func LatestVersions(entries []Entry) []Entry {
	best := map[string]int{} // identity → index of the newest entry so far
	keys := make([]string, len(entries))
	for i, e := range entries {
		m := versionRe.FindStringSubmatch(e.Path)
		if m == nil {
			continue
		}
		keys[i] = m[1] + "\x00" + m[3]
		if j, ok := best[keys[i]]; !ok || newer(m[2], versionOf(entries[j].Path)) {
			best[keys[i]] = i
		}
	}
	var out []Entry
	for i, e := range entries {
		if keys[i] == "" || best[keys[i]] == i {
			out = append(out, e)
		}
	}
	return out
}

func versionOf(path string) string { return versionRe.FindStringSubmatch(path)[2] }

// newer reports whether version a is greater than b.
func newer(a, b string) bool {
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
