package docbenchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

const (
	GoldRunSchemaVersion       = 2
	GoldRunResultRows          = "rows"
	GoldRunResultNotRegistered = "not_registered"

	ProfileReportSchemaVersion = 1
	ProfileEvidenceKind        = "structural_yield"
)

type GoldRunDatasetIdentity struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	ContentHash string `json:"content_hash"`
}

type GoldRunProcessorResult struct {
	State string            `json:"state"`
	Rows  *[]map[string]any `json:"rows,omitempty"`
}

type GoldRunDocumentResult struct {
	Document string                            `json:"document"`
	RecordID int64                             `json:"record_id"`
	RunError string                            `json:"run_error,omitempty"`
	Results  map[string]GoldRunProcessorResult `json:"results,omitempty"`
}

type GoldRunEnvelope struct {
	SchemaVersion      int                     `json:"schema_version"`
	Dataset            GoldRunDatasetIdentity  `json:"dataset"`
	CaseID             string                  `json:"case_id"`
	SelectedProcessors []string                `json:"selected_processors"`
	DryRun             bool                    `json:"dry_run"`
	Results            []GoldRunDocumentResult `json:"results"`
}

type ProfileReportRow struct {
	StoreProfile        string `json:"store_profile"`
	DocumentKind        string `json:"document_kind"`
	Processor           string `json:"processor"`
	Documents           int    `json:"documents"`
	SuccessfulDocuments int    `json:"successful_documents"`
	FailedDocuments     int    `json:"failed_documents"`
	DocumentsWithOutput int    `json:"documents_with_output"`
	OutputRows          int    `json:"output_rows"`
	NotRegistered       int    `json:"not_registered"`
	Applicability       string `json:"applicability"`
	EvidenceKind        string `json:"evidence_kind"`
	Assessment          string `json:"assessment"`
}

type ProfileReport struct {
	SchemaVersion      int                    `json:"schema_version"`
	Dataset            GoldRunDatasetIdentity `json:"dataset"`
	CaseID             string                 `json:"case_id"`
	SelectedProcessors []string               `json:"selected_processors"`
	Rows               []ProfileReportRow     `json:"rows"`
}

