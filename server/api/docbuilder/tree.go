package docbuilder

import (
	"sort"
	"strconv"
	"strings"
)

// BuildTree groups entries by category, then lists them by title. Categories are alphabetical
// with Uncategorized last.
func BuildTree(entries []Entry) []TreeNode {
	groups := map[string][]Entry{}
	for _, e := range entries {
		c := strings.TrimSpace(e.Category)
		if c == "" {
			c = Uncategorized
		}
		groups[c] = append(groups[c], e)
	}
	names := make([]string, 0, len(groups))
	for c := range groups {
		names = append(names, c)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == Uncategorized) != (names[j] == Uncategorized) {
			return names[j] == Uncategorized
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	tree := make([]TreeNode, 0, len(names))
	for _, c := range names {
		es := groups[c]
		sort.SliceStable(es, func(i, j int) bool { return strings.ToLower(es[i].Title) < strings.ToLower(es[j].Title) })
		node := TreeNode{ID: "cat:" + c, Label: c}
		for _, e := range es {
			node.Children = append(node.Children, TreeNode{ID: "doc:" + strconv.FormatInt(e.DocID, 10), Label: e.Title, Kind: e.Kind, DocID: e.DocID})
		}
		tree = append(tree, node)
	}
	return tree
}
