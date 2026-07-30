package docbenchmark

// A corpus-level case (ADR 2026072901 DR22/P3's "outcome scorer") spans
// several generated documents scored as one verdict matrix, unlike the
// existing single-input-file Dataset/Case in dataset.go. Rather than
// retrofitting Case/Manifest -- both load-bearing across hashing
// (dataset_hash.go), execution (application_execute.go), and evidence
// (application_evidence.go) -- CorpusDataset is a fully parallel type: it
// shares this package's path-safety helpers (readRegularFile,
// validateReference, decodeStrict, fieldError) but touches none of the
// existing Dataset/Case machinery.
//
// This loader stops at "load a corpus case, generate its documents, and
// compute its expected/simulated verdict matrices" -- it is not wired into
// the orchestrator/runner/store execution engine that actually invokes a
// live doc-processor pipeline (that needs a real DB, NATS, and LLM
// credentials this loader does not require and this package's tests do not
// assume are available).

import (
	"fmt"
	"os"
	"path/filepath"

	gold "github.com/chendingplano/deepdoc/benchmark/doc-processors/gold/display-module-v1"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

type CorpusManifest struct {
	SchemaVersion  int                  `json:"schema_version"`
	DatasetID      string               `json:"dataset_id"`
	DatasetVersion string               `json:"dataset_version"`
	Cases          []CorpusManifestCase `json:"cases"`
}

type CorpusManifestCase struct {
	CaseID string   `json:"case_id"`
	Gold   string   `json:"gold"`
	Tags   []string `json:"tags,omitempty"`
}

// CorpusCase is one loaded, validated corpus-level case: a gold ontology
// fixture (ADR 2026072901 DR12) resolved into generated CDM documents and an
// expected verdict matrix.
type CorpusCase struct {
	CaseID string
	Tags   []string

	Gold     gold.File
	Resolved *gold.Resolved
}

// Documents renders this case's gold fixture into CDM documents, one per
// authority_document, via the same path TestGroundingRoundTrip exercises
// against the real Typst renderer.
func (c *CorpusCase) Documents() []model.Document {
	return gold.BuildDocuments(c.Gold)
}

// Expected returns this case's gold-expected verdict matrix as VerdictCells,
// ready for ScoreVerdictMatrix.
func (c *CorpusCase) Expected() []VerdictCell {
	return toVerdictCells(c.Resolved.Rows)
}

// SimulatedActual returns a stand-in verdict matrix computed directly from
// this case's own clause data (comparison.EvaluateFamily), as a perfect,
// already-normalized pipeline run would produce it. It is NOT real pipeline
// output -- extract_metrics still emits free text and normalize_assertions
// does not exist (ADR 2026072901) -- and must never be presented as one. Its
// purpose is to exercise ScoreVerdictMatrix and this dataset-loading path
// end to end before those pieces exist.
func (c *CorpusCase) SimulatedActual() ([]VerdictCell, error) {
	rows, err := c.Resolved.SimulatedActual()
	if err != nil {
		return nil, err
	}
	return toVerdictCells(rows), nil
}

func toVerdictCells(rows []gold.VerdictRow) []VerdictCell {
	cells := make([]VerdictCell, len(rows))
	for i, r := range rows {
		cells[i] = VerdictCell{
			VerdictCellKey: VerdictCellKey{Metric: r.Metric, Family: r.Family, Object: r.Object},
			Verdict:        r.Verdict,
		}
	}
	return cells
}

// CorpusDataset is a loaded, validated set of corpus-level cases.
type CorpusDataset struct {
	Root     string
	Manifest CorpusManifest
	Cases    []CorpusCase
}

// LoadCorpusDataset loads and validates a corpus dataset manifest.json at
// root, resolving each case's referenced gold.toml file. It applies the same
// path-safety discipline as LoadDataset: every reference must be a plain,
// ASCII, relative, non-escaping path, checked through an os.Root-scoped,
// symlink-rejecting reader, and no two cases may reference the same
// normalized path.
func LoadCorpusDataset(root string) (*CorpusDataset, error) {
	canonicalRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("corpus dataset root: %w", err)
	}
	datasetRoot, err := os.OpenRoot(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("corpus dataset root: %w", err)
	}
	defer datasetRoot.Close()

	manifestBytes, err := readRegularFile(datasetRoot, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	var manifest CorpusManifest
	if err := decodeStrict(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}

	var problems validationErrors
	if manifest.SchemaVersion != 1 {
		problems = append(problems, fmt.Sprintf("schema_version: unsupported value %d (want 1)", manifest.SchemaVersion))
	}
	if manifest.DatasetID == "" {
		problems = append(problems, "dataset_id: required")
	}
	if manifest.DatasetVersion == "" {
		problems = append(problems, "dataset_version: required")
	}
	if len(manifest.Cases) == 0 {
		problems = append(problems, "cases: must not be empty")
	}

	seenCaseIDs := map[string]bool{}
	refs := map[string]string{}
	cases := make([]CorpusCase, 0, len(manifest.Cases))
	for i, mc := range manifest.Cases {
		caseID := mc.CaseID
		if caseID == "" {
			caseID = fmt.Sprintf("cases[%d]", i)
			problems = append(problems, fieldError(caseID, "case_id", "required"))
		} else if seenCaseIDs[caseID] {
			problems = append(problems, fieldError(caseID, "case_id", "duplicate"))
		}
		seenCaseIDs[caseID] = true

		goldRel, ok := validateReference(canonicalRoot, mc.Gold, "gold", caseID, refs, &problems)
		if !ok {
			continue
		}
		goldBytes, err := readRegularFile(datasetRoot, goldRel)
		if err != nil {
			problems = append(problems, fieldError(caseID, "gold", fmt.Sprintf("reading: %v", err)))
			continue
		}
		goldFile, err := gold.Parse(goldBytes)
		if err != nil {
			problems = append(problems, fieldError(caseID, "gold", fmt.Sprintf("parsing: %v", err)))
			continue
		}
		resolved, err := gold.Resolve(goldFile)
		if err != nil {
			problems = append(problems, fieldError(caseID, "gold", fmt.Sprintf("resolving: %v", err)))
			continue
		}

		cases = append(cases, CorpusCase{CaseID: caseID, Tags: mc.Tags, Gold: goldFile, Resolved: resolved})
	}

	if len(problems) > 0 {
		return nil, problems
	}
	return &CorpusDataset{Root: canonicalRoot, Manifest: manifest, Cases: cases}, nil
}