func BuildProfileReport(ds *CorpusDataset, caseID string, run GoldRunEnvelope) (ProfileReport, error) {
	corpusCase := findProfileReportCase(ds, caseID)
	if corpusCase == nil {
		return ProfileReport{}, fmt.Errorf("case %q not found", caseID)
	}
	if run.SchemaVersion != GoldRunSchemaVersion {
		return ProfileReport{}, fmt.Errorf("schema_version: unsupported value %d", run.SchemaVersion)
	}
	if run.DryRun {
		return ProfileReport{}, fmt.Errorf("dry_run: profile evidence requires dry_run=false")
	}
	if run.Dataset.ID != ds.Manifest.DatasetID {
		return ProfileReport{}, fmt.Errorf("dataset.id: got %q, want %q", run.Dataset.ID, ds.Manifest.DatasetID)
	}
	if run.Dataset.Version != ds.Manifest.DatasetVersion {
		return ProfileReport{}, fmt.Errorf("dataset.version: got %q, want %q", run.Dataset.Version, ds.Manifest.DatasetVersion)
	}
	if run.Dataset.ContentHash != corpusCase.ContentHash {
		return ProfileReport{}, fmt.Errorf("dataset.content_hash: got %q, want %q", run.Dataset.ContentHash, corpusCase.ContentHash)
	}
	if run.CaseID != corpusCase.CaseID {
		return ProfileReport{}, fmt.Errorf("case_id: got %q, want %q", run.CaseID, corpusCase.CaseID)
	}

	selectedSet, err := validateSelectedProcessors(run.SelectedProcessors)
	if err != nil {
		return ProfileReport{}, err
	}

	documents := corpusCase.Documents()
	docKeys := make([]string, 0, len(documents))
	docIndex := make(map[string]struct{}, len(documents))
	for _, doc := range documents {
		docKeys = append(docKeys, doc.Key)
		docIndex[doc.Key] = struct{}{}
	}
	sort.Strings(docKeys)

	resultByDocument := make(map[string]GoldRunDocumentResult, len(run.Results))
	for _, result := range run.Results {
		if _, ok := docIndex[result.Document]; !ok {
			return ProfileReport{}, fmt.Errorf("results[%s]: unknown document", result.Document)
		}
		if _, exists := resultByDocument[result.Document]; exists {
			return ProfileReport{}, fmt.Errorf("results[%s]: duplicate result document", result.Document)
		}
		if result.RunError != "" && result.Results != nil {
			return ProfileReport{}, fmt.Errorf("results[%s]: run_error must not include processor results", result.Document)
		}
		if result.RunError == "" {
			for processor, procResult := range result.Results {
				if !selectedSet[processor] {
					return ProfileReport{}, fmt.Errorf("results[%s].results[%s]: processor not selected", result.Document, processor)
				}
				switch procResult.State {
				case GoldRunResultRows:
					if procResult.Rows == nil {
						return ProfileReport{}, fmt.Errorf("results[%s].results[%s]: rows state requires rows field", result.Document, processor)
					}
				case GoldRunResultNotRegistered:
					if procResult.Rows != nil {
						return ProfileReport{}, fmt.Errorf("results[%s].results[%s]: not_registered state must omit rows field", result.Document, processor)
					}
				default:
					return ProfileReport{}, fmt.Errorf("results[%s].results[%s]: unknown processor-result state %q", result.Document, processor, procResult.State)
				}
			}
		}
		resultByDocument[result.Document] = result
	}

	type aggregateKey struct {
		storeProfile string
		documentKind string
		processor    string
	}
	aggregates := map[aggregateKey]*ProfileReportRow{}

	for _, docKey := range docKeys {
		result, ok := resultByDocument[docKey]
		if !ok {
			return ProfileReport{}, fmt.Errorf("results[%s]: missing result document", docKey)
		}
		profile := corpusCase.DocumentProfiles[docKey]
		for _, processor := range run.SelectedProcessors {
			applicability := string(profile.ExpectedProcessors[processor])
			key := aggregateKey{storeProfile: profile.StoreProfile, documentKind: profile.DocumentKind, processor: processor}
			row := aggregates[key]
			if row == nil {
				row = &ProfileReportRow{
					StoreProfile:  profile.StoreProfile,
					DocumentKind:  profile.DocumentKind,
					Processor:     processor,
					Applicability: applicability,
					EvidenceKind:  ProfileEvidenceKind,
				}
				aggregates[key] = row
			} else if row.Applicability != applicability {
				return ProfileReport{}, fmt.Errorf("row[%s/%s/%s]: inconsistent applicability inside aggregate key", key.storeProfile, key.documentKind, key.processor)
			}
			row.Documents++
			if result.RunError != "" {
				row.FailedDocuments++
				continue
			}
			procResult, ok := result.Results[processor]
			if !ok {
				return ProfileReport{}, fmt.Errorf("results[%s].results[%s]: missing result for selected processor", docKey, processor)
			}
			row.SuccessfulDocuments++
			switch procResult.State {
			case GoldRunResultRows:
				rows := *procResult.Rows
				row.OutputRows += len(rows)
				if len(rows) > 0 {
					row.DocumentsWithOutput++
				}
			case GoldRunResultNotRegistered:
				row.NotRegistered++
			default:
				return ProfileReport{}, fmt.Errorf("results[%s].results[%s]: unknown processor-result state %q", docKey, processor, procResult.State)
			}
		}
	}

	rows := make([]ProfileReportRow, 0, len(aggregates))
	for _, row := range aggregates {
		row.Assessment = assessProfileReportRow(*row)
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].StoreProfile != rows[j].StoreProfile {
			return rows[i].StoreProfile < rows[j].StoreProfile
		}
		if rows[i].DocumentKind != rows[j].DocumentKind {
			return rows[i].DocumentKind < rows[j].DocumentKind
		}
		return rows[i].Processor < rows[j].Processor
	})

	return ProfileReport{
		SchemaVersion:      ProfileReportSchemaVersion,
		Dataset:            run.Dataset,
		CaseID:             run.CaseID,
		SelectedProcessors: append([]string{}, run.SelectedProcessors...),
		Rows:               rows,
	}, nil
}

