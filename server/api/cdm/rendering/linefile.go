package rendering

import (
	"fmt"
	"strconv"
	"strings"
)

// GenerateLineFile emits a line file in the same tab-separated dialect
// server/api/file-converters/opendata.go writes for uploaded documents
// (line_number, page, type, font, font_size, bbox, content), so existing
// extractors consume CDM documents with no changes (spec §10.1). One line is
// emitted per unit, in order; lineUnitIDs[i] is the unit ID for line i+1,
// which the caller uses to key kb.cdm_anchors.line_number.
//
// A unit's page/bbox come from its first fragment (fragment_ordinal 0). A
// unit that spans pages has further fragments recorded only in the anchor
// map (kb.cdm_anchors) -- the single-page/bbox line-file format has no way
// to represent more than one box per line, matching real MinerU lines, which
// never span pages either.
func GenerateLineFile(units []Unit, fragsByUnit map[string][]Fragment) (content string, lineUnitIDs []string, err error) {
	const defaultFont = "unknown-font"
	const defaultFontSize = "12"

	var out strings.Builder
	lineUnitIDs = make([]string, 0, len(units))

	for i, u := range units {
		frags := fragsByUnit[u.ID]
		if len(frags) == 0 {
			return "", nil, fmt.Errorf("cdm: unit %q has no derived fragment", u.ID)
		}
		f := frags[0]

		lineUnitIDs = append(lineUnitIDs, u.ID)
		fmt.Fprintf(&out, "%d\t%d\t%s\t%s\t%s\t%s\t%s\n",
			i+1, f.Page, u.Type, defaultFont, defaultFontSize,
			bboxLiteral(f), escapeLineContent(u.Text))
	}
	return out.String(), lineUnitIDs, nil
}

func bboxLiteral(f Fragment) string {
	return fmt.Sprintf("[%s, %s, %s, %s]",
		formatCoord(f.X), formatCoord(f.Y), formatCoord(f.X+f.W), formatCoord(f.Y+f.H))
}

func formatCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// escapeLineContent mirrors
// server/api/file-converters/opendata.go:escapeLineContent, so downstream
// consumers of either dialect see the same escaping.
func escapeLineContent(content string) string {
	c := strings.ReplaceAll(content, "\r\n", "\\n")
	c = strings.ReplaceAll(c, "\n", "\\n")
	c = strings.ReplaceAll(c, "\r", "\\n")
	c = strings.ReplaceAll(c, "\t", "\\t")
	return c
}
