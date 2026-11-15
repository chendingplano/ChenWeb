package documentgeometry

import (
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"testing"
)

func TestNoteMarkersRejectIncompleteOrMisalignedSequences(t *testing.T) {
	notes := []docprocessing.TableNote{{Label: "a", Text: "试验用水条件。"}, {Label: "b", Text: "运行期间融霜。"}}
	base := PhysicalCell{Text: "garbled", Coords: []float64{100, 100, 900, 300}, Lines: []PhysicalTextLine{
		{Text: "0 unreadable", Marker: "0", Coords: []float64{110, 120, 800, 135}},
		{Text: "1 unreadable", Marker: "1", Coords: []float64{110, 150, 800, 165}},
		{Text: "wrapped", Coords: []float64{120, 170, 600, 185}},
	}}
	for _, kind := range []string{"missing", "encoding", "alignment", "bounds", "overlap"} {
		t.Run(kind, func(t *testing.T) {
			cell := base
			cell.Lines = append([]PhysicalTextLine{}, base.Lines...)
			switch kind {
			case "missing":
				cell.Lines[0].Marker = ""
			case "encoding":
				cell.Lines[1].Marker = "3"
			case "alignment":
				cell.Lines[1].Coords = []float64{150, 150, 800, 165}
			case "bounds":
				cell.Lines[2].Coords = []float64{120, 170, 950, 185}
			case "overlap":
				cell.Lines[2].Coords = []float64{120, 160, 600, 185}
			}
			if got := noteMarkerBoxes(notes[1], notes, cell, 1, 0); len(got) != 0 {
				t.Fatalf("unsafe markers accepted: %+v", got)
			}
		})
	}
	for _, rotation := range []int{0, 90, 180, 270} {
		rotate := func(c []float64) []float64 {
			switch rotation {
			case 90:
				return []float64{1000 - c[3], c[0], 1000 - c[1], c[2]}
			case 180:
				return []float64{1000 - c[2], 1000 - c[3], 1000 - c[0], 1000 - c[1]}
			case 270:
				return []float64{c[1], 1000 - c[2], c[3], 1000 - c[0]}
			default:
				return c
			}
		}
		cell := base
		cell.Coords = rotate(base.Coords)
		cell.Lines = append([]PhysicalTextLine{}, base.Lines...)
		for i := range cell.Lines {
			cell.Lines[i].Coords = rotate(cell.Lines[i].Coords)
		}
		if got := noteMarkerBoxes(notes[1], notes, cell, 1, rotation); len(got) != 2 || got[0].Rotation != rotation {
			t.Fatalf("rotation %d: %+v", rotation, got)
		}
	}
}
