package main

// gold-run is a deliberately narrow validation/execution tool: it takes a
// CorpusDataset case (server/api/doc-benchmark/corpus_dataset.go), generates
// its documents (benchmark/doc-processors/gold), inserts one kb.inputs row
// and line file per document, and runs them through the real, production
// doc-processing runtime (docprocessing.NewProductionRuntime) -- the same
// in-process invocation the benchmark's own experiment runner uses
// (server/api/doc-benchmark/application.go), without any of that runner's
// experiment/variant/case-run bookkeeping. It exists to answer one question:
// does real doc-processor output exist for a generated document, and what
// does it look like -- as a step before building a full corpus-level
// experiment case kind.
//
// --processors selects which doc processor(s) to run against every document
// in the case (comma-separated, or "all" for every processor known to
// docprocessing.ProductionRuntime). Results are fetched generically per
// processor from its own output table (see processorResultTables) -- there
// is no metrics-specific scoring here; that lives in the benchmark's
// verdict/coverage scorers (comparison, gold's ScoreCoverage) and in the
// separate `analyze` command for processors that don't have one.
//
// --dry-run stops after generating documents, rendering, and writing line
// files + kb.inputs rows -- no LLM call, no RunEvent -- so the plumbing can
// be verified for free before the one paid step.

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/rendering"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func runGoldCase(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gold-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	datasetRoot := fs.String("dataset", "", "corpus dataset root (contains manifest.json)")
	caseID := fs.String("case", "", "corpus case id within the dataset")
	documentKey := fs.String("document", "", "if set, run only this document key; otherwise run every document in the case")
	artifactRoot := fs.String("artifact-root", os.Getenv("ARTIFACT_DIR"), "production artifact root")
	artifactWebRoot := fs.String("artifact-web-root", os.Getenv("ARTIFACT_WEB_DIR"), "production artifact web root (optional)")
	processorsFlag := fs.String("processors", "", "comma-separated doc processor names to run (e.g. extract_metrics,extract_provisions), or \"all\" for every known processor")
	dryRun := fs.Bool("dry-run", false, "generate documents and write kb.inputs rows, but stop before invoking the real pipeline")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if *datasetRoot == "" || *caseID == "" || *artifactRoot == "" {
		return fmt.Errorf("%w: --dataset, --case, and --artifact-root (or ARTIFACT_DIR) are required", errUsage)
	}
	var processors []string
	if !*dryRun {
		var perr error
		processors, perr = resolveProcessorSelection(*processorsFlag)
		if perr != nil {
			return fmt.Errorf("%w: %v", errUsage, perr)
		}
	}
	if err := os.Setenv("ARTIFACT_DIR", filepath.Clean(*artifactRoot)); err != nil {
		return fmt.Errorf("set ARTIFACT_DIR: %w", err)
	}
	if *artifactWebRoot != "" {
		if err := os.Setenv("ARTIFACT_WEB_DIR", filepath.Clean(*artifactWebRoot)); err != nil {
			return fmt.Errorf("set ARTIFACT_WEB_DIR: %w", err)
		}
	}

	ds, err := docbenchmark.LoadCorpusDataset(*datasetRoot)
	if err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	corpusCase := findCorpusCase(ds, *caseID)
	if corpusCase == nil {
		return fmt.Errorf("%w: case %q not found in dataset %s", errUsage, *caseID, *datasetRoot)
	}

	docs := corpusCase.Documents()
	if *documentKey != "" {
		var filtered []model.Document
		for _, d := range docs {
			if d.Key == *documentKey {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("%w: document %q not found in case %q", errUsage, *documentKey, *caseID)
		}
		docs = filtered
	}

	typstBin, err := exec.LookPath("typst")
	if err != nil {
		return fmt.Errorf("typst not found on PATH: %w", err)
	}

	db, err := bootstrap(ctx, common.config)
	if err != nil {
		return err
	}

	var runtime *docprocessing.ProductionRuntime
	if !*dryRun {
		runtime, err = docprocessing.NewProductionRuntime(docprocessing.ProductionRuntimeOptions{
			RequiredProcessors: processors,
		})
		if err != nil {
			return fmt.Errorf("build production runtime: %w", err)
		}
	}

	results := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		recordID, linePath, lineCount, err := prepareGoldInput(ctx, db, *artifactRoot, typstBin, doc)
		if err != nil {
			return fmt.Errorf("document %s: prepare: %w", doc.Key, err)
		}
		entry := map[string]any{
			"document":    doc.Key,
			"record_id":   recordID,
			"line_file":   linePath,
			"line_count":  lineCount,
			"block_count": len(doc.Blocks),
		}
		if *dryRun {
			results = append(results, entry)
			continue
		}

		// filename is intentionally omitted: leaving it empty makes
		// ResolveInputFilePath derive the path from result_filename the same
		// way it would for an uploaded PDF, landing on the exact linePath
		// prepareGoldInput already wrote (see its doc comment).
		payload, err := json.Marshal(map[string]any{"record_id": recordID, "force": true})
		if err != nil {
			return fmt.Errorf("document %s: marshal payload: %w", doc.Key, err)
		}
		if err := runtime.Control.RunEvent(ctx, payload); err != nil {
			entry["run_error"] = err.Error()
			results = append(results, entry)
			continue
		}
		processorResults := make(map[string]any, len(processors))
		for _, proc := range processors {
			rows, ferr := fetchProcessorResults(ctx, db, proc, recordID)
			if ferr != nil {
				return fmt.Errorf("document %s: fetch %s results: %w", doc.Key, proc, ferr)
			}
			if rows == nil {
				processorResults[proc] = "not applicable: no per-record result table registered for this processor"
				continue
			}
			processorResults[proc] = rows
		}
		entry["results"] = processorResults
		results = append(results, entry)
	}

	return json.NewEncoder(stdout).Encode(map[string]any{"dry_run": *dryRun, "results": results})
}

