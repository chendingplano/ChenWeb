package docbenchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/pelletier/go-toml/v2"
)

const (
	defaultExperimentTimeout = 20 * time.Minute
	defaultAttemptLeaseExtra = 5 * time.Minute

	// MaxParallelCases bounds scheduler fan-out and per-run allocation.
	MaxParallelCases = 256
	// MaxParallelVariants bounds simultaneously initialized variant workers.
	MaxParallelVariants = 64
	// MaxAttempts bounds persisted retry allocation for one logical case run.
	MaxAttempts = 100
)

var processorOverrides = map[Processor]map[string]struct{}{
	ProcessorChunking: {
		"CHUNK_SIZE": {}, "CHUNK_OVERLAP_PERCENT": {},
	},
	ProcessorExtractMetrics: {
		"EXTRACT_METRIC_CANDIDATES_PROMPT":         {},
		"EXTRACT_METRIC_CANDIDATES_MODEL_NAME":     {},
		"EXTRACT_METRIC_CANDIDATES_MODEL_FALLBACK": {},
		"ENRICH_METRICS_PROMPT":                    {},
		"ENRICH_METRICS_MODEL_NAME":                {},
		"EXTRACT_METRICS_MODEL_NAME":               {},
		"METRIC_MERGE_RESOLVE_PROMPT":              {},
		"METRIC_MERGE_RESOLVE_MODEL_NAME":          {},
		"METRIC_MERGE_RESOLVE_MODEL_FALLBACK":      {},
		"METRIC_ENRICH_GROUP_SIZE":                 {},
		"EXTRACT_METRICS_MAX_TASKS":                {},
	},
}

// DatasetResolver resolves one immutable dataset identity. Implementations must not
// mutate benchmark persistence while validating an experiment.
type DatasetResolver interface {
	ResolveDataset(id, version string) (*Dataset, error)
}

// DatasetRootResolver loads datasets from Root/<dataset-id>/<dataset-version>.
type DatasetRootResolver struct{ Root string }

func (r DatasetRootResolver) ResolveDataset(id, version string) (*Dataset, error) {
	if !safeDatasetComponent(id) {
		return nil, fmt.Errorf("dataset id %q is invalid", id)
	}
	if !safeDatasetComponent(version) || !semverRE.MatchString(version) {
		return nil, fmt.Errorf("dataset version %q is invalid", version)
	}
	canonicalRoot, err := filepath.Abs(r.Root)
	if err != nil {
		return nil, fmt.Errorf("datasets root: %w", err)
	}
	datasetsRoot, err := os.OpenRoot(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("datasets root: %w", err)
	}
	defer datasetsRoot.Close()
	if err := requireNonSymlinkDirectory(datasetsRoot, id, "dataset ID"); err != nil {
		return nil, err
	}
	prefix := filepath.Join(id, version)
	if err := requireNonSymlinkDirectory(datasetsRoot, prefix, "dataset version"); err != nil {
		return nil, err
	}
	dataset, err := loadDatasetFromRoot(datasetsRoot, prefix, filepath.Join(canonicalRoot, prefix))
	if err != nil {
		return nil, err
	}
	if dataset.Manifest.DatasetID != id {
		return nil, fmt.Errorf("manifest dataset_id %q does not match requested %q", dataset.Manifest.DatasetID, id)
	}
	if dataset.Manifest.DatasetVersion != version {
		return nil, fmt.Errorf("manifest dataset_version %q does not match requested %q", dataset.Manifest.DatasetVersion, version)
	}
	return dataset, nil
}

func safeDatasetComponent(value string) bool {
	return value != "." && value != ".." && restrictedASCII(value, false)
}

func requireNonSymlinkDirectory(root *os.Root, name, field string) error {
	info, err := root.Lstat(name)
	if err != nil {
		return fmt.Errorf("%s directory %q: %w", field, name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s directory %q: symlink is forbidden", field, name)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s directory %q: not a directory", field, name)
	}
	return nil
}

// ExperimentVariant is one explicit requested run. Overrides remain requested
// strings; they are not an authoritative production runtime snapshot.
type ExperimentVariant struct {
	Name          string            `json:"name"`
	Overrides     map[string]string `json:"overrides"`
	OverridesJSON json.RawMessage   `json:"overrides_json"`
}

