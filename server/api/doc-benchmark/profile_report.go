package docbenchmark

const (
	GoldRunSchemaVersion       = 2
	GoldRunResultRows          = "rows"
	GoldRunResultNotRegistered = "not_registered"
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
