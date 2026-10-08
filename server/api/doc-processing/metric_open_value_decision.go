package docprocessing

// Open-value decision (openspec change metric-row-soft-drop-decision-model, spec
// metric-open-value-decision). A requirement whose value is left open is either a quantity of
// an object (gold rule A4: kept) or an activity's time, frequency or method that parties must
// agree or announce (X2: not a metric). Only rows of kind requirement_value_open are judged;
// every other kind is decided deterministically.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/deepdoc/server/api/decisionmodel"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/decisionpolicy"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const (
	openValuePolicyName      = "metric_open_value_kind"
	openValueChoiceSchedule  = "activity_schedule"
	openValueDefaultDropMinP = 0.9
	openValueJudgeParallel   = 8
)

// openValueOptions are the choice question's options and what each means; the meaning is also
// recorded with every decision so a reader can see what the model chose.
var openValueOptions = map[string]string{
	openValueChoiceSchedule: "when, how often or how an activity is carried out; parties agree, set or announce it",
	"not_a_quantity":        "a method, process, practice, feature, record or identity, not a quantity",
	"object_quantity":       "a quantity of a thing, product, material, equipment, sample or test",
}

// Why a judged row was kept or dropped (ext_info.open_value_decision.reason).
const (
	openValueReasonDropped        = "activity_schedule_confident"
	openValueReasonObjectQuantity = "object_quantity"
	openValueReasonNotAQuantity   = "not_a_quantity_not_droppable"
	openValueReasonBelowThreshold = "activity_schedule_below_threshold"
	openValueReasonError          = "decision_error"
	openValueReasonNotConfigured  = "decision_model_not_configured"
)

// openValueJudge returns one decision record per row, aligned with rows. A record holds
// choice and probabilities, or error when the row could not be judged.
type openValueJudge interface {
	JudgeOpenValueRows(ctx context.Context, recordID int64, rows []map[string]any) []map[string]any
}

