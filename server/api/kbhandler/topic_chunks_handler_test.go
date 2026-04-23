package kbhandler

import (
	"reflect"
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
	entry, ok := parseTopicChunkLine("3\ttable\t[38-45, 47]\t[k1, k2]\tTable about emissions")
	if !ok {
		t.Fatalf("expected parse success")
	}
	if entry.SeqNo != 3 {
		t.Fatalf("unexpected seqno: %d", entry.SeqNo)
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
