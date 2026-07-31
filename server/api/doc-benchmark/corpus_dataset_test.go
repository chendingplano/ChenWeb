package docbenchmark

import (
	"os"
	"path/filepath"
	"strings"
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
		"cases": [{
			"case_id": "case1",
			"gold": "gold.toml",
			` + validDocumentProfilesJSON() + `
		}]
	}`
}

func validDocumentProfilesJSON() string {
	return `"document_profiles": {
		"doc:ent-q-syn-001-2026": {
			"store_profile": "product-specification",
			"document_kind": "enterprise-standard",
			"expected_processors": {
				"extract_metrics": "required",
				"extract_provisions": "useful"
			}
		},
		"doc:cn": {
			"store_profile": "regulated-reference",
			"document_kind": "authority-standard",
			"expected_processors": {
				"extract_metrics": "useful",
				"extract_provisions": "required"
			}
		}
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

func TestLoadCorpusDatasetRejectsInvalidDocumentProfiles(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     []string
	}{
		{
			name: "missing generated document",
			manifest: `{
				"schema_version": 1,
				"dataset_id": "test-corpus",
				"dataset_version": "1.0.0",
				"cases": [{
					"case_id": "case1",
					"gold": "gold.toml",
					"document_profiles": {
						"doc:ent-q-syn-001-2026": {
							"store_profile": "product-specification",
							"document_kind": "enterprise-standard",
							"expected_processors": {
								"extract_metrics": "required",
								"extract_provisions": "useful"
							}
						}
					}
				}]
			}`,
			want: []string{`document_profiles[doc:cn]: required`},
		},
		{
			name: "unknown document",
			manifest: `{
				"schema_version": 1,
				"dataset_id": "test-corpus",
				"dataset_version": "1.0.0",
				"cases": [{
					"case_id": "case1",
					"gold": "gold.toml",
					"document_profiles": {
						"doc:ent-q-syn-001-2026": {
							"store_profile": "product-specification",
							"document_kind": "enterprise-standard",
							"expected_processors": {
								"extract_metrics": "required",
								"extract_provisions": "useful"
							}
						},
						"doc:cn": {
							"store_profile": "regulated-reference",
							"document_kind": "authority-standard",
							"expected_processors": {
								"extract_metrics": "useful",
								"extract_provisions": "required"
							}
						},
						"doc:ghost": {
							"store_profile": "regulated-reference",
							"document_kind": "authority-standard",
							"expected_processors": {
								"extract_metrics": "useful",
								"extract_provisions": "required"
							}
						}
					}
				}]
			}`,
			want: []string{`document_profiles[doc:ghost]: document is not generated`},
		},
		{
			name:     "blank store profile",
			manifest: strings.Replace(validCorpusManifest(), `"store_profile": "product-specification"`, `"store_profile": "   "`, 1),
			want:     []string{`document_profiles[doc:ent-q-syn-001-2026].store_profile`},
		},
		{
			name:     "blank document kind",
			manifest: strings.Replace(validCorpusManifest(), `"document_kind": "enterprise-standard"`, `"document_kind": "  "`, 1),
			want:     []string{`document_profiles[doc:ent-q-syn-001-2026].document_kind`},
		},
		{
			name: "missing expected processors",
			manifest: `{
				"schema_version": 1,
				"dataset_id": "test-corpus",
				"dataset_version": "1.0.0",
				"cases": [{
					"case_id": "case1",
					"gold": "gold.toml",
					"document_profiles": {
						"doc:ent-q-syn-001-2026": {
							"store_profile": "product-specification",
							"document_kind": "enterprise-standard"
						},
						"doc:cn": {
							"store_profile": "regulated-reference",
							"document_kind": "authority-standard",
							"expected_processors": {
								"extract_metrics": "useful",
								"extract_provisions": "required"
							}
						}
					}
				}]
			}`,
			want: []string{`document_profiles[doc:ent-q-syn-001-2026].expected_processors: required`},
		},
		{
			name: "empty expected processors",
			manifest: strings.Replace(validCorpusManifest(), `"expected_processors": {
				"extract_metrics": "required",
				"extract_provisions": "useful"
			}`, `"expected_processors": {}`, 1),
			want: []string{`document_profiles[doc:ent-q-syn-001-2026].expected_processors: must not be empty`},
		},
		{
			name:     "unknown processor",
			manifest: strings.Replace(validCorpusManifest(), `"extract_metrics": "required"`, `"not_a_processor": "required"`, 1),
			want:     []string{`document_profiles[doc:ent-q-syn-001-2026].expected_processors[not_a_processor]`},
		},
		{
			name:     "processor alias rejected",
			manifest: strings.Replace(validCorpusManifest(), `"extract_metrics": "required"`, `"extract-metrics": "required"`, 1),
			want:     []string{`document_profiles[doc:ent-q-syn-001-2026].expected_processors[extract-metrics]`, `processor name must be canonical`},
		},
		{
			name:     "unknown applicability",
			manifest: strings.Replace(validCorpusManifest(), `"extract_metrics": "required"`, `"extract_metrics": "sometimes"`, 1),
			want:     []string{`document_profiles[doc:ent-q-syn-001-2026].expected_processors[extract_metrics]`},
		},
		{
			name: "missing selected expectation",
			manifest: `{
				"schema_version": 1,
				"dataset_id": "test-corpus",
				"dataset_version": "1.0.0",
				"cases": [{
					"case_id": "case1",
					"gold": "gold.toml",
					"document_profiles": {
						"doc:ent-q-syn-001-2026": {
							"store_profile": "product-specification",
							"document_kind": "enterprise-standard",
							"expected_processors": {
								"extract_metrics": "required"
							}
						},
						"doc:cn": {
							"store_profile": "regulated-reference",
							"document_kind": "authority-standard",
							"expected_processors": {
								"extract_metrics": "useful",
								"extract_provisions": "required"
							}
						}
					}
				}]
			}`,
			want: []string{`expected_processors[extract_provisions]: required`},
		},
		{
			name: "normalized duplicate document id",
			manifest: strings.Replace(validCorpusManifest(), `"doc:cn": {`, `" doc:cn ": {
			"store_profile": "regulated-reference",
			"document_kind": "authority-standard",
			"expected_processors": {
				"extract_metrics": "useful",
				"extract_provisions": "required"
			}
		},
		"doc:cn": {`, 1),
			want: []string{`document_profiles[doc:cn]: duplicate normalized document id`},
		},
		{
			name:     "normalized duplicate processor id",
			manifest: strings.Replace(validCorpusManifest(), `"extract_provisions": "useful"`, `" extract_provisions ": "useful", "extract_provisions": "required"`, 1),
			want:     []string{`expected_processors[extract_provisions]: duplicate normalized processor id`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeCorpusDataset(t, tt.manifest, minimalGoldTOML)
			_, err := LoadCorpusDataset(root)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q missing %q", err, want)
				}
			}
		})
	}
}

