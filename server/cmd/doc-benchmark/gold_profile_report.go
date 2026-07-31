package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
)

func runGoldProfileReport(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("profile-report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	datasetRoot := fs.String("dataset", "", "corpus dataset root (contains manifest.json)")
	caseID := fs.String("case", "", "corpus case id within the dataset")
	resultsPath := fs.String("results", "", "path to schema-v2 gold-run JSON")
	format := fs.String("format", "json", "json or markdown")
	output := fs.String("output", "", "output file (stdout when omitted)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if *datasetRoot == "" || *caseID == "" || *resultsPath == "" {
		return fmt.Errorf("%w: --dataset, --case, and --results are required", errUsage)
	}

	ds, err := docbenchmark.LoadCorpusDataset(*datasetRoot)
	if err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	raw, err := os.ReadFile(*resultsPath)
	if err != nil {
		return fmt.Errorf("%w: read --results: %v", errUsage, err)
	}
	run, err := ParseGoldRunEnvelope(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	report, err := docbenchmark.BuildProfileReport(ds, *caseID, run)
	if err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}

	var rendered []byte
	switch *format {
	case "json":
		rendered, err = docbenchmark.RenderProfileReportJSON(report)
	case "markdown", "md":
		rendered = []byte(docbenchmark.RenderProfileReportMarkdown(report))
	default:
		return fmt.Errorf("%w: --format must be json or markdown", errUsage)
	}
	if err != nil {
		return err
	}
	return writeCommandOutput(*output, rendered, stdout)
}

func ParseGoldRunEnvelope(raw []byte) (docbenchmark.GoldRunEnvelope, error) {
	type strictDatasetIdentity struct {
		ID          string `json:"id"`
		Version     string `json:"version"`
		ContentHash string `json:"content_hash"`
	}
	type strictProcessorResult struct {
		State string            `json:"state"`
		Rows  *[]map[string]any `json:"rows,omitempty"`
	}
	type strictDocumentResult struct {
		Document string                           `json:"document"`
		RecordID int64                            `json:"record_id"`
		RunError string                           `json:"run_error,omitempty"`
		Results  map[string]strictProcessorResult `json:"results,omitempty"`
	}
	type strictEnvelope struct {
		SchemaVersion      int                    `json:"schema_version"`
		Dataset            strictDatasetIdentity  `json:"dataset"`
		CaseID             string                 `json:"case_id"`
		SelectedProcessors []string               `json:"selected_processors"`
		DryRun             bool                   `json:"dry_run"`
		Results            []strictDocumentResult `json:"results"`
	}

	var decoded strictEnvelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		return docbenchmark.GoldRunEnvelope{}, fmt.Errorf("parse gold-run JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return docbenchmark.GoldRunEnvelope{}, fmt.Errorf("parse gold-run JSON: multiple JSON values are not allowed")
		}
		return docbenchmark.GoldRunEnvelope{}, fmt.Errorf("parse gold-run JSON: %w", err)
	}
	if decoded.SchemaVersion == 0 {
		return docbenchmark.GoldRunEnvelope{}, fmt.Errorf("schema_version: required")
	}

	out := docbenchmark.GoldRunEnvelope{
		SchemaVersion: decoded.SchemaVersion,
		Dataset: docbenchmark.GoldRunDatasetIdentity{
			ID:          decoded.Dataset.ID,
			Version:     decoded.Dataset.Version,
			ContentHash: decoded.Dataset.ContentHash,
		},
		CaseID:             decoded.CaseID,
		SelectedProcessors: append([]string(nil), decoded.SelectedProcessors...),
		DryRun:             decoded.DryRun,
		Results:            make([]docbenchmark.GoldRunDocumentResult, 0, len(decoded.Results)),
	}
	for _, result := range decoded.Results {
		if result.RunError != "" && result.Results != nil {
			return docbenchmark.GoldRunEnvelope{}, fmt.Errorf("results[%s]: run_error must not include processor results", result.Document)
		}
		outResult := docbenchmark.GoldRunDocumentResult{
			Document: result.Document,
			RecordID: result.RecordID,
			RunError: result.RunError,
		}
		if result.Results != nil {
			outResult.Results = make(map[string]docbenchmark.GoldRunProcessorResult, len(result.Results))
			for processor, procResult := range result.Results {
				switch procResult.State {
				case docbenchmark.GoldRunResultRows:
					if procResult.Rows == nil {
						return docbenchmark.GoldRunEnvelope{}, fmt.Errorf("results[%s].results[%s]: rows state requires rows field", result.Document, processor)
					}
				case docbenchmark.GoldRunResultNotRegistered:
					if procResult.Rows != nil {
						return docbenchmark.GoldRunEnvelope{}, fmt.Errorf("results[%s].results[%s]: not_registered state must omit rows field", result.Document, processor)
					}
				default:
					return docbenchmark.GoldRunEnvelope{}, fmt.Errorf("results[%s].results[%s]: unknown processor-result state %q", result.Document, processor, procResult.State)
				}
				outResult.Results[processor] = docbenchmark.GoldRunProcessorResult{
					State: procResult.State,
					Rows:  procResult.Rows,
				}
			}
		}
		out.Results = append(out.Results, outResult)
	}
	return out, nil
}

func writeCommandOutput(output string, raw []byte, stdout io.Writer) error {
	if output == "" {
		_, err := stdout.Write(raw)
		return err
	}
	if dir := filepath.Dir(output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(output, raw, 0o644)
}
