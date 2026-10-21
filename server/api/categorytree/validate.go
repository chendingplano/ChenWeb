package categorytree

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// canonicalNamePattern: a language-neutral ASCII slug (devdoc §3.1).
var canonicalNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

// ValidationError lists every problem found in a tree.
type ValidationError struct {
	Problems []string `json:"problems"`
}

func (e *ValidationError) Error() string {
	return "invalid category tree: " + strings.Join(e.Problems, "; ")
}

func labelKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Validate runs the devdoc §3.3 checks. It returns nil or a *ValidationError.
func Validate(t *Tree) error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	roots := t.Roots()
	switch {
	case len(roots) == 0 && len(t.Nodes) > 0:
		add("tree has no root")
	case len(roots) > 1:
		names := make([]string, len(roots))
		for i, r := range roots {
			names[i] = r.CanonicalName
		}
		add("tree has %d roots (%s)", len(roots), strings.Join(names, ", "))
	case len(roots) == 1:
		if roots[0].IsOthers {
			add("root %s cannot be an \"others\" node", roots[0].CanonicalName)
		}
		if len(t.OrdinaryChildren(roots[0].ID)) == 0 {
			add("root %s must be an inner node (it has no ordinary children)", roots[0].CanonicalName)
		}
	}

	ids := make([]string, 0, len(t.Nodes))
	for id := range t.Nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return t.Nodes[ids[i]].CanonicalName < t.Nodes[ids[j]].CanonicalName })

	names := map[string]string{}            // canonical name -> node ID
	prefs := map[string]map[string]string{} // lang -> label key -> node name
	alts := map[string]map[string]string{}  // lang -> label key -> node name
	sameTreeIRIs := map[string]bool{}
	for _, id := range ids {
		sameTreeIRIs[t.IRI(id)] = true
	}

	for _, id := range ids {
		n := t.Nodes[id]
		name := n.CanonicalName
		if !canonicalNamePattern.MatchString(name) {
			add("node %s: canonical name %q must be a lower-case ASCII slug", id, name)
		}
		if other, dup := names[name]; dup {
			add("canonical name %s is used by nodes %s and %s", name, other, id)
		}
		names[name] = id

		for _, lang := range requiredLangs {
			if strings.TrimSpace(n.PrefLabels[lang]) == "" {
				add("node %s: missing preferred label in %s", name, lang)
			}
			if strings.TrimSpace(n.Definitions[lang]) == "" {
				add("node %s: missing definition in %s", name, lang)
			}
		}
		if !n.IsOthers {
			for lang, label := range n.PrefLabels {
				k := labelKey(label)
				if k == "" {
					continue
				}
				if prefs[lang] == nil {
					prefs[lang] = map[string]string{}
				}
				if other, dup := prefs[lang][k]; dup {
					add("preferred label %q (%s) is used by %s and %s", label, lang, other, name)
				}
				prefs[lang][k] = name
			}
		}
		for lang, labels := range n.AltLabels {
			if alts[lang] == nil {
				alts[lang] = map[string]string{}
			}
			own := map[string]bool{}
			for _, label := range labels {
				k := labelKey(label)
				if k == "" || own[k] {
					continue
				}
				own[k] = true
				if k == labelKey(n.PrefLabels[lang]) {
					add("node %s: %q (%s) is both its preferred and an alternative label", name, label, lang)
				}
				if other, dup := alts[lang][k]; dup {
					add("alternative label %q (%s) is used by %s and %s", label, lang, other, name)
				}
				alts[lang][k] = name
			}
		}

		if n.ParentID != "" {
			p := t.Nodes[n.ParentID]
			switch {
			case p == nil:
				add("node %s: parent %s does not exist", name, n.ParentID)
			case p.IsOthers:
				add("node %s: parent %s is an \"others\" leaf", name, p.CanonicalName)
			}
		}
		if path := t.Path(id); len(path) == 0 || path[0].ParentID != "" {
			if t.Nodes[n.ParentID] != nil {
				add("node %s is in a cycle or does not reach the root", name)
			}
		}

		ordinary := t.OrdinaryChildren(id)
		others := 0
		for _, c := range t.Children(id) {
			if c.IsOthers {
				others++
			}
		}
		switch {
		case len(ordinary) > 0 && others != 1:
			add("inner node %s must have exactly one \"others\" child, has %d", name, others)
		case len(ordinary) == 0 && others > 0:
			add("leaf %s must not have an \"others\" child", name)
		}

		for _, m := range n.Matches {
			switch m.Type {
			case "exact":
				if sameTreeIRIs[m.IRI] {
					add("node %s: exactMatch to %s, a node of the same tree (a duplicate)", name, m.IRI)
				}
			case "close":
			default:
				add("node %s: match type %q must be exact or close", name, m.Type)
			}
		}
	}

	// An alternative label must not be another node's preferred label: a label identifies one node.
	for lang, byLabel := range alts {
		for k, name := range byLabel {
			if owner, ok := prefs[lang][k]; ok && owner != name {
				add("alternative label %q (%s) of %s is the preferred label of %s", k, lang, name, owner)
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return &ValidationError{Problems: problems}
}
