package main

// analyze asks a real LLM to read gold-run's raw output side by side with
// the gold fixture's own source text and write a Markdown assessment of
// extraction quality. Unlike the benchmark's deterministic scorers
// (comparison.EvaluateFamily, gold.ScoreCoverage) -- which only exist for
// extract_metrics -- this works for every processor gold-run can select,
// because it doesn't need a hand-authored expected-answer key: the LLM is
// given the exact source prose (reconstructed from the fixture) and judges
// each processor's rows against it directly.
//
// It is a pure post-processing step over gold-run's own JSON stdout: run
// gold-run, save its output, then point analyze at both that file and the
// same --dataset/--case so it can pull the source text back out of the
// fixture. It never touches the database.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gold "github.com/chendingplano/deepdoc/benchmark/doc-processors/gold/display-module-v1"
	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/joho/godotenv"
)

type goldRunOutput struct {
	DryRun  bool                    `json:"dry_run"`
	Results []goldRunDocumentResult `json:"results"`
}

type goldRunDocumentResult struct {
	Document string         `json:"document"`
	RecordID int64          `json:"record_id"`
	RunError string         `json:"run_error,omitempty"`
	Results  map[string]any `json:"results,omitempty"`
}

func runGoldAnalyze(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(stderr)
	datasetRoot := fs.String("dataset", "", "corpus dataset root (contains manifest.json)")
	caseID := fs.String("case", "", "corpus case id within the dataset")
	resultsPath := fs.String("results", "", "path to gold-run's JSON stdout output")
	modelRef := fs.String("model", envOr("ANALYZE_BENCHMARK_MODEL_NAME", "deepseek-flash-chen"), "model ref (key in MODEL_DEF_FILE / .models.toml)")
	promptRef := fs.String("prompt", envOr("ANALYZE_BENCHMARK_RESULTS_PROMPT", "prompt-analyze-benchmark-results-v1.md"), "prompt file name or path")
	output := fs.String("output", "", "output markdown file (stdout when omitted)")
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
	corpusCase := findCorpusCase(ds, *caseID)
	if corpusCase == nil {
		return fmt.Errorf("%w: case %q not found in dataset %s", errUsage, *caseID, *datasetRoot)
	}

	raw, err := os.ReadFile(*resultsPath)
	if err != nil {
		return fmt.Errorf("read --results: %w", err)
	}
	var run goldRunOutput
	if err := json.Unmarshal(raw, &run); err != nil {
		return fmt.Errorf("%w: parse --results as gold-run JSON: %v", errUsage, err)
	}
	if len(run.Results) == 0 {
		return fmt.Errorf("%w: --results has no documents to analyze", errUsage)
	}

	inputText, err := buildAnalysisInput(*datasetRoot, *caseID, corpusCase.Gold, run)
	if err != nil {
		return err
	}

	_ = godotenv.Load()
	promptText, promptRefOut, _, err := docprocessing.LoadPromptByRef(*promptRef)
	if err != nil {
		return fmt.Errorf("load prompt %q: %w", *promptRef, err)
	}

	client, modelName, err := docprocessing.BuildReviewerLLMClient(*modelRef)
	if err != nil {
		return fmt.Errorf("build LLM client for model %q: %w", *modelRef, err)
	}

	in := docprocessing.NewLLMJSONInput(ctx, promptRefOut, promptText, modelName, inputText, "gold-run corpus analysis", "CWB_DOC_BENCHMARK/analyze")
	resp, err := client.ExtractJSON(ctx, in)
	if err != nil {
		return fmt.Errorf("analysis LLM call: %w", err)
	}
	report, ok := resp["report_markdown"].(string)
	if !ok || strings.TrimSpace(report) == "" {
		return fmt.Errorf("analysis response missing non-empty %q field: %v", "report_markdown", resp)
	}

	if *output == "" {
		_, err = io.WriteString(stdout, report)
		return err
	}
	if dir := filepath.Dir(*output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(*output, []byte(report), 0o644)
}

// buildAnalysisInput reconstructs each document's exact source text from the
// gold fixture's clauses (in fixture order) and pairs it with that
// document's raw processor results from the gold-run output, in one text
// block per document.
func buildAnalysisInput(datasetRoot, caseID string, f gold.File, run goldRunOutput) (string, error) {
	docsByID := map[string]struct{ Family, Title string }{}
	for _, d := range f.AuthorityDocument {
		docsByID[d.ID] = struct{ Family, Title string }{d.Family, d.Title}
	}
	sourceByDoc := map[string][]string{}
	for _, c := range f.Clause {
		sourceByDoc[c.Document] = append(sourceByDoc[c.Document], c.TextTemplate)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Dataset: %s\nCase: %s\n\n", datasetRoot, caseID)
	for _, entry := range run.Results {
		meta := docsByID[entry.Document]
		fmt.Fprintf(&b, "=== Document %s (%s) — %s ===\n", entry.Document, meta.Family, meta.Title)
		if entry.RunError != "" {
			fmt.Fprintf(&b, "RUN ERROR: %s\n\n", entry.RunError)
			continue
		}
		b.WriteString("Source text:\n")
		for _, line := range sourceByDoc[entry.Document] {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\nProcessor results (JSON):\n")
		resultsJSON, err := json.MarshalIndent(entry.Results, "", "  ")
		if err != nil {
			return "", fmt.Errorf("document %s: marshal results: %w", entry.Document, err)
		}
		b.Write(resultsJSON)
		b.WriteString("\n\n")
	}
	return b.String(), nil
}
