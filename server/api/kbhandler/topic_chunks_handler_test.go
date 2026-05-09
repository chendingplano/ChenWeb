package kbhandler

import (
	"os"
	"path/filepath"
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

func TestReadChunkEntries_IgnoresCategoryTreeWithoutChunksFile(t *testing.T) {
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

	_, err := readChunkEntries(runRoot, 53)
	if err == nil {
		t.Fatalf("expected error when .chunks file is missing")
	}
}

func TestReadChunkEntries_DoesNotFallbackToTopicsFile(t *testing.T) {
	root := t.TempDir()
	runRoot := filepath.Join(root, "0", "53")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "1\ttable\t[38-45, 47]\t[risk, scoring]\tScoring table\n2\tlist\t[9]\t[checklist]\tInspection checklist\n"
	if err := os.WriteFile(filepath.Join(runRoot, "topics.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write topics.txt: %v", err)
	}

	_, err := readChunkEntries(runRoot, 53)
	if err == nil {
		t.Fatalf("expected error when only topics.txt exists")
	}
}

func TestParseChunksFile(t *testing.T) {
	chunksContent := "overlap: []\nlines: [1-5]\noverlap: [5]\nlines: [6-10]\n"
	root := t.TempDir()
	chunkPath := filepath.Join(root, "test.chunks")
	if err := os.WriteFile(chunkPath, []byte(chunksContent), 0o644); err != nil {
		t.Fatalf("write chunks: %v", err)
	}

	entries, err := parseChunksFile(chunkPath, 42)
	if err != nil {
		t.Fatalf("parseChunksFile: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries)=%d, want 2", len(entries))
	}
	if entries[0].SeqNo != 1 || entries[1].SeqNo != 2 {
		t.Fatalf("unexpected seqno: %+v", entries)
	}
	if !reflect.DeepEqual(entries[0].LineTokens, []string{"1-5"}) {
		t.Fatalf("chunk 1 line_tokens: %v", entries[0].LineTokens)
	}
	if !reflect.DeepEqual(entries[1].LineTokens, []string{"6-10"}) {
		t.Fatalf("chunk 2 line_tokens: %v", entries[1].LineTokens)
	}
}

func TestParseChunksFile_IgnoresBlankLinesBetweenEntries(t *testing.T) {
	chunksContent := "overlap: []\nlines: [1-5]\n\noverlap: [5]\nlines: [6-10]\n\noverlap: [10]\nlines: [11-12]\n"
	root := t.TempDir()
	chunkPath := filepath.Join(root, "test.chunks")
	if err := os.WriteFile(chunkPath, []byte(chunksContent), 0o644); err != nil {
		t.Fatalf("write chunks: %v", err)
	}

	entries, err := parseChunksFile(chunkPath, 42)
	if err != nil {
		t.Fatalf("parseChunksFile: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries)=%d, want 3", len(entries))
	}
	if !reflect.DeepEqual(entries[0].LineTokens, []string{"1-5"}) {
		t.Fatalf("chunk 1 line_tokens: %v", entries[0].LineTokens)
	}
	if !reflect.DeepEqual(entries[1].LineTokens, []string{"6-10"}) {
		t.Fatalf("chunk 2 line_tokens: %v", entries[1].LineTokens)
	}
	if !reflect.DeepEqual(entries[2].LineTokens, []string{"11-12"}) {
		t.Fatalf("chunk 3 line_tokens: %v", entries[2].LineTokens)
	}
}

func TestParseTopicsEnrichmentFile(t *testing.T) {
	content := "topic_id: 1\ntopic_type: \"table\"\nlines: [1-5]\ntopic_keywords: [\"risk\", \"scoring\"]\ntopic: \"Scoring table\"\ncategory_paths: []\n\ntopic_id: 2\ntopic_type: \"list\"\nlines: [6-10]\ntopic_keywords: [\"checklist\"]\ntopic: \"Inspection checklist\"\ncategory_paths: []\n"
	root := t.TempDir()
	topicPath := filepath.Join(root, "test.topics")
	if err := os.WriteFile(topicPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write topics: %v", err)
	}

	entries, err := readTopicEnrichment(topicPath)
	if err != nil {
		t.Fatalf("readTopicEnrichment: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries)=%d, want 2", len(entries))
	}
	e1, ok := entries[1]
	if !ok {
		t.Fatalf("missing seqno 1")
	}
	if e1.TopicType != "table" {
		t.Fatalf("topic_type: %q", e1.TopicType)
	}
	if e1.Topic != "Scoring table" {
		t.Fatalf("topic: %q", e1.Topic)
	}
	if !reflect.DeepEqual(e1.Keywords, []string{"risk", "scoring"}) {
		t.Fatalf("keywords: %v", e1.Keywords)
	}
	e2, ok := entries[2]
	if !ok {
		t.Fatalf("missing seqno 2")
	}
	if e2.TopicType != "list" {
		t.Fatalf("topic_type: %q", e2.TopicType)
	}
}

func TestReadChunkEntries_ReadsChunksFile(t *testing.T) {
	root := t.TempDir()
	runRoot := filepath.Join(root, "0", "42")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	chunksContent := "overlap: []\nlines: [1-3]\noverlap: [3]\nlines: [4-6]\n"
	if err := os.WriteFile(filepath.Join(runRoot, "doc_parser.chunks"), []byte(chunksContent), 0o644); err != nil {
		t.Fatalf("write chunks: %v", err)
	}

	entries, err := readChunkEntries(runRoot, 42)
	if err != nil {
		t.Fatalf("readChunkEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries)=%d, want 2", len(entries))
	}
	if entries[0].SeqNo != 1 || entries[1].SeqNo != 2 {
		t.Fatalf("unexpected seqno: %+v", entries)
	}
	if entries[0].TopicType != "general" {
		t.Fatalf("default topic_type: %q", entries[0].TopicType)
	}
}

func TestReadChunkSummaryEnrichment_ReadsLeafSummaries(t *testing.T) {
	root := t.TempDir()
	mustSummary := `summary_id: "93_0_0001"
record_id: 93
level: 0
lines: ["1-3"]
keywords: ["screening", "hearing"]
category_paths: []
summary_begin:
Leaf summary text
summary_end`
	if err := os.WriteFile(filepath.Join(root, "summary_0_0001.txt"), []byte(mustSummary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	got, err := readChunkSummaryEnrichment(root)
	if err != nil {
		t.Fatalf("readChunkSummaryEnrichment: %v", err)
	}
	entry, ok := got[1]
	if !ok {
		t.Fatalf("missing seqno 1: %+v", got)
	}
	if entry.summaryText != "Leaf summary text" {
		t.Fatalf("summaryText=%q", entry.summaryText)
	}
	if !reflect.DeepEqual(entry.keywords, []string{"screening", "hearing"}) {
		t.Fatalf("keywords=%v", entry.keywords)
	}
}

func TestReadChunkEntries_EnrichesChunksFromLeafSummaries(t *testing.T) {
	root := t.TempDir()
	runRoot := filepath.Join(root, "0", "93")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	chunksContent := "overlap: []\nlines: [1-3]\noverlap: [3]\nlines: [4-6]\n"
	if err := os.WriteFile(filepath.Join(runRoot, "doc_parser.chunks"), []byte(chunksContent), 0o644); err != nil {
		t.Fatalf("write chunks: %v", err)
	}
	summary1 := `summary_id: "93_0_0001"
record_id: 93
level: 0
lines: ["1-3"]
keywords: ["screening", "hearing"]
category_paths: []
summary_begin:
First chunk summary
summary_end`
	summary2 := `summary_id: "93_0_0002"
record_id: 93
level: 0
lines: ["4-6"]
keywords: ["vision"]
category_paths: []
summary_begin:
Second chunk summary
summary_end`
	if err := os.WriteFile(filepath.Join(runRoot, "summary_0_0001.txt"), []byte(summary1), 0o644); err != nil {
		t.Fatalf("write summary1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "summary_0_0002.txt"), []byte(summary2), 0o644); err != nil {
		t.Fatalf("write summary2: %v", err)
	}

	entries, err := readChunkEntries(runRoot, 93)
	if err != nil {
		t.Fatalf("readChunkEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries)=%d, want 2", len(entries))
	}
	if entries[0].Topic != "First chunk summary" || entries[1].Topic != "Second chunk summary" {
		t.Fatalf("topics=%q / %q", entries[0].Topic, entries[1].Topic)
	}
	if entries[0].TopicType != "summary" || entries[1].TopicType != "summary" {
		t.Fatalf("topic types=%q / %q", entries[0].TopicType, entries[1].TopicType)
	}
	if !reflect.DeepEqual(entries[0].Keywords, []string{"screening", "hearing"}) {
		t.Fatalf("keywords[0]=%v", entries[0].Keywords)
	}
	if !reflect.DeepEqual(entries[1].Keywords, []string{"vision"}) {
		t.Fatalf("keywords[1]=%v", entries[1].Keywords)
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
