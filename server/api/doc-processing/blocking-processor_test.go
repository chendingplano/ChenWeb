package docprocessing

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestBuildBlocks_SinglePage(t *testing.T) {
	input := "1\t1\theading\tArial\t12\t[0,0,100,20]\tHello"
	buf, err := buildBlocks([]byte(input), 3)
	if err != nil {
		t.Fatalf("buildBlocks error: %v", err)
	}
	if len(buf.Blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(buf.Blocks))
	}
	b := buf.Blocks[0]
	if b.Index != 1 {
		t.Errorf("block index: want 1, got %d", b.Index)
	}
	if len(b.Lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(b.Lines))
	}
	if b.Lines[0].Flag != "n" {
		t.Errorf("flag: want n, got %s", b.Lines[0].Flag)
	}
	if b.Lines[0].Content != "Hello" {
		t.Errorf("content: want Hello, got %s", b.Lines[0].Content)
	}
}

func TestBuildBlocks_NoPreviousOverlapOnFirstBlock(t *testing.T) {
	// 4 pages, block size 3 → 2 blocks
	// block 1: core pages 1-3 (n) + next-overlap page 4 (o)
	// block 2: prev-overlap page 3 (o) + core page 4 (n)
	lines := []string{
		"1\t1\theading\tArial\t12\t[0,0,1,1]\tPage1Line1",
		"2\t2\tparagraph\tArial\t11\t[0,0,1,1]\tPage2Line1",
		"3\t3\tparagraph\tArial\t11\t[0,0,1,1]\tPage3Line1",
		"4\t4\tparagraph\tArial\t11\t[0,0,1,1]\tPage4Line1",
	}
	buf, err := buildBlocks([]byte(strings.Join(lines, "\n")), 3)
	if err != nil {
		t.Fatalf("buildBlocks error: %v", err)
	}
	if len(buf.Blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(buf.Blocks))
	}

	b1 := buf.Blocks[0]
	// 3 normal (pages 1,2,3) + 1 overlap (page 4)
	if len(b1.Lines) != 4 {
		t.Fatalf("block1: want 4 lines, got %d", len(b1.Lines))
	}
	for i := 0; i < 3; i++ {
		if b1.Lines[i].Flag != "n" {
			t.Errorf("block1 line %d: want flag n, got %s", i, b1.Lines[i].Flag)
		}
	}
	if b1.Lines[3].Flag != "o" {
		t.Errorf("block1 last line: want flag o, got %s", b1.Lines[3].Flag)
	}

	b2 := buf.Blocks[1]
	// 1 overlap (page 3) + 1 normal (page 4)
	if len(b2.Lines) != 2 {
		t.Fatalf("block2: want 2 lines, got %d", len(b2.Lines))
	}
	if b2.Lines[0].Flag != "o" || b2.Lines[0].PageNumber != 3 {
		t.Errorf("block2[0]: want o/page3, got %s/page%d", b2.Lines[0].Flag, b2.Lines[0].PageNumber)
	}
	if b2.Lines[1].Flag != "n" || b2.Lines[1].PageNumber != 4 {
		t.Errorf("block2[1]: want n/page4, got %s/page%d", b2.Lines[1].Flag, b2.Lines[1].PageNumber)
	}
}

func TestBuildBlocks_ThreeFullBlocks(t *testing.T) {
	// 9 pages, block size 3 → 3 blocks
	// block 1: pages 1-3 (n), page 4 (o)              = 4 lines
	// block 2: page 3 (o), pages 4-6 (n), page 7 (o)  = 5 lines
	// block 3: page 6 (o), pages 7-9 (n)              = 4 lines
	var lines []string
	for pg := 1; pg <= 9; pg++ {
		lines = append(lines, fmt.Sprintf("1\t%d\tparagraph\tArial\t11\t[0,0,1,1]\tcontent", pg))
	}
	buf, err := buildBlocks([]byte(strings.Join(lines, "\n")), 3)
	if err != nil {
		t.Fatalf("buildBlocks error: %v", err)
	}
	if len(buf.Blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d", len(buf.Blocks))
	}
	if len(buf.Blocks[0].Lines) != 4 {
		t.Errorf("block1: want 4 lines, got %d", len(buf.Blocks[0].Lines))
	}
	if len(buf.Blocks[1].Lines) != 5 {
		t.Errorf("block2: want 5 lines, got %d", len(buf.Blocks[1].Lines))
	}
	if len(buf.Blocks[2].Lines) != 4 {
		t.Errorf("block3: want 4 lines, got %d", len(buf.Blocks[2].Lines))
	}

	// Spot-check overlap flags in block 2
	b2 := buf.Blocks[1]
	if b2.Lines[0].Flag != "o" || b2.Lines[0].PageNumber != 3 {
		t.Errorf("block2[0]: want o/page3, got %s/page%d", b2.Lines[0].Flag, b2.Lines[0].PageNumber)
	}
	if b2.Lines[4].Flag != "o" || b2.Lines[4].PageNumber != 7 {
		t.Errorf("block2[4]: want o/page7, got %s/page%d", b2.Lines[4].Flag, b2.Lines[4].PageNumber)
	}
}

