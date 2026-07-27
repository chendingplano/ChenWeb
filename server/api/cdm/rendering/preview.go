package rendering

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

// RenderPreviewSVG renders an in-memory draft without creating a stored
// document version or any persistent rendering artifacts.
func RenderPreviewSVG(
	ctx context.Context,
	doc *model.Document,
	themeSrc []byte,
	typstBin string,
) ([][]byte, error) {
	if err := model.Validate(doc); err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "cdm-preview-*")
	if err != nil {
		return nil, fmt.Errorf("cdm: create preview workdir: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, "theme.typ"), themeSrc, 0o644); err != nil {
		return nil, fmt.Errorf("cdm: write preview theme: %w", err)
	}

	typstSrc, err := (&TypstRenderer{}).RenderDocument(doc)
	if err != nil {
		return nil, fmt.Errorf("cdm: render preview document: %w", err)
	}
	typstPath := filepath.Join(dir, "doc.typ")
	if err := os.WriteFile(typstPath, typstSrc, 0o644); err != nil {
		return nil, fmt.Errorf("cdm: write preview source: %w", err)
	}

	return RenderSVGPagesContext(ctx, typstBin, typstPath)
}