// judgeOpenValueRows sends every requirement_value_open row to the judge and sets aside those
// answered activity_schedule with p >= the drop threshold. Every judged row carries its
// decision: dropped rows in droppedMetricRow.Decision, kept rows in ext_info.open_value_decision.
// A row without a usable decision is always kept.
func (p *MetricsProcessor) judgeOpenValueRows(ctx context.Context, recordID int64, metrics []map[string]any) (kept []map[string]any, dropped []droppedMetricRow) {
	var judged []map[string]any
	for _, m := range metrics {
		if metricStatementKind(m) == statementKindRequirementValueOpen {
			judged = append(judged, m)
		}
	}
	if len(judged) == 0 {
		return metrics, nil
	}
	var decisions []map[string]any
	notConfigured := p.OpenValueJudge == nil
	if notConfigured {
		p.Logger.Warn("open-value decision skipped: METRIC_DECISION_MODEL not configured",
			"record_id", recordID, "rows", len(judged), "error", p.OpenValueJudgeErr)
		reason := "decision model not configured"
		if p.OpenValueJudgeErr != nil {
			reason = p.OpenValueJudgeErr.Error()
		}
		for range judged {
			decisions = append(decisions, map[string]any{"error": reason})
		}
	} else {
		decisions = p.OpenValueJudge.JudgeOpenValueRows(ctx, recordID, judged)
	}
	minP := p.OpenValueDropMinP
	if minP <= 0 {
		minP = openValueDefaultDropMinP
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	droppedRows := map[string]bool{}
	for i, m := range judged {
		var decision map[string]any
		if i < len(decisions) {
			decision = decisions[i]
		}
		if decision == nil {
			decision = map[string]any{"error": "no decision returned"}
		}
		if errMsg := asString(decision["error"]); errMsg != "" && p.OpenValueJudge != nil {
			p.Logger.Warn("open-value decision failed; row kept",
				"record_id", recordID, "candidate_id", m["candidate_id"], "metric_name", m["metric_name"], "error", errMsg)
		}
		drop := annotateOpenValueDecision(decision, minP, notConfigured, now())
		if drop {
			dropped = append(dropped, droppedMetricRow{Row: m, Stage: metricDropStageDecision,
				Reason: openValueChoiceSchedule, Kind: statementKindRequirementValueOpen, Decision: decision})
			droppedRows[fmt.Sprintf("%p", m)] = true
			continue
		}
		ext, _ := m["ext_info"].(map[string]any)
		if ext == nil {
			ext = map[string]any{}
		}
		ext["open_value_decision"] = decision
		m["ext_info"] = ext
	}
	if len(dropped) == 0 {
		return metrics, nil
	}
	kept = make([]map[string]any, 0, len(metrics)-len(dropped))
	for _, m := range metrics {
		if !droppedRows[fmt.Sprintf("%p", m)] {
			kept = append(kept, m)
		}
	}
	return kept, dropped
}

// annotateOpenValueDecision decides whether a judged row is set aside (choice activity_schedule
// with probability >= minP, and no error) and records the verdict in the decision itself:
// examined, outcome (kept|dropped), reason (a code), reason_text, threshold, statement_kind,
// choice_meaning and judged_at. The decision is stored with the row (kb.metrics_dropped.decision
// or kb.metrics.ext_info.open_value_decision), so every judged row says why it was kept.
func annotateOpenValueDecision(decision map[string]any, minP float64, notConfigured bool, now time.Time) (drop bool) {
	choice := asString(decision["choice"])
	probs, _ := decision["probabilities"].(map[string]float64)
	p := probs[choice]
	var reason, text string
	switch {
	case notConfigured:
		reason, text = openValueReasonNotConfigured, "kept: no decision model is configured (METRIC_DECISION_MODEL)"
	case asString(decision["error"]) != "":
		reason, text = openValueReasonError, "kept: the decision call failed, and a failed decision never drops a row"
	case choice == openValueChoiceSchedule && p >= minP:
		drop = true
		reason, text = openValueReasonDropped, fmt.Sprintf("dropped: judged activity_schedule with p=%.2f >= %.2f", p, minP)
	case choice == openValueChoiceSchedule:
		reason, text = openValueReasonBelowThreshold, fmt.Sprintf("kept: judged activity_schedule but p=%.2f < %.2f", p, minP)
	case choice == "not_a_quantity":
		reason, text = openValueReasonNotAQuantity, fmt.Sprintf("kept: judged not_a_quantity (p=%.2f), which is recorded but never dropped", p)
	default:
		reason, text = openValueReasonObjectQuantity, fmt.Sprintf("kept: judged %s (p=%.2f), a quantity of an object", choice, p)
	}
	decision["examined"] = !notConfigured
	decision["outcome"] = map[bool]string{true: "dropped", false: "kept"}[drop]
	decision["reason"] = reason
	decision["reason_text"] = text
	decision["threshold"] = minP
	decision["statement_kind"] = statementKindRequirementValueOpen
	decision["judged_at"] = now.UTC().Format(time.RFC3339)
	if meaning, ok := openValueOptions[choice]; ok {
		decision["choice_meaning"] = meaning
	}
	return drop
}

// decisionModelJudge asks the configured decision model (jev_emulated or jev_compatible) one
// choice question per row, with the current version of policy metric_open_value_kind.
type decisionModelJudge struct {
	client       llmclients.Client
	modelName    string
	profile      string
	policies     *decisionpolicy.Store
	policySeed   string // prompt file text used when the policy does not exist yet
	policySeedID string // prompt file name, for the version note
	logger       ApiTypes.JimoLogger
}

// newOpenValueJudgeFromEnv builds the judge from METRIC_DECISION_MODEL (a .models.toml profile)
// and the policy prompt. It returns (nil, nil) when METRIC_DECISION_MODEL is unset.
func newOpenValueJudgeFromEnv(logger ApiTypes.JimoLogger) (openValueJudge, error) {
	ref := strings.TrimSpace(os.Getenv("METRIC_DECISION_MODEL"))
	if ref == "" {
		return nil, nil
	}
	modelPath, err := resolveModelsFilePath("MODEL_DEF_FILE")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26100811) read %s: %w", modelPath, err)
	}
	parsed := ApiTypes.LLMModelsFile{}
	if err := parseTOMLMap(raw, &parsed); err != nil {
		return nil, fmt.Errorf("(MID_26100812) parse %s: %w", modelPath, err)
	}
	def, ok := parsed[ref]
	if !ok {
		return nil, fmt.Errorf("(MID_26100813) METRIC_DECISION_MODEL %q not found in %s", ref, modelPath)
	}
	pc, err := decisionmodel.ProviderConfig(ref, def)
	if err != nil {
		return nil, fmt.Errorf("(MID_26100814) %w", err)
	}
	client, err := llmclients.NewClient(pc, logger)
	if err != nil {
		return nil, fmt.Errorf("(MID_26100815) decision client for %q: %w", ref, err)
	}
	if ApiTypes.SharedDBHandle == nil {
		return nil, errors.New("(MID_26100816) shared database is not available for the decision policy store")
	}
	seed, seedRef, _, err := loadProductPromptFromEnvKeys([]string{"METRIC_OPEN_VALUE_POLICY_PROMPT"}, "prompt-metric-open-value-policy-v1.md")
	if err != nil {
		return nil, fmt.Errorf("(MID_26100817) load open-value policy prompt: %w", err)
	}
	return &decisionModelJudge{
		client:       client,
		modelName:    strings.TrimSpace(def.ModelName),
		profile:      ref,
		policies:     decisionpolicy.NewStore(ApiTypes.SharedDBHandle, logger),
		policySeed:   seed,
		policySeedID: seedRef,
		logger:       logger,
	}, nil
}

