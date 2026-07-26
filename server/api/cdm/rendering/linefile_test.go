package rendering_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/rendering"
)

func TestGenerateLineFile_OneLinePerUnit(t *testing.T) {
	typstBin := requireTypst(t)
	doc := cdmfixtures.JaroWinkler()
	typPath := renderToTempDir(t, &doc)

	units := rendering.CollectUnits(&doc)
	marks, err := rendering.ExtractAnchors(typstBin, typPath)
	if err != nil {
		t.Fatalf("extract anchors: %v", err)
	}
	frags, err := rendering.DeriveFragments(marks)
	if err != nil {
		t.Fatalf("derive fragments: %v", err)
	}
	fragsByUnit := map[string][]rendering.Fragment{}
	for _, f := range frags {
		fragsByUnit[f.UnitID] = append(fragsByUnit[f.UnitID], f)
	}

	content, lineUnitIDs, err := rendering.GenerateLineFile(units, fragsByUnit)
	if err != nil {
		t.Fatalf("generate line file: %v", err)
	}

	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) != len(units) {
		t.Fatalf("expected %d lines, got %d", len(units), len(lines))
	}
	if len(lineUnitIDs) != len(units) {
		t.Fatalf("expected %d line-to-unit mappings, got %d", len(units), len(lineUnitIDs))
	}

	for i, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			t.Fatalf("line %d: expected 7 tab-separated fields, got %d: %q", i+1, len(fields), line)
		}
		if fields[0] != strconv.Itoa(i+1) {
			t.Errorf("line %d: expected line number %d in field 1, got %q", i+1, i+1, fields[0])
		}
	}

	// Spot-check the table row content is verbalized, not left as raw cell
	// markup.
	found := false
	for i, id := range lineUnitIDs {
		if id == "example-table/row0" {
			found = true
			if !strings.Contains(lines[i], "John") || !strings.Contains(lines[i], "0.93") {
				t.Errorf("expected row content in line %d, got %q", i+1, lines[i])
			}
		}
	}
	if !found {
		t.Fatal("expected a line for example-table/row0")
	}
}

func TestGenerateLineFile_EscapesTabsAndNewlines(t *testing.T) {
	units := []rendering.Unit{{ID: "u1", Type: "paragraph", Text: "line1\nline2\twith tab"}}
	frags := map[string][]rendering.Fragment{
		"u1": {{UnitID: "u1", Page: 1, X: 54, Y: 54, W: 100, H: 20}},
	}
	content, _, err := rendering.GenerateLineFile(units, frags)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// Only the trailing newline after this single generated line should
	// remain; the content's own \n and \t must be escaped, not literal.
	if strings.Count(content, "\n") != 1 {
		t.Fatalf("expected exactly one literal newline (the line terminator), got %d in: %q",
			strings.Count(content, "\n"), content)
	}
	if !strings.Contains(content, `line1\nline2\twith tab`) {
		t.Fatalf("expected escaped content, got: %q", content)
	}
}

func TestGenerateLineFile_MissingFragmentErrors(t *testing.T) {
	units := []rendering.Unit{{ID: "u1", Type: "paragraph", Text: "x"}}
	_, _, err := rendering.GenerateLineFile(units, map[string][]rendering.Fragment{})
	if err == nil {
		t.Fatal("expected error for unit with no derived fragment")
	}
}
