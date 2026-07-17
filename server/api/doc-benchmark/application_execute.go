package docbenchmark

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

type allocatedAttemptWorkspace struct{ allocation *WorkspaceAllocation }

func (w allocatedAttemptWorkspace) Path() string { return w.allocation.WorkPath }
func (w allocatedAttemptWorkspace) Capture(raw []byte, name string) (Artifact, error) {
	return w.allocation.Capture(bytes.NewReader(raw), name)
}
func (w allocatedAttemptWorkspace) Cleanup(options CleanupOptions) error {
	return w.allocation.Cleanup(options)
}
func (w allocatedAttemptWorkspace) Close() error { return w.allocation.Close() }

type caseExecutionState struct {
	workspace AttemptWorkspace
	seeded    SeededInput
	adapters  map[Processor]Adapter
}
type reconciledEvidence struct {
	bundle  EvidenceBundle
	actuals map[Processor]any
}

type VariantWorker struct {
	Application Application
	Experiment  *Experiment
	Run         PreparedRun
	Session     VariantSession
	initialized bool
	terminal    bool
}

func (w *VariantWorker) Initialize(ctx context.Context) error {
	if w.initialized {
		return nil
	}
	if w.Experiment == nil {
		return fmt.Errorf("variant worker: nil experiment")
	}
	var lifecycle string
	if err := w.Application.Config.DB.QueryRowContext(ctx, `SELECT lifecycle FROM kb.benchmark_runs WHERE id=$1`, w.Run.RunID).Scan(&lifecycle); err != nil {
		return err
	}
	if lifecycle == "succeeded" || lifecycle == "failed" || lifecycle == "canceled" {
		w.initialized, w.terminal = true, true
		return nil
	}
	session, err := w.Application.InitializeVariant(ctx, w.Run.Variant, w.Experiment.Processors)
	if err != nil {
		return err
	}
	store := SQLStore{DB: w.Application.Config.DB}
	if err = store.AttachResolvedRuntime(ctx, w.Run.RunID, session.ConfigJSON, session.ConfigHash); err != nil {
		return err
	}
	if err = store.MarkRunRunning(ctx, w.Run.RunID); err != nil {
		return err
	}
	w.Session, w.initialized = session, true
	return nil
}

func (w *VariantWorker) RunCases(ctx context.Context) error {
	if err := w.Initialize(ctx); err != nil {
		return err
	}
	if w.terminal {
		return nil
	}
	units := make([]CaseUnit, 0, len(w.Run.CaseRuns))
	for unit := range w.Run.CaseRuns {
		units = append(units, unit)
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].CaseID != units[j].CaseID {
			return units[i].CaseID < units[j].CaseID
		}
		return units[i].Repetition < units[j].Repetition
	})
	for _, unit := range units {
		if err := w.Application.ExecuteCase(ctx, w.Experiment, w.Run, unit, w.Session); err != nil {
			return err
		}
	}
	_, err := (SQLStore{DB: w.Application.Config.DB}).FinalizeRunIfComplete(ctx, w.Run.RunID)
	return err
}