// prepareGoldInput renders doc through the real Typst pipeline exactly as
// gold's own TestGroundingRoundTrip does, then places its line file (plus
// the intermediate Typst source, kept alongside for inspection) at the same
// bucketed artifact path every doc-processing artifact uses:
// <artifactRoot>/<recordID/1000>/<recordID>/<stagingFilename>_<parserName>.<ext>
// -- the identical formula repeated across docprocessing (e.g.
// topic_chunking_shared.go's buildRecordArtifactDir, extract-metrics.go).
// Matching it exactly means a CDM-origin record's artifact directory is
// indistinguishable in layout from a PDF-origin one: same bucket, same
// naming, chunks/metrics/line-file all siblings.
//
// This requires the record to exist first (the bucket path is keyed by its
// id), so the sequence is: insert a placeholder row, compute the bucket
// path from the returned id, write the line file there, then update
// result_filename to point at it -- mirroring how a real ingestion pipeline
// only learns its parsed output path after the kb.inputs row exists.
//
// It returns the new record's id, the line file's absolute path (which the
// caller must NOT also pass as the RunEvent payload's "filename" -- leaving
// that empty lets ResolveInputFilePath's standard derivation land on this
// same path, the same way it would for an uploaded PDF), and the number of
// lines written.
func prepareGoldInput(ctx context.Context, db *sql.DB, artifactRoot, typstBin string, doc model.Document) (int64, string, int, error) {
	r := &rendering.TypstRenderer{}
	src, err := r.RenderDocument(&doc)
	if err != nil {
		return 0, "", 0, fmt.Errorf("render: %w", err)
	}

	var recordID int64
	const insertStmt = `
INSERT INTO kb.inputs (type, staging_filename, title, parser_name, result_filename, status)
VALUES ('cdm', $1, $2, 'gold-run', 'pending', '[]'::jsonb)
RETURNING id`
	if err := db.QueryRowContext(ctx, insertStmt, doc.Key, doc.Title).Scan(&recordID); err != nil {
		return 0, "", 0, fmt.Errorf("insert kb.inputs: %w", err)
	}

	groupID := recordID / 1000 // matches docprocessing's buildRecordArtifactDir exactly
	targetDir := filepath.Join(artifactRoot, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return 0, "", 0, fmt.Errorf("mkdir target dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(targetDir, "theme.typ"), rendering.DefaultTheme, 0o644); err != nil {
		return 0, "", 0, fmt.Errorf("write theme.typ: %w", err)
	}
	typPath := filepath.Join(targetDir, doc.Key+"_gold-run.typ")
	if err := os.WriteFile(typPath, src, 0o644); err != nil {
		return 0, "", 0, fmt.Errorf("write doc.typ: %w", err)
	}

	marks, err := rendering.ExtractAnchors(typstBin, typPath)
	if err != nil {
		return 0, "", 0, fmt.Errorf("extract anchors: %w", err)
	}
	fragments, err := rendering.DeriveFragments(marks)
	if err != nil {
		return 0, "", 0, fmt.Errorf("derive fragments: %w", err)
	}
	fragsByUnit := map[string][]rendering.Fragment{}
	for _, f := range fragments {
		fragsByUnit[f.UnitID] = append(fragsByUnit[f.UnitID], f)
	}
	units := rendering.CollectUnits(&doc)
	lineContent, lineUnitIDs, err := rendering.GenerateLineFile(units, fragsByUnit)
	if err != nil {
		return 0, "", 0, fmt.Errorf("generate line file: %w", err)
	}

	linePath := filepath.Join(targetDir, doc.Key+"_gold-run.txt")
	if err := os.WriteFile(linePath, []byte(lineContent), 0o644); err != nil {
		return 0, "", 0, fmt.Errorf("write line file: %w", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE kb.inputs SET result_filename=$1 WHERE id=$2`, linePath, recordID); err != nil {
		return 0, "", 0, fmt.Errorf("update result_filename: %w", err)
	}

	return recordID, linePath, len(lineUnitIDs), nil
}

// findCorpusCase returns the case with the given ID, or nil if absent.
// Shared by gold-run and analyze so both look up cases identically.
func findCorpusCase(ds *docbenchmark.CorpusDataset, caseID string) *docbenchmark.CorpusCase {
	for i := range ds.Cases {
		if ds.Cases[i].CaseID == caseID {
			return &ds.Cases[i]
		}
	}
	return nil
}

// allGoldProcessors is docprocessing's productionOrder (runtime.go) minus the
// always-mandatory static_analyzer/chunking pair, which resolveRequiredProcessors
// forces on regardless of what is requested.
var allGoldProcessors = []string{
	"generate_summaries", "generate_topics", "extract_doc_metadata",
	"extract_semantic_projections", "extract_structured_knowledge",
	"extract_entity", "extract_relation", "extract_inventory_items",
	"extract_metrics", "extract_provisions", "generate_scene_blocks",
}

// resolveProcessorSelection parses --processors: a comma-separated list of
// names from allGoldProcessors, or the literal "all".
func resolveProcessorSelection(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("--processors is required (comma-separated names, or \"all\"); known: %s", strings.Join(allGoldProcessors, ", "))
	}
	if raw == "all" {
		return append([]string(nil), allGoldProcessors...), nil
	}
	known := map[string]bool{}
	for _, p := range allGoldProcessors {
		known[p] = true
	}
	seen := map[string]bool{}
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !known[name] {
			return nil, fmt.Errorf("unknown processor %q; known: %s", name, strings.Join(allGoldProcessors, ", "))
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--processors resolved to no processors")
	}
	return out, nil
}

// processorResultTables maps a processor name to the table (and its
// input_record_id-equivalent foreign key column) that holds its per-document
// output, so gold-run can report real results without any processor-specific
// scoring logic. extract_doc_metadata writes directly onto kb.inputs rather
// than a child table, so it's registered against kb.inputs/id. generate_summaries
// and generate_topics are intentionally absent: their output is chunk-derived
// artifacts, not a queryable per-record row set, and reporting on them belongs
// to `analyze`, not this generic fetch.
var processorResultTables = map[string]struct{ table, fkCol string }{
	"extract_metrics":              {"kb.metrics", "input_record_id"},
	"extract_provisions":           {"kb.provisions", "input_record_id"},
	"extract_entity":               {"kb.entities", "input_record_id"},
	"extract_relation":             {"kb.relations", "input_record_id"},
	"extract_inventory_items":      {"kb.inventory_items", "input_record_id"},
	"extract_semantic_projections": {"kb.semantic_projections", "input_record_id"},
	"extract_structured_knowledge": {"kb.knowledges", "input_record_id"},
	"generate_scene_blocks":        {"kb.scene_objects", "input_record_id"},
	"extract_doc_metadata":         {"kb.inputs", "id"},
}

// fetchProcessorResults returns every row a processor wrote for recordID, as
// generic column-name-keyed maps -- nil (not an error) when the processor has
// no registered result table.
func fetchProcessorResults(ctx context.Context, db *sql.DB, processor string, recordID int64) ([]map[string]any, error) {
	spec, ok := processorResultTables[processor]
	if !ok {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT * FROM %s WHERE %s = $1 ORDER BY id`, spec.table, spec.fkCol)
	rows, err := db.QueryContext(ctx, query, recordID)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", spec.table, err)
	}
	defer rows.Close()
	return scanRowsAsMaps(rows)
}

// scanRowsAsMaps scans every remaining row into a column-name-keyed map,
// coercing driver byte slices (text/jsonb come back as []byte) to string so
// the result marshals to JSON cleanly regardless of the table's schema.
func scanRowsAsMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
