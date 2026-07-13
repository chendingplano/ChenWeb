package generator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
)

func TestGenerateDeterministicAndValid(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if err := GenerateDataset(a); err != nil {
		t.Fatal(err)
	}
	if err := GenerateDataset(b); err != nil {
		t.Fatal(err)
	}
	rootA := filepath.Join(a, DatasetID, DatasetVersion)
	rootB := filepath.Join(b, DatasetID, DatasetVersion)
	var walk func(string, string) error
	walk = func(x, y string) error {
		entries, err := os.ReadDir(x)
		if err != nil {
			return err
		}
		for _, e := range entries {
			xa, ya := filepath.Join(x, e.Name()), filepath.Join(y, e.Name())
			if e.IsDir() {
				if err := walk(xa, ya); err != nil {
					return err
				}
			} else {
				p, _ := os.ReadFile(xa)
				r, _ := os.ReadFile(ya)
				if !bytes.Equal(p, r) {
					t.Fatalf("nondeterministic %s", xa)
				}
			}
		}
		return nil
	}
	if err := walk(rootA, rootB); err != nil {
		t.Fatal(err)
	}
	if err := docbenchmark.ValidateDataset(rootA); err != nil {
		t.Fatalf("generated dataset invalid: %v", err)
	}
}
