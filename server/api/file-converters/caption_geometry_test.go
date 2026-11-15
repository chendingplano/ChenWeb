package fileconverters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMineruCaptionGeometry(t *testing.T) {
	for _, tc := range []struct{ name, boxes, want string }{
		{"caption", `[[280,650,620,670]]`, "[280, 650, 620, 670]"},
		{"legacy", `[]`, "[103, 678, 881, 884]"},
		{"invalid", `[null]`, "[103, 678, 881, 884]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc_mineru.json")
			raw := `{"pages":[{"page_number":8,"items":[{"type":"table","bbox":[103,678,881,884],"table_caption":["Table 2"],"table_caption_bboxes":` + tc.boxes + `,"img_path":"table.jpg","table_body":"<table></table>"}]}]}`
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			out, err := ConvertMineruFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) != 3 {
				t.Fatalf("unexpected lines: %s", data)
			}
			if !strings.Contains(lines[0], tc.want) {
				t.Fatalf("caption geometry: %s", lines[0])
			}
			for _, line := range lines[1:] {
				if !strings.Contains(line, "[103, 678, 881, 884]") {
					t.Fatalf("table geometry changed: %s", line)
				}
			}
		})
	}
}

func TestMineruImageAnnotationGeometry(t *testing.T) {
	for _, tc := range []struct{ name, boxes, want string }{
		{"own boxes", `[[610,380,720,400]]`, "[610, 380, 720, 400]"},
		{"legacy", `[]`, "[499, 138, 836, 340]"},
		{"invalid", `[null]`, "[499, 138, 836, 340]"},
		{"misaligned", `[[1,2,3,4],[5,6,7,8]]`, "[499, 138, 836, 340]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var doc mineruDocument
			raw := `{"pages":[{"page_number":22,"items":[{"type":"image","bbox":[499,138,836,340],"image_caption":["Picture"],"image_caption_bboxes":` + tc.boxes + `,"image_footnote":["b) 不带水泵"],"image_footnote_bboxes":` + tc.boxes + `,"img_path":"picture.jpg"}]}]}`
			if err := json.Unmarshal([]byte(raw), &doc); err != nil {
				t.Fatal(err)
			}
			lines := extractMineruLineItems(doc.Pages)
			if len(lines) != 3 {
				t.Fatalf("unexpected lines: %+v", lines)
			}
			if lines[0].BBox != tc.want || lines[2].BBox != tc.want {
				t.Fatalf("annotation geometry: %+v", lines)
			}
			if lines[1].BBox != "[499, 138, 836, 340]" {
				t.Fatalf("image geometry: %+v", lines[1])
			}
		})
	}
}
