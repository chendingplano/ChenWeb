package wpi

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Reference roots. A reference is a whole string value such as
// "$input_doc.address.city"; there is no interpolation inside strings.
const (
	RootInputDoc = "input_doc"
	RootRecord   = "record"
	RootArgs     = "args"
)

var segmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// forbiddenSegments would let a path reach JavaScript object internals.
var forbiddenSegments = map[string]bool{"__proto__": true, "constructor": true, "prototype": true}

// Ref is a parsed reference.
type Ref struct {
	Root string
	Path []string
}

// parseRef reports whether s is a reference and, if so, parses it.
func parseRef(s string) (Ref, bool, error) {
	if !strings.HasPrefix(s, "$") {
		return Ref{}, false, nil
	}
	parts := strings.Split(s[1:], ".")
	root := parts[0]
	if root != RootInputDoc && root != RootRecord && root != RootArgs {
		return Ref{}, true, fmt.Errorf("unknown reference root in %q", s)
	}
	if len(parts) < 2 {
		return Ref{}, true, fmt.Errorf("reference %q needs a path after $%s", s, root)
	}
	if err := checkPath(parts[1:]); err != nil {
		return Ref{}, true, fmt.Errorf("reference %q: %w", s, err)
	}
	return Ref{Root: root, Path: parts[1:]}, true, nil
}

func checkPath(segments []string) error {
	for _, seg := range segments {
		if forbiddenSegments[seg] {
			return fmt.Errorf("path segment %q is not allowed", seg)
		}
		if !segmentPattern.MatchString(seg) {
			return fmt.Errorf("invalid path segment %q", seg)
		}
	}
	return nil
}

// checkValue validates a literal-or-reference value whose references may only
// use the given roots.
func checkValue(v any, roots ...string) error {
	s, ok := v.(string)
	if !ok {
		switch v.(type) {
		case nil, bool, json.Number, float64:
			return nil
		}
		return fmt.Errorf("value must be a string, number, boolean or null")
	}
	ref, isRef, err := parseRef(s)
	if err != nil || !isRef {
		return err
	}
	for _, r := range roots {
		if ref.Root == r {
			return nil
		}
	}
	return fmt.Errorf("$%s is not allowed here", ref.Root)
}

// substituteArgs replaces a "$args.<path>" value with the matching argument.
// Other values are returned unchanged.
func substituteArgs(v any, args map[string]any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return v, nil
	}
	ref, isRef, err := parseRef(s)
	if err != nil {
		return nil, err
	}
	if !isRef {
		return v, nil
	}
	if ref.Root != RootArgs {
		return nil, fmt.Errorf("$%s cannot be resolved on the server", ref.Root)
	}
	var cur any = args
	for _, seg := range ref.Path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("missing argument %q", strings.Join(ref.Path, "."))
		}
		if cur, ok = m[seg]; !ok {
			return nil, fmt.Errorf("missing argument %q", strings.Join(ref.Path, "."))
		}
	}
	return cur, nil
}
