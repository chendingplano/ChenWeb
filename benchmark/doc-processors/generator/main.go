package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DatasetID      = "doc-processors-synthetic-core"
	DatasetVersion = "1.0.0"
	Seed           = int64(20260713)
)

// GenerateDataset writes the complete deterministic dataset beneath root.
func GenerateDataset(root string) error {
	base := filepath.Join(root, DatasetID, DatasetVersion)
	if err := os.MkdirAll(filepath.Join(base, "cases"), 0o755); err != nil {
		return err
	}
	cs := Cases()
	manifest := struct {
		SchemaVersion    int    `json:"schema_version"`
		DatasetID        string `json:"dataset_id"`
		DatasetVersion   string `json:"dataset_version"`
		GeneratorVersion string `json:"generator_version"`
		Seed             int64  `json:"seed"`
		Cases            []any  `json:"cases"`
	}{1, DatasetID, DatasetVersion, DatasetVersion, Seed, nil}
	for _, c := range cs {
		inRel := filepath.ToSlash(filepath.Join("cases", c.ID, "input.lines.txt"))
		expRel := filepath.ToSlash(filepath.Join("cases", c.ID, "expected.json"))
		manifest.Cases = append(manifest.Cases, map[string]any{"case_id": c.ID, "input": inRel, "expected": expRel, "processors": c.Processors, "tags": c.Tags})
		dir := filepath.Join(base, "cases", c.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		data := []byte("")
		for i, line := range c.Lines {
			// Canonical line files contain seven tab-separated fields; field one is
			// the stable physical line identity and field two carries the text.
			data = append(data, []byte(fmt.Sprintf("%d\t%s\t-\t-\t-\t-\t-\n", i+1, line))...)
		}
		if err := os.WriteFile(filepath.Join(dir, "input.lines.txt"), data, 0o644); err != nil {
			return err
		}
		exp, err := json.MarshalIndent(c.Expected, "", "  ")
		if err != nil {
			return err
		}
		exp = append(exp, '\n')
		if err := os.WriteFile(filepath.Join(dir, "expected.json"), exp, 0o644); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(filepath.Join(base, "manifest.json"), b, 0o644)
}

// Main is exposed for the tiny generator command wrapper.
func Main() error {
	root := "benchmark/doc-processors/datasets"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := GenerateDataset(root); err != nil {
		return fmt.Errorf("generate dataset: %w", err)
	}
	return nil
}
