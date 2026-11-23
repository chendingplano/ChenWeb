package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/llmusage"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// complianceTry is one model a structured call can be sent to.
type complianceTry struct {
	client docprocessing.LLMStructuredJSONExtractor
	model  string
	label  string // primary, correction or fallback
}

// complianceCall is one structured model call of the reviewer.
type complianceCall struct {
	Stage      string // detect, doc_activity, judgement or verify
	PromptRef  string
	PromptText string
	Input      string
	Schema     map[string]any
	Check      func(map[string]any) error
	RecordID   int64
	ChunkIndex *int
}

type complianceFailedTry struct {
	attempt int
	model   string
	err     error
	raw     string
}

// callWithTries sends a structured call to up to three models: the reviewer's own, then the
// correction model, then the fallback model. Each gets one attempt. After a wrong answer
// (an answer that fails the schema or the check), the next try gets a correction request: the original
// input plus the correction prompt filled with the latest problems and wrong answer. A try after a
// provider failure, before any wrong answer, sends the original input. The chain raises no warnings;
// every failed try is written to kb.failed_llm_calls. It returns the accepted answer and the model
// that gave it.
func (r *complianceReviewer) callWithTries(ctx context.Context, cfg ReviewerConfig, call complianceCall) (map[string]any, string, error) {
	primary, ok := r.client.(docprocessing.LLMStructuredJSONExtractor)
	if !ok {
		return nil, "", fmt.Errorf("compliance: the reviewer's model client does not support structured output")
	}
	tries := []complianceTry{{client: primary, model: cfg.ModelName, label: "primary"}, r.correction, r.fallback}
	schema, err := json.Marshal(call.Schema)
	if err != nil {
		return nil, "", err
	}
	contract := llmclients.StructuredOutputContract{Name: "chenweb_review_compliance_" + call.Stage, Schema: schema, AllowRepair: true, MaxRetries: 0, CheckParsed: call.Check}
	alertCtx, finish := llmclients.DeferCallAlerts(ctx)
	defer finish(true)

	input := call.Input
	correction := ""
	previousEventID := ""
	var failures []complianceFailedTry
	for i, try := range tries {
		if try.client == nil {
			failures = append(failures, complianceFailedTry{attempt: i + 1, model: try.model, err: fmt.Errorf("the %s model is not configured", try.label)})
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, "", context.Cause(ctx)
		}
		in := newDocReviewLLMJSONInputWithMetadata(alertCtx, call.PromptRef, call.PromptText, try.model, input,
			"review_compliance_"+call.Stage, "MID-20261122-41", map[string]any{"attempt": i + 1, "try": try.label})
		if previousEventID != "" {
			in.Metadata["correcting_call_id"] = previousEventID
		}
		if call.ChunkIndex != nil {
			in.Metadata["chunk_index"] = *call.ChunkIndex
		}
		res, err := try.client.ExtractStructuredJSON(alertCtx, in, contract)
		if err == nil && res != nil {
			r.recordFailedTries(ctx, call, cfg.ModelName, try.model, failures)
			return res.Parsed, try.model, nil
		}
		if err == nil {
			err = fmt.Errorf("empty structured result")
		}
		previousEventID = ""
		var failedCall *llmclients.StructuredOutputError
		if errors.As(err, &failedCall) {
			previousEventID = failedCall.EventID
		}
		raw := complianceRawAnswer(err)
		failures = append(failures, complianceFailedTry{attempt: i + 1, model: try.model, err: err, raw: raw})
		r.logger.Info("compliance structured call try failed", "stage", call.Stage, "record_id", call.RecordID,
			"attempt", i+1, "model", try.model, "wrong_answer", complianceWrongAnswer(err), "error", err)
		if complianceWrongAnswer(err) {
			correction = r.correctionRequest(call.Input, complianceProblems(err), raw)
		}
		if correction != "" {
			input = correction
		}
	}
	r.recordFailedTries(ctx, call, cfg.ModelName, "", failures)
	last := failures[len(failures)-1]
	return nil, "", fmt.Errorf("%s: all three tries failed; last (%s): %w", call.Stage, last.model, last.err)
}

// correctionRequest is the original input followed by the correction prompt with the problems and
// the previous answer filled in.
func (r *complianceReviewer) correctionRequest(original, problems, previous string) string {
	tmpl := r.prompts["correction"].Text
	filled := strings.NewReplacer("{{errors}}", problems, "{{previous_answer}}", previous).Replace(tmpl)
	return original + "\n\n" + strings.TrimSpace(filled)
}

func (r *complianceReviewer) recordFailedTries(ctx context.Context, call complianceCall, primaryModel, acceptedModel string, failures []complianceFailedTry) {
	if r.recordFailed == nil || len(failures) == 0 {
		return
	}
	outcome := llmusage.FailedCallFailed
	if acceptedModel != "" {
		outcome = llmusage.FailedCallRecovered
	}
	for _, f := range failures {
		r.recordFailed(ctx, llmusage.FailedCall{
			RecordID: call.RecordID, RunID: llmRunIDFromContext(ctx), ReviewRunID: reviewRunIDFromContext(ctx), Aspect: "compliance",
			Operation: "review_compliance_" + call.Stage, Prompt: call.PromptRef, Attempt: f.attempt, Model: f.model,
			PrimaryModel: primaryModel, FallbackModel: acceptedModel, Error: f.err.Error(), RawResponse: f.raw,
			Outcome: outcome, ChunkIndex: call.ChunkIndex,
		})
	}
}

// complianceWrongAnswer reports whether a structured call failed because of the answer itself
// (parse, schema or check failure), which a correction request can fix, rather than because the
// provider failed or cut the answer off.
func complianceWrongAnswer(err error) bool {
	if err == nil || errors.Is(err, llmclients.ErrStructuredOutputProvider) || errors.Is(err, llmclients.ErrStructuredOutputTruncated) {
		return false
	}
	var soe *llmclients.StructuredOutputError
	return errors.As(err, &soe) && soe.Raw != ""
}

func complianceRawAnswer(err error) string {
	var soe *llmclients.StructuredOutputError
	if errors.As(err, &soe) {
		return soe.Raw
	}
	return ""
}

// complianceProblems returns the innermost message of a structured-output error: for a check failure,
// the check's own message, which names each item and field.
func complianceProblems(err error) string {
	msg := err.Error()
	cur := err
	for {
		var soe *llmclients.StructuredOutputError
		if !errors.As(cur, &soe) || soe.Err == nil || soe.Err == cur {
			break
		}
		msg = soe.Err.Error()
		cur = soe.Err
	}
	return msg
}

type reviewRunIDKey struct{}

// withReviewRunID tags the context with the doc-review run, for the failed-calls table.
func withReviewRunID(ctx context.Context, runID int64) context.Context {
	return context.WithValue(ctx, reviewRunIDKey{}, runID)
}

func reviewRunIDFromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(reviewRunIDKey{}).(int64)
	return id
}
