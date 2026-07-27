package rendering

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// RenderSVGPages compiles the .typ file at typstPath into one SVG per page
// (spec §5.7: paginated SVG is the viewing target, not Typst's HTML export --
// verified that HTML export discards pagination). Pages are returned in
// order, page 1 first.
func RenderSVGPages(typstBin, typstPath string) ([][]byte, error) {
	return RenderSVGPagesContext(context.Background(), typstBin, typstPath)
}

// RenderSVGPagesContext is RenderSVGPages with cancellation support for live
// previews whose compilation becomes obsolete when the author keeps typing.
func RenderSVGPagesContext(ctx context.Context, typstBin, typstPath string) ([][]byte, error) {
	if typstBin == "" {
		typstBin = "typst"
	}

	dir, err := os.MkdirTemp("", "cdm-svg-*")
	if err != nil {
		return nil, fmt.Errorf("cdm: create temp dir for svg output: %w", err)
	}
	defer os.RemoveAll(dir)

	outPattern := filepath.Join(dir, "page-{p}.svg")
	cmd := exec.CommandContext(ctx, typstBin, "compile", "--format", "svg", typstPath, outPattern)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cdm: typst compile svg failed: %w: %s", err, stderr.String())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cdm: read svg output dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if err := checkPageOrdering(names); err != nil {
		return nil, err
	}

	pages := make([][]byte, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("cdm: read %s: %w", name, err)
		}
		pages = append(pages, data)
	}
	return pages, nil
}

// checkPageOrdering guards against the lexicographic-sort pitfall for
// double-digit-plus page counts ("page-10.svg" would otherwise sort before
// "page-2.svg"). Typst's {p} template has no zero-padding option as of
// 0.14.2, so page numbers are parsed and sorted numerically instead of
// trusting directory-listing order for documents over 9 pages.
func checkPageOrdering(names []string) error {
	type numbered struct {
		n    int
		name string
	}
	var parsed []numbered
	for _, name := range names {
		var n int
		if _, err := fmt.Sscanf(name, "page-%d.svg", &n); err != nil {
			return fmt.Errorf("cdm: unexpected svg output filename %q", name)
		}
		parsed = append(parsed, numbered{n: n, name: name})
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].n < parsed[j].n })
	for i, p := range parsed {
		if p.n != i+1 {
			return fmt.Errorf("cdm: svg pages are not contiguous starting at 1: got page number %d at position %d", p.n, i)
		}
	}
	for i := range parsed {
		names[i] = parsed[i].name
	}
	return nil
}
