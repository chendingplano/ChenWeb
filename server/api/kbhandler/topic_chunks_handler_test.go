package kbhandler

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseTopicArrayItems(t *testing.T) {
	tokens := parseTopicArrayItems("[38-45, 47, 49-50]")
	want := []string{"38-45", "47", "49-50"}
	if !reflect.DeepEqual(tokens, want) {
		t.Fatalf("unexpected tokens: got=%v want=%v", tokens, want)
	}
}

func TestExpandTopicLineTokens(t *testing.T) {
	got := expandTopicLineTokens([]string{"10-12", "8", "11", "9-10", "x"})
	want := []int{8, 9, 10, 11, 12}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected line numbers: got=%v want=%v", got, want)
	}
}

func TestParseTopicChunkLine(t *testing.T) {
	entry, ok := parseTopicChunkLine("53\ttable\t[38-45, 47]\t[k1, k2]\tTable about emissions", 53)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if entry.RecordID != 53 {
		t.Fatalf("unexpected record_id: %d", entry.RecordID)
	}
	if entry.TopicType != "table" {
		t.Fatalf("unexpected topic_type: %q", entry.TopicType)
	}
	if entry.Topic != "Table about emissions" {
		t.Fatalf("unexpected topic: %q", entry.Topic)
	}
	if !reflect.DeepEqual(entry.LineTokens, []string{"38-45", "47"}) {
		t.Fatalf("unexpected line tokens: %v", entry.LineTokens)
	}
	if !reflect.DeepEqual(entry.Keywords, []string{"k1", "k2"}) {
		t.Fatalf("unexpected keywords: %v", entry.Keywords)
	}
}

func TestReadTopicChunkEntries_FromCategoryTree(t *testing.T) {
	root := t.TempDir()
	runRoot := filepath.Join(root, "0", "53")
	if err := os.MkdirAll(filepath.Join(runRoot, "safety_evaluation"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "project_overview.txt"), []byte("53\tpolicy\t[1]\t[intro]\tOverview"), 0o644); err != nil {
		t.Fatalf("write project_overview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "safety_evaluation", "seismic_design.txt"), []byte("53\tlist\t[11-12]\t[risk]\tSeismic scoring"), 0o644); err != nil {
		t.Fatalf("write seismic_design: %v", err)
	}

	entries, err := readTopicChunkEntries(runRoot, 53)
	if err != nil {
		t.Fatalf("readTopicChunkEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries)=%d, want 2", len(entries))
	}
	if entries[0].SeqNo != 1 || entries[1].SeqNo != 2 {
		t.Fatalf("seq numbers not assigned incrementally: %+v", entries)
	}
}

func TestListTopicLeafFiles_SkipsTopicsTxt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "topics.txt"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	leaf := filepath.Join(root, "a", "b.txt")
	if err := os.WriteFile(leaf, []byte("53\tlist\t[1]\t[k]\ttopic"), 0o644); err != nil {
		t.Fatalf("write leaf: %v", err)
	}

	files, err := listTopicLeafFiles(root)
	if err != nil {
		t.Fatalf("listTopicLeafFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files)=%d, want 1 (%v)", len(files), files)
	}
	if !strings.HasSuffix(files[0], string(filepath.Separator)+"a"+string(filepath.Separator)+"b.txt") {
		t.Fatalf("unexpected file listed: %v", files)
	}
}

func TestBuildChunkBoundingBoxes(t *testing.T) {
	lines := []rawLine{
		{PageNumber: 1, LineNumber: 1, Coords: []float64{10, 20, 30, 40}},
		{PageNumber: 1, LineNumber: 2, Coords: []float64{5, 25, 35, 50}},
		{PageNumber: 2, LineNumber: 1, Coords: []float64{100, 110, 120, 140}},
	}
	boxes := buildChunkBoundingBoxes(lines)
	if len(boxes) != 2 {
		t.Fatalf("expected 2 page boxes, got %d", len(boxes))
	}
	if boxes[0].PageNumber != 1 || !reflect.DeepEqual(boxes[0].Coords, []float64{5, 20, 35, 50}) {
		t.Fatalf("unexpected page1 box: %+v", boxes[0])
	}
	if boxes[1].PageNumber != 2 || !reflect.DeepEqual(boxes[1].Coords, []float64{100, 110, 120, 140}) {
		t.Fatalf("unexpected page2 box: %+v", boxes[1])
	}
}
