package rendering_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/rendering"
)

func TestRenderSVGPages_SinglePage(t *testing.T) {
	typstBin := requireTypst(t)
	doc := cdmfixtures.JaroWinkler()
	typPath := renderToTempDir(t, &doc)

	pages, err := rendering.RenderSVGPages(typstBin, typPath)
	if err != nil {
		t.Fatalf("render svg: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}
	if len(pages[0]) == 0 {
		t.Fatal("expected non-empty SVG content")
	}
	if !bytesContainSVGRoot(pages[0]) {
		t.Fatalf("expected SVG content, got:\n%s", pages[0])
	}
}

func TestRenderSVGPages_MultiplePagesInOrder(t *testing.T) {
	typstBin := requireTypst(t)

	var blocks []model.Block
	for i := range 40 {
		blocks = append(blocks, model.Block{
			ID:   idFor(i),
			Type: "paragraph",
			Content: []model.Inline{
				{Type: "text", Text: "Content used to force pagination for the SVG page-ordering test."},
			},
		})
	}
	doc := model.Document{Title: "Pagination", SchemaVersion: model.SchemaVersion, Blocks: blocks}
	typPath := renderToTempDir(t, &doc)

	pages, err := rendering.RenderSVGPages(typstBin, typPath)
	if err != nil {
		t.Fatalf("render svg: %v", err)
	}
	if len(pages) < 2 {
		t.Fatalf("expected at least 2 pages, got %d", len(pages))
	}
	for i, p := range pages {
		if len(p) == 0 {
			t.Errorf("page %d is empty", i+1)
		}
	}
}

func TestRenderSVGPagesContext_ReturnsContextCanceledWhenCompileIsAborted(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-typst.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatalf("write fake typst: %v", err)
	}
	typPath := filepath.Join(dir, "doc.typ")
	if err := os.WriteFile(typPath, []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write typst input: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	_, err := rendering.RenderSVGPagesContext(ctx, script, typPath)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func bytesContainSVGRoot(b []byte) bool {
	for i := 0; i+4 < len(b); i++ {
		if string(b[i:i+4]) == "<svg" {
			return true
		}
	}
	return false
}
