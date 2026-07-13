package docbenchmark

import "encoding/json"

type Processor string

const (
	ProcessorChunking       Processor = "chunking"
	ProcessorExtractMetrics Processor = "extract_metrics"
)

type Manifest struct {
	SchemaVersion    int            `json:"schema_version"`
	DatasetID        string         `json:"dataset_id"`
	DatasetVersion   string         `json:"dataset_version"`
	GeneratorVersion string         `json:"generator_version"`
	Seed             int64          `json:"seed"`
	Cases            []ManifestCase `json:"cases"`
	seedPresent      bool
	casesPresent     bool
}

// manifestJSON is presence-aware so required scalar and collection fields can
// distinguish omission from valid zero and empty values during strict decoding.
type manifestJSON struct {
	SchemaVersion    int             `json:"schema_version"`
	DatasetID        string          `json:"dataset_id"`
	DatasetVersion   string          `json:"dataset_version"`
	GeneratorVersion string          `json:"generator_version"`
	Seed             *int64          `json:"seed"`
	Cases            *[]ManifestCase `json:"cases"`
}

type ManifestCase struct {
	CaseID     string      `json:"case_id"`
	Input      string      `json:"input"`
	Expected   string      `json:"expected"`
	Processors []Processor `json:"processors"`
	Tags       []string    `json:"tags"`
}

type ExpectedOutput struct {
	SchemaVersion  int                    `json:"schema_version"`
	Chunking       *ExpectedChunking      `json:"chunking,omitempty"`
	ExtractMetrics *ExpectedMetricSection `json:"extract_metrics,omitempty"`
}

type ExpectedChunking struct {
	ProtectedGroups []ProtectedGroup `json:"protected_groups"`
	Chunks          []ExpectedChunk  `json:"chunks"`
}

type ProtectedGroup struct {
	GroupID     string `json:"group_id"`
	Kind        string `json:"kind"`
	SplitPolicy string `json:"split_policy"`
	Lines       []int  `json:"lines"`
}

type ExpectedChunk struct {
	Sequence     int   `json:"sequence"`
	OverlapLines []int `json:"overlap_lines"`
	NormalLines  []int `json:"normal_lines"`
}

type ExpectedMetricSection struct {
	Metrics []GoldMetric `json:"metrics"`
}

// GoldMetric contains the stable metric fields benchmark scorers may assert. Pointers
// preserve the contract's distinction between an absent field and a present empty value.
type GoldMetric struct {
	GoldID               string          `json:"gold_id"`
	MetricName           *string         `json:"metric_name,omitempty"`
	MetricNameEn         *string         `json:"metric_name_en,omitempty"`
	MetricSubject        *string         `json:"metric_subject,omitempty"`
	MetricSubjectEn      *string         `json:"metric_subject_en,omitempty"`
	MetricValue          *string         `json:"metric_value,omitempty"`
	MetricUnit           *string         `json:"metric_unit,omitempty"`
	MetricUnitEn         *string         `json:"metric_unit_en,omitempty"`
	IsExplicitMetric     *bool           `json:"is_explicit_metric,omitempty"`
	MetricDesc           *string         `json:"metric_desc,omitempty"`
	MetricDescEn         *string         `json:"metric_desc_en,omitempty"`
	MetricContext        *string         `json:"metric_context,omitempty"`
	MetricContextEn      *string         `json:"metric_context_en,omitempty"`
	MetricKeywords       []string        `json:"metric_keywords,omitempty"`
	MetricKeywordsEn     []string        `json:"metric_keywords_en,omitempty"`
	LocationType         *string         `json:"location_type,omitempty"`
	ValueDataType        *string         `json:"value_data_type,omitempty"`
	ValueRangeType       *string         `json:"value_range_type,omitempty"`
	ValueClass           *string         `json:"value_class,omitempty"`
	ValueClassEn         *string         `json:"value_class_en,omitempty"`
	FormulaOrDefinition  *string         `json:"formula_or_definition,omitempty"`
	ThresholdOrTarget    *string         `json:"threshold_or_target,omitempty"`
	MeasurementFrequency *string         `json:"measurement_frequency,omitempty"`
	Confidence           *float64        `json:"confidence,omitempty"`
	TableNameOrSection   *string         `json:"table_name_or_section,omitempty"`
	ReasoningTags        []string        `json:"reasoning_tags,omitempty"`
	MetricCategories     json.RawMessage `json:"metric_categories,omitempty"`
	MetricCategoriesEn   json.RawMessage `json:"metric_categories_en,omitempty"`
	CategoryPaths        json.RawMessage `json:"category_paths,omitempty"`
	CategoryPathsEn      json.RawMessage `json:"category_paths_en,omitempty"`
	Objects              json.RawMessage `json:"objects,omitempty"`
	SourceLines          []int           `json:"source_lines"`
}

type DatasetCase struct {
	ManifestCase
	ExpectedOutput ExpectedOutput
	InputBytes     []byte
	ExpectedBytes  []byte
	LineNumbers    []int
}

type Dataset struct {
	Root                   string
	Manifest               Manifest
	ManifestBytes          []byte
	Cases                  []DatasetCase
	Hash                   string
	DatasetHash            string
	FileHashes             map[string]string
	ProcessorCaseSetHashes map[Processor]string
}