func TestBuildBlocks_EmptyInput(t *testing.T) {
	buf, err := buildBlocks([]byte(""), 3)
	if err != nil {
		t.Fatalf("buildBlocks error: %v", err)
	}
	if len(buf.Blocks) != 0 {
		t.Errorf("want 0 blocks, got %d", len(buf.Blocks))
	}
}

func TestBuildBlocks_MalformedLinesSkipped(t *testing.T) {
	input := "not-valid\n1\t1\theading\tArial\t12\t[0,0,1,1]\tGood"
	buf, err := buildBlocks([]byte(input), 3)
	if err != nil {
		t.Fatalf("buildBlocks error: %v", err)
	}
	if len(buf.Blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(buf.Blocks))
	}
	if len(buf.Blocks[0].Lines) != 1 {
		t.Errorf("want 1 line, got %d", len(buf.Blocks[0].Lines))
	}
}

func TestBuildBlocks_FontFieldsDropped(t *testing.T) {
	input := "1\t1\theading\tTimes\t14\t[1,2,3,4]\tContent Here"
	buf, err := buildBlocks([]byte(input), 3)
	if err != nil {
		t.Fatalf("buildBlocks error: %v", err)
	}
	line := buf.Blocks[0].Lines[0]
	if line.Content != "Content Here" {
		t.Errorf("content: want 'Content Here', got %q", line.Content)
	}
	out := line.String()
	if strings.Contains(out, "Times") || strings.Contains(out, "\t14\t") || strings.Contains(out, "[1,2,3,4]") {
		t.Errorf("output should not contain font/size/coordinate fields: %s", out)
	}
}

func TestBlockLine_String(t *testing.T) {
	l := BlockLine{Flag: "n", LineNumber: 5, PageNumber: 2, LineType: "paragraph", Content: "hello world"}
	got := l.String()
	want := "n\t5\t2\tparagraph\thello world"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestBlockBufferFromContext_NilWhenNotSet(t *testing.T) {
	if buf := BlockBufferFromContext(context.Background()); buf != nil {
		t.Errorf("expected nil block buffer from plain context")
	}
}

func TestBlockBufferFromContext_ReturnsAfterSet(t *testing.T) {
	ctx, h := withBlockBufferHolder(context.Background())
	h.mu.Lock()
	h.buffer = &BlockBuffer{Blocks: []Block{{Index: 1}}}
	h.mu.Unlock()

	buf := BlockBufferFromContext(ctx)
	if buf == nil {
		t.Fatal("expected non-nil block buffer")
	}
	if len(buf.Blocks) != 1 {
		t.Errorf("want 1 block, got %d", len(buf.Blocks))
	}
}

func TestControlService_BlockingProcessorAlwaysRuns(t *testing.T) {
	// Even when operations filters to a specific processor, blocking still runs first.
	got := make([]string, 0, 3)
	svc := &ControlService{
		BlockingProcessor: fakeProcessor{name: "blocking", calls: &got},
		Processors: []Processor{
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "extract_doc_metadata", calls: &got},
		},
	}

	// Request only chunking — blocking should still run before it.
	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1","operation":["chunking"]}`))

	if len(got) != 2 {
		t.Fatalf("want 2 calls (blocking + chunking), got %v", got)
	}
	if got[0] != "blocking" {
		t.Errorf("first call: want blocking, got %s", got[0])
	}
	if got[1] != "chunking" {
		t.Errorf("second call: want chunking, got %s", got[1])
	}
}

func TestControlService_BlockingProcessorRunsWithNoOperations(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		BlockingProcessor: fakeProcessor{name: "blocking", calls: &got},
		Processors: []Processor{
			fakeProcessor{name: "chunking", calls: &got},
		},
	}

	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`))

	if len(got) != 2 {
		t.Fatalf("want 2 calls, got %v", got)
	}
	if got[0] != "blocking" {
		t.Errorf("first call: want blocking, got %s", got[0])
	}
}
