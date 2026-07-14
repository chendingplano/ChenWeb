package docbenchmark

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const ChunkScorerVersion = "chunk-scorer-v1"

type EvidenceProcessor struct {
	Capture      json.RawMessage `json:"capture,omitempty"`
	CaptureError string          `json:"capture_error,omitempty"`
	Actual       json.RawMessage `json:"actual,omitempty"`
	Diagnostics  json.RawMessage `json:"diagnostics,omitempty"`
}

// EvidenceBundle is the single immutable attempt artifact. Raw production
// bytes live inside processor captures, alongside canonical actuals and all
// immutable scoring inputs required for a zero-runtime rescore.
type EvidenceBundle struct {
	SchemaVersion int                          `json:"schema_version"`
	AttemptID     string                       `json:"attempt_id"`
	InputSHA256   string                       `json:"input_sha256"`
	InputBytes    []byte                       `json:"input_bytes"`
	ExpectedJSON  json.RawMessage              `json:"expected_json"`
	ConfigJSON    json.RawMessage              `json:"config_json"`
	ConfigHash    string                       `json:"config_hash"`
	ScorerJSON    json.RawMessage              `json:"scorer_json"`
	ScorerHash    string                       `json:"scorer_hash"`
	Processors    map[string]EvidenceProcessor `json:"processors"`
}

func (b EvidenceBundle) CanonicalJSON() ([]byte, error) {
	if b.SchemaVersion != 1 || b.AttemptID == "" || b.InputSHA256 == "" {
		return nil, fmt.Errorf("invalid evidence bundle identity")
	}
	if b.ScorerHash != sha256Hex(b.ScorerJSON) {
		return nil, fmt.Errorf("evidence bundle scorer hash mismatch")
	}
	raw, err := canonicalJSON(b)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	if path, ok := secretPath(decoded, ""); ok {
		return nil, fmt.Errorf("evidence bundle contains secret-shaped field %q", path)
	}
	return canonicalJSON(decoded)
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func loadVerifiedEvidence(path, expectedHash string, expectedSize int64) (EvidenceBundle, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return EvidenceBundle{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return EvidenceBundle{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != expectedSize {
		return EvidenceBundle{}, fmt.Errorf("source evidence file identity mismatch")
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return EvidenceBundle{}, err
	}
	if sha256Hex(raw) != expectedHash {
		return EvidenceBundle{}, fmt.Errorf("source evidence hash mismatch")
	}
	var bundle EvidenceBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return EvidenceBundle{}, err
	}
	canonical, err := bundle.CanonicalJSON()
	if err != nil {
		return EvidenceBundle{}, err
	}
	if string(canonical) != string(raw) {
		return EvidenceBundle{}, fmt.Errorf("source evidence is not canonical")
	}
	return bundle, nil
}

func lineFileGeneratedPayload(recordID int64, filename string, processors []Processor) ([]byte, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	operations := make([]string, 0, len(processors))
	seen := map[Processor]bool{}
	for _, processor := range processors {
		if !seen[processor] {
			seen[processor] = true
			operations = append(operations, string(processor))
		}
	}
	return canonicalJSON(map[string]any{
		"record_id": recordID, "filename": abs, "force": true, "force_clear": true,
		"type": "pdf", "status": "success", "operation": operations,
	})
}

func chunkScoreRecords(attemptID string, score ChunkScore) ([]ScoreRecord, error) {
	diagnostics, err := canonicalJSON(score.Diagnostics)
	if err != nil {
		return nil, err
	}
	metrics := []struct {
		name      string
		direction string
		kind      string
		value     ScoreMetric
	}{
		{"exact_sequence_match", "higher", "binary_macro", score.ExactSequenceMatch},
		{"exact_case_pass", "higher", "binary_macro", score.ExactCasePass},
		{"boundary_precision", "higher", "count_derived_micro", score.BoundaryPrecision},
		{"boundary_recall", "higher", "count_derived_micro", score.BoundaryRecall},
		{"boundary_f1", "higher", "count_derived_micro", score.BoundaryF1},
		{"normal_coverage", "higher", "count_derived_micro", score.NormalCoverage},
		{"missing_rate", "lower", "count_derived_micro", score.MissingRate},
		{"extra_rate", "lower", "count_derived_micro", score.ExtraRate},
		{"duplicate_rate", "lower", "count_derived_micro", score.DuplicateRate},
		{"reordered_rate", "lower", "count_derived_micro", score.ReorderedRate},
		{"overlap_precision", "higher", "count_derived_micro", score.OverlapPrecision},
		{"overlap_recall", "higher", "count_derived_micro", score.OverlapRecall},
		{"overlap_f1", "higher", "count_derived_micro", score.OverlapF1},
	}
	out := make([]ScoreRecord, 0, len(metrics)+len(score.RuleCounts)+2)
	for _, metric := range metrics {
		record := ScoreRecord{
			AttemptID: sql.NullString{String: attemptID, Valid: true}, Processor: string(ProcessorChunking),
			Scorer: "chunk", ScorerVersion: ChunkScorerVersion, Metric: metric.name, Slice: "",
			Direction: metric.direction, AggregationKind: metric.kind, Applicable: true, Metadata: diagnostics,
			Numerator:   sql.NullFloat64{Float64: float64(metric.value.Numerator), Valid: true},
			Denominator: sql.NullFloat64{Float64: float64(metric.value.Denominator), Valid: true},
		}
		if metric.value.Value != nil {
			record.Value = sql.NullFloat64{Float64: *metric.value.Value, Valid: true}
			record.NonNull = true
		}
		out = append(out, record)
	}
	counts := map[string]int{"cases_with_any_hard_violation": score.CasesWithAnyHardViolation, "hard_violation_count": score.HardViolationCount}
	for rule, value := range score.RuleCounts {
		counts["rule."+rule] = value
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := float64(counts[key])
		out = append(out, ScoreRecord{
			AttemptID: sql.NullString{String: attemptID, Valid: true}, Processor: string(ProcessorChunking),
			Scorer: "chunk", ScorerVersion: ChunkScorerVersion, Metric: key, Direction: "lower",
			AggregationKind: "additive_count", Value: sql.NullFloat64{Float64: value, Valid: true},
			AdditiveComponent: sql.NullFloat64{Float64: value, Valid: true}, NonNull: true,
			Applicable: true, Metadata: diagnostics,
		})
	}
	return out, nil
}