// openValueDropMinPFromEnv reads METRIC_DECISION_DROP_MIN_P; invalid or unset → default.
func openValueDropMinPFromEnv() float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("METRIC_DECISION_DROP_MIN_P")), 64); err == nil && v > 0 && v <= 1 {
		return v
	}
	return openValueDefaultDropMinP
}

// currentPolicy returns the current policy version, creating version 1 from the prompt file
// the first time the policy does not exist.
func (j *decisionModelJudge) currentPolicy(ctx context.Context) (*decisionpolicy.PolicyVersion, error) {
	v, err := j.policies.GetCurrent(ctx, openValuePolicyName)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, decisionpolicy.ErrNotFound) {
		return nil, err
	}
	if _, err := j.policies.Create(ctx, decisionpolicy.CreateInput{
		Name:        openValuePolicyName,
		Description: "extract_metrics: is a requirement's open value a quantity of an object, an activity schedule, or not a quantity",
		Content:     j.policySeed,
		Note:        "seeded from " + j.policySeedID,
		Actor:       "extract_metrics",
	}); err != nil && !errors.Is(err, decisionpolicy.ErrNameTaken) {
		return nil, err
	}
	return j.policies.GetCurrent(ctx, openValuePolicyName)
}

func (j *decisionModelJudge) JudgeOpenValueRows(ctx context.Context, recordID int64, rows []map[string]any) []map[string]any {
	out := make([]map[string]any, len(rows))
	policy, err := j.currentPolicy(ctx)
	if err != nil {
		for i := range out {
			out[i] = map[string]any{"error": fmt.Sprintf("load policy %s: %v", openValuePolicyName, err)}
		}
		return out
	}
	question := llmclients.JevQuestions{"kind": {
		Type:         "choice",
		Instructions: "Under `policy`, what does the open value of `row` belong to?",
		Criteria:     openValueOptions,
	}}
	sem := make(chan struct{}, openValueJudgeParallel)
	var wg sync.WaitGroup
	for i, row := range rows {
		wg.Add(1)
		go func(i int, row map[string]any) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			decision := map[string]any{
				"model": j.modelName, "profile": j.profile,
				"policy_id": policy.PolicyID, "policy_version": policy.Version,
			}
			state, _ := json.Marshal(map[string]any{"policy": policy.Content, "row": map[string]any{
				"metric_name":         row["metric_name"],
				"subject":             row["subject"],
				"threshold_or_target": row["threshold_or_target"],
				"desc":                row["desc"],
				"context":             row["context"],
			}})
			resp, err := j.client.Complete(ctx, llmclients.Request{
				Model:        j.modelName,
				UserID:       "extract_metrics",
				RecordID:     recordID,
				RunID:        llmRunIDFromContext(ctx),
				PromptName:   openValuePolicyName,
				CallReason:   "extract_metrics",
				CallLoc:      "MID-26100818",
				Metadata:     map[string]any{"policy_id": policy.PolicyID, "policy_version": policy.Version, "candidate_id": row["candidate_id"]},
				Messages:     []llmclients.Message{{Role: llmclients.RoleUser, Content: string(state)}},
				JevQuestions: question,
			})
			if err == nil {
				var answers map[string]llmclients.JevAnswer
				if answers, err = llmclients.ParseJevAnswers(resp.Content); err == nil {
					decision["choice"] = answers["kind"].Choice
					decision["probabilities"] = answers["kind"].Probabilities
				}
			}
			if err != nil {
				decision["error"] = err.Error()
			}
			out[i] = decision
		}(i, row)
	}
	wg.Wait()
	return out
}
