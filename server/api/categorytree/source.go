package categorytree

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ArtifactSource reads the artifacts of one type, with their LLM category proposals.
type ArtifactSource interface {
	Type() string
	ListRecord(ctx context.Context, recordID int64) ([]Artifact, error)
	Get(ctx context.Context, artifactID string) (*Artifact, error)
}

// MetricsSource reads kb.metrics. Proposals are the extractor's category paths
// (category_paths_en, else category_paths) and category keys (metric_categories_en, else
// metric_categories).
type MetricsSource struct{ DB *sql.DB }

func (MetricsSource) Type() string { return "metrics" }

const metricSelect = `SELECT id, input_record_id, COALESCE(metric_name, ''), COALESCE(metric_name_en, ''),
	COALESCE(metric_subject, ''), COALESCE(metric_desc, ''), COALESCE(metric_context, ''), COALESCE(metric_unit, ''),
	COALESCE(threshold_or_target, ''), COALESCE(condition, ''), category_paths_en, category_paths,
	COALESCE(metric_categories_en, ''), COALESCE(metric_categories, '')
	FROM kb.metrics`

func (s MetricsSource) ListRecord(ctx context.Context, recordID int64) ([]Artifact, error) {
	rows, err := s.DB.QueryContext(ctx, metricSelect+` WHERE input_record_id = $1 AND ref_mention_id IS NULL ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Artifact
	for rows.Next() {
		a, err := scanMetric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s MetricsSource) Get(ctx context.Context, artifactID string) (*Artifact, error) {
	id, err := strconv.ParseInt(artifactID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("metric id %q: %w", artifactID, err)
	}
	a, err := scanMetric(s.DB.QueryRowContext(ctx, metricSelect+` WHERE id = $1 AND ref_mention_id IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("metric %s: %w", artifactID, ErrNotFound)
	}
	return a, err
}

func scanMetric(row interface{ Scan(...any) error }) (*Artifact, error) {
	var id, recordID int64
	var name, nameEn, subject, desc, mctx, unit, target, cond, catsEn, cats string
	var pathsEn, paths []byte
	if err := row.Scan(&id, &recordID, &name, &nameEn, &subject, &desc, &mctx, &unit, &target, &cond,
		&pathsEn, &paths, &catsEn, &cats); err != nil {
		return nil, err
	}
	content := map[string]any{"kind": "metric", "name": name}
	for k, v := range map[string]string{"name_en": nameEn, "subject": subject, "description": desc, "context": mctx,
		"unit": unit, "value": target, "condition": cond} {
		if strings.TrimSpace(v) != "" && !(k == "name_en" && v == name) {
			content[k] = v
		}
	}
	proposals := categoryPathProposals(pathsEn)
	if len(proposals) == 0 {
		proposals = categoryPathProposals(paths)
	}
	keys := categoryKeyProposals(catsEn)
	if len(keys) == 0 {
		keys = categoryKeyProposals(cats)
	}
	proposals = appendUnique(proposals, keys...)
	lang := "en"
	if hasCJK(name) {
		lang = "zh"
	}
	return &Artifact{Type: "metrics", ID: strconv.FormatInt(id, 10), RecordID: recordID, Lang: lang,
		Content: content, Proposals: proposals}, nil
}

// categoryPathProposals renders [{category_path: [{name}...]}] as "A > B > C".
func categoryPathProposals(raw []byte) []string {
	var paths []struct {
		CategoryPath []struct {
			Name string `json:"name"`
		} `json:"category_path"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &paths) != nil {
		return nil
	}
	var out []string
	for _, p := range paths {
		var parts []string
		for _, n := range p.CategoryPath {
			if s := strings.TrimSpace(n.Name); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			out = appendUnique(out, strings.Join(parts, " > "))
		}
	}
	return out
}

// categoryKeyProposals reads a JSON string array, or a comma-separated list.
func categoryKeyProposals(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var keys []string
	if json.Unmarshal([]byte(s), &keys) != nil {
		keys = strings.Split(s, ",")
	}
	var out []string
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			out = appendUnique(out, k)
		}
	}
	return out
}

func appendUnique(list []string, items ...string) []string {
	for _, it := range items {
		dup := false
		for _, x := range list {
			if x == it {
				dup = true
				break
			}
		}
		if !dup {
			list = append(list, it)
		}
	}
	return list
}

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
