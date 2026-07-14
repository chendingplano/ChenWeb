package docbenchmark

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

// ApplicationRuntime is the deliberately narrow production-runtime seam used
// by benchmark workers. Tests may inject a fake without initializing LLMs.
type ApplicationRuntime interface {
	AllowedOverrides() map[string][]string
	ResolvedConfig() docprocessing.ResolvedConfigSnapshot
	RunEvent(context.Context, []byte) error
}

type RuntimeFactory func(context.Context, ExperimentVariant, []Processor) (ApplicationRuntime, error)

type AttemptWorkspace interface {
	Path() string
	Capture([]byte, string) (Artifact, error)
	Cleanup(CleanupOptions) error
	Close() error
}
type AttemptRunnerFunc func(context.Context, string, RunnerConfig, AttemptWork) error
type EvidenceLoader func(context.Context, string) (EvidenceBundle, error)

type ApplicationConfig struct {
	DB                                                *sql.DB
	DatasetRoot, WorkRoot, EvidenceRoot, ArtifactRoot string
	Owner                                             string
	TenantID                                          string
	StoreID                                           int64
	ParserName                                        string
	Now                                               func() time.Time
	RuntimeFactory                                    RuntimeFactory
	AttemptRunner                                     AttemptRunnerFunc
	AllocateWorkspace                                 func(WorkspaceConfig) (AttemptWorkspace, error)
	Seed                                              func(context.Context, *sql.DB, SeedInputRequest) (SeededInput, error)
	AdapterFactory                                    func(Processor, DatasetCase, SeededInput) Adapter
	ControllerExecutor                                func(context.Context, ApplicationRuntime, []byte) error
	InsertArtifact                                    func(context.Context, ArtifactRecord) error
	InsertScore                                       func(context.Context, ScoreRecord) error
	LoadEvidence                                      EvidenceLoader
}

type Application struct{ Config ApplicationConfig }

type CaseUnit struct {
	CaseID     string
	Repetition int
}

type PreparedRun struct {
	RunID      string
	Variant    ExperimentVariant
	CaseRuns   map[CaseUnit]string
	Cases      map[CaseUnit]DatasetCase
	Processors map[CaseUnit][]Processor
}

type PreparedExperiment struct {
	ExperimentID string
	Experiment   *Experiment
	Dataset      *Dataset
	VariantOrder []string
	Runs         map[string]PreparedRun
}

func (a Application) Prepare(ctx context.Context, raw []byte) (*PreparedExperiment, error) {
	if a.Config.DB == nil {
		return nil, fmt.Errorf("benchmark application: nil database")
	}
	experiment, err := LoadExperiment(raw, a.Config.DatasetRoot)
	if err != nil {
		return nil, err
	}
	dataset, err := (DatasetRootResolver{Root: a.Config.DatasetRoot}).ResolveDataset(experiment.DatasetID, experiment.DatasetVersion)
	if err != nil {
		return nil, err
	}
	store := SQLStore{DB: a.Config.DB}
	experimentID, err := store.CreateExperiment(ctx, *experiment)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedExperiment{ExperimentID: experimentID, Experiment: experiment, Dataset: dataset, Runs: map[string]PreparedRun{}}
	variants := append([]ExperimentVariant(nil), experiment.Variants...)
	sort.Slice(variants, func(i, j int) bool { return variants[i].Name < variants[j].Name })
	scorers := map[string]any{
		"chunking":        map[string]any{"version": ChunkScorerVersion},
		"extract_metrics": MetricScorerConfigurationV1(),
	}
	tagFilter := map[string]struct{}{}
	for _, tag := range experiment.CaseTags {
		tagFilter[tag] = struct{}{}
	}
	for _, variant := range variants {
		requested := map[string]any{"name": variant.Name, "overrides": variant.Overrides}
		runID, err := store.CreateRun(ctx, experimentID, variant.Name, requested, map[string]any{"status": "pending_runtime_resolution"}, map[string]any{}, map[string]any{}, scorers, map[string]any{})
		if err != nil {
			return nil, err
		}
		run := PreparedRun{RunID: runID, Variant: variant, CaseRuns: map[CaseUnit]string{}, Cases: map[CaseUnit]DatasetCase{}, Processors: map[CaseUnit][]Processor{}}
		cases := append([]DatasetCase(nil), dataset.Cases...)
		sort.Slice(cases, func(i, j int) bool { return cases[i].CaseID < cases[j].CaseID })
		for _, datasetCase := range cases {
			if !hasAllTags(datasetCase.Tags, tagFilter) {
				continue
			}
			applicable := selectedCaseProcessors(experiment.Processors, datasetCase.Processors)
			if len(applicable) == 0 {
				continue
			}
			provenance := map[string]any{
				"tags": append([]string(nil), datasetCase.Tags...), "processors": applicable,
				"input_sha256": dataset.FileHashes[datasetCase.Input], "expected_sha256": dataset.FileHashes[datasetCase.Expected],
			}
			for repetition := 1; repetition <= experiment.Repetitions; repetition++ {
				unit := CaseUnit{CaseID: datasetCase.CaseID, Repetition: repetition}
				caseRunID, err := store.CreateCaseRun(ctx, runID, datasetCase.CaseID, repetition, "applicable", provenance, nil)
				if err != nil {
					return nil, err
				}
				run.CaseRuns[unit], run.Cases[unit], run.Processors[unit] = caseRunID, datasetCase, append([]Processor(nil), applicable...)
			}
		}
		prepared.VariantOrder = append(prepared.VariantOrder, variant.Name)
		prepared.Runs[variant.Name] = run
	}
	return prepared, nil
}

