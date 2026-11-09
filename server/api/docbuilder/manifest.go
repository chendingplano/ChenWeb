package docbuilder

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadManifest reads <dir>/.docbuilder/manifest.json; a missing file is an empty manifest.
func LoadManifest(dir string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, manifestDir, manifestFile))
	if errors.Is(err, os.ErrNotExist) {
		return &Manifest{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := json.Unmarshal(raw, m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}

// SaveManifest writes the manifest atomically.
func SaveManifest(dir string, m *Manifest) error {
	m.Version = 1
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	d := filepath.Join(dir, manifestDir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(d, manifestFile+".tmp")
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(d, manifestFile))
}

// Refresh scans dir and merges the result into m: a file keeps its saved category (even when its
// content changed, so the one-time classification is not repeated), new files start unclassified,
// and files that disappeared are dropped.
func Refresh(dir string, m *Manifest) error {
	scanned, err := Scan(dir)
	if err != nil {
		return err
	}
	saved := map[string]string{}
	for _, e := range m.Entries {
		saved[e.Path] = e.Category
	}
	for i := range scanned {
		scanned[i].Category = saved[scanned[i].Path]
	}
	m.Entries = scanned
	return nil
}