// Experiment is validated, materialized requested intent plus dataset provenance.
// Runtime-resolved prompts, models, providers, defaults, and secrets intentionally do
// not belong here; workers attach those values after production initialization.
type Experiment struct {
	Name                   string
	DatasetID              string
	DatasetVersion         string
	Processors             []Processor
	Repetitions            int
	CaseTags               []string
	Timeout                time.Duration
	MaxParallelCases       int
	MaxParallelVariants    int
	MaxAttempts            int
	AttemptLease           time.Duration
	AllowUpstreamVariation bool
	RetainWorkspaces       bool
	Variants               []ExperimentVariant

	RawTOML                []byte
	RequestHash            string
	RequestedOverridesJSON json.RawMessage
	MaterializedConfigJSON json.RawMessage
	DatasetHash            string
	FileHashes             map[string]string
	ProcessorCaseSetHashes map[Processor]string
}

type rawExperiment struct {
	Name                   string       `toml:"name"`
	Dataset                string       `toml:"dataset"`
	Processors             []Processor  `toml:"processors"`
	Repetitions            *int         `toml:"repetitions"`
	CaseTags               []string     `toml:"case_tags"`
	Timeout                *string      `toml:"timeout"`
	MaxParallelCases       *int         `toml:"max_parallel_cases"`
	MaxParallelVariants    *int         `toml:"max_parallel_variants"`
	MaxAttempts            *int         `toml:"max_attempts"`
	AttemptLease           *string      `toml:"attempt_lease"`
	AllowUpstreamVariation bool         `toml:"allow_upstream_variation"`
	RetainWorkspaces       bool         `toml:"retain_workspaces"`
	Variants               []rawVariant `toml:"variants"`
}

type rawVariant struct {
	Name      string            `toml:"name"`
	Overrides map[string]string `toml:"overrides"`
}

type materializedRequestedConfig struct {
	Name                   string                `json:"name"`
	Dataset                string                `json:"dataset"`
	Processors             []Processor           `json:"processors"`
	Repetitions            int                   `json:"repetitions"`
	CaseTags               []string              `json:"case_tags"`
	Timeout                string                `json:"timeout"`
	MaxParallelCases       int                   `json:"max_parallel_cases"`
	MaxParallelVariants    int                   `json:"max_parallel_variants"`
	MaxAttempts            int                   `json:"max_attempts"`
	AttemptLease           string                `json:"attempt_lease"`
	AllowUpstreamVariation bool                  `json:"allow_upstream_variation"`
	RetainWorkspaces       bool                  `json:"retain_workspaces"`
	Variants               []materializedVariant `json:"variants"`
}

type materializedVariant struct {
	Name      string            `json:"name"`
	Overrides map[string]string `json:"overrides"`
}

// LoadExperiment validates raw TOML using a configured datasets root.
func LoadExperiment(raw []byte, datasetsRoot string) (*Experiment, error) {
	return ResolveExperiment(raw, DatasetRootResolver{Root: datasetsRoot})
}

// ResolveExperiment strictly parses and validates requested experiment intent.
func ResolveExperiment(raw []byte, resolver DatasetResolver) (*Experiment, error) {
	if resolver == nil {
		return nil, fmt.Errorf("dataset resolver is required")
	}
	var request rawExperiment
	decoder := toml.NewDecoder(bytes.NewReader(raw)).DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("experiment TOML: %w", err)
	}

	datasetID, datasetVersion, err := parseDatasetReference(request.Dataset)
	if err != nil {
		return nil, err
	}
	experiment, err := materializeExperiment(request, datasetID, datasetVersion)
	if err != nil {
		return nil, err
	}
	dataset, err := resolver.ResolveDataset(datasetID, datasetVersion)
	if err != nil {
		return nil, fmt.Errorf("resolve dataset %s@%s: %w", datasetID, datasetVersion, err)
	}
	if dataset == nil {
		return nil, fmt.Errorf("resolve dataset %s@%s: resolver returned nil", datasetID, datasetVersion)
	}
	if dataset.Manifest.DatasetID != datasetID {
		return nil, fmt.Errorf("manifest dataset_id %q does not match requested %q", dataset.Manifest.DatasetID, datasetID)
	}
	if dataset.Manifest.DatasetVersion != datasetVersion {
		return nil, fmt.Errorf("manifest dataset_version %q does not match requested %q", dataset.Manifest.DatasetVersion, datasetVersion)
	}

	experiment.RawTOML = append([]byte(nil), raw...)
	requestSum := sha256.Sum256(raw)
	experiment.RequestHash = hex.EncodeToString(requestSum[:])
	experiment.DatasetHash = dataset.Hash
	experiment.FileHashes = ComputeFileHashes(dataset)
	experiment.ProcessorCaseSetHashes = make(map[Processor]string, len(experiment.Processors))
	for _, processor := range experiment.Processors {
		caseSetHash, err := dataset.CaseSetHash(processor, experiment.CaseTags, experiment.Repetitions)
		if err != nil {
			return nil, fmt.Errorf("%s case-set hash: %w", processor, err)
		}
		experiment.ProcessorCaseSetHashes[processor] = caseSetHash
	}
	if err := attachRequestedJSON(experiment); err != nil {
		return nil, err
	}
	return experiment, nil
}