func RenderProfileReportJSON(report ProfileReport) ([]byte, error) {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func RenderProfileReportMarkdown(report ProfileReport) string {
	var b bytes.Buffer
	b.WriteString("# Store-profile report\n\n")
	b.WriteString("## Provenance\n\n")
	b.WriteString("- dataset_id: " + mdEscape(report.Dataset.ID) + "\n")
	b.WriteString("- dataset_version: " + mdEscape(report.Dataset.Version) + "\n")
	b.WriteString("- content_hash: " + mdEscape(report.Dataset.ContentHash) + "\n")
	b.WriteString("- case_id: " + mdEscape(report.CaseID) + "\n\n")
	b.WriteString("## Selected processors\n\n")
	for _, processor := range report.SelectedProcessors {
		b.WriteString("- " + mdEscape(processor) + "\n")
	}
	b.WriteString("\n## Rows\n\n")
	b.WriteString("| Store profile | Document kind | Processor | Documents | Successful documents | Failed documents | Documents with output | Output rows | Not registered | Applicability | Evidence kind | Assessment |\n")
	b.WriteString("|---|---|---|---:|---:|---:|---:|---:|---:|---|---|---|\n")
	for _, row := range report.Rows {
		b.WriteString("| " + mdEscape(row.StoreProfile))
		b.WriteString(" | " + mdEscape(row.DocumentKind))
		b.WriteString(" | " + mdEscape(row.Processor))
		b.WriteString(fmt.Sprintf(" | %d | %d | %d | %d | %d | %d | %s | %s | %s |\n",
			row.Documents,
			row.SuccessfulDocuments,
			row.FailedDocuments,
			row.DocumentsWithOutput,
			row.OutputRows,
			row.NotRegistered,
			mdEscape(row.Applicability),
			mdEscape(row.EvidenceKind),
			mdEscape(row.Assessment),
		))
	}
	b.WriteString("\nStructural yield is not semantic correctness.\n")
	return b.String()
}

func assessProfileReportRow(row ProfileReportRow) string {
	switch row.Applicability {
	case string(ProcessorRequired):
		if row.FailedDocuments > 0 || row.NotRegistered > 0 || row.DocumentsWithOutput < row.Documents {
			return "required_failure"
		}
		return "required_output_observed"
	case string(ProcessorUseful):
		if row.FailedDocuments > 0 || row.DocumentsWithOutput == 0 {
			return "useful_review_warning"
		}
		return "useful_output_observed"
	case string(ProcessorNotRequired):
		return "informational_not_required"
	default:
		return "informational_not_required"
	}
}

func findProfileReportCase(ds *CorpusDataset, caseID string) *CorpusCase {
	if ds == nil {
		return nil
	}
	for i := range ds.Cases {
		if ds.Cases[i].CaseID == caseID {
			return &ds.Cases[i]
		}
	}
	return nil
}

func validateSelectedProcessors(selected []string) (map[string]bool, error) {
	known := OptionalProcessorSet()
	if len(selected) == 0 {
		return nil, fmt.Errorf("selected_processors: must not be empty")
	}
	selectedSet := make(map[string]bool, len(selected))
	lastIndex := -1
	for _, processor := range selected {
		index, ok := known[processor]
		if !ok {
			return nil, fmt.Errorf("selected_processors[%s]: unknown selected processor", processor)
		}
		if selectedSet[processor] {
			return nil, fmt.Errorf("selected_processors[%s]: duplicate selected processor", processor)
		}
		if index <= lastIndex {
			return nil, fmt.Errorf("selected_processors: selected processors out of canonical order")
		}
		lastIndex = index
		selectedSet[processor] = true
	}
	return selectedSet, nil
}

func OptionalProcessorSet() map[string]int {
	registry := docprocessing.OptionalProductionProcessorNames()
	out := make(map[string]int, len(registry))
	for i, processor := range registry {
		out[processor] = i
	}
	return out
}
