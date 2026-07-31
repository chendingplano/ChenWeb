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
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gold "github.com/chendingplano/deepdoc/benchmark/doc-processors/gold/display-module-v1"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

type CorpusManifest struct {
	SchemaVersion  int                  `json:"schema_version"`
	DatasetID      string               `json:"dataset_id"`
	DatasetVersion string               `json:"dataset_version"`
	Cases          []CorpusManifestCase `json:"cases"`
}

type ProcessorApplicability string

const (
	ProcessorRequired    ProcessorApplicability = "required"
	ProcessorUseful      ProcessorApplicability = "useful"
	ProcessorNotRequired ProcessorApplicability = "not_required"
)

type DocumentProfile struct {
	StoreProfile       string                            `json:"store_profile"`
	DocumentKind       string                            `json:"document_kind"`
	ExpectedProcessors map[string]ProcessorApplicability `json:"expected_processors"`

	expectedProcessorsPresent bool
}

type CorpusManifestCase struct {
	CaseID           string                     `json:"case_id"`
	Gold             string                     `json:"gold"`
	Tags             []string                   `json:"tags,omitempty"`
	DocumentProfiles map[string]DocumentProfile `json:"document_profiles"`
}

// CorpusCase is one loaded, validated corpus-level case: a gold ontology
// fixture (ADR 2026072901 DR12) resolved into generated CDM documents and an
// expected verdict matrix.
type CorpusCase struct {
	CaseID string
	Tags   []string

	Gold             gold.File
	Resolved         *gold.Resolved
	DocumentProfiles map[string]DocumentProfile
	ContentHash      string
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
	if err := rejectDuplicateJSONKeys(manifestBytes); err != nil {
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
	canonicalManifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("manifest.json: canonical JSON: %w", err)
	}
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
		documentProfiles := validateCorpusDocumentProfiles(caseID, mc.DocumentProfiles, gold.BuildDocuments(goldFile), &problems)
		contentHash, err := corpusCaseContentHash(canonicalManifestBytes, goldBytes)
		if err != nil {
			problems = append(problems, fieldError(caseID, "content_hash", err.Error()))
			continue
		}

		cases = append(cases, CorpusCase{
			CaseID:           caseID,
			Tags:             mc.Tags,
			Gold:             goldFile,
			Resolved:         resolved,
			DocumentProfiles: documentProfiles,
			ContentHash:      contentHash,
		})
	}

	if len(problems) > 0 {
		return nil, problems
	}
	return &CorpusDataset{Root: canonicalRoot, Manifest: manifest, Cases: cases}, nil
}

func (p *DocumentProfile) UnmarshalJSON(raw []byte) error {
	type alias DocumentProfile
	aux := struct {
		ExpectedProcessors *map[string]ProcessorApplicability `json:"expected_processors"`
		*alias
	}{alias: (*alias)(p)}
	if err := decodeStrict(raw, &aux); err != nil {
		return err
	}
	p.expectedProcessorsPresent = aux.ExpectedProcessors != nil
	if aux.ExpectedProcessors != nil {
		p.ExpectedProcessors = *aux.ExpectedProcessors
	}
	return nil
}