func parseDatasetReference(reference string) (string, string, error) {
	separator := strings.LastIndexByte(reference, '@')
	if separator <= 0 || separator == len(reference)-1 || strings.Count(reference, "@") != 1 {
		return "", "", fmt.Errorf("dataset: must use dataset-id@version")
	}
	id, version := reference[:separator], reference[separator+1:]
	if !restrictedASCII(id, false) {
		return "", "", fmt.Errorf("dataset: invalid dataset ID %q", id)
	}
	if !semverRE.MatchString(version) {
		return "", "", fmt.Errorf("dataset: invalid dataset version %q", version)
	}
	return id, version, nil
}

func materializeExperiment(request rawExperiment, datasetID, datasetVersion string) (*Experiment, error) {
	if strings.TrimSpace(request.Name) == "" {
		return nil, fmt.Errorf("name: must not be empty")
	}
	processors, err := validateProcessors(request.Processors)
	if err != nil {
		return nil, err
	}
	tags, err := validateCaseTags(request.CaseTags)
	if err != nil {
		return nil, err
	}
	repetitions := 1
	if hasProcessor(processors, ProcessorExtractMetrics) {
		repetitions = 3
	}
	if request.Repetitions != nil {
		repetitions = *request.Repetitions
	}
	if repetitions < 1 || repetitions > MaxRepetitions {
		return nil, fmt.Errorf("repetitions: must be between 1 and %d", MaxRepetitions)
	}
	timeout, err := parsePositiveDuration("timeout", request.Timeout, defaultExperimentTimeout)
	if err != nil {
		return nil, err
	}
	attemptLease, err := parsePositiveDuration("attempt_lease", request.AttemptLease, timeout+defaultAttemptLeaseExtra)
	if err != nil {
		return nil, err
	}
	if attemptLease <= timeout {
		return nil, fmt.Errorf("attempt_lease: must be longer than timeout")
	}
	maxParallelCases, err := boundedPositiveCount("max_parallel_cases", request.MaxParallelCases, 1, MaxParallelCases)
	if err != nil {
		return nil, err
	}
	maxParallelVariants, err := boundedPositiveCount("max_parallel_variants", request.MaxParallelVariants, 1, MaxParallelVariants)
	if err != nil {
		return nil, err
	}
	maxAttempts, err := boundedPositiveCount("max_attempts", request.MaxAttempts, 2, MaxAttempts)
	if err != nil {
		return nil, err
	}
	variants, err := validateVariants(request.Variants, processors)
	if err != nil {
		return nil, err
	}
	return &Experiment{
		Name: request.Name, DatasetID: datasetID, DatasetVersion: datasetVersion,
		Processors: processors, Repetitions: repetitions, CaseTags: tags,
		Timeout: timeout, MaxParallelCases: maxParallelCases,
		MaxParallelVariants: maxParallelVariants, MaxAttempts: maxAttempts,
		AttemptLease: attemptLease, AllowUpstreamVariation: request.AllowUpstreamVariation,
		RetainWorkspaces: request.RetainWorkspaces, Variants: variants,
	}, nil
}

func validateProcessors(processors []Processor) ([]Processor, error) {
	if len(processors) == 0 {
		return nil, fmt.Errorf("processors: must not be empty")
	}
	seen := make(map[Processor]struct{}, len(processors))
	for _, processor := range processors {
		if processor != ProcessorChunking && processor != ProcessorExtractMetrics {
			return nil, fmt.Errorf("processors: unsupported processor %q", processor)
		}
		if _, exists := seen[processor]; exists {
			return nil, fmt.Errorf("processors: duplicate processor %q", processor)
		}
		seen[processor] = struct{}{}
	}
	out := append([]Processor(nil), processors...)
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out, nil
}

func validateCaseTags(tags []string) ([]string, error) {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if _, ok := allowedTags[tag]; !ok {
			return nil, fmt.Errorf("case_tags: unknown tag %q", tag)
		}
		seen[tag] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out, nil
}

