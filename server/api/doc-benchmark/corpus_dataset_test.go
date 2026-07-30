package docbenchmark

import (
	"os"
	"path/filepath"
	"testing"
)

// A minimal, self-contained gold.toml -- enough for gold.Parse/Resolve to
// succeed, independent of the real checked-in fixture used by
// TestLoadCorpusDatasetAgainstRealFixture below. Its subject document id
// must equal gold.SubjectDocument (hardcoded there to this fixture family's
// one subject organization); it is repeated as a literal here rather than
// interpolated to avoid an extra import for one string.
const minimalGoldTOML = `
[[metric_definition]]
id = "m1"
quantity_kind = "Time"

[[authority_document]]
id = "doc:ent-q-syn-001-2026"
family = "enterprise"
title = "Subject standard"

[[authority_document]]
id = "doc:cn"
family = "cn_national"
title = "CN standard"

[[clause]]
id = "c1"
document = "doc:ent-q-syn-001-2026"
metric = "m1"
form = "upper_bound"
value = 120
unit = "ms"
text_template = "Subject clause."

[[clause]]
id = "c2"
document = "doc:cn"
metric = "m1"
form = "upper_bound"
value = 150
unit = "ms"
text_template = "CN clause."

[[expected_verdict]]
metric = "m1"
vs_family = "cn_national"
verdict = "stronger"
`

func writeCorpusDataset(t *testing.T, manifestJSON string, goldTOML string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "gold.toml"), []byte(goldTOML), 0o644); err != nil {
		t.Fatalf("write gold.toml: %v", err)
	}
	return root
}

func validCorpusManifest() string {
	return `{
		"schema_version": 1,
		"dataset_id": "test-corpus",
		"dataset_version": "1.0.0",
		"cases": [{"case_id": "case1", "gold": "gold.toml"}]
	}`
}

func TestLoadCorpusDatasetValid(t *testing.T) {
	root := writeCorpusDataset(t, validCorpusManifest(), minimalGoldTOML)
	ds, err := LoadCorpusDataset(root)
	if err != nil {
		t.Fatalf("LoadCorpusDataset: %v", err)
	}
	if len(ds.Cases) != 1 || ds.Cases[0].CaseID != "case1" {
		t.Fatalf("unexpected dataset: %#v", ds)
	}

	docs := ds.Cases[0].Documents()
	if len(docs) != 2 {
		t.Fatalf("got %d documents, want 2", len(docs))
	}

	expected := ds.Cases[0].Expected()
	if len(expected) != 1 || expected[0].Verdict != "stronger" {
		t.Fatalf("unexpected expected verdicts: %+v", expected)
	}

	actual, err := ds.Cases[0].SimulatedActual()
	if err != nil {
		t.Fatalf("SimulatedActual: %v", err)
	}
	score := ScoreVerdictMatrix(expected, actual)
	if score.Accuracy != 1.0 {
		t.Fatalf("got %+v, want a perfect score on this minimal fixture", score)
	}
}

func TestLoadCorpusDatasetRejectsMissingManifest(t *testing.T) {
	root := t.TempDir() // no manifest.json written
	if _, err := LoadCorpusDataset(root); err == nil {
		t.Fatal("expected an error for a missing manifest.json, got nil")
	}
}

func TestLoadCorpusDatasetRejectsBadSchemaVersion(t *testing.T) {
	manifest := `{"schema_version": 2, "dataset_id": "d", "dataset_version": "1.0.0", "cases": [{"case_id": "c", "gold": "gold.toml"}]}`
	root := writeCorpusDataset(t, manifest, minimalGoldTOML)
	if _, err := LoadCorpusDataset(root); err == nil {
		t.Fatal("expected an error for an unsupported schema_version, got nil")
	}
}

func TestLoadCorpusDatasetRejectsEmptyCases(t *testing.T) {
	manifest := `{"schema_version": 1, "dataset_id": "d", "dataset_version": "1.0.0", "cases": []}`
	root := writeCorpusDataset(t, manifest, minimalGoldTOML)
	if _, err := LoadCorpusDataset(root); err == nil {
		t.Fatal("expected an error for an empty cases list, got nil")
	}
}

