package fileconverters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/documentgeometry"
)

func physicalGeometryCurrent(pdfPath string) (bool, error) {
	pdf, err := os.ReadFile(pdfPath)
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(documentgeometry.PhysicalPath(pdfPath))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var physical documentgeometry.PhysicalDocument
	if json.Unmarshal(raw, &physical) != nil {
		return false, nil
	}
	sum := sha256.Sum256(pdf)
	return physical.Version == documentgeometry.Version && physical.Extractor == "pymupdf-tables-v1" && physical.PDFHash == hex.EncodeToString(sum[:]), nil
}

func (s *Service) ensurePhysicalTableGeometry(ctx context.Context, pdfPath string) error {
	current, err := physicalGeometryCurrent(pdfPath)
	if err != nil || current {
		return err
	}
	extract := s.ExtractTableGeometry
	if extract == nil {
		extract = extractPDFTableGeometry
	}
	s.Logger.Info("(20261007-651) extracting missing or stale PDF table geometry", "pdf", pdfPath)
	if err = extract(ctx, pdfPath); err != nil {
		return err
	}
	current, err = physicalGeometryCurrent(pdfPath)
	if err != nil {
		return err
	}
	if !current {
		return fmt.Errorf("(20261007-652) extractor did not produce current PDF geometry for %s", pdfPath)
	}
	return nil
}

// Search ancestors of the service working directory and installed executable.
// Explicit paths support deployments where Python is outside the ChenWeb tree.
func tableGeometryCommand() (python, script string, err error) {
	script = strings.TrimSpace(os.Getenv("PDF_TABLE_GEOMETRY_SCRIPT"))
	if script == "" {
		cwd, _ := os.Getwd()
		executable, _ := os.Executable()
		for _, start := range []string{cwd, filepath.Dir(executable)} {
			for dir := start; dir != ""; dir = filepath.Dir(dir) {
				candidate := filepath.Join(dir, "python", "pdf-parser", "table_geometry.py")
				if info, e := os.Stat(candidate); e == nil && !info.IsDir() {
					script = candidate
					break
				}
				if filepath.Dir(dir) == dir {
					break
				}
			}
			if script != "" {
				break
			}
		}
	}
	if script == "" {
		return "", "", fmt.Errorf("(20261007-653) table_geometry.py not found; set PDF_TABLE_GEOMETRY_SCRIPT")
	}
	script, err = filepath.Abs(script)
	if err != nil {
		return "", "", err
	}
	python = strings.TrimSpace(os.Getenv("PDF_TABLE_GEOMETRY_PYTHON"))
	if python == "" {
		python = filepath.Join(filepath.Dir(script), ".venv", "bin", "python")
		if runtime.GOOS == "windows" {
			python = filepath.Join(filepath.Dir(script), ".venv", "Scripts", "python.exe")
		}
		if _, e := os.Stat(python); e != nil {
			python, err = exec.LookPath("python3")
			if err != nil {
				return "", "", fmt.Errorf("(20261007-654) Python not found; set PDF_TABLE_GEOMETRY_PYTHON: %w", err)
			}
		}
	}
	return python, script, nil
}

func extractPDFTableGeometry(ctx context.Context, pdfPath string) error {
	python, script, err := tableGeometryCommand()
	if err != nil {
		return err
	}
	// CommandContext propagates cancellation; the bound prevents stalled extraction.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, python, script, pdfPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("(20261007-655) PDF geometry extraction failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