func validateVariants(raw []rawVariant, processors []Processor) ([]ExperimentVariant, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("variants: at least one explicit variant is required")
	}
	allowed := allowedOverrides(processors)
	seen := make(map[string]struct{}, len(raw))
	out := make([]ExperimentVariant, 0, len(raw))
	for i, variant := range raw {
		if strings.TrimSpace(variant.Name) == "" {
			return nil, fmt.Errorf("variants[%d].name: must not be empty", i)
		}
		if _, exists := seen[variant.Name]; exists {
			return nil, fmt.Errorf("variants[%d].name: duplicate variant %q", i, variant.Name)
		}
		seen[variant.Name] = struct{}{}
		overrides := make(map[string]string, len(variant.Overrides))
		keys := make([]string, 0, len(variant.Overrides))
		for key := range variant.Overrides {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if secretShaped(key) {
				return nil, fmt.Errorf("variants[%d].overrides.%s: secret-shaped override keys are forbidden", i, key)
			}
			if _, ok := allowed[key]; !ok {
				return nil, fmt.Errorf("variants[%d].overrides.%s: override is not allowed by the selected processor closure", i, key)
			}
			value, err := materializeOverrideValue(variant.Overrides[key])
			if err != nil {
				return nil, fmt.Errorf("variants[%d].overrides.%s: %w", i, key, err)
			}
			overrides[key] = value
		}
		encoded, err := json.Marshal(overrides)
		if err != nil {
			return nil, fmt.Errorf("variants[%d].overrides: %w", i, err)
		}
		out = append(out, ExperimentVariant{Name: variant.Name, Overrides: overrides, OverridesJSON: encoded})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func allowedOverrides(processors []Processor) map[string]struct{} {
	out := make(map[string]struct{})
	for _, processor := range processors {
		for key := range processorOverrides[processor] {
			out[key] = struct{}{}
		}
		if processor == ProcessorExtractMetrics {
			for key := range processorOverrides[ProcessorChunking] {
				out[key] = struct{}{}
			}
		}
	}
	return out
}

func materializeOverrideValue(raw string) (string, error) {
	if len(raw) >= 4 && strings.HasPrefix(raw, "${") && strings.HasSuffix(raw, "}") {
		name := strings.TrimSpace(raw[2 : len(raw)-1])
		if name == "" {
			return "", fmt.Errorf("environment variable name must not be empty")
		}
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			return "", fmt.Errorf("environment variable %q is not set", name)
		}
		return value, nil
	}
	return raw, nil
}

func secretShaped(key string) bool {
	var normalized strings.Builder
	for _, r := range strings.ToLower(key) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
		}
	}
	value := normalized.String()
	for _, fragment := range []string{"apikey", "token", "password", "passwd", "passphrase", "secret", "credential", "privatekey", "accesskey"} {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

func parsePositiveDuration(field string, raw *string, defaultValue time.Duration) (time.Duration, error) {
	if raw == nil {
		return defaultValue, nil
	}
	value, err := time.ParseDuration(*raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid Go duration: %w", field, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s: must be positive", field)
	}
	return value, nil
}

func boundedPositiveCount(field string, raw *int, defaultValue, maximum int) (int, error) {
	if raw == nil {
		return defaultValue, nil
	}
	if *raw < 1 || *raw > maximum {
		return 0, fmt.Errorf("%s: must be between 1 and %d", field, maximum)
	}
	return *raw, nil
}

func attachRequestedJSON(experiment *Experiment) error {
	requestedOverrides := make(map[string]map[string]string, len(experiment.Variants))
	materializedVariants := make([]materializedVariant, 0, len(experiment.Variants))
	for _, variant := range experiment.Variants {
		requestedOverrides[variant.Name] = variant.Overrides
		materializedVariants = append(materializedVariants, materializedVariant{Name: variant.Name, Overrides: variant.Overrides})
	}
	requestedJSON, err := json.Marshal(requestedOverrides)
	if err != nil {
		return fmt.Errorf("canonical requested overrides: %w", err)
	}
	configJSON, err := json.Marshal(materializedRequestedConfig{
		Name:                   experiment.Name,
		Dataset:                experiment.DatasetID + "@" + experiment.DatasetVersion,
		Processors:             experiment.Processors,
		Repetitions:            experiment.Repetitions,
		CaseTags:               experiment.CaseTags,
		Timeout:                experiment.Timeout.String(),
		MaxParallelCases:       experiment.MaxParallelCases,
		MaxParallelVariants:    experiment.MaxParallelVariants,
		MaxAttempts:            experiment.MaxAttempts,
		AttemptLease:           experiment.AttemptLease.String(),
		AllowUpstreamVariation: experiment.AllowUpstreamVariation,
		RetainWorkspaces:       experiment.RetainWorkspaces,
		Variants:               materializedVariants,
	})
	if err != nil {
		return fmt.Errorf("materialized requested config: %w", err)
	}
	experiment.RequestedOverridesJSON = requestedJSON
	experiment.MaterializedConfigJSON = configJSON
	return nil
}
