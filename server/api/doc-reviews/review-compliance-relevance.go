package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// The relevance check sends one call per activity with all its candidates. For each candidate the
// model answers yes or no and gives a score from 0 to 9; the reviewer keeps a candidate whose score
// reaches the effort's minimum. The call runs at temperature 0 (every structured call does): the
// answers are written one after another, and at temperature 1 an early answer drawn at random sways
// the later ones. Chosen on 30 activities of record 825 (900 candidates) against labels from a
// reasoning model: normal (score 3 or more) kept 89% of the relevant candidates and passed about 345
// to the judgement; high (2 or more) kept all of them and passed about 600 (devdoc 2026111201).
const (
	complianceRelevanceNormalMinScore = 3
	complianceRelevanceHighMinScore   = 2
)

// complianceRelevanceMinScore maps [reviewers.compliance].effort to the lowest score a candidate needs.
// parseDocReviewConfig has already refused any value other than "", "normal" and "high".
func complianceRelevanceMinScore(effort string) int {
	if strings.TrimSpace(effort) == "high" {
		return complianceRelevanceHighMinScore
	}
	return complianceRelevanceNormalMinScore
}

// complianceRelevanceDecision is one candidate's relevance decision, as stored with the finding.
type complianceRelevanceDecision struct {
	ActivityID  string `json:"activity_id"`
	ArtifactKey string `json:"artifact_key"`
	Relevant    string `json:"relevant,omitempty"`
	Score       *int   `json:"score,omitempty"`
	Kept        bool   `json:"kept"`
}

// complianceRelevanceAnswer is the model's answer for one candidate.
type complianceRelevanceAnswer struct {
	Relevant string
	Score    int
}

// complianceRelevanceCandidate is what the relevance call sees of one candidate.
type complianceRelevanceCandidate struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text"`
}

// checkRelevance asks, in one call for the activity, whether each found candidate could decide
// whether it complies. It returns the keys it keeps, in found order, every decision, and the model
// that answered.
func (r *complianceReviewer) checkRelevance(ctx context.Context, recordID int64, chunkIndex int, cfg ReviewerConfig, m *complianceMember, found []string, pool map[string]*complianceCandidate) ([]string, []complianceRelevanceDecision, string, error) {
	if len(found) == 0 {
		return nil, nil, "", nil
	}
	keys := make([]string, len(found))
	cands := make([]complianceRelevanceCandidate, len(found))
	for i, artifact := range found {
		keys[i] = "c" + strconv.Itoa(i+1)
		c := pool[artifact]
		cands[i] = complianceRelevanceCandidate{Key: keys[i], Type: c.Type, Title: c.Title, Text: c.Text}
	}
	b, err := json.Marshal(map[string]any{"activity": complianceMemberView(m), "candidates": cands})
	if err != nil {
		return nil, nil, "", err
	}
	var answers map[string]complianceRelevanceAnswer
	p := r.prompts["relevance"]
	_, model, err := r.callWithTries(ctx, cfg, complianceCall{
		Stage: "relevance", PromptRef: p.Ref, PromptText: p.Text, Input: string(b), Schema: map[string]any{"type": "object"},
		RecordID: recordID, ChunkIndex: &chunkIndex,
		Check: func(parsed map[string]any) error {
			var cerr error
			answers, cerr = checkComplianceRelevance(parsed, keys)
			return cerr
		},
	})
	if err != nil {
		return nil, nil, "", fmt.Errorf("relevance check: %w", err)
	}
	var kept []string
	decisions := make([]complianceRelevanceDecision, len(found))
	for i, artifact := range found {
		a := answers[keys[i]]
		score := a.Score
		decisions[i] = complianceRelevanceDecision{ActivityID: m.ID, ArtifactKey: artifact, Relevant: a.Relevant, Score: &score,
			Kept: score >= r.settings.RelevanceMinScore}
		if decisions[i].Kept {
			kept = append(kept, artifact)
		}
	}
	r.logger.Info("compliance relevance check", "record_id", recordID, "activity", m.Label, "candidates", len(found),
		"kept", len(kept), "min_score", r.settings.RelevanceMinScore, "model", model)
	return kept, decisions, model, nil
}

// checkComplianceRelevance decodes one answer per candidate key and reports every problem at once,
// naming the candidate and the field.
func checkComplianceRelevance(parsed map[string]any, keys []string) (map[string]complianceRelevanceAnswer, error) {
	answers := map[string]complianceRelevanceAnswer{}
	expected := map[string]bool{}
	var problems []string
	for _, key := range keys {
		expected[key] = true
		raw, ok := parsed[key]
		if !ok {
			problems = append(problems, fmt.Sprintf(`Candidate %s: no answer; return {"relevant": "yes" or "no", "score": 0 to 9}`, key))
			continue
		}
		obj, ok := raw.(map[string]any)
		if !ok {
			problems = append(problems, fmt.Sprintf(`Candidate %s: the answer must be an object {"relevant": "yes" or "no", "score": 0 to 9}`, key))
			continue
		}
		var a complianceRelevanceAnswer
		var bad bool
		if a.Relevant, _ = obj["relevant"].(string); a.Relevant != "yes" && a.Relevant != "no" {
			problems = append(problems, fmt.Sprintf(`Candidate %s: relevant %v must be "yes" or "no"`, key, complianceJSONValue(obj["relevant"])))
			bad = true
		}
		score, isNumber := obj["score"].(float64)
		if !isNumber || score != math.Trunc(score) || score < 0 || score > 9 {
			problems = append(problems, fmt.Sprintf("Candidate %s: score %v must be a whole number from 0 to 9", key, complianceJSONValue(obj["score"])))
			bad = true
		}
		if !bad {
			a.Score = int(score)
			answers[key] = a
		}
	}
	var extra []string
	for key := range parsed {
		if !expected[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		problems = append(problems, fmt.Sprintf("%s is not a candidate key; answer only %s through %s", key, keys[0], keys[len(keys)-1]))
	}
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "\n"))
	}
	return answers, nil
}

// complianceJSONValue shows a decoded JSON value as the model wrote it, for correction messages.
func complianceJSONValue(v any) string {
	if v == nil {
		return "(missing)"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// complianceMemberView is what the relevance and judgement calls see of one activity.
func complianceMemberView(m *complianceMember) map[string]any {
	v := map[string]any{"activity_id": m.ID, "kind": m.Kind, "label": m.Label, "text": m.Text, "line_start": m.LineStart, "line_end": m.LineEnd}
	if a := m.Activity; a != nil {
		v["object"] = a.Object
		if a.Actor != "" {
			v["actor"] = a.Actor
		}
		if a.Action != "" {
			v["action"] = a.Action
		}
		if a.Purpose != "" {
			v["purpose"] = a.Purpose
		}
		if a.Relationship != "" {
			v["document_relationship"] = a.Relationship
		}
		if len(a.ContextFacts) > 0 {
			v["context_facts"] = a.ContextFacts
		}
		v["document_level"] = a.DocumentLevel
	}
	return v
}
