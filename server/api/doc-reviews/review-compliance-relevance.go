package docreviews

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/decisionmodel"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// complianceRelevanceThreshold: a candidate whose probability of being relevant is below it is removed.
// Chosen on the first live run (record 825, lines 1-200, 42 activities, 819 judged candidates, the
// judgement's verdicts as reference): with prompt-review-compliance-relevance-v2.md, 0.2 removed 74% of
// the candidates the judgement found unusable and kept 91% of the useful ones; 0.5 kept only 84%.
const complianceRelevanceThreshold = 0.2

// complianceQuestionHeading separates the relevance prompt's policy from its per-candidate question,
// which holds complianceQuestionKey where the candidate's key goes.
const (
	complianceQuestionHeading = "\n## Question\n"
	complianceQuestionKey     = "{{key}}"
)

// complianceRelevanceRequest asks whether each candidate could apply to one activity.
type complianceRelevanceRequest struct {
	RecordID   int64
	Activity   map[string]any
	Candidates []*complianceCandidate
}

// complianceRelevanceDecider returns, per candidate key, the probability that the candidate is
// relevant. A key without an answer is absent.
type complianceRelevanceDecider interface {
	Decide(ctx context.Context, req complianceRelevanceRequest) (map[string]float64, error)
	Model() string
}

// complianceRelevanceDecision is one candidate's relevance decision, as stored with the finding.
type complianceRelevanceDecision struct {
	ActivityID  string   `json:"activity_id"`
	ArtifactKey string   `json:"artifact_key"`
	Probability *float64 `json:"probability,omitempty"`
	Kept        bool     `json:"kept"`
}

// jevComplianceRelevance asks a decision model one yes-or-no question per candidate, all in one
// request (the pattern of categorytree.JevDecider).
type jevComplianceRelevance struct {
	client                llmclients.Client
	model, profile        string
	policy, question, ref string
}

// newComplianceRelevanceDeciderFromEnv builds the decider from DOC_REVIEWER_RELEVANCE_MODEL_NAME (a
// profile in MODEL_DEF_FILE of type decision-model or llm) and the relevance prompt.
func newComplianceRelevanceDeciderFromEnv(logger ApiTypes.JimoLogger, prompt compliancePrompt) (complianceRelevanceDecider, error) {
	ref := strings.TrimSpace(os.Getenv(envComplianceRelevanceModel))
	if ref == "" {
		return nil, fmt.Errorf("%s is not set (the decision model of the Compliance Review relevance check)", envComplianceRelevanceModel)
	}
	policy, question, ok := strings.Cut(prompt.Text, complianceQuestionHeading)
	if !ok || !strings.Contains(question, complianceQuestionKey) {
		return nil, fmt.Errorf("relevance_prompt %s needs a %q heading followed by a question containing %s", prompt.Ref, strings.TrimSpace(complianceQuestionHeading), complianceQuestionKey)
	}
	path, err := docprocessing.ResolveModelsFilePath("MODEL_DEF_FILE")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", envComplianceRelevanceModel, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s: %w", envComplianceRelevanceModel, path, err)
	}
	models := ApiTypes.LLMModelsFile{}
	if err := docprocessing.ParseTOMLMap(raw, &models); err != nil {
		return nil, fmt.Errorf("%s: parse %s: %w", envComplianceRelevanceModel, path, err)
	}
	def, ok := models[ref]
	if !ok {
		return nil, fmt.Errorf("%s=%q is not a profile in %s", envComplianceRelevanceModel, ref, path)
	}
	pc, err := decisionmodel.ProviderConfig(ref, def)
	if err != nil {
		return nil, fmt.Errorf("%s=%q: %w", envComplianceRelevanceModel, ref, err)
	}
	client, err := llmclients.NewClient(pc, logger)
	if err != nil {
		return nil, fmt.Errorf("%s=%q: decision client: %w", envComplianceRelevanceModel, ref, err)
	}
	return &jevComplianceRelevance{client: client, model: strings.TrimSpace(def.ModelName), profile: ref,
		policy: strings.TrimSpace(policy), question: strings.TrimSpace(question), ref: prompt.Ref}, nil
}

func (d *jevComplianceRelevance) Model() string { return d.profile }

func (d *jevComplianceRelevance) Decide(ctx context.Context, req complianceRelevanceRequest) (map[string]float64, error) {
	if len(req.Candidates) == 0 {
		return map[string]float64{}, nil
	}
	cands := map[string]any{}
	keyToArtifact := map[string]string{}
	questions := llmclients.JevQuestions{}
	for i, c := range req.Candidates {
		key := fmt.Sprintf("c%d", i+1)
		keyToArtifact[key] = c.Key
		cands[key] = map[string]any{"type": c.Type, "title": c.Title, "text": c.Text}
		questions[key] = llmclients.JevQuestion{Type: "noul", Instructions: strings.ReplaceAll(d.question, complianceQuestionKey, key)}
	}
	state, err := json.Marshal(map[string]any{"policy": d.policy, "activity": req.Activity, "candidates": cands})
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Complete(ctx, llmclients.Request{
		Model:        d.model,
		UserID:       docprocessing.LLMUserIDFromContext(ctx),
		RecordID:     req.RecordID,
		RunID:        llmRunIDFromContext(ctx),
		PromptName:   d.ref,
		CallReason:   "review_compliance_relevance",
		CallLoc:      "MID-20261122-51",
		Messages:     []llmclients.Message{{Role: llmclients.RoleUser, Content: string(state)}},
		JevQuestions: questions,
	})
	if err != nil {
		return nil, fmt.Errorf("relevance check (%s): %w", d.profile, err)
	}
	answers, err := llmclients.ParseJevAnswers(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("relevance check (%s): %w", d.profile, err)
	}
	out := map[string]float64{}
	for key, a := range answers {
		if artifact, ok := keyToArtifact[key]; ok && a.Noul != nil {
			out[artifact] = *a.Noul
		}
	}
	return out, nil
}

// checkRelevance asks whether each found candidate could apply to the activity and returns the
// keys it keeps, in found order, and every decision. A candidate without an answer is kept.
func (r *complianceReviewer) checkRelevance(ctx context.Context, recordID int64, m *complianceMember, found []string, pool map[string]*complianceCandidate) ([]string, []complianceRelevanceDecision, error) {
	if len(found) == 0 {
		return nil, nil, nil
	}
	cands := make([]*complianceCandidate, 0, len(found))
	for _, key := range found {
		cands = append(cands, pool[key])
	}
	req := complianceRelevanceRequest{RecordID: recordID, Activity: complianceMemberView(m), Candidates: cands}
	probs, err := r.relevance.Decide(ctx, req)
	if err != nil && ctx.Err() == nil {
		// One retry: the first live run lost three activities to dropped connections.
		r.logger.Info("compliance relevance request failed; retrying once", "record_id", recordID, "activity", m.Label, "error", err)
		probs, err = r.relevance.Decide(ctx, req)
	}
	if err != nil {
		return nil, nil, err
	}
	var kept []string
	decisions := make([]complianceRelevanceDecision, 0, len(found))
	for _, key := range found {
		d := complianceRelevanceDecision{ActivityID: m.ID, ArtifactKey: key, Kept: true}
		if p, ok := probs[key]; ok {
			p := p
			d.Probability = &p
			d.Kept = p >= complianceRelevanceThreshold
		}
		if d.Kept {
			kept = append(kept, key)
		}
		decisions = append(decisions, d)
	}
	return kept, decisions, nil
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
