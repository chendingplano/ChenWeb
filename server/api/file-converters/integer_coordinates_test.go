package fileconverters

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFutureLineFilesUseIntegerCoordinates(t *testing.T) {
	raw := `[113.68008474576271, 697.3100000000001, 901.6398305084746, 730.49]`
	for name, bbox := range map[string]string{
		"mineru-list-item":   mineruBBoxStr(json.RawMessage(raw)),
		"opendata":           formatBBox([]any{113.68008474576271, 697.3100000000001, 901.6398305084746, 730.49}),
		"computed-table-row": formatComputedBBox([4]float64{113.68008474576271, 697.3100000000001, 901.6398305084746, 730.49}),
		"direct":             raw,
		"half":               `[0.5, 1.49, 999.5, 1000]`,
	} {
		t.Run(name, func(t *testing.T) {
			line := formatOpenDataLines([]extractedOpenDataLine{{BBox: bbox, Content: "Value 1.25"}})[0]
			fields := strings.Split(line, "\t")
			var coords []int
			if err := json.Unmarshal([]byte(fields[5]), &coords); err != nil {
				t.Fatalf("noninteger coordinates %s: %v", fields[5], err)
			}
			want := []int{114, 697, 902, 730}
			if name == "half" {
				want = []int{1, 1, 1000, 1000}
			}
			for i, v := range want {
				if coords[i] != v {
					t.Fatalf("coordinates=%v, want=%v", coords, want)
				}
			}
			if fields[6] != "Value 1.25" {
				t.Fatal("rounded document content")
			}
		})
	}
}