func validateCorpusDocumentProfiles(caseID string, raw map[string]DocumentProfile, generated []model.Document, out *validationErrors) map[string]DocumentProfile {
	generatedKeys := make(map[string]struct{}, len(generated))
	for _, doc := range generated {
		generatedKeys[doc.Key] = struct{}{}
	}

	normalizedProfiles := make(map[string]DocumentProfile, len(raw))
	normalizedDocs := make(map[string]string, len(raw))
	union := map[string]struct{}{}

	for rawDocID, profile := range raw {
		docID := strings.TrimSpace(rawDocID)
		field := fmt.Sprintf("document_profiles[%s]", docID)
		if rawDocID != docID {
			*out = append(*out, fieldError(caseID, field, "document id must be canonical"))
		}
		if previous, exists := normalizedDocs[docID]; exists && previous != rawDocID {
			*out = append(*out, fieldError(caseID, field, "duplicate normalized document id"))
			continue
		}
		normalizedDocs[docID] = rawDocID
		if _, ok := generatedKeys[docID]; !ok {
			*out = append(*out, fieldError(caseID, field, "document is not generated"))
		}
		validateCanonicalNonblankProfileText(caseID, field+".store_profile", profile.StoreProfile, out)
		validateCanonicalNonblankProfileText(caseID, field+".document_kind", profile.DocumentKind, out)
		if !profile.expectedProcessorsPresent {
			*out = append(*out, fieldError(caseID, field+".expected_processors", "required"))
		} else if len(profile.ExpectedProcessors) == 0 {
			*out = append(*out, fieldError(caseID, field+".expected_processors", "must not be empty"))
		}

		normalizedExpected := make(map[string]ProcessorApplicability, len(profile.ExpectedProcessors))
		normalizedProcessors := make(map[string]string, len(profile.ExpectedProcessors))
		for rawProcessor, applicability := range profile.ExpectedProcessors {
			processorID := strings.TrimSpace(rawProcessor)
			procField := fmt.Sprintf("%s.expected_processors[%s]", field, processorID)
			if rawProcessor != processorID {
				*out = append(*out, fieldError(caseID, procField, "processor name must be canonical"))
			}
			canonicalProcessor, ok := docprocessing.CanonicalOptionalProductionProcessor(rawProcessor)
			if !ok {
				*out = append(*out, fieldError(caseID, procField, "unknown processor"))
				continue
			}
			if canonicalProcessor != processorID {
				*out = append(*out, fieldError(caseID, procField, "processor name must be canonical"))
				continue
			}
			if previous, exists := normalizedProcessors[canonicalProcessor]; exists && previous != rawProcessor {
				*out = append(*out, fieldError(caseID, fmt.Sprintf("%s.expected_processors[%s]", field, canonicalProcessor), "duplicate normalized processor id"))
				continue
			}
			normalizedProcessors[canonicalProcessor] = rawProcessor
			switch applicability {
			case ProcessorRequired, ProcessorUseful, ProcessorNotRequired:
			default:
				*out = append(*out, fieldError(caseID, procField, "unknown applicability"))
				continue
			}
			normalizedExpected[canonicalProcessor] = applicability
			union[canonicalProcessor] = struct{}{}
		}
		profile.ExpectedProcessors = normalizedExpected
		normalizedProfiles[docID] = profile
	}

	generatedIDs := make([]string, 0, len(generatedKeys))
	for docID := range generatedKeys {
		generatedIDs = append(generatedIDs, docID)
	}
	sort.Strings(generatedIDs)
	for _, docID := range generatedIDs {
		if _, ok := normalizedProfiles[docID]; !ok {
			*out = append(*out, fieldError(caseID, fmt.Sprintf("document_profiles[%s]", docID), "required"))
		}
	}

	unionKeys := make([]string, 0, len(union))
	for name := range union {
		unionKeys = append(unionKeys, name)
	}
	sort.Strings(unionKeys)
	for _, docID := range generatedIDs {
		profile, ok := normalizedProfiles[docID]
		if !ok {
			continue
		}
		for _, processor := range unionKeys {
			if _, ok := profile.ExpectedProcessors[processor]; !ok {
				*out = append(*out, fieldError(caseID, fmt.Sprintf("document_profiles[%s].expected_processors[%s]", docID, processor), "required"))
			}
		}
	}

	return normalizedProfiles
}

func validateCanonicalNonblankProfileText(caseID, field, raw string, out *validationErrors) {
	switch {
	case raw == "":
		*out = append(*out, fieldError(caseID, field, "required"))
	case strings.TrimSpace(raw) == "":
		*out = append(*out, fieldError(caseID, field, "must be canonical nonblank text"))
	}
}

func rejectDuplicateJSONKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := checkJSONValue(dec, ""); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		return fmt.Errorf("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func checkJSONValue(dec *json.Decoder, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			keyPath := joinJSONPath(path, key)
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON key at %s", keyPath)
			}
			seen[key] = struct{}{}
			if err := checkJSONValue(dec, keyPath); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("malformed JSON object")
		}
	case '[':
		for i := 0; dec.More(); i++ {
			if err := checkJSONValue(dec, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("malformed JSON array")
		}
	}
	return nil
}

func joinJSONPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func corpusCaseContentHash(canonicalManifest, goldBytes []byte) (string, error) {
	hasher := sha256.New()
	for _, frame := range [][]byte{[]byte("chenweb-corpus-case-v1\n"), canonicalManifest, goldBytes} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(frame)))
		if _, err := hasher.Write(length[:]); err != nil {
			return "", err
		}
		if _, err := hasher.Write(frame); err != nil {
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}