func (a Application) ExecuteCase(ctx context.Context, experiment *Experiment, run PreparedRun, unit CaseUnit, session VariantSession) error {
	caseRunID, ok := run.CaseRuns[unit]
	if !ok {
		return fmt.Errorf("case unit %s/%d is not prepared", unit.CaseID, unit.Repetition)
	}
	datasetCase, processors := run.Cases[unit], run.Processors[unit]
	if experiment == nil || len(processors) == 0 {
		return fmt.Errorf("case unit has no experiment or applicable processors")
	}
	if err := a.verifyCaseFiles(experiment, datasetCase); err != nil {
		return err
	}
	captureProcessors := append([]Processor(nil), processors...)
	if containsProcessor(processors, ProcessorExtractMetrics) && !containsProcessor(captureProcessors, ProcessorChunking) {
		captureProcessors = append([]Processor{ProcessorChunking}, captureProcessors...)
	}
	state := &caseExecutionState{adapters: map[Processor]Adapter{}}
	store := SQLStore{DB: a.Config.DB}
	adapterFor := func(processor Processor, seeded SeededInput) Adapter {
		if adapter := state.adapters[processor]; adapter != nil {
			return adapter
		}
		var adapter Adapter
		if a.Config.AdapterFactory != nil {
			adapter = a.Config.AdapterFactory(processor, datasetCase, seeded)
		} else {
			path := func(extension string) func(int64) string {
				return func(id int64) string {
					value, _ := ProductionArtifactPath(a.Config.ArtifactRoot, id, seeded.StagingFilename, seeded.ParserName, extension)
					return value
				}
			}
			switch processor {
			case ProcessorChunking:
				adapter = ChunkAdapter{DB: a.Config.DB, ArtifactPath: path(".chunks"), SourceMaxLine: len(datasetCase.LineNumbers)}
			case ProcessorExtractMetrics:
				adapter = MetricAdapter{DB: a.Config.DB, ArtifactPath: path(".metrics")}
			}
		}
		state.adapters[processor] = adapter
		return adapter
	}
	work := AttemptWork{}
	work.Execute = func(ctx context.Context, attempt AttemptRecord) error {
		allocator := a.Config.AllocateWorkspace
		if allocator == nil {
			allocator = func(config WorkspaceConfig) (AttemptWorkspace, error) {
				allocation, err := AllocateWorkspace(config)
				if err != nil {
					return nil, err
				}
				return allocatedAttemptWorkspace{allocation}, nil
			}
		}
		workspace, err := allocator(WorkspaceConfig{WorkRoot: a.Config.WorkRoot, EvidenceRoot: a.Config.EvidenceRoot, AttemptID: attempt.ID, CaseID: unit.CaseID, RunID: run.RunID, Store: store})
		if err != nil {
			return err
		}
		state.workspace = workspace
		seed := a.Config.Seed
		if seed == nil {
			seed = SeedInput
		}
		parser, tenant := a.Config.ParserName, a.Config.TenantID
		if parser == "" {
			parser = "benchmark"
		}
		if tenant == "" {
			tenant = "benchmark-" + attempt.ID
		}
		state.seeded, err = seed(ctx, a.Config.DB, SeedInputRequest{AttemptID: attempt.ID, Workspace: workspace.Path(), TenantID: tenant, StoreID: a.Config.StoreID, ParserName: parser, Case: datasetCase, Status: `[]`})
		if err != nil {
			return err
		}
		payload, err := lineFileGeneratedPayload(state.seeded.ID, state.seeded.ResultFilename, processors)
		if err != nil {
			return err
		}
		executor := a.Config.ControllerExecutor
		if executor == nil {
			executor = func(ctx context.Context, runtime ApplicationRuntime, payload []byte) error {
				return runtime.RunEvent(ctx, payload)
			}
		}
		if err := executor(ctx, session.Runtime, payload); err != nil {
			return &ProcessorError{Err: err}
		}
		return nil
	}
	work.Capture = func(ctx context.Context, attempt AttemptRecord) (any, error) {
		if attempt.Kind == "rescore" {
			if !attempt.SourceExecutionAttemptID.Valid {
				return nil, fmt.Errorf("rescore has no source execution")
			}
			loader := a.Config.LoadEvidence
			if loader == nil {
				loader = store.LoadVerifiedEvidence
			}
			bundle, err := loader(ctx, attempt.SourceExecutionAttemptID.String)
			if err != nil {
				return nil, err
			}
			scorerJSON, scorerHash := scorerSnapshot(processors)
			if bundle.InputSHA256 != sha256Hex(datasetCase.InputBytes) || !bytes.Equal(bundle.InputBytes, datasetCase.InputBytes) ||
				!bytes.Equal(bundle.ExpectedJSON, datasetCase.ExpectedBytes) || !bytes.Equal(bundle.ConfigJSON, session.ConfigJSON) ||
				bundle.ConfigHash != session.ConfigHash || !bytes.Equal(bundle.ScorerJSON, scorerJSON) || bundle.ScorerHash != scorerHash {
				return nil, fmt.Errorf("rescore evidence identity does not match the prepared case and runtime")
			}
			return bundle, nil
		}
		scorerJSON, scorerHash := scorerSnapshot(processors)
		bundle := EvidenceBundle{SchemaVersion: 1, AttemptID: attempt.ID, InputSHA256: sha256Hex(datasetCase.InputBytes), InputBytes: datasetCase.InputBytes, ExpectedJSON: datasetCase.ExpectedBytes, ConfigJSON: session.ConfigJSON, ConfigHash: session.ConfigHash, ScorerJSON: scorerJSON, ScorerHash: scorerHash, Processors: map[string]EvidenceProcessor{}}
		for _, processor := range captureProcessors {
			entry := EvidenceProcessor{}
			adapter := adapterFor(processor, state.seeded)
			if adapter == nil {
				entry.CaptureError = "missing adapter"
			} else if captured, err := adapter.Capture(ctx, state.seeded.ID); err != nil {
				entry.CaptureError = err.Error()
			} else {
				entry.Capture, _ = canonicalJSON(captured)
			}
			bundle.Processors[string(processor)] = entry
		}
		raw, err := bundle.CanonicalJSON()
		if err != nil {
			return nil, err
		}
		if state.workspace == nil {
			return nil, fmt.Errorf("execution workspace is missing")
		}
		artifact, err := state.workspace.Capture(raw, "evidence.json")
		if err != nil {
			return nil, err
		}
		record := ArtifactRecord{AttemptID: sql.NullString{String: attempt.ID, Valid: true}, Kind: "evidence_bundle", Path: artifact.Path, SHA256: artifact.SHA256, SizeBytes: artifact.SizeBytes, Verified: artifact.Verified, Metadata: json.RawMessage(`{"schema_version":1}`)}
		if a.Config.InsertArtifact != nil {
			err = a.Config.InsertArtifact(ctx, record)
		} else {
			_, err = store.InsertArtifact(ctx, record)
		}
		return bundle, err
	}
	work.Reconcile = func(captured any) (any, error) {
		bundle, ok := captured.(EvidenceBundle)
		if !ok {
			return nil, fmt.Errorf("invalid evidence value")
		}
		result := reconciledEvidence{bundle: bundle, actuals: map[Processor]any{}}
		for _, processor := range captureProcessors {
			entry, exists := bundle.Processors[string(processor)]
			if !exists || entry.CaptureError != "" || len(entry.Capture) == 0 {
				return result, fmt.Errorf("%w: %s capture unavailable: %s", ErrInvalidOutput, processor, entry.CaptureError)
			}
			var capturedValue any
			switch processor {
			case ProcessorChunking:
				capturedValue = &ChunkCapture{}
			case ProcessorExtractMetrics:
				capturedValue = &MetricCapture{}
			}
			if err := json.Unmarshal(entry.Capture, capturedValue); err != nil {
				return result, fmt.Errorf("%w: %v", ErrInvalidOutput, err)
			}
			switch value := capturedValue.(type) {
			case *ChunkCapture:
				capturedValue = *value
			case *MetricCapture:
				capturedValue = *value
			}
			adapter := adapterFor(processor, state.seeded)
			if adapter == nil {
				return result, fmt.Errorf("%w: missing %s adapter", ErrInvalidOutput, processor)
			}
			actual, err := adapter.Reconcile(capturedValue)
			if err != nil {
				return result, err
			}
			result.actuals[processor] = actual
		}
		return result, nil
	}
	work.Score = func(ctx context.Context, attempt AttemptRecord, captured any) error {
		reconciled, ok := captured.(reconciledEvidence)
		if !ok {
			return &ScorerError{Err: fmt.Errorf("invalid reconciled evidence")}
		}
		for _, processor := range processors {
			var records []ScoreRecord
			switch processor {
			case ProcessorChunking:
				lines, err := docprocessing.ParseInputLinesIncludingTOC(reconciled.bundle.InputBytes)
				if err != nil {
					return &ScorerError{Err: err}
				}
				actual, valid := reconciled.actuals[processor].(ChunkActual)
				if !valid || datasetCase.ExpectedOutput.Chunking == nil {
					return &ScorerError{Err: fmt.Errorf("invalid chunk score input")}
				}
				score := ScoreChunks(ChunkScoreInput{SourceLines: lines, ExpectedChunks: datasetCase.ExpectedOutput.Chunking.Chunks, ProtectedGroups: datasetCase.ExpectedOutput.Chunking.ProtectedGroups, ActualChunks: actual.Chunks, ResolvedChunkSize: resolvedInt(reconciled.bundle.ConfigJSON, "chunk_size")})
				records, err = chunkScoreRecords(attempt.ID, score)
				if err != nil {
					return &ScorerError{Err: err}
				}
			case ProcessorExtractMetrics:
				if datasetCase.ExpectedOutput.ExtractMetrics == nil {
					return &ScorerError{Err: fmt.Errorf("missing metric gold")}
				}
				gold := make([]MetricRecord, 0, len(datasetCase.ExpectedOutput.ExtractMetrics.Metrics))
				for _, item := range datasetCase.ExpectedOutput.ExtractMetrics.Metrics {
					record, err := goldMetricRecord(item)
					if err != nil {
						return &ScorerError{Err: err}
					}
					gold = append(gold, record)
				}
				actual, valid := reconciled.actuals[processor].(MetricActual)
				if !valid {
					return &ScorerError{Err: fmt.Errorf("invalid metric actual")}
				}
				predictions, err := metricActualRecords(actual)
				if err != nil {
					return &ScorerError{Err: err}
				}
				_, upstreamValid := reconciled.actuals[ProcessorChunking].(ChunkActual)
				score, err := ScoreMetricsContext(ctx, MetricScoreInput{Gold: gold, Predictions: predictions, UpstreamValid: upstreamValid})
				if err != nil {
					return &ScorerError{Err: err}
				}
				diagnostics, _ := canonicalJSON(score.Diagnostics)
				records, err = metricScoreRecords(attempt.ID, score.Rows, diagnostics)
				if err != nil {
					return &ScorerError{Err: err}
				}
			}
			for _, record := range records {
				var err error
				if a.Config.InsertScore != nil {
					err = a.Config.InsertScore(ctx, record)
				} else {
					_, err = store.InsertScore(ctx, record)
				}
				if err != nil {
					return &ScorerError{Err: err}
				}
			}
			success := ScoreRecord{AttemptID: sql.NullString{String: attempt.ID, Valid: true}, Processor: string(processor), Scorer: "harness", ScorerVersion: "processor-success-v1", Metric: "processor_success", Slice: string(processor), Direction: "higher", AggregationKind: "binary_macro", Value: sql.NullFloat64{Float64: 1, Valid: true}, Numerator: sql.NullFloat64{Float64: 1, Valid: true}, Denominator: sql.NullFloat64{Float64: 1, Valid: true}, NonNull: true, Applicable: true, Metadata: json.RawMessage(`{}`)}
			if a.Config.InsertScore != nil {
				if err := a.Config.InsertScore(ctx, success); err != nil {
					return &ScorerError{Err: err}
				}
			} else if _, err := store.InsertScore(ctx, success); err != nil {
				return &ScorerError{Err: err}
			}
		}
		return nil
	}
	work.Cleanup = func(context.Context, AttemptRecord) error {
		if state.workspace == nil {
			return nil
		}
		return state.workspace.Cleanup(CleanupOptions{Cleanup: func(tx CleanupTx) error {
			if err := tx.DeleteProductionRows(); err != nil {
				return err
			}
			if err := tx.DeleteInput(); err != nil {
				return err
			}
			return tx.MarkState("files_pending", nil)
		}})
	}
	runAttempt := a.Config.AttemptRunner
	if runAttempt == nil {
		runAttempt = func(ctx context.Context, id string, config RunnerConfig, work AttemptWork) error {
			return (Runner{Store: store, Config: config, Work: work}).RunCase(ctx, id)
		}
	}
	return runAttempt(ctx, caseRunID, RunnerConfig{Owner: a.Config.Owner, Timeout: experiment.Timeout, Heartbeat: benchmarkHeartbeat(experiment.AttemptLease), AttemptLease: experiment.AttemptLease, MaxAttempts: experiment.MaxAttempts, RetainWorkspaces: experiment.RetainWorkspaces, Now: a.Config.Now}, work)
}