func TestLoadCorpusDatasetRejectsDuplicateCaseID(t *testing.T) {
	manifest := `{
		"schema_version": 1, "dataset_id": "d", "dataset_version": "1.0.0",
		"cases": [
			{"case_id": "dup", "gold": "gold.toml"},
			{"case_id": "dup", "gold": "gold.toml"}
		]
	}`
	root := writeCorpusDataset(t, manifest, minimalGoldTOML)
	if _, err := LoadCorpusDataset(root); err == nil {
		t.Fatal("expected an error for a duplicate case_id, got nil")
	}
}

func TestLoadCorpusDatasetRejectsPathTraversal(t *testing.T) {
	manifest := `{
		"schema_version": 1, "dataset_id": "d", "dataset_version": "1.0.0",
		"cases": [{"case_id": "c", "gold": "../outside.toml"}]
	}`
	root := writeCorpusDataset(t, manifest, minimalGoldTOML)
	if _, err := LoadCorpusDataset(root); err == nil {
		t.Fatal("expected an error for a path-traversing gold reference, got nil")
	}
}

func TestLoadCorpusDatasetRejectsAbsolutePath(t *testing.T) {
	manifest := `{
		"schema_version": 1, "dataset_id": "d", "dataset_version": "1.0.0",
		"cases": [{"case_id": "c", "gold": "/etc/passwd"}]
	}`
	root := writeCorpusDataset(t, manifest, minimalGoldTOML)
	if _, err := LoadCorpusDataset(root); err == nil {
		t.Fatal("expected an error for an absolute gold path, got nil")
	}
}

func TestLoadCorpusDatasetRejectsMissingGoldFile(t *testing.T) {
	manifest := validCorpusManifest()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	// gold.toml intentionally not written.
	if _, err := LoadCorpusDataset(root); err == nil {
		t.Fatal("expected an error for a referenced gold.toml that does not exist, got nil")
	}
}

func TestLoadCorpusDatasetRejectsUnresolvableGold(t *testing.T) {
	// A gold.toml that Resolve rejects (clause references an unknown metric).
	badGold := `
[[authority_document]]
id = "doc:subject"
family = "enterprise"
title = "Subject"

[[clause]]
id = "c1"
document = "doc:subject"
metric = "does-not-exist"
form = "upper_bound"
value = 1
unit = "ms"
text_template = "x"
`
	root := writeCorpusDataset(t, validCorpusManifest(), badGold)
	if _, err := LoadCorpusDataset(root); err == nil {
		t.Fatal("expected an error for a gold file that fails Resolve, got nil")
	}
}

// TestLoadCorpusDatasetAgainstRealFixture loads the actual checked-in
// display-module-v1 corpus dataset (benchmark/doc-processors/gold/
// display-module-v1/manifest.json + gold.toml) through the new production
// loader and confirms it reproduces the same 36/36 perfect score the
// hand-rolled test loaders in verdict_score_gold_test.go and
// comparison/gold_fixture_test.go already established -- this time via the
// real LoadCorpusDataset path, not test-only duplicated logic.
func TestLoadCorpusDatasetAgainstRealFixture(t *testing.T) {
	ds, err := LoadCorpusDataset("../../../benchmark/doc-processors/gold/display-module-v1")
	if err != nil {
		t.Fatalf("LoadCorpusDataset: %v", err)
	}
	if len(ds.Cases) != 1 {
		t.Fatalf("got %d cases, want 1", len(ds.Cases))
	}
	c := ds.Cases[0]

	docs := c.Documents()
	if len(docs) != 9 {
		t.Fatalf("got %d documents, want 9 authority documents", len(docs))
	}

	expected := c.Expected()
	if len(expected) != 36 {
		t.Fatalf("got %d expected verdict cells, want 36", len(expected))
	}

	actual, err := c.SimulatedActual()
	if err != nil {
		t.Fatalf("SimulatedActual: %v", err)
	}
	score := ScoreVerdictMatrix(expected, actual)
	if score.TotalCells != 36 || score.MatchedCells != 36 || score.Accuracy != 1.0 {
		t.Fatalf("loading the real fixture through LoadCorpusDataset scored %+v, want 36/36", score)
	}
}
