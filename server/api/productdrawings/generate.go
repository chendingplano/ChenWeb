package productdrawings

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPromptDir is the template directory GenerateAndSave and the System
// Admin drawing generator both read from, exposed for callers (e.g. Product
// Review intake) that need to compose a prompt themselves before calling
// GenerateAndSave.
func DefaultPromptDir() string { return defaultConfig().PromptDir }

// GenerateAndSaveInput is the metadata for a drawing generated with no human
// review step (see GenerateAndSave).
type GenerateAndSaveInput struct {
	Name        string
	Description string
	Keywords    string
	Notes       string
	Prompt      string
	Model       string
}

// GenerateAndSave generates a drawing image and persists it straight to
// kb.product_drawings in one step, skipping the pending/keep review flow
// GeneratePending/KeepPending use for the System Admin page — there is no
// human reviewing the image before a caller like automatic Product Review
// intake keeps it.
func GenerateAndSave(ctx context.Context, in GenerateAndSaveInput) (*ProductDrawing, error) {
	cfg := defaultConfig()
	selection := normalizeSelection(in.Model)
	if selection == "" {
		return nil, fmt.Errorf("unsupported model %q", in.Model)
	}
	img, err := cfg.Provider.Generate(ctx, selection, in.Prompt)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(img, []byte("\x89PNG\r\n\x1a\n")) {
		return nil, fmt.Errorf("provider did not return PNG data")
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, err
	}
	filename := drawingFilename()
	path := filepath.Join(cfg.OutputDir, filename)
	if err := writeExclusive(path, img); err != nil {
		return nil, err
	}
	id, err := insertDrawingRow(ctx, in.Name, in.Description, in.Prompt, in.Keywords, in.Notes, filename, path, selection)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &ProductDrawing{
		ID:          id,
		Name:        strings.TrimSpace(in.Name),
		Description: strings.TrimSpace(in.Description),
		Prompt:      strings.TrimSpace(in.Prompt),
		Keywords:    strings.TrimSpace(in.Keywords),
		Notes:       strings.TrimSpace(in.Notes),
		Filename:    filename,
		Model:       selection,
		ModelName:   modelNameForSelection(selection),
		ImageURL:    fmt.Sprintf("/api/v1/product-drawings/%d/content", id),
	}, nil
}