func containsProcessor(processors []Processor, want Processor) bool {
	for _, processor := range processors {
		if processor == want {
			return true
		}
	}
	return false
}

func (a Application) verifyCaseFiles(experiment *Experiment, datasetCase DatasetCase) error {
	if a.Config.AllowUnverifiedFixtureFiles {
		return nil
	}
	if a.Config.DatasetRoot == "" {
		return fmt.Errorf("benchmark dataset root is required for case-file verification")
	}
	base := filepath.Join(a.Config.DatasetRoot, experiment.DatasetID, experiment.DatasetVersion)
	checks := []struct {
		path string
		want []byte
	}{{datasetCase.Input, datasetCase.InputBytes}, {datasetCase.Expected, datasetCase.ExpectedBytes}}
	for _, check := range checks {
		raw, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(check.path)))
		if err != nil {
			return fmt.Errorf("benchmark case %s reverify %s: %w", datasetCase.CaseID, check.path, err)
		}
		if !bytes.Equal(raw, check.want) || experiment.FileHashes[check.path] != sha256Hex(raw) {
			return fmt.Errorf("benchmark case %s file hash changed after preparation: %s", datasetCase.CaseID, check.path)
		}
	}
	return nil
}

func benchmarkHeartbeat(lease time.Duration) time.Duration {
	interval := lease / 4
	if interval > time.Minute {
		return time.Minute
	}
	return interval
}

func resolvedInt(raw json.RawMessage, key string) int {
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	switch value := values[key].(type) {
	case float64:
		return int(value)
	case string:
		n, _ := strconv.Atoi(value)
		return n
	}
	return 0
}