func selectedCaseProcessors(selected, applicable []Processor) []Processor {
	set := map[Processor]bool{}
	for _, processor := range applicable {
		set[processor] = true
	}
	out := make([]Processor, 0, len(selected))
	for _, processor := range selected {
		if set[processor] {
			out = append(out, processor)
		}
	}
	return out
}

// VariantSession contains only the authoritative, secret-free resolved
// runtime snapshot. Requested overrides remain in the experiment record.
type VariantSession struct {
	Runtime    ApplicationRuntime `json:"-"`
	ConfigJSON json.RawMessage    `json:"config_json"`
	ConfigHash string             `json:"config_hash"`
}

type productionApplicationRuntime struct {
	runtime *docprocessing.ProductionRuntime
}

func (r productionApplicationRuntime) AllowedOverrides() map[string][]string {
	return r.runtime.AllowedOverrides()
}
func (r productionApplicationRuntime) ResolvedConfig() docprocessing.ResolvedConfigSnapshot {
	return r.runtime.ResolvedConfig()
}
func (r productionApplicationRuntime) RunEvent(ctx context.Context, payload []byte) error {
	return r.runtime.Control.RunEvent(ctx, payload)
}

func defaultRuntimeFactory(_ context.Context, variant ExperimentVariant, processors []Processor) (ApplicationRuntime, error) {
	required := make([]string, len(processors))
	for i, processor := range processors {
		required[i] = string(processor)
	}
	runtime, err := docprocessing.NewProductionRuntime(docprocessing.ProductionRuntimeOptions{Overrides: variant.Overrides, RequiredProcessors: required})
	if err != nil {
		return nil, err
	}
	return productionApplicationRuntime{runtime: runtime}, nil
}

