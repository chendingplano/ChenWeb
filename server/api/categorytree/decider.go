package categorytree

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

const (
	envDecisionModel     = "CATEGORY_DECISION_MODEL"
	defaultDecisionModel = "deepseek-flash-decision-model"
	envPolicyPrompt      = "CATEGORY_MEMBERSHIP_POLICY_PROMPT"
	envQuestionPrompt    = "CATEGORY_MEMBERSHIP_QUESTION_PROMPT"
	defaultPolicyRef     = "prompt-category-membership-policy-v1.md"
	defaultQuestion      = "prompt-category-membership-question-v1.md"
	questionKeyMarker    = "{{key}}"
)

// JevDecider asks a decision model (Jev, or a chat model through jev_emulated) one noul
// question per candidate child (design D7).
type JevDecider struct {
	Client      llmclients.Client
	ModelName   string
	Profile     string
	Policy      string
	PolicyRef   string
	Question    string // contains {{key}}
	QuestionRef string
}

// NewJevDeciderFromEnv builds the decider from CATEGORY_DECISION_MODEL (a .models.toml
// profile of type decision-model or llm; default deepseek-flash-decision-model) and the two prompt files.
func NewJevDeciderFromEnv(logger ApiTypes.JimoLogger) (*JevDecider, error) {
	ref := envOr(envDecisionModel, defaultDecisionModel)
	path, err := docprocessing.ResolveModelsFilePath("MODEL_DEF_FILE")
	if err != nil {
		return nil, fmt.Errorf("(MID_26102102) %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("(MID_26102103) read %s: %w", path, err)
	}
	models := ApiTypes.LLMModelsFile{}
	if err := docprocessing.ParseTOMLMap(raw, &models); err != nil {
		return nil, fmt.Errorf("(MID_26102104) parse %s: %w", path, err)
	}
	def, ok := models[ref]
	if !ok {
		return nil, fmt.Errorf("(MID_26102105) %s %q not found in %s", envDecisionModel, ref, path)
	}
	pc, err := decisionmodel.ProviderConfig(ref, def)
	if err != nil {
		return nil, fmt.Errorf("(MID_26102106) %w", err)
	}
	client, err := llmclients.NewClient(pc, logger)
	if err != nil {
		return nil, fmt.Errorf("(MID_26102107) decision client for %q: %w", ref, err)
	}
	policy, policyRef, _, err := docprocessing.LoadPromptByRef(envOr(envPolicyPrompt, defaultPolicyRef))
	if err != nil {
		return nil, fmt.Errorf("(MID_26102108) load policy prompt: %w", err)
	}
	question, questionRef, _, err := docprocessing.LoadPromptByRef(envOr(envQuestionPrompt, defaultQuestion))
	if err != nil {
		return nil, fmt.Errorf("(MID_26102109) load question prompt: %w", err)
	}
	if !strings.Contains(question, questionKeyMarker) {
		return nil, fmt.Errorf("(MID_26102110) question prompt %s has no %s placeholder", questionRef, questionKeyMarker)
	}
	return &JevDecider{Client: client, ModelName: strings.TrimSpace(def.ModelName), Profile: ref,
		Policy: policy, PolicyRef: policyRef, Question: strings.TrimSpace(question), QuestionRef: questionRef}, nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// PromptRefs names the prompt files, for the classification record.
func (d *JevDecider) PromptRefs() map[string]string {
	return map[string]string{"policy": d.PolicyRef, "question": d.QuestionRef}
}

// categoryView is what the model sees of a category, in the artifact's language.
type categoryView struct {
	Label      string   `json:"label"`
	AltLabels  []string `json:"alt_labels,omitempty"`
	Definition string   `json:"definition"`
	ScopeNote  string   `json:"scope_note,omitempty"`
}

func viewOf(n *Node, lang string) categoryView {
	pick := func(m LangText) string {
		if s := strings.TrimSpace(m[lang]); s != "" {
			return s
		}
		return m["en"]
	}
	alts := n.AltLabels[lang]
	if len(alts) == 0 {
		alts = n.AltLabels["en"]
	}
	return categoryView{Label: n.Label(lang), AltLabels: alts, Definition: pick(n.Definitions), ScopeNote: pick(n.ScopeNotes)}
}

// Decide sends one request for the batch and maps the answers back to node IDs.
func (d *JevDecider) Decide(ctx context.Context, req DecisionRequest) (map[string]float64, error) {
	lang := req.Artifact.Lang
	cats := map[string]categoryView{}
	keyToID := map[string]string{}
	questions := llmclients.JevQuestions{}
	for i, c := range req.Children {
		key := fmt.Sprintf("c%d", i+1)
		keyToID[key] = c.ID
		cats[key] = viewOf(c, lang)
		questions[key] = llmclients.JevQuestion{Type: "noul", Instructions: strings.ReplaceAll(d.Question, questionKeyMarker, key)}
	}
	state, err := json.Marshal(map[string]any{
		"policy":     d.Policy,
		"artifact":   req.Artifact.Content,
		"proposals":  req.Artifact.Proposals,
		"parent":     viewOf(req.Parent, lang),
		"categories": cats,
	})
	if err != nil {
		return nil, err
	}
	resp, err := d.Client.Complete(ctx, llmclients.Request{
		Model:        d.ModelName,
		UserID:       "category_classification",
		RecordID:     req.Artifact.RecordID,
		PromptName:   "category_membership",
		CallReason:   "category_classification",
		CallLoc:      "MID-26102111",
		Metadata:     map[string]any{"artifact_type": req.Artifact.Type, "artifact_id": req.Artifact.ID, "parent_id": req.Parent.ID},
		Messages:     []llmclients.Message{{Role: llmclients.RoleUser, Content: string(state)}},
		JevQuestions: questions,
	})
	if err != nil {
		return nil, err
	}
	answers, err := llmclients.ParseJevAnswers(resp.Content)
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for key, a := range answers {
		id, ok := keyToID[key]
		if !ok || a.Noul == nil {
			continue // unknown key or no answer: the child stays uncertain
		}
		out[id] = *a.Noul
	}
	return out, nil
}