func TestLoadCorpusDatasetRejectsDuplicateDocumentProfileJSONKey(t *testing.T) {
	root := writeCorpusDataset(t, `{
		"schema_version": 1,
		"dataset_id": "test-corpus",
		"dataset_version": "1.0.0",
		"cases": [{
			"case_id": "case1",
			"gold": "gold.toml",
			"document_profiles": {
				"doc:ent-q-syn-001-2026": {
					"store_profile": "product-specification",
					"document_kind": "enterprise-standard",
					"expected_processors": {
						"extract_metrics": "required",
						"extract_provisions": "useful"
					}
				},
				"doc:cn": {
					"store_profile": "regulated-reference",
					"document_kind": "authority-standard",
					"expected_processors": {
						"extract_metrics": "useful",
						"extract_provisions": "required"
					}
				},
				"doc:cn": {
					"store_profile": "duplicate",
					"document_kind": "duplicate",
					"expected_processors": {
						"extract_metrics": "required",
						"extract_provisions": "required"
					}
				}
			}
		}]
	}`, minimalGoldTOML)
	_, err := LoadCorpusDataset(root)
	if err == nil || !strings.Contains(err.Error(), `document_profiles.doc:cn`) {
		t.Fatalf("got %v, want duplicate document_profiles key error", err)
	}
}

func TestLoadCorpusDatasetRejectsDuplicateExpectedProcessorJSONKey(t *testing.T) {
	root := writeCorpusDataset(t, `{
		"schema_version": 1,
		"dataset_id": "test-corpus",
		"dataset_version": "1.0.0",
		"cases": [{
			"case_id": "case1",
			"gold": "gold.toml",
			"document_profiles": {
				"doc:ent-q-syn-001-2026": {
					"store_profile": "product-specification",
					"document_kind": "enterprise-standard",
					"expected_processors": {
						"extract_metrics": "required",
						"extract_metrics": "useful",
						"extract_provisions": "useful"
					}
				},
				"doc:cn": {
					"store_profile": "regulated-reference",
					"document_kind": "authority-standard",
					"expected_processors": {
						"extract_metrics": "useful",
						"extract_provisions": "required"
					}
				}
			}
		}]
	}`, minimalGoldTOML)
	_, err := LoadCorpusDataset(root)
	if err == nil || !strings.Contains(err.Error(), `expected_processors.extract_metrics`) {
		t.Fatalf("got %v, want duplicate expected_processors key error", err)
	}
}