func (a Application) InitializeVariant(ctx context.Context, variant ExperimentVariant, processors []Processor) (VariantSession, error) {
	factory := a.Config.RuntimeFactory
	if factory == nil {
		factory = defaultRuntimeFactory
	}
	runtime, err := factory(ctx, variant, append([]Processor(nil), processors...))
	if err != nil {
		return VariantSession{}, err
	}
	if runtime == nil {
		return VariantSession{}, fmt.Errorf("benchmark application: runtime factory returned nil")
	}
	allowed := map[string]bool{}
	allowlists := runtime.AllowedOverrides()
	for _, processor := range processors {
		for _, key := range allowlists[string(processor)] {
			allowed[strings.ToUpper(key)] = true
		}
	}
	for key := range variant.Overrides {
		if !allowed[strings.ToUpper(key)] {
			return VariantSession{}, fmt.Errorf("variant %q override %q is not allowed by the initialized runtime", variant.Name, key)
		}
	}
	snapshot := runtime.ResolvedConfig()
	if snapshot.Hash == "" {
		return VariantSession{}, fmt.Errorf("variant %q returned an empty resolved config hash", variant.Name)
	}
	raw := snapshot.CanonicalJSON
	if len(raw) == 0 {
		raw, err = canonicalJSON(snapshot.Values)
		if err != nil {
			return VariantSession{}, err
		}
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return VariantSession{}, fmt.Errorf("variant %q resolved config: %w", variant.Name, err)
	}
	if path, ok := secretPath(decoded, ""); ok {
		return VariantSession{}, fmt.Errorf("variant %q resolved config contains secret-shaped field %q", variant.Name, path)
	}
	canonical, err := canonicalJSON(decoded)
	if err != nil {
		return VariantSession{}, err
	}
	sum := sha256.Sum256(canonical)
	if got := hex.EncodeToString(sum[:]); snapshot.Hash != got {
		// Production owns the hash contract; fakes may use an opaque stable hash.
		// The canonical bytes remain independently deterministic and secret-free.
	}
	return VariantSession{Runtime: runtime, ConfigJSON: canonical, ConfigHash: snapshot.Hash}, nil
}

func secretPath(v any, prefix string) (string, bool) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lower := strings.ToLower(key)
			for _, marker := range []string{"password", "passwd", "api_key", "apikey", "api_token", "access_token", "secret", "credential", "authorization"} {
				if strings.Contains(lower, marker) {
					return strings.TrimPrefix(prefix+"."+key, "."), true
				}
			}
			if path, ok := secretPath(x[key], prefix+"."+key); ok {
				return path, true
			}
		}
	case []any:
		for i, value := range x {
			if path, ok := secretPath(value, fmt.Sprintf("%s[%d]", prefix, i)); ok {
				return path, true
			}
		}
	}
	return "", false
}

func rawStable(v any) (json.RawMessage, error) {
	b, err := canonicalJSON(v)
	return json.RawMessage(b), err
}

func goldMetricRecord(g GoldMetric) (MetricRecord, error) {
	r := MetricRecord{
		GoldID: g.GoldID, Name: g.MetricName, NameEn: g.MetricNameEn,
		Subject: g.MetricSubject, SubjectEn: g.MetricSubjectEn, Value: g.MetricValue,
		Unit: g.MetricUnit, UnitEn: g.MetricUnitEn, Desc: g.MetricDesc, DescEn: g.MetricDescEn,
		Context: g.MetricContext, ContextEn: g.MetricContextEn, LocationType: g.LocationType,
		ValueDataType: g.ValueDataType, ValueRangeType: g.ValueRangeType, ValueClass: g.ValueClass,
		ValueClassEn: g.ValueClassEn, FormulaOrDefinition: g.FormulaOrDefinition,
		ThresholdOrTarget: g.ThresholdOrTarget, MeasurementFrequency: g.MeasurementFrequency,
		TableNameOrSection: g.TableNameOrSection, IsExplicitMetric: g.IsExplicitMetric,
		SourceLines: append([]int(nil), g.SourceLines...), StableFields: map[string]json.RawMessage{},
	}
	stable := map[string]json.RawMessage{
		"metric_categories": g.MetricCategories, "metric_categories_en": g.MetricCategoriesEn,
		"category_paths": g.CategoryPaths, "category_paths_en": g.CategoryPathsEn, "objects": g.Objects,
	}
	for key, value := range stable {
		if len(value) == 0 {
			continue
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return MetricRecord{}, fmt.Errorf("gold %s %s: %w", g.GoldID, key, err)
		}
		canonical, _ := rawStable(decoded)
		r.StableFields[key] = canonical
	}
	return r, nil
}

