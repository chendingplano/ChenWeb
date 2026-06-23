package docreviews

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestTypContentLineEscapesMarkupDelimiters guards the fix for the "unclosed
// delimiter" compile failure: finding text containing an unbalanced count of
// Typst emphasis/raw delimiters (`*`, `_`, `` ` ``) or `~` must be escaped so it
// renders as literal text instead of opening markup Typst never closes.
func TestTypContentLineEscapesMarkupDelimiters(t *testing.T) {
	// Mirrors the real failure: wildcard ranges like "0.*", "1.*" produced an
	// odd number of asterisks.
	in := `“0.*”, “1.*”, “3.*”, “*”`
	got := typContentLine(in)
	for _, ch := range []string{`*`, `_`, "`", `~`} {
		// Every occurrence of the delimiter must be backslash-escaped.
		if strings.Count(got, ch) != strings.Count(got, `\`+ch) {
			t.Fatalf("delimiter %q not fully escaped in %q", ch, got)
		}
	}
}

// TestTypstReportCompilesWithDelimiterHeavyContent compiles a content block
// built the same way report findings are (typLines) from the exact text that
// triggered the "unclosed delimiter" failure (request 21). Skipped when the
// `typst` binary is not available.
func TestTypstReportCompilesWithDelimiterHeavyContent(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst binary not available; skipping compile check")
	}

	// Representative finding text: wildcard ranges (odd `*` count) and an
	// HTML-ish table fragment (the two cases seen in the request-21 report).
	raw := strings.Join([]string{
		`161: $$\n“ 0 . * ” , “ 1 . * ” , “ 3 . * ” , “ * ” 。\n$$`,
		`1083: <table><tr><td>一级编码</td><td>liquid chromatograph, LC</td></tr></table>`,
	}, "\n")

	src := "#[\n" + typLines(raw) + "\n]\n"

	dir := t.TempDir()
	typPath := dir + "/check.typ"
	if err := os.WriteFile(typPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write typ: %v", err)
	}

	out, err := exec.Command("typst", "compile", "--root", "/", typPath, dir+"/check.pdf").CombinedOutput()
	if err != nil {
		t.Fatalf("typst compile failed: %v\n%s\n--- source ---\n%s", err, out, src)
	}
}
