package documentgeometry

import (
	"math"
	"strings"
	"unicode/utf8"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

// noteTextBoxes matches the complete note against the cell's actual text lines.
// Chinese text can corroborate notes in PDFs with broken digit/font encodings,
// just as row matching does. A line shared with another note is left unresolved.
func noteTextBoxes(note docprocessing.TableNote, cell PhysicalCell, page, rotation int) []Box {
	normalize := normalized
	if len(han(note.Text)) >= 4 {
		normalize = func(s string) string { return string(han(s)) }
	} else if utf8.RuneCountInString(normalize(note.Text)) < 8 {
		return nil
	}
	want := normalize(note.Text)
	var joined strings.Builder
	starts, ends := make([]int, len(cell.Lines)), make([]int, len(cell.Lines))
	for i, line := range cell.Lines {
		starts[i] = joined.Len()
		joined.WriteString(normalize(line.Text))
		ends[i] = joined.Len()
	}
	text := joined.String()
	start := strings.Index(text, want)
	if start < 0 || strings.Index(text[start+len(want):], want) >= 0 {
		return nil
	}
	end := start + len(want)
	var boxes []Box
	for i, line := range cell.Lines {
		if ends[i] <= start || starts[i] >= end {
			continue
		}
		// A superscript letter on the first line is allowed; neighboring prose is not.
		if starts[i] < start && text[starts[i]:start] != note.Label {
			return nil
		}
		if ends[i] > end || !validBox(line.Coords) {
			return nil
		}
		c := line.Coords
		if !validBox(cell.Coords) || c[0] < cell.Coords[0] || c[1] < cell.Coords[1] || c[2] > cell.Coords[2] || c[3] > cell.Coords[3] {
			return nil
		}
		boxes = append(boxes, Box{Page: page, Rotation: rotation, Coords: integerCoords(c)})
	}
	return boxes
}

// noteMarkerBoxes is restricted to structurally verified tables whose Chinese
// text layer is unreadable. Every canonical note must have one small PDF marker,
// in the same sequence, with the same encoding offset and aligned left edges.
func noteMarkerBoxes(note docprocessing.TableNote, notes []docprocessing.TableNote, cell PhysicalCell, page, rotation int) []Box {
	if len(notes) < 2 || len(han(note.Text)) < 4 || len(han(cell.Text)) >= 4 || !validBox(cell.Coords) {
		return nil
	}
	var markers []int
	for i, line := range cell.Lines {
		if line.Marker != "" {
			markers = append(markers, i)
		}
	}
	if len(markers) != len(notes) {
		return nil
	}
	offset, selected := 0, -1
	unrotated := func(c []float64) []float64 {
		switch rotation {
		case 90:
			return []float64{c[1], 1000 - c[2], c[3], 1000 - c[0]}
		case 180:
			return []float64{1000 - c[2], 1000 - c[3], 1000 - c[0], 1000 - c[1]}
		case 270:
			return []float64{1000 - c[3], c[0], 1000 - c[1], c[2]}
		default:
			return c
		}
	}
	for i, index := range markers {
		line := cell.Lines[index]
		if len(line.Marker) != 1 || len(notes[i].Label) != 1 || !validBox(line.Coords) {
			return nil
		}
		delta := int(line.Marker[0]) - int(notes[i].Label[0])
		if i == 0 {
			offset = delta
		} else if delta != offset || notes[i].Label[0] != notes[i-1].Label[0]+1 {
			return nil
		}
		if math.Abs(unrotated(line.Coords)[0]-unrotated(cell.Lines[markers[0]].Coords)[0]) > 3 {
			return nil
		}
		if i > 0 && unrotated(line.Coords)[1] <= unrotated(cell.Lines[markers[i-1]].Coords)[3] {
			return nil
		}
		if notes[i].Label == note.Label {
			selected = i
		}
	}
	if selected < 0 {
		return nil
	}
	end := len(cell.Lines)
	if selected+1 < len(markers) {
		end = markers[selected+1]
	}
	var boxes []Box
	var previous []float64
	markerLeft := unrotated(cell.Lines[markers[0]].Coords)[0]
	for i := markers[selected]; i < end; i++ {
		c := cell.Lines[i].Coords
		if !validBox(c) || c[0] < cell.Coords[0] || c[1] < cell.Coords[1] || c[2] > cell.Coords[2] || c[3] > cell.Coords[3] {
			return nil
		}
		readingBox := unrotated(c)
		// Some broken fonts emit an extra fragment in the left cell margin at
		// the same height as note a. It is outside the aligned note-marker column.
		if readingBox[2] <= markerLeft {
			continue
		}
		if previous != nil && readingBox[1] < previous[3] {
			return nil
		}
		boxes = append(boxes, Box{Page: page, Rotation: rotation, Coords: integerCoords(c)})
		previous = readingBox
	}
	return boxes
}