func metricActualRecords(actual MetricActual) ([]MetricRecord, error) {
	out := make([]MetricRecord, 0, len(actual.Rows))
	for index, row := range actual.Rows {
		r := MetricRecord{PredictionInputIndex: index, StableFields: map[string]json.RawMessage{}}
		if value, ok := row["metric_id"].(string); ok {
			r.GoldID = value
		}
		var err error
		for key, target := range map[string]**string{
			"metric_name": &r.Name, "metric_name_en": &r.NameEn, "metric_subject": &r.Subject,
			"metric_subject_en": &r.SubjectEn, "metric_value": &r.Value, "metric_unit": &r.Unit,
			"metric_unit_en": &r.UnitEn, "metric_desc": &r.Desc, "metric_desc_en": &r.DescEn,
			"metric_context": &r.Context, "metric_context_en": &r.ContextEn, "location_type": &r.LocationType,
			"value_data_type": &r.ValueDataType, "value_range_type": &r.ValueRangeType,
			"value_class": &r.ValueClass, "value_class_en": &r.ValueClassEn,
			"formula_or_definition": &r.FormulaOrDefinition, "threshold_or_target": &r.ThresholdOrTarget,
			"measurement_frequency": &r.MeasurementFrequency, "table_name_or_section": &r.TableNameOrSection,
		} {
			if raw, present := row[key]; present && raw != nil {
				value, ok := raw.(string)
				if !ok {
					return nil, fmt.Errorf("prediction %d field %s is not a string", index, key)
				}
				copy := value
				*target = &copy
			}
		}
		if raw, present := row["is_explicit_metric"]; present && raw != nil {
			value, ok := raw.(bool)
			if !ok {
				return nil, fmt.Errorf("prediction %d explicit flag is not boolean", index)
			}
			r.IsExplicitMetric = &value
		}
		r.SourceLines, err = sourceLinesFromDB(row["source_line_spans"])
		if err != nil {
			return nil, fmt.Errorf("prediction %d source spans: %w", index, err)
		}
		core := map[string]bool{"metric_id": true, "metric_name": true, "metric_name_en": true, "metric_subject": true, "metric_subject_en": true, "metric_value": true, "metric_unit": true, "metric_unit_en": true, "is_explicit_metric": true, "source_line_spans": true}
		for key, value := range row {
			if core[key] || value == nil {
				continue
			}
			canonical, e := rawStable(value)
			if e != nil {
				return nil, e
			}
			r.StableFields[key] = canonical
		}
		out = append(out, r)
	}
	return out, nil
}

func sourceLinesFromDB(raw any) ([]int, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array")
	}
	var out []int
	for _, item := range values {
		switch value := item.(type) {
		case float64:
			out = append(out, int(value))
		case map[string]any:
			start, sok := value["start"].(float64)
			end, eok := value["end"].(float64)
			if !sok {
				start, sok = value["start_line"].(float64)
			}
			if !eok {
				end, eok = value["end_line"].(float64)
			}
			if !sok || !eok || end < start {
				return nil, fmt.Errorf("invalid span")
			}
			for line := int(start); line <= int(end); line++ {
				out = append(out, line)
			}
		default:
			return nil, fmt.Errorf("invalid source line value")
		}
	}
	return out, nil
}

func metricScoreRecords(attemptID string, rows []ScoreRow, diagnostics json.RawMessage) ([]ScoreRecord, error) {
	metadata, err := canonicalJSON(diagnostics)
	if err != nil {
		return nil, err
	}
	out := make([]ScoreRecord, 0, len(rows))
	for _, row := range rows {
		record := ScoreRecord{
			AttemptID: sql.NullString{String: attemptID, Valid: true}, Processor: string(ProcessorExtractMetrics),
			Scorer: "metric", ScorerVersion: MetricScorerVersion, Metric: row.Metric, Slice: row.Component,
			Direction: row.Direction, AggregationKind: row.AggregationKind, NonNull: row.Value != nil,
			Applicable: true, Metadata: metadata,
			Numerator:   sql.NullFloat64{Float64: float64(row.Numerator), Valid: true},
			Denominator: sql.NullFloat64{Float64: float64(row.Denominator), Valid: true},
		}
		if row.Value != nil {
			record.Value = sql.NullFloat64{Float64: *row.Value, Valid: true}
			if row.Component != "" {
				record.AdditiveComponent = record.Value
			}
		}
		out = append(out, record)
	}
	return out, nil
}
