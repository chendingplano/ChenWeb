package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ListRunsByExperiment returns the immutable variant runs used by the CLI.
func (s SQLStore) ListRunsByExperiment(ctx context.Context, experimentID string) ([]RunRecord, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,experiment_id,variant_name,lifecycle,requested_json,resolved_json,config_json,prompt_json,scorer_json,pricing_json,requested_hash,resolved_hash,config_hash,prompt_hash,scorer_hash,pricing_hash,created_at,updated_at,started_at,finished_at FROM kb.benchmark_runs WHERE experiment_id=$1 ORDER BY variant_name`, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunRecord
	for rows.Next() {
		var r RunRecord
		if err := rows.Scan(&r.ID, &r.ExperimentID, &r.VariantName, &r.Lifecycle, &r.Requested, &r.Resolved, &r.Config, &r.Prompt, &r.Scorer, &r.Pricing, &r.RequestedHash, &r.ResolvedHash, &r.ConfigHash, &r.PromptHash, &r.ScorerHash, &r.PricingHash, &r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s SQLStore) reportUnits(ctx context.Context, runID string) ([]ScoreUnit, map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.case_id,c.repetition,c.tags_json,c.lifecycle,COALESCE(c.upstream_hash,''),s.metric,s.slice,s.direction,s.aggregation_kind,s.value,s.numerator,s.denominator,s.applicable FROM kb.benchmark_case_runs c LEFT JOIN kb.benchmark_scores s ON s.attempt_id=c.selected_attempt_id WHERE c.run_id=$1 ORDER BY c.case_id,c.repetition,s.processor,s.metric,s.slice,s.aggregation_kind`, runID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	type key struct {
		id         string
		repetition int
	}
	units := map[key]*ScoreUnit{}
	failures := map[string]int{}
	for rows.Next() {
		var caseID, lifecycle, upstream string
		var repetition int
		var tagsRaw json.RawMessage
		var metric, slice, direction, kind sql.NullString
		var value, numerator, denominator sql.NullFloat64
		var applicable sql.NullBool
		if err := rows.Scan(&caseID, &repetition, &tagsRaw, &lifecycle, &upstream, &metric, &slice, &direction, &kind, &value, &numerator, &denominator, &applicable); err != nil {
			return nil, nil, err
		}
		k := key{caseID, repetition}
		u := units[k]
		if u == nil {
			var provenance struct {
				Tags []string `json:"tags"`
			}
			_ = json.Unmarshal(tagsRaw, &provenance)
			u = &ScoreUnit{CaseID: caseID, Repetition: repetition, Tags: provenance.Tags, Applicable: true, UpstreamChunkHash: upstream}
			units[k] = u
			if lifecycle != "success" {
				failures[lifecycle]++
			}
		}
		if !metric.Valid {
			continue
		}
		row := ScoreRow{Metric: metric.String, Direction: direction.String, AggregationKind: kind.String, Numerator: int(numerator.Float64), Denominator: int(denominator.Float64)}
		if slice.Valid && slice.String != "" && (metric.String == "processor_success" || slice.String != "chunking" && slice.String != "extract_metrics") {
			row.Component = slice.String
		}
		if value.Valid {
			v := value.Float64
			row.Value = &v
		}
		u.Applicable = applicable.Bool
		u.Scores = append(u.Scores, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	out := make([]ScoreUnit, 0, len(units))
	for _, unit := range units {
		out = append(out, *unit)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CaseID < out[j].CaseID || out[i].CaseID == out[j].CaseID && out[i].Repetition < out[j].Repetition
	})
	return out, failures, nil
}

// BuildExperimentReport creates a deterministic report from selected attempts.
// When baseline and candidate are supplied, paired deltas are included.
func (s SQLStore) BuildExperimentReport(ctx context.Context, experimentID, baseline, candidate string, allowIncompatible bool) (BenchmarkReport, error) {
	var datasetHash string
	var caseSets json.RawMessage
	if err := s.DB.QueryRowContext(ctx, `SELECT dataset_hash,resolved_case_set_json FROM kb.benchmark_experiments WHERE id=$1`, experimentID).Scan(&datasetHash, &caseSets); err != nil {
		return BenchmarkReport{}, err
	}
	runs, err := s.ListRunsByExperiment(ctx, experimentID)
	if err != nil {
		return BenchmarkReport{}, err
	}
	report := BenchmarkReport{ID: experimentID, GeneratedAt: time.Now().UTC().Format(time.RFC3339), DatasetHash: datasetHash, ScorerVersion: MetricScorerVersion, NormalizationVersion: NormalizationVersion, PrimaryVectors: map[string][]AggregateRow{}, Completion: map[string]int{}, Failures: map[string]int{}, Provenance: map[string]string{"case_set_hash": sha256Hex(caseSets)}, NonGating: true}
	byName := map[string][]ScoreUnit{}
	for _, run := range runs {
		units, failures, err := s.reportUnits(ctx, run.ID)
		if err != nil {
			return BenchmarkReport{}, err
		}
		aggregates, err := AggregateScores(units, len(units))
		if err != nil {
			return BenchmarkReport{}, fmt.Errorf("variant %s: %w", run.VariantName, err)
		}
		report.PrimaryVectors[run.VariantName] = aggregates
		report.Completion[run.VariantName+".total"] = len(units)
		for kind, count := range failures {
			report.Failures[run.VariantName+"."+kind] += count
			report.Incomplete = true
		}
		byName[run.VariantName] = units
	}
	if baseline != "" || candidate != "" {
		if baseline == "" || candidate == "" || byName[baseline] == nil || byName[candidate] == nil {
			return BenchmarkReport{}, fmt.Errorf("baseline and candidate must name stored variants")
		}
		h := sha256Hex(caseSets)
		deltas, warnings, err := CompareVariants(VariantComparison{Baseline: byName[baseline], Candidate: byName[candidate], DatasetHash: datasetHash, BaselineCaseSetHash: h, CandidateCaseSetHash: h, ScorerVersion: MetricScorerVersion, NormalizationVersion: NormalizationVersion, AllowIncompatible: allowIncompatible})
		if err != nil {
			return BenchmarkReport{}, err
		}
		report.PairedDeltas, report.Warnings = deltas, warnings
		report.Incompatible = len(warnings) > 0
	}
	return report, nil
}