func TestCorpusCaseContentHashChangesWithProfileOrGold(t *testing.T) {
	root := writeCorpusDataset(t, validCorpusManifest(), minimalGoldTOML)
	one, err := LoadCorpusDataset(root)
	if err != nil {
		t.Fatalf("LoadCorpusDataset: %v", err)
	}

	profileManifest := strings.Replace(validCorpusManifest(), `"product-specification"`, `"subject-profile"`, 1)
	profileRoot := writeCorpusDataset(t, profileManifest, minimalGoldTOML)
	two, err := LoadCorpusDataset(profileRoot)
	if err != nil {
		t.Fatalf("LoadCorpusDataset profile mutation: %v", err)
	}
	if one.Cases[0].ContentHash == two.Cases[0].ContentHash {
		t.Fatal("profile mutation did not change content hash")
	}

	goldRoot := writeCorpusDataset(t, validCorpusManifest(), minimalGoldTOML+"\n# gold mutation\n")
	three, err := LoadCorpusDataset(goldRoot)
	if err != nil {
		t.Fatalf("LoadCorpusDataset gold mutation: %v", err)
	}
	if one.Cases[0].ContentHash == three.Cases[0].ContentHash {
		t.Fatal("gold mutation did not change content hash")
	}
}

func TestCorpusCaseContentHashIgnoresMapInsertionOrder(t *testing.T) {
	manifestA := `{
		"schema_version": 1,
		"dataset_id": "test-corpus",
		"dataset_version": "1.0.0",
		"cases": [{
			"case_id": "case1",
			"gold": "gold.toml",
			"tags": ["alpha", "beta"],
			"document_profiles": {
				"doc:ent-q-syn-001-2026": {
					"store_profile": "product-specification",
					"document_kind": "enterprise-standard",
					"expected_processors": {
						"extract_metrics": "required",
						"extract_provisions": "useful"
					}
				},
				"doc:cn": {
					"store_profile": "regulated-reference",
					"document_kind": "authority-standard",
					"expected_processors": {
						"extract_metrics": "useful",
						"extract_provisions": "required"
					}
				}
			}
		}]
	}`
	manifestB := `{
		"schema_version": 1,
		"dataset_id": "test-corpus",
		"dataset_version": "1.0.0",
		"cases": [{
			"case_id": "case1",
			"gold": "gold.toml",
			"tags": ["alpha", "beta"],
			"document_profiles": {
				"doc:cn": {
					"document_kind": "authority-standard",
					"store_profile": "regulated-reference",
					"expected_processors": {
						"extract_provisions": "required",
						"extract_metrics": "useful"
					}
				},
				"doc:ent-q-syn-001-2026": {
					"expected_processors": {
						"extract_provisions": "useful",
						"extract_metrics": "required"
					},
					"document_kind": "enterprise-standard",
					"store_profile": "product-specification"
				}
			}
		}]
	}`

	left, err := LoadCorpusDataset(writeCorpusDataset(t, manifestA, minimalGoldTOML))
	if err != nil {
		t.Fatalf("LoadCorpusDataset A: %v", err)
	}
	right, err := LoadCorpusDataset(writeCorpusDataset(t, manifestB, minimalGoldTOML))
	if err != nil {
		t.Fatalf("LoadCorpusDataset B: %v", err)
	}
	if left.Cases[0].ContentHash != right.Cases[0].ContentHash {
		t.Fatalf("got %q and %q, want equal hashes", left.Cases[0].ContentHash, right.Cases[0].ContentHash)
	}
}

func TestCorpusCaseContentHashUsesSHA256Prefix(t *testing.T) {
	ds, err := LoadCorpusDataset(writeCorpusDataset(t, validCorpusManifest(), minimalGoldTOML))
	if err != nil {
		t.Fatalf("LoadCorpusDataset: %v", err)
	}
	if !strings.HasPrefix(ds.Cases[0].ContentHash, "sha256:") {
		t.Fatalf("got %q, want sha256: prefix", ds.Cases[0].ContentHash)
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
